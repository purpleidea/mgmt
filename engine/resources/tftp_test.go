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

//go:build !root

package resources

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
)

func TestTFTPServerWatchReportsListenErrorBeforeCheckApply(t *testing.T) {
	addr := tftpHeldUDPAddr(t)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- obj.Watch(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected listen error")
		}
	case <-events:
		t.Fatalf("watch emitted startup event before reporting listen error")
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for listen error")
	}
}

func TestTFTPServerWaitsForSuccessfulCheckApplyBeforeServing(t *testing.T) {
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

	root := filepath.Join(t.TempDir(), "missing") + string(os.PathSeparator)
	obj := &TFTPServerRes{
		Address: addr,
		Root:    root,
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
	if err := file.Init(init); err != nil {
		t.Fatalf("file init failed: %+v", err)
	}
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

	tftpExpectNoData(t, addr, "example.bin")

	if checkOK, err := obj.CheckApply(ctx, true); err == nil {
		t.Fatalf("expected missing Root error, got checkOK=%t", checkOK)
	}
	tftpExpectNoData(t, addr, "example.bin")

	if err := os.MkdirAll(root, 0777); err != nil {
		t.Fatalf("mkdir root failed: %+v", err)
	}
	checkOK, err := obj.CheckApply(ctx, true)
	if err != nil {
		t.Fatalf("checkapply failed: %+v", err)
	}
	if !checkOK {
		t.Fatalf("expected checkOK=true")
	}

	if err := tftpWaitForData(addr, "example.bin", []byte("hello"), 2*time.Second); err != nil {
		t.Fatalf("server did not serve after successful CheckApply: %+v", err)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch returned unexpected error: %+v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("watch blocked on shutdown")
	}
}

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
	if err := file.Init(init); err != nil {
		t.Fatalf("file init failed: %+v", err)
	}
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

	checkOK, err := obj.CheckApply(ctx, true)
	if err != nil {
		t.Fatalf("checkapply failed: %+v", err)
	}
	if !checkOK {
		t.Fatalf("expected checkOK=true")
	}

	client, packet, err := tftpReadFirstData(addr, "example.bin", 2*time.Second)
	if err != nil {
		t.Fatalf("read data failed: %+v", err)
	}
	defer client.Close()
	if len(packet) < 4 || packet[0] != 0 || packet[1] != 3 || packet[2] != 0 || packet[3] != 1 {
		t.Fatalf("unexpected tftp packet: %v", packet)
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

func tftpExpectNoData(t *testing.T, addr string, filename string) {
	t.Helper()

	client, err := tftpWriteRRQ(addr, filename)
	if err != nil {
		t.Fatalf("write rrq failed: %+v", err)
	}
	defer client.Close()

	buf := make([]byte, 516)
	if err := client.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline failed: %+v", err)
	}
	n, _, err := client.ReadFromUDP(buf)
	if err != nil {
		return
	}
	if n >= 4 && buf[0] == 0 && buf[1] == 3 {
		t.Fatalf("server sent data before successful CheckApply: %v", buf[:n])
	}
}

func tftpWaitForData(addr string, filename string, data []byte, timeout time.Duration) error {
	client, packet, err := tftpReadFirstData(addr, filename, timeout)
	if err != nil {
		return err
	}
	client.Close()
	if len(packet) >= 4 && packet[0] == 0 && packet[1] == 3 && string(packet[4:]) == string(data) {
		return nil
	}
	return fmt.Errorf("unexpected tftp packet: %v", packet)
}

func tftpReadFirstData(addr string, filename string, timeout time.Duration) (*net.UDPConn, []byte, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := tftpWriteRRQ(addr, filename)
		if err != nil {
			return nil, nil, err
		}

		buf := make([]byte, 516)
		if err := client.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			client.Close()
			return nil, nil, err
		}
		n, _, err := client.ReadFromUDP(buf)
		if err != nil {
			client.Close()
			time.Sleep(25 * time.Millisecond)
			continue
		}
		packet := append([]byte{}, buf[:n]...)
		return client, packet, nil
	}
	return nil, nil, fmt.Errorf("timed out waiting for data")
}

func tftpWriteRRQ(addr string, filename string) (*net.UDPConn, error) {
	serverAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	client, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	rrq := []byte{0, 1}
	rrq = append(rrq, []byte(filename)...)
	rrq = append(rrq, 0)
	rrq = append(rrq, []byte("octet")...)
	rrq = append(rrq, 0)
	if _, err := client.WriteToUDP(rrq, serverAddr); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func tftpHeldUDPAddr(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp failed: %+v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn.LocalAddr().String()
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
