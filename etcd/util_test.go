// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// +build !root

package etcd

import (
	"net/url"
	"testing"
)

func TestCopyURL0(t *testing.T) {
	// list of urls to test
	strs := []string{
		"",
		"http://192.168.13.42:2379",
		"https://192.168.13.42:2380",
		"http://192.168.13.42",
		"https://192.168.13.42",
	}
	for _, str := range strs {
		t.Logf("testing: `%s`", str)
		u1, err := url.Parse(str)
		if err != nil {
			t.Errorf("url did not parse: %+v", err)
			continue
		}

		u2, err := copyURL(u1)
		if err != nil {
			t.Errorf("url did not copy: %+v", err)
			continue
		}

		if s := u2.String(); s != str {
			t.Errorf("url did not cmp, got: `%s`, expected: `%s`", s, str)
		}

		// bonus test (add to separate lists of size one)
		if err := cmpURLs([]url.URL{*u1}, []url.URL{*u2}); err != nil {
			t.Errorf("urls did not cmp, err: %+v", err)
		}
	}
}

func TestCopyURLs0(t *testing.T) {
	// list of urls lists to test
	nstrs := [][]string{
		{}, // empty!
		{
			"http://192.168.13.42:2379",
			"https://192.168.13.42:2380",
			"http://192.168.13.42",
			"https://192.168.13.42",
		},
		{
			"http://192.168.42.42:2379",
			"https://192.168.13.42:2380",
			"http://192.168.99.42",
			"https://10.10.1.255",
		},
		{
			"http://example.com:2379",
			"https://purpleidea.com/:2379",
			"http://192.168.13.42",
			"https://192.168.13.42",
		},
	}
	for _, strs := range nstrs {
		t.Logf("testing: `%s`", strs)

		urls1 := []url.URL{}
		for _, str := range strs {
			u, err := url.Parse(str)
			if err != nil {
				t.Errorf("url did not parse: %+v", err)
				continue
			}
			urls1 = append(urls1, *u)
		}

		urls2, err := copyURLs(urls1)
		if err != nil {
			t.Errorf("urls did not copy: %+v", err)
			continue
		}

		if err := cmpURLs(urls1, urls2); err != nil {
			t.Errorf("urls did not cmp, err: %+v", err)
		}
	}
}

func TestCopyURLsMap0(t *testing.T) {
	// list of urls lists to test
	nmstrs := []map[string][]string{
		{}, // empty!
		{
			"h1": []string{}, // empty
			"h2": []string{}, // empty
			"h3": []string{}, // empty
		},
		{
			"h1": []string{}, // empty
			"h2": nil,        // nil !
			"h3": []string{}, // empty
		},
		{
			"h1": []string{}, // empty
			"h2": []string{
				"http://example.com:2379",
				"https://purpleidea.com/:2379",
				"http://192.168.13.42",
				"https://192.168.13.42",
			},
		},
		{
			"h1": []string{
				"http://192.168.13.42:2379",
				"https://192.168.13.42:2380",
				"http://192.168.13.42",
				"https://192.168.13.42",
			},
			"h2": []string{
				"http://example.com:2379",
				"https://purpleidea.com/:2379",
				"http://192.168.13.42",
				"https://192.168.13.42",
			},
		},
		{
			"h1": []string{
				"http://192.168.13.42:2379",
				"https://192.168.13.42:2380",
				"http://192.168.13.42",
				"https://192.168.13.42",
			},
			"h2": nil, // nil !
			"h3": []string{
				"http://example.com:2379",
				"https://purpleidea.com/:2379",
				"http://192.168.13.42",
				"https://192.168.13.42",
			},
		},
	}

	for _, mstrs := range nmstrs {
		t.Logf("testing: `%s`", mstrs)
		urlsMap1 := newURLsMap()
		for key, strs := range mstrs {
			urls := []url.URL{}
			for _, str := range strs {
				u, err := url.Parse(str)
				if err != nil {
					t.Errorf("url did not parse: %+v", err)
					continue
				}
				urls = append(urls, *u)
			}
			urlsMap1[key] = urls
		}

		urlsMap2, err := copyURLsMap(urlsMap1)
		if err != nil {
			t.Errorf("urlsMap did not copy: %+v", err)
			continue
		}

		if err := cmpURLsMap(urlsMap1, urlsMap2); err != nil {
			t.Errorf("urlsMap did not cmp, err: %+v", err)
		}
	}
}
