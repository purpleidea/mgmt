package main

import (
	"testing"
)

func TestGetResReturnsCorrectType(t *testing.T) {
	v := (&FileRes{}).GetRes()
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
	_, errors := (&FileRes{Path: "./test_resources/non_existent.txt", Content: "test content"}).FileHashSHA256Check()
	if errors == nil {
		t.Error("Shouldn't be able to open the file './test_resources/non_existent.txt' but got no errors")
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
