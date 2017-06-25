package resources

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setup(t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))

	return ts
}

const (
	responseSum = "uv9HivXgof+/yspdGjuJk4cun1Qs8aX4LdScPq7XbBw="
)

func TestHTTPResource(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	res := (&HTTPRes{}).Default()

	res.(*HTTPRes).URL = ts.URL

	if err := res.Init(); err != nil {
		t.Error(err)
	}

	pass, err := res.CheckApply(false)
	if err != nil {
		t.Error(err)
	}

	if res.(*HTTPRes).ShaSum != responseSum {
		t.Errorf("expected ShaSum to equal %s got %s", responseSum, res.(*HTTPRes).ShaSum)
	}

	fmt.Println(pass)
}

func TestHTTPResourceWatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	res := (&HTTPRes{}).Default()

	res.(*HTTPRes).URL = ts.URL

	if err := res.Init(); err != nil {
		t.Error(err)
	}

	go func() {
		err := res.Watch()
		if err != nil {
			t.Error(err)
		}
	}()
}
