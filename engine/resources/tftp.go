// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package resources

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	securefilepath "github.com/cyphar/filepath-securejoin"
	tftp "github.com/pin/tftp/v3"
)

func init() {
	engine.RegisterResource("tftp:server", func() engine.Res { return &TFTPServerRes{} })
	engine.RegisterResource("tftp:file", func() engine.Res { return &TFTPFileRes{} })
}

const (
	// TftpDefaultTimeout is the default timeout in seconds for server
	// connections.
	TftpDefaultTimeout = 5

	// TftpUseSecureJoin specifies that we should add in a "secure join" lib
	// so that we avoid the ../../etc/passwd and symlink problems.
	TftpUseSecureJoin = true
)

// TFTPServerRes is a tftp server resource. It serves files, but does not
// actually apply any state. The name is used as the address to listen on,
// unless the Address field is specified, and in that case it is used instead.
// This resource can offer up files for serving that are specified either inline
// in this resource by specifying a tftp root, or as tftp:file resources which
// will get autogrouped into this resource at runtime. The two methods can be
// combined as well. The resource does *not* start serving until CheckApply for
// the resource runs.
type TFTPServerRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can have TFTPFileRes grouped into it

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

	once  *sync.Once
	start chan struct{} // closes by once

	wg *sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *TFTPServerRes) Default() engine.Res {
	return &TFTPServerRes{
		Timeout: TftpDefaultTimeout,
	}
}

// getAddress returns the actual address to use. When Address is not specified,
// we use the Name.
func (obj *TFTPServerRes) getAddress() string {
	if obj.Address != "" {
		return obj.Address
	}
	return obj.Name()
}

// Validate checks if the resource data structure was populated correctly.
func (obj *TFTPServerRes) Validate() error {
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

	return nil
}

// Init runs some startup code for this resource.
func (obj *TFTPServerRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// TODO: should we do this in the engine? Do we want to decide it here?
	for _, res := range obj.GetGroup() { // grouped elements
		if err := res.Init(init); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}
	}

	obj.once = &sync.Once{}
	obj.start = make(chan struct{})

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *TFTPServerRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TFTPServerRes) Watch(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)

	addr, err := net.ResolveUDPAddr("udp", obj.getAddress())
	if err != nil {
		return errwrap.Wrapf(err, "could not resolve address")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return errwrap.Wrapf(err, "could not start listener")
	}
	defer conn.Close()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-obj.start: // opened by CheckApply after runtime checks succeed
	case <-ctx.Done(): // closed by the engine to signal shutdown
		return context.Cause(ctx)
	}
	// XXX: Should we even do this drain? Do we move start above ListenUDP?
	if err := tftpDrainUDPConn(conn); err != nil {
		return errwrap.Wrapf(err, "could not drain queued packets")
	}

	hook := obj.hook()
	if hook == nil {
		return fmt.Errorf("the hook is nil") // programming error
	}

	// Use nil in place of handler to disable read or write operations.
	server := tftp.NewServer(obj.readHandler(ctx), obj.writeHandler())
	server.SetTimeout(time.Duration(obj.Timeout) * time.Second) // optional
	server.SetBackoff(func(int) time.Duration {
		// Match the library's default randomized retry delay while
		// running, but skip it once shutdown starts so active transfers
		// can drain.
		select {
		case <-ctx.Done():
			return 0
		default:
			return time.Duration(rand.Int63n(int64(time.Second)))
		}
	})
	server.SetHook(hook)

	obj.wg = &sync.WaitGroup{}
	defer obj.wg.Wait()

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()

		err := server.Serve(conn) // blocks until Shutdown() is called!
		if err == nil {
			cancel(nil)
			return
		}
		cancel(errwrap.Wrapf(err, "the server errored"))
	}()
	defer server.Shutdown()

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
		return context.Cause(ctx)
	}
}

// CheckApply never has anything to apply for this resource. It does however
// check that runtime requirements are fulfilled before Watch starts serving.
func (obj *TFTPServerRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
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

	checkOK := true
	for _, res := range obj.GetGroup() { // grouped elements
		if c, err := res.CheckApply(ctx, apply); err != nil {
			return false, errwrap.Wrapf(err, "autogrouped CheckApply failed")
		} else if !c {
			checkOK = false
		}
	}

	if checkOK {
		obj.once.Do(func() { close(obj.start) })
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TFTPServerRes) Cmp(r engine.Res) error {
	// we can only compare TFTPServerRes to others of the same resource kind
	res, ok := r.(*TFTPServerRes)
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

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *TFTPServerRes) Copy() engine.CopyableRes {
	return &TFTPServerRes{
		Address: obj.Address,
		Timeout: obj.Timeout,
		Root:    obj.Root,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *TFTPServerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TFTPServerRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*TFTPServerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TFTPServerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TFTPServerRes(raw) // restore from indirection with type conversion!
	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *TFTPServerRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*TFTPFileRes) // different from what we usually do!
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
func (obj *TFTPServerRes) readHandler(ctx context.Context) func(string, io.ReaderFrom) error {
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
			res, ok := x.(*TFTPFileRes) // convert from Res
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
				obj.init.Logf("error: %v", err)
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
			if obj.init.Debug || true { // XXX: maybe we should always do this?
				obj.init.Logf("File not found: %s", filename)
			}
			// don't leak additional information to client!
			return errwrap.Wrapf(os.ErrNotExist, "file: %s", filename)
		}

		// Set transfer size before calling ReadFrom if the thing we're
		// passing to ReadFrom doesn't support the io.Seeker interface.
		// NOTE: os.File does for example.
		//rf.(tftp.OutgoingTransfer).SetSize(myFileSize)

		// XXX: This is a giant (clever) hack to disconnect readers who
		// are misbehaving or otherwise. There may be a better way to
		// prevent needing this hack, but until it is found, at least do
		// something. See more at: https://github.com/pin/tftp/issues/41
		transfer, err := tftpTransferConn(rf)
		if err != nil && obj.init.Debug {
			obj.init.Logf("could not get transfer connection: %+v", err)
		}
		done := make(chan struct{})
		if transfer != nil {
			defer close(done)
			obj.wg.Add(1)
			go func() {
				defer obj.wg.Done()

				select {
				case <-ctx.Done():
					transfer.Close() // this unblocks ReadFrom
				case <-done:
				}
			}()
		}
		if closer, ok := handle.(io.Closer); ok {
			defer closer.Close()
		}

		n, err := rf.ReadFrom(handle)
		if err != nil {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			default:
			}
			obj.init.Logf("could not read %s, error: %+v", filename, err)
			// don't leak additional information to client!
			return fmt.Errorf("could not read: %s", filename)

		}
		obj.init.Logf("%d bytes sent", n)
		return nil
	}
}

// writeHandler handles all the incoming upload requests from clients.
func (obj *TFTPServerRes) writeHandler() func(string, io.WriterTo) error {
	// Use nil in place of handler function to disable that operation.
	return nil // not implemented
}

// hook is a helper function to build the tftp.Hook that we'd like to use. It
// must not be called before Init.
func (obj *TFTPServerRes) hook() tftp.Hook {
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

// TFTPFileRes is a file that exists within a tftp server. The name is used as
// the public path of the file, unless the filename field is specified, and in
// that case it is used instead. The way this works is that it autogroups at
// runtime with an existing tftp resource, and in doing so makes the file
// associated with this resource available for serving from that tftp server.
type TFTPFileRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into TFTPServerRes

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
func (obj *TFTPFileRes) Default() engine.Res {
	return &TFTPFileRes{}
}

// getFilename returns the actual filename to use. When Filename is not
// specified, we use the Name. Note that this is the filename that will be seen
// on the tftp server, it is *not* the source path to the actual file contents
// being sent by the server.
func (obj *TFTPFileRes) getFilename() string {
	if obj.Filename != "" {
		return obj.Filename
	}
	return obj.Name()
}

// getContent returns the content that we expect from this resource. It depends
// on whether the user specified the Path or Data fields, and whether the Path
// exists or not.
func (obj *TFTPFileRes) getContent() (io.ReadSeeker, error) {
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
func (obj *TFTPFileRes) Validate() error {
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
func (obj *TFTPFileRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *TFTPFileRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *TFTPFileRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	return nil
}

// CheckApply never has anything to apply for this resource. It does however
// check that the source Path can be served if one was specified.
func (obj *TFTPFileRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	if obj.Path != "" {
		fileInfo, err := os.Stat(obj.Path)
		if err != nil {
			return false, errwrap.Wrapf(err, "can't stat Path")
		}
		if fileInfo.IsDir() {
			return false, fmt.Errorf("the Path is a dir")
		}

		handle, err := os.Open(obj.Path)
		if err != nil {
			return false, errwrap.Wrapf(err, "can't open Path")
		}
		defer handle.Close()
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TFTPFileRes) Cmp(r engine.Res) error {
	// we can only compare TFTPFileRes to others of the same resource kind
	res, ok := r.(*TFTPFileRes)
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
func (obj *TFTPFileRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TFTPFileRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*TFTPFileRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TFTPFileRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TFTPFileRes(raw) // restore from indirection with type conversion!
	return nil
}

// tftpTransferConn returns the per-transfer UDP connection owned by the tftp
// library. The library does not expose a way to interrupt one blocked transfer,
// so we capture the connection before ReadFrom starts and close it if the
// resource context is cancelled.
func tftpTransferConn(rf io.ReaderFrom) (*net.UDPConn, error) {
	value := reflect.ValueOf(rf)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return nil, fmt.Errorf("unexpected transfer type: %T", rf)
	}

	value = value.Elem()
	connField := value.FieldByName("conn")
	if !connField.IsValid() {
		return nil, fmt.Errorf("missing transfer connection")
	}
	if connField.IsNil() {
		return nil, fmt.Errorf("nil transfer connection")
	}

	connValue := reflect.NewAt(connField.Type(), unsafe.Pointer(connField.UnsafeAddr())).Elem()
	conn := reflect.ValueOf(connValue.Interface())
	if conn.Kind() == reflect.Interface {
		conn = conn.Elem()
	}
	if conn.Kind() != reflect.Pointer || conn.IsNil() {
		return nil, fmt.Errorf("unexpected connection type: %T", connValue.Interface())
	}

	conn = conn.Elem()
	udpField := conn.FieldByName("conn")
	if !udpField.IsValid() {
		return nil, fmt.Errorf("missing UDP connection")
	}
	if udpField.IsNil() {
		return nil, fmt.Errorf("nil UDP connection")
	}

	udpValue := reflect.NewAt(udpField.Type(), unsafe.Pointer(udpField.UnsafeAddr())).Elem()
	udpConn, ok := udpValue.Interface().(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("unexpected UDP connection type: %T", udpValue.Interface())
	}
	return udpConn, nil
}

// tftpDrainUDPConn discards datagrams queued while Watch was waiting for the
// first successful CheckApply. Those requests arrived before the resource was
// allowed to serve, so they must not be handled after the gate opens.
func tftpDrainUDPConn(conn *net.UDPConn) error {
	if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})

	buf := make([]byte, 64*1024)
	for {
		if _, _, err := conn.ReadFromUDP(buf); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil
			}
			return err
		}
	}
}
