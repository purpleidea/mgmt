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

package sshutil

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestPrioritizeHostKeyAlgorithms(t *testing.T) {
	helper := &Helper{
		Logf: func(format string, v ...interface{}) {},
	}

	// A representative default offer order where RSA outranks ecdsa. Without
	// prioritization the server would return an RSA key.
	offered := []string{
		ssh.KeyAlgoRSASHA256,
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoECDSA256,
		ssh.KeyAlgoRSA,
	}

	// When known_hosts only has an ecdsa entry for this host, that algorithm
	// must be moved to the front so we negotiate a key we can verify.
	got := helper.PrioritizeHostKeyAlgorithms(offered, nil, ssh.KeyAlgoECDSA256)
	if len(got) == 0 || got[0] != ssh.KeyAlgoECDSA256 {
		t.Fatalf("func PrioritizeHostKeyAlgorithms: expected %s first, got: %v", ssh.KeyAlgoECDSA256, got)
	}

	// A known ssh-rsa entry should prefer the SHA-2 variants first, but must
	// not drop the offered algorithms.
	got = helper.PrioritizeHostKeyAlgorithms(offered, nil, ssh.KeyAlgoRSA)
	if len(got) == 0 || got[0] != ssh.KeyAlgoRSASHA256 {
		t.Fatalf("func PrioritizeHostKeyAlgorithms: expected %s first, got: %v", ssh.KeyAlgoRSASHA256, got)
	}
	if len(got) != len(offered) {
		t.Fatalf("func PrioritizeHostKeyAlgorithms: dropped algorithms, got: %v", got)
	}
}

func TestDialSSHWithContextCancelsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		client, err := DialSSHWithContext(ctx, "tcp", listener.Addr().String(), &ssh.ClientConfig{
			User:            "test",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // G106: isolated loopback test has no SSH server key
		})
		if client != nil {
			client.Close() //nolint:gosec // G104: preserve the dial result checked by the test
		}
		result <- err
	}()

	var conn net.Conn
	select {
	case conn = <-accepted:
		defer conn.Close()
	case err := <-acceptErr:
		t.Fatalf("could not accept: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out accepting connection")
	}

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the SSH handshake did not stop after context cancellation")
	}
}
