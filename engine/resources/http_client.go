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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

const (
	httpClientKind = httpKind + ":client"

	httpClientLongpollGraceTime = 15 // seconds, arbitrary, pick a 2^n-1

	httpClientLongpollWaitTime = 60 // seconds, arbitrary

	// httpClientStatusNone is the sentinel meaning that this CheckApply has
	// no new outcome to report (eg: a 304 not-modified, or a local cache
	// hit). When this is the case we leave the previously published value
	// in the local bridge untouched, so that the status code we captured
	// alongside the data on the last real change persists.
	httpClientStatusNone = 0

	// httpClientStatusError is the status we publish to the local bridge
	// when an engine-level error happens (eg: a transport, disk, or
	// validation failure) instead of getting an HTTP status code.
	httpClientStatusError = -1
)

func init() {
	engine.RegisterResource(httpClientKind, func() engine.Res { return &HTTPClientRes{} })
}

// HTTPClientRes is an http client resource. The Name will be used as the
// destination file path if the File param is not specified and if Name is in
// the shape of a valid absolute path. This resource will not redownload a file
// if it already exists and has the same mtime or sha256sum as we expect it to.
// If this resource receives a "refresh" (edge) notification, then it will force
// a redownload. This resource sends the file contents as a send/recv edge. This
// resource can use an http long poll endpoint to receive events and know when a
// file has changed. If you attempt to use the Longpoll option with an endpoint
// which does not support that, then you may cause infinite looping. This
// resource publishes its response status and output file location to the local
// API bridge, so that a net/http.response("${name}") function can read and
// watch the status and output data which changes over time as this resource
// downloads files. One interesting detail: while you could just write the file
// to a known location and read it via os.readfile, that would be usually worse
// since that would see filesystem (inotify) style events when it's written,
// rather than the safer, internal event system which notifies only once, and
// when the actual (precise) change occurs.
// TODO: send/recv the http status too?
// TODO: add support for TLS
type HTTPClientRes struct {
	traits.Base     // add the base methods without re-implementation
	traits.Edgeable // XXX: add autoedge support
	traits.Refreshable
	traits.Sendable

	init *engine.Init

	// File is the output destination to write to if we download a file. It
	// must be an absolute file path. If you don't specify this, then the
	// Name will be used if it's a valid absolute file path. If no valid
	// file path exists anywhere, then we only download to a temporary
	// directory. We always download to a temporary directory anyways so
	// that we can atomically rename when we complete successfully. As a
	// result, if you don't particularly want to store a file somewhere,
	// then make sure the $name you specify does not resemble an absolute
	// file path which would begin with a slash and end without one.
	File string `lang:"file" yaml:"file"`

	// Method is the HTTP method (GET, POST, PUT, etc...) that is used. If
	// you omit this then "GET" is used.
	Method string `lang:"method" yaml:"method"`

	// URL is the endpoint to connect to. You may specify the protocol,
	// username, password, port, and all the other variables.
	URL string `lang:"url" yaml:"url"`

	// Body is the body to use when sending your request. It can be nil.
	Body *string `lang:"body" yaml:"body"`

	// MtimeCheck specifies that we can attempt to avoid redownloading based
	// on if the mtime that the server announces match what we already have
	// on disk.
	// TODO: what to name this?
	MtimeCheck bool `lang:"mtime_check" yaml:"mtime_check"`

	// Sha256 is specified if you expect the file contents to have this
	// hash.
	Sha256 string `lang:"sha256" yaml:"sha256"`

	// Longpoll specifies that the server supports http long polling. If the
	// endpoint actually does not, then this will cause infinite looping...
	// if specifying this option you must also specify one of the types of
	// long polling that you're using. This should match what the server
	// expects as this is an arbitrary contract on top of HTTP and not an
	// explicit part of the protocol.
	Longpoll bool `lang:"longpoll" yaml:"longpoll"`

	// LongpollRedirect specifies that the server will use an HTTP redirect
	// to notify you that the long poll is active and watching.
	LongpollRedirect bool `lang:"longpoll_redirect" yaml:"longpoll_redirect"`

	// LongpollConditional specifies that the server will need a second HTTP
	// request with a `Prefer: wait=<seconds>` header to set up the long
	// polling.
	// XXX: Can this method really guarantee the server started a watch?
	LongpollConditional bool `lang:"longpoll_conditional" yaml:"longpoll_conditional"`

	tmp string // download location
	dst string // the file location

	sha256 string // cache of dst

	// respCh passes the long poll response from Watch to CheckApply so that
	// we don't open a second connection just to download the body.
	respCh chan *http.Response

	// ackCh sends a message from CheckApply back to Watch to tell it that
	// we consumed the resp. This prevents Watch from running the next long
	// poll prematurely, which gives us natural backpressure.
	ackCh chan struct{}

	// status and output record the outcome of the download operations so
	// that they can be passed to the local API for use in the
	// net/http.response function.
	status int
	output string
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPClientRes) Default() engine.Res {
	return &HTTPClientRes{
		MtimeCheck: true,
	}
}

// getDst returns the path to the actual final file that we output to. If the
// user requested one via obj.File we use that, otherwise we try using the Name
// if it was a valid path, if not, we use an internal vardir handle. This method
// caches the resolved dst once it has been determined.
func (obj *HTTPClientRes) getDst() string {
	if obj.dst != "" { // already known!
		return obj.dst
	}

	if obj.File != "" {
		return obj.File
	}
	if strings.HasPrefix(obj.Name(), "/") && !strings.HasSuffix(obj.Name(), "/") {
		return obj.Name()
	}

	return "" // not known yet
}

// readAll reads the contents of the dst file that contains the data we want.
func (obj *HTTPClientRes) readAll() (string, error) {
	b, err := os.ReadFile(obj.dst)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// getMethod returns the actual method to use.
func (obj *HTTPClientRes) getMethod() string {
	if obj.Method == "" {
		return http.MethodGet // default
	}
	return obj.Method
}

// getRequest is a simple helper to build the request we'll be using.
func (obj *HTTPClientRes) getRequest(ctx context.Context, wait bool) (*http.Request, error) {

	var body io.Reader // body can be nil
	if s := obj.Body; s != nil {
		body = bytes.NewBufferString(*s)
	}

	// This only creates the object, it doesn't make any network connection.
	req, err := http.NewRequestWithContext(ctx, obj.getMethod(), obj.URL, body)
	if err != nil {
		return nil, err
	}

	if wait && obj.LongpollConditional {
		req.Header.Set("Prefer", fmt.Sprintf("wait=%d", httpClientLongpollWaitTime))
	}

	if !obj.MtimeCheck {
		return req, nil // done early
	}

	// Stat whichever on-disk copy we're looking for. If we have one at the
	// final destination, that's perfect, if not then we use the vardir one.
	fi, err := os.Stat(obj.dst)
	if err != nil && !os.IsNotExist(err) {
		return nil, err // real errors are always returned

	} else if err != nil { // file doesn't exist
		return req, nil // done early
	}

	req.Header.Set("If-Modified-Since", fi.ModTime().UTC().Format(http.TimeFormat))

	return req, nil
}

// retryAfter returns the delay requested by a long poll endpoint that is
// intentionally asking the client to reconnect instead of consuming a body.
func (obj *HTTPClientRes) retryAfter(resp *http.Response) (time.Duration, bool, error) {
	if resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false, nil
	}
	s := resp.Header.Get("Retry-After")
	if s == "" {
		return 0, false, nil
	}
	seconds, err := strconv.ParseUint(s, 10, 64)
	if err == nil {
		return time.Duration(seconds) * time.Second, true, nil
	}
	t, err := http.ParseTime(s)
	if err != nil {
		return 0, false, errwrap.Wrapf(err, "invalid Retry-After header")
	}
	if d := time.Until(t); d > 0 {
		return d, true, nil
	}
	return 0, true, nil
}

// hashFile returns the string sha256 sum of the file at the given path.
func (obj *HTTPClientRes) hashFile(f string) (string, error) {
	src, err := os.Open(f)
	if os.IsNotExist(err) {
		return "", nil // empty hash
	}
	if err != nil {
		return "", err
	}
	defer src.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}

	sum := hash.Sum(nil) // []byte
	return hex.EncodeToString(sum), nil
}

// validateMethod determines if the method is valid or not.
func (obj *HTTPClientRes) validateMethod(m string) error {
	// Unless otherwise noted, these are defined in RFC 7231 section 4.3.
	// https://rfc-editor.org/rfc/rfc7231.html#section-4.3

	// TODO: will all of these work?
	switch m {
	case http.MethodGet:
		return nil
	case http.MethodHead:
		return nil
	case http.MethodPost:
		return nil
	case http.MethodPut:
		return nil
	case http.MethodPatch:
		return nil
	case http.MethodDelete:
		return nil
	case http.MethodConnect:
		return nil
	case http.MethodOptions:
		return nil
	case http.MethodTrace:
		return nil
	}

	return fmt.Errorf("unknown method: %s", m)
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPClientRes) Validate() error {

	if f := obj.getDst(); f != "" {
		if !strings.HasPrefix(f, "/") {
			return fmt.Errorf("the File path must be absolute")
		}
		if strings.HasSuffix(f, "/") {
			return fmt.Errorf("the File path must not be a directory")
		}
	}

	m := obj.getMethod()
	if err := obj.validateMethod(m); err != nil {
		// it's invalid, but make the error easier:
		if s := strings.ToUpper(m); obj.validateMethod(s) == nil {
			return fmt.Errorf("method name %s must be in upper case, got: %s", s, m)
		}
		return err
	}

	u, err := url.ParseRequestURI(obj.URL)
	if err != nil {
		return errwrap.Wrapf(err, "invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("the URL scheme must be http or https, got: %s", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("the URL must have a host")
	}

	if obj.Sha256 != strings.ToLower(obj.Sha256) {
		return fmt.Errorf("the sha256 must be lower case")
	}

	if obj.MetaParams().Poll != 0 && obj.Longpoll {
		return fmt.Errorf("can't Longpoll when polling")
	}

	if obj.LongpollRedirect && obj.LongpollConditional {
		return fmt.Errorf("can't combine LongpollRedirect with LongpollConditional")
	}

	if obj.Longpoll && (!obj.LongpollRedirect && !obj.LongpollConditional) {
		return fmt.Errorf("with Longpoll must specify LongpollRedirect or LongpollConditional")
	}

	// XXX: If we obj.Longpoll but don't have obj.MtimeCheck will it be a
	// problem of infinite re-downloading since we have nothing to pause on?
	//if obj.Longpoll && !obj.MtimeCheck {
	//	return fmt.Errorf("can't Longpoll without MtimeCheck")
	//}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPClientRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.tmp = filepath.Join(dir, "tmp") // return a unique file

	obj.dst = obj.getDst()
	if obj.dst == "" { // user didn't specify file
		obj.dst = filepath.Join(dir, "dst") // so return a unique file!
	}

	obj.respCh = make(chan *http.Response, 1)
	obj.ackCh = make(chan struct{}, 1)

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPClientRes) Cleanup() error {
	// Watch has now exited, but we might still have a response in the chan.
	// Close the body so that we don't leak the underlying connection.
	select {
	case r := <-obj.respCh:
		r.Body.Close()
	default:
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *HTTPClientRes) Watch(ctx context.Context) error {

	if obj.MetaParams().Poll > 0 {
		// programming error
		return fmt.Errorf("unexpected Watch when polling")
	}
	if obj.MetaParams().Poll < 0 {
		// programming error
		return fmt.Errorf("unexpected Watch when polling once")
	}

	if !obj.Longpoll {
		return fmt.Errorf("must use Meta:poll when not using Longpoll")
	}

	// Here we HTTP long poll. This holds open an HTTP request against
	// obj.URL and fires an event each time the server returns. This gives
	// us immediate events for remote endpoints over HTTP.

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	// In case someone messes with the dst file...
	recWatcher, err := recwatch.NewRecWatcher(obj.dst, false)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventCh := make(chan struct{}) // events from long poll Watch!
	pollErr := make(chan error)    // long poll Watch if it errors
	//defer close(eventCh)

	checkRedirectFunc := func(req *http.Request, via []*http.Request) error {
		// Watch is ready!
		if !obj.LongpollRedirect { // not long poll related, so continue
			return nil // follows normally
		}

		// If CheckRedirect returns an error, the Client's Get method
		// returns both the previous Response (with its Body closed) and
		// CheckRedirect's error (wrapped in a url.Error) instead of
		// issuing the Request req. This means Watch() should error out,
		// which is the important behaviour that we depend on.
		select {
		case eventCh <- struct{}{}:
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	client := &http.Client{
		// No client-side timeout! the whole point of a long poll is
		// that the server holds the request until it has something to
		// send. ctx cancellation is what unblocks us on shutdown.
		//Timeout: time.Duration(timeout) * time.Second, // don't!
		// XXX: would the download part of the response need a timeout?
		CheckRedirect: checkRedirectFunc,
	}

	// Startup the long poll Watch in parallel...
	wg.Add(1)
	go func() {
		defer close(pollErr)
		defer wg.Done()
		select {
		case pollErr <- obj.longpollWatch(ctx, client, eventCh):

		case <-ctx.Done(): // unblock
		}
	}()

	// When do we send our startup notification?
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-eventCh: // long poll goroutine event!

		case err, ok := <-pollErr:
			if !ok {
				pollErr = nil
				continue
			}
			// The long poll goroutine exited. On clean shutdown
			// this is the ctx error, otherwise it's a real failure.
			// Either way Watch is done.
			return err

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// longpollWatch runs the blocking HTTP long poll loop.
func (obj *HTTPClientRes) longpollWatch(ctx context.Context, client *http.Client, eventCh chan<- struct{}) error {
	// If wait is false, it means the first request isn't long polling.
	wait := false
	grace := &retryAfterGrace{Max: httpClientLongpollGraceTime}
	for {
		req, err := obj.getRequest(ctx, wait)
		if err != nil {
			return errwrap.Wrapf(err, "could not build long poll request")
		}
		if obj.LongpollConditional {
			wait = true
		}

		resp, err := client.Do(req)
		if err != nil {
			// A cancelled context is a clean shutdown, not a watch
			// failure. Return the ctx error if there was one so the
			// engine recognizes and strips it.
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Retry transport failures if we're in the grace window
			// started when we got the "retry after" result back...
			if d, ok := grace.Delay(time.Now()); ok {
				obj.init.Logf("long poll request failed during grace period, retry after %s: %+v", d, err)
				if err := ctxSleep(ctx, d); err != nil {
					return err
				}
				continue
			}

			return errwrap.Wrapf(err, "long poll request failed")
		}

		// Did the server tell us to reconnect?
		if d, ok, err := obj.retryAfter(resp); err != nil {
			resp.Body.Close()
			return err

		} else if ok {
			resp.Body.Close()

			obj.init.Logf("long poll server requested reconnect after %s", d)
			if err := ctxSleep(ctx, d); err != nil {
				return err
			}

			grace.Start(time.Now())
			continue
		}
		grace.Stop()

		// Drain any stale, un-consumed response before sending the new
		// one. The ackCh handshake means this shouldn't fire normally,
		// but it is an extra point of redundancy just to make sure...
		select {
		case old := <-obj.respCh:
			old.Body.Close()

		default:
		}

		// Hand the live response off to CheckApply. CheckApply owns it
		// and will run resp.Body.Close() when it is done unless it is
		// pulled back in during the above select.
		select {
		case obj.respCh <- resp:

		case <-ctx.Done():
			resp.Body.Close()
			return ctx.Err()
		}

		// Notify the parent Watch() that new data is available!
		select {
		case eventCh <- struct{}{}:

		case <-ctx.Done():
			return ctx.Err()
		}

		// Backpressure: wait until CheckApply has consumed the body
		// before issuing the next long poll.
		select {
		case <-obj.ackCh:

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CheckApply for this resource downloads data over HTTP.
// XXX: This will contact the http endpoint even when in noop (!apply) mode. Is
// that a bug? Should it never POST? What's the correct approach?
// XXX: We don't want the initial CheckApply to return true until the Watch has
// started up, so we must block there until that's the case if their Startup is
// not perfectly timed. (How can a long poll always know when watcher is ready?)
func (obj *HTTPClientRes) CheckApply(ctx context.Context, apply bool) (checkOK bool, reterr error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}
	obj.sha256 = "" // start by always invalidating the sha256 cache

	// Reset the response state that we publish to the local bridge so that
	// the net/http.response("${name}") function can read it. Below we set
	// obj.status and obj.output at the points wherever we can determine
	// their values.
	obj.status = httpClientStatusNone
	obj.output = ""
	defer func() {
		if err := obj.publishResponse(ctx, reterr); err != nil { // reads whatever is in reterr
			// rare, and not even possible today
			checkOK = false
			reterr = errwrap.Append(reterr, err)
		}
	}()

	if obj.MetaParams().Poll != 0 {
		// TODO: merge this code into this main CheckApply if we can...
		return obj.pollCheckApply(ctx, apply)
	}

	if !obj.Longpoll {
		// programming error
		return false, fmt.Errorf("must use Meta:poll when not using Longpoll")
	}

	return obj.longpollCheckApply(ctx, apply)
}

// longpollCheckApply consumes a response handed off by Watch over respChan so
// we don't have to open a second connection just to fetch the body. If no
// response is pending, it falls back to a one-shot fetch via pollCheckApply.
// The initial CheckApply often happens due to the initial Watch startup event,
// since server-side changes might not be happening constantly. These can also
// happen if a Refresh or another routine parent CheckApply pokes us.
func (obj *HTTPClientRes) longpollCheckApply(ctx context.Context, apply bool) (bool, error) {
	select {
	case resp, ok := <-obj.respCh: // buffered
		if !ok {
			// programming error
			return false, fmt.Errorf("resp chan unexpectedly closed")
		}

		// We have a pending "live" response from Watch. Ack on exit
		// (success or failure) so Watch can issue the next long poll.
		// This is non-blocking because Watch might have exited early.
		defer func() {
			select {
			case obj.ackCh <- struct{}{}:
			default:
			}
		}()

		return obj.processResp(ctx, resp, apply)

	default:
		// Nothing pending, so fallthrough and run a standalone request.
	}

	return obj.pollCheckApply(ctx, apply)
}

// pollCheckApply is the simple, non-Watching, non long polling CheckApply.
// TODO: merge this code into this main CheckApply if we can...
func (obj *HTTPClientRes) pollCheckApply(ctx context.Context, apply bool) (bool, error) {

	req, err := obj.getRequest(ctx, false)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not build request")

	}

	checkOK := true

	// Stat whichever on-disk copy we're looking for. If we have one at the
	// final destination, that's perfect, if not then we use the vardir one.
	if _, err := os.Stat(obj.dst); err != nil && !os.IsNotExist(err) {
		return false, err // real errors are always returned

	} else if err != nil { // file doesn't exist
		checkOK = false
	}

	// If !checkOK, no need to check if hash matches.
	if checkOK && obj.Sha256 != "" {
		sum, err := obj.hashFile(obj.dst)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not hash %s", obj.dst)
		}
		obj.sha256 = sum // cache
		if !strings.EqualFold(obj.sha256, obj.Sha256) {
			// XXX: should we just delete it right now or wait?
			checkOK = false // hash is bad, rewrite
		}
	}

	// Everything looks okay, return early. An upstream Refresh overrides
	// this since the caller has explicitly asked us to re-fetch for some
	// reason. We can only do this if the hash is correct. If it does not
	// match (we checked it above) or it's empty, then we can not confirm
	// that the file isn't new because we don't have a metric to compare!
	if checkOK && obj.Sha256 != "" && !obj.init.Refresh() {
		content, err := obj.readAll()
		if err != nil {
			return false, err
		}
		if err := obj.send(content); err != nil { // send/recv
			return false, err
		}

		// A local sha256 match is positive confirmation that we hold
		// valid data, so report 200 and pass along the path. This also
		// seeds the bridge with a 200 on a fresh start where we find a
		// correct file already on disk but have no previously captured
		// state. (Unlike a 304, this isn't relabeling a server's code.)
		obj.status = http.StatusOK
		obj.output = obj.dst
		return true, nil
	}

	// Don't send a POST or other similar "dangerous" methods if we're only
	// in noop mode. Right now we only allow GET or HEAD when not applying.
	// XXX: What other methods should be avoided? Is it safe to run a HEAD?
	if !apply && obj.getMethod() != http.MethodGet && obj.getMethod() != http.MethodHead {
		// XXX: do we need to send/recv in noop mode?
		return false, nil
	}

	client := &http.Client{
		// Don't add a timeout here! If you want to prevent a download
		// from taking too long, then use the Meta:timeout param.
		//Timeout: time.Duration(timeout) * time.Second, // don't!
	}

	resp, err := client.Do(req) // Actually run the request.
	if err != nil {
		return false, err
	}

	return obj.processResp(ctx, resp, apply)
}

// processResp runs the CheckApply work for an http response. It does everything
// that happens after the typical `!apply {}` stage, and after we've received an
// *http.Response. It's split out into a helper with this kind of type signature
// because we may process from a standalone CheckApply, or we may process from a
// response which came from the Watch longpoll which eliminates needing a second
// http request to get the data for CheckApply! This runs the Close on the body.
// This has the same return signature as what is used when running CheckApply...
func (obj *HTTPClientRes) processResp(ctx context.Context, resp *http.Response, apply bool) (bool, error) {

	defer resp.Body.Close()
	if obj.init.Debug {
		obj.init.Logf("resp: %v", resp)
	}

	// Did the http server tell us that our file is already just fine?
	if resp.StatusCode == http.StatusNotModified && obj.MtimeCheck {
		obj.init.Logf("file is up to date, skipping download")
		content, err := obj.readAll()
		if err != nil {
			return false, err
		}
		if err := obj.send(content); err != nil { // send/recv
			return false, err
		}

		// 304 means our on-disk data is still current. Record this as
		// the outcome and let publishResponse decide what to do with
		// the bridge. Usually it leaves the last captured value alone
		// (so a steady stream of 304s never surfaces as a status), but
		// it seeds a 200 on a fresh start where the bridge is empty.
		obj.status = http.StatusNotModified
		obj.output = obj.dst
		return true, nil
	}
	if resp.StatusCode != http.StatusOK {
		obj.status = resp.StatusCode // report the HTTP code (eg: 404, 500)
		obj.output = ""
		return false, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	// TODO: are there more ways to early return with (true, nil) right now?

	// In noop mode we must not write to disk. The server has content for us
	// but saving it (writing the file) is a change we're not allowed to do.
	if !apply {
		// XXX: leave the obj.status / obj.output untouched rather than
		// reporting a code for data we didn't actually capture?
		// XXX: do we need to send/recv in noop mode?
		return false, nil
	}

	// If we're writing a file, we should stream it to a vardir and then
	// swap it atomically so that (1) we don't buffer the entire file into
	// memory and (2) so that we don't have a partial (corrupt) file if the
	// http fails to complete or validate the hash correctly. Note that we
	// *don't* store the file twice in the steady state, since the vardir
	// version overwrites the final destination when things check out okay.
	tmpdel := true
	defer func() {
		// once file moved away, we should not run remove!
		if !tmpdel {
			return
		}
		os.Remove(obj.tmp) // cleanup the partial file if not moved...
	}()

	file, err := os.Create(obj.tmp)
	if err != nil {
		return false, err
	}
	defer file.Close() // will return an error if it has already been called

	hasher := sha256.New()
	buf := &bytes.Buffer{} // for send/recv
	pw := &progressWriter{
		// XXX: pass in expected total bytes so we can log a known % ?
		Logf:  obj.init.Logf,
		Debug: obj.init.Debug,
	}
	// XXX: can we avoid writing to buf if aren't sending to anyone?
	if _, err := io.Copy(io.MultiWriter(file, hasher, buf, pw), resp.Body); err != nil {
		return false, err
	}

	var got string
	sum := func() string {
		if got == "" {
			got = hex.EncodeToString(hasher.Sum(nil))
		}
		return got
	}

	if obj.Sha256 != "" {
		// stream check the sha256 and don't keep it if it's bad...
		if !strings.EqualFold(sum(), obj.Sha256) { // case-insensitive
			return false, fmt.Errorf("sha256 mismatch: got %s, expected %s", got, obj.Sha256)
		}
	}

	if err := file.Sync(); err != nil {
		return false, err
	}
	// "The behavior of Close after the first call is undefined. Specific
	// implementations may document their own behavior." We double close in
	// defer above, which as per the *os.File implementation is allowed.
	if err := file.Close(); err != nil {
		return false, err
	}

	// Set the mtime on the download from the server's Last-Modified header
	// so that subsequent calls using the `If-Modified-Since` header will
	// prevent unnecessary redownloading! The rename below carries this mtime
	// onto dst, where it acts as our long poll cursor.
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		t, err := http.ParseTime(lm)
		if err != nil { // not a permanent error
			obj.init.Logf("could not parse Last-Modified %q: %v", lm, err)

		} else if err := os.Chtimes(obj.tmp, t, t); err != nil {
			return false, errwrap.Wrapf(err, "could not set mtime on %s", obj.tmp)
		}
	}

	if obj.sha256 == "" { // not cached
		sum, err := obj.hashFile(obj.dst)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not hash %s", obj.dst)
		}
		obj.sha256 = sum // cache
	}

	// Did the freshly downloaded data match what's already on disk? If so,
	// this CheckApply reports unchanged (returns true), but we still need
	// to Chtimes it below (via Rename) so that dst's mtime advances to the
	// server version. That mtime is our long poll cursor: if stale it would
	// make the server return immediately on every poll in an infinite loop.
	unchanged := obj.sha256 == sum()

	// XXX: VarDir may be on a different filesystem than dst, in which case
	// os.Rename fails with syscall.EXDEV. Eventually VarDir should expose
	// that and give us an API that lets us receive a dir on the same
	// filesystem we're on or just fall back to a copy to the same dir as
	// the dst but with a .tmp?
	if err := os.Rename(obj.tmp, obj.dst); err != nil {
		return false, err
	}
	tmpdel = false

	content := buf.String()
	if err := obj.send(content); err != nil { // send/recv
		return false, err
	}

	obj.status = resp.StatusCode // 200, freshly downloaded and validated
	obj.output = obj.dst
	return unchanged, nil
}

// publishResponse writes the current response state to the local bridge so that
// a corresponding net/http.response("${name}") function can read and watch it.
// We publish when there's a new outcome to report: a fresh download (the real
// status code paired with its data), an HTTP failure code, or -1 for an engine
// type error (eg: a transport, disk, or validation failure). When nothing
// changes we leave the bridge untouched so the status and data captured on the
// last real change persist. The http.StatusNotModified (304) case is special:
// the data is current but it's not a new outcome, so we keep whatever the
// bridge already has (which keeps a steady stream of 304s from ever surfacing
// as a status, so consumers see a stable 200 rather than a 200-or-304), unless
// the bridge is still empty, in which case we seed a 200 since we genuinely
// hold valid data. We don't publish during a clean shutdown.
func (obj *HTTPClientRes) publishResponse(ctx context.Context, reterr error) error {
	if ctx.Err() != nil { // clean shutdown, don't publish a spurious error
		return nil // let CheckApply pass through the correct error!
	}

	status := obj.status
	switch obj.status {
	case httpClientStatusNone:
		if reterr == nil {
			return nil // nothing changed, keep the bridge's last value
		}
		status = httpClientStatusError // engine-level failure, no HTTP code

	case http.StatusNotModified: // 304
		prev, err := obj.init.Local.HTTPGet(ctx, obj.Name())
		if err != nil {
			// rare, and not even possible today
			return errwrap.Wrapf(err, "could not read http response")
		}
		if prev == nil {
			// programming error
			return fmt.Errorf("unexpected nil response")
		}
		if prev.Status != 0 { // the bridge already holds a captured value
			return nil // keep the bridge's last captured value
		}
		status = http.StatusOK // seed a 200 since we hold valid data
	}

	if err := obj.init.Local.HTTPSet(ctx, obj.Name(), status, obj.output); err != nil {
		// rare, and not even possible today
		return errwrap.Wrapf(err, "could not publish http response")
	}

	return nil
}

// send is a helper to avoid duplication of the same send operation.
func (obj *HTTPClientRes) send(content string) error {
	return obj.init.Send(&HTTPClientSends{
		Content: &content,
	})
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPClientRes) Cmp(r engine.Res) error {
	// we can only compare HTTPClientRes to others of the same resource kind
	res, ok := r.(*HTTPClientRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.File != res.File {
		return fmt.Errorf("the File differs")
	}
	if obj.Method != res.Method {
		return fmt.Errorf("the Method differs")
	}
	if obj.URL != res.URL {
		return fmt.Errorf("the URL differs")
	}

	if (obj.Body == nil) != (res.Body == nil) { // xor
		return fmt.Errorf("the Body differs")
	}
	if obj.Body != nil && res.Body != nil {
		if *obj.Body != *res.Body { // compare the strings
			return fmt.Errorf("the contents of Body differ")
		}
	}

	if obj.MtimeCheck != res.MtimeCheck {
		return fmt.Errorf("the MtimeCheck differs")
	}
	if obj.Sha256 != res.Sha256 {
		return fmt.Errorf("the Sha256 differs")
	}

	if obj.Longpoll != res.Longpoll {
		return fmt.Errorf("the Longpoll differs")
	}
	if obj.LongpollRedirect != res.LongpollRedirect {
		return fmt.Errorf("the LongpollRedirect differs")
	}
	if obj.LongpollConditional != res.LongpollConditional {
		return fmt.Errorf("the LongpollConditional differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPClientRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPClientRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*HTTPClientRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPClientRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPClientRes(raw) // restore from indirection with type conversion!
	return nil
}

// HTTPClientSends is the struct of data which is sent after a successful
// download. Content is a *string but holds raw bytes. Golang strings are not
// constrained to UTF-8, so binary payloads pass through unchanged.
type HTTPClientSends struct {
	// Content is the downloaded body.
	Content *string `lang:"content"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *HTTPClientRes) Sends() interface{} {
	return &HTTPClientSends{
		Content: nil,
	}
}

// progressWriter is just a helper that implementes io.Writer to give the user
// progress updates on a slow download. It throttles its log output so a fast
// stream doesn't drown the log.
type progressWriter struct {
	Logf  func(format string, v ...interface{})
	Debug bool

	written int64
	lastLog time.Time
}

// Write is the method required by the io.Writer interface.
func (obj *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	obj.written += int64(n)
	now := time.Now()
	// Log the first write so the user sees the download started, then
	// at most once per second after that.
	if obj.lastLog.IsZero() || now.Sub(obj.lastLog) >= time.Second {
		obj.Logf("downloaded %d bytes", obj.written)
		obj.lastLog = now
	}
	return n, nil
}

// retryAfterGrace is a helper to do the delay math associated with grace retry.
type retryAfterGrace struct {
	Max int // grace period

	until time.Time     // This much time till it's over.
	delay time.Duration // How long we should wait before retrying again?
}

// Start counting from now.
func (obj *retryAfterGrace) Start(now time.Time) {
	obj.until = now.Add(time.Duration(obj.Max) * time.Second)
	obj.delay = 0
}

// Stop resets the counters.
func (obj *retryAfterGrace) Stop() {
	obj.until = time.Time{}
	obj.delay = 0
}

// Delay tells us how much more time to wait if we should keep doing so!
func (obj *retryAfterGrace) Delay(now time.Time) (time.Duration, bool) {
	remaining := obj.until.Sub(now)
	if remaining <= 0 {
		obj.Stop()
		return 0, false
	}

	if obj.delay <= 0 {
		obj.delay = time.Second // 1 sec minimum
	}
	delay := obj.delay
	if delay > remaining {
		delay = remaining
	}

	obj.delay *= 2 // exponential backoff: 1, 2, 4, ...
	if obj.delay > time.Duration(obj.Max)*time.Second {
		obj.delay = time.Duration(obj.Max) * time.Second
	}

	return delay, true
}

// ctxSleep returns when either ctx does or duration timer exits/finishes. If
// duration is zero we only wait on ctx.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
