// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package util

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
)

// copyURL copies a URL.
// TODO: submit this upstream to etcd ?
func copyURL(u *url.URL) (*url.URL, error) {
	if u == nil {
		return nil, fmt.Errorf("empty URL specified")
	}
	return url.Parse(u.String()) // copy it
}

// CopyURLs copies a URLs.
// TODO: submit this upstream to etcd ?
func CopyURLs(urls etcdtypes.URLs) (etcdtypes.URLs, error) {
	out := []url.URL{}
	for _, x := range urls {
		u, err := copyURL(&x)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, nil
}

// CopyURLsMap copies a URLsMap.
// TODO: submit this upstream to etcd ?
func CopyURLsMap(urlsMap etcdtypes.URLsMap) (etcdtypes.URLsMap, error) {
	out := make(etcdtypes.URLsMap)
	for k, v := range urlsMap {
		urls, err := CopyURLs(v)
		if err != nil {
			return nil, err
		}
		out[k] = urls
	}
	return out, nil
}

// cmpURLs compares two URLs, and returns nil if they are the same.
func cmpURLs(u1, u2 etcdtypes.URLs) error {
	if (u1 == nil) != (u2 == nil) { // xor
		return fmt.Errorf("lists differ")
	}
	if len(u1) != len(u2) {
		return fmt.Errorf("length of lists is not the same")
	}

	for i, v1 := range u1 {
		if v1 != u2[i] {
			return fmt.Errorf("index %d differs", i)
		}
	}

	return nil
}

// FromStringListToURLs takes a list of string urls and converts them into the
// native type.
func FromStringListToURLs(surls []string) (etcdtypes.URLs, error) {
	result := []url.URL{}
	for _, s := range surls { // flatten map
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		result = append(result, *u)
	}

	return result, nil
}

// CmpURLsMap compares two URLsMap's, and returns nil if they are the same.
func CmpURLsMap(m1, m2 etcdtypes.URLsMap) error {
	if (m1 == nil) != (m2 == nil) { // xor
		return fmt.Errorf("maps differ")
	}
	if len(m1) != len(m2) {
		return fmt.Errorf("length of maps is not the same")
	}

	for k, v1 := range m1 {
		v2, exists := m2[k]
		if !exists {
			return fmt.Errorf("key `%s` not found in map 2", k)
		}
		if err := cmpURLs(v1, v2); err != nil {
			return errwrap.Wrapf(err, "values at key `%s` differ", k)
		}
	}

	return nil
}

// newURLsMap is a helper to build a new URLsMap without having to import the
// messy etcdtypes package.
func newURLsMap() etcdtypes.URLsMap {
	return make(etcdtypes.URLsMap)
}

// FromURLsToStringList turns a list of etcd URLs into a list of strings using
// the full URL scheme.
func FromURLsToStringList(urls etcdtypes.URLs) []string {
	result := []string{}
	for _, u := range urls { // flatten map
		result = append(result, u.String()) // use full url including scheme
	}
	return result
}

// FromURLsMapToStringList flattens a map of URLs into a single string list.
// Remember to sort the result if you want it to be deterministic!
func FromURLsMapToStringList(m etcdtypes.URLsMap) []string {
	result := []string{}
	for _, x := range m { // flatten map
		for _, u := range x {
			result = append(result, u.String()) // use full url including scheme
		}
	}
	return result
}

// validateURLsMap checks if each embedded URL is parseable correctly.
//func validateURLsMap(urlsMap etcdtypes.URLsMap) error {
//	_, err := CopyURLsMap(urlsMap) // would fail if anything didn't parse
//	return err
//}

// LocalhostURLs returns the most localhost like URLs for direct connection.
// This gets clients to talk to the local servers first before looking remotely.
// TODO: improve this algorithm as it's currently a bad heuristic
func LocalhostURLs(urls etcdtypes.URLs) etcdtypes.URLs {
	out := etcdtypes.URLs{}
	for _, u := range urls {
		// "localhost" or anything in 127.0.0.0/8 is valid!
		if strings.HasPrefix(u.Host, "localhost") || strings.HasPrefix(u.Host, "127.") {
			out = append(out, u)
			continue
		}
		// or ipv6 localhost
		// TODO: are there others to add here?
		if strings.HasPrefix(u.Host, "[::1]") {
			out = append(out, u)
			continue
		}
		// or local unix domain sockets
		if u.Scheme == "unix" {
			out = append(out, u)
			continue
		}
	}
	return out
}

//func urlRemoveScheme(urls etcdtypes.URLs) []string {
//	strs := []string{}
//	for _, u := range urls {
//		strs = append(strs, u.Host) // remove http:// prefix
//	}
//	return strs
//}
