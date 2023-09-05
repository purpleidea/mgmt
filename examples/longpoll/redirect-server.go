// This is an example longpoll server. On client connection it starts a "Watch",
// and notifies the client with a redirect when that watch is ready. This is
// important to avoid a possible race between when the client believes the watch
// is actually ready, and when the server actually is watching.
package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// you can use `wget http://127.0.0.1:12345/hello -O /dev/null` or you can run
// `go run client.go`
const (
	addr = ":12345"
)

// WatchStart kicks off the initial watch and then redirects the client to
// notify them that we're ready. The watch operation here is simulated.
func WatchStart(w http.ResponseWriter, req *http.Request) {
	log.Printf("Start received...")
	time.Sleep(time.Duration(5) * time.Second) // 5 seconds to get ready and start *our* watch ;)
	//started := time.Now().UnixNano() // time since watch is "started"
	log.Printf("URL: %+v", req.URL)

	token := fmt.Sprintf("%d", rand.Intn(2^32-1))
	http.Redirect(w, req, fmt.Sprintf("/ready?token=%s", token), http.StatusSeeOther) // TODO: which code should we use ?
	log.Printf("Redirect sent!")
}

// WatchReady receives the client connection when it has been notified that the
// watch has started, and it returns to signal that an event on the watch
// occurred. The event operation here is simulated.
func WatchReady(w http.ResponseWriter, req *http.Request) {
	log.Printf("Ready received")
	log.Printf("URL: %+v", req.URL)

	//time.Sleep(time.Duration(10) * time.Second)
	time.Sleep(time.Duration(rand.Intn(10)) * time.Second) // wait until an "event" happens

	io.WriteString(w, "Event happened!\n")
	log.Printf("Event sent")
}

func main() {
	log.Printf("Starting...")
	//rand.Seed(time.Now().UTC().UnixNano())
	http.HandleFunc("/watch", WatchStart)
	http.HandleFunc("/ready", WatchReady)
	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
