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

package interfaces

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
)

func TestMetadataParse0(t *testing.T) {
	type test struct { // an individual test
		name string
		yaml string
		fail bool
		meta *Metadata
	}
	testCases := []test{}

	//{
	//	testCases = append(testCases, test{
	//		"",
	//		``,
	//		false,
	//		nil,
	//	})
	//}
	{
		testCases = append(testCases, test{
			name: "empty",
			yaml: ``,
			fail: false,
			meta: DefaultMetadata(),
		})
	}
	{
		testCases = append(testCases, test{
			name: "empty file defaults",
			yaml: util.Code(`
			# empty file
			`),
			fail: false,
			meta: DefaultMetadata(),
		})
	}
	{
		testCases = append(testCases, test{
			name: "empty document defaults",
			yaml: util.Code(`
			--- # new document
			`),
			fail: false,
			meta: DefaultMetadata(),
		})
	}
	{
		testCases = append(testCases, test{
			name: "set values",
			yaml: util.Code(`
			main: "hello.mcl"
			files: "xfiles/"
			path: "vendor/"
			`),
			fail: false,
			meta: &Metadata{
				Main:  "hello.mcl",
				Files: "xfiles/",
				Path:  "vendor/",
			},
		})
	}
	{
		meta := DefaultMetadata()
		meta.Main = "start.mcl"
		testCases = append(testCases, test{
			name: "partial document defaults",
			yaml: util.Code(`
			main: "start.mcl"
			`),
			fail: false,
			meta: meta,
		})
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if (index != 20 && index != 21) {
		//if tc.name != "nil" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, yaml, fail, meta := tc.name, tc.yaml, tc.fail, tc.meta

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			str := strings.NewReader(yaml)
			metadata, err := ParseMetadata(str)
			meta.bug395 = true // workaround for https://github.com/go-yaml/yaml/issues/395

			if !fail && err != nil {
				t.Errorf("test #%d: metadata parse failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: metadata parse passed, expected fail", index)
				return
			}
			if !fail && metadata == nil {
				t.Errorf("test #%d: metadata parse output was nil", index)
				return
			}

			if metadata == nil {
				return
			}
			if reflect.DeepEqual(meta, metadata) {
				return
			}
			// double check because DeepEqual is different since the func exists
			diff := pretty.Compare(meta, metadata)
			if diff == "" { // bonus
				return
			}
			t.Errorf("test #%d: metadata did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n\n%s\n", index, spew.Sdump(meta))
			t.Logf("test #%d: expected: \n\n%s", index, spew.Sdump(metadata))

			// more details, for tricky cases:
			diffable := &pretty.Config{
				Diffable:          true,
				IncludeUnexported: true,
				//PrintStringers: false,
				//PrintTextMarshalers: false,
				//SkipZeroFields: false,
			}
			t.Logf("test #%d:   actual: \n\n%s\n", index, diffable.Sprint(meta))
			t.Logf("test #%d: expected: \n\n%s", index, diffable.Sprint(metadata))
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
	}
}

func TestMetadataSave0(t *testing.T) {
	// Since we put local path information into metadataPath, we'd like to
	// test that we don't leak it into our remote filesystem. This isn't a
	// major issue, but it's not technically nice to tell anyone about it.
	sentinel := "nope!"
	md := &Metadata{
		Main:         "hello.mcl",
		metadataPath: sentinel, // this value should not get seen
	}
	b, err := md.ToBytes()
	if err != nil {
		t.Errorf("can't print metadata file: %+v", err)
		return
	}
	s := string(b)                     // convert
	if strings.Contains(s, sentinel) { // did we find the sentinel?
		t.Errorf("sentinel was found")
	}
	t.Logf("got:\n%s", s)
}
