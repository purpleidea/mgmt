// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	securefilepath "github.com/cyphar/filepath-securejoin"
	"github.com/pin/tftp"
)

func init() {
	engine.RegisterResource("tftp:server", func() engine.Res { return &TftpServerRes{} })
	engine.RegisterResource("tftp:file", func() engine.Res { return &TftpFileRes{} })
}

const (
	// TftpDefaultTimeout is the default timeout in seconds for server
	// connections.
	TftpDefaultTimeout = 5

	// TftpUseSecureJoin specifies that we should add in a "secure join" lib
	// so that we avoid the ../../etc/passwd and symlink problems.
	TftpUseSecureJoin = true
)

// TftpServerRes is a tftp server resource. It serves files, but does not
// actually apply any state. The name is used as the address to listen on,
// unless the Address field is specified, and in that case it is used instead.
// This resource can offer up files for serving that are specified either inline
// in this resource by specifying a tftp root, or as tftp:file resources which
// will get autogrouped into this resource at runtime. The two methods can be
// combined as well.
type TftpServerRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can have TftpFileRes grouped into it

	init *engine.Init

	// Address is the listen address to use for the tftp server. It is
	// common to use `:69` (the standard) to listen on UDP port 69 on all
	// addresses.
	Address string `lang:"address" yaml:"address"`

	// Timeout is the timeout in seconds to use for server connections.
	Timeout uint64 `lang:"timeout" yaml:"timeout"`

	// Root is the root directory that we should serve files from. If it is
	// not specified, then it is not used. Any tftp file resources will have
	// precedence over anything in here, in case the same path exists twice.
	// TODO: should we have a flag to determine the precedence rules here?
	Root string `lang:"root" yaml:"root"`

	// TODO: should we allow adding a list of one-of files directly here?
}

// Default returns some sensible defaults for this resource.
func (obj *TftpServerRes) Default() engine.Res {
	return &TftpServerRes{
		Timeout: TftpDefaultTimeout,
	}
}

// getAddress returns the actual address to use. When Address is not specified,
// we use the Name.
func (obj *TftpServerRes) getAddress() string {
	if obj.Address != "" {
		return obj.Address
	}
	return obj.Name()
}

// Validate checks if the resource data structure was populated correctly.
func (obj *TftpServerRes) Validate() error {
	if obj.getAddress() == "" {
		return fmt.Errorf("empty address")
	}

	// FIXME: parse getAddress and ensure it's in a legal format

	if obj.Root != "" && !strings.HasPrefix(obj.Root, "/") {
		return fmt.Errorf("the Root must be absolute")
	}
	if obj.Root != "" && !strings.HasSuffix(obj.Root, "/") {
		return fmt.Errorf("the Root must be a dir")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *TftpServerRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *TftpServerRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TftpServerRes) Watch() error {
	addr, err := net.ResolveUDPAddr("udp", obj.getAddress())
	if err != nil {
		return errwrap.Wrapf(err, "could not resolve address")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return errwrap.Wrapf(err, "could not start listener")
	}
	defer conn.Close()

	hook := obj.hook()
	if hook == nil {
		return fmt.Errorf("the hook is nil") // programming error
	}

	obj.init.Running() // when started, notify engine that we're running

	// Use nil in place of handler to disable read or write operations.
	server := tftp.NewServer(obj.readHandler(), obj.writeHandler())
	server.SetTimeout(time.Duration(obj.Timeout) * time.Second) // optional
	server.SetHook(hook)

	var closeError error
	closeSignal := make(chan struct{})

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(closeSignal)

		err := server.Serve(conn) // blocks until Shutdown() is called!
		if err == nil {
			return
		}
		// if this returned on its own, then closeSignal can be used...
		closeError = errwrap.Wrapf(err, "the server errored")
	}()
	defer server.Shutdown()

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("%s: Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil
			send = true

		case <-closeSignal: // something shut us down early
			return closeError

		case <-obj.init.Done: // closed by the engine to signal shutdown
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
// existing if one was specified) are fulfilled.
func (obj *TftpServerRes) CheckApply(apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	// Cheap runtime validation!
	if obj.Root != "" {
		fileInfo, err := os.Stat(obj.Root)
		if err != nil {
			return false, errwrap.Wrapf(err, "can't stat Root dir")
		}
		if !fileInfo.IsDir() {
			return false, fmt.Errorf("the Root path is not a dir")
		}
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TftpServerRes) Cmp(r engine.Res) error {
	// we can only compare TftpServerRes to others of the same resource kind
	res, ok := r.(*TftpServerRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Address != res.Address {
		return fmt.Errorf("the Address differs")
	}
	if obj.Timeout != res.Timeout {
		return fmt.Errorf("the Timeout differs")
	}
	if obj.Root != res.Root {
		return fmt.Errorf("the Root differs")
	}

	return nil
}

//// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
//// TODO: should this copy internal state?
//func (obj *TftpServerRes) Copy() engine.CopyableRes {
//	return &TftpServerRes{
//		Address: obj.Address,
//		Timeout: obj.Timeout,
//		Root:    obj.Root,
//	}
//}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *TftpServerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TftpServerRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*TftpServerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TftpServerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TftpServerRes(raw) // restore from indirection with type conversion!
	return nil
}

// GroupCmp returns whether two resources can be grouped together or not.
// Can these two resources be merged, aka, does this resource support doing so?
// Will resource allow itself to be grouped _into_ this obj?
func (obj *TftpServerRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*TftpFileRes) // different from what we usually do!
	if !ok {
		return fmt.Errorf("resource is not the right kind")
	}

	// If the tftp file resource has the Server field specified, then it
	// must match against our name field if we want it to group with us.
	if res.Server != "" && res.Server != obj.Name() {
		return fmt.Errorf("resource groups with a different server name")
	}

	return nil
}

// readHandler handles all the incoming download requests from clients.
func (obj *TftpServerRes) readHandler() func(string, io.ReaderFrom) error {
	return func(filename string, rf io.ReaderFrom) error {
		raddr := rf.(tftp.OutgoingTransfer).RemoteAddr()
		laddr := rf.(tftp.RequestPacketInfo).LocalIP() // may be nil
		if obj.init.Debug {
			s := "<unknown>"
			if laddr != nil {
				s = laddr.String()
			}
			obj.init.Logf("Client: %s Server: %s", raddr.String(), s)
		}

		obj.init.Logf("Read: %s", filename)

		//var handle io.Reader // TODO: simplify?
		var handle io.ReadSeeker

		// Look through the autogrouped resources!
		// TODO: can we improve performance by only searching here once?
		for _, x := range obj.GetGroup() { // grouped elements
			res, ok := x.(*TftpFileRes) // convert from Res
			if !ok {
				continue
			}
			if filename != res.getFilename() {
				continue // not me
			}

			if obj.init.Debug {
				obj.init.Logf("Got grouped file: %s", res.String())
			}
			var err error
			handle, err = res.getContent()
			if err != nil {
				obj.init.Logf("could not get content for: %s", filename)
				// don't leak additional information to client!
				return fmt.Errorf("could not get content for: %s", filename)
			}
			break
		}

		// Look in root if we have one, and we haven't got a file yet...
		if obj.Root != "" && handle == nil {
			// We build a common error so the client can't tell the
			// difference between their lame path hack failing, and
			// a missing file which isn't actually there...
			openError := fmt.Errorf("could not open: %s", filename)

			p := filepath.Join(obj.Root, filename) // normal unsafe!
			if !strings.HasPrefix(p, obj.Root) {   // root ends with /
				// user might have tried a ../../etc/passwd hack
				obj.init.Logf("join inconsistency: %s", p)
				return openError // match this to below error...
			}
			if TftpUseSecureJoin {
				var err error
				p, err = securefilepath.SecureJoin(obj.Root, filename)
				if err != nil {
					obj.init.Logf("secure join fail: %s", p)
					return openError // match this to below error...
				}
			}
			if obj.init.Debug {
				obj.init.Logf("Got file at root: %s", p)
			}
			var err error
			handle, err = os.Open(p)
			if err != nil {
				obj.init.Logf("could not open: %s", p)
				// don't leak the full path with Root to client!
				return openError // don't differentiate the err!
			}
		}

		// We never found a file...
		if handle == nil {
			if obj.init.Debug {
				obj.init.Logf("Never found file: %s", filename)
			}
			// don't leak additional information to client!
			return errwrap.Wrapf(os.ErrNotExist, "file: %s", filename)
		}

		// Set transfer size before calling ReadFrom if the thing we're
		// passing to ReadFrom doesn't support the io.Seeker interface.
		// NOTE: os.File does for example.
		//rf.(tftp.OutgoingTransfer).SetSize(myFileSize)

		n, err := rf.ReadFrom(handle)
		if err != nil {
			obj.init.Logf("could not read %s, error: %+v", filename, err)
			// don't leak additional information to client!
			return fmt.Errorf("could not read: %s", filename)

		}
		obj.init.Logf("%d bytes sent", n)
		return nil
	}
}

// writeHandler handles all the incoming upload requests from clients.
func (obj *TftpServerRes) writeHandler() func(string, io.WriterTo) error {
	// Use nil in place of handler function to disable that operation.
	return nil // not implemented
}

// hook is a helper function to build the tftp.Hook that we'd like to use. It
// must not be called before Init.
func (obj *TftpServerRes) hook() tftp.Hook {
	if obj.init == nil {
		return nil // should not happen
	}
	return &hook{
		debug: obj.init.Debug,
		logf: func(format string, v ...interface{}) {
			obj.init.Logf("tftp: "+format, v...)
		},
	}
}

// hook is a struct that implements the tftp.Hook interface. When we build it we
// pass in a debug flag and a logging handle, in case we want to log some stuff.
type hook struct {
	debug bool
	logf  func(format string, v ...interface{})
}

// OnSuccess is called by the tftp server if a transfer succeeds.
func (obj *hook) OnSuccess(stats tftp.TransferStats) {
	if !obj.debug {
		return
	}
	obj.logf("transfer success: %+v", stats)
}

// OnFailure is called by the tftp server if a transfer fails.
func (obj *hook) OnFailure(stats tftp.TransferStats, err error) {
	if !obj.debug {
		return
	}
	obj.logf("transfer failure: %+v", stats)
}

// TftpFileRes is a file that exists within a tftp server. The name is used as
// the public path of the file, unless the filename field is specified, and in
// that case it is used instead. The way this works is that it autogroups at
// runtime with an existing tftp resource, and in doing so makes the file
// associated with this resource available for serving from that tftp server.
type TftpFileRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into TftpServerRes

	init *engine.Init

	// Server is the name of the tftp server resource to group this into. If
	// it is omitted, and there is only a single tftp resource, then it will
	// be grouped into it automatically. If there is more than one main tftp
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Filename is the name of the file this data should appear as on the
	// tftp server.
	Filename string `lang:"filename" yaml:"filename"`

	// Path is the absolute path to a file that should be used as the source
	// for this file resource. It must not be combined with the data field.
	Path string `lang:"path" yaml:"path"`

	// Data is the file content that should be used as the source for this
	// file resource. It must not be combined with the path field.
	// TODO: should this be []byte instead?
	Data string `lang:"data" yaml:"data"`
}

// Default returns some sensible defaults for this resource.
func (obj *TftpFileRes) Default() engine.Res {
	return &TftpFileRes{}
}

// getFilename returns the actual filename to use. When Filename is not
// specified, we use the Name. Note that this is the filename that will be seen
// on the tftp server, it is *not* the source path to the actual file contents
// being sent by the server.
func (obj *TftpFileRes) getFilename() string {
	if obj.Filename != "" {
		return obj.Filename
	}
	return obj.Name()
}

// getContent returns the content that we expect from this resource. It depends
// on whether the user specified the Path or Data fields, and whether the Path
// exists or not.
func (obj *TftpFileRes) getContent() (io.ReadSeeker, error) {
	if obj.Path != "" && obj.Data != "" {
		// programming error! this should have been caught in Validate!
		return nil, fmt.Errorf("must not specify Path and Data")
	}

	if obj.Path != "" {
		return os.Open(obj.Path)
	}

	return bytes.NewReader([]byte(obj.Data)), nil
}

// Validate checks if the resource data structure was populated correctly.
func (obj *TftpFileRes) Validate() error {
	if obj.getFilename() == "" {
		return fmt.Errorf("empty filename")
	}
	// FIXME: does getFilename need to start with a slash?

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
func (obj *TftpFileRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *TftpFileRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *TftpFileRes) Watch() error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-obj.init.Done: // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *TftpFileRes) CheckApply(apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TftpFileRes) Cmp(r engine.Res) error {
	// we can only compare TftpFileRes to others of the same resource kind
	res, ok := r.(*TftpFileRes)
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

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *TftpFileRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TftpFileRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*TftpFileRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TftpFileRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TftpFileRes(raw) // restore from indirection with type conversion!
	return nil
}
