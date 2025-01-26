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

package resources

import (
	"fmt"
	"testing"
)

func TestHttpProxyPathParser0(t *testing.T) {

	type test struct { // an individual test
		fail        bool
		requestPath string
		path        string
		sub         string
		head        string
		cache       string
		proxyURL    string
		cachePath   string
	}
	testCases := []test{}
	testCases = append(testCases, test{ // index: 0
		fail: true, // can't be empty
	})
	testCases = append(testCases, test{
		fail: true, // more fields need to exist
		path: "/",
	})
	testCases = append(testCases, test{
		fail:        true,
		requestPath: "", // can't be empty or not absolute
	})
	testCases = append(testCases, test{
		requestPath: "/fedora/releases/39/Everything/x86_64/os/repodata/repomd.xml",
		path:        "/fedora/releases/39/Everything/x86_64/os/",
		sub:         "/fedora/",
		head:        "https://mirror.example.com/fedora/linux/", // this is the dir with the releases/ folder in it
		cache:       "/tmp/cache/",
		proxyURL:    "https://mirror.example.com/fedora/linux/releases/39/Everything/x86_64/os/repodata/repomd.xml",
		cachePath:   "/tmp/cache/repodata/repomd.xml",
	})
	testCases = append(testCases, test{
		requestPath: "/fedora/releases/39/Everything/x86_64/os/repodata/repomd.xml",
		path:        "/fedora/releases/39/Everything/x86_64/os/",
		sub:         "/fedora/",
		head:        "https://mirror.example.com/fedora/", // this is the dir with the releases/ folder in it
		cache:       "/tmp/cache/",
		proxyURL:    "https://mirror.example.com/fedora/releases/39/Everything/x86_64/os/repodata/repomd.xml",
		cachePath:   "/tmp/cache/repodata/repomd.xml",
	})
	testCases = append(testCases, test{
		fail:        true,
		requestPath: "/fedora/nope/", // not within path!
		path:        "/fedora/releases/whatever/",
		sub:         "/fedora/",
		head:        "https://mirror.example.com/fedora/",
		//cache:       "",
		//proxyURL:    "",
		//cachePath:   "",
	})
	testCases = append(testCases, test{
		fail:        true,
		requestPath: "/fedora/releases/39/Everything/x86_64/os/../repodata/repomd.xml",
		path:        "/fedora/releases/39/Everything/x86_64/os/",
		sub:         "/fedora/",
		head:        "https://mirror.example.com/fedora/", // this is the dir with the releases/ folder in it
		cache:       "/tmp/cache/",
		proxyURL:    "https://mirror.example.com/fedora/releases/39/Everything/x86_64/os/repodata/repomd.xml",
		cachePath:   "/tmp/repodata/repomd.xml", // fail b/c ../ path
	})

	for index, tc := range testCases { // run all the tests
		t.Run(fmt.Sprintf("test #%d", index), func(t *testing.T) {
			fail, requestPath, path, sub, head, cache, proxyURL, cachePath := tc.fail, tc.requestPath, tc.path, tc.sub, tc.head, tc.cache, tc.proxyURL, tc.cachePath

			pp := &pathParser{
				path:  path,
				sub:   sub,
				head:  head, // mirror
				cache: cache,
			}
			result, err := pp.parse(requestPath)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: parse failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: parse expected error, not nil", index)
				t.Logf("test #%d: result: %+v", index, result)
				return
			}
			if fail { // we failed as expected, don't continue...
				return
			}

			// if head is empty, the proxyURL result isn't relevant
			if head != "" && proxyURL != result.proxyURL {
				t.Errorf("test #%d: unexpected value for: `proxyURL`", index)
				t.Logf("test #%d:  input.path: %s", index, pp.path)
				t.Logf("test #%d:   input.sub: %s", index, pp.sub)
				t.Logf("test #%d:  input.head: %s", index, pp.head)
				t.Logf("test #%d: input.cache: %s", index, pp.cache)
				t.Logf("test #%d: requestPath: %s", index, requestPath)
				t.Logf("test #%d:    proxyURL: %s", index, proxyURL)
				t.Logf("test #%d:      result: %s", index, result.proxyURL)
				//return
			}

			// if cache is empty, the cachePath result isn't relevant
			if cache != "" && cachePath != result.cachePath {
				t.Errorf("test #%d: unexpected value for: `cachePath`", index)
				t.Logf("test #%d:  input.path: %s", index, pp.path)
				t.Logf("test #%d:   input.sub: %s", index, pp.sub)
				t.Logf("test #%d:  input.head: %s", index, pp.head)
				t.Logf("test #%d: input.cache: %s", index, pp.cache)
				t.Logf("test #%d: requestPath: %s", index, requestPath)
				t.Logf("test #%d:   cachePath: %s", index, cachePath)
				t.Logf("test #%d:      result: %s", index, result.cachePath)
				//return
			}
		})
	}
}
