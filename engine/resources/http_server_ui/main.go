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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"syscall/js"
	"time"

	"github.com/purpleidea/mgmt/engine/resources/http_server_ui/common"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Main is the main implementation of this process. It holds our shared data.
type Main struct {
	// some values we pull in
	program  string
	version  string
	hostname string
	title    string
	path     string

	document js.Value
	body     js.Value

	// window.location.origin (the base url with port for XHR)
	wlo string

	// base is the wlo + the specific path suffix
	base string

	response chan *Response
}

// Init must be called before the Main struct is used.
func (obj *Main) Init() error {
	fmt.Println("Hello from mgmt wasm!")

	obj.program = js.Global().Get("_mgmt_program").String()
	obj.version = js.Global().Get("_mgmt_version").String()
	obj.hostname = js.Global().Get("_mgmt_hostname").String()
	obj.title = js.Global().Get("_mgmt_title").String()
	obj.path = js.Global().Get("_mgmt_path").String()

	obj.document = js.Global().Get("document")
	obj.body = obj.document.Get("body")

	obj.wlo = js.Global().Get("window").Get("location").Get("origin").String()

	obj.base = obj.wlo + obj.path

	obj.response = make(chan *Response)

	return nil
}

// Run is the main execution of this program.
func (obj *Main) Run() error {
	h1 := obj.document.Call("createElement", "h1")
	h1.Set("innerHTML", obj.title)
	obj.body.Call("appendChild", h1)

	h6 := obj.document.Call("createElement", "h6")
	pre := obj.document.Call("createElement", "pre")
	pre.Set("textContent", fmt.Sprintf("This is: %s, version: %s, on %s", obj.program, obj.version, obj.hostname))
	//pre.Set("innerHTML", fmt.Sprintf("This is: %s, version: %s, on %s", obj.program, obj.version, obj.hostname))
	h6.Call("appendChild", pre)
	obj.body.Call("appendChild", h6)

	obj.body.Call("appendChild", obj.document.Call("createElement", "hr"))

	//document.baseURI
	// XXX: how to get the base so we can add our own querystring???
	fmt.Println("URI: ", obj.document.Get("baseURI").String())
	fmt.Println("window.location.origin: ", obj.wlo)

	fmt.Println("BASE: ", obj.base)

	fieldset := obj.document.Call("createElement", "fieldset")
	legend := obj.document.Call("createElement", "legend")
	legend.Set("textContent", "live!") // XXX: pick some message here
	fieldset.Call("appendChild", legend)

	// XXX: consider using this instead: https://github.com/hashicorp/go-retryablehttp
	//client := retryablehttp.NewClient()
	//client.RetryMax = 10
	client := &http.Client{
		//Timeout:       time.Duration(timeout) * time.Second,
		//CheckRedirect: checkRedirectFunc,
	}

	// Startup form building...
	// XXX: Add long polling to know if the form shape changes, and offer a
	// refresh to the end-user to see the new form.
	listURL := obj.base + "list/"
	watchURL := obj.base + "watch/"
	resp, err := client.Get(listURL) // works
	if err != nil {
		return errwrap.Wrapf(err, "could not list ui")
	}
	s, err := io.ReadAll(resp.Body) // TODO: apparently we can stream
	resp.Body.Close()
	if err != nil {
		return errwrap.Wrapf(err, "could read from listed ui")
	}

	fmt.Printf("Response: %+v\n", string(s))

	var form *common.Form
	if err := json.Unmarshal(s, &form); err != nil {
		return errwrap.Wrapf(err, "could not unmarshal form")
	}
	//fmt.Printf("%+v\n", form) // debug

	// Sort according to the "sort" field so elements are in expected order.
	sort.Slice(form.Elements, func(i, j int) bool {
		return form.Elements[i].Sort < form.Elements[j].Sort
	})

	for _, x := range form.Elements {
		id := x.ID
		resp, err := client.Get(listURL + id)
		if err != nil {
			return errwrap.Wrapf(err, "could not get id %s", id)
		}
		s, err := io.ReadAll(resp.Body) // TODO: apparently we can stream
		resp.Body.Close()
		if err != nil {
			return errwrap.Wrapf(err, "could not read from id %s", id)
		}
		fmt.Printf("Response: %+v\n", string(s))

		var element *common.FormElementGeneric // XXX: switch based on x.Kind
		if err := json.Unmarshal(s, &element); err != nil {
			return errwrap.Wrapf(err, "could not unmarshal id %s", id)
		}
		//fmt.Printf("%+v\n", element) // debug

		inputType, exists := x.Type[common.HTTPServerUIInputType] // "text" or "range" ...
		if !exists {
			fmt.Printf("Element has no input type: %+v\n", element)
			continue
		}

		label := obj.document.Call("createElement", "label")
		label.Call("setAttribute", "for", id)
		label.Set("innerHTML", fmt.Sprintf("%s: ", id))
		fieldset.Call("appendChild", label)

		el := obj.document.Call("createElement", "input")
		el.Set("id", id)
		//el.Call("setAttribute", "id", id)
		//el.Call("setAttribute", "name", id)
		el.Set("type", inputType)

		if inputType == common.HTTPServerUIInputTypeRange {
			if val, exists := x.Type[common.HTTPServerUIInputTypeRangeMin]; exists {
				el.Set("min", val)
			}
			if val, exists := x.Type[common.HTTPServerUIInputTypeRangeMax]; exists {
				el.Set("max", val)
			}
			if val, exists := x.Type[common.HTTPServerUIInputTypeRangeStep]; exists {
				el.Set("step", val)
			}
		}

		el.Set("value", element.Value) // XXX: here or after change handler?

		// event handler
		changeEvent := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			event := args[0]
			value := event.Get("target").Get("value").String()

			//obj.wg.Add(1)
			go func() {
				//defer obj.wg.Done()
				fmt.Println("Action!")

				u := obj.base + "save/"
				values := url.Values{
					"id":    {id},
					"value": {value},
				}

				resp, err := http.PostForm(u, values)
				//fmt.Println(resp, err) // debug
				s, err := io.ReadAll(resp.Body) // TODO: apparently we can stream
				resp.Body.Close()
				fmt.Printf("Response: %+v\n", string(s))
				fmt.Printf("Error: %+v\n", err)
				obj.response <- &Response{
					Str: string(s),
					Err: err,
				}
			}()

			return nil
		})
		defer changeEvent.Release()
		el.Call("addEventListener", "change", changeEvent)

		// http long poll
		go func() {
			for {
				fmt.Printf("About to long poll for: %s\n", id)
				//resp, err := client.Get(watchURL + id) // XXX: which?
				resp, err := http.Get(watchURL + id)
				if err != nil {
					fmt.Println("Error fetching:", watchURL+id, err) // XXX: test error paths
					time.Sleep(2 * time.Second)
					continue
				}

				s, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					fmt.Println("Error reading response:", err)
					time.Sleep(2 * time.Second)
					continue
				}

				var element *common.FormElementGeneric // XXX: switch based on x.Kind
				if err := json.Unmarshal(s, &element); err != nil {
					fmt.Println("could not unmarshal id %s: %v", id, err)
					time.Sleep(2 * time.Second)
					continue
				}
				//fmt.Printf("%+v\n", element) // debug

				fmt.Printf("Long poll for %s got: %s\n", id, element.Value)

				obj.document.Call("getElementById", id).Set("value", element.Value)
				//time.Sleep(1 * time.Second)
			}
		}()

		fieldset.Call("appendChild", el)
		br := obj.document.Call("createElement", "br")
		fieldset.Call("appendChild", br)
	}

	obj.body.Call("appendChild", fieldset)

	// We need this mainloop for receiving the results of our async stuff...
	for {
		select {
		case resp, ok := <-obj.response:
			if !ok {
				break
			}
			if err := resp.Err; err != nil {
				fmt.Printf("Err: %+v\n", err)
				continue
			}
			fmt.Printf("Str: %+v\n", resp.Str)
		}
	}

	return nil
}

// Response is a standard response struct which we pass through.
type Response struct {
	Str string
	Err error
}

func main() {
	m := &Main{}
	if err := m.Init(); err != nil {
		fmt.Printf("Error: %+v\n", err)
		return
	}

	if err := m.Run(); err != nil {
		fmt.Printf("Error: %+v\n", err)
		return
	}

	select {} // don't shutdown wasm
}
