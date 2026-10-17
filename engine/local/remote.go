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

package local

import (
	"fmt"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/go-json-experiment/json"
)

// remoteJSONOptions are used to both encode and decode the Remote struct, so
// that the two sides can't drift apart. The v2 json package is already strict
// by default: it rejects duplicate member names, invalid UTF-8, and trailing
// data, and it matches member names case sensitively. Unknown members are the
// one thing it would otherwise silently ignore, so we reject those too.
var remoteJSONOptions = json.JoinOptions(
	json.RejectUnknownMembers(true),
)

// Remote is the information that a remote resource passes to each child that it
// starts. It's encoded as JSON so that new fields can be added here without
// needing to plumb a new command line flag through for each one. Since the
// parent pushes the same binary that it's running, both sides should always
// agree on the shape of this struct.
type Remote struct {
	// Parent is the unique hostname of the parent which started us.
	Parent string `json:"parent"`

	// Name is the name of the remote resource in the parent which started
	// us. Together with Parent, it identifies that resource uniquely.
	Name string `json:"name"`

	// Transient is true if the remote resource which started us is
	// transient, which means that it waits for us to exit.
	Transient bool `json:"transient"`
}

// Validate checks that the remote information is sane.
func (obj *Remote) Validate() error {
	if obj == nil {
		return fmt.Errorf("remote info is nil")
	}
	if err := util.ValidHostname(obj.Parent); err != nil {
		return errwrap.Wrapf(err, "invalid parent hostname: %s", obj.Parent)
	}
	if obj.Name == "" {
		return fmt.Errorf("empty resource name")
	}
	return nil
}

// Encode returns the canonical JSON encoding of this struct. It validates first
// so that a bad value is caught here, instead of by a child on a remote host.
func (obj *Remote) Encode() (string, error) {
	if err := obj.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(obj, remoteJSONOptions)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeRemote decodes the output of Remote.Encode. Since that output always
// comes from the same binary, there's exactly one correct encoding for any one
// value, so after decoding, we encode again and reject the input unless it's
// byte for byte identical. This catches anything the parser would otherwise
// accept, such as an explicit null, extra whitespace, or a member name written
// with escapes, along with any future bugs in how the string was transported.
func DecodeRemote(s string) (*Remote, error) {
	obj := &Remote{}
	if err := json.Unmarshal([]byte(s), obj, remoteJSONOptions); err != nil {
		return nil, err
	}
	b, err := json.Marshal(obj, remoteJSONOptions)
	if err != nil {
		return nil, err
	}
	if string(b) != s {
		return nil, fmt.Errorf("not in canonical form, expected: %s", b)
	}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}
