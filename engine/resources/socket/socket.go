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

// Package socket keeps a listening socket open across the servers which use it,
// one at a time, so that a new server can take over an address without the
// socket ever closing. Connections which arrive while no server is attached
// wait for the next one, instead of being refused.
package socket

import (
	"fmt"
	"net"
	"sync"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// Listen binds a new listening tcp socket for this address.
func Listen(address string) (net.Listener, error) {
	// TODO: I think we could replace all this with:
	//listener, err := net.Listen("tcp", address)
	// ...but what is the advantage?
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not resolve address")
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

// Map is a map from an address to its socket. It's safe to use from more than
// one goroutine.
type Map struct {
	mutex   *sync.Mutex
	sockets map[string]*Socket
}

// NewMap returns a new empty map.
func NewMap() *Map {
	return &Map{
		mutex:   &sync.Mutex{},
		sockets: make(map[string]*Socket),
	}
}

// Add makes a new socket for this address. It doesn't bind it, the first Attach
// does that. It errors if there's already a socket for this address.
func (obj *Map) Add(address string) (*Socket, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if _, exists := obj.sockets[address]; exists {
		return nil, fmt.Errorf("there's already a socket for %s", address)
	}
	socket := &Socket{
		address:  address,
		mutex:    &sync.Mutex{},
		accepted: make(chan *acceptResult),
		done:     make(chan struct{}),
		wg:       &sync.WaitGroup{},
	}
	obj.sockets[address] = socket
	return socket, nil
}

// Get returns the socket for this address, or nil if there isn't one.
func (obj *Map) Get(address string) *Socket {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.sockets[address] // nil if missing
}

// Remove takes this socket out of the map, so that nobody new can find it, and
// then closes it.
func (obj *Map) Remove(socket *Socket) error {
	obj.mutex.Lock()
	if obj.sockets[socket.address] == socket {
		delete(obj.sockets, socket.address)
	}
	obj.mutex.Unlock()

	return socket.close()
}

// Socket is a listening socket which outlives the servers that serve it. One
// server at a time may attach to it. The first one to attach binds it, so that
// a bind error is an error of that server, and not one of whoever added this
// socket. One goroutine accepts the connections, and hands them over to the
// attached server. When no server is attached, eg: while one replaces another,
// the next connection waits in there, and the others wait in the kernel's
// accept queue, so that they get served by the next server, instead of being
// refused.
type Socket struct {
	address string

	mutex    *sync.Mutex  // guards the fields below
	listener net.Listener // nil until a server binds it
	attached bool         // is a server serving this?
	closed   bool         // was this closed?

	accepted chan *acceptResult // from the accept goroutine
	done     chan struct{}      // closed when this is closed
	wg       *sync.WaitGroup    // for the accept goroutine
}

// acceptResult is the result of one Accept call on the real listener.
type acceptResult struct {
	conn net.Conn
	err  error
}

// Attach binds the socket if that wasn't done yet, and returns a listener for
// one server to serve. Closing that listener detaches the server, and leaves
// the socket open. This errors if another server is attached, since two servers
// can't share an address.
func (obj *Socket) Attach() (net.Listener, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.closed {
		return nil, fmt.Errorf("the socket for %s is closed", obj.address)
	}
	if obj.attached {
		return nil, fmt.Errorf("another server is using %s", obj.address)
	}
	if obj.listener == nil {
		listener, err := Listen(obj.address)
		if err != nil {
			return nil, err
		}
		obj.listener = listener
		obj.wg.Add(1)
		go obj.accept(listener)
	}
	obj.attached = true

	return &attachedListener{
		socket: obj,
		addr:   obj.listener.Addr(),
		once:   &sync.Once{},
		closed: make(chan struct{}),
	}, nil
}

// detach is run when the attached server closes its listener.
func (obj *Socket) detach() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.attached = false
}

// accept is the goroutine which accepts every connection to the socket, until
// the socket is closed. The server which receives them handles the errors, eg:
// an http.Server retries the temporary ones after a short wait. Since each one
// blocks until it's received, that also paces this loop.
func (obj *Socket) accept(listener net.Listener) {
	defer obj.wg.Done()
	for {
		conn, err := listener.Accept()
		select {
		case <-obj.done: // we closed the listener
			if conn != nil {
				_ = conn.Close()
			}
			return
		default:
		}

		select {
		case obj.accepted <- &acceptResult{conn: conn, err: err}:
		case <-obj.done:
			if conn != nil {
				_ = conn.Close()
			}
			return
		}
	}
}

// close closes the socket, and waits for the accept goroutine to exit. If a
// connection is still waiting in there, then it's closed too.
func (obj *Socket) close() error {
	obj.mutex.Lock()
	if obj.closed {
		obj.mutex.Unlock()
		return nil
	}
	obj.closed = true
	close(obj.done)
	var err error
	if obj.listener != nil {
		err = obj.listener.Close() // unblocks the accept goroutine
	}
	obj.mutex.Unlock()

	obj.wg.Wait()
	return err
}

// attachedListener is the net.Listener which an attached server serves. Closing
// it detaches that server, and leaves the socket open for the next one.
type attachedListener struct {
	socket *Socket
	addr   net.Addr
	once   *sync.Once
	closed chan struct{}
}

// Accept waits for the next connection to the socket. Once this is closed, it
// never returns a connection, even one which is already waiting, since that one
// belongs to the next server. Only one which arrives while this is being closed
// might still be returned. A server which is shutting down, eg: an http.Server,
// then serves it as part of its drain, which is still allowed.
func (obj *attachedListener) Accept() (net.Conn, error) {
	// Check this first, since a select picks at random among the cases
	// which are ready, and a connection might be waiting already.
	select {
	case <-obj.closed:
		return nil, net.ErrClosed
	default:
	}

	select {
	case result := <-obj.socket.accepted:
		return result.conn, result.err
	case <-obj.closed:
		return nil, net.ErrClosed
	case <-obj.socket.done:
		// Whoever removes the socket must only do that once every
		// server that uses it has stopped, so this would be a bug. It's
		// not net.ErrClosed, so that the server errors instead of
		// exiting.
		return nil, fmt.Errorf("the socket for %s was closed while in use", obj.socket.address)
	}
}

// Close detaches the server from the socket, and makes Accept return. It can be
// called more than once.
func (obj *attachedListener) Close() error {
	obj.once.Do(func() {
		close(obj.closed)
		obj.socket.detach()
	})
	return nil
}

// Addr returns the address of the socket.
func (obj *attachedListener) Addr() net.Addr {
	return obj.addr
}
