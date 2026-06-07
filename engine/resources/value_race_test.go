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
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/lang/types"
)

// TestValueResSendRecvAliasRace reproduces the value-resource send/recv
// aliasing race that survives the issue #926 Sendable fix.
//
// ValueRes.CheckApply does `obj.cachedAny = obj.Any` and then publishes
// `&ValueSends{Any: obj.cachedAny}`. That makes the published snapshot's `Any`
// pointer alias obj.Any's long-lived *interface{} storage. The engine recv path
// (lang/types.Into) writes back into that *non-nil* *interface{} destination
// *in place*: in types.Into the `for kind == reflect.Ptr` loop only reallocates
// when the pointer is nil, otherwise it descends with rv.Elem() and the
// isInterface branch does rv.Set(x), mutating the existing interface{} word. A
// downstream Worker reads that same word from the previously published snapshot
// via types.ValueOf (see engine/graph/sendrecv.go). The Sendable atomic.Pointer
// only synchronizes the slot, not the aliased pointee, so without the
// snapshotAny copy this is a data race that the race detector flags.
//
// The two goroutines below mirror that exact handoff at the smallest shared
// state: the sender drives real CheckApply cycles (so the snapshotAny call site
// gates the test) with the in-place recv write emulated, and the receiver reads
// Sent() the way engine/graph/sendrecv.go does.
func TestValueResSendRecvAliasRace(t *testing.T) {
	res := &ValueRes{}
	res.SetKind("value")
	res.SetName("test-value-race")

	api := (&local.API{
		Prefix: t.TempDir(),
		Logf:   func(string, ...interface{}) {},
	}).Init()

	// A persistent recv slot so every CheckApply takes the
	// `obj.cachedAny = obj.Any` branch, exactly as the engine would after
	// receiving on `any`.
	recv := map[string]*engine.Send{
		"any": {Changed: true},
	}

	init := &engine.Init{
		Send:  engine.GenerateSendFunc(res), // routes into the Sendable trait
		Recv:  func() map[string]*engine.Send { return recv },
		Local: api,
		Logf:  func(string, ...interface{}) {},
	}
	if err := res.Init(init); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	// obj.Any must be a stable, non-nil *interface{} across cycles so the
	// recv in-place write keeps aliasing prior snapshots. This mirrors
	// types.Into instantiating the pointer once and then mutating in place.
	any := interface{}("value")
	res.Any = &any

	ctx := context.Background()
	const iterations = 100000
	start := make(chan struct{})
	wg := &sync.WaitGroup{}

	wg.Add(2)
	// Sender / value Worker: emulate the recv in-place write into obj.Any's
	// pointee, then run the real CheckApply which publishes the snapshot.
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < iterations; i++ {
			// Equivalent to lang/types.Into writing into the
			// existing non-nil *interface{} in place (rv.Set on the
			// interface word). Same value keeps CheckApply cheap,
			// and the race is on the access, not the contents.
			*res.Any = "value"
			if _, err := res.CheckApply(ctx, true); err != nil {
				t.Errorf("method CheckApply failed: %+v", err)
				return
			}
			runtime.Gosched()
		}
	}()
	// Downstream receiver Worker: read the published snapshot exactly as
	// engine/graph/sendrecv.go does (FieldByName then types.ValueOf).
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < iterations; i++ {
			st := res.Sent()
			if st == nil {
				runtime.Gosched()
				continue
			}
			rv := reflect.Indirect(reflect.ValueOf(st)).FieldByName("Any")
			_, _ = types.ValueOf(rv)
			runtime.Gosched()
		}
	}()

	close(start)
	wg.Wait()
}
