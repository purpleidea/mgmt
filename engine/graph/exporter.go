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

package graph

import (
	"context"
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"

	//"github.com/purpleidea/mgmt/pgraph"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
)

// Exporter is the main engine mechanism that sends the exported resource data
// to the World database. The code is relatively succinct, but slightly subtle.
type Exporter struct {
	// Watch specifies if we want to enable the additional watch feature. It
	// should probably be left off unless we're debugging something or using
	// weird environments where we expect someone to mess with our res data.
	Watch bool

	World engine.World

	Debug bool
	Logf  func(format string, v ...interface{})

	state map[engine.ResDelete]bool // key NOT a pointer for it to be unique
	prev  map[engine.ResDelete]pgraph.Vertex
	mutex *sync.Mutex

	// watch specific variables
	workerRunning bool
	workerWg      *sync.WaitGroup
	workerCtx     context.Context
	workerCancel  func()
}

// Init performs some initialization before first use. This is required.
func (obj *Exporter) Init() error {
	obj.state = make(map[engine.ResDelete]bool)
	obj.prev = make(map[engine.ResDelete]pgraph.Vertex)
	obj.mutex = &sync.Mutex{}

	obj.workerRunning = false
	obj.workerWg = &sync.WaitGroup{}
	obj.workerCtx, obj.workerCancel = context.WithCancel(context.Background())

	return nil
}

// Export performs the worldly export, and then stores the resource unique ID in
// our in-memory data store. Exported resources use this tracking to know when
// to run their cleanups. If this function encounters an error, it returns
// (false, err). If it does nothing it returns (true, nil). If it does work it
// return (false, nil). These return codes match how CheckApply returns. This
// may run concurrently by multiple different resources, so as a result it must
// stay thread safe.
func (obj *Exporter) Export(ctx context.Context, res engine.Res) (bool, error) {
	// As a result of running this operation in roughly the same places that
	// the usual CheckApply step would run, we end up with a more nuanced
	// and mature "exported resources" model than what was ever possible
	// with other tools. We can now "wait" (via the resource graph
	// dependencies) to run an export until an earlier resource dependency
	// step has run. We can also programmatically "un-export" a resource by
	// publishing a subsequent resource graph which either removes that
	// Export flag or the entire resource. The one downside is that
	// exporting to the database happens in multiple transactions rather
	// than a batched bolus, but this is more appropriate because we're now
	// more accurately modelling real-time systems, and this bandwidth is
	// not a significant amount anyways. Lastly, we make sure to not run the
	// purge when we ^C, since it should be safe to shutdown without killing
	// all the data we left there.

	if res.MetaParams().Noop {
		return true, nil // did nothing
	}

	exports := res.MetaParams().Export
	if len(exports) == 0 {
		return true, nil // did nothing
	}

	// It's OK to check the cache here instead of re-sending via the World
	// API and so on, because the only way the Res data would change in
	// World is if (1) someone messed with etcd, which we'd see with Watch,
	// or (2) if the Res data changed because we have a new resource graph.
	// If we have a new resource graph, then any changed elements will get
	// pruned from this state cache via the Prune method, which helps us.
	// If send/recv or any other weird resource method changes things, then
	// we also want to invalidate the state cache.
	state := true

	// TODO: This recv code is untested!
	if r, ok := res.(engine.RecvableRes); ok {
		for _, v := range r.Recv() { // map[string]*Send
			// XXX: After we read the changed value, will it persist?
			state = state && !v.Changed
		}
	}

	obj.mutex.Lock()
	for _, ptrUID := range obj.ptrUID(res) {
		b := obj.state[*ptrUID] // no need to check if exists
		state = state && b      // if any are false, it's all false
	}
	obj.mutex.Unlock()
	if state {
		return true, nil // state OK!
	}

	// XXX: Do we want to change any metaparams when we export?
	// XXX: Do we want to change any metaparams when we collect?
	b64, err := obj.resToB64(res)
	if err != nil {
		return false, err
	}

	resourceExports := []*engine.ResExport{}
	duplicates := make(map[string]struct{})
	for _, export := range exports {
		//ptrUID := engine.ResDelete{
		//	Kind: res.Kind(),
		//	Name: res.Name(),
		//	Host: export,
		//}
		if export == "*" {
			export = "" // XXX: use whatever means "all"
		}
		if _, exists := duplicates[export]; exists {
			continue
		}
		duplicates[export] = struct{}{}
		// skip this check since why race it or split the resource...
		//if stateOK := obj.state[ptrUID]; stateOK {
		//	// rare that we'd have a split of some of these from a
		//	// single resource updated and others already fine, but
		//	// might as well do the check since it's cheap...
		//	continue
		//}
		resExport := &engine.ResExport{
			Kind: res.Kind(),
			Name: res.Name(),
			Host: export,
			Data: b64, // encoded res data
		}
		resourceExports = append(resourceExports, resExport)
	}

	// The fact that we Watch the write-only-by-us values at all, is a
	// luxury that allows us to handle mischievous actors that overwrote an
	// exported value. It really isn't necessary. It's the consumers that
	// really need to watch.
	if err := obj.worker(); err != nil {
		return false, err // big error
	}

	// TODO: Do we want to log more information about where this exports to?
	obj.Logf("%s", res)
	// XXX: Add a TTL if requested
	b, err := obj.World.ResExport(ctx, resourceExports) // do it!
	if err != nil {
		return false, err
	}

	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	// NOTE: The Watch() method *must* invalidate this state if it changes.
	// This is only pertinent if we're using the luxury Watch add-ons.
	for _, ptrUID := range obj.ptrUID(res) {
		obj.state[*ptrUID] = true // state OK!
	}

	return b, nil
}

// Prune removes any exports which are no longer actively being presented in the
// resource graph. This cleans things up between graph swaps. This should NOT
// run if we're shutting down cleanly. Keep in mind that this must act on the
// new graph which is available by "Commit", not before we're ready to "Commit".
func (obj *Exporter) Prune(ctx context.Context, graph *pgraph.Graph) error {
	// mutex should be optional since this should only run when graph paused
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	// make searching faster by initially storing it all in a map
	m := make(map[engine.ResDelete]pgraph.Vertex) // key is NOT a pointer
	for _, v := range graph.Vertices() {
		res, ok := v.(engine.Res)
		if !ok { // should not happen
			return fmt.Errorf("not a Res")
		}
		for _, ptrUID := range obj.ptrUID(res) { // skips non-export things
			m[*ptrUID] = v
		}
	}

	resourceDeletes := []*engine.ResDelete{}
	for k := range obj.state {
		v, exists := m[k] // exists means it's in the graph
		prev := obj.prev[k]
		obj.prev[k] = v          // may be nil
		if exists && v != prev { // pointer compare to old vertex
			// Here we have a Res that previously existed under the
			// same kind/name/host. We need to invalidate the state
			// only if it's a different Res than the previous one!
			// If we do this erroneously, it causes extra traffic.
			obj.state[k] = false // do this only if the Res is NEW
			continue             // skip it, it's staying

		} else if exists {
			// If it exists and it's the same as it was, do nothing.
			// This is important to prevent thrashing/flapping...
			continue
		}

		// These don't exist anymore, we have to get rid of them...
		delete(obj.state, k) // it's gone!
		resourceDeletes = append(resourceDeletes, &k)
	}

	if len(resourceDeletes) == 0 {
		return nil
	}

	obj.Logf("prune: %d exports", len(resourceDeletes))
	for _, x := range resourceDeletes {
		obj.Logf("prune: %s to %s", engine.Repr(x.Kind, x.Name), x.Host)
	}
	// XXX: this function could optimize the grouping since we split the
	// list of host entries out from the kind/name since we can't have a
	// unique map key with a struct that contains a slice.
	if _, err := obj.World.ResDelete(ctx, resourceDeletes); err != nil {
		return err
	}

	return nil
}

// resToB64 is a helper to refactor out this method.
func (obj *Exporter) resToB64(res engine.Res) (string, error) {
	if r, ok := res.(engine.ExportableRes); ok {
		return r.ToB64()
	}

	return engineUtil.ResToB64(res)
}

// ptrUID is a helper for this repetitive code.
func (obj *Exporter) ptrUID(res engine.Res) []*engine.ResDelete {
	a := []*engine.ResDelete{}
	for _, export := range res.MetaParams().Export {
		if export == "*" {
			export = "" // XXX: use whatever means "all"
		}

		ptrUID := &engine.ResDelete{
			Kind: res.Kind(),
			Name: res.Name(),
			Host: export,
		}
		a = append(a, ptrUID)
	}
	return a
}

// worker is a helper to kick off the optional Watch workers.
func (obj *Exporter) worker() error {
	if !obj.Watch {
		return nil // feature is disabled
	}

	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.workerRunning {
		return nil // already running
	}

	kind := ""                                         // watch everything
	ch, err := obj.World.ResWatch(obj.workerCtx, kind) // (chan error, error)
	if err != nil {
		return err // big error
	}
	obj.workerRunning = true
	obj.workerWg.Add(1)
	go func() {
		defer func() {
			obj.mutex.Lock()
			obj.workerRunning = false
			obj.mutex.Unlock()
		}()
		defer obj.workerWg.Done()
	Loop:
		for {
			var e error
			var ok bool
			select {
			case e, ok = <-ch:
				if !ok {
					// chan closed
					break Loop
				}

			case <-obj.workerCtx.Done():
				break Loop
			}
			if e != nil {
				// something errored... shutdown coming!
			}
			// event!
			obj.mutex.Lock()
			for k := range obj.state {
				obj.state[k] = false // reset it all
			}
			obj.mutex.Unlock()
		}
	}()

	return nil
}

// Shutdown cancels any running workers and waits for them to finish.
func (obj *Exporter) Shutdown() {
	obj.workerCancel()
	obj.workerWg.Wait()
}
