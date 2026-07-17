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

package esphome

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// dialTimeout is how long we wait for the tcp connect and handshake.
	dialTimeout = 10 * time.Second

	// pollSettle is how long a poll cycle waits after connecting for the
	// initial state snapshot burst to arrive before disconnecting again.
	pollSettle = 1 * time.Second

	// maxBackoff is the maximum delay between reconnect attempts when we
	// hold a persistent connection.
	maxBackoff = 60 * time.Second
)

// pendingCmd is one queued command waiting for a driver to run it against.
type pendingCmd struct {
	fn   func(driver) error
	done chan error // buffered so the mainloop never blocks on it
}

// Session is the shared, per-endpoint connection manager. All consumers of one
// endpoint uid (functions and resources alike) share a single session, and as a
// result, a single real connection to the device. Get one with SessionReserve
// and free it with Release. It starts out unconfigured, knowing nothing but its
// uid, and does not connect anywhere until Configure hands it the connection
// info that the esphome:endpoint resource published.
type Session struct {
	uid   string
	count int // reservation refcount, guarded by the pool sessionMutex

	// mutex guards all of the fields below.
	mutex      *sync.Mutex
	info       *ConnInfo // desired config, nil means unconfigured
	generation uint64    // bumped whenever info changes
	states     map[string]*State
	index      map[uint32][]string        // entity key -> addressable identifiers
	connected  bool                       // do we consider the device healthy?
	lastAlive  time.Time                  // last moment the device was under our control
	lastOutage time.Duration              // duration of the most recent outage
	outageID   uint64                     // increments whenever lastOutage is set
	failures   uint32                     // consecutive connect failures
	pending    []*pendingCmd              // queued commands
	notify     map[chan struct{}]struct{} // one chan (unique ptr) per watch

	wake chan struct{} // buffered (1) wake signal for the mainloop

	newDriver func() driver

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

// newSession builds a session and starts its mainloop. It must only be called
// from SessionReserve.
func newSession(uid string) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	obj := &Session{
		uid:       uid,
		mutex:     &sync.Mutex{},
		states:    make(map[string]*State),
		index:     make(map[uint32][]string),
		notify:    make(map[chan struct{}]struct{}),
		wake:      make(chan struct{}, 1),
		newDriver: newDriverFunc,
		ctx:       ctx,
		cancel:    cancel,
		wg:        &sync.WaitGroup{},
	}
	obj.wg.Add(1)
	go obj.mainloop()
	return obj
}

// Configure tells the session which device to talk to and how. It is
// idempotent, and safe to call from every consumer whenever their bridge watch
// fires: the session dedupes identical values. Passing nil (the endpoint was
// unpublished) disconnects and clears the state cache. Passing a different
// value reconnects with the new params.
func (obj *Session) Configure(info *ConnInfo) {
	obj.mutex.Lock()
	if obj.info.Cmp(info) == nil { // handles the nil == nil case too
		obj.mutex.Unlock()
		return
	}
	obj.info = info
	obj.generation++
	obj.mutex.Unlock()
	obj.wakeup()
}

// Watch returns a channel which sends a single startup event, and after that,
// one event whenever anything observable about the session changes: an entity
// state update, a connect, or a disconnect. The channel closes when the session
// shuts down. Cancel the input ctx to unsubscribe.
func (obj *Session) Watch(ctx context.Context) (chan struct{}, error) {
	obj.mutex.Lock()
	notifyCh := make(chan struct{}, 1) // so we can async send
	obj.notify[notifyCh] = struct{}{}  // add (while within the mutex)
	notifyCh <- struct{}{}             // startup signal, send one!
	obj.mutex.Unlock()

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		defer func() { // cleanup
			obj.mutex.Lock()
			defer obj.mutex.Unlock()
			delete(obj.notify, notifyCh) // free memory (in mutex)
		}()
		for {
			select {
			case _, ok := <-notifyCh:
				if !ok {
					// programming error
					panic("unexpected channel closure")
				}
				// recv

			case <-ctx.Done():
				return // we exit

			case <-obj.ctx.Done():
				return // session shutdown
			}

			select {
			case ch <- struct{}{}:
				// send

			case <-ctx.Done():
				return // we exit

			case <-obj.ctx.Done():
				return // session shutdown
			}
		}
	}()

	return ch, nil
}

// State returns the last-known state of the entity with the given identifier,
// or nil if we don't know anything about it (yet). An identifier can be the
// entity's exact name, or its legacy object_id when the device provides one.
// Callers must treat the returned value as read-only.
func (obj *Session) State(identifier string) *State {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.states[identifier] // nil if it doesn't exist
}

// Connected reports whether we consider the device healthy. With a persistent
// connection this means the connection is currently up. In polling mode it
// means the most recent poll cycle succeeded.
func (obj *Session) Connected() bool {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.connected
}

// WaitConnected waits up to the given duration for the device to be healthy. It
// exists so that resources whose CheckApply needs the device can tolerate the
// asynchronous connection startup instead of erroring immediately.
func (obj *Session) WaitConnected(ctx context.Context, timeout time.Duration) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ch, err := obj.Watch(wctx)
	if err != nil {
		return err
	}
	for {
		if obj.Connected() {
			return nil
		}
		select {
		case _, ok := <-ch:
			if !ok {
				return fmt.Errorf("session closed")
			}
		case <-wctx.Done():
			if obj.Connected() { // last chance
				return nil
			}
			return fmt.Errorf("timed out waiting for device: %v", wctx.Err())
		}
	}
}

// LastOutage returns the duration of the most recent completed outage, and an
// id which increments each time a new outage completes. An outage is the time
// between two consecutive moments that the device was under our control, so
// with a persistent connection it's the disconnected gap, and in polling mode
// it's the gap between successful polls, which is normally about one interval.
// Consumers that implement a safety threshold should remember the last id they
// acted on, so that each outage is only handled once.
func (obj *Session) LastOutage() (time.Duration, uint64) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.lastOutage, obj.outageID
}

// SetSwitch commands a switch entity by exact name or legacy object_id. With a
// persistent connection it runs immediately. In polling mode it wakes the
// poller to connect right away, and returns once the command has actually been
// sent.
func (obj *Session) SetSwitch(ctx context.Context, identifier string, on bool) error {
	return obj.run(ctx, identifier, func(d driver, key uint32) error {
		return d.setSwitch(key, on)
	})
}

// SetNumber commands a number entity by exact name or legacy object_id. With a
// persistent connection it runs immediately. In polling mode it wakes the
// poller to connect right away, and returns once the command has actually been
// sent.
func (obj *Session) SetNumber(ctx context.Context, identifier string, value float64) error {
	return obj.run(ctx, identifier, func(d driver, key uint32) error {
		return d.setNumber(key, value)
	})
}

// SetNumberWithInfo commands a number entity through a one-shot connection
// using the supplied connection info. It is intended for shutdown safety
// cleanup, where the shared endpoint may already be unpublished.
func (obj *Session) SetNumberWithInfo(ctx context.Context, info *ConnInfo, identifier string, value float64) error {
	return obj.runOnce(ctx, info, identifier, func(d driver, key uint32) error {
		return d.setNumber(key, value)
	})
}

// PressButton presses a button entity by exact name or legacy object_id. With a
// persistent connection it runs immediately. In polling mode it wakes the
// poller to connect right away, and returns once the command has actually been
// sent.
func (obj *Session) PressButton(ctx context.Context, identifier string) error {
	return obj.run(ctx, identifier, func(d driver, key uint32) error {
		return d.pressButton(key)
	})
}

// SetFan commands a fan entity by exact name or legacy object_id.
func (obj *Session) SetFan(ctx context.Context, identifier string, command FanCommand) error {
	return obj.run(ctx, identifier, func(d driver, key uint32) error {
		return d.setFan(key, command)
	})
}

// SetFanWithInfo commands a fan entity through a one-shot connection using the
// supplied connection info. It is intended for shutdown safety cleanup, where
// the shared endpoint may already be unpublished.
func (obj *Session) SetFanWithInfo(ctx context.Context, info *ConnInfo, identifier string, command FanCommand) error {
	return obj.runOnce(ctx, info, identifier, func(d driver, key uint32) error {
		return d.setFan(key, command)
	})
}

// SetLight commands an RGB light entity by exact name or legacy object_id.
func (obj *Session) SetLight(ctx context.Context, identifier string, command LightCommand) error {
	return obj.run(ctx, identifier, func(d driver, key uint32) error {
		return d.setLight(key, command)
	})
}

func (obj *Session) runOnce(ctx context.Context, info *ConnInfo, identifier string, fn func(driver, uint32) error) error {
	if info == nil {
		return fmt.Errorf("endpoint `%s` is not configured", obj.uid)
	}
	d := obj.newDriver()
	if err := d.connect(ctx, info); err != nil {
		return err
	}
	defer d.close()
	entities, err := d.entities()
	if err != nil {
		return err
	}
	key, err := lookupEntityKey(entities, identifier)
	if err != nil {
		return err
	}
	return fn(d, key)
}

// run queues one command for the mainloop to execute against a live driver,
// wakes the mainloop, and waits for the result. Queueing even when we hold a
// persistent connection keeps a single code path, and means commands never race
// with a teardown.
func (obj *Session) run(ctx context.Context, identifier string, fn func(driver, uint32) error) error {
	obj.mutex.Lock()
	if obj.info == nil {
		obj.mutex.Unlock()
		return fmt.Errorf("endpoint `%s` is not configured", obj.uid)
	}
	p := &pendingCmd{
		fn: func(d driver) error {
			key, err := obj.lookup(identifier)
			if err != nil {
				return err
			}
			return fn(d, key)
		},
		done: make(chan error, 1),
	}
	obj.pending = append(obj.pending, p)
	obj.mutex.Unlock()
	obj.wakeup()

	select {
	case err := <-p.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-obj.ctx.Done():
		return fmt.Errorf("session closed")
	}
}

// lookup resolves an entity name or legacy object_id to the numeric entity key
// of the current connection.
func (obj *Session) lookup(identifier string) (uint32, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	for key, identifiers := range obj.index {
		for _, id := range identifiers {
			if id == identifier {
				return key, nil
			}
		}
	}
	return 0, fmt.Errorf("unknown entity: `%s`", identifier)
}

func lookupEntityKey(entities []*EntityInfo, identifier string) (uint32, error) {
	for _, entity := range entities {
		if entity.Name == identifier || entity.ObjectID == identifier {
			return entity.Key, nil
		}
	}
	return 0, fmt.Errorf("unknown entity: `%s`", identifier)
}

// wakeup pokes the mainloop. The wake chan is buffered (1) so if it's full then
// a wakeup is already pending and dropping this one coalesces them.
func (obj *Session) wakeup() {
	select {
	case obj.wake <- struct{}{}:
	default:
	}
}

// notifySend sends an event to every watcher. It must be called while holding
// the mutex.
func (obj *Session) notifySend() {
	for ch := range obj.notify {
		select {
		case ch <- struct{}{}: // must be async and not block forever
			// send

		default:
			// The notify chan is buffered (1) so if it's full then
			// a notification is already pending and dropping this
			// one coalesces them, which is exactly what we want.
		}
	}
}

// failPending errors out every queued command. It must be called while holding
// the mutex.
func (obj *Session) failPending(err error) {
	for _, p := range obj.pending {
		p.done <- err // buffered
	}
	obj.pending = []*pendingCmd{}
}

// flushPending runs every queued command against the given driver.
func (obj *Session) flushPending(d driver) {
	obj.mutex.Lock()
	pending := obj.pending
	obj.pending = []*pendingCmd{}
	obj.mutex.Unlock()

	for _, p := range pending {
		p.done <- p.fn(d) // buffered
	}
}

// handleState stores one state update in the cache and notifies the watchers if
// anything changed.
func (obj *Session) handleState(es *EntityState) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	identifiers, exists := obj.index[es.Key]
	if !exists {
		return // unknown entity, ignore
	}
	s := es.State // copy
	changed := false
	for _, identifier := range identifiers {
		if old, exists := obj.states[identifier]; !exists || *old != s {
			changed = true
			break
		}
	}
	if !changed {
		return // no change
	}
	for _, identifier := range identifiers {
		obj.states[identifier] = &s
	}
	obj.notifySend()
}

// markConnected records a successful connection, computes the outage that just
// ended, stores the entity index, and notifies the watchers.
func (obj *Session) markConnected(entities []*EntityInfo) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	now := time.Now()
	if !obj.lastAlive.IsZero() {
		obj.lastOutage = now.Sub(obj.lastAlive)
		obj.outageID++
	}
	obj.lastAlive = now
	obj.failures = 0

	obj.index = make(map[uint32][]string)
	for _, e := range entities {
		identifiers := []string{}
		if e.Name != "" {
			identifiers = append(identifiers, e.Name)
		}
		if e.ObjectID != "" && e.ObjectID != e.Name {
			identifiers = append(identifiers, e.ObjectID)
		}
		if len(identifiers) > 0 {
			obj.index[e.Key] = identifiers
		}
	}

	if !obj.connected {
		obj.connected = true
		obj.notifySend()
	}
}

// markDisconnected records that we no longer control the device, and notifies
// the watchers if that's a change. In polling mode a clean end of cycle is not
// a disconnect, so this is only used for failures and teardowns.
func (obj *Session) markDisconnected() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.connected {
		obj.lastAlive = time.Now()
		obj.connected = false
		obj.notifySend()
	}
}

// clearStates drops the whole state cache. This runs when the session becomes
// unconfigured so that consumers see zero values again.
func (obj *Session) clearStates() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	changed := len(obj.states) > 0 || obj.connected
	obj.states = make(map[string]*State)
	obj.index = make(map[uint32][]string)
	if obj.connected {
		obj.lastAlive = time.Now()
		obj.connected = false
	}
	if changed {
		obj.notifySend()
	}
}

// snapshot returns the current config and generation.
func (obj *Session) snapshot() (*ConnInfo, uint64) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.info, obj.generation
}

// stale is true when the config changed since the given generation.
func (obj *Session) stale(generation uint64) bool {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.generation != generation
}

// consumeQueuedWake drains one wake queued before a connection cycle starts.
// It returns true only when that wake represents a real configuration change.
// Command wakes can be coalesced because the cycle flushes pending commands.
func (obj *Session) consumeQueuedWake(generation uint64) bool {
	select {
	case <-obj.wake:
		return obj.stale(generation)
	default:
		return false
	}
}

// sleep waits for the given duration, but returns early on a wakeup or on
// shutdown. It returns an error only on shutdown.
func (obj *Session) sleep(d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-obj.wake:
		return nil
	case <-obj.ctx.Done():
		return obj.ctx.Err()
	}
}

// backoff returns how long to wait before the next connect attempt, and
// increments the consecutive failure count.
func (obj *Session) backoff() time.Duration {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	d := time.Duration(1<<min(obj.failures, 6)) * time.Second
	obj.failures++
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// mainloop is the single goroutine which owns the device connection for the
// whole life of the session.
func (obj *Session) mainloop() {
	defer obj.wg.Done()
	defer func() {
		obj.mutex.Lock()
		defer obj.mutex.Unlock()
		obj.failPending(fmt.Errorf("session closed"))
	}()

	for {
		select {
		case <-obj.ctx.Done():
			return
		default:
		}

		info, generation := obj.snapshot()
		if info == nil {
			obj.clearStates()
			obj.mutex.Lock()
			obj.failPending(fmt.Errorf("endpoint `%s` is not configured", obj.uid))
			obj.mutex.Unlock()
			select {
			case <-obj.wake:
				continue
			case <-obj.ctx.Done():
				return
			}
		}

		// Configure can race with mainloop startup: the loop can observe the
		// new info before it consumes the corresponding wake signal. Do not
		// let that stale signal cut a polling snapshot-settle window short.
		// A real reconfiguration changes the generation and restarts now;
		// command wakes are safe to coalesce because pending commands flush on
		// the connection below.
		if obj.consumeQueuedWake(generation) {
			continue
		}

		if info.Interval > 0 {
			obj.pollCycle(info, generation)
			continue
		}
		obj.persistent(info, generation)
	}
}

// connect builds a fresh driver, connects it, and returns it along with its
// entity list. On error the driver is already closed.
func (obj *Session) connect(info *ConnInfo) (driver, error) {
	d := obj.newDriver()
	if err := d.connect(obj.ctx, info); err != nil {
		return nil, err
	}
	entities, err := d.entities()
	if err != nil {
		d.close()
		return nil, err
	}
	obj.markConnected(entities) // index must exist before states arrive
	if err := d.subscribe(obj.handleState); err != nil {
		d.close()
		obj.markDisconnected()
		return nil, err
	}
	if info.LogLevel != "" && info.Logf != nil {
		if err := d.subscribeLogs(info.LogLevel, info.Logf); err != nil {
			d.close()
			obj.markDisconnected()
			return nil, err
		}
	}
	return d, nil
}

// persistent holds one connection open and lets the device push state changes
// to us natively. It returns when the connection drops, the config changes, or
// the session shuts down.
func (obj *Session) persistent(info *ConnInfo, generation uint64) {
	d, err := obj.connect(info)
	if err != nil {
		obj.mutex.Lock()
		obj.failPending(err)
		obj.mutex.Unlock()
		obj.sleep(obj.backoff()) // returns early on wakeup/shutdown
		return
	}
	defer d.close()

	obj.flushPending(d) // anything that queued while we were down

	for {
		select {
		case <-d.done(): // connection dropped
			obj.markDisconnected()
			return

		case <-obj.wake:
			if obj.stale(generation) {
				obj.markDisconnected() // teardown for reconfigure
				return
			}
			obj.flushPending(d)

		case <-obj.ctx.Done():
			obj.markDisconnected()
			return
		}
	}
}

// pollCycle runs a single poll: connect, wait for the state snapshot, flush any
// queued commands, disconnect, and then sleep for the interval. Commands wake
// it up early so they don't have to wait for the next cycle.
func (obj *Session) pollCycle(info *ConnInfo, generation uint64) {
	d, err := obj.connect(info)
	if err != nil {
		obj.markDisconnected()
		obj.mutex.Lock()
		obj.failPending(err)
		obj.mutex.Unlock()
		obj.sleep(obj.backoff())
		return
	}

	// Wait briefly for the initial snapshot burst to land in the cache.
	obj.sleep(pollSettle)

	obj.flushPending(d)
	d.close()

	if obj.stale(generation) {
		return // reconfigure right away
	}

	// Note that we stay "connected" (healthy) between successful polls.
	// Sleep until the next poll. A queued command or a config change wakes
	// us early, and the next mainloop cycle handles it right away.
	obj.sleep(time.Duration(info.Interval) * time.Second)
}
