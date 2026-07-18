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
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	apiclient "github.com/flavio-fernandes/go-aioesphomeapi"
)

// fakeDriver is a scriptable in-memory driver for testing the session logic.
type fakeDriver struct {
	mutex sync.Mutex

	failConnect bool
	connectErr  error
	connectGate chan struct{} // when set, connect blocks until closed
	entityList  []*EntityInfo
	initial     []*EntityState // states pushed right after subscribe
	fn          func(*EntityState)
	logFn       func(*LogEntry)
	logLevel    string
	doneCh      chan struct{}
	closed      bool
	closeErr    error // reported by closeReason after fail
	commands    []string
}

func (obj *fakeDriver) connect(ctx context.Context, info *ConnInfo) error {
	if obj.failConnect {
		if obj.connectErr != nil {
			return obj.connectErr
		}
		return fmt.Errorf("fake connect error")
	}
	if obj.connectGate != nil {
		select {
		case <-obj.connectGate:
		case <-ctx.Done():
			return ctx.Err()
		}
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

func (obj *fakeDriver) closeReason() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.closeErr
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

// fail simulates the connection dying on its own with the given cause. A
// deliberate close keeps the reason nil, exactly like the real driver.
func (obj *fakeDriver) fail(err error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if !obj.closed {
		obj.closed = true
		obj.closeErr = err
		close(obj.doneCh)
	}
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
			{
				Key: 4, ObjectID: "conveyor_motor", Name: "Conveyor Motor", Domain: DomainFan,
				FanSupportsSpeed: true, FanSupportedSpeedCount: 100, FanSupportsDirection: true,
			},
			{
				Key: 5, ObjectID: "status_light", Name: "Status Light", Domain: DomainLight,
				LightSupportedColorModes: []string{LightColorModeRGB},
			},
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
			t.Fatalf("normalizeLogLevel(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeLogLevel(%q) = %q, want %q", input, got, want)
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

func TestSessionConnectFailureIsLoggedAndExposed(t *testing.T) {
	wantErr := errors.New("resolver unavailable")
	factory := &fakeFactory{prepare: func(d *fakeDriver) {
		d.failConnect = true
		d.connectErr = wantErr
	}}
	session := testSession(t, factory)
	defer session.Release()

	logs := make(chan string, 1)
	info := &ConnInfo{
		Host: "broken-device.local", Port: DefaultPort,
		ConnectLogf: func(format string, v ...interface{}) {
			select {
			case logs <- fmt.Sprintf(format, v...):
			default:
			}
		},
	}
	session.Configure(info)

	select {
	case got := <-logs:
		if !strings.Contains(got, info.Addr()) || !strings.Contains(got, wantErr.Error()) || !strings.Contains(got, "retrying in") {
			t.Fatalf("connect log lacks target, cause, or retry: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for connect failure log")
	}

	waitFor(t, "last connection error", func() bool { return session.LastError() != nil })
	if !errors.Is(session.LastError(), wantErr) {
		t.Fatalf("lastError() = %v, want wrapped cause", session.LastError())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := session.WaitConnected(ctx, time.Second)
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitConnected() = %v, want wrapped cause", err)
	}
	if !strings.Contains(err.Error(), info.Addr()) {
		t.Fatalf("waitConnected() error lacks target: %v", err)
	}
}

func TestSessionCapabilitiesRejectUnsupportedCommands(t *testing.T) {
	factory := &fakeFactory{prepare: func(d *fakeDriver) {
		for _, entity := range d.entityList {
			switch entity.Domain {
			case DomainFan:
				entity.FanSupportedSpeedCount = 3
				entity.FanSupportsDirection = false
			case DomainLight:
				entity.LightSupportedColorModes = []string{"COLOR_MODE_BRIGHTNESS"}
			}
		}
	}}
	session := testSession(t, factory)
	defer session.Release()
	session.Configure(&ConnInfo{Host: "fake", Port: DefaultPort})
	waitFor(t, "connect", session.Connected)

	fan := FanCommand{State: true, Speed: 35, Direction: FanDirectionForward, HasSpeed: true, HasDirection: true}
	if err := session.ValidateFanCommand("Conveyor Motor", fan); err == nil || !strings.Contains(err.Error(), "supports 3 speed levels, got 35") {
		t.Fatalf("fan capability error = %v", err)
	}
	light := LightCommand{State: true, HasBrightness: true, HasRGB: true}
	if err := session.ValidateLightCommand("Status Light", light); err == nil || !strings.Contains(err.Error(), "does not support RGB") {
		t.Fatalf("light capability error = %v", err)
	}

	if err := session.ValidateFanCommand("Conveyor Motor", FanCommand{State: false}); err != nil {
		t.Fatalf("fan stop should not require optional capabilities: %v", err)
	}
	if err := session.ValidateLightCommand("Status Light", LightCommand{State: false}); err != nil {
		t.Fatalf("light off should not require RGB capability: %v", err)
	}
}

func TestSessionOneShotCleanupCommandUsesExplicitInfo(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	info := &ConnInfo{Host: "fake", Port: DefaultPort}
	session.Configure(info)
	waitFor(t, "connect", session.Connected)
	session.Configure(nil)
	waitFor(t, "unconfigure", func() bool { return !session.Connected() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.SetFanWithInfo(ctx, info, "Conveyor Motor", FanCommand{State: false, Speed: 35, Direction: FanDirectionForward}); err != nil {
		t.Fatalf("one-shot fan cleanup: %v", err)
	}
	if err := session.SetNumberWithInfo(ctx, info, "motor_speed", 0); err != nil {
		t.Fatalf("one-shot number cleanup: %v", err)
	}

	if factory.count() < 3 {
		t.Fatalf("expected persistent plus two one-shot connections, got: %d", factory.count())
	}
	for i, want := range []string{"fan/4/false/35/forward", "number/3/0"} {
		d := factory.driver(i + 1)
		if d == nil {
			t.Fatalf("missing one-shot driver %d", i+1)
		}
		d.mutex.Lock()
		commands := append([]string(nil), d.commands...)
		d.mutex.Unlock()
		if fmt.Sprint(commands) != fmt.Sprintf("[%s]", want) {
			t.Fatalf("driver %d commands = %v, want %s", i+1, commands, want)
		}
	}
}

func TestSessionCleanupCommandPrefersHealthySession(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	defer session.Release()

	info := &ConnInfo{Host: "fake", Port: DefaultPort}
	session.Configure(info)
	waitFor(t, "connect", session.Connected)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.SetFanForCleanup(ctx, info, "Conveyor Motor", FanCommand{State: false}); err != nil {
		t.Fatalf("shared fan cleanup: %v", err)
	}
	if err := session.SetNumberForCleanup(ctx, info, "motor_speed", 0); err != nil {
		t.Fatalf("shared number cleanup: %v", err)
	}
	if factory.count() != 1 {
		t.Fatalf("healthy cleanup opened another connection: %d", factory.count())
	}
	d := factory.driver(0)
	d.mutex.Lock()
	commands := append([]string(nil), d.commands...)
	d.mutex.Unlock()
	want := []string{"fan/4/false/0/", "number/3/0"}
	if fmt.Sprint(commands) != fmt.Sprint(want) {
		t.Fatalf("healthy cleanup commands = %v, want %v", commands, want)
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
	if err := session.SetFan(cctx, "Conveyor Motor", FanCommand{State: true, Speed: 40, Direction: FanDirectionForward, HasSpeed: true, HasDirection: true}); err != nil {
		t.Fatalf("set fan error: %v", err)
	}
	if err := session.SetLight(cctx, "Status Light", LightCommand{State: true, Brightness: 0.5, Green: 1, HasBrightness: true, HasRGB: true}); err != nil {
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
	if err := factory.driver(0).close(); err != nil {
		t.Fatalf("close first driver: %v", err)
	}
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

func TestSessionQueuedWakeDistinguishesStaleConfiguration(t *testing.T) {
	session := &Session{
		mutex:      &sync.Mutex{},
		generation: 1,
		wake:       make(chan struct{}, 1),
	}

	// A wake for the configuration already observed by the mainloop is
	// stale and must not truncate the polling snapshot-settle window.
	session.wakeup()
	if session.consumeQueuedWake(1) {
		t.Fatal("unchanged configuration requested an unnecessary restart")
	}
	if len(session.wake) != 0 {
		t.Fatal("stale configuration wake was not drained")
	}

	// A wake paired with a newer generation is a real reconfiguration and
	// must restart before connecting with obsolete parameters.
	session.mutex.Lock()
	session.generation = 2
	session.mutex.Unlock()
	session.wakeup()
	if !session.consumeQueuedWake(1) {
		t.Fatal("new configuration did not request a restart")
	}
}

// testCauseError is a typed cause used to prove that errors.As can reach the
// underlying failure through the session's wrapping.
type testCauseError struct {
	stage string
}

func (obj *testCauseError) Error() string {
	return "test cause at stage " + obj.stage
}

// asyncDisconnectSession connects a persistent session, kills its connection
// with the given cause, and waits for the disconnect. Every later driver blocks
// in connect until the returned gate closes, so the recorded cause can be
// asserted without racing a reconnect.
func asyncDisconnectSession(t *testing.T, cause error) (*Session, *ConnInfo, chan string, chan struct{}) {
	gate := make(chan struct{})
	count := 0
	factory := &fakeFactory{}
	factory.prepare = func(d *fakeDriver) {
		count++ // safe: prepare runs under the factory mutex
		if count > 1 {
			d.connectGate = gate
		}
	}
	session := testSession(t, factory)
	t.Cleanup(session.Release)

	logs := make(chan string, 10)
	info := &ConnInfo{
		Host: "fake-device.local", Port: DefaultPort,
		ConnectLogf: func(format string, v ...interface{}) {
			select {
			case logs <- fmt.Sprintf(format, v...):
			default:
			}
		},
	}
	session.Configure(info)
	waitFor(t, "connect", session.Connected)

	factory.driver(0).fail(cause)
	waitFor(t, "async disconnect", func() bool { return !session.Connected() })
	return session, info, logs, gate
}

func TestSessionPersistentAsyncDisconnectRecordsCause(t *testing.T) {
	cause := &testCauseError{stage: "read"}
	session, info, logs, _ := asyncDisconnectSession(t, cause)

	err := session.LastError()
	if err == nil {
		t.Fatalf("asynchronous disconnect did not record a cause")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is cannot reach the cause: %v", err)
	}
	target := &testCauseError{}
	if !errors.As(err, &target) || target.stage != "read" {
		t.Fatalf("errors.As cannot reach the typed cause: %v", err)
	}
	if !strings.Contains(err.Error(), info.Addr()) {
		t.Fatalf("recorded cause lacks the target address: %v", err)
	}

	select {
	case line := <-logs:
		if !strings.Contains(line, info.Addr()) || !strings.Contains(line, cause.Error()) {
			t.Fatalf("disconnect log lacks target or cause: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the disconnect log")
	}
}

func TestSessionPersistentAsyncDisconnectMarksDisconnectedBeforeLogging(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	t.Cleanup(session.Release)

	logStarted := make(chan struct{}, 1)
	releaseLog := make(chan struct{})
	var releaseOnce sync.Once
	unblockLog := func() {
		releaseOnce.Do(func() { close(releaseLog) })
	}
	defer unblockLog()

	info := &ConnInfo{
		Host: "fake-device.local", Port: DefaultPort,
		ConnectLogf: func(format string, v ...interface{}) {
			select {
			case logStarted <- struct{}{}:
			default:
			}
			<-releaseLog
		},
	}
	session.Configure(info)
	waitFor(t, "connect", session.Connected)

	factory.driver(0).fail(&testCauseError{stage: "blocked-log"})
	select {
	case <-logStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the disconnect logger")
	}

	// A caller-supplied logger may block. Health and the typed cause must be
	// published before invoking it so reconnection state never depends on
	// logging progress.
	if session.Connected() {
		t.Fatal("dead connection remained healthy while ConnectLogf was blocked")
	}
	var target *testCauseError
	if err := session.LastError(); !errors.As(err, &target) || target.stage != "blocked-log" {
		t.Fatalf("close reason was not recorded before logging: %v", err)
	}

	unblockLog()
}

func TestSessionQueueOverflowCauseIsRecognizable(t *testing.T) {
	// A slow mgmt consumer ends the connection with ErrEventQueueFull. That
	// is actionable configuration feedback, so the sentinel must stay
	// recognizable through the session's wrapping.
	session, _, _, gate := asyncDisconnectSession(t, apiclient.ErrEventQueueFull)
	defer close(gate)

	if err := session.LastError(); !errors.Is(err, apiclient.ErrEventQueueFull) {
		t.Fatalf("queue overflow is not recognizable: %v", err)
	}
}

func TestSessionAsyncCloseReasonSuppressedOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &Session{ctx: ctx}
	d := &fakeDriver{doneCh: make(chan struct{})}
	d.fail(context.Canceled)

	// A live session must surface the driver's cause.
	if got := session.asyncCloseReason(d); !errors.Is(got, context.Canceled) {
		t.Fatalf("live session hid the driver cause: %v", got)
	}

	// After shutdown the same cause is our own teardown, not a failure,
	// even when the done and ctx channels race in the mainloop select.
	cancel()
	if got := session.asyncCloseReason(d); got != nil {
		t.Fatalf("shut down session reported a failure: %v", got)
	}
}

func TestSessionDeliberateTeardownRecordsNoFailure(t *testing.T) {
	factory := &fakeFactory{}
	session := testSession(t, factory)
	logs := make(chan string, 10)
	info := &ConnInfo{
		Host: "fake", Port: DefaultPort,
		ConnectLogf: func(format string, v ...interface{}) {
			select {
			case logs <- fmt.Sprintf(format, v...):
			default:
			}
		},
	}
	session.Configure(info)
	waitFor(t, "connect", session.Connected)

	// Reconfiguration tears the connection down deliberately.
	session.Configure(nil)
	waitFor(t, "unconfigure", func() bool { return !session.Connected() })
	if err := session.LastError(); err != nil {
		t.Fatalf("reconfiguration recorded a failure: %v", err)
	}

	// Shutting down while connected is deliberate too.
	session.Configure(info)
	waitFor(t, "reconnect", session.Connected)
	session.Release()
	if err := session.LastError(); err != nil {
		t.Fatalf("session shutdown recorded a failure: %v", err)
	}
	select {
	case line := <-logs:
		t.Fatalf("deliberate teardown logged a failure: %q", line)
	default:
	}
}

func TestSessionPollAsyncDeathDuringWindow(t *testing.T) {
	cause := &testCauseError{stage: "poll-window"}
	gate := make(chan struct{})
	count := 0
	factory := &fakeFactory{}
	factory.prepare = func(d *fakeDriver) {
		count++ // safe: prepare runs under the factory mutex
		if count > 1 {
			d.connectGate = gate
		}
	}
	session := testSession(t, factory)
	defer session.Release()

	logs := make(chan string, 10)
	info := &ConnInfo{
		Host: "fake", Port: DefaultPort, Interval: 3600,
		ConnectLogf: func(format string, v ...interface{}) {
			select {
			case logs <- fmt.Sprintf(format, v...):
			default:
			}
		},
	}
	session.Configure(info)
	waitFor(t, "first poll connect", session.Connected)

	// Kill the connection inside the snapshot-settle window, before the
	// cycle's own deliberate cleanup close.
	factory.driver(0).fail(cause)

	waitFor(t, "poll cycle failure", func() bool { return !session.Connected() })
	if err := session.LastError(); !errors.Is(err, cause) || !strings.Contains(err.Error(), info.Addr()) {
		t.Fatalf("poll window death not recorded with target and cause: %v", err)
	}
	select {
	case line := <-logs:
		if !strings.Contains(line, info.Addr()) || !strings.Contains(line, cause.Error()) {
			t.Fatalf("poll disconnect log lacks target or cause: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the poll disconnect log")
	}

	// The next cycle owns the recovery. Its clean end-of-cycle close is
	// deliberate and must not record a new failure.
	close(gate)
	waitFor(t, "poll recovery", func() bool {
		return session.Connected() && session.LastError() == nil
	})
	waitFor(t, "clean cycle close", func() bool {
		d := factory.driver(1)
		if d == nil {
			return false
		}
		d.mutex.Lock()
		defer d.mutex.Unlock()
		return d.closed
	})
	if err := session.LastError(); err != nil {
		t.Fatalf("clean poll cycle recorded a failure: %v", err)
	}
	select {
	case line := <-logs:
		t.Fatalf("clean poll cycle logged a failure: %q", line)
	default:
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
