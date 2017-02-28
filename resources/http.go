// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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
	"encoding/gob"
	"fmt"

	"github.com/purpleidea/mgmt/event"
	"github.com/r3labs/sse"
)

func init() {
	gob.Register(&HttpRes{})
}

// HttpRes is a resource that fetches information using the http protocol,
// and even pushes back the information when it does not match, or sends it
// to other resources, using the send/recv mechanism.
type HttpRes struct {
	BaseRes `yaml:",inline"`

	// URL is the url targetted by this resource.
	URL string `yaml:"url"`

	// Method is the method to use: fetch or push.
	// The fetch method will retrieve data with a GET method,
	// and use the send/recv mechanism.
	// The push method will retrieve data, and POST
	// it as soon as it differs.
	Method string `yaml:"method"`

	// Payload is the data that is pushed in case of the "push" Method.
	Payload string `yaml:"payload"`

	// SSE Stream to subscribe to.
	Stream string `yaml:"stream"`

	// CheckSSL defines if we should check SSL certificate validity.
	CheckSSL bool `yaml:"check_ssl"`
}

// HttpSet represents a key/value pair of settings to be applied.
type HttpSet struct {
	Path  string `yaml:"path"`  // The relative path to the value to be changed.
	Value string `yaml:"value"` // The value to be set on the given Path.
}

// NewHttpRes is a constructor for this resource. It also calls Init() for you.
func NewHttpRes(name string) (*HttpRes, error) {
	obj := &HttpRes{
		BaseRes: BaseRes{
			Name: name,
		},
	}
	return obj, obj.Init()
}

// Default returns some sensible defaults for this resource.
func (obj *HttpRes) Default() Res {
	return &HttpRes{
		Method:   "fetch",
		CheckSSL: true,
		Stream:   "mgmt",
	}
}

// Validate if the params passed in are valid data.
func (obj *HttpRes) Validate() error {
	return fmt.Errorf("http: validate: not Implemented")
	// TODO validate method
}

// Init initiates the resource.
func (obj *HttpRes) Init() error {
	obj.BaseRes.kind = "Http"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
// Taken from the File resource.
func (obj *HttpRes) Watch() error {
	client := sse.NewClient(obj.URL)
	client.Subscribe(obj.SseStream, func(msg *sse.Event) {
		obj.Event()
	}
	return nil
}

// CheckApply method for Http resource.
func (obj *HttpRes) CheckApply(apply bool) (bool, error) {
	return false, fmt.Errorf("http: checkapply: not Implemented")
}

// HttpUID is a UID struct for HttpRes.
type HttpNameUID struct {
	BaseUID
	name string
}

type HttpUrlUID struct {
	BaseUID
	url string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *HttpRes) AutoEdges() AutoEdge {
	return nil
}

// UIDs includes all params to make a unique identification of this object.
func (obj *HttpRes) UIDs() []ResUID {
	name := &HttpNameUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		name:    obj.Name,
	}
	url := &HttpUrlUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		url:     obj.URL,
	}
	return []ResUID{name, url}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *HttpRes) GroupCmp(r Res) bool {
	return false // Http commands can not be grouped together.
}

// Compare two resources and return if they are equivalent.
func (obj *HttpRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare HttpRes to others of the same resource
	case *HttpRes:
		res := res.(*HttpRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}
		if obj.Name != res.Name || obj.URL != res.URL {
			return false
		}
	default:
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *HttpRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HttpRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*HttpRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HttpRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HttpRes(raw) // restore from indirection with type conversion!
	return nil
}
