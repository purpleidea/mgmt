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

package resources

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
)

func TestTFTPServerWatchShutdownWithStalledClient(t *testing.T) {
	addr := tftpTestUDPAddr(t)

	events := make(chan struct{}, 1)
	init := &engine.Init{
		Event: func(ctx context.Context) error {
			select {
			case events <- struct{}{}:
			default:
			}
			return nil
		},
		Logf: func(format string, v ...interface{}) {
			t.Logf(format, v...)
		},
	}

	obj := &TFTPServerRes{
		Address: addr,
		Timeout: TftpDefaultTimeout,
	}
	obj.SetName("server")
	if err := obj.Init(init); err != nil {
		t.Fatalf("init failed: %+v", err)
	}

	file := &TFTPFileRes{
		Filename: "example.bin",
		Data:     "hello",
	}
	file.SetName("example.bin")
	if err := obj.GroupRes(file); err != nil {
		t.Fatalf("group failed: %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- obj.Watch(ctx)
	}()

	select {
	case <-events:
	case err := <-done:
		t.Fatalf("watch exited before startup event: %+v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for startup event")
	}

	serverAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		t.Fatalf("resolve server address failed: %+v", err)
	}
	client, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatalf("listen client failed: %+v", err)
	}
	defer client.Close()

	rrq := []byte{
		0, 1,
		'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'b', 'i', 'n', 0,
		'o', 'c', 't', 'e', 't', 0,
	}
	if _, err := client.WriteToUDP(rrq, serverAddr); err != nil {
		t.Fatalf("write rrq failed: %+v", err)
	}

	buf := make([]byte, 516)
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %+v", err)
	}
	n, _, err := client.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read data failed: %+v", err)
	}
	if n < 4 || buf[0] != 0 || buf[1] != 3 || buf[2] != 0 || buf[3] != 1 {
		t.Fatalf("unexpected tftp packet: %v", buf[:n])
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch returned unexpected error: %+v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("watch blocked on a stalled client transfer")
	}
}

func tftpTestUDPAddr(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp failed: %+v", err)
	}
	defer conn.Close()

	return conn.LocalAddr().String()
}
