// This is an example longpoll client. The connection to the corresponding
// server initiates a request on a "Watch". It then waits until a redirect is
// received from the server which indicates that the watch is ready. To signal
// than an event on this watch has occurred, the server sends a final message.
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"
)

const (
	timeout = 15
)

func main() {
	log.Printf("Starting...")

	checkRedirectFunc := func(req *http.Request, via []*http.Request) error {
		log.Printf("Watch is ready!")
		return nil
	}

	client := &http.Client{
		Timeout:       time.Duration(timeout) * time.Second,
		CheckRedirect: checkRedirectFunc,
	}

	id := rand.Intn(2 ^ 32 - 1)
	body := bytes.NewBufferString("hello")
	url := fmt.Sprintf("http://127.0.0.1:12345/watch?id=%d", id)
	req, err := http.NewRequest("GET", url, body)
	if err != nil {
		log.Printf("err: %+v", err)
		return
	}
	result, err := client.Do(req)
	if err != nil {
		log.Printf("err: %+v", err)
		return
	}
	log.Printf("Event received: %+v", result)

	s, err := ioutil.ReadAll(result.Body) // TODO: apparently we can stream
	result.Body.Close()
	log.Printf("Response: %+v", string(s))
}
