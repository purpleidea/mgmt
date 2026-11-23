// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package socket

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// acceptWithin runs Accept, and fails if it doesn't return within the timeout.
func acceptWithin(t *testing.T, listener net.Listener, timeout time.Duration) (net.Conn, error) {
	t.Helper()
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan *result, 1)
	go func() {
		conn, err := listener.Accept()
		ch <- &result{conn: conn, err: err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-time.After(timeout):
		t.Fatalf("func Accept didn't return within %s", timeout)
		return nil, nil // unreachable
	}
}

// checkConn checks that the client and the server ends of a connection are
// joined, by sending one byte across.
func checkConn(t *testing.T, client, server net.Conn) {
	t.Helper()
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatalf("client Write: %v", err)
	}
	if err := server.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}
	b := make([]byte, 1)
	if _, err := io.ReadFull(server, b); err != nil || string(b) != "x" {
		t.Fatalf("server Read: %q %v", b, err)
	}
}

// TestMap checks that there's at most one socket for each address, and that one
// which was removed can be added again.
func TestMap(t *testing.T) {
	const address = "127.0.0.1:0"
	m := NewMap()

	sock, err := m.Add(address)
	if err != nil {
		t.Fatalf("func Add: %v", err)
	}
	if _, err := m.Add(address); err == nil {
		t.Fatal("added a second socket for the same address")
	}
	if m.Get(address) != sock {
		t.Fatal("func Get didn't return the socket")
	}

	if err := m.Remove(sock); err != nil { // it was never bound
		t.Fatalf("func Remove: %v", err)
	}
	if m.Get(address) != nil {
		t.Fatal("func Get returned a removed socket")
	}
	if _, err := sock.Attach(); err == nil {
		t.Fatal("attached to a removed socket")
	}

	again, err := m.Add(address)
	if err != nil {
		t.Fatalf("func Add after Remove: %v", err)
	}
	if err := m.Remove(again); err != nil {
		t.Fatalf("func Remove: %v", err)
	}
}

// TestSocketAcrossAttach checks that the socket stays open between one attached
// listener and the next, that a connection which is made in between goes to the
// next one, and that only one listener at a time can be attached.
func TestSocketAcrossAttach(t *testing.T) {
	m := NewMap()
	sock, err := m.Add("127.0.0.1:0") // the first Attach picks the port
	if err != nil {
		t.Fatalf("func Add: %v", err)
	}
	removed := false
	defer func() {
		if !removed {
			_ = m.Remove(sock)
		}
	}()

	l1, err := sock.Attach()
	if err != nil {
		t.Fatalf("func Attach: %v", err)
	}
	addr := l1.Addr().String()
	if _, err := sock.Attach(); err == nil {
		t.Fatal("two listeners are attached at once")
	}

	c1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("func Dial: %v", err)
	}
	defer c1.Close()
	s1, err := acceptWithin(t, l1, 5*time.Second)
	if err != nil {
		t.Fatalf("func Accept: %v", err)
	}
	defer s1.Close()
	checkConn(t, c1, s1)

	// Detach. Closing more than once is fine, and Accept then returns the
	// error that tells a server that it was closed normally.
	if err := l1.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	if err := l1.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := acceptWithin(t, l1, 5*time.Second); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("func Accept after Close: %v", err)
	}

	// Nothing is attached, but the socket is still open.
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("the socket closed between two listeners: %v", err)
	}
	defer c2.Close()

	l2, err := sock.Attach()
	if err != nil {
		t.Fatalf("second Attach: %v", err)
	}
	defer l2.Close()
	if a := l2.Addr().String(); a != addr {
		t.Fatalf("second listener is on %s, not on %s", a, addr)
	}
	s2, err := acceptWithin(t, l2, 5*time.Second)
	if err != nil {
		t.Fatalf("second Accept: %v", err)
	}
	defer s2.Close()
	checkConn(t, c2, s2)

	// Removing the socket while a listener is attached is a bug of the
	// caller, so Accept must not report it as a normal close.
	removed = true
	if err := m.Remove(sock); err != nil {
		t.Fatalf("func Remove: %v", err)
	}
	if _, err := acceptWithin(t, l2, 5*time.Second); err == nil || errors.Is(err, net.ErrClosed) {
		t.Fatalf("func Accept after Remove: %v", err)
	}
	if conn, err := net.Dial("tcp", addr); err == nil {
		_ = conn.Close()
		t.Fatal("the socket is still open after Remove")
	}
}

// TestSocketClosedListenerTakesNothing checks that once a listener is closed,
// its Accept never returns a connection, even one which is already waiting for
// the next listener. We repeat this, since without the check, Accept would only
// take the waiting connection about half of the time.
func TestSocketClosedListenerTakesNothing(t *testing.T) {
	m := NewMap()
	sock, err := m.Add("127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Add: %v", err)
	}
	defer func() {
		_ = m.Remove(sock)
	}()

	listener, err := sock.Attach()
	if err != nil {
		t.Fatalf("func Attach: %v", err)
	}
	addr := listener.Addr().String()

	for i := 0; i < 20; i++ {
		if err := listener.Close(); err != nil {
			t.Fatalf("func Close: %v", err)
		}

		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("func Dial: %v", err)
		}
		defer conn.Close()
		// Give the accept goroutine time to pick it up, so that it's
		// waiting to be handed over. If it's not there yet, then this
		// round can't fail, but it's still correct.
		time.Sleep(20 * time.Millisecond)

		if c, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
			if c != nil {
				_ = c.Close()
			}
			t.Fatalf("a closed listener took a connection, in round %d: %v", i, err)
		}

		listener, err = sock.Attach()
		if err != nil {
			t.Fatalf("func Attach: %v", err)
		}
		s, err := acceptWithin(t, listener, 5*time.Second)
		if err != nil {
			t.Fatalf("func Accept: %v", err)
		}
		checkConn(t, conn, s)
		_ = s.Close()
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
}

// TestSocketRemoveClosesWaiting checks that a connection which is waiting for a
// listener to attach is closed when the socket is removed, instead of hanging.
func TestSocketRemoveClosesWaiting(t *testing.T) {
	m := NewMap()
	sock, err := m.Add("127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Add: %v", err)
	}
	listener, err := sock.Attach() // bind it
	if err != nil {
		_ = m.Remove(sock)
		t.Fatalf("func Attach: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		_ = m.Remove(sock)
		t.Fatalf("func Close: %v", err)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		_ = m.Remove(sock)
		t.Fatalf("func Dial: %v", err)
	}
	defer conn.Close()

	if err := m.Remove(sock); err != nil {
		t.Fatalf("func Remove: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}
	_, err = conn.Read(make([]byte, 1)) // EOF, or maybe a reset
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("the waiting connection wasn't closed")
	}
	if err == nil {
		t.Fatal("the waiting connection got data")
	}
}

// TestSocketBindError checks that a port which is in use is an error of the
// Attach, and that a later Attach can retry.
func TestSocketBindError(t *testing.T) {
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Listen: %v", err)
	}
	address := occupier.Addr().String()

	m := NewMap()
	sock, err := m.Add(address)
	if err != nil {
		_ = occupier.Close()
		t.Fatalf("func Add: %v", err)
	}
	defer func() {
		_ = m.Remove(sock)
	}()

	if listener, err := sock.Attach(); err == nil {
		_ = listener.Close()
		_ = occupier.Close()
		t.Fatal("bound a port which is in use")
	}

	if err := occupier.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	listener, err := sock.Attach()
	if err != nil {
		t.Fatalf("the retry couldn't bind the port: %v", err)
	}
	defer listener.Close()
}
