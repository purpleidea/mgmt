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
	_ "embed" // embed data with go:embed
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources/http_server_ui/common"
	"github.com/purpleidea/mgmt/engine/resources/http_server_ui/static"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/gin-gonic/gin"
)

const (
	httpServerUIKind = httpServerKind + ":ui"

	httpServerUIIndexHTMLTmpl = "index.html.tmpl"
)

var (
	//go:embed http_server_ui/index.html.tmpl
	httpServerUIIndexHTMLTmplData string

	//go:embed http_server_ui/wasm_exec.js
	httpServerUIWasmExecData []byte

	//go:embed http_server_ui/main.wasm
	httpServerUIMainWasmData []byte
)

func init() {
	engine.RegisterResource(httpServerUIKind, func() engine.Res { return &HTTPServerUIRes{} })

	// XXX: here for now: https://github.com/gin-gonic/gin/issues/1180
	gin.SetMode(gin.ReleaseMode) // for production
}

var _ HTTPServerGroupableRes = &HTTPServerUIRes{} // compile time check

// HTTPServerUIGroupableRes is the interface that you must implement if you want
// to allow a resource the ability to be grouped into the http server ui
// resource. As an added safety, the Kind must also begin with
// "http:server:ui:", and not have more than one colon to avoid accidents of
// unwanted grouping.
type HTTPServerUIGroupableRes interface {
	engine.Res

	// ParentName is used to limit which resources autogroup into this one.
	// If it's empty then it's ignored, otherwise it must match the Name of
	// the parent to get grouped.
	ParentName() string

	// GetKind returns the "kind" of resource that this UI element is. This
	// is technically different than the Kind() field, because it can be a
	// unique kind that's specific to the HTTP form UI resources.
	GetKind() string

	// GetID returns the unique ID that this UI element responds to. Note
	// that this is NOT replaceable by Name() because this ID is used in
	// places that might be public, such as in webui form source code.
	GetID() string

	// SetValue sends the new value that was obtained from submitting the
	// form. This is the raw, unsafe value that you must validate first.
	SetValue(context.Context, []string) error

	// GetValue gets a string representation for the form value, that we'll
	// use in our html form.
	GetValue(context.Context) (string, error)

	// GetType returns a map that you can use to build the input field in
	// the ui.
	GetType() map[string]string

	// GetSort returns a string that you can use to determine the global
	// sorted display order of all the elements in a ui.
	GetSort() string
}

// HTTPServerUIResData represents some additional data to attach to the
// resource.
type HTTPServerUIResData struct {
	// Title is the generated page title that is displayed to the user.
	Title string `lang:"title" yaml:"title"`

	// Head is a list of strings to insert into the <head> and </head> tags
	// of your page. This string allows HTML, so choose carefully!
	// XXX: a *string should allow a partial struct here without having this
	// field, but our type unification algorithm isn't this fancy yet...
	Head string `lang:"head" yaml:"head"`
}

// HTTPServerUIRes is a web UI resource that exists within an http server. The
// name is used as the public path of the ui, unless the path field is
// specified, and in that case it is used instead. The way this works is that it
// autogroups at runtime with an existing http server resource, and in doing so
// makes the form associated with this resource available for serving from that
// http server.
type HTTPServerUIRes struct {
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

	// Path is the name of the path that this should be exposed under. For
	// example, you might want to name this "/ui/" to expose it as "ui"
	// under the server root. This overrides the name variable that is set.
	Path string `lang:"path" yaml:"path"`

	// Data represents some additional data to attach to the resource.
	Data *HTTPServerUIResData `lang:"data" yaml:"data"`

	//eventStream chan error
	eventsChanMap map[engine.Res]chan error

	// notifications contains a channel for every long poller waiting for a
	// reply.
	notifications map[engine.Res]map[chan struct{}]struct{}

	// rwmutex guards the notifications map.
	rwmutex *sync.RWMutex

	ctx context.Context // set by Watch
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerUIRes) Default() engine.Res {
	return &HTTPServerUIRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name. Note that this is the handler path that will be seen on the
// root http server, and this ui application might use a querystring and/or POST
// data as well.
func (obj *HTTPServerUIRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// routerPath returns an appropriate path for our router based on what we want
// to achieve using our parent prefix.
func (obj *HTTPServerUIRes) routerPath(p string) string {
	if strings.HasPrefix(p, "/") {
		return obj.getPath() + p[1:]
	}

	return obj.getPath() + p
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPServerUIRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPServerUIRes) AcceptHTTP(req *http.Request) error {
	requestPath := req.URL.Path // TODO: is this what we want here?
	//if requestPath != obj.getPath() {
	//	return fmt.Errorf("unhandled path")
	//}
	if !strings.HasPrefix(requestPath, obj.getPath()) {
		return fmt.Errorf("unhandled path")
	}
	return nil
}

// getResByID returns the grouped resource with the id we're searching for if it
// exists, otherwise nil and false.
func (obj *HTTPServerUIRes) getResByID(id string) (HTTPServerUIGroupableRes, bool) {
	for _, x := range obj.GetGroup() { // grouped elements
		res, ok := x.(HTTPServerUIGroupableRes) // convert from Res
		if !ok {
			continue
		}
		if obj.init.Debug {
			obj.init.Logf("Got grouped resource: %s", res.String())
		}
		if id != res.GetID() {
			continue
		}
		return res, true
	}
	return nil, false
}

// ginLogger is a helper to get structured logs out of gin.
func (obj *HTTPServerUIRes) ginLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		//start := time.Now()
		c.Next()
		//duration := time.Since(start)

		//timestamp := time.Now().Format(time.RFC3339)
		method := c.Request.Method
		path := c.Request.URL.Path
		status := c.Writer.Status()
		//latency := duration
		clientIP := c.ClientIP()
		if obj.init.Debug {
			return
		}
		obj.init.Logf("%v %s %s (%d)", clientIP, method, path, status)
	}
}

// getTemplate builds the super template that contains the map of each file name
// so that it can be used easily to send out named, templated documents.
func (obj *HTTPServerUIRes) getTemplate() (*template.Template, error) {
	// XXX: get this from somewhere
	m := make(map[string]string)
	//m["foo.tmpl"] = "hello from file1" // TODO: add more content?
	m[httpServerUIIndexHTMLTmpl] = httpServerUIIndexHTMLTmplData // index.html.tmpl

	filenames := []string{}
	for filename := range m {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames) // deterministic order

	var t *template.Template

	// This logic from golang/src/html/template/template.go:parseFiles(...)
	for _, filename := range filenames {
		data := m[filename]
		var tmpl *template.Template
		if t == nil {
			t = template.New(filename)
		}
		if filename == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(filename)
		}
		if _, err := tmpl.Parse(data); err != nil {
			return nil, err
		}
	}
	t = t.Option("missingkey=error") // be thorough
	return t, nil
}

// ServeHTTP is the standard HTTP handler that will be used here.
func (obj *HTTPServerUIRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	// XXX: do all the router bits in Init() if we can...
	//gin.SetMode(gin.ReleaseMode) // for production
	router := gin.New()
	router.Use(obj.ginLogger(), gin.Recovery())

	templ, err := obj.getTemplate() // do in init?
	if err != nil {
		obj.init.Logf("template error: %+v", err)
		return
	}
	router.SetHTMLTemplate(templ)

	router.GET(obj.routerPath("/"), func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, obj.routerPath("/index.html"))
	})

	router.GET(obj.routerPath("/index.html"), func(c *gin.Context) {
		h := gin.H{}
		h["program"] = obj.init.Program
		h["version"] = obj.init.Version
		h["hostname"] = obj.init.Hostname
		h["embedded"] = static.HTTPServerUIStaticEmbedded // true or false
		h["title"] = ""                                   // key must be specified
		h["path"] = obj.getPath()
		if obj.Data != nil {
			h["title"] = obj.Data.Title // template var
			h["head"] = template.HTML(obj.Data.Head)
		}
		c.HTML(http.StatusOK, httpServerUIIndexHTMLTmpl, h)
	})
	router.GET(obj.routerPath("/main.wasm"), func(c *gin.Context) {
		c.Data(http.StatusOK, "application/wasm", httpServerUIMainWasmData)
	})
	router.GET(obj.routerPath("/wasm_exec.js"), func(c *gin.Context) {
		// the version of this file has to match compiler version
		// the original came from: ~golang/lib/wasm/wasm_exec.js
		// XXX: add a test to ensure this matches the compiler version
		// the content-type matters or this won't work in the browser
		c.Data(http.StatusOK, "text/javascript;charset=UTF-8", httpServerUIWasmExecData)
	})

	if static.HTTPServerUIStaticEmbedded {
		router.GET(obj.routerPath("/"+static.HTTPServerUIIndexBootstrapCSS), func(c *gin.Context) {
			c.Data(http.StatusOK, "text/css;charset=UTF-8", static.HTTPServerUIIndexStaticBootstrapCSS)
		})
		router.GET(obj.routerPath("/"+static.HTTPServerUIIndexBootstrapJS), func(c *gin.Context) {
			c.Data(http.StatusOK, "text/javascript;charset=UTF-8", static.HTTPServerUIIndexStaticBootstrapJS)
		})
	}

	router.POST(obj.routerPath("/save/"), func(c *gin.Context) {
		id, ok := c.GetPostForm("id")
		if !ok || id == "" {
			msg := "missing id"
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		values, ok := c.GetPostFormArray("value")
		if !ok {
			msg := "missing value"
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		res, ok := obj.getResByID(id)
		if !ok {
			msg := fmt.Sprintf("id `%s` not found", id)
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		// we're storing data...
		if err := res.SetValue(obj.ctx, values); err != nil {
			msg := fmt.Sprintf("bad data: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		// XXX: instead of an event to everything, instead if SetValue
		// is an active sub resource (instead of something that noop's)
		// that should send an event and eventually propagate to here,
		// so skip sending this global one...

		// Trigger a Watch() event so that CheckApply() calls Send/Recv,
		// so our newly received POST value gets sent through the graph.
		//select {
		//case obj.eventStream <- nil: // send an event
		//case <-obj.ctx.Done(): // in case Watch dies
		//	c.JSON(http.StatusInternalServerError, gin.H{
		//		"error": "Internal Server Error",
		//		"code":  500,
		//	})
		//}

		c.JSON(http.StatusOK, nil)
	})

	router.GET(obj.routerPath("/list/"), func(c *gin.Context) {
		elements := []*common.FormElement{}
		for _, x := range obj.GetGroup() { // grouped elements
			res, ok := x.(HTTPServerUIGroupableRes) // convert from Res
			if !ok {
				continue
			}

			element := &common.FormElement{
				Kind: res.GetKind(),
				ID:   res.GetID(),
				Type: res.GetType(),
				Sort: res.GetSort(),
			}

			elements = append(elements, element)
		}
		form := &common.Form{
			Elements: elements,
		}
		// XXX: c.JSON or c.PureJSON ?
		c.JSON(http.StatusOK, form) // send the struct as json
	})

	router.GET(obj.routerPath("/list/:id"), func(c *gin.Context) {
		id := c.Param("id")
		res, ok := obj.getResByID(id)
		if !ok {
			msg := fmt.Sprintf("id `%s` not found", id)
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		val, err := res.GetValue(obj.ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal Server Error",
				"code":  500,
			})
			return
		}

		el := &common.FormElementGeneric{ // XXX: text or string?
			Value: val,
		}

		c.JSON(http.StatusOK, el) // send the struct as json
	})

	router.GET(obj.routerPath("/watch/:id"), func(c *gin.Context) {
		id := c.Param("id")
		res, ok := obj.getResByID(id)
		if !ok {
			msg := fmt.Sprintf("id `%s` not found", id)
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		ch := make(chan struct{})
		//defer close(ch) // don't close, let it gc instead
		obj.rwmutex.Lock()
		obj.notifications[res][ch] = struct{}{} // add to notification "list"
		obj.rwmutex.Unlock()
		defer func() {
			obj.rwmutex.Lock()
			delete(obj.notifications[res], ch)
			obj.rwmutex.Unlock()
		}()
		select {
		case <-ch: // http long poll
			// pass
		//case <-obj.???[res].Done(): // in case Watch dies
		//	c.JSON(http.StatusInternalServerError, gin.H{
		//		"error": "Internal Server Error",
		//		"code":  500,
		//	})
		case <-obj.ctx.Done(): // in case Watch dies
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal Server Error",
				"code":  500,
			})
			return
		}

		val, err := res.GetValue(obj.ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal Server Error",
				"code":  500,
			})
			return
		}

		el := &common.FormElementGeneric{ // XXX: text or string?
			Value: val,
		}
		c.JSON(http.StatusOK, el) // send the struct as json
	})

	router.GET(obj.routerPath("/ping"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	router.ServeHTTP(w, req)
	return
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPServerUIRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("empty path")
	}
	// FIXME: does getPath need to start with a slash or end with one?

	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("the Path must be absolute")
	}

	if !strings.HasSuffix(obj.getPath(), "/") {
		return fmt.Errorf("the Path must end with a slash")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPServerUIRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	//obj.eventStream = make(chan error)
	obj.eventsChanMap = make(map[engine.Res]chan error)
	obj.notifications = make(map[engine.Res]map[chan struct{}]struct{})
	obj.rwmutex = &sync.RWMutex{}

	// NOTE: If we don't Init anything that's autogrouped, then it won't
	// even get an Init call on it.
	// TODO: should we do this in the engine? Do we want to decide it here?
	for _, res := range obj.GetGroup() { // grouped elements
		// NOTE: We build a new init, but it's not complete. We only add
		// what we're planning to use, and we ignore the rest for now...
		r := res // bind the variable!

		obj.eventsChanMap[r] = make(chan error)
		obj.notifications[r] = make(map[chan struct{}]struct{})
		event := func(ctx context.Context) error {
			select {
			case obj.eventsChanMap[r] <- nil:
				// send!
			case <-ctx.Done():
				return ctx.Err()
			}

			obj.rwmutex.RLock()
			for ch := range obj.notifications[r] {
				select {
				case ch <- struct{}{}:
					// send!
				default:
					// skip immediately if nobody is listening
				}
			}
			obj.rwmutex.RUnlock()

			// We don't do this here (why?) we instead read from the
			// above channel and then send on multiplexedChan to the
			// main loop, where it runs the obj.init.Event function.
			//if err := obj.init.Event(ctx); err != nil { return err } // notify engine of an event (this can block)

			return nil
		}

		newInit := &engine.Init{
			Program:  obj.init.Program,
			Version:  obj.init.Version,
			Hostname: obj.init.Hostname,

			// Watch:
			Running: event,
			Event:   event,

			// CheckApply:
			//Refresh: func() bool { // TODO: do we need this?
			//	innerRes, ok := r.(engine.RefreshableRes)
			//	if !ok {
			//		panic("res does not support the Refreshable trait")
			//	}
			//	return innerRes.Refresh()
			//},
			Send: engine.GenerateSendFunc(r),
			Recv: engine.GenerateRecvFunc(r), // unused

			FilteredGraph: func() (*pgraph.Graph, error) {
				panic("FilteredGraph for HTTP:Server:UI not implemented")
			},

			Local: obj.init.Local,
			World: obj.init.World,
			//VarDir: obj.init.VarDir, // TODO: wrap this

			Debug: obj.init.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.init.Logf(res.Kind()+": "+format, v...)
			},
		}

		if err := res.Init(newInit); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPServerUIRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPServerUIRes) Watch(ctx context.Context) error {

	multiplexedChan := make(chan error)
	defer close(multiplexedChan) // closes after everyone below us is finished

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	innerCtx, cancel := context.WithCancel(ctx) // store for ServeHTTP
	defer cancel()
	obj.ctx = innerCtx

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

	if err := obj.init.Running(ctx); err != nil { return err } // when started, notify engine that we're running

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	for {
		if obj.init.Debug {
			obj.init.Logf("Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil

		//case err, ok := <-obj.eventStream:
		//	if !ok { // shouldn't happen
		//		obj.eventStream = nil
		//		continue
		//	}
		//	if err != nil {
		//		return err
		//	}

		case err, ok := <-multiplexedChan:
			if !ok { // shouldn't happen
				multiplexedChan = nil
				continue
			}
			if err != nil {
				return err
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if err := obj.init.Event(ctx); err != nil { return err } // notify engine of an event (this can block)
	}

	//return nil // unreachable
}

// CheckApply is responsible for the Send/Recv aspects of the autogrouped
// resources. It recursively calls any autogrouped children.
func (obj *HTTPServerUIRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	checkOK := true
	for _, res := range obj.GetGroup() { // grouped elements
		if c, err := res.CheckApply(ctx, apply); err != nil {
			return false, errwrap.Wrapf(err, "autogrouped CheckApply failed")
		} else if !c {
			checkOK = false
		}
	}

	return checkOK, nil // w00t
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerUIRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerUIRes to others of the same resource kind
	res, ok := r.(*HTTPServerUIRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerUIRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerUIRes // indirection to avoid infinite recursion

	def := obj.Default()              // get the default
	res, ok := def.(*HTTPServerUIRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerUIRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerUIRes(raw) // restore from indirection with type conversion!
	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *HTTPServerUIRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(HTTPServerUIGroupableRes) // different from what we usually do!
	if !ok {
		return fmt.Errorf("resource is not the right kind")
	}

	// If the http resource has the parent name field specified, then it
	// must match against our name field if we want it to group with us.
	if pn := res.ParentName(); pn != "" && pn != obj.Name() {
		return fmt.Errorf("resource groups with a different parent name")
	}

	p := httpServerUIKind + ":"

	// http:server:ui:foo is okay, but http:server:file is not
	if !strings.HasPrefix(r.Kind(), p) {
		return fmt.Errorf("not one of our children")
	}

	// http:server:ui:foo is okay, but http:server:ui:foo:bar is not
	s := strings.TrimPrefix(r.Kind(), p)
	if len(s) != len(r.Kind()) && strings.Count(s, ":") > 0 { // has prefix
		return fmt.Errorf("maximum one resource after `%s` prefix", httpServerUIKind)
	}

	return nil
}
