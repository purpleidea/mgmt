// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package dage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

type testFunc struct {
	Name string
	Type *types.Type
	Func func(types.Value) (types.Value, error)
	Meta *meta

	value types.Value
	init  *interfaces.Init
}

func (obj *testFunc) String() string { return obj.Name }

func (obj *testFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: should this be something we specify here?
		Sig:  obj.Type,
		Err:  obj.Validate(),
	}
}

func (obj *testFunc) Validate() error {
	if obj.Meta == nil {
		return fmt.Errorf("test case error: did you add the vertex to the vertices list?")
	}
	return nil
}

func (obj *testFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

func (obj *testFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	defer obj.init.Logf("stream closed")
	obj.init.Logf("stream startup")

	// make some placeholder value because obj.value is nil
	constValue, err := types.ValueOfGolang("hello")
	if err != nil {
		return err // unlikely
	}

	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Logf("stream input closed")
				obj.init.Input = nil // don't get two closes
				// already sent one value, so we can shutdown
				if obj.value != nil {
					return nil // can't output any more
				}

				obj.value = constValue
			} else {
				obj.init.Logf("stream got input type(%T) value: (%+v)", input, input)
				if obj.Func == nil {
					obj.value = constValue
				}

				if obj.Func != nil {
					//obj.init.Logf("running internal function...")
					v, err := obj.Func(input) // run me!
					if err != nil {
						return err
					}
					obj.value = v
				}
			}

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.value: // send anything
			// add some monitoring...
			obj.Meta.wg.Add(1)
			go func() {
				// no need for lock here
				defer obj.Meta.wg.Done()
				if obj.Meta.debug {
					obj.Meta.logf("sending an internal event!")
				}

				select {
				case obj.Meta.Events[obj.Name] <- struct{}{}:
				case <-obj.Meta.ctx.Done():
				}
			}()

		case <-ctx.Done():
			return nil
		}
	}
}

type meta struct {
	EventCount int
	Event      chan struct{}
	Events     map[string]chan struct{}

	ctx   context.Context
	wg    *sync.WaitGroup
	mutex *sync.Mutex

	debug bool
	logf  func(format string, v ...interface{})
}

func (obj *meta) Lock()   { obj.mutex.Lock() }
func (obj *meta) Unlock() { obj.mutex.Unlock() }

type dageTestOp func(*Engine, *meta) error

func TestDageTable(t *testing.T) {

	type test struct { // an individual test
		name     string
		vertices []interfaces.Func
		actions  []dageTestOp
	}
	testCases := []test{}
	{
		testCases = append(testCases, test{
			name:     "empty graph",
			vertices: []interfaces.Func{},
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					time.Sleep(1 * time.Second) // XXX: unfortunate
					defer engine.Unlock()
					return nil
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					meta.Lock()
					defer meta.Unlock()
					// We don't expect an empty graph to send events.
					if meta.EventCount != 0 {
						return fmt.Errorf("got too many stream events")
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testFunc{Name: "f1", Type: types.NewType("func() str")}

		testCases = append(testCases, test{
			name:     "simple add vertex",
			vertices: []interfaces.Func{f1}, // so the test engine can pass in debug/observability handles
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.AddVertex(f1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					meta.Lock()
					defer meta.Unlock()
					if meta.EventCount < 1 {
						return fmt.Errorf("didn't get any stream events")
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testFunc{Name: "f1", Type: types.NewType("func() str")}
		// e1 arg name must match incoming edge to it
		f2 := &testFunc{Name: "f2", Type: types.NewType("func(e1 str) str")}
		e1 := testEdge("e1")

		testCases = append(testCases, test{
			name:     "simple add edge",
			vertices: []interfaces.Func{f1, f2},
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.AddVertex(f1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					engine.Lock()
					defer engine.Unlock()
					// This newly added node should get a notification after it starts.
					return engine.AddEdge(f1, f2, e1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					meta.Lock()
					defer meta.Unlock()
					if meta.EventCount < 2 {
						return fmt.Errorf("didn't get enough stream events")
					}
					return nil
				},
			},
		})
	}
	{
		// diamond
		f1 := &testFunc{Name: "f1", Type: types.NewType("func() str")}
		f2 := &testFunc{Name: "f2", Type: types.NewType("func(e1 str) str")}
		f3 := &testFunc{Name: "f3", Type: types.NewType("func(e2 str) str")}
		f4 := &testFunc{Name: "f4", Type: types.NewType("func(e3 str, e4 str) str")}
		e1 := testEdge("e1")
		e2 := testEdge("e2")
		e3 := testEdge("e3")
		e4 := testEdge("e4")

		testCases = append(testCases, test{
			name:     "simple add multiple edges",
			vertices: []interfaces.Func{f1, f2, f3, f4},
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.AddVertex(f1)
				},
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					if err := engine.AddEdge(f1, f2, e1); err != nil {
						return err
					}
					if err := engine.AddEdge(f1, f3, e2); err != nil {
						return err
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					if err := engine.AddEdge(f2, f4, e3); err != nil {
						return err
					}
					if err := engine.AddEdge(f3, f4, e4); err != nil {
						return err
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					//meta.Lock()
					//defer meta.Unlock()
					num := 1
					for {
						if num == 0 {
							break
						}
						select {
						case _, ok := <-meta.Event:
							if !ok {
								return fmt.Errorf("unexpectedly channel close")
							}
							num--
							if meta.debug {
								meta.logf("got an event!")
							}
						case <-meta.ctx.Done():
							return meta.ctx.Err()
						}
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					meta.Lock()
					defer meta.Unlock()
					if meta.EventCount < 1 {
						return fmt.Errorf("didn't get enough stream events")
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					//meta.Lock()
					//defer meta.Unlock()
					num := 1
					for {
						if num == 0 {
							break
						}
						bt := util.BlockedTimer{Seconds: 2}
						defer bt.Cancel()
						bt.Printf("waiting for f4...\n")
						select {
						case _, ok := <-meta.Events["f4"]:
							bt.Cancel()
							if !ok {
								return fmt.Errorf("unexpectedly channel close")
							}
							num--
							if meta.debug {
								meta.logf("got an event from f4!")
							}
						case <-meta.ctx.Done():
							return meta.ctx.Err()
						}
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testFunc{Name: "f1", Type: types.NewType("func() str")}

		testCases = append(testCases, test{
			name:     "simple add/delete vertex",
			vertices: []interfaces.Func{f1},
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.AddVertex(f1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					meta.Lock()
					defer meta.Unlock()
					if meta.EventCount < 1 {
						return fmt.Errorf("didn't get enough stream events")
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()

					//meta.Lock()
					//defer meta.Unlock()
					if meta.debug {
						meta.logf("about to delete vertex f1!")
						defer meta.logf("done deleting vertex f1!")
					}

					return engine.DeleteVertex(f1)
				},
			},
		})
	}
	{
		f1 := &testFunc{Name: "f1", Type: types.NewType("func() str")}
		// e1 arg name must match incoming edge to it
		f2 := &testFunc{Name: "f2", Type: types.NewType("func(e1 str) str")}
		e1 := testEdge("e1")

		testCases = append(testCases, test{
			name:     "simple add/delete edge",
			vertices: []interfaces.Func{f1, f2},
			actions: []dageTestOp{
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.AddVertex(f1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					engine.Lock()
					defer engine.Unlock()
					// This newly added node should get a notification after it starts.
					return engine.AddEdge(f1, f2, e1)
				},
				func(engine *Engine, meta *meta) error {
					time.Sleep(1 * time.Second) // XXX: unfortunate
					meta.Lock()
					defer meta.Unlock()
					if meta.EventCount < 2 {
						return fmt.Errorf("didn't get enough stream events")
					}
					return nil
				},
				func(engine *Engine, meta *meta) error {
					engine.Lock()
					defer engine.Unlock()
					return engine.DeleteEdge(e1)
				},
			},
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple txn" {
		//	continue
		//}

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, vertices, actions := tc.name, tc.vertices, tc.actions

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			//logf := func(format string, v ...interface{}) {
			//	t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			//}

			//now := time.Now()

			wg := &sync.WaitGroup{}
			defer wg.Wait() // defer is correct b/c we're in a func!

			min := 5 * time.Second // approx min time needed for the test
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deadline, ok := t.Deadline(); ok {
				d := deadline.Add(-min)
				//t.Logf("  now: %+v", now)
				//t.Logf("    d: %+v", d)
				newCtx, cancel := context.WithDeadline(ctx, d)
				ctx = newCtx
				defer cancel()
			}

			debug := testing.Verbose() // set via the -test.v flag to `go test`

			meta := &meta{
				Event:  make(chan struct{}),
				Events: make(map[string]chan struct{}),

				ctx:   ctx,
				wg:    &sync.WaitGroup{},
				mutex: &sync.Mutex{},

				debug: debug,
				logf: func(format string, v ...interface{}) {
					// safe Logf in case f.String contains %? chars...
					s := fmt.Sprintf(format, v...)
					t.Logf("%s", s)
				},
			}
			defer meta.wg.Wait()

			for _, f := range vertices {
				testFunc, ok := f.(*testFunc)
				if !ok {
					t.Errorf("bad test function: %+v", f)
					return
				}
				meta.Events[testFunc.Name] = make(chan struct{})
				testFunc.Meta = meta // add the handle
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					t.Logf("cancelling test...")
				}
			}()

			engine := &Engine{
				Name: "dage",

				Debug: debug,
				Logf:  t.Logf,
			}

			if err := engine.Setup(); err != nil {
				t.Errorf("could not setup engine: %+v", err)
				return
			}
			defer engine.Cleanup()

			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := engine.Run(ctx); err != nil {
					t.Errorf("error while running engine: %+v", err)
					return
				}
				t.Logf("engine shutdown cleanly...")
			}()

			<-engine.Started() // wait for startup (will not block forever)

			wg.Add(1)
			go func() {
				defer wg.Done()
				ch := engine.Stream()
				for {
					select {
					case err, ok := <-ch: // channel must close to shutdown
						if !ok {
							return
						}
						meta.Lock()
						meta.EventCount++
						meta.Unlock()
						meta.wg.Add(1)
						go func() {
							// no need for lock here
							defer meta.wg.Done()
							if meta.debug {
								meta.logf("sending an event!")
							}
							select {
							case meta.Event <- struct{}{}:
							case <-meta.ctx.Done():
							}
						}()
						if err != nil {
							t.Errorf("graph error event: %v", err)
							continue
						}
						t.Logf("graph stream event!")
					}
				}
			}()

			// Run a list of actions. Any error kills it all.
			t.Logf("starting actions...")
			for i, action := range actions {
				t.Logf("running action %d...", i+1)
				if err := action(engine, meta); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: action #%d failed with: %+v", index, i, err)
					break // so that cancel runs
				}
			}

			t.Logf("test done...")
			cancel()
		})
	}

	if testing.Short() {
		t.Skip("skipping all tests...")
	}
}
