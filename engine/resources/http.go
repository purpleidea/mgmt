// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/safepath"

	securefilepath "github.com/cyphar/filepath-securejoin"
)

const (
	// HTTPUseSecureJoin specifies that we should add in a "secure join" lib
	// so that we avoid the ../../etc/passwd and symlink problems.
	HTTPUseSecureJoin = true

	httpKind       = "http"
	httpServerKind = httpKind + ":server"
	httpFileKind   = httpKind + ":file"
)

func init() {
	engine.RegisterResource(httpServerKind, func() engine.Res { return &HTTPServerRes{} })
	engine.RegisterResource(httpFileKind, func() engine.Res { return &HTTPFileRes{} })
}

// HTTPServerGroupableRes is the interface that you must implement if you want
// to allow a resource the ability to be grouped into the http server resource.
// As an added safety, the Kind must also begin with "http:", and not have more
// than one colon, or it must begin with http:server:, and not have any further
// colons to avoid accidents of unwanted grouping.
type HTTPServerGroupableRes interface {
	engine.Res

	// ParentName is used to limit which resources autogroup into this one.
	// If it's empty then it's ignored, otherwise it must match the Name of
	// the parent to get grouped.
	ParentName() string

	// AcceptHTTP determines whether this will respond to this request.
	// Return nil to accept, or any error to pass. This should be
	// deterministic (pure) and fast.
	AcceptHTTP(req *http.Request) error

	// ServeHTTP is the standard HTTP handler that will be used for this.
	http.Handler // ServeHTTP(w http.ResponseWriter, req *http.Request)
}

// HTTPServerRes is an http server resource. It serves files, but does not
// actually apply any state. The name is used as the address to listen on,
// unless the Address field is specified, and in that case it is used instead.
// This resource can offer up files for serving that are specified either inline
// in this resource by specifying an http root, or as http:file resources which
// will get autogrouped into this resource at runtime. The two methods can be
// combined as well.
//
// This server also supports autogrouping some more magical resources into it.
// For example, the http:flag and http:ui resources add in magic endpoints.
//
// This server is not meant as a featureful replacement for the venerable and
// modern httpd servers out there, but rather as a simple, dynamic, integrated
// alternative for bootstrapping new machines and clusters in an elegant way.
//
// TODO: add support for TLS
// XXX: Add an http:flag resource that lets an http client set a flag somewhere!
// XXX: Add a http:ui resource that functions can read data from!
// XXX: The http:ui resource can also take in values from those functions!
type HTTPServerRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can have HTTPFileRes and others grouped into it

	init *engine.Init

	// Address is the listen address to use for the http server. It is
	// common to use `:80` (the standard) to listen on TCP port 80 on all
	// addresses.
	Address string `mcl:"address" yaml:"address"`

	// Timeout is the maximum duration in seconds to use for unspecified
	// timeouts. In other words, when this value is specified, it is used as
	// the value for the other *Timeout values when they aren't used. Put
	// another way, this makes it easy to set all the different timeouts
	// with a single parameter.
	Timeout *uint64 `mcl:"timeout" yaml:"timeout"`

	// ReadTimeout is the maximum duration in seconds for reading during the
	// http request. If it is zero, then there is no timeout. If this is
	// unspecified, then the value of Timeout is used instead if it is set.
	// For more information, see the golang net/http Server documentation.
	ReadTimeout *uint64 `mcl:"read_timeout" yaml:"read_timeout"`

	// WriteTimeout is the maximum duration in seconds for writing during
	// the http request. If it is zero, then there is no timeout. If this is
	// unspecified, then the value of Timeout is used instead if it is set.
	// For more information, see the golang net/http Server documentation.
	WriteTimeout *uint64 `mcl:"write_timeout" yaml:"write_timeout"`

	// ShutdownTimeout is the maximum duration in seconds to wait for the
	// server to shutdown gracefully before calling Close. By default it is
	// nice to let client connections terminate gracefully, however it might
	// take longer than we are willing to wait, particularly if one is long
	// polling or running a very long download. As a result, you can set a
	// timeout here. The default is zero which means it will wait
	// indefinitely. The shutdown process can also be cancelled by the
	// interrupt handler which this resource supports. If this is
	// unspecified, then the value of Timeout is used instead if it is set.
	ShutdownTimeout *uint64 `mcl:"shutdown_timeout" yaml:"shutdown_timeout"`

	// Root is the root directory that we should serve files from. If it is
	// not specified, then it is not used. Any http file resources will have
	// precedence over anything in here, in case the same path exists twice.
	// TODO: should we have a flag to determine the precedence rules here?
	Root string `mcl:"root" yaml:"root"`

	// TODO: should we allow adding a list of one-of files directly here?

	eventsChanMap map[engine.Res]chan error
	interruptChan chan struct{}

	conn     net.Listener
	serveMux *http.ServeMux // can't share the global one between resources!
	server   *http.Server
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerRes) Default() engine.Res {
	return &HTTPServerRes{}
}

// getAddress returns the actual address to use. When Address is not specified,
// we use the Name.
func (obj *HTTPServerRes) getAddress() string {
	if obj.Address != "" {
		return obj.Address
	}
	return obj.Name()
}

// getReadTimeout determines the value for ReadTimeout, because if unspecified,
// this will default to the value of Timeout.
func (obj *HTTPServerRes) getReadTimeout() *uint64 {
	if obj.ReadTimeout != nil {
		return obj.ReadTimeout
	}
	return obj.Timeout // might be nil
}

// getWriteTimeout determines the value for WriteTimeout, because if
// unspecified, this will default to the value of Timeout.
func (obj *HTTPServerRes) getWriteTimeout() *uint64 {
	if obj.WriteTimeout != nil {
		return obj.WriteTimeout
	}
	return obj.Timeout // might be nil
}

// getShutdownTimeout determines the value for ShutdownTimeout, because if
// unspecified, this will default to the value of Timeout.
func (obj *HTTPServerRes) getShutdownTimeout() *uint64 {
	if obj.ShutdownTimeout != nil {
		return obj.ShutdownTimeout
	}
	return obj.Timeout // might be nil
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass. In this particular case, it accepts for the
// Root directory handler, but it happens to be implemented with this signature
// in case it gets moved. It doesn't intentionally match the
// HTTPServerGroupableRes interface.
func (obj *HTTPServerRes) AcceptHTTP(req *http.Request) error {
	// Look in root if we have one, and we haven't got a file yet...
	if obj.Root == "" {
		return fmt.Errorf("no Root directory")
	}
	return nil
}

// ServeHTTP is the standard HTTP handler that will be used here. In this
// particular case, it serves the Root directory handler, but it happens to be
// implemented with this signature in case it gets moved. It doesn't
// intentionally match the HTTPServerGroupableRes interface.
func (obj *HTTPServerRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow GET at the moment.
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	requestPath := req.URL.Path // TODO: is this what we want here?

	p := filepath.Join(obj.Root, requestPath) // normal unsafe!
	if !strings.HasPrefix(p, obj.Root) {      // root ends with /
		// user might have tried a ../../etc/passwd hack
		obj.init.Logf("join inconsistency: %s", p)
		http.NotFound(w, req) // lie to them...
		return
	}
	if HTTPUseSecureJoin {
		var err error
		p, err = securefilepath.SecureJoin(obj.Root, requestPath)
		if err != nil {
			obj.init.Logf("secure join fail: %s", p)
			http.NotFound(w, req) // lie to them...
			return
		}
	}
	if obj.init.Debug {
		obj.init.Logf("Got file at root: %s", p)
	}

	handle, err := os.Open(p)
	if err != nil {
		obj.init.Logf("could not open: %s", p)
		sendHTTPError(w, err)
		return
	}
	defer handle.Close() // ignore error

	// Determine the last-modified time if we can.
	modtime := time.Now()
	fi, err := handle.Stat()
	if err == nil {
		modtime = fi.ModTime()
	}
	// TODO: if Stat errors, should we fail the whole thing?

	// XXX: is requestPath what we want for the name field?
	http.ServeContent(w, req, requestPath, modtime, handle)
	//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?

	return
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPServerRes) Validate() error {
	if obj.getAddress() == "" {
		return fmt.Errorf("empty address")
	}

	host, _, err := net.SplitHostPort(obj.getAddress())
	if err != nil {
		return errwrap.Wrapf(err, "the Address is in an invalid format: %s", obj.getAddress())
	}
	if host != "" {
		// TODO: should we allow fqdn's here?
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("the Address is not a valid IP: %s", host)
		}
	}

	if obj.Root != "" && !strings.HasPrefix(obj.Root, "/") {
		return fmt.Errorf("the Root must be absolute")
	}
	if obj.Root != "" && !strings.HasSuffix(obj.Root, "/") {
		return fmt.Errorf("the Root must be a dir")
	}

	// XXX: validate that the autogrouped resources don't have paths that
	// conflict with each other. We can only have a single unique entry for
	// what handles a /whatever URL.

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPServerRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// No need to error in Validate if Timeout is ignored, but log it.
	// These are all specified, so Timeout effectively does nothing.
	a := obj.ReadTimeout != nil
	b := obj.WriteTimeout != nil
	c := obj.ShutdownTimeout != nil
	if obj.Timeout != nil && (a && b && c) {
		obj.init.Logf("the Timeout param is being ignored")
	}

	// NOTE: If we don't Init anything that's autogrouped, then it won't
	// even get an Init call on it.

	obj.eventsChanMap = make(map[engine.Res]chan error)

	// TODO: should we do this in the engine? Do we want to decide it here?
	for _, res := range obj.GetGroup() { // grouped elements
		// NOTE: We build a new init, but it's not complete. We only add
		// what we're planning to use, and we ignore the rest for now...
		r := res // bind the variable!

		obj.eventsChanMap[r] = make(chan error)
		event := func() {
			select {
			case obj.eventsChanMap[r] <- nil:
				// send!
			}
			// We don't do this here (why?) we instead read from the
			// above channel and then send on multiplexedChan to the
			// main loop, where it runs the obj.init.Event function.
			//obj.init.Event() // notify engine of an event (this can block)
		}

		newInit := &engine.Init{
			Program:  obj.init.Program,
			Version:  obj.init.Version,
			Hostname: obj.init.Hostname,

			// Watch:
			Running: event,
			Event:   event,

			// CheckApply:
			Refresh: func() bool {
				innerRes, ok := r.(engine.RefreshableRes)
				if !ok {
					panic("res does not support the Refreshable trait")
				}
				return innerRes.Refresh()
			},
			Send: engine.GenerateSendFunc(r),
			Recv: engine.GenerateRecvFunc(r), // unused

			FilteredGraph: func() (*pgraph.Graph, error) {
				panic("FilteredGraph for HTTP not implemented")
			},

			Local: obj.init.Local,
			World: obj.init.World,
			//VarDir: obj.init.VarDir, // TODO: wrap this

			Debug: obj.init.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.init.Logf(r.String()+": "+format, v...)
			},
		}

		if err := res.Init(newInit); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}
	}

	obj.interruptChan = make(chan struct{})

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPServerRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *HTTPServerRes) Watch(ctx context.Context) error {
	// TODO: I think we could replace all this with:
	//obj.conn, err := net.Listen("tcp", obj.getAddress())
	// ...but what is the advantage?
	addr, err := net.ResolveTCPAddr("tcp", obj.getAddress())
	if err != nil {
		return errwrap.Wrapf(err, "could not resolve address")
	}

	obj.conn, err = net.ListenTCP("tcp", addr)
	if err != nil {
		return errwrap.Wrapf(err, "could not start listener")
	}
	defer obj.conn.Close()

	obj.serveMux = http.NewServeMux() // do it here in case Watch restarts!
	// TODO: We could consider having the obj.GetGroup loop here, instead of
	// essentially having our own "router" API with AcceptHTTP.
	obj.serveMux.HandleFunc("/", obj.handler())

	readTimeout := uint64(0)
	if i := obj.getReadTimeout(); i != nil {
		readTimeout = *i
	}
	writeTimeout := uint64(0)
	if i := obj.getWriteTimeout(); i != nil {
		writeTimeout = *i
	}
	obj.server = &http.Server{
		Addr:         obj.getAddress(),
		Handler:      obj.serveMux,
		ReadTimeout:  time.Duration(readTimeout) * time.Second,
		WriteTimeout: time.Duration(writeTimeout) * time.Second,
		//MaxHeaderBytes: 1 << 20, XXX: should we add a param for this?
	}

	multiplexedChan := make(chan error)
	defer close(multiplexedChan) // closes after everyone below us is finished

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	for _, r := range obj.GetGroup() { // grouped elements
		res := r // optional in newer golang
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(obj.eventsChanMap[res]) // where Watch sends events
			if err := res.Watch(ctx); err != nil {
				select {
				case multiplexedChan <- err:
				case <-ctx.Done():
				}
			}
		}()
		// wait for Watch first Running() call or immediate error...
		select {
		case <-obj.eventsChanMap[res]: // triggers on start or on err...
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var ok bool
				var err error
				select {
				// receive
				case err, ok = <-obj.eventsChanMap[res]:
					if !ok {
						return
					}
				}

				// send (multiplex)
				select {
				case multiplexedChan <- err:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	// we block until all the children are started first...

	obj.init.Running() // when started, notify engine that we're running

	var closeError error
	closeSignal := make(chan struct{})

	shutdownChan := make(chan struct{}) // server shutdown finished signal
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.interruptChan:
			// TODO: should we bubble up the error from Close?
			// TODO: do we need a mutex around this Close?
			obj.server.Close() // kill it quickly!
		case <-shutdownChan:
			// let this exit
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(closeSignal)

		err := obj.server.Serve(obj.conn) // blocks until Shutdown() is called!
		if err == nil || err == http.ErrServerClosed {
			return
		}
		// if this returned on its own, then closeSignal can be used...
		closeError = errwrap.Wrapf(err, "the server errored")
	}()

	// When Shutdown is called, Serve, ListenAndServe, and ListenAndServeTLS
	// immediately return ErrServerClosed. Make sure the program doesn't
	// exit and waits instead for Shutdown to return.
	defer func() {
		defer close(shutdownChan) // signal that shutdown is finished
		innerCtx := context.Background()
		if i := obj.getShutdownTimeout(); i != nil && *i > 0 {
			var cancel context.CancelFunc
			innerCtx, cancel = context.WithTimeout(innerCtx, time.Duration(*i)*time.Second)
			defer cancel()
		}
		err := obj.server.Shutdown(innerCtx) // shutdown gracefully
		if err == context.DeadlineExceeded {
			// TODO: should we bubble up the error from Close?
			// TODO: do we need a mutex around this Close?
			obj.server.Close() // kill it now
		}
	}()

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil
			send = true

		case err, ok := <-multiplexedChan:
			if !ok { // shouldn't happen
				multiplexedChan = nil
				continue
			}
			if err != nil {
				return err
			}
			send = true

		case <-closeSignal: // something shut us down early
			return closeError

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply never has anything to do for this resource, so it always succeeds.
// It does however check that certain runtime requirements (such as the Root dir
// existing if one was specified) are fulfilled. If there are any autogrouped
// resources, those will be recursively called so that they can send/recv.
func (obj *HTTPServerRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	// XXX: We don't want the initial CheckApply to return true until the
	// Watch has started up, so we must block here until that's the case...

	// Cheap runtime validation!
	// XXX: maybe only do this only once to avoid repeated, unnecessary checks?
	if obj.Root != "" {
		fileInfo, err := os.Stat(obj.Root)
		if err != nil {
			return false, errwrap.Wrapf(err, "can't stat Root dir")
		}
		if !fileInfo.IsDir() {
			return false, fmt.Errorf("the Root path is not a dir")
		}
	}

	checkOK := true
	for _, res := range obj.GetGroup() { // grouped elements
		if c, err := res.CheckApply(ctx, apply); err != nil {
			return false, errwrap.Wrapf(err, "autogrouped CheckApply failed")
		} else if !c {
			checkOK = false
		}
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerRes to others of the same resource kind
	res, ok := r.(*HTTPServerRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Address != res.Address {
		return fmt.Errorf("the Address differs")
	}

	if (obj.Timeout == nil) != (res.Timeout == nil) { // xor
		return fmt.Errorf("the Timeout differs")
	}
	if obj.Timeout != nil && res.Timeout != nil {
		if *obj.Timeout != *res.Timeout { // compare the values
			return fmt.Errorf("the value of Timeout differs")
		}
	}
	if (obj.ReadTimeout == nil) != (res.ReadTimeout == nil) {
		return fmt.Errorf("the ReadTimeout differs")
	}
	if obj.ReadTimeout != nil && res.ReadTimeout != nil {
		if *obj.ReadTimeout != *res.ReadTimeout {
			return fmt.Errorf("the value of ReadTimeout differs")
		}
	}
	if (obj.WriteTimeout == nil) != (res.WriteTimeout == nil) {
		return fmt.Errorf("the WriteTimeout differs")
	}
	if obj.WriteTimeout != nil && res.WriteTimeout != nil {
		if *obj.WriteTimeout != *res.WriteTimeout {
			return fmt.Errorf("the value of WriteTimeout differs")
		}
	}
	if (obj.ShutdownTimeout == nil) != (res.ShutdownTimeout == nil) {
		return fmt.Errorf("the ShutdownTimeout differs")
	}
	if obj.ShutdownTimeout != nil && res.ShutdownTimeout != nil {
		if *obj.ShutdownTimeout != *res.ShutdownTimeout {
			return fmt.Errorf("the value of ShutdownTimeout differs")
		}
	}

	// TODO: We could do this sort of thing to skip checking Timeout when it
	// is not used, but for the moment, this is overkill and not needed yet.
	//a := obj.ReadTimeout != nil
	//b := obj.WriteTimeout != nil
	//c := obj.ShutdownTimeout != nil
	//if !(obj.Timeout != nil && (a && b && c)) {
	//	// the Timeout param is not being ignored
	//}

	if obj.Root != res.Root {
		return fmt.Errorf("the Root differs")
	}

	return nil
}

// Interrupt is called to ask the execution of this resource to end early. It
// will cause the server Shutdown to end abruptly instead of leading open client
// connections terminate gracefully. It does this by causing the server Close
// method to run.
func (obj *HTTPServerRes) Interrupt() error {
	close(obj.interruptChan) // this should cause obj.server.Close() to run!
	return nil
}

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *HTTPServerRes) Copy() engine.CopyableRes {
	var timeout, readTimeout, writeTimeout, shutdownTimeout *uint64
	if obj.Timeout != nil {
		x := *obj.Timeout
		timeout = &x
	}
	if obj.ReadTimeout != nil {
		x := *obj.ReadTimeout
		readTimeout = &x
	}
	if obj.WriteTimeout != nil {
		x := *obj.WriteTimeout
		writeTimeout = &x
	}
	if obj.ShutdownTimeout != nil {
		x := *obj.ShutdownTimeout
		shutdownTimeout = &x
	}
	return &HTTPServerRes{
		Address:         obj.Address,
		Timeout:         timeout,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ShutdownTimeout: shutdownTimeout,
		Root:            obj.Root,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*HTTPServerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerRes(raw) // restore from indirection with type conversion!
	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *HTTPServerRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(HTTPServerGroupableRes) // different from what we usually do!
	if !ok {
		return fmt.Errorf("resource is not the right kind")
	}

	// If the http resource has the parent name field specified, then it
	// must match against our name field if we want it to group with us.
	if pn := res.ParentName(); pn != "" && pn != obj.Name() {
		return fmt.Errorf("resource groups with a different parent name")
	}

	// http:foo is okay, but file or config:etcd is not
	if !strings.HasPrefix(r.Kind(), httpKind+":") {
		return fmt.Errorf("not one of our children")
	}

	// http:server:foo is okay, but http:server:foo:bar is not
	p1 := httpServerKind + ":"
	s1 := strings.TrimPrefix(r.Kind(), p1)
	if len(s1) != len(r.Kind()) && strings.Count(s1, ":") > 0 { // has prefix
		return fmt.Errorf("maximum one resource after `%s` prefix", httpServerKind)
	}

	// http:foo is okay, but http:foo:bar is not
	p2 := httpKind + ":"
	s2 := strings.TrimPrefix(r.Kind(), p2)
	if len(s2) != len(r.Kind()) && strings.Count(s2, ":") > 0 { // has prefix
		return fmt.Errorf("maximum one resource after `%s` prefix", httpKind)
	}

	return nil
}

// readHandler handles all the incoming download requests from clients.
func (obj *HTTPServerRes) handler() func(http.ResponseWriter, *http.Request) {
	// TODO: we could statically pre-compute some stuff here...

	return func(w http.ResponseWriter, req *http.Request) {

		if obj.init.Debug {
			obj.init.Logf("Client: %s", req.RemoteAddr)
		}
		// TODO: would this leak anything security sensitive in our log?
		obj.init.Logf("URL: %s", req.URL)
		requestPath := req.URL.Path // TODO: is this what we want here?
		if obj.init.Debug {
			obj.init.Logf("Path: %s", requestPath)
		}

		// Look through the autogrouped resources!
		// TODO: can we improve performance by only searching here once?
		for _, x := range obj.GetGroup() { // grouped elements
			res, ok := x.(HTTPServerGroupableRes) // convert from Res
			if !ok {
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("Got grouped resource: %s", res.String())
			}

			err := res.AcceptHTTP(req)
			if err == nil {
				res.ServeHTTP(w, req)
				return
			}
			if obj.init.Debug {
				obj.init.Logf("Could not serve: %+v", err)
			}

			//continue // not me
		}

		// Look in root if we have one, and we haven't got a file yet...
		err := obj.AcceptHTTP(req)
		if err == nil {
			obj.ServeHTTP(w, req)
			return
		}
		if obj.init.Debug {
			obj.init.Logf("Could not serve Root: %+v", err)
		}

		// We never found something to serve...
		if obj.init.Debug || true { // XXX: maybe we should always do this?
			obj.init.Logf("File not found: %s", requestPath)
		}
		http.NotFound(w, req)
		return
	}
}

// HTTPFileRes is a file that exists within an http server. The name is used as
// the public path of the file, unless the filename field is specified, and in
// that case it is used instead. The way this works is that it autogroups at
// runtime with an existing http resource, and in doing so makes the file
// associated with this resource available for serving from that http server.
type HTTPFileRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes

	init *engine.Init

	// Server is the name of the http server resource to group this into. If
	// it is omitted, and there is only a single http resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `mcl:"server" yaml:"server"`

	// Filename is the name of the file this data should appear as on the
	// http server.
	Filename string `mcl:"filename" yaml:"filename"`

	// Path is the absolute path to a file that should be used as the source
	// for this file resource. It must not be combined with the data field.
	// If this corresponds to a directory, then it will used as a root dir
	// that will be served as long as the resource name or Filename are also
	// a directory ending with a slash.
	Path string `mcl:"path" yaml:"path"`

	// Data is the file content that should be used as the source for this
	// file resource. It must not be combined with the path field.
	// TODO: should this be []byte instead?
	Data string `mcl:"data" yaml:"data"`
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPFileRes) Default() engine.Res {
	return &HTTPFileRes{}
}

// getPath returns the actual path we respond to. When Filename is not
// specified, we use the Name. Note that this is the filename that will be seen
// on the http server, it is *not* the source path to the actual file contents
// being sent by the server.
func (obj *HTTPFileRes) getPath() string {
	if obj.Filename != "" {
		return obj.Filename
	}
	return obj.Name()
}

// getContent returns the content that we expect from this resource. It depends
// on whether the user specified the Path or Data fields, and whether the Path
// exists or not.
func (obj *HTTPFileRes) getContent(requestPath safepath.AbsPath) (io.ReadSeeker, error) {
	if obj.Path != "" && obj.Data != "" {
		// programming error! this should have been caught in Validate!
		return nil, fmt.Errorf("must not specify Path and Data")
	}

	if obj.Data != "" {
		return bytes.NewReader([]byte(obj.Data)), nil
	}

	absFile, err := obj.getContentRelative(requestPath)
	if err != nil { // on error, we just assume no root/prefix stuff happens
		return os.Open(obj.Path)
	}

	return os.Open(absFile.Path())
}

// getContentRelative takes a request, and returns the absolute path to the file
// that we want to request, if it's safely under what we can provide.
func (obj *HTTPFileRes) getContentRelative(requestPath safepath.AbsPath) (safepath.AbsFile, error) {
	// the location on disk of the data
	srcPath, err := safepath.SmartParseIntoPath(obj.Path) // (safepath.Path, error)
	if err != nil {
		return safepath.AbsFile{}, err
	}
	srcAbsDir, ok := srcPath.(safepath.AbsDir)
	if !ok {
		return safepath.AbsFile{}, fmt.Errorf("the Path is not an abs dir")
	}

	// the public path we respond to (might be a dir prefix or just a file)
	pubPath, err := safepath.SmartParseIntoPath(obj.getPath()) // (safepath.Path, error)
	if err != nil {
		return safepath.AbsFile{}, err
	}
	pubAbsDir, ok := pubPath.(safepath.AbsDir)
	if !ok {
		return safepath.AbsFile{}, fmt.Errorf("the name is not an abs dir")
	}

	// is the request underneath what we're providing?
	if !safepath.HasPrefix(requestPath, pubAbsDir) {
		return safepath.AbsFile{}, fmt.Errorf("wrong prefix")
	}

	// make the delta
	delta, err := safepath.StripPrefix(requestPath, pubAbsDir) // (safepath.Path, error)
	if err != nil {
		return safepath.AbsFile{}, err
	}
	relFile, ok := delta.(safepath.RelFile)
	if !ok {
		return safepath.AbsFile{}, fmt.Errorf("the delta is not a rel file")
	}

	return safepath.JoinToAbsFile(srcAbsDir, relFile), nil // AbsFile
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPFileRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPFileRes) AcceptHTTP(req *http.Request) error {
	requestPath := req.URL.Path // TODO: is this what we want here?

	if strings.HasSuffix(obj.Path, "/") { // a dir!
		if strings.HasPrefix(requestPath, obj.getPath()) {
			// relative dir root
			return nil
		}
	}

	if requestPath != obj.getPath() {
		return fmt.Errorf("unhandled path")
	}
	return nil
}

// ServeHTTP is the standard HTTP handler that will be used here.
func (obj *HTTPFileRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow GET at the moment.
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	requestPath := req.URL.Path // TODO: is this what we want here?

	absPath, err := safepath.ParseIntoAbsPath(requestPath)
	if err != nil {
		obj.init.Logf("invalid input path: %s", requestPath)
		sendHTTPError(w, err)
		return
	}

	handle, err := obj.getContent(absPath)
	if err != nil {
		obj.init.Logf("could not get content for: %s", requestPath)
		sendHTTPError(w, err)
		return
	}
	//if readSeekCloser, ok := handle.(io.ReadSeekCloser); ok { // same
	//	defer readSeekCloser.Close() // ignore error
	//}
	if closer, ok := handle.(io.Closer); ok {
		defer closer.Close() // ignore error
	}

	// Determine the last-modified time if we can.
	modtime := time.Now()
	if f, ok := handle.(*os.File); ok {
		fi, err := f.Stat()
		if err == nil {
			modtime = fi.ModTime()
		}
		// TODO: if Stat errors, should we fail the whole thing?
	}

	// XXX: is requestPath what we want for the name field?
	http.ServeContent(w, req, requestPath, modtime, handle)
	//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPFileRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("empty filename")
	}
	// FIXME: does getPath need to start with a slash?

	if obj.Path != "" && !strings.HasPrefix(obj.Path, "/") {
		return fmt.Errorf("the Path must be absolute")
	}

	if obj.Path != "" && obj.Data != "" {
		return fmt.Errorf("must not specify Path and Data")
	}

	// NOTE: if obj.Path == "" && obj.Data == "" then we have an empty file!

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPFileRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPFileRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPFileRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPFileRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPFileRes) Cmp(r engine.Res) error {
	// we can only compare HTTPFileRes to others of the same resource kind
	res, ok := r.(*HTTPFileRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Filename != res.Filename {
		return fmt.Errorf("the Filename differs")
	}
	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}
	if obj.Data != res.Data {
		return fmt.Errorf("the Data differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPFileRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPFileRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*HTTPFileRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPFileRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPFileRes(raw) // restore from indirection with type conversion!
	return nil
}

// httpError represents a specific HTTP error to send, but can be stored as an
// internal golang `error` type.
type httpError struct {
	msg  string
	code int
}

// Error is required to implement the `error` type interface.
func (obj *httpError) Error() string {
	return strconv.Itoa(obj.code) + " " + obj.msg
}

// newHTTPError generates a new httpError based on a single status code. It gets
// the msg text from the http.StatusText method.
func newHTTPError(code int) error {
	return &httpError{
		msg:  http.StatusText(code),
		code: code,
	}
}

// toHTTPError returns a non-specific HTTP error message and status code for a
// given non-nil error value. It's important that toHTTPError does not actually
// return err.Error(), since msg and httpStatus are returned to users, and
// historically Go's ServeContent always returned just "404 Not Found" for all
// errors. We don't want to start leaking information in error messages.
// NOTE: This was copied and modified slightly from the golang net/http package.
// See: https://github.com/golang/go/issues/38375
func toHTTPError(err error) (msg string, httpStatus int) {
	if e, ok := err.(*httpError); ok {
		return e.msg, e.code
	}
	if os.IsNotExist(err) {
		//return "404 page not found", http.StatusNotFound
		return http.StatusText(http.StatusNotFound), http.StatusNotFound
	}
	if os.IsPermission(err) {
		//return "403 Forbidden", http.StatusForbidden
		return http.StatusText(http.StatusForbidden), http.StatusForbidden
	}
	// Default:
	//return "500 Internal Server Error", http.StatusInternalServerError
	return http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError
}

// sendHTTPError is a helper function for sending an http error response.
func sendHTTPError(w http.ResponseWriter, err error) {
	msg, httpStatus := toHTTPError(err)
	http.Error(w, msg, httpStatus)
}
