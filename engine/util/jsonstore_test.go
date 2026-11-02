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

//go:build !root

package util

import (
	"testing"
)

func TestParseJSONStore(t *testing.T) {
	tests := []struct {
		name     string
		handle   string
		scheme   string
		location string
		str      string
		fail     bool
	}{
		{
			name:     "etcd",
			handle:   "etcd:///foo/hosts",
			scheme:   JSONStoreSchemeEtcd,
			location: "foo/hosts",
			str:      "etcd:///foo/hosts",
		},
		{
			name:     "absolute file path",
			handle:   "/tmp/store.json",
			scheme:   JSONStoreSchemeFile,
			location: "/tmp/store.json",
			str:      "file:///tmp/store.json",
		},
		{
			name:     "file URI",
			handle:   "file:///tmp/store.json",
			scheme:   JSONStoreSchemeFile,
			location: "/tmp/store.json",
			str:      "file:///tmp/store.json",
		},
		{
			name:     "absolute file path with special characters",
			handle:   "/tmp/a #b%c?.json",
			scheme:   JSONStoreSchemeFile,
			location: "/tmp/a #b%c?.json",
			str:      "file:///tmp/a%20%23b%25c%3F.json",
		},
		{
			name:     "escaped file URI",
			handle:   "file:///tmp/a%23b.json",
			scheme:   JSONStoreSchemeFile,
			location: "/tmp/a#b.json",
			str:      "file:///tmp/a%23b.json",
		},
		{
			name:     "etcd key with leading slash",
			handle:   "etcd:////foo",
			scheme:   JSONStoreSchemeEtcd,
			location: "/foo",
			str:      "etcd:////foo",
		},
		{
			name:   "relative path",
			handle: "tmp/store.json",
			fail:   true,
		},
		{
			name:   "empty",
			handle: "",
			fail:   true,
		},
		{
			name:   "empty etcd key",
			handle: "etcd://",
			fail:   true,
		},
		{
			name:   "etcd query",
			handle: "etcd://?key=foo%2Fhosts",
			fail:   true,
		},
		{
			name:   "unknown scheme",
			handle: "http:///foo",
			fail:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := ParseJSONStore(test.handle)
			if test.fail {
				if err == nil {
					t.Fatalf("func ParseJSONStore unexpectedly succeeded: %#v", store)
				}
				return
			}
			if err != nil {
				t.Fatalf("func ParseJSONStore: %v", err)
			}
			if store.Scheme != test.scheme {
				t.Errorf("unexpected scheme: got %q, want %q", store.Scheme, test.scheme)
			}
			if store.Location != test.location {
				t.Errorf("unexpected location: got %q, want %q", store.Location, test.location)
			}
			s := store.String()
			if s != test.str {
				t.Errorf("unexpected string: got %q, want %q", s, test.str)
			}

			again, err := ParseJSONStore(s)
			if err != nil {
				t.Fatalf("func ParseJSONStore on %q: %v", s, err)
			}
			if *again != *store {
				t.Errorf("store changed in a round trip: got %#v, want %#v", again, store)
			}
		})
	}
}

func TestDecodeJSONObject(t *testing.T) {
	for _, data := range []string{"", "[]", "1", "null", "{} {}", "{"} {
		if root, err := DecodeJSONObject([]byte(data)); err == nil {
			t.Errorf("func DecodeJSONObject unexpectedly succeeded on %q: %#v", data, root)
		}
	}

	root, err := DecodeJSONObject([]byte("{\"b\": 1.50, \"a\": {}}\n"))
	if err != nil {
		t.Fatalf("func DecodeJSONObject: %v", err)
	}
	s, err := EncodeJSONObject(root)
	if err != nil {
		t.Fatalf("func EncodeJSONObject: %v", err)
	}
	if want := "{\n\t\"a\": {},\n\t\"b\": 1.50\n}\n"; s != want {
		t.Errorf("unexpected encoding: got %q, want %q", s, want)
	}
}
