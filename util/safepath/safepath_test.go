// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package safepath

import (
	"fmt"
	"testing"
)

func ExampleJoinToAbsFile() {
	absDir := UnsafeParseIntoAbsDir("/foo/bar/")
	relFile := UnsafeParseIntoRelFile("baz")
	fmt.Println(JoinToAbsFile(absDir, relFile).String())

	// Output: /foo/bar/baz
}

func ExampleJoinToAbsDir() {
	absDir := UnsafeParseIntoAbsDir("/foo/bar/")
	relDir := UnsafeParseIntoRelDir("baz/")
	fmt.Println(JoinToAbsDir(absDir, relDir).String())

	// Output: /foo/bar/baz/
}

func ExampleJoinToRelFile() {
	relDir := UnsafeParseIntoRelDir("foo/bar/")
	relFile := UnsafeParseIntoRelFile("baz")
	fmt.Println(JoinToRelFile(relDir, relFile).String())

	// Output: foo/bar/baz
}

func ExampleJoinToRelDir() {
	relDir1 := UnsafeParseIntoRelDir("foo/")
	relDir2 := UnsafeParseIntoRelDir("bar/")
	relDir3 := UnsafeParseIntoRelDir("baz/")
	fmt.Println(JoinToRelDir(relDir1, relDir2, relDir3).String())

	// Output: foo/bar/baz/
}

func TestAbsFileParse(t *testing.T) {
	tests := []struct {
		AbsFile string
		Expect  error
	}{
		{
			AbsFile: "",
			Expect:  fmt.Errorf("path is empty"),
		},
		{
			AbsFile: "/", // root is an abs dir
			Expect:  fmt.Errorf("path is not a file"),
		},
		{
			AbsFile: "//",
			Expect:  fmt.Errorf("path is not a file"),
		},
		{
			AbsFile: "./",
			Expect:  fmt.Errorf("file is not absolute"),
		},
		{
			AbsFile: ".",
			Expect:  fmt.Errorf("file is not absolute"),
		},
		{
			AbsFile: "/../../",
			Expect:  fmt.Errorf("path is not a file"),
		},
		{
			AbsFile: "../../",
			Expect:  fmt.Errorf("file is not absolute"),
		},
		{
			AbsFile: "/foo/bar/baz",
			Expect:  nil,
		},
		{
			AbsFile: "foo/bar/baz", // this is rel
			Expect:  fmt.Errorf("file is not absolute"),
		},
	}

	for _, x := range tests {
		_, err := ParseIntoAbsFile(x.AbsFile)
		if err == nil && x.Expect == nil {
			continue
		}
		if (err == nil) != (x.Expect == nil) {
			t.Errorf("%s exp: %+v, got: %+v", x.AbsFile, x.Expect, err)
			continue
		}
		if s1, s2 := x.Expect.Error(), err.Error(); s1 != s2 {
			t.Errorf("%s exp: %+v, got: %+v", x.AbsFile, s1, s2)
		}
	}
}

func TestAbsDirParse(t *testing.T) {
	tests := []struct {
		AbsDir string
		Expect error
	}{
		{
			AbsDir: "",
			Expect: fmt.Errorf("path is empty"),
		},
		{
			AbsDir: "/", // root is an abs dir
			Expect: nil,
		},
		{
			AbsDir: "//",
			Expect: nil, // TODO: should this pass?
		},
		{
			AbsDir: "./",
			Expect: fmt.Errorf("dir is not absolute"),
		},
		{
			AbsDir: ".",
			Expect: fmt.Errorf("dir is not absolute"),
		},
		{
			AbsDir: "/../../",
			Expect: nil, // TODO: should this pass?
		},
		{
			AbsDir: "../../",
			Expect: fmt.Errorf("dir is not absolute"),
		},
		{
			AbsDir: "/foo/bar/baz/",
			Expect: nil,
		},
		{
			AbsDir: "/foo/bar/baz", // omitting the trailing slash is ok
			Expect: nil,
		},
		{
			AbsDir: "foo/bar/baz", // this is rel
			Expect: fmt.Errorf("dir is not absolute"),
		},
	}

	for _, x := range tests {
		_, err := ParseIntoAbsDir(x.AbsDir)
		if err == nil && x.Expect == nil {
			continue
		}
		if (err == nil) != (x.Expect == nil) {
			t.Errorf("%s exp: %+v, got: %+v", x.AbsDir, x.Expect, err)
			continue
		}
		if s1, s2 := x.Expect.Error(), err.Error(); s1 != s2 {
			t.Errorf("%s exp: %+v, got: %+v", x.AbsDir, s1, s2)
		}
	}
}

func TestRelFileParse(t *testing.T) {
	tests := []struct {
		RelFile string
		Expect  error
	}{
		//{
		//	RelFile: "",
		//	Expect:  nil, // TODO: should this pass?
		//},
		{
			RelFile: "/", // root is an abs dir
			Expect:  fmt.Errorf("file is not relative"),
		},
		{
			RelFile: "//",
			Expect:  fmt.Errorf("file is not relative"),
		},
		{
			// this is seen as a file named: .
			RelFile: ".",
			Expect:  nil,
		},
		{
			// this is seen as a file named: .
			// remember: the parser removes the trailing slash here
			RelFile: "./",
			Expect:  nil,
		},
		{
			RelFile: "/../../",
			Expect:  fmt.Errorf("file is not relative"),
		},
		{
			// this is seen as a file named: ..
			RelFile: "..",
			Expect:  nil,
		},
		{
			// this is seen as a file named: ..
			// remember: the parser removes the trailing slash here
			RelFile: "../../",
			Expect:  nil,
		},
		{
			// this is seen as a file named: ...
			RelFile: "...",
			Expect:  nil,
		},
		{
			RelFile: "/foo/bar/baz/",
			Expect:  fmt.Errorf("file is not relative"),
		},
		{
			RelFile: "foo/bar/baz",
			Expect:  nil,
		},
	}

	for _, x := range tests {
		_, err := ParseIntoRelFile(x.RelFile)
		if err == nil && x.Expect == nil {
			continue
		}
		if (err == nil) != (x.Expect == nil) {
			t.Errorf("%s exp: %+v, got: %+v", x.RelFile, x.Expect, err)
			continue
		}
		if s1, s2 := x.Expect.Error(), err.Error(); s1 != s2 {
			t.Errorf("%s exp: %+v, got: %+v", x.RelFile, s1, s2)
		}
	}
}

func TestRelDirParse(t *testing.T) {
	tests := []struct {
		RelDir string
		Expect error
	}{
		{
			RelDir: "",
			Expect: fmt.Errorf("path is empty"),
		},
		{
			RelDir: "/", // root is an abs dir
			Expect: fmt.Errorf("dir is not relative"),
		},
		{
			RelDir: "//",
			Expect: fmt.Errorf("dir is not relative"),
		},
		{
			RelDir: "./",
			Expect: nil,
		},
		{
			RelDir: ".", // remember, the parser adds a slash here
			Expect: nil,
		},
		{
			RelDir: "/../../",
			Expect: fmt.Errorf("dir is not relative"),
		},
		{
			RelDir: "../../",
			Expect: nil,
		},
		{
			RelDir: "foo/bar/baz/",
			Expect: nil,
		},
		{
			RelDir: "foo/bar/baz", // omitting the trailing slash is ok
			Expect: nil,
		},
	}

	for _, x := range tests {
		_, err := ParseIntoRelDir(x.RelDir)
		if err == nil && x.Expect == nil {
			continue
		}
		if (err == nil) != (x.Expect == nil) {
			t.Errorf("%s exp: %+v, got: %+v", x.RelDir, x.Expect, err)
			continue
		}
		if s1, s2 := x.Expect.Error(), err.Error(); s1 != s2 {
			t.Errorf("%s exp: %+v, got: %+v", x.RelDir, s1, s2)
		}
	}
}

func TestAbsFileHasDir(t *testing.T) {
	tests := []struct {
		AbsFile string
		Has     string
		Expect  bool
	}{
		//{
		//	AbsFile: "/foo",
		//	Has: "", // not currently permitted
		//	Expect: false,
		//},
		{
			AbsFile: "/foo/bar/baz",
			Has:     "x/",
			Expect:  false,
		},
		{
			AbsFile: "/foo/bar/baz",
			Has:     "bar/",
			Expect:  true,
		},
		{
			AbsFile: "/foo/bar/baz",
			Has:     "bar/baz/", // baz is a file, not a dir!
			Expect:  false,
		},
		{
			AbsFile: "/foo/bar/baz",
			Has:     "foo/bar/", // this one is sneaky for the brain
			Expect:  true,
		},
		{
			AbsFile: "/foo/bar/baz",
			Has:     "foo/bar/b",
			Expect:  false,
		},
	}

	for _, x := range tests {
		absFile := UnsafeParseIntoAbsFile(x.AbsFile)
		has := UnsafeParseIntoRelDir(x.Has)
		if out := absFile.HasDir(has); x.Expect != out {
			t.Errorf("%s HasDir %s exp: %+v, got: %+v", x.AbsFile, x.Has, x.Expect, out)
		}
	}
}

func TestAbsDirHasDir(t *testing.T) {
	tests := []struct {
		AbsDir string
		Has    string
		Expect bool
	}{
		//{
		//	AbsDir: "/",
		//	Has: "", // empty path is not supported, should it?
		//	Expect: false,
		//},
		//{
		//	AbsDir: "/whatever",
		//	Has:    "/", // not a rel dir!
		//	Expect: false,
		//},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "x/",
			Expect: false,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "def/",
			Expect: true,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "ghi/",
			Expect: true,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "def/ghi/",
			Expect: true,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "c/def/ghi/", // c is not a whole dir!
			Expect: false,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "bc/def/ghi/", // bc is not a whole dir!
			Expect: false,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "abc/def/ghi/",
			Expect: true,
		},
		{
			AbsDir: "/abc/def/ghi",
			Has:    "def/gh/",
			Expect: false,
		},
	}

	for _, x := range tests {
		absDir := UnsafeParseIntoAbsDir(x.AbsDir)
		has := UnsafeParseIntoRelDir(x.Has)
		if out := absDir.HasDir(has); x.Expect != out {
			t.Errorf("%s HasDir %s exp: %+v, got: %+v", x.AbsDir, x.Has, x.Expect, out)
		}
	}
}

func TestRelFileHasDir(t *testing.T) {
	tests := []struct {
		RelFile string
		Has     string
		Expect  bool
	}{
		//{
		//	RelFile: "/foo",
		//	Has: "", // not currently permitted
		//	Expect: false,
		//},
		{
			RelFile: "foo",
			Has:     "foo/",
			Expect:  false,
		},
		{
			RelFile: "foo/bar/baz",
			Has:     "x/",
			Expect:  false,
		},
		{
			RelFile: "foo/bar/baz",
			Has:     "bar/",
			Expect:  true,
		},
		{
			RelFile: "foo/bar/baz",
			Has:     "baz/",
			Expect:  false, // baz is a file, not a dir
		},
		{
			RelFile: "foo/bar/baz",
			Has:     "bar/baz/",
			Expect:  false, // baz is a file, not a dir
		},
		{
			RelFile: "foo/bar/baz",
			Has:     "bar/b/",
			Expect:  false,
		},
	}

	for _, x := range tests {
		relFile := UnsafeParseIntoRelFile(x.RelFile)
		has := UnsafeParseIntoRelDir(x.Has)
		if out := relFile.HasDir(has); x.Expect != out {
			t.Errorf("%s HasDir %s exp: %+v, got: %+v", x.RelFile, x.Has, x.Expect, out)
		}
	}
}

func TestRelDirHasDir(t *testing.T) {
	tests := []struct {
		RelDir string
		Has    string
		Expect bool
	}{
		//{
		//	RelDir: "foo/",
		//	Has: "", // not currently permitted
		//	Expect: false,
		//},
		{
			RelDir: "foo/bar/baz/",
			Has:    "x/",
			Expect: false,
		},
		{
			RelDir: "foo/bar/baz/",
			Has:    "bar/",
			Expect: true,
		},
		{
			RelDir: "foo/bar/baz", // omitting the trailing slash is ok
			Has:    "bar/baz/",
			Expect: true,
		},
		{
			RelDir: "foo/bar/baz/",
			Has:    "bar/baz/",
			Expect: true,
		},
		{
			RelDir: "foo/bar/baz/",
			Has:    "bar/ba", // omitting the trailing slash is ok
			Expect: false,
		},
	}

	for _, x := range tests {
		relDir := UnsafeParseIntoRelDir(x.RelDir)
		has := UnsafeParseIntoRelDir(x.Has)
		if out := relDir.HasDir(has); x.Expect != out {
			t.Errorf("%s HasDir %s exp: %+v, got: %+v", x.RelDir, x.Has, x.Expect, out)
		}
	}
}

func TestPathHasPrefix(t *testing.T) {
	tests := []struct {
		Path   string // obey slash conventions
		Prefix string // Dir
		Expect bool
	}{
		//{
		//	Path: "/foo/bar/baz",
		//	Prefix:    "", // not currently permitted
		//	Expect: true,
		//},
		{
			Path:   "/foo/bar/baz/ff", // absfile
			Prefix: "/foo/",
			Expect: true,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "foo/",
			Expect: false, // an abs dir can't have a rel dir prefix
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "foo/bar/",
			Expect: false,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "foo/bar/baz/",
			Expect: false,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "foo/barb",
			Expect: false,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "/foo/",
			Expect: true,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "/foo/bar/",
			Expect: true,
		},
		{
			Path:   "/foo/bar/baz/ff",
			Prefix: "/foo/bar/baz/",
			Expect: true,
		},

		{
			Path:   "/foo/bar/baz/", // absdir
			Prefix: "/foo/",
			Expect: true,
		},
		{
			Path:   "/foo/bar/baz/",
			Prefix: "foo/bar/",
			Expect: false,
		},
		{
			Path:   "foo/bar/baz/",
			Prefix: "foo/bar/baz/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/",
			Prefix: "foo/barb",
			Expect: false,
		},

		{
			Path:   "foo/bar/baz/ff", // relfile
			Prefix: "/foo/",
			Expect: false,
		},
		{
			Path:   "foo/bar/baz/ff",
			Prefix: "foo/bar/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/ff",
			Prefix: "foo/bar/baz/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/ff",
			Prefix: "foo/barb",
			Expect: false,
		},

		{
			Path:   "foo/bar/baz/", // reldir
			Prefix: "foo/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/",
			Prefix: "foo/bar/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/",
			Prefix: "foo/bar/baz/",
			Expect: true,
		},
		{
			Path:   "foo/bar/baz/",
			Prefix: "foo/barb",
			Expect: false,
		},
	}

	for _, x := range tests {
		path := UnsafeSmartParseIntoPath(x.Path)
		prefix := UnsafeParseIntoDir(x.Prefix)
		if out := HasPrefix(path, prefix); x.Expect != out {
			t.Errorf("%s HasPrefix %s exp: %+v, got: %+v", x.Path, x.Prefix, x.Expect, out)
		}
	}
}
