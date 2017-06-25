// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	errwrap "github.com/pkg/errors"
	"github.com/purpleidea/mgmt/resources"
)

func init() {
	resources.RegisterResource("http", func() resources.Res { return &HTTPRes{} })
}

type event struct {
	Error   error
	BodySum string
}

// HTTPRes is an http resource for retrieving data over http
type HTTPRes struct {
	resources.BaseRes `yaml:",inline"`

	client *http.Client

	URL      string            `yaml:"url"`
	Timeout  int               `yaml:"timeout"` // the http timeout in seconds
	Method   string            `yaml:"method"`
	Header   map[string]string `yaml:"header"`
	Body     string            `yaml:"body"`
	Interval string            `yaml:"interval"`

	ticker *time.Ticker
	ShaSum string

	Response *string
	Code     *string
	Headers  http.Header

	events chan event
	close  chan struct{}

	wg sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPRes) Default() resources.Res {
	return &HTTPRes{
		BaseRes: resources.BaseRes{
			MetaParams: resources.DefaultMetaParams, // force a default
		},
		Interval: "10s",
		Method:   http.MethodGet,
	}
}

// Validate if the params passed in are valid data.
func (obj *HTTPRes) Validate() error {
	if obj.URL == "" { // this is the only thing that is really required
		return fmt.Errorf("url can't be empty")
	}

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *HTTPRes) Init() error {
	obj.ShaSum = ""
	obj.events = make(chan event)
	obj.close = make(chan struct{})

	d, err := time.ParseDuration(obj.Interval)
	if err != nil {
		return fmt.Errorf("invalid interval setting: %s", err)
	}

	obj.ticker = time.NewTicker(d)

	obj.client = &http.Client{
		Timeout: time.Duration(obj.Timeout) * time.Second,
	}

	obj.SetKind("http")

	go obj.poll()

	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

func (obj *HTTPRes) poll() {
	obj.wg.Add(1)
	defer obj.wg.Done()
	fmt.Printf("polling: every %s", obj.Interval)
	for {
		select {
		case _, ok := <-obj.close:
			if !ok {
				return
			}
		case <-obj.ticker.C:
			res, err := obj.do()
			if err != nil {
				obj.events <- event{Error: err}
			}

			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				obj.events <- event{Error: err}
			}

			hash := sha256.New()
			hash.Write(b)
			shaSum := base64.StdEncoding.EncodeToString(hash.Sum(nil))

			if shaSum != obj.ShaSum {
				obj.events <- event{BodySum: shaSum}
			}
		}
	}
}

func (obj *HTTPRes) closeWatcher() {
	close(obj.close)
	obj.wg.Wait()
	close(obj.events)
}

func (obj *HTTPRes) watch() <-chan event {
	return obj.events
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *HTTPRes) Watch() error {
	defer obj.closeWatcher()

	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error
	for {
		select {
		case e, ok := <-obj.watch():
			if !ok {
				return nil
			}
			if err := e.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}

}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *HTTPRes) CheckApply(apply bool) (bool, error) {
	if val, exists := obj.Recv["URL"]; exists && val.Changed {
		// if we received on Content, and it changed, invalidate the cache!
		log.Printf("contentCheckApply: Invalidating sha256sum of `Content`")
		obj.ShaSum = "" // invalidate!!
	}

	res, err := obj.do()
	if err != nil {
		return false, nil
	}

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	if obj.ShaSum == "" { // cache is invalid
		hash := sha256.New()
		hash.Write(b)
		obj.ShaSum = base64.StdEncoding.EncodeToString(hash.Sum(nil))
	}

	t := string(b)
	obj.Response = &t

	return true, nil
}

func (obj *HTTPRes) do() (*http.Response, error) {
	body := bytes.NewBufferString(obj.Body)
	req, err := http.NewRequest(obj.Method, obj.URL, body)
	if err != nil {
		return nil, err
	}

	return obj.client.Do(req)
}

// HTTPUID is the UID struct for HTTPRes.
type HTTPUID struct {
	resources.BaseUID
	URL string
	// TODO: add more elements here
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *HTTPRes) AutoEdges() (resources.AutoEdge, error) {
	// TODO: parse as many exec params to look for auto edges, for example
	// the path of the binary in the Cmd variable might be from in a pkg
	return nil, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *HTTPRes) UIDs() []resources.ResUID {
	x := &HTTPUID{
		BaseUID: resources.BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		URL:     obj.URL,
		// TODO: add more params here
	}
	return []resources.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *HTTPRes) GroupCmp(r resources.Res) bool {
	_, ok := r.(*HTTPRes)
	if !ok {
		return false
	}
	return false // not possible atm
}

// Compare two resources and return if they are equivalent.
func (obj *HTTPRes) Compare(r resources.Res) bool {
	// we can only compare ExecRes to others of the same resource kind
	res, ok := r.(*HTTPRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *HTTPRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*HTTPRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPRes(raw) // restore from indirection with type conversion!
	return nil
}
