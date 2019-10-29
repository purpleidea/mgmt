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
	"github.com/purpleidea/mgmt/util/errwrap"
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

// FileExpect takes a path and a string to expect in that file, and builds a
// Step that checks that out of them.
func FileExpect(p, s string) Step { // path & string
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

// FileExpect takes a path and a string to write to that file, and builds a Step
// that does that to them.
func FileWrite(p, s string) Step { // path & string
	return &manualStep{
		action: func() error {
			// TODO: apparently using 0666 is equivalent to respecting the current umask
			const umask = 0666
			return ioutil.WriteFile(p, []byte(s), umask)
		},
		expect: func() error { return nil },
	}
}

// ErrIsNotExistOK returns nil if we get an IsNotExist true result on the error.
func ErrIsNotExistOK(e error) error {
	if os.IsNotExist(e) {
		return nil
	}
	return errwrap.Wrapf(e, "unexpected error")
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

	testCases := []test{}
	{
		r := makeRes("file", "r1")
		res := r.(*FileRes) // if this panics, the test will panic
		p := "/tmp/whatever"
		s := "hello, world\n"
		res.Path = p
		res.State = "exists"
		contents := s
		res.Content = &contents

		timeline := []Step{
			NewStartupStep(1000 * 60),          // startup
			NewChangedStep(1000*60, false),     // did we do something?
			FileExpect(p, s),                   // check initial state
			NewClearChangedStep(1000 * 15),     // did we do something?
			FileWrite(p, "this is whatever\n"), // change state
			NewChangedStep(1000*60, false),     // did we do something?
			FileExpect(p, s),                   // check again
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
			FileExpect(f, s+"\n"),            // check initial state
			NewClearChangedStep(1000 * 15),   // did we do something?
			FileWrite(f, "this is stuff!\n"), // change state
			NewChangedStep(1000*60, false),   // did we do something?
			FileExpect(f, s+"\n"),            // check again
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
			FileExpect(p, ""),              // check initial state
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
			FileExpect(p, content),        // check initial state
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
				t.Logf(fmt.Sprintf("test #%d: ", index)+format, v...)
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

			if startup != nil {
				t.Logf("test #%d: running startup()", index)
				if err := startup(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not startup: %+v", index, err)
				}
			}
			// run init
			t.Logf("test #%d: running Init", index)
			err = res.Init(init)
			defer func() {
				if cleanup == nil {
					return
				}
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

// TestResources2 just tests a partial execution of the resource by running
// CheckApply and Reverse and basics without the mainloop. It's a less accurate
// representation of a running resource, but is still useful for many
// circumstances. This also uses a simpler timeline, because it was not possible
// to get the reference passing of the reversed resource working with the fancy
// version.
func TestResources2(t *testing.T) {
	type test struct { // an individual test
		name     string
		timeline []func() error // TODO: this could be a generator that keeps pushing out steps until it's done!
		expect   func() error   // function to check for expected state
		startup  func() error   // function to run as startup (unused?)
		cleanup  func() error   // function to run as cleanup
	}

	// resValidate runs Validate on the res.
	resValidate := func(res engine.Res) func() error {
		// run Close
		return func() error {
			return res.Validate()
		}
	}
	// resInit runs Init on the res.
	resInit := func(res engine.Res) func() error {
		logf := func(format string, v ...interface{}) {
			// noop for now
		}
		init := &engine.Init{
			//Debug: debug,
			Logf: logf,

			// unused
			Send: func(st interface{}) error {
				return nil
			},
			Recv: func() map[string]*engine.Send {
				return map[string]*engine.Send{}
			},

			// Copied from state.go
			FilteredGraph: func() (*pgraph.Graph, error) {
				graph, err := pgraph.NewGraph("filtered")
				if err != nil {
					return nil, errwrap.Wrapf(err, "could not create graph")
				}
				// Hack: We just add ourself as allowed since
				// we're just a one-vertex test suite...
				graph.AddVertex(res) // hack!

				return graph, nil // we return in a func so it's fresh!
			},
		}
		// run Init
		return func() error {
			return res.Init(init)

		}
	}
	// resCheckApplyError runs CheckApply with noop = false for the res. It
	// errors if the returned checkOK values isn't what we were expecting or
	// if the errOK function returns an error when given a chance to inspect
	// the returned error.
	resCheckApplyError := func(res engine.Res, expCheckOK bool, errOK func(e error) error) func() error {
		return func() error {
			checkOK, err := res.CheckApply(true) // no noop!
			if e := errOK(err); e != nil {
				return errwrap.Wrapf(e, "error from CheckApply did not match expected")
			}
			if checkOK != expCheckOK {
				return fmt.Errorf("result from CheckApply did not match expected: `%t` != `%t`", checkOK, expCheckOK)
			}
			return nil
		}
	}
	// resCheckApply runs CheckApply with noop = false for the res. It
	// errors if the returned checkOK values isn't what we were expecting or
	// if there was an error.
	resCheckApply := func(res engine.Res, expCheckOK bool) func() error {
		errOK := func(e error) error {
			if e == nil {
				return nil
			}
			return errwrap.Wrapf(e, "unexpected error from CheckApply")
		}
		return resCheckApplyError(res, expCheckOK, errOK)
	}
	// resClose runs Close on the res.
	resClose := func(res engine.Res) func() error {
		// run Close
		return func() error {
			return res.Close()
		}
	}
	// resReversal runs Reverse on the resource and stores the result in the
	// rev variable. This should be called before the res CheckApply, and
	// usually before Init, but after Validate.
	resReversal := func(res engine.Res, rev *engine.Res) func() error {
		return func() error {
			r, ok := res.(engine.ReversibleRes)
			if !ok {
				return fmt.Errorf("res is not a ReversibleRes")
			}

			// We don't really need this to be checked here.
			//if r.ReversibleMeta().Disabled {
			//	return fmt.Errorf("res did not specify Meta:reverse")
			//}

			if r.ReversibleMeta().Reversal {
				//logf("triangle reversal") // warn!
			}

			reversed, err := r.Reversed()
			if err != nil {
				return errwrap.Wrapf(err, "could not reverse: %s", r.String())
			}
			if reversed == nil {
				return nil // this can't be reversed, or isn't implemented here
			}

			reversed.ReversibleMeta().Reversal = true // set this for later...

			retRes, ok := reversed.(engine.Res)
			if !ok {
				return fmt.Errorf("not a Res")
			}

			*rev = retRes // store!
			return nil
		}
	}
	fileWrite := func(p, s string) func() error {
		// write the file to path
		return func() error {
			return ioutil.WriteFile(p, []byte(s), 0666)
		}
	}
	fileExpect := func(p, s string) func() error {
		// check the contents at the path match the string we expect
		return func() error {
			content, err := ioutil.ReadFile(p)
			if err != nil {
				return err
			}
			if string(content) != s {
				return fmt.Errorf("contents did not match in %s", p)
			}
			return nil
		}
	}
	fileExists := func(p string, dir bool) func() error {
		// does the file exist?
		return func() error {
			fi, err := os.Stat(p)
			if err != nil {
				return fmt.Errorf("file was supposed to be present, got: %+v", err)
			}
			if fi.IsDir() != dir {
				if dir {
					return fmt.Errorf("not a dir")
				}
				return fmt.Errorf("not a regular file")
			}
			return nil
		}
	}
	fileAbsent := func(p string) func() error {
		// does the file exist?
		return func() error {
			_, err := os.Stat(p)
			if !os.IsNotExist(err) {
				return fmt.Errorf("file was supposed to be absent, got: %+v", err)
			}
			return nil
		}
	}
	fileRemove := func(p string) func() error {
		// remove the file at path
		return func() error {
			err := os.Remove(p)
			// if the file isn't there, don't error
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
	}
	fileMkdir := func(p string, all bool) func() error {
		// mkdir at the path
		return func() error {
			if all {
				return os.MkdirAll(p, 0777)
			}
			return os.Mkdir(p, 0777)
		}
	}

	testCases := []test{}
	{
		//file "/tmp/somefile" {
		//	state => "exists",
		//	content => "some new text\n",
		//}
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "exists"
		content := "some new text\n"
		res.Content = &content

		timeline := []func() error{
			fileWrite(p, "whatever"),
			resValidate(r1),
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileExpect(p, content),
			resCheckApply(r1, true), // it's already good
			resClose(r1),
			fileExpect(p, content), // ensure it exists
		}

		testCases = append(testCases, test{
			name:     "simple file",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	# state is NOT specified
		//	content => "some new text\n",
		//}
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		//res.State = "exists" // not specified!
		content := "some new text\n"
		res.Content = &content

		timeline := []func() error{
			fileWrite(p, "whatever"),
			resValidate(r1),
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileExpect(p, content),
			resCheckApply(r1, true), // it's already good
			resClose(r1),
			fileExpect(p, content), // ensure it exists
		}

		testCases = append(testCases, test{
			name:     "edit file only",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	# state is NOT specified
		//	content => "some new text\n",
		//}
		// and no existing file exists! (therefore we want an error!)
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		//res.State = "exists" // not specified!
		content := "some new text\n"
		res.Content = &content

		timeline := []func() error{
			fileRemove(p), // nothing here
			resValidate(r1),
			resInit(r1),
			resCheckApplyError(r1, false, ErrIsNotExistOK), // should error
			resCheckApplyError(r1, false, ErrIsNotExistOK), // double check
			resClose(r1),
			fileAbsent(p), // ensure it's absent
		}

		testCases = append(testCases, test{
			name:     "strict file",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	state => "absent",
		//}
		// and no existing file exists!
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "absent"

		timeline := []func() error{
			fileRemove(p), // nothing here
			resValidate(r1),
			resInit(r1),
			resCheckApply(r1, true),
			resCheckApply(r1, true),
			resClose(r1),
			fileAbsent(p), // ensure it's absent
		}

		testCases = append(testCases, test{
			name:     "absent file",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	state => "absent",
		//}
		// and a file already exists!
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "absent"

		timeline := []func() error{
			fileWrite(p, "whatever"),
			resValidate(r1),
			resInit(r1),
			resCheckApply(r1, false),
			resCheckApply(r1, true),
			resClose(r1),
			fileAbsent(p), // ensure it's absent
		}

		testCases = append(testCases, test{
			name:     "absent file pre-existing",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	content => "some new text\n",
		//	state => "exists",
		//
		//	Meta:reverse => true,
		//}
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "exists"
		content := "some new text\n"
		res.Content = &content
		original := "this is the original state\n" // original state
		var r2 engine.Res                          // future reversed resource

		timeline := []func() error{
			fileWrite(p, original),
			fileExpect(p, original),
			resValidate(r1),
			resReversal(r1, &r2), // runs in Init to snapshot
			func() error { // random test
				if st := r2.(*FileRes).State; st != "absent" {
					return fmt.Errorf("unexpected state: %s", st)
				}
				return nil
			},
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileExpect(p, content),
			resCheckApply(r1, true), // it's already good
			resClose(r1),
			//resValidate(r2), // no!!!
			func() error {
				// wrap it b/c it is currently nil
				return r2.Validate()
			},
			func() error {
				return resInit(r2)()
			},
			func() error {
				return resCheckApply(r2, false)()
			},
			func() error {
				return resCheckApply(r2, true)()
			},
			func() error {
				return resClose(r2)()
			},
			fileAbsent(p), // ensure it's absent
		}

		testCases = append(testCases, test{
			name:     "some file",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	content => "some new text\n",
		//
		//	Meta:reverse => true,
		//}
		//# and there's an existing file at this path...
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		//res.State = "exists" // unspecified
		content := "some new text\n"
		res.Content = &content
		original := "this is the original state\n" // original state
		var r2 engine.Res                          // future reversed resource

		timeline := []func() error{
			fileWrite(p, original),
			fileExpect(p, original),
			resValidate(r1),
			resReversal(r1, &r2), // runs in Init to snapshot
			func() error { // random test
				// state should be unspecified
				if st := r2.(*FileRes).State; st == "absent" || st == "exists" {
					return fmt.Errorf("unexpected state: %s", st)
				}
				return nil
			},
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileExpect(p, content),
			resCheckApply(r1, true), // it's already good
			resClose(r1),
			//resValidate(r2),
			func() error {
				// wrap it b/c it is currently nil
				return r2.Validate()
			},
			func() error {
				return resInit(r2)()
			},
			func() error {
				return resCheckApply(r2, false)()
			},
			func() error {
				return resCheckApply(r2, true)()
			},
			func() error {
				return resClose(r2)()
			},
			fileExpect(p, original), // we restored the contents!
			fileRemove(p),           // cleanup
		}

		testCases = append(testCases, test{
			name:     "some file restore",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	content => "some new text\n",
		//
		//	Meta:reverse => true,
		//}
		//# and there's NO existing file at this path...
		//# NOTE: This used to be a corner case subtlety for reversal.
		//# Now that we error in this scenario before reversal, it's ok!
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		//res.State = "exists" // unspecified
		content := "some new text\n"
		res.Content = &content
		var r2 engine.Res // future reversed resource

		timeline := []func() error{
			fileRemove(p), // ensure no file exists
			resValidate(r1),
			resReversal(r1, &r2), // runs in Init to snapshot
			func() error { // random test
				// state should be unspecified i think
				// TODO: or should it be absent?
				if st := r2.(*FileRes).State; st == "absent" || st == "exists" {
					return fmt.Errorf("unexpected state: %s", st)
				}
				return nil
			},
			resInit(r1),
			resCheckApplyError(r1, false, ErrIsNotExistOK), // changed
			//fileExpect(p, content),
			//resCheckApply(r1, true), // it's already good
			resClose(r1),
			//func() error {
			//	// wrap it b/c it is currently nil
			//	return r2.Validate()
			//},
			//func() error {
			//	return resInit(r2)()
			//},
			//func() error { // it's already in the correct state
			//	return resCheckApply(r2, true)()
			//},
			//func() error {
			//	return resClose(r2)()
			//},
			//fileExpect(p, content), // we never changed it back...
			//fileRemove(p),          // cleanup
		}

		testCases = append(testCases, test{
			name:     "ambiguous file restore",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	state => "absent",
		//
		//	Meta:reverse => true,
		//}
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "absent"
		original := "this is the original state\n" // original state
		var r2 engine.Res                          // future reversed resource

		timeline := []func() error{
			fileWrite(p, original),
			fileExpect(p, original),
			resValidate(r1),
			resReversal(r1, &r2), // runs in Init to snapshot
			func() error { // random test
				if st := r2.(*FileRes).State; st != "exists" {
					return fmt.Errorf("unexpected state: %s", st)
				}
				return nil
			},
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileAbsent(p),            // ensure it got removed
			resCheckApply(r1, true),  // it's already good
			resClose(r1),
			//resValidate(r2), // no!!!
			func() error {
				// wrap it b/c it is currently nil
				return r2.Validate()
			},
			func() error {
				return resInit(r2)()
			},
			func() error {
				return resCheckApply(r2, false)()
			},
			func() error {
				return resCheckApply(r2, true)()
			},
			func() error {
				return resClose(r2)()
			},
			fileExpect(p, original), // ensure it's back to original
		}

		testCases = append(testCases, test{
			name:     "some removal",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		//file "/tmp/somefile" {
		//	state => "exists",
		//	fragments => [
		//		"/tmp/frag1",
		//		"/tmp/fragdir1/",
		//		"/tmp/frag2",
		//		"/tmp/fragdir2/",
		//		"/tmp/frag3",
		//	],
		//}
		r1 := makeRes("file", "r1")
		res := r1.(*FileRes) // if this panics, the test will panic
		p := "/tmp/somefile"
		res.Path = p
		res.State = "exists"
		res.Fragments = []string{
			"/tmp/frag1",
			"/tmp/fragdir1/",
			"/tmp/frag2",
			"/tmp/fragdir2/",
			"/tmp/frag3",
		}

		frag1 := "frag1\n"
		f1 := "f1\n"
		f2 := "f2\n"
		f3 := "f3\n"
		frag2 := "frag2\n"
		f1d2 := "f1 from fragdir2\n"
		f2d2 := "f2 from fragdir2\n"
		f3d2 := "f3 from fragdir2\n"
		frag3 := "frag3\n"
		content := frag1 + f1 + f2 + f3 + frag2 + f1d2 + f2d2 + f3d2 + frag3

		timeline := []func() error{
			fileWrite("/tmp/frag1", frag1),
			fileWrite("/tmp/frag2", frag2),
			fileWrite("/tmp/frag3", frag3),
			fileMkdir("/tmp/fragdir1/", true),
			fileWrite("/tmp/fragdir1/f1", f1),
			fileWrite("/tmp/fragdir1/f2", f2),
			fileWrite("/tmp/fragdir1/f3", f3),
			fileMkdir("/tmp/fragdir2/", true),
			fileWrite("/tmp/fragdir2/f1", f1d2),
			fileWrite("/tmp/fragdir2/f2", f2d2),
			fileWrite("/tmp/fragdir2/f3", f3d2),
			fileWrite(p, "whatever"),
			resValidate(r1),
			resInit(r1),
			resCheckApply(r1, false), // changed
			fileExpect(p, content),
			resCheckApply(r1, true), // it's already good
			resClose(r1),
			fileExpect(p, content), // ensure it exists
		}

		testCases = append(testCases, test{
			name:     "file fragments",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
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
			timeline, expect, startup, cleanup := tc.timeline, tc.expect, tc.startup, tc.cleanup

			t.Logf("test #%d: starting...\n", index)
			defer t.Logf("test #%d: done!", index)

			//debug := testing.Verbose() // set via the -test.v flag to `go test`
			//logf := func(format string, v ...interface{}) {
			//	t.Logf(fmt.Sprintf("test #%d: ", index)+format, v...)
			//}

			if startup != nil {
				t.Logf("test #%d: running startup()", index)
				if err := startup(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not startup: %+v", index, err)
				}
			}
			defer func() {
				if cleanup == nil {
					return
				}
				t.Logf("test #%d: running cleanup()", index)
				if err := cleanup(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not cleanup: %+v", index, err)
				}
			}()

			// run timeline
			t.Logf("test #%d: executing timeline", index)
			for ix, step := range timeline {
				t.Logf("test #%d: step(%d)...", index, ix)
				if err := step(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: step(%d) action failed: %s", index, ix, err.Error())
					break
				}
			}

			t.Logf("test #%d: shutting down...", index)

			if err := expect(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expect failed: %s", index, err.Error())
				return
			}

			// all done!
		})
	}
}
