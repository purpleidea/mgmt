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
	"testing"
)

func TestRemoteEncode(t *testing.T) {
	remote := &Remote{
		Parent:    "h1",
		Name:      "ssh://root@h2:22/",
		Transient: true,
	}
	s, err := remote.Encode()
	if err != nil {
		t.Fatalf("func Encode: %v", err)
	}
	if exp := `{"parent":"h1","name":"ssh://root@h2:22/","transient":true}`; s != exp {
		t.Errorf("func Encode: got: %s, expected: %s", s, exp)
	}

	decoded, err := DecodeRemote(s)
	if err != nil {
		t.Fatalf("func DecodeRemote: %v", err)
	}
	if *decoded != *remote {
		t.Errorf("func DecodeRemote: got: %+v, expected: %+v", decoded, remote)
	}

	if _, err := (&Remote{}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an empty parent")
	}
	if _, err := (&Remote{Parent: "bad host", Name: "r0"}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an invalid parent")
	}
	if _, err := (&Remote{Parent: "h1"}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an empty name")
	}
}

func TestRemoteDecodeInvalid(t *testing.T) {
	// Each of these differs from a valid encoding in one way only.
	invalid := []string{
		``,
		`{}`,
		`null`,
		`{"parent":null,"name":"r0","transient":true}`,
		`{"parent":"","name":"r0","transient":true}`,
		`{"parent":"bad host","name":"r0","transient":true}`,
		`{"parent":"h1","name":"","transient":true}`,
		`{"parent":"h1","name":"r0"}`,                                  // missing member
		`{"parent":"h1","transient":true}`,                             // missing member
		`{"parent":"h1","name":"r0","transient":"true"}`,               // wrong type
		`{"parent":"h1","name":"r0","transient":true,"evil":1}`,        // unknown member
		`{"PARENT":"h1","name":"r0","transient":true}`,                 // case mismatch
		`{"parent":"h1","parent":"h2","name":"r0","transient":true}`,   // duplicate member
		"{\"parent\":\"h\xff1\",\"name\":\"r0\",\"transient\":true}",   // invalid UTF-8
		`{"parent":"h1","name":"r0","transient":true} {}`,              // trailing data
		` {"parent":"h1","name":"r0","transient":true}`,                // whitespace
		`{"parent":"h1","name":"r0","transient":true} `,                // whitespace
		`{"parent": "h1","name":"r0","transient":true}`,                // whitespace
		`{"name":"r0","parent":"h1","transient":true}`,                 // order
		"{\"\\u0070arent\":\"h1\",\"name\":\"r0\",\"transient\":true}", // escaped member name
		"{\"parent\":\"h\\u0031\",\"name\":\"r0\",\"transient\":true}", // escaped value
		`{"parent":1,"name":"r0","transient":true}`,
		`{"parent":["h1"],"name":"r0","transient":true}`,
		`["h1"]`,
		`"h1"`,
		`h1`,
	}
	for _, s := range invalid {
		if remote, err := DecodeRemote(s); err == nil {
			t.Errorf("func DecodeRemote(%q): expected error, got: %+v", s, remote)
		}
	}
}

func TestRemoteDoneEncode(t *testing.T) {
	done := &RemoteDone{
		Signal: RemoteSignalNone,
		Error:  "missing /etc/myapp.conf",
	}
	s, err := done.Encode()
	if err != nil {
		t.Fatalf("func Encode: %v", err)
	}
	if exp := `{"signal":"none","error":"missing /etc/myapp.conf"}`; s != exp {
		t.Errorf("func Encode: got: %s, expected: %s", s, exp)
	}

	decoded, err := DecodeRemoteDone(s)
	if err != nil {
		t.Fatalf("func DecodeRemoteDone: %v", err)
	}
	if *decoded != *done {
		t.Errorf("func DecodeRemoteDone: got: %+v, expected: %+v", decoded, done)
	}

	if _, err := (&RemoteDone{}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an empty signal")
	}
	if _, err := (&RemoteDone{Signal: "kill"}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an invalid signal")
	}
	if _, err := (&RemoteDone{Signal: RemoteSignalExit, Error: "\xff"}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an invalid error")
	}
}

func TestRemoteDoneString(t *testing.T) {
	tests := []struct {
		done *RemoteDone
		exp  string
	}{
		{
			done: &RemoteDone{Signal: RemoteSignalExit},
			exp:  "success, signal: exit",
		},
		{
			done: &RemoteDone{Signal: RemoteSignalExit, Error: "missing /etc/myapp.conf"},
			exp:  "failure, signal: exit, error: missing /etc/myapp.conf",
		},
		{
			done: &RemoteDone{Signal: RemoteSignalSoft, Error: "oops, signal: hard"},
			exp:  "failure, signal: soft, error: oops, signal: hard",
		},
	}
	for _, tc := range tests {
		if s := tc.done.String(); s != tc.exp {
			t.Errorf("func String: got: %s, expected: %s", s, tc.exp)
		}
	}
}

func TestRemoteDoneDecodeInvalid(t *testing.T) {
	// Each of these differs from a valid encoding in one way only.
	invalid := []string{
		``,
		`{}`,
		`null`,
		`{"signal":"","error":""}`,
		`{"signal":"kill","error":""}`,
		`{"signal":"exit"}`,                     // missing member
		`{"signal":"exit","error":"","evil":1}`, // unknown member
		`{"error":"","signal":"exit"}`,          // order
		`{"signal": "exit","error":""}`,         // whitespace
		`{"signal":"exit","error":null}`,        // null
		`{"signal":"exit","error":""} {}`,       // trailing data
		"{\"signal\":\"exit\",\"error\":\"\xff\"}", // invalid UTF-8
	}
	for _, s := range invalid {
		if done, err := DecodeRemoteDone(s); err == nil {
			t.Errorf("func DecodeRemoteDone(%q): expected error, got: %+v", s, done)
		}
	}
}

func TestRemoteAckDecode(t *testing.T) {
	ack := &RemoteAck{
		Error: "oops",
	}
	s, err := ack.Encode()
	if err != nil {
		t.Fatalf("func Encode: %v", err)
	}
	if exp := `{"error":"oops"}`; s != exp {
		t.Errorf("func Encode: got: %s, expected: %s", s, exp)
	}

	// The ack decoding is tolerant of missing members, since an older
	// binary might have written it.
	valid := map[string]string{
		`{"error":"oops"}`:  "oops",
		`{"error":""}`:      "",
		`{}`:                "",
		`{ "error":"oops"}`: "oops",
	}
	for s, exp := range valid {
		decoded, err := DecodeRemoteAck(s)
		if err != nil {
			t.Errorf("func DecodeRemoteAck(%q): %v", s, err)
			continue
		}
		if decoded.Error != exp {
			t.Errorf("func DecodeRemoteAck(%q): got: %q, expected: %q", s, decoded.Error, exp)
		}
	}

	invalid := []string{
		``,
		`null`,
		` {"error":""}`,
		`"oops"`,
		`{"error":"","evil":1}`, // unknown member, eg: from a newer binary
		`{"error":1}`,
		`{"error":""} {}`,
		"{\"error\":\"\xff\"}",
	}
	for _, s := range invalid {
		if ack, err := DecodeRemoteAck(s); err == nil {
			t.Errorf("func DecodeRemoteAck(%q): expected error, got: %+v", s, ack)
		}
	}
}
