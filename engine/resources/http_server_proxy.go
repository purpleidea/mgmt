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
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

const (
	httpServerProxyKind = httpServerKind + ":proxy"

	// httpServerProxyTempPrefix is the start of the name of every partial
	// download in the cache. Requests for any path which has a component
	// that starts with this are refused, so that a partial download can't
	// ever be served.
	httpServerProxyTempPrefix = ".mgmt-partial-"
)

func init() {
	engine.RegisterResource(httpServerProxyKind, func() engine.Res { return &HTTPServerProxyRes{} })
}

var _ HTTPServerGroupableRes = &HTTPServerProxyRes{} // compile time check

// HTTPServerProxyRes is a resource representing a special path that exists
// within an http server. The name is used as the public path of the endpoint,
// unless the path field is specified, and in that case it is used instead. The
// way this works is that it autogroups at runtime with an existing http server
// resource, and in doing so makes the path associated with this resource
// available when serving files. When something under the path is accessed, this
// is pulled from the backing http server, which makes an http client connection
// if needed to pull the authoritative file down, saves it locally for future
// use, and then returns it to the original http client caller. On a subsequent
// call, if the cache was not invalidated, the file doesn't need to be fetched
// from the network. In effect, this works as a caching http proxy. If you
// create this as a resource which responds to the same type of request as an
// http:server:file resource or any other kind of resource, it is undefined
// behaviour which will answer the request. The most common clash will happen if
// both are present at the same path. The resultant proxy path is determined by
// subtracting the `Sub` field from the `Path` (and request path) and then
// appending the result to the `Head` field.
//
// The cache can be used by any number of concurrent clients, and by more than
// one of these resources, even across graph swaps. A file in the cache is
// either absent or complete, since a download is written to a private partial
// file, which is only renamed into place once the whole body has arrived, just
// after the client that it was downloaded for has received all of it. As a
// result, no client ever waits for another client or for another download, so a
// slow or stalled client can only slow itself down. Both the cache hits and the
// downloads are streamed, so the memory used doesn't grow with file size. If
// the cache can't be written to, then we still serve the client. A request for
// a file which is not in the cache yet downloads it from upstream, even if
// another request for that same file is already doing so, and whichever
// finishes last is the one in the cache, but they're all identical anyways.
type HTTPServerProxyRes struct {
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
	// TODO: Concurrent requests for the same uncached file should share one
	// upstream download, which they would each stream from at their own
	// pace, instead of each one downloading it separately.
	// TODO: Clean up partial files left behind in the cache after a crash.

	// pathParser is the local instantiation of our parser that gets reused.
	pathParser *pathParser
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerProxyRes) Default() engine.Res {
	return &HTTPServerProxyRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name.
func (obj *HTTPServerProxyRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// serveHTTP is the real implementation of ServeHTTP. It only returns an error
// if it hasn't written anything to the client yet, so that we can send one.
func (obj *HTTPServerProxyRes) serveHTTP(w http.ResponseWriter, req *http.Request) error {
	// TODO: switch requestPath to use safepath.AbsPath instead of a string
	requestPath := req.URL.Path // TODO: is this what we want here?

	result, err := obj.pathParser.parse(requestPath)
	if err != nil {
		return err
	}

	if obj.Cache != "" { // check in the cache...
		// TODO: do cache invalidation here
		served, err := obj.serveCached(w, req, result.cachePath)
		if err != nil {
			return err // cache dir is broken
		}
		if served {
			return nil
		}
		// otherwise, it's not in the cache...
	}

	if obj.Head == "" { // we can't download (we can only pull from cache)
		return fmt.Errorf("can't proxy") // NOT a 404 error!
	}

	return obj.serveUpstream(w, req, result)
}

// serveCached serves the file from our local cache if it's in there. It returns
// false if it isn't, and it only errors if the cache itself is broken. Files
// are only renamed into the cache once they're complete, so we don't need to
// lock anything here. Once we've opened it, we keep reading that same file,
// even if a newer download replaces it while we're still serving it.
func (obj *HTTPServerProxyRes) serveCached(w http.ResponseWriter, req *http.Request, cachePath string) (bool, error) {
	f, err := os.Open(cachePath)
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil // not in the cache
	}
	if err != nil {
		return false, err
	}
	defer f.Close() // ignore error

	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if fi.IsDir() { // we only cache files
		return false, nil
	}

	obj.init.Logf("cache: %q", cachePath)
	requestPath := req.URL.Path // TODO: is this what we want here?
	http.ServeContent(w, req, requestPath, fi.ModTime(), f)
	//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?
	return true, nil
}

// serveUpstream downloads the file from upstream, and streams it to the client.
// If we have a cache, then it also streams it into a partial file there, which
// is put into the cache once all of it has arrived. Problems with the cache are
// logged, but they don't affect the client.
func (obj *HTTPServerProxyRes) serveUpstream(w http.ResponseWriter, req *http.Request, result *pathResult) error {
	method := req.Method
	proxyURL := result.proxyURL
	cachePath := result.cachePath

	// FIXME: should we be using a different client?
	client := http.DefaultClient
	// This keeps going while the http:server drains, and it's cancelled at
	// the cut if the drain takes too long or on an interrupt.
	//nolint:gosec // G704: proxyURL is rooted at the operator-configured Head; proxying to it is this resource's purpose
	request, err := http.NewRequestWithContext(req.Context(), method, proxyURL, nil) // (*Request, error)
	if err != nil {
		return err
	}

	// TODO: add a progress logf...
	obj.init.Logf("%s: %s", method, proxyURL)

	// Tell the client right away, that we're working on things...
	// TODO: Is this valuable to give us more time to block?
	// NOTE: Using this header breaks wget2!
	//w.WriteHeader(http.StatusProcessing) // http 102, RFC 2518, 10.1

	//nolint:gosec // G704: request targets the operator-configured Head; proxying to it is this resource's purpose
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

	// We stream the download into both the client requesting it indirectly
	// through this proxy, and also into the cache if we want to store the
	// file. This lets us not OOM on large files. This is necessary because
	// otherwise a client would get bored of waiting for us to download
	// large files before sending them off. One downside is we haven't
	// implemented range requests that http.ServeContent did for us
	// magically, and we also currently might not do all of the http/header
	// magic it did.

	// A HEAD request only retrieves metadata, so it must not create an
	// empty cache entry, and a request that looks like a dir can't be
	// stored as a file.
	var fill *httpServerProxyFill // nil when we're not caching
	if obj.Cache != "" && method != http.MethodHead && !strings.HasSuffix(cachePath, "/") {
		fill, err = newHTTPServerProxyFill(cachePath)
		if err != nil {
			obj.init.Logf("not caching: %q: %s", cachePath, err)
			fill = nil
		} else {
			defer fill.abort() // does nothing once it's committed
		}
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

	// Use the file's extension to find the ctype, skip if we would need to
	// seek into the file to read some header content bytes.
	// TODO: Is this sufficient for setting Content-Type ?
	if ctype := mime.TypeByExtension(filepath.Ext(requestPath)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}

	w.WriteHeader(http.StatusOK) // Tell client everything is ok.
	if method == http.MethodHead {
		// end early, no content
		return nil
	}

	var body io.Reader = response.Body
	if fill != nil {
		body = io.TeeReader(response.Body, fill) // this never errors
	}
	if _, err := io.Copy(w, body); err != nil {
		if fill != nil {
			obj.init.Logf("discarding a partial download: %q", cachePath)
		}
		// Even if we have an error, it's too late to error.
		//return err // superfluous response.WriteHeader call
		obj.init.Logf("copy error: %s", err)
		return nil
	}
	//obj.init.Logf("%d bytes sent", n) // XXX: how do we know (on the server-side) if it worked?

	if fill == nil {
		return nil
	}

	// The client has everything now, so send it all to them before we wait
	// on the disk. They can't tell if the cache works or not, so it's fine.
	// TODO: This only helps when upstream sent a Content-Length. Without
	// one, the response is chunked, and its last chunk is only sent once
	// we return, so the client waits for the fsync in the commit. Either
	// way, the next request on a keep-alive connection waits for it too. We
	// could instead commit in a goroutine which the http:server waits for
	// like it does for the handlers, eg: an httpServerGo(ctx, fn) that
	// finds the handler WaitGroup in the request ctx. Then we'd return at
	// once, and the cache entry would still exist before Watch returns.
	_ = http.NewResponseController(w).Flush() // ignore error
	if err := fill.commit(modtime); err != nil {
		obj.init.Logf("could not cache: %q: %s", cachePath, err)
	}
	return nil
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPServerProxyRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPServerProxyRes) AcceptHTTP(req *http.Request) error {
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
func (obj *HTTPServerProxyRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow GET and HEAD at the moment.
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// TODO: use safepath instead
	//absPath, err := safepath.ParseIntoAbsPath(requestPath)
	//if err != nil {
	//	obj.init.Logf("invalid input path: %q", requestPath)
	//	sendHTTPError(w, err)
	//	return
	//}

	if err := obj.serveHTTP(w, req); err != nil {
		obj.init.Logf("error: %s", err)
		sendHTTPError(w, err)
		return
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPServerProxyRes) Validate() error {
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
func (obj *HTTPServerProxyRes) Init(init *engine.Init) error {
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
func (obj *HTTPServerProxyRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPServerProxyRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	return ctx.Err()
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPServerProxyRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerProxyRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerProxyRes to others of the same resource kind
	res, ok := r.(*HTTPServerProxyRes)
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

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerProxyRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerProxyRes // indirection to avoid infinite recursion

	def := obj.Default()                 // get the default
	res, ok := def.(*HTTPServerProxyRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerProxyRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerProxyRes(raw) // restore from indirection with type conversion!
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
	if obj.cache != "" {
		// Never serve a partial download, even if it's asked for.
		for _, s := range strings.Split(rel, "/") {
			if strings.HasPrefix(s, httpServerProxyTempPrefix) {
				return nil, newHTTPError(http.StatusNotFound) // 404
			}
		}
	}
	result.cachePath = obj.cache + rel

	return result, nil
}

// httpServerProxyFill is a download into the cache, which is only visible at
// its path once it's complete. It's written to a private partial file next to
// that path, which commit renames into place. Since a rename is atomic, anyone
// reading the cache only ever sees whole files, without any locking. Writes to
// this never fail, so that a broken cache never breaks the stream to a client,
// but after the first error it stops writing, and it can't be committed.
type httpServerProxyFill struct {
	path string   // the destination in the cache
	file *os.File // the partial file, in the same dir so rename is atomic
	err  error    // the first write error
	done bool     // set once the partial file is committed or removed
}

// newHTTPServerProxyFill starts a new partial download for this cache path. It
// comes from the request, but it's always inside the cache dir, since parse
// only takes a clean request path, and AcceptHTTP keeps that under our path.
func newHTTPServerProxyFill(path string) (*httpServerProxyFill, error) {
	dir := filepath.Dir(path)
	//nolint:gosec // G703: path is inside the cache dir, see above
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, httpServerProxyTempPrefix+"*")
	if err != nil {
		return nil, err
	}
	return &httpServerProxyFill{
		path: path,
		file: file,
	}, nil
}

// Write writes into the partial file. It never errors, see the struct docs.
func (obj *httpServerProxyFill) Write(p []byte) (int, error) {
	if obj.err == nil {
		_, obj.err = obj.file.Write(p)
	}
	return len(p), nil
}

// commit puts the download into the cache, and replaces what's there if needed.
// Only run it once the whole body has arrived. The data is synced first, so
// that after a crash, the file in the cache is either absent or complete.
func (obj *httpServerProxyFill) commit(modtime time.Time) error {
	if obj.err != nil {
		return obj.err
	}
	if err := obj.file.Sync(); err != nil {
		return err
	}
	if err := obj.file.Close(); err != nil {
		return err
	}
	name := obj.file.Name()
	if !isZeroTime(modtime) {
		// TODO: is there a file last-modified-time precision issue if
		// we use this value in a future HTTP If-Modified-Since header?
		if err := os.Chtimes(name, time.Time{}, modtime); err != nil {
			return err
		}
	}
	//nolint:gosec // G703: path is inside the cache dir, see newHTTPServerProxyFill
	if err := os.Rename(name, obj.path); err != nil {
		return err
	}
	obj.done = true
	return nil
}

// abort removes the partial file, unless it was committed already.
func (obj *httpServerProxyFill) abort() {
	if obj.done {
		return
	}
	obj.done = true
	_ = obj.file.Close() // it might be closed already
	//nolint:gosec // G703: CreateTemp made this inside the cache dir
	_ = os.Remove(obj.file.Name())
}

// isZeroTime reports whether t is obviously unspecified (either zero or
// Unix()=0). Adapted from: src/net/http/fs.go
func isZeroTime(t time.Time) bool {
	var unixEpochTime = time.Unix(0, 0)

	return t.IsZero() || t.Equal(unixEpochTime)
}
