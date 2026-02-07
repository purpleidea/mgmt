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
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/safepath"
)

const (
	httpServerFileKind = httpServerKind + ":file"
)

func init() {
	engine.RegisterResource(httpServerFileKind, func() engine.Res { return &HTTPServerFileRes{} })
}

var _ HTTPServerGroupableRes = &HTTPServerFileRes{} // compile time check

// HTTPServerFileRes is a file that exists within an http server. The name is
// used as the public path of the file, unless the filename field is specified,
// and in that case it is used instead. The way this works is that it autogroups
// at runtime with an existing http resource, and in doing so makes the file
// associated with this resource available for serving from that http server.
type HTTPServerFileRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes

	init *engine.Init

	// Server is the name of the http server resource to group this into. If
	// it is omitted, and there is only a single http resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Filename is the name of the file this data should appear as on the
	// http server.
	Filename string `lang:"filename" yaml:"filename"`

	// Path is the absolute path to a file that should be used as the source
	// for this file resource. It must not be combined with the data field.
	// If this corresponds to a directory, then it will used as a root dir
	// that will be served as long as the resource name or Filename are also
	// a directory ending with a slash.
	Path string `lang:"path" yaml:"path"`

	// Data is the file content that should be used as the source for this
	// file resource. It must not be combined with the path field.
	// TODO: should this be []byte instead?
	Data string `lang:"data" yaml:"data"`
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerFileRes) Default() engine.Res {
	return &HTTPServerFileRes{}
}

// getPath returns the actual path we respond to. When Filename is not
// specified, we use the Name. Note that this is the filename that will be seen
// on the http server, it is *not* the source path to the actual file contents
// being sent by the server.
func (obj *HTTPServerFileRes) getPath() string {
	if obj.Filename != "" {
		return obj.Filename
	}
	return obj.Name()
}

// getContent returns the content that we expect from this resource. It depends
// on whether the user specified the Path or Data fields, and whether the Path
// exists or not.
func (obj *HTTPServerFileRes) getContent(requestPath safepath.AbsPath) (io.ReadSeeker, error) {
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
func (obj *HTTPServerFileRes) getContentRelative(requestPath safepath.AbsPath) (safepath.AbsFile, error) {
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
func (obj *HTTPServerFileRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPServerFileRes) AcceptHTTP(req *http.Request) error {
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
func (obj *HTTPServerFileRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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
func (obj *HTTPServerFileRes) Validate() error {
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
func (obj *HTTPServerFileRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPServerFileRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPServerFileRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPServerFileRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerFileRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerFileRes to others of the same resource kind
	res, ok := r.(*HTTPServerFileRes)
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

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *HTTPServerFileRes) UIDs() []engine.ResUID {
	return []engine.ResUID{}
}

// AutoEdges returns the autoedge data for this resource. If the http:file has a
// Path set, it creates a reversed edge to the file resource at that path, so
// the file is ensured to exist before the http server tries to serve it.
func (obj *HTTPServerFileRes) AutoEdges() (engine.AutoEdge, error) {
	if obj.Path == "" {
		return nil, nil // data is inline, no file dependency
	}

	var reversed = true
	return &HTTPServerFileResAutoEdges{
		data: []engine.ResUID{
			&FileUID{
				BaseUID: engine.BaseUID{
					Name:     obj.Name(),
					Kind:     obj.Kind(),
					Reversed: &reversed,
				},
				path: obj.Path,
			},
		},
	}, nil
}

// HTTPServerFileResAutoEdges holds the state of the auto edge generator.
type HTTPServerFileResAutoEdges struct {
	data []engine.ResUID
	done bool
}

// Next returns the next automatic edge.
func (obj *HTTPServerFileResAutoEdges) Next() []engine.ResUID {
	if obj.done {
		return nil
	}
	return obj.data
}

// Test gets results of the earlier Next() call, and returns if we should
// continue.
func (obj *HTTPServerFileResAutoEdges) Test(input []bool) bool {
	obj.done = true
	return false
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerFileRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerFileRes // indirection to avoid infinite recursion

	def := obj.Default()                // get the default
	res, ok := def.(*HTTPServerFileRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerFileRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerFileRes(raw) // restore from indirection with type conversion!
	return nil
}
