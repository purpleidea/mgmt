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

package esphome

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeDriver is a scriptable in-memory driver for testing the session logic.
type fakeDriver struct {
	mutex sync.Mutex

	failConnect bool
	entityList  []*EntityInfo
	initial     []*EntityState // states pushed right after subscribe
	fn          func(*EntityState)
	logFn       func(*LogEntry)
	logLevel    string
	doneCh      chan struct{}
	closed      bool
	commands    []string
}

func (obj *fakeDriver) connect(ctx context.Context, info *ConnInfo) error {
	if obj.failConnect {
		return fmt.Errorf("fake connect error")
	}
	obj.doneCh = make(chan struct{})
	return nil
}

func (obj *fakeDriver) entities() ([]*EntityInfo, error) {
	return obj.entityList, nil
}

func (obj *fakeDriver) subscribe(fn func(*EntityState)) error {
	obj.mutex.Lock()
	obj.fn = fn
	obj.mutex.Unlock()
	for _, es := range obj.initial {
		fn(es)
	}
	return nil
}

func (obj *fakeDriver) subscribeLogs(level string, fn func(*LogEntry)) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.logLevel = level
	obj.logFn = fn
	return nil
}

func (obj *fakeDriver) done() <-chan struct{} {
	return obj.doneCh
}

func (obj *fakeDriver) close() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if !obj.closed {
		obj.closed = true
		close(obj.doneCh)
	}
	return nil
}

func (obj *fakeDriver) push(es *EntityState) {
	obj.mutex.Lock()
	fn := obj.fn
	obj.mutex.Unlock()
	if fn != nil {
		fn(es)
	}
}

func (obj *fakeDriver) pushLog(entry *LogEntry) {
	obj.mutex.Lock()
	fn := obj.logFn
	obj.mutex.Unlock()
	if fn != nil {
		fn(entry)
	}
}

func (obj *fakeDriver) setSwitch(key uint32, on bool) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.commands = append(obj.commands, fmt.Sprintf("switch/%d/%t", key, on))
	return nil
}

func (obj *fakeDriver) setNumber(key uint32, value float64) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.commands = append(obj.commands, fmt.Sprintf("number/%d/%v", key, value))
	return nil
}

func (obj *fakeDriver) pressButton(key uint32) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.commands = append(obj.commands, fmt.Sprintf("button/%d", key))
	return nil
}

func (obj *fakeDriver) setFan(key uint32, command FanCommand) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.commands = append(obj.commands, fmt.Sprintf("fan/%d/%t/%d/%s", key, command.State, command.Speed, command.Direction))
	return nil
}

func (obj *fakeDriver) setLight(key uint32, command LightCommand) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.commands = append(obj.commands, fmt.Sprintf("light/%d/%t/%g/%g/%g/%g", key, command.State, command.Brightness, command.Red, command.Green, command.Blue))
	return nil
}

// fakeFactory hands out fake drivers and remembers them in order.
type fakeFactory struct {
	mutex   sync.Mutex
	drivers []*fakeDriver
	prepare func(*fakeDriver) // runs on each new driver
}

func (obj *fakeFactory) newDriver() driver {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	d := &fakeDriver{
		entityList: []*EntityInfo{
			{Key: 1, ObjectID: "button_a", Name: "Button A", Domain: DomainBinarySensor},
			{Key: 2, ObjectID: "led_1", Name: "LED 1", Domain: DomainSwitch},
			{Key: 3, ObjectID: "motor_speed", Name: "Motor Speed", Domain: DomainNumber},
			{Key: 4, ObjectID: "conveyor_motor", Name: "Conveyor Motor", Domain: DomainFan},
			{Key: 5, ObjectID: "status_light", Name: "Status Light", Domain: DomainLight},
		},
		initial: []*EntityState{
			{Key: 1, State: State{Domain: DomainBinarySensor, Bool: false}},
			{Key: 2, State: State{Domain: DomainSwitch, Bool: false}},
			{Key: 4, State: State{Domain: DomainFan, Bool: false, Speed: 40, Direction: FanDirectionForward}},
			{Key: 5, State: State{Domain: DomainLight, Bool: true, Brightness: 0.5, Red: 0, Green: 1, Blue: 0}},
		},
	}
	if obj.prepare != nil {
		obj.prepare(d)
	}
	obj.drivers = append(obj.drivers, d)
	return d
}

func (obj *fakeFactory) driver(i int) *fakeDriver {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if i < len(obj.drivers) {
		return obj.drivers[i]
	}
	return nil
}

func (obj *fakeFactory) count() int {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return len(obj.drivers)
}

// testSession builds a session wired to a fake factory, bypassing the pool so
// tests can't collide on the shared package map.
func testSession(t *testing.T, factory *fakeFactory) *Session {
	session := newSession("test-" + t.Name())
	session.newDriver = factory.newDriver
	session.count++
	return session
}

// waitFor polls the condition until it's true or the deadline passes.
func waitFor(t *testing.T, msg string, fn func() bool) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

func TestNormalizeLogLevel(t *testing.T) {
	tests := map[string]string{
		"":             "",
		"DEBUG":        LogLevelDebug,
		" warning ":    LogLevelWarn,
		"very_verbose": LogLevelVeryVerbose,
	}
	for input, want := range tests {
		got, err := NormalizeLogLevel(input)
		if err != nil {
			t.Fatalf("NormalizeLogLevel(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeLogLevel(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := NormalizeLogLevel("trace"); err == nil {
		t.Fatalf("expected invalid level error")
	}
}

func TestConnInfoValidateRequiresNoise(t *testing.T) {
	info := &ConnInfo{Host: "device.example", Port: DefaultPort}
	if err := info.Validate(); err == nil {
		t.Fatalf("plaintext connection info unexpectedly validated")
	}
	info.Key = "kJ7hc0lJ0Zw9N3DcJzXn1kJ7hc0lJ0Zw9N3DcJzXn1k="
	if err := info.Validate(); err != nil {
		t.Fatalf("valid encrypted connection info: %v", err)
	}
}

func TestSessionReserveRelease(t *testing.T) {
	s1 := SessionReserve("dev1")
	s2 := SessionReserve("dev1")
	if s1 != s2 {
		t.Fatalf("expected the same session for the same uid")
	}
	s3 := SessionReserve("dev2")
	if s3 == s1 {
		t.Fatalf("expected a different session for a different uid")
	}
	s1.Release()
	s2.Release()
	s3.Release()

	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	if len(sessionMap) != 0 {
		t.Fatalf("expected an empty pool, got: %d", len(sessionMap))
	}
}

func TestSessionUnconfigured(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	if session.Connected() {
		t.Fatalf("unconfigured session must not be connected")
	}
	if st := session.State("button_a"); st != nil {
		t.Fatalf("unconfigured session must have no state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.SetSwitch(ctx, "led_1", true); err == nil {
		t.Fatalf("commands must fail when unconfigured")
	}
	if factory.count() != 0 {
		t.Fatalf("unconfigured session must not connect")
	}
}

func TestSessionPersistent(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := session.Watch(ctx)
	if err != nil {
		t.Fatalf("watch error: %v", err)
	}
	go func() { // drain
		for range events {
		}
	}()

	session.Configure(&ConnInfo{Host: "fake", Port: DefaultPort}) // interval 0

	waitFor(t, "connect", session.Connected)

	// The initial snapshot must be in the cache.
	waitFor(t, "initial state", func() bool {
		st := session.State("button_a")
		return st != nil && st.Domain == DomainBinarySensor && !st.Bool
	})

	// A pushed state change must become visible.
	factory.driver(0).push(&EntityState{Key: 1, State: State{Domain: DomainBinarySensor, Bool: true}})
	waitFor(t, "pushed state", func() bool {
		st := session.State("button_a")
		return st != nil && st.Bool
	})

	// Commands run against the live connection.
	cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ccancel()
	if err := session.SetSwitch(cctx, "led_1", true); err != nil {
		t.Fatalf("set switch error: %v", err)
	}
	if err := session.SetFan(cctx, "Conveyor Motor", FanCommand{State: true, Speed: 40, Direction: FanDirectionForward}); err != nil {
		t.Fatalf("set fan error: %v", err)
	}
	if err := session.SetLight(cctx, "Status Light", LightCommand{State: true, Brightness: 0.5, Green: 1}); err != nil {
		t.Fatalf("set light error: %v", err)
	}
	d0 := factory.driver(0)
	d0.mutex.Lock()
	commands := append([]string(nil), d0.commands...)
	d0.mutex.Unlock()
	wantCommands := []string{
		"switch/2/true",
		"fan/4/true/40/forward",
		"light/5/true/0.5/0/1/0",
	}
	if fmt.Sprint(commands) != fmt.Sprint(wantCommands) {
		t.Fatalf("unexpected commands: got %v, want %v", commands, wantCommands)
	}

	// An unknown entity must error.
	if err := session.SetSwitch(cctx, "nope", true); err == nil {
		t.Fatalf("expected an unknown entity error")
	}

	// Killing the connection must reconnect and record an outage.
	_, id0 := session.LastOutage()
	factory.driver(0).close()
	waitFor(t, "reconnect", func() bool {
		_, id := session.LastOutage()
		return id != id0 && session.Connected()
	})
	if factory.count() < 2 {
		t.Fatalf("expected a second connection, got: %d", factory.count())
	}

	// Unconfiguring must clear the cache.
	session.Configure(nil)
	waitFor(t, "unconfigure", func() bool {
		return !session.Connected() && session.State("button_a") == nil
	})
}

func TestSessionDeviceLogs(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	logs := make(chan *LogEntry, 1)
	session.Configure(&ConnInfo{
		Host:     "fake",
		Port:     DefaultPort,
		LogLevel: LogLevelDebug,
		Logf:     func(entry *LogEntry) { logs <- entry },
	})
	waitFor(t, "connect", session.Connected)

	d := factory.driver(0)
	d.mutex.Lock()
	level := d.logLevel
	d.mutex.Unlock()
	if level != LogLevelDebug {
		t.Fatalf("expected debug log subscription, got: %q", level)
	}

	want := &LogEntry{Level: LogLevelInfo, Message: "hello from device"}
	d.pushLog(want)
	select {
	case got := <-logs:
		if *got != *want {
			t.Fatalf("unexpected log entry: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for device log")
	}
}

func TestSessionEntityNamesWithoutObjectIDs(t *testing.T) {
	factory := &fakeFactory{
		prepare: func(d *fakeDriver) {
			for _, entity := range d.entityList {
				entity.ObjectID = ""
			}
		},
	}
	session := testSession(t, factory)
	defer session.Release()

	session.Configure(&ConnInfo{Host: "fake", Port: DefaultPort})
	waitFor(t, "connect", session.Connected)
	waitFor(t, "state addressed by name", func() bool {
		state := session.State("Button A")
		return state != nil && state.Domain == DomainBinarySensor && !state.Bool
	})
	if state := session.State("button_a"); state != nil {
		t.Fatalf("empty object_id must not create a legacy alias")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.SetSwitch(ctx, "LED 1", true); err != nil {
		t.Fatalf("set switch by name: %v", err)
	}
	d := factory.driver(0)
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if len(d.commands) != 1 || d.commands[0] != "switch/2/true" {
		t.Fatalf("unexpected commands: %v", d.commands)
	}
}

func TestSessionPoll(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	session.Configure(&ConnInfo{Host: "fake", Port: DefaultPort, Interval: 3600})

	waitFor(t, "first poll", session.Connected)
	waitFor(t, "poll snapshot", func() bool {
		return session.State("led_1") != nil
	})

	// The cycle must have disconnected after the snapshot.
	waitFor(t, "poll cycle close", func() bool {
		d := factory.driver(0)
		d.mutex.Lock()
		defer d.mutex.Unlock()
		return d.closed
	})

	// We stay "healthy" between polls.
	if !session.Connected() {
		t.Fatalf("expected to stay healthy between polls")
	}

	// A command mid-sleep must wake the poller, run, and disconnect again.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.SetNumber(ctx, "motor_speed", 0.5); err != nil {
		t.Fatalf("set number error: %v", err)
	}
	if factory.count() != 2 {
		t.Fatalf("expected a second poll cycle for the command, got: %d", factory.count())
	}
	d1 := factory.driver(1)
	d1.mutex.Lock()
	commands := len(d1.commands)
	closed := d1.closed
	d1.mutex.Unlock()
	if commands != 1 {
		t.Fatalf("expected 1 command in the second cycle, got: %d", commands)
	}
	if !closed {
		t.Fatalf("expected the second cycle to disconnect")
	}
}

func TestSessionConnectFailure(t *testing.T) {
	factory := &fakeFactory{
		prepare: func(d *fakeDriver) { d.failConnect = true },
	}
	session := testSession(t, factory)
	defer session.Release()

	session.Configure(&ConnInfo{Host: "fake", Port: DefaultPort})

	waitFor(t, "connect attempt", func() bool { return factory.count() >= 1 })
	if session.Connected() {
		t.Fatalf("must not be connected when connects fail")
	}

	// Queued commands must fail rather than hang.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.SetSwitch(ctx, "led_1", true); err == nil {
		t.Fatalf("expected a command error when disconnected")
	}
}
