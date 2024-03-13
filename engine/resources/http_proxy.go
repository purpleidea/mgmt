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
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

const (
	httpProxyKind = httpKind + ":proxy"
)

var (
	// httpProxyRWMutex synchronizes against reads and writes to the cache.
	// TODO: we could instead have a per-cache path individual mutex, but to
	// keep things simple for now, we just lumped them all together.
	httpProxyRWMutex *sync.RWMutex
)

func init() {
	httpProxyRWMutex = &sync.RWMutex{}

	engine.RegisterResource(httpProxyKind, func() engine.Res { return &HTTPProxyRes{} })
}

// HTTPProxyRes is a resource representing a special path that exists within an
// http server. The name is used as the public path of the endpoint, unless the
// path field is specified, and in that case it is used instead. The way this
// works is that it autogroups at runtime with an existing http resource, and in
// doing so makes the path associated with this resource available when serving
// files. When something under the path is accessed, this is pulled from the
// backing http server, which makes an http client connection if needed to pull
// the authoritative file down, saves it locally for future use, and then
// returns it to the original http client caller. On a subsequent call, if the
// cache was not invalidated, the file doesn't need to be fetched from the
// network. In effect, this works as a caching http proxy. If you create this as
// a resource which responds to the same type of request as an http:file
// resource or any other kind of resource, it is undefined behaviour which will
// answer the request. The most common clash will happen if both are present at
// the same path. This particular implementation stores some file data in memory
// as a convenience instead of streaming directly to clients. This makes locking
// much easier, but is wasteful. If you plan on using this for huge files and on
// systems with low amounts of memory, you might want to optimize this. The
// resultant proxy path is determined by subtracting the `Sub` field from the
// `Path` (and request path) and then appending the result to the `Head` field.
type HTTPProxyRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes
	traits.Sendable

	init *engine.Init

	// Server is the name of the http server resource to group this into. If
	// it is omitted, and there is only a single http resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Path is the path that this presents as on the grouped http server. It
	// overrides the Name var if specified.
	Path string `lang:"path" yaml:"path"`

	// Sub is the string to remove from the start of the request, the path
	// of which is looking at the Name/Path field to see if it matches. If
	// it matches, it then translates to the destination server by removing
	// this `Sub` string from the start of the path request, and then
	// prepending on the `Head` field to get to the resultant proxy URL. If
	// this is empty, then nothing is subtracted.
	Sub string `lang:"sub" yaml:"sub"`

	// Head is the string to add on as a prefix to the new URL we are
	// building for the proxy. If this is empty, the proxy can't work, and
	// we can only rely on what is available in our local cache. This is
	// typically the protocol and hostname for the backing server.
	Head string `lang:"head" yaml:"head"`

	// Cache is an absolute path to a location on disk where cached files
	// can be stored. If this is empty then we will not cache any files.
	// TODO: We could add future in-memory stores, a checksum feature, etc
	Cache string `lang:"cache" yaml:"cache"`

	// TODO: Add tests for the Path+Sub+Head+Cache path math stuff
	// TODO: Allow this to support single file proxying and caching
	// TODO: Add a TTL param to expire cached downloads
	// TODO: Add a max depth param to prevent people creating 1000 deep dirs
	// TODO: Allow single-file caching that can then also send/recv it out
	// TODO: Add a Force param to let a dir in cache folder get converted to a file or vice-versa
	// TODO: Add an alternate API that consumes a "mapping" function instead
	// of Sub/Head. Eg: mapping => func($s) { {"/fedora/" => "https://example.com/foo/" }[$s] }

	// pathParser is the local instantiation of our parser that gets reused.
	pathParser *pathParser
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPProxyRes) Default() engine.Res {
	return &HTTPProxyRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name.
func (obj *HTTPProxyRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// serveHTTP is the real implementation of ServeHTTP, but with a more ergonomic
// signature.
func (obj *HTTPProxyRes) serveHTTP(ctx context.Context, requestPath string) (handlerFuncError, error) {
	// TODO: switch requestPath to use safepath.AbsPath instead of a string

	result, err := obj.pathParser.parse(requestPath)
	if err != nil {
		return nil, err
	}

	cachePath := result.cachePath

	if obj.Cache != "" { // check in the cache...
		// TODO: do cache invalidation here
		fn, err := obj.getCachedFile(ctx, cachePath)
		if err != nil && !os.IsNotExist(err) {
			// cache dir is broken, error?
			return nil, err
		}
		if err == nil {
			obj.init.Logf("cache: %s", cachePath)
			return fn, nil
		}
		// otherwise, it must be a file not found in cache error...
	}

	if obj.Head == "" { // we can't download (we can only pull from cache)
		return nil, fmt.Errorf("can't proxy") // NOT a 404 error!
	}

	proxyURL := result.proxyURL

	// FIXME: should we be using a different client?
	client := http.DefaultClient
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyURL, nil) // (*Request, error)
	if err != nil {
		return nil, err
	}

	// TODO: add a progress logf...
	obj.init.Logf("get: %s", proxyURL)

	return func(w http.ResponseWriter, req *http.Request) error {

		// Tell the client right away, that we're working on things...
		// TODO: Is this valuable to give us more time to block?
		w.WriteHeader(http.StatusProcessing) // http 102, RFC 2518, 10.1

		response, err := client.Do(request) // (*Response, error)
		if err != nil {
			return err
		}
		defer response.Body.Close() // free

		if response.StatusCode != http.StatusOK { // 200
			return &httpError{
				//http.StatusText(http.StatusCode)
				msg:  fmt.Sprintf("bad status: %d", response.StatusCode),
				code: http.StatusNotFound, // 404
			}
		}

		// Determine the last-modified time if we can.
		modtime := time.Now()

		key := http.CanonicalHeaderKey("Last-Modified")
		if headers, exists := response.Header[key]; exists && len(headers) > 0 { // []string
			text := headers[len(headers)-1]           // last element
			lastModified, err := http.ParseTime(text) // (time.Time, error)
			if err == nil {
				modtime = lastModified
			}
		}

		// We stream the download into both the client requesting it
		// indirectly through this proxy, and also into the cache if we
		// want to store the file. This lets us not OOM on large files.
		// This is necessary because otherwise a client would get bored
		// of waiting for us to download large files before sending them
		// off. One downside is we haven't implemented range requests
		// that http.ServeContent did for us magically, and we also
		// currently might not do all of the http/header magic it did.

		var copyError error
		writers := []io.Writer{w} // out to the client

		if obj.Cache != "" { // check in the cache...
			httpProxyRWMutex.Lock()
			defer httpProxyRWMutex.Unlock()

			// store in cachePath
			if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
				return err
			}

			// store the last modified time if set
			// TODO: is there a file last-modified-time precision
			// issue if we use this value in a future HTTP
			// If-Modified-Since header?
			defer func() { // Do this after the file closes
				if copyError != nil {
					return // skip if we have a copy error
				}
				if err := os.Chtimes(cachePath, time.Time{}, modtime); err != nil {
					// TODO: what do we do here?
					obj.init.Logf("could not chtimes: %s", cachePath)
				}
			}()

			// TODO: use ctx
			out, err := os.Create(cachePath)
			if err != nil {
				return err
			}
			defer out.Close()
			writers = append(writers, out)

			// TODO: consider doing something like this to stream?
			//reader = io.TeeReader(reader, writer)
		}

		requestPath := req.URL.Path // TODO: is this what we want here?

		if !isZeroTime(modtime) { // from: src/net/http/fs.go:setLastModified
			w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
		}

		if obj.init.Debug || true {
			obj.init.Logf("got content-length: %d bytes", response.ContentLength)
		}

		if size := response.ContentLength; size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}

		// Use the file's extension to find the ctype, skip if we would
		// need to seek into the file to read some header content bytes.
		// TODO: Is this sufficient for setting Content-Type ?
		if ctype := mime.TypeByExtension(filepath.Ext(requestPath)); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}

		w.WriteHeader(http.StatusOK) // Tell client everything is ok.

		//if progressBar {
		//	writers = append(writers, progress)
		//}
		writer := io.MultiWriter(writers...)
		if len(writers) == 1 {
			writer = writers[0] // simplify
		}
		if _, err := io.Copy(writer, response.Body); err != nil {
			if obj.Cache != "" { // check in the cache...
				// We already took the mutex earlier!
				obj.init.Logf("removing a partial file: %s", cachePath)
				if err := os.Remove(cachePath); err != nil {
					// TODO: what do we do here?
					obj.init.Logf("could not remove: %s", cachePath)
				}
			}
			// Even if we have an error, it's too late to error.
			//return err // superfluous response.WriteHeader call
			copyError = err // store for defer
			return nil
		}

		//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?
		return nil
	}, nil
}

// getCachedFile pulls a file from our local cache if it exists. It returns the
// correct http handler on success, which we can then run.
func (obj *HTTPProxyRes) getCachedFile(ctx context.Context, absPath string) (handlerFuncError, error) {
	// TODO: if infinite reads keep coming in, do we indefinitely-postpone
	// the locking so that a new file can be saved in the cache?
	httpProxyRWMutex.RLock()
	defer httpProxyRWMutex.RUnlock()

	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close() // ignore error

	// Determine the last-modified time if we can.
	modtime := time.Now()
	fi, err := f.Stat()
	if err == nil {
		modtime = fi.ModTime()
	}
	// TODO: if Stat errors, should we fail the whole thing?

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	handle := bytes.NewReader(data) // buffer for mutex

	return func(w http.ResponseWriter, req *http.Request) error {
		requestPath := req.URL.Path // TODO: is this what we want here?
		http.ServeContent(w, req, requestPath, modtime, handle)
		//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?
		return nil
	}, nil
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPProxyRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPProxyRes) AcceptHTTP(req *http.Request) error {
	requestPath := req.URL.Path // TODO: is this what we want here?

	if p := obj.getPath(); strings.HasSuffix(p, "/") { // a dir!
		if strings.HasPrefix(requestPath, p) {
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
func (obj *HTTPProxyRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow GET at the moment.
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// XXX: does this cancel when our resource does?
	ctx := req.Context()        // context.Context
	requestPath := req.URL.Path // TODO: is this what we want here?

	// TODO: use safepath instead
	//absPath, err := safepath.ParseIntoAbsPath(requestPath)
	//if err != nil {
	//	obj.init.Logf("invalid input path: %s", requestPath)
	//	sendHTTPError(w, err)
	//	return
	//}
	//fn, err := obj.serveHTTP(ctx, absPath)

	fn, err := obj.serveHTTP(ctx, requestPath)
	if err != nil {
		obj.init.Logf("error: %s", err)
		sendHTTPError(w, err)
		return
	}
	if err := fn(w, req); err != nil {
		obj.init.Logf("error: %s", err)
		sendHTTPError(w, err)
		return
	}
	//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPProxyRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("empty filename")
	}
	// FIXME: does getPath need to start with a slash?
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("the path must be absolute")
	}

	if obj.Sub != "" && (!strings.HasPrefix(obj.Sub, "/") || !strings.HasSuffix(obj.Sub, "/")) {
		return fmt.Errorf("the Sub field must be either empty or an absolute dir prefix")
	}

	if obj.Head != "" {
		if !strings.HasSuffix(obj.Head, "/") {
			return fmt.Errorf("the Head must end with a slash")
		}
		if _, err := url.Parse(obj.Head); err != nil {
			return err
		}
	}

	if obj.Cache != "" && (!strings.HasPrefix(obj.Cache, "/") || !strings.HasSuffix(obj.Cache, "/")) {
		return fmt.Errorf("the Cache field must be either empty or an absolute dir")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPProxyRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.pathParser = &pathParser{
		path:  obj.getPath(),
		sub:   obj.Sub,
		head:  obj.Head, // mirror
		cache: obj.Cache,
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPProxyRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPProxyRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPProxyRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPProxyRes) Cmp(r engine.Res) error {
	// we can only compare HTTPProxyRes to others of the same resource kind
	res, ok := r.(*HTTPProxyRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}

	if obj.Sub != res.Sub {
		return fmt.Errorf("the Sub differs")
	}
	if obj.Head != res.Head {
		return fmt.Errorf("the Head differs")
	}
	if obj.Cache != res.Cache {
		return fmt.Errorf("the Cache differs")
	}

	return nil
}

// HTTPProxySends is the struct of data which is sent after a successful Apply.
type HTTPProxySends struct {
	// Data is the received value being sent.
	// TODO: should this be []byte or *[]byte instead?
	Data *string `lang:"data"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *HTTPProxyRes) Sends() interface{} {
	return &HTTPProxySends{
		Data: nil,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPProxyRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPProxyRes // indirection to avoid infinite recursion

	def := obj.Default()           // get the default
	res, ok := def.(*HTTPProxyRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPProxyRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPProxyRes(raw) // restore from indirection with type conversion!
	return nil
}

// pathResult is the output resultant paths.
type pathResult struct {
	proxyURL  string
	cachePath string
}

// pathParser is the input data that is used by parse.
type pathParser struct {
	path  string // obj.getPath() (name field)
	sub   string // obj.Sub (???)
	head  string // obj.Head (mirror)
	cache string // obj.Cache (destination dir)
}

// parse handles the logic of transforming the input settings and data into the
// resultant paths. It is implemented as a standalone function to facilitate
// testing.
func (obj *pathParser) parse(requestPath string) (*pathResult, error) {
	if !strings.HasPrefix(requestPath, "/") {
		return nil, fmt.Errorf("request was not absolute") // unexpected!
	}

	tail := "" // !isDir
	if strings.HasSuffix(requestPath, "/") {
		tail = "/" // isDir
		// TODO: can we handle paths that look like dirs?
	}

	// check for ../ type stuff?
	if requestPath+tail != filepath.Clean(requestPath) {
		return nil, fmt.Errorf("request was not clean")
	}

	// request path must have our configured path as a base
	if !strings.HasPrefix(requestPath, obj.path) {
		return nil, newHTTPError(http.StatusNotFound) // 404
	}

	// Does our request path (which we now know has the same prefix as
	// obj.path) have sub as a prefix? If not, we can't sub it and we error.
	if obj.sub != "" && !strings.HasPrefix(requestPath, obj.sub) {
		return nil, newHTTPError(http.StatusNotFound) // 404
	}

	// start building new proxyURL and cachePath
	tailPath := strings.TrimPrefix(requestPath, obj.sub) // relFile or relDir (if we get a dir-like requestPath)

	proxyURL := obj.head + tailPath // good!
	result := &pathResult{
		proxyURL: proxyURL,
		//cachePath: cachePath,
	}
	if obj.head == "" { // we might _only_ have a cache and no proxyURL
		result = nil
	}

	if obj.cache != "" {
		if !strings.HasPrefix(obj.cache, "/") {
			return result, fmt.Errorf("cache was not absolute")
		}
		if !strings.HasSuffix(obj.cache, "/") {
			return result, fmt.Errorf("cache was not a dir")
		}
	}

	if result == nil { // we have a cachePath, but no proxyURL
		result = &pathResult{}
	}

	rel := strings.TrimPrefix(requestPath, obj.path) // we already checked
	result.cachePath = obj.cache + rel

	return result, nil
}

// handlerFuncError is http.HandlerFunc, but with a more ergonomic signature.
type handlerFuncError func(http.ResponseWriter, *http.Request) error

// isZeroTime reports whether t is obviously unspecified (either zero or
// Unix()=0). Adapted from: src/net/http/fs.go
func isZeroTime(t time.Time) bool {
	var unixEpochTime = time.Unix(0, 0)

	return t.IsZero() || t.Equal(unixEpochTime)
}
