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

//go:build race

package resources

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
)

// TestHTTPServerFlagSendRecvRace is a regression guard for the http:server:flag
// send/recv publication path. A race was once reported here with the receiver's
// SendRecv reflection (reflect.DeepEqual at engine/graph/sendrecv.go:328 and
// types.ValueOf at :334) reading the HTTPServerFlagSends.Value word and the
// `value` string while HTTPServerFlagRes.checkApply was still constructing them
// (value := "" and &HTTPServerFlagSends{Value: &value}). That was the
// consumer-side symptom of the same unsynchronized-slot bug as issue #926, not
// a separate one: the receiver only reaches the payload via Sent(), so the
// atomic publication (now traits.Sendable's atomic.Value) supplies the
// happens-before that orders every pre-Send write before the receiver's reads.
//
// Unlike the value resource (which needed snapshotAny because obj.cachedAny
// aliased a persistent field mutated in place), `value` here is a fresh
// per-call local that nothing mutates after Send, so the snapshot contract
// already holds and no flag-specific copy is needed. This test drives the real
// checkApply, the ServeHTTP-style mapResValue writer, and the exact SendRecv
// read pattern concurrently; it passes with the atomic publication and would
// fail if that publication ever regressed to an unsynchronized slot.
func TestHTTPServerFlagSendRecvRace(t *testing.T) {
	flag := &HTTPServerFlagRes{
		Key: "key",
	}
	flag.SetKind("http:server:flag")
	flag.SetName("/flag1")

	init := &engine.Init{
		Send:  engine.GenerateSendFunc(flag),
		Recv:  func() map[string]*engine.Send { return map[string]*engine.Send{} },
		Debug: false,
		Logf:  func(string, ...interface{}) {},
	}
	if err := flag.Init(init); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	ctx := context.Background()
	const iterations = 100000
	start := make(chan struct{})
	wg := &sync.WaitGroup{}

	wg.Add(3)

	// HTTP handler: update mapResValue under the mutex, exactly like
	// ServeHTTP does (it stores &val, a pointer to a handler local).
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < iterations; i++ {
			val := "value"
			flag.mutex.Lock()
			flag.mapResValue[flag] = &val
			flag.mutex.Unlock()
			runtime.Gosched()
		}
	}()

	// Sender / http:server Worker: drive the real checkApply, which
	// publishes the &value snapshot via the Sendable trait.
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < iterations; i++ {
			if _, err := flag.checkApply(ctx, true, flag); err != nil {
				t.Errorf("checkApply failed: %+v", err)
				return
			}
			runtime.Gosched()
		}
	}()

	// Downstream receiver Worker: read the published snapshot exactly as
	// engine/graph/sendrecv.go does.
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < iterations; i++ {
			st := flag.Sent()
			if st == nil {
				runtime.Gosched()
				continue
			}
			value1 := reflect.Indirect(reflect.ValueOf(st)).FieldByName("Value")
			// mirrors engine/graph/sendrecv.go:328
			_ = reflect.DeepEqual(value1.Interface(), value1.Interface())
			// mirrors engine/graph/sendrecv.go:334
			_, _ = types.ValueOf(value1)
			runtime.Gosched()
		}
	}()

	close(start)
	wg.Wait()
}
