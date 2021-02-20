package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/pin/tftp"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		log.Printf("Usage: ./%s [port] <filename>", os.Args[0])
		return
	}

	port := "69" // default
	if len(os.Args) >= 3 {
		port = os.Args[1]
	}
	file := os.Args[len(os.Args)-1]

	addr := fmt.Sprintf("127.0.0.1:%s", port)
	log.Printf("Connecting to: %s", addr)

	c, err := tftp.NewClient(addr)
	if err != nil {
		log.Printf("Error connecting to server: %v", err)
		return
	}
	wt, err := c.Receive(file, "octet") // no idea why this is "octet"
	if err != nil {
		log.Printf("Error receiving from server: %v", err)
		return
	}

	// Optionally obtain transfer size before actual data.
	if n, ok := wt.(tftp.IncomingTransfer).Size(); ok {
		log.Printf("Transfer size: %d", n)
	}

	buf := new(bytes.Buffer)
	n, err := wt.WriteTo(buf)
	if err != nil {
		log.Printf("Error writing to buffer: %v", err)
		return
	}

	log.Printf("%d bytes received", n)
	log.Printf("Got: %s", buf.String())

	log.Printf("Done!")
}
