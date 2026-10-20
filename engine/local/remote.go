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
	"strings"
	"unicode/utf8"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/go-json-experiment/json"
)

const (
	// RemoteSignalNone asks the parent to send the child nothing once it
	// has recorded that the child is done, and to wait for it to exit by
	// itself.
	RemoteSignalNone = "none"

	// RemoteSignalExit asks the parent to send the child a ^C once it has
	// recorded that the child is done.
	RemoteSignalExit = "exit"

	// RemoteSignalSoft asks the parent to send the child a soft interrupt,
	// which is the equivalent of a second ^C.
	RemoteSignalSoft = "soft"

	// RemoteSignalHard asks the parent to send the child a hard interrupt,
	// which is the equivalent of a third ^C.
	RemoteSignalHard = "hard"
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

// RemoteDone is the record which a child stores for the remote resource which
// started it, to say that it's done. Only the parent which pushed this same
// binary to the child ever reads it, so it has the same strict encoding as the
// Remote struct.
type RemoteDone struct {
	// Signal is what the parent should send us after it has recorded that
	// we're done. It's one of the RemoteSignal constants. It's never empty
	// here, since the default is resolved before it is stored.
	Signal string `json:"signal"`

	// Return is the bool that the parent's CheckApply returns if we
	// succeeded, so true means that we didn't change anything, and false
	// means that we did. If it's nil, then the parent decides. It must not
	// be true if Error is set.
	Return *bool `json:"return"`

	// Error is empty if we succeeded. Otherwise the parent fails with this
	// message after we have shut down.
	Error string `json:"error"`
}

// Validate checks that the done record is sane.
func (obj *RemoteDone) Validate() error {
	if obj == nil {
		return fmt.Errorf("done record is nil")
	}
	switch obj.Signal {
	case RemoteSignalNone, RemoteSignalExit, RemoteSignalSoft, RemoteSignalHard:
	default:
		return fmt.Errorf("invalid signal: %s", obj.Signal)
	}
	if obj.Error != "" && obj.Return != nil && *obj.Return {
		return fmt.Errorf("the return must not be true when error is set")
	}
	if !utf8.ValidString(obj.Error) { // json can't encode it otherwise!
		return fmt.Errorf("the error is not valid UTF-8")
	}
	return nil
}

// String returns a human readable summary of the done record for the logs. The
// error is last, since it's arbitrary text which might contain anything.
func (obj *RemoteDone) String() string {
	s := "success"
	if obj.Error != "" {
		s = "failure"
	}
	if obj.Return != nil {
		s += fmt.Sprintf(", return: %t", *obj.Return)
	}
	s += fmt.Sprintf(", signal: %s", obj.Signal)
	if obj.Error != "" {
		s += fmt.Sprintf(", error: %s", obj.Error)
	}
	return s
}

// Encode returns the canonical JSON encoding of this struct.
func (obj *RemoteDone) Encode() (string, error) {
	if err := obj.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(obj, remoteJSONOptions)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeRemoteDone decodes the output of RemoteDone.Encode. Just like with
// DecodeRemote, it rejects anything which isn't in canonical form.
func DecodeRemoteDone(s string) (*RemoteDone, error) {
	obj := &RemoteDone{}
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

// RemoteAck is the record which a parent stores to acknowledge that its child
// is done. It persists until it is cleared, which means that a newer binary
// might read one that an older binary wrote. As a result, its decoding doesn't
// require the canonical form, so that any member which a newer version adds is
// simply the zero value when it's missing. Unknown members are still rejected,
// so an older binary errors on a record that it doesn't understand, instead of
// silently misreading it.
type RemoteAck struct {
	// Child is the hostname of the child which told us that it was done.
	Child string `json:"child"`

	// Error is the recorded result. It's empty if the child succeeded.
	Error string `json:"error"`
}

// Validate checks that the ack record is sane.
func (obj *RemoteAck) Validate() error {
	if obj == nil {
		return fmt.Errorf("ack record is nil")
	}
	if err := util.ValidHostname(obj.Child); err != nil {
		return errwrap.Wrapf(err, "invalid child hostname")
	}
	if !utf8.ValidString(obj.Error) { // json can't encode it otherwise!
		return fmt.Errorf("the error is not valid UTF-8")
	}
	return nil
}

// Encode returns the JSON encoding of this struct.
func (obj *RemoteAck) Encode() (string, error) {
	if err := obj.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(obj, remoteJSONOptions)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeRemoteAck decodes an ack record. See the RemoteAck struct for why this
// is more tolerant than the other decoders.
func DecodeRemoteAck(s string) (*RemoteAck, error) {
	// A JSON null would otherwise decode as an empty, successful record. We
	// only ever write an object, with no leading whitespace.
	if !strings.HasPrefix(s, "{") {
		return nil, fmt.Errorf("not a JSON object")
	}
	obj := &RemoteAck{}
	if err := json.Unmarshal([]byte(s), obj, remoteJSONOptions); err != nil {
		return nil, err
	}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}
