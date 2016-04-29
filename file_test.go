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

package main

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestGetResReturnsCorrectType(t *testing.T) {
	v := NewFileRes("", "", "", "", "", "").Kind()
	expected := "File"
	if v != expected {
		t.Error("Expected '", expected, "', got: ", v)
	}
}

var getpathtests = []struct {
	fr       FileRes
	expected string
}{
	{FileRes{Path: "/a/b/c"}, "/a/b/c"},                                                      // Dirname and Basename fields are empty
	{FileRes{Path: "/a/b/c", Basename: "basename"}, "/a/b/basename"},                         // Dirname is empty and Basename is valid
	{FileRes{Path: "/a/b/c", Dirname: "dirname/"}, "dirname/c"},                              // Dirname is valid and Basename is empty
	{FileRes{Path: "/a/b/c", Dirname: "dirname/", Basename: "basename"}, "dirname/basename"}, // Dirname and Basename are valid
	{FileRes{Path: "/a/b/c", Dirname: "dirname", Basename: "basename"}, "/a/b/c"},            // Dirname is invalid and Basename is valid
	{FileRes{Path: "/a/b/c", Dirname: "dirname/", Basename: "/basename"}, "/a/b/c"},          // Dirname is valid and Basename is invalid

}

func TestGetPath(t *testing.T) {
	for _, testcase := range getpathtests {
		if testcase.fr.GetPath() != testcase.expected {
			t.Error("Expected '", testcase.expected, "', got: ", testcase.fr.GetPath())
		}
	}
}

var validatetests = []struct {
	fr       FileRes
	expected bool
}{
	{FileRes{Dirname: "dirname/", Basename: "basename"}, true},   // Dirname and Basename are valid
	{FileRes{Dirname: "dirname", Basename: "basename"}, false},   // Dirname is invalid
	{FileRes{Dirname: "dirname/", Basename: "/basename"}, false}, // Basename is invalid
}

func TestValidate(t *testing.T) {
	for _, testcase := range validatetests {
		if testcase.fr.Validate() != testcase.expected {
			t.Error("Expected '", testcase.expected, "', got: ", testcase.fr.Validate())
		}
	}
}

func TestHashSHA256fromContent(t *testing.T) {
	sha256 := (&FileRes{Content: "test content"}).HashSHA256fromContent()
	expected := "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72"
	if sha256 != expected {
		t.Error("Expected '", expected, "', got: '", sha256, "'")
	}
}

func TestFileHashSHA256CheckContentUnchanged(t *testing.T) {
	unchanged, errors := (&FileRes{Path: "./test_resources/file_content_test.txt", Content: "test content"}).FileHashSHA256Check()
	if !unchanged || errors != nil {
		t.Error("Expected no changes in content and no errors, got: content changed: '", unchanged, "', error: '", errors, "'")
	}
}

func TestFileHashSHA256CheckContentChanged(t *testing.T) {
	changed, errors := (&FileRes{Path: "./test_resources/file_content_test.txt", Content: "test changed"}).FileHashSHA256Check()
	if changed || errors != nil {
		t.Error("Expected changed content and no errors, got: content changed: '", changed, "', error: '", errors, "'")
	}
}

func TestFileHashSHA256CheckInvalidPath(t *testing.T) {
	ok, errors := (&FileRes{Path: "./test_resources/non_existent.txt", Content: "test content"}).FileHashSHA256Check()
	if errors != nil {
		t.Error("Shouldn't get an error when calling FileHashSHA256Check for a non-existing file")
	}
	if ok {
		t.Error("FileHashSHA256Check should return false for a non-existing file")
	}
}

var comparetests = []struct {
	left     FileRes
	right    FileRes
	expected bool
}{
	{FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "absent"},
		FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "absent"}, true}, // equal
	{FileRes{BaseRes: BaseRes{Name: "fileres_one"}, Path: "/a/b/c", Content: "content", State: "absent"},
		FileRes{BaseRes: BaseRes{Name: "fileres_two"}, Path: "/a/b/c", Content: "content", State: "absent"}, false}, // different names
	{FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "absent"},
		FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/d", Content: "content", State: "absent"}, false}, // different paths
	{FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content one", State: "absent"},
		FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content two", State: "absent"}, false}, // different content
	{FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "absent"},
		FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "exists"}, false}, // different state
	//	{FileRes{BaseRes: BaseRes{Name: "fileres"}, Path: "/a/b/c", Content: "content", State: "absent"},
	//		NoopRes{}, false}, // different resources
}

// Pending: need to implement FileRes.BackPoke to satisfy Res interface
// func TestCompare(t *testing.T) {
//	for _, testcase := range comparetests {
//		if testcase.left.Compare(testcase.right) != testcase.expected {
//			t.Error("Expected '", testcase.expected, "', got: ", testcase.left.Compare(testcase.right))
//		}
//	}
//}

func TestCopyFileWithExistingPathsShouldNotReturnAnyErrors(t *testing.T) {
	dstpath := "./file_copy_test.txt"
	err := (&FileRes{Path: "./test_resources/non_existent.txt"}).CopyFile("./test_resources/file_content_test.txt", dstpath)
	if err != nil {
		t.Error("Shouldn't return any errors when copying a file")
	}
	defer os.Remove(dstpath)

	content, err := ioutil.ReadFile(dstpath)
	if err != nil {
		t.Error("Destination file should be readable")
	}
	if string(content) != "test content" {
		t.Error("Expected copied file to contain 'test content', got: '", content, "'")
	}
}

func TestCopyFileToNonExistentDir(t *testing.T) {
	err := (&FileRes{Path: "./test_resources/non_existent.txt"}).CopyFile("./test_resources/file_content_test.txt", "./a/b/file_content_test.txt")
	if err == nil {
		t.Error("Should return an error on attempting to copy a file to a non-existent dir")
	}
}

func TestCopyNonExistentSourceFile(t *testing.T) {
	err := (&FileRes{Path: "./test_resources/non_existent.txt"}).CopyFile("./a/b/file_content_test.txt", "./file_content_test.txt")
	if err == nil {
		t.Error("Should return an error on attempting to copy a non-existent file")
	}
}

//func TestCopyDirWithExistingPaths(t *testing.T) {
//	dstdirpath := "./dir_copy_test"
//	dstfilepath := "./dir_copy_test/file_content_test.txt"
//	err := (&FileRes{Path: "./test_resources/non_existent.txt"}).CopyDir("./test_resources/file_content_test.txt", dstdirpath)
//	if err != nil {
//		t.Error("Shouldn't return any errors when copying a dir")
//	}
//
//	if _, err := os.Stat(dstdirpath); err != nil {
//		t.Error("Should've created dst dir, but: ", err)
//	}
//	if _, err := os.Stat(dstfilepath); err != nil {
//		t.Error("Should've created dst file, but: ", err)
//	}
//	defer os.Remove(dstdirpath)
//
//	content, err := ioutil.ReadFile(dstfilepath)
//	if err != nil {
//		t.Error("Destination file should be readable")
//	}
//	if string(content) != "test content" {
//		t.Error("Expected copied file to contain 'test content', got: '", content, "'")
//	}
//}
