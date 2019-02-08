// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// +build !root

package resources

import (
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/event"
	"github.com/purpleidea/mgmt/util"
)

// TODO: consider providing this as a lib so that we can add tests into the
// specific _test.go file of each resource.

// makeRes is a helper function to build a res. It should only be called in
// tests, because it panics if something goes wrong.
func makeRes(kind, name string) engine.Res {
	res, err := engine.NewNamedResource(kind, name)
	if err != nil {
		panic(fmt.Sprintf("could not create resource: %+v", err))
	}
	return res
}

// Step is used for the timeline in tests.
type Step interface {
	Action() error
	Expect() error
}

type manualStep struct {
	action func() error
	expect func() error
}

func (obj *manualStep) Action() error {
	return obj.action()
}
func (obj *manualStep) Expect() error {
	return obj.expect()
}

// NewManualStep creates a new manual step with an action and an expect test.
func NewManualStep(action, expect func() error) Step {
	return &manualStep{
		action: action,
		expect: expect,
	}
}

type startupStep struct {
	ms uint
	ch chan struct{} // set by test harness
}

func (obj *startupStep) Action() error {
	select {
	case <-obj.ch: // called by Running() in Watch
	case <-time.After(time.Duration(obj.ms) * time.Millisecond):
		return fmt.Errorf("took too long to startup")
	}
	return nil
}
func (obj *startupStep) Expect() error { return nil }

// NewStartupStep waits up to this many ms for the Watch function to startup.
func NewStartupStep(ms uint) Step {
	return &startupStep{
		ms: ms,
	}
}

func TestResources1(t *testing.T) {
	type test struct { // an individual test
		name      string
		res       engine.Res // a resource
		fail      bool
		experr    error        // expected error if fail == true (nil ignores it)
		experrstr string       // expected error prefix
		timeline  []Step       // TODO: this could be a generator that keeps pushing out steps until it's done!
		expect    func() error // function to check for expected state
	}

	// helpers
	// TODO: make a series of helps to orchestrate the resources (eg: edit
	// file, wait for event w/ timeout, run command w/ timeout, etc...)
	sleep := func(ms uint) Step {
		return &manualStep{
			action: func() error {
				time.Sleep(time.Duration(ms) * time.Millisecond)
				return nil
			},
			expect: func() error { return nil },
		}
	}
	fileExpect := func(p, s string) Step { // path & string
		return &manualStep{
			action: func() error { return nil },
			expect: func() error {
				content, err := ioutil.ReadFile(p)
				if err != nil {
					return err
				}
				if string(content) != s {
					return fmt.Errorf("contents did not match in %s", p)
				}
				return nil
			},
		}
	}
	fileWrite := func(p, s string) Step { // path & string
		return &manualStep{
			action: func() error {
				// TODO: apparently using 0666 is equivalent to respecting the current umask
				const umask = 0666
				return ioutil.WriteFile(p, []byte(s), umask)
			},
			expect: func() error { return nil },
		}
	}

	testCases := []test{}
	{
		res := makeRes("file", "r1")
		r := res.(*FileRes) // if this panics, the test will panic
		p := "/tmp/whatever"
		s := "hello, world\n"
		r.Path = p
		contents := s
		r.Content = &contents

		timeline := []Step{
			NewStartupStep(3000),               // startup
			fileExpect(p, s),                   // check initial state
			fileWrite(p, "this is whatever\n"), // change state
			sleep(1000),                        // wait for converge
			fileExpect(p, s),                   // check again
		}

		testCases = append(testCases, test{
			name:     "simple res",
			res:      res,
			fail:     false,
			timeline: timeline,
			expect:   func() error { return nil },
		})
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
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			res, fail, experr, experrstr, timeline, expect := tc.res, tc.fail, tc.experr, tc.experrstr, tc.timeline, tc.expect

			t.Logf("\n\ntest #%d: Res: %+v\n", index, res)
			defer t.Logf("test #%d: done!", index)

			// run validate!
			err := res.Validate()

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not validate Res: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: validate passed, expected fail", index)
				return
			}
			if fail && experr != nil && err != experr { // test for specific error!
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected validate fail, got wrong error", index)
				t.Errorf("test #%d: got error: %+v", index, err)
				t.Errorf("test #%d: exp error: %+v", index, experr)
				return
			}
			// test for specific error string!
			if fail && experrstr != "" && !strings.HasPrefix(err.Error(), experrstr) {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected validate fail, got wrong error", index)
				t.Errorf("test #%d: got error: %s", index, err.Error())
				t.Errorf("test #%d: exp error: %s", index, experrstr)
				return
			}
			if fail && err != nil {
				t.Logf("test #%d: err: %+v", index, err)
			}

			readyChan := make(chan struct{})
			eventChan := make(chan struct{})
			eventsChan := make(chan *event.Msg)
			debug := testing.Verbose() // set via the -test.v flag to `go test`
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d: Res: ", index)+format, v...)
			}
			init := &engine.Init{
				Running: func() error {
					close(readyChan)
					select { // this always sends one!
					case eventChan <- struct{}{}:

					}
					return nil
				},
				// Watch runs this to send a changed event.
				Event: func() error {
					select {
					case eventChan <- struct{}{}:

					}
					return nil
				},

				// Watch listens on this for close/pause events.
				Events: eventsChan,
				Debug:  debug,
				Logf:   logf,

				// unused
				Dirty: func() {},
				Recv: func() map[string]*engine.Send {
					return map[string]*engine.Send{}
				},
			}

			// run init
			t.Logf("test #%d: running Init", index)
			err = res.Init(init)
			closeFn := func() {
				// run close (we don't ever expect an error on close!)
				t.Logf("test #%d: running Close", index)
				if err := res.Close(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not close Res: %+v", index, err)
					//return
				}
			}

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init Res: %+v", index, err)
				return
			}
			if fail && err == nil {
				closeFn() // close if Init didn't fail
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: init passed, expected fail", index)
				return
			}
			if fail && experr != nil && err != experr { // test for specific error!
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected init fail, got wrong error", index)
				t.Errorf("test #%d: got error: %+v", index, err)
				t.Errorf("test #%d: exp error: %+v", index, experr)
				return
			}
			// test for specific error string!
			if fail && experrstr != "" && !strings.HasPrefix(err.Error(), experrstr) {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected init fail, got wrong error", index)
				t.Errorf("test #%d: got error: %s", index, err.Error())
				t.Errorf("test #%d: exp error: %s", index, experrstr)
				return
			}
			if fail && err != nil {
				t.Logf("test #%d: err: %+v", index, err)
			}
			defer closeFn()

			// run watch
			wg := &sync.WaitGroup{}
			defer wg.Wait() // if we return early
			wg.Add(1)
			go func() {
				defer wg.Done()
				t.Logf("test #%d: running Watch", index)
				if err := res.Watch(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: Watch failed: %s", index, err.Error())
				}
				close(eventChan) // done with this part
			}()

			// TODO: can we block here if the test fails early?
			select {
			case <-readyChan: // called by Running() in Watch
			}
			wg.Add(1)
			go func() { // run timeline
				t.Logf("test #%d: executing timeline", index)
				defer wg.Done()
				for ix, step := range timeline {

					// magic setting of important value...
					if s, ok := step.(*startupStep); ok {
						s.ch = readyChan
					}

					t.Logf("test #%d: step(%d)...", index, ix)
					if err := step.Action(); err != nil {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: step(%d) action failed: %s", index, ix, err.Error())
						break
					}
					if err := step.Expect(); err != nil {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: step(%d) expect failed: %s", index, ix, err.Error())
						break
					}
				}
				t.Logf("test #%d: shutting down Watch", index)
				close(eventsChan) // send Watch shutdown command
			}()
		Loop:
			for {
				select {
				case _, ok := <-eventChan: // from Watch()
					if !ok {
						//t.Logf("test #%d: break!", index)
						break Loop
					}
				}

				t.Logf("test #%d: running CheckApply", index)
				checkOK, err := res.CheckApply(true) // no noop!
				if err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: CheckApply failed: %s", index, err.Error())
					return
				}
				_ = checkOK // TODO: do we look at this?

			}

			t.Logf("test #%d: waiting for shutdown", index)
			wg.Wait()

			if err := expect(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expect failed: %s", index, err.Error())
				return
			}

			// all done!
		})
	}
}
