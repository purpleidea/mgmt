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
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

func init() {
	RegisterResource("http", func() Res { return &HTTPRes{} })
}

// HTTPRes is an http resource for retrieving data over http
type HTTPRes struct {
	BaseRes        `yaml:",inline"`
	URL            string            `yaml:"url"`
	Timeout        int               `yaml:"timeout"` // the http timeout in seconds
	Method         string            `yaml:"method"`
	Header         map[string]string `yaml:"header"`
	Body           string            `yaml:"body"`
	responseBody   *string
	responseCode   int
	responseHeader http.Header
	client         *http.Client
}

type httpResponse struct {
	code   int
	body   string
	err    error
	header http.Header
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPRes) Default() Res {
	return &HTTPRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
		Method: http.MethodGet,
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
	obj.client = &http.Client{
		Timeout: time.Duration(obj.Timeout) * time.Second,
	}

	obj.SetKind("http")
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *HTTPRes) Watch() error {

	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error

	wg := &sync.WaitGroup{}

	httpResponseChan := make(chan *httpResponse)
	closeChan := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			body := bytes.NewBufferString(obj.Body)
			req, err := http.NewRequest(obj.Method, obj.URL, body)
			if err != nil {
				resp := httpResponse{0, "", err, nil}
				select {
				case httpResponseChan <- &resp:
					return
				case <-closeChan:
					return
				}
			}
			req = req.WithContext(ctx)

			res, err := obj.client.Do(req)
			if err != nil {
				resp := httpResponse{0, "", err, nil}
				select {
				case httpResponseChan <- &resp:
					return
				case <-closeChan:
					return
				}
			}
			response_body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				resp := httpResponse{res.StatusCode, string(response_body), err, res.Header}
				select {
				case httpResponseChan <- &resp:
					return
				case <-closeChan:
					return
				}
			}
			resp := httpResponse{res.StatusCode, string(response_body), nil, res.Header}
			select {
			case httpResponseChan <- &resp:
			case <-closeChan:
				return
			}
		}
	}()
	defer wg.Wait()
	defer close(closeChan)
	defer cancel()
	for {
		select {
		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		case result, ok := <-httpResponseChan:
			if !ok {
				log.Printf("channel closed")
				return nil
			}
			if result.err != nil {
				return result.err
			}
			obj.responseCode = result.code
			obj.responseBody = &result.body
			obj.responseHeader = result.header
			log.Printf("we got a chan message of %d %s", result.code, result.body)
			send = true
			obj.StateOK(false) // dirty
		}
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
		log.Printf("contentCheckApply: Stuff")
	}

	return true, nil
}

// HTTPUID is the UID struct for HTTPRes.
type HTTPUID struct {
	BaseUID
	URL string
	// TODO: add more elements here
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *HTTPRes) AutoEdges() (AutoEdge, error) {
	// TODO: parse as many exec params to look for auto edges, for example
	// the path of the binary in the Cmd variable might be from in a pkg
	return nil, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *HTTPRes) UIDs() []ResUID {
	x := &HTTPUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		URL:     obj.URL,
		// TODO: add more params here
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *HTTPRes) GroupCmp(r Res) bool {
	_, ok := r.(*HTTPRes)
	if !ok {
		return false
	}
	return false // not possible atm
}

// Compare two resources and return if they are equivalent.
func (obj *HTTPRes) Compare(r Res) bool {
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
