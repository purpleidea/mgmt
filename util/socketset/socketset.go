// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// Package socketset is in API for creating a select style netlink
// socket to receive events from the kernel.
package socketset

import (
	"fmt"
	"os"
	"path"
	"syscall"

	errwrap "github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// SocketSet is used to receive events from a socket and shut it down cleanly
// when asked. It contains a socket for events and a pipe socket to unblock
// receive on shutdown.
type SocketSet struct {
	fdEvents int
	fdPipe   int
	pipeFile string
}

// NewSocketSet returns a socketSet, initialized with the given parameters.
func NewSocketSet(groups uint32, file string) (*SocketSet, error) {
	// make a netlink socket file descriptor
	fdEvents, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating netlink socket")
	}
	// bind to the socket and add add the netlink groups we need to get events
	if err := unix.Bind(fdEvents, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: groups,
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding netlink socket")
	}

	// create a pipe socket to unblock unix.Select when we close
	fdPipe, err := unix.Socket(unix.AF_UNIX, unix.SOCK_RAW, unix.PROT_NONE)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating pipe socket")
	}
	// bind the pipe to a file
	if err = unix.Bind(fdPipe, &unix.SockaddrUnix{
		Name: file,
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding pipe socket")
	}
	return &SocketSet{
		fdEvents: fdEvents,
		fdPipe:   fdPipe,
		pipeFile: file,
	}, nil
}

// Shutdown closes the event file descriptor and unblocks receive by sending
// a message to the pipe file descriptor. It must be called before close, and
// should only be called once.
func (obj *SocketSet) Shutdown() error {
	// close the event socket so no more events are produced
	if err := unix.Close(obj.fdEvents); err != nil {
		return err
	}
	// send a message to the pipe to unblock select
	return unix.Sendto(obj.fdPipe, nil, 0, &unix.SockaddrUnix{
		Name: path.Join(obj.pipeFile),
	})
}

// Close closes the pipe file descriptor. It must only be called after
// shutdown has closed fdEvents, and unblocked receive. It should only be
// called once.
func (obj *SocketSet) Close() error {
	return unix.Close(obj.fdPipe)
}

// Receive waits for bytes from fdEvents and parses them into a slice of
// netlink messages. It will block until an event is produced, or shutdown
// is called.
func (obj *SocketSet) Receive() ([]syscall.NetlinkMessage, error) {
	// Select will return when any fd in fdSet (fdEvents and fdPipe) is ready
	// to read.
	_, err := unix.Select(obj.nfd(), obj.fdSet(), nil, nil, nil)
	if err != nil {
		// if a system interrupt is caught
		if err == unix.EINTR { // signal interrupt
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error selecting on fd")
	}
	// receive the message from the netlink socket into b
	b := make([]byte, os.Getpagesize())
	n, _, err := unix.Recvfrom(obj.fdEvents, b, unix.MSG_DONTWAIT) // non-blocking receive
	if err != nil {
		// if fdEvents is closed
		if err == unix.EBADF { // bad file descriptor
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error receiving messages")
	}
	// if we didn't get enough bytes for a header, something went wrong
	if n < unix.NLMSG_HDRLEN {
		return nil, fmt.Errorf("received short header")
	}
	b = b[:n] // truncate b to message length
	// use syscall to parse, as func does not exist in x/sys/unix
	return syscall.ParseNetlinkMessage(b)
}

// nfd returns one more than the highest fd value in the struct, for use as as
// the nfds parameter in select. It represents the file descriptor set maximum
// size. See man select for more info.
func (obj *SocketSet) nfd() int {
	if obj.fdEvents > obj.fdPipe {
		return obj.fdEvents + 1
	}
	return obj.fdPipe + 1
}

// fdSet returns a bitmask representation of the integer values of fdEvents
// and fdPipe. See man select for more info.
func (obj *SocketSet) fdSet() *unix.FdSet {
	fdSet := &unix.FdSet{}
	// Generate the bitmask representing the file descriptors in the socketSet.
	// The rightmost bit corresponds to file descriptor zero, and each bit to
	// the left represents the next file descriptor number in the sequence of
	// all real numbers. E.g. the FdSet containing containing 0 and 4 is 10001.
	fdSet.Bits[obj.fdEvents/64] |= 1 << uint(obj.fdEvents)
	fdSet.Bits[obj.fdPipe/64] |= 1 << uint(obj.fdPipe)
	return fdSet
}
