// Modified from: golang/src/crypto/tls/generate_cert.go

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/purpleidea/mgmt/util"
)

// HelloServer is a simple handler.
func HelloServer(w http.ResponseWriter, req *http.Request) {
	fmt.Printf("req: %+v\n", req)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("This is hello world!\n"))
}

func main() {
	// wget --no-check-certificate https://127.0.0.1:1443/hello -O -

	tls := util.NewTLS()
	tls.Host = "localhost" // TODO: choose something
	keyPemFile := "/tmp/key.pem"
	certPemFile := "/tmp/cert.pem"

	if err := tls.Generate(keyPemFile, certPemFile); err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	http.HandleFunc("/hello", HelloServer)
	if err := http.ListenAndServeTLS(":1443", certPemFile, keyPemFile, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
