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

package core // TODO: should this be in its own individual package?

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// HistoryFuncName is the name this function is registered as.
	// TODO: move this into a separate package
	HistoryFuncName = "history"

	// arg names...
	historyArgNameValue = "value"
	historyArgNameIndex = "index"

	// factor helps us sample much faster for precision reasons.
	factor = 10
)

func init() {
	funcs.Register(HistoryFuncName, func() interfaces.Func { return &HistoryFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &HistoryFunc{} // ensure it meets this expectation

// HistoryFunc is special function which returns the value N milliseconds ago.
// It must store up incoming values until it gets enough to return the desired
// one. If it doesn't yet have a value, it will initially return the oldest
// value it can. A restart of the program, will expunge the stored state. This
// obviously takes more memory, the further back you wish to index. A change in
// the index var is generally not useful, but it is permitted. Moving it to a
// smaller value will cause older index values to be expunged. If this is
// undesirable, a max count could be added. This was not implemented with
// efficiency in mind. This implements a *time* based hysteresis, since
// previously this only looked at the last N changed values. Since some
// functions might not send out un-changed values, it might make more sense this
// way. This time based hysteresis should tick every precision-width, and store
// whatever the latest value at that time is. This is implemented wrong, because
// we can't guarantee the sampling interval is constant, and it's also wasteful.
// We should implement a better version that keeps track of the time, so that we
// can pick the closest one and also not need to store duplicates.
// XXX: This function needs another look. We likely we to snapshot everytime we
// get a new value in obj.Call instead of having a ticker.
type HistoryFunc struct {
	Type *types.Type // type of input value (same as output type)

	init *interfaces.Init

	input chan int
	delay *int

	value     types.Value // last value
	buffer    []*valueWithTimestamp
	interval  int
	retention int

	ticker *time.Ticker
	mutex  *sync.Mutex // don't need an rwmutex since only one reader
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *HistoryFunc) String() string {
	return HistoryFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *HistoryFunc) ArgGen(index int) (string, error) {
	seq := []string{historyArgNameValue, historyArgNameIndex}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *HistoryFunc) sig() *types.Type {
	// func(value ?1, index int) ?1
	s := "?1"
	if obj.Type != nil {
		s = obj.Type.String()
	}
	return types.NewType(fmt.Sprintf("func(%s %s, %s int) %s", historyArgNameValue, s, historyArgNameIndex, s))
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. That type is used to build our function
// statically.
func (obj *HistoryFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the history function needs exactly two args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	if t1.Cmp(types.TypeInt) != nil {
		return nil, fmt.Errorf("second arg for history must be an int")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	obj.Type = t0 // type of historical value is now known!

	return obj.sig(), nil
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *HistoryFunc) Copy() interfaces.Func {
	return &HistoryFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *HistoryFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *HistoryFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *HistoryFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan int)
	obj.mutex = &sync.Mutex{}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *HistoryFunc) Stream(ctx context.Context) error {
	obj.ticker = time.NewTicker(1) // build it however (non-zero to avoid panic!)
	defer obj.ticker.Stop()        // double stop is safe
	obj.ticker.Stop()              // begin with a stopped ticker
	select {
	case <-obj.ticker.C: // drain if needed
	default:
	}

	for {
		select {
		case delay, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			// obj.delay is only used here for duplicate detection,
			// and while similar to obj.interval, we don't reuse it
			// because we don't want a race condition reading delay
			if obj.delay != nil && *obj.delay == delay {
				continue // nothing changed
			}
			obj.delay = &delay

			obj.reinit(int(delay)) // starts ticker!

		case <-obj.ticker.C: // received the timer event
			obj.store()
			// XXX: We deadlock here if the select{} in obj.Call
			// runs at the same time and the event obj.ag is
			// unbuffered. Should the engine buffer?

			// XXX: If we send events, we basically infinite loop :/
			// XXX:  Didn't look into the feedback mechanism yet.
			//if err := obj.init.Event(ctx); err != nil {
			//	return err
			//}

		case <-ctx.Done():
			return nil
		}
	}
}

func (obj *HistoryFunc) reinit(delay int) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.buffer == nil {
	}

	obj.interval = delay
	obj.retention = delay + 10000 // XXX: arbitrary
	obj.buffer = []*valueWithTimestamp{}

	duration := delay / factor // XXX: sample more often than delay?

	// Start sampler...
	if duration == 0 { // can't be zero or ticker will panic
		duration = 100 // XXX: 1ms is probably too fast
	}
	obj.ticker.Reset(time.Duration(duration) * time.Millisecond)
}

func (obj *HistoryFunc) store() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	val := obj.value.Copy() // copy

	now := time.Now()
	v := &valueWithTimestamp{
		Timestamp: now,
		Value:     val,
	}
	obj.buffer = append(obj.buffer, v) // newer values go at the end

	retention := time.Duration(obj.retention) * time.Millisecond

	// clean up old entries
	cutoff := now.Add(-retention)
	i := 0
	for ; i < len(obj.buffer); i++ {
		if obj.buffer[i].Timestamp.After(cutoff) {
			break
		}
	}
	obj.buffer = obj.buffer[i:]
}

func (obj *HistoryFunc) peekAgo(ms int) types.Value {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.buffer == nil { // haven't started yet
		return nil
	}
	if len(obj.buffer) == 0 { // no data exists yet
		return nil
	}

	target := time.Now().Add(-time.Duration(ms) * time.Millisecond)

	for i := len(obj.buffer) - 1; i >= 0; i-- {
		if !obj.buffer[i].Timestamp.After(target) {
			return obj.buffer[i].Value
		}
	}

	// If no value found, return the oldest one.
	return obj.buffer[0].Value
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *HistoryFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("not enough args")
	}
	value := args[0]
	interval := args[1].Int() // ms (used to be index)

	if interval < 0 {
		return nil, fmt.Errorf("can't use a negative interval of %d", interval)
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	obj.mutex.Lock()
	obj.value = value // store a copy
	obj.mutex.Unlock()

	// XXX: we deadlock here if obj.init.Event also runs at the same time!
	// XXX: ...only if it's unbuffered of course. Should the engine buffer?
	select {
	case obj.input <- interval: // inform the delay interval
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	val := obj.peekAgo(int(interval)) // contains mutex
	if val == nil {                   // don't have a value yet, return self...
		return obj.value, nil
	}
	return val, nil
}

// valueWithTimestamp stores a value alongside the time it was recorded.
type valueWithTimestamp struct {
	Timestamp time.Time
	Value     types.Value
}
