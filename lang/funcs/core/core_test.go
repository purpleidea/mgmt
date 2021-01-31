// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package core

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
)

func TestPureFuncExec0(t *testing.T) {
	type test struct { // an individual test
		name     string
		funcname string
		args     []types.Value
		fail     bool
		expect   types.Value
	}
	testCases := []test{}

	//{
	//	testCases = append(testCases, test{
	//		name: "",
	//		funcname: "",
	//		args: []types.Value{
	//		},
	//		fail: false,
	//		expect: nil,
	//	})
	//}
	{
		testCases = append(testCases, test{
			name:     "strings.to_lower 0",
			funcname: "strings.to_lower",
			args: []types.Value{
				&types.StrValue{
					V: "HELLO",
				},
			},
			fail: false,
			expect: &types.StrValue{
				V: "hello",
			},
		})
	}
	{
		testCases = append(testCases, test{
			name:     "datetime.now fail",
			funcname: "datetime.now",
			args:     nil,
			fail:     true,
			expect:   nil,
		})
	}
	// TODO: run unification in PureFuncExec if it makes sense to do so...
	//{
	//	testCases = append(testCases, test{
	//		name:     "len 0",
	//		funcname: "len",
	//		args: []types.Value{
	//			&types.StrValue{
	//				V: "Hello, world!",
	//			},
	//		},
	//		fail: false,
	//		expect: &types.IntValue{
	//			V: 13,
	//		},
	//	})
	//}

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
		//if (index != 20 && index != 21) {
		//if tc.name != "nil" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, funcname, args, fail, expect := tc.name, tc.funcname, tc.args, tc.fail, tc.expect

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			f, err := funcs.Lookup(funcname)
			if err != nil {
				t.Errorf("test #%d: func lookup failed with: %+v", index, err)
				return
			}

			result, err := funcs.PureFuncExec(f, args)

			if !fail && err != nil {
				t.Errorf("test #%d: func failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: func passed, expected fail", index)
				return
			}
			if !fail && result == nil {
				t.Errorf("test #%d: func output was nil", index)
				return
			}

			if reflect.DeepEqual(result, expect) {
				return
			}

			// double check because DeepEqual is different since the func exists
			diff := pretty.Compare(result, expect)
			if diff == "" { // bonus
				return
			}
			t.Errorf("test #%d: result did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n\n%s\n", index, spew.Sdump(result))
			t.Logf("test #%d: expected: \n\n%s", index, spew.Sdump(expect))

			// more details, for tricky cases:
			diffable := &pretty.Config{
				Diffable:          true,
				IncludeUnexported: true,
				//PrintStringers: false,
				//PrintTextMarshalers: false,
				//SkipZeroFields: false,
			}
			t.Logf("test #%d:   actual: \n\n%s\n", index, diffable.Sprint(result))
			t.Logf("test #%d: expected: \n\n%s", index, diffable.Sprint(expect))
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
	}
}

// Step is used for the timeline in tests.
type Step interface {
	Action() error
	Expect() error
}

type manualStep struct {
	action func() error
	expect func() error

	exit       chan struct{}      // exit signal, set by test harness
	argch      chan []types.Value // send new inputs, set by test harness
	valueptrch chan int           // incoming values, set by test harness
	results    []types.Value      // all values, set by test harness
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

// NewSendInputs sends a list of inputs to the running function to populate it.
// If you send the wrong input signature, then you'll cause a failure. Testing
// this kind of failure is not a goal of these tests, since the unification code
// is meant to guarantee we always send the correct type signature.
func NewSendInputs(inputs []types.Value) Step {
	return &sendInputsStep{
		inputs: inputs,
	}
}

type sendInputsStep struct {
	inputs []types.Value

	exit  chan struct{}      // exit signal, set by test harness
	argch chan []types.Value // send new inputs, set by test harness
	//valueptrch chan int // incoming values, set by test harness
	//results []types.Value // all values, set by test harness
}

func (obj *sendInputsStep) Action() error {
	select {
	case obj.argch <- obj.inputs:
		return nil
	case <-obj.exit:
		return fmt.Errorf("exit called")
	}
}

func (obj *sendInputsStep) Expect() error { return nil }

// NewWaitForNSeconds waits this many seconds for new values from the stream. It
// can timeout if it gets bored of waiting.
func NewWaitForNSeconds(number int, timeout int) Step {
	return &waitAmountStep{
		timer:   number, // timer seconds
		timeout: timeout,
	}
}

// NewWaitForNValues waits for this many values from the stream. It can timeout
// if it gets bored of waiting. If you request more values than can be produced,
// then it will block indefinitely if there's no timeout.
func NewWaitForNValues(number int, timeout int) Step {
	return &waitAmountStep{
		count:   number, // count values
		timeout: timeout,
	}
}

// waitAmountStep waits for either a count of N values, or a timer of N seconds,
// or both. It also accepts a timeout which will cause it to error.
// TODO: have the timeout timer be overall instead of per step!
type waitAmountStep struct {
	count   int // nth count (set to a negative value to disable)
	timer   int // seconds (set to a negative value to disable)
	timeout int // seconds to fail after

	exit chan struct{} // exit signal, set by test harness
	//argch chan []types.Value // send new inputs, set by test harness
	valueptrch chan int      // incoming values, set by test harness
	results    []types.Value // all values, set by test harness
}

func (obj *waitAmountStep) Action() error {
	count := 0
	ticked := false // did we get the timer event?

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticker := util.TimeAfterOrBlockCtx(ctx, obj.timer)
	if obj.timer < 0 { // disable timer
		ticked = true
	}

	for {
		if count >= obj.count { // got everything we wanted
			if ticked {
				break
			}
		}
		select {
		case <-obj.exit:
			return fmt.Errorf("exit called")

		case <-util.TimeAfterOrBlock(obj.timeout):
			// TODO: make this overall instead of re-running it each time
			return fmt.Errorf("waited too long for a value")

		case n, ok := <-obj.valueptrch: // read output
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			count++
			_ = n // this is the index of the value we're at

		case <-ticker: // received the timer event
			ticker = nil
			ticked = true
			if obj.count > -1 {
				break
			}
		}
	}
	return nil
}
func (obj *waitAmountStep) Expect() error { return nil }

// NewRangeExpect passes in an expect function which will receive the entire
// range of values ever received. This stream (list) of values can be matched on
// however you like.
func NewRangeExpect(fn func([]types.Value) error) Step {
	return &rangeExpectStep{
		fn: fn,
	}
}

type rangeExpectStep struct {
	fn func([]types.Value) error

	// TODO: we could pass exit to the expect fn if we wanted in the future
	exit chan struct{} // exit signal, set by test harness
	//argch chan []types.Value // send new inputs, set by test harness
	//valueptrch chan int // incoming values, set by test harness
	results []types.Value // all values, set by test harness
}

func (obj *rangeExpectStep) Action() error { return nil }

func (obj *rangeExpectStep) Expect() error {
	results := []types.Value{}
	for _, v := range obj.results { // copy
		value := v.Copy()
		results = append(results, value)
	}
	return obj.fn(results) // run with a copy
}

// vog is a helper function to produce mcl values from golang equivalents that
// is only safe in tests because it panics on error.
func vog(i interface{}) types.Value {
	v, err := types.ValueOfGolang(i)
	if err != nil {
		panic(fmt.Sprintf("unexpected error in vog: %+v", err))
	}
	return v
}

// rcopy is a helper to copy a list of types.Value structs.
func rcopy(input []types.Value) []types.Value {
	result := []types.Value{}
	for i := range input {
		x := input[i].Copy()
		result = append(result, x)
	}
	return result
}

// TestLiveFuncExec0 runs a live execution timeline on a function stream. It is
// very useful for testing function streams.
// FIXME: if the function returns a different type than what is specified by its
// signature, we might block instead of returning a useful error.
func TestLiveFuncExec0(t *testing.T) {
	type args struct {
		argv []types.Value
		next func() // specifies we're ready for the next set of inputs
	}

	type test struct { // an individual test
		name     string
		hostname string // in case we want to simulate a hostname
		funcname string

		// TODO: this could be a generator that keeps pushing out steps until it's done!
		timeline []Step
		expect   func() error // function to check for expected state
		startup  func() error // function to run as startup
		cleanup  func() error // function to run as cleanup
	}

	timeout := -1 // default timeout (block) if not specified elsewhere
	testCases := []test{}
	{
		count := 5
		timeline := []Step{
			NewWaitForNValues(count, timeout), // get 5 values
			// pass in a custom validation function
			NewRangeExpect(func(args []types.Value) error {
				//fmt.Printf("range: %+v\n", args) // debugging
				if len(args) < count {
					return fmt.Errorf("no args found")
				}
				// check for increasing ints (ideal delta == 1)
				x := args[0].Int()
				for i := 1; i < count; i++ {
					if args[i].Int()-x < 1 {
						return fmt.Errorf("range jumps: %+v", args)
					}
					if args[i].Int()-x != 1 {
						// if this fails, travis is just slow
						return fmt.Errorf("timing error: %+v", args)
					}
					x = args[i].Int()
				}
				return nil
			}),
			//NewWaitForNSeconds(5, timeout), // not needed
		}

		testCases = append(testCases, test{
			name:     "simple func",
			hostname: "", // not needed for this func
			funcname: "datetime.now",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		timeline := []Step{
			NewSendInputs([]types.Value{
				vog("helloXworld"),
				vog("X"), // split by this
			}),

			NewWaitForNValues(1, timeout), // more than 1 blocks here

			// pass in a custom validation function
			NewRangeExpect(func(args []types.Value) error {
				//fmt.Printf("range: %+v\n", args) // debugging
				if c := len(args); c != 1 {
					return fmt.Errorf("wrong args count, got: %d", c)
				}
				if args[0].Type().Kind != types.KindList {
					return fmt.Errorf("expected list, got: %+v", args[0])
				}
				if err := vog([]string{"hello", "world"}).Cmp(args[0]); err != nil {
					return errwrap.Wrapf(err, "got different expected value: %+v", args[0])
				}
				return nil
			}),
		}

		testCases = append(testCases, test{
			name:     "simple pure func",
			hostname: "", // not needed for this func
			funcname: "strings.split",
			timeline: timeline,
			expect:   func() error { return nil },
			startup:  func() error { return nil },
			cleanup:  func() error { return nil },
		})
	}
	{
		p := "/tmp/somefiletoread"
		content := "hello world!\n"
		timeline := []Step{
			NewSendInputs([]types.Value{
				vog(p),
			}),

			NewWaitForNValues(1, timeout), // more than 1 blocks here
			NewWaitForNSeconds(5, 10),     // wait longer just to be sure

			// pass in a custom validation function
			NewRangeExpect(func(args []types.Value) error {
				//fmt.Printf("range: %+v\n", args) // debugging
				if c := len(args); c != 1 {
					return fmt.Errorf("wrong args count, got: %d", c)
				}
				if args[0].Type().Kind != types.KindStr {
					return fmt.Errorf("expected str, got: %+v", args[0])
				}
				if err := vog(content).Cmp(args[0]); err != nil {
					return errwrap.Wrapf(err, "got different expected value: %+v", args[0])
				}
				return nil
			}),
		}

		testCases = append(testCases, test{
			name:     "readfile",
			hostname: "", // not needed for this func
			funcname: "os.readfile",
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
			hostname, funcname, timeline, expect, startup, cleanup := tc.hostname, tc.funcname, tc.timeline, tc.expect, tc.startup, tc.cleanup

			t.Logf("\n\ntest #%d: func: %+v\n", index, funcname)
			defer t.Logf("test #%d: done!", index)

			handle, err := funcs.Lookup(funcname) // get function...
			if err != nil {
				t.Errorf("test #%d: func lookup failed with: %+v", index, err)
				return
			}
			// run validate!
			if err := handle.Validate(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not validate Func: %+v", index, err)
				return
			}
			sig := handle.Info().Sig
			if sig.Kind != types.KindFunc {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: must be kind func: %+v", index, err)
				return
			}

			input := make(chan types.Value)  // we close this when we're done
			output := make(chan types.Value) // we create it, func closes it

			debug := testing.Verbose() // set via the -test.v flag to `go test`
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d: func: ", index)+format, v...)
			}
			init := &interfaces.Init{
				Hostname: hostname,
				Input:    input,
				Output:   output,
				World:    nil, // TODO: add me somehow!
				Debug:    debug,
				Logf:     logf,
			}

			t.Logf("test #%d: running startup()", index)
			if err := startup(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not startup: %+v", index, err)
				return
			}

			// run init
			t.Logf("test #%d: running Init", index)
			if err := handle.Init(init); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init func: %+v", index, err)
				return
			}
			defer func() {
				t.Logf("test #%d: running cleanup()", index)
				if err := cleanup(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not cleanup: %+v", index, err)
				}
			}()

			wg := &sync.WaitGroup{}
			defer wg.Wait() // if we return early

			argch := make(chan []types.Value)
			errch := make(chan error)
			close1 := make(chan struct{})
			close2 := make(chan struct{})
			kill1 := make(chan struct{})
			kill2 := make(chan struct{})
			//kill3 := make(chan struct{}) // future use
			exit := make(chan struct{})

			mutex := &sync.RWMutex{}
			results := []types.Value{}          // all values received so far
			valueptrch := make(chan int)        // which Nth value are we at?
			killTimeline := make(chan struct{}) // ask timeline to exit

			// wait for close signals
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(errch) // last one turns out the lights
				select {
				case <-close1:
				}
				select {
				case <-close2:
				}
			}()

			// wait for kill signals
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-exit:
				case <-kill1:
				case <-kill2:
					//case <-kill3: // future use
				}
				close(killTimeline) // main kill signal for tl
			}()

			// run the stream
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(close1)
				if debug {
					logf("Running func")
				}
				err := handle.Stream() // sends to output chan
				t.Logf("test #%d: stream exited with: %+v", index, err)
				if debug {
					logf("Exiting func")
				}
				if err == nil {
					return
				}
				// we closed with an error...
				select {
				case errch <- errwrap.Wrapf(err, "problem streaming func"):
				}
			}()

			// read from incoming args and send to input channel
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(close2)
				defer close(input) // close input when done
				if argch == nil {  // no args
					return
				}
				si := &types.Type{
					// input to functions are structs
					Kind: types.KindStruct,
					Map:  handle.Info().Sig.Map,
					Ord:  handle.Info().Sig.Ord,
				}
				t.Logf("test #%d: func has sig: %s", index, si)

				// TODO: should this be a select with an exit signal?
				for args := range argch { // chan
					st := types.NewStruct(si)
					count := 0
					for i, arg := range args {
						//name := util.NumToAlpha(i) // assume (incorrectly) for now...
						name := handle.Info().Sig.Ord[i]          // better
						if err := st.Set(name, arg); err != nil { // populate struct
							select {
							case errch <- errwrap.Wrapf(err, "struct set failure"):
							}
							close(kill1) // unblock tl and cause fail
							return
						}
						count++
					}
					if count != len(si.Map) { // expect this number
						select {
						case errch <- fmt.Errorf("struct field count is wrong"):
						}
						close(kill1) // unblock tl and cause fail
						return
					}

					t.Logf("test #%d: send to func: %s", index, args)
					select {
					case input <- st: // send to function (must not block)
					case <-close1: // unblock the input send in case stream closed
						select {
						case errch <- fmt.Errorf("stream closed early"):
						}
					}
				}
			}()

			// run timeline
			wg.Add(1)
			go func() {
				t.Logf("test #%d: executing timeline", index)
				defer wg.Done()
			Timeline:
				for ix, step := range timeline {
					select {
					case <-killTimeline:
						break Timeline
					default:
						// pass
					}

					mutex.RLock()
					// magic setting of important values...
					if s, ok := step.(*manualStep); ok {
						s.exit = killTimeline      // kill signal
						s.argch = argch            // send inputs here
						s.valueptrch = valueptrch  // receive value ptr
						s.results = rcopy(results) // all results as array
					}
					if s, ok := step.(*sendInputsStep); ok {
						s.exit = killTimeline
						s.argch = argch
						//s.valueptrch = valueptrch
						//s.results = rcopy(results)
					}
					if s, ok := step.(*waitAmountStep); ok {
						s.exit = killTimeline
						//s.argch = argch
						s.valueptrch = valueptrch
						s.results = rcopy(results)
					}
					if s, ok := step.(*rangeExpectStep); ok {
						s.exit = killTimeline
						//s.argch = argch
						//s.valueptrch = valueptrch
						s.results = rcopy(results)
					}
					mutex.RUnlock()

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
				t.Logf("test #%d: timeline finished", index)
				close(argch)

				t.Logf("test #%d: running Close", index)
				if err := handle.Close(); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not close func: %+v", index, err)
					return
				}
			}()

			// read everything
			counter := 0
		Loop:
			for {
				select {
				case value, ok := <-output: // read from channel
					if !ok {
						output = nil
						continue Loop // only exit via errch closing!
					}
					t.Logf("test #%d: got from func: %s", index, value)
					// check return type
					if err := handle.Info().Sig.Out.Cmp(value.Type()); err != nil {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: unexpected return type from func: %+v", index, err)
						close(kill2)
						continue Loop // only exit via errch closing!
					}

					mutex.Lock()
					results = append(results, value) // save value
					mutex.Unlock()
					counter++

				case err, ok := <-errch: // handle possible errors
					if !ok {
						break Loop
					}
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: error: %+v", index, err)
					continue Loop // only exit via errch closing!
				}

				// send events to our timeline
				select {
				case valueptrch <- counter: // TODO: send value?

					// TODO: add this sort of thing, but don't block everyone who doesn't read
					//case <-time.After(time.Duration(globalStepReadTimeout) * time.Second):
					//	t.Errorf("test #%d: FAIL", index)
					//	t.Errorf("test #%d: timeline receiver was too slow for value", index)
					//	t.Errorf("test #%d: got(%d): %+v", index, counter, results[counter])
					//	close(kill3) // shut everything down
					//	continue Loop // only exit via errch closing!
				}
			}

			t.Logf("test #%d: waiting for shutdown", index)
			close(exit)
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
