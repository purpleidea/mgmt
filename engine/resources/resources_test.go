// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
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

type changedStep struct {
	ms     uint
	expect bool      // what checkOK value we're expecting
	ch     chan bool // set by test harness, filled with checkOK values
}

func (obj *changedStep) Action() error {
	select {
	case checkOK, ok := <-obj.ch: // from CheckApply() in test Process loop
		if !ok {
			return fmt.Errorf("channel closed unexpectedly")
		}
		if checkOK != obj.expect {
			return fmt.Errorf("got unexpected checkOK value of: %t", checkOK)
		}
	case <-time.After(time.Duration(obj.ms) * time.Millisecond):
		return fmt.Errorf("took too long to startup")
	}
	return nil
}
func (obj *changedStep) Expect() error { return nil }

// NewChangedStep waits up to this many ms for a CheckApply action to occur. Watch function to startup.
func NewChangedStep(ms uint, expect bool) Step {
	return &changedStep{
		ms:     ms,
		expect: expect,
	}
}

type clearChangedStep struct {
	ms uint
	ch chan bool // set by test harness, filled with checkOK values
}

func (obj *clearChangedStep) Action() error {
	// read all pending events...
	for {
		select {
		case _, ok := <-obj.ch: // from CheckApply() in test Process loop
			if !ok {
				return fmt.Errorf("channel closed unexpectedly")
			}
		case <-time.After(time.Duration(obj.ms) * time.Millisecond):
			return nil // done waiting
		}
	}
}
func (obj *clearChangedStep) Expect() error { return nil }

// NewClearChangedStep waits up to this many ms for additional CheckApply
// actions to occur, and flushes them all so that a future NewChangedStep won't
// see unwanted events.
func NewClearChangedStep(ms uint) Step {
	return &clearChangedStep{
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
		startup   func() error // function to run as startup
		cleanup   func() error // function to run as cleanup
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
		r := makeRes("file", "r1")
		res := r.(*FileRes) // if this panics, the test will panic
		p := "/tmp/whatever"
		s := "hello, world\n"
		res.Path = p
		contents := s
		res.Content = &contents

		timeline := []Step{
			NewStartupStep(1000 * 60),          // startup
			NewChangedStep(1000*60, false),     // did we do something?
			fileExpect(p, s),                   // check initial state
			NewClearChangedStep(1000 * 15),     // did we do something?
			fileWrite(p, "this is whatever\n"), // change state
			NewChangedStep(1000*60, false),     // did we do something?
			fileExpect(p, s),                   // check again
			sleep(1),                           // we can sleep too!
		}

		testCases = append(testCases, test{
			name:     "simple file",
			res:      res,
			fail:     false,
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return os.Remove(p) },
		})
	}
	{
		r := makeRes("exec", "x1")
		res := r.(*ExecRes) // if this panics, the test will panic
		s := "hello, world"
		f := "/tmp/whatever"
		res.Cmd = fmt.Sprintf("echo '%s' > '%s'", s, f)
		res.Shell = "/bin/bash"
		res.IfCmd = "! diff <(cat /tmp/whatever) <(echo hello, world)"
		res.IfShell = "/bin/bash"
		res.WatchCmd = fmt.Sprintf("/usr/bin/inotifywait -e modify -m %s", f)
		//res.WatchShell = "/bin/bash"

		timeline := []Step{
			NewStartupStep(1000 * 60),        // startup
			NewChangedStep(1000*60, false),   // did we do something?
			fileExpect(f, s+"\n"),            // check initial state
			NewClearChangedStep(1000 * 15),   // did we do something?
			fileWrite(f, "this is stuff!\n"), // change state
			NewChangedStep(1000*60, false),   // did we do something?
			fileExpect(f, s+"\n"),            // check again
			sleep(1),                         // we can sleep too!
		}

		testCases = append(testCases, test{
			name:     "simple exec",
			res:      res,
			fail:     false,
			timeline: timeline,
			expect:   func() error { return nil },
			// build file for inotifywait
			startup: func() error { return ioutil.WriteFile(f, []byte("starting...\n"), 0666) },
			cleanup: func() error { return os.Remove(f) },
		})
	}
	{
		r := makeRes("file", "r1")
		res := r.(*FileRes) // if this panics, the test will panic
		p := "/tmp/emptyfile"
		res.Path = p
		res.State = "exists"

		timeline := []Step{
			NewStartupStep(1000 * 60),      // startup
			NewChangedStep(1000*60, false), // did we do something?
			fileExpect(p, ""),              // check initial state
			NewClearChangedStep(1000 * 15), // did we do something?
		}

		testCases = append(testCases, test{
			name:     "touch file",
			res:      res,
			fail:     false,
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return os.Remove(p) },
		})
	}
	{
		r := makeRes("file", "r1")
		res := r.(*FileRes) // if this panics, the test will panic
		p := "/tmp/existingfile"
		res.Path = p
		res.State = "exists"
		content := "some existing text\n"

		timeline := []Step{
			NewStartupStep(1000 * 60),     // startup
			NewChangedStep(1000*60, true), // did we do something?
			fileExpect(p, content),        // check initial state
		}

		testCases = append(testCases, test{
			name:     "existing file",
			res:      res,
			fail:     false,
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return ioutil.WriteFile(p, []byte(content), 0666) },
			cleanup:  func() error { return os.Remove(p) },
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
			res, fail, experr, experrstr, timeline, expect, startup, cleanup := tc.res, tc.fail, tc.experr, tc.experrstr, tc.timeline, tc.expect, tc.startup, tc.cleanup

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

			changedChan := make(chan bool, 1) // buffered!
			readyChan := make(chan struct{})
			eventChan := make(chan struct{})
			doneChan := make(chan struct{})
			debug := testing.Verbose() // set via the -test.v flag to `go test`
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d: Res: ", index)+format, v...)
			}
			init := &engine.Init{
				Running: func() {
					close(readyChan)
					select { // this always sends one!
					case eventChan <- struct{}{}:

					}
				},
				// Watch runs this to send a changed event.
				Event: func() {
					select {
					case eventChan <- struct{}{}:

					}
				},

				// Watch listens on this for close/pause events.
				Done:  doneChan,
				Debug: debug,
				Logf:  logf,

				// unused
				Send: func(st interface{}) error {
					return nil
				},
				Recv: func() map[string]*engine.Send {
					return map[string]*engine.Send{}
				},
			}

			t.Logf("test #%d: running startup()", index)
			if err := startup(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not startup: %+v", index, err)
			}
			// run init
			t.Logf("test #%d: running Init", index)
			err = res.Init(init)
			defer func() {
				t.Logf("test #%d: running cleanup()", index)
				if err := cleanup(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not cleanup: %+v", index, err)
				}
			}()
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

					// magic setting of important values...
					if s, ok := step.(*startupStep); ok {
						s.ch = readyChan
					}
					if s, ok := step.(*changedStep); ok {
						s.ch = changedChan
					}
					if s, ok := step.(*clearChangedStep); ok {
						s.ch = changedChan
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
				close(doneChan) // send Watch shutdown command
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
				//t.Logf("test #%d: CheckApply(true) (%t, %+v)", index, checkOK, err)
				select {
				// send a msg if we can, but never block
				case changedChan <- checkOK:
				default:
				}
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
