// Mgmt
// Copyright (C) James Shubin and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !root

package esphome

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/flavio-fernandes/go-aioesphomeapi/pb"
	"github.com/flavio-fernandes/go-aioesphomeapi/simulator"
)

func TestAPIClientSessionPollRealWire(t *testing.T) {
	peer := startRealWireSimulator(t, "127.0.0.1:0")
	defer peer.close(t)
	session := realWireSession(t)
	defer session.Release()

	session.Configure(peer.info(3600))
	waitFor(t, "encrypted polling snapshot", func() bool {
		state := session.State("motor_speed")
		stats := peer.device.Stats()
		return session.Connected() && state != nil && state.Float == 0.75 &&
			stats.AcceptedConnections == 1 && stats.ActiveConnections == 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.SetNumber(ctx, "motor_speed", 0.5); err != nil {
		t.Fatalf("wake polling session for number command: %v", err)
	}
	select {
	case message := <-peer.device.Commands():
		command, ok := message.(*pb.NumberCommandRequest)
		if !ok || command.Key != simulator.BasicNumberKey || command.State != 0.5 {
			t.Fatalf("unexpected polling command: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("polling command did not reach the encrypted simulator")
	}
	waitFor(t, "second polling cycle cleanup", func() bool {
		stats := peer.device.Stats()
		return stats.AcceptedConnections == 2 && stats.ActiveConnections == 0
	})
	if !session.Connected() {
		t.Fatal("successful polling session did not remain healthy between cycles")
	}
}

func TestAPIClientSessionReconnectAndOutageRealWire(t *testing.T) {
	first := startRealWireSimulator(t, "127.0.0.1:0")
	session := realWireSession(t)
	defer session.Release()
	session.Configure(first.info(0))

	waitFor(t, "first encrypted persistent session", func() bool {
		return session.Connected() && session.State("button_a") != nil
	})
	_, initialOutageID := session.LastOutage()
	address := first.address
	first.close(t)
	waitFor(t, "persistent disconnect", func() bool { return !session.Connected() })

	second := startRealWireSimulator(t, address)
	defer second.close(t)
	waitFor(t, "MGMT-owned encrypted reconnect", func() bool {
		outage, outageID := session.LastOutage()
		return session.Connected() && second.device.Stats().AcceptedConnections == 1 &&
			outageID > initialOutageID && outage > 0
	})
	if state := session.State("button_a"); state == nil || state.Domain != DomainBinarySensor {
		t.Fatalf("reconnected session did not restore entity state: %#v", state)
	}
	select {
	case command := <-second.device.Commands():
		t.Fatalf("reconnect silently replayed an unrequested command: %#v", command)
	case <-time.After(50 * time.Millisecond):
	}
}

type realWirePeer struct {
	device  *simulator.Device
	address string
	done    chan error
}

func startRealWireSimulator(t *testing.T, address string) *realWirePeer {
	t.Helper()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen for encrypted simulator: %v", err)
	}
	device := simulator.New(simulator.BasicIOScenario())
	done := make(chan error, 1)
	go func() { done <- device.Serve(listener) }()
	return &realWirePeer{device: device, address: listener.Addr().String(), done: done}
}

func (p *realWirePeer) info(interval uint32) *ConnInfo {
	host, portString, _ := net.SplitHostPort(p.address)
	port, _ := strconv.Atoi(portString)
	return &ConnInfo{Host: host, Port: port, Key: simulator.DefaultTestEncryptionKey, Interval: interval}
}

func (p *realWirePeer) close(t *testing.T) {
	t.Helper()
	if err := p.device.Close(); err != nil {
		t.Fatalf("close encrypted simulator: %v", err)
	}
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("serve encrypted simulator: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("encrypted simulator did not stop")
	}
}

func realWireSession(t *testing.T) *Session {
	t.Helper()
	session := newSession("real-wire-" + t.Name())
	session.count++
	return session
}
