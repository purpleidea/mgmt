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

package coreexample

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// VUMeterFuncName is the name this function is registered as.
	VUMeterFuncName = "vumeter"

	// arg names...
	vuMeterArgNameSymbol     = "symbol"
	vuMeterArgNameMultiplier = "multiplier"
	vuMeterArgNamePeak       = "peak"
)

func init() {
	funcs.ModuleRegister(ModuleName, VUMeterFuncName, func() interfaces.Func { return &VUMeterFunc{} }) // must register the func and name
}

// VUMeterFunc is a gimmic function to display a vu meter from the microphone.
type VUMeterFunc struct {
	init *interfaces.Init
	last types.Value // last value received to use for diff

	symbol     string
	multiplier int64
	peak       float64

	result *string // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *VUMeterFunc) String() string {
	return VUMeterFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *VUMeterFunc) ArgGen(index int) (string, error) {
	seq := []string{vuMeterArgNameSymbol, vuMeterArgNameMultiplier, vuMeterArgNamePeak}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *VUMeterFunc) Validate() error {
	check := func(binary string) error {
		args := []string{"--help"}

		prog := fmt.Sprintf("%s %s", binary, strings.Join(args, " "))

		//obj.init.Logf("running: %s", prog)

		p, err := filepath.EvalSymlinks(binary)
		if err != nil {
			return err
		}
		// TODO: do we need to do the ^C handling?
		// XXX: is the ^C context cancellation propagating into this correctly?
		cmd := exec.CommandContext(context.TODO(), p, args...)
		cmd.Dir = ""
		cmd.Env = []string{}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}

		if err := cmd.Run(); err != nil {
			if e, ok := err.(*exec.Error); ok && e.Err == exec.ErrNotFound {
				return fmt.Errorf("is %s in your $PATH ?", binary)
			}

			return errwrap.Wrapf(err, "error running: %s", prog)
		}
		return nil
	}

	// if rec is a symlink, this will error without the above EvalSymlinks!
	for _, x := range []string{"/usr/bin/rec", "/usr/bin/sox"} {
		if err := check(x); err != nil {
			return err
		}
	}
	return nil
}

// Info returns some static info about itself.
func (obj *VUMeterFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str, %s int, %s float) str", vuMeterArgNameSymbol, vuMeterArgNameMultiplier, vuMeterArgNamePeak)),
	}
}

// Init runs some startup code for this function.
func (obj *VUMeterFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *VUMeterFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	ticker := newTicker()
	defer ticker.Stop()
	// FIXME: this goChan seems to work better than the ticker :)
	// this is because we have a ~1sec delay in capturing the value in exec
	goChan := make(chan struct{})
	once := &sync.Once{}
	onceFunc := func() { close(goChan) } // only run once!
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			obj.symbol = input.Struct()[vuMeterArgNameSymbol].Str()
			obj.multiplier = input.Struct()[vuMeterArgNameMultiplier].Int()
			obj.peak = input.Struct()[vuMeterArgNamePeak].Float()
			once.Do(onceFunc)
			continue // we must wrap around and go in through goChan

		//case <-ticker.C: // received the timer event
		case <-goChan: // triggers constantly

			if obj.last == nil {
				continue // still waiting for input values
			}

			// record for one second to a shared memory file
			// rec /dev/shm/mgmt_rec.wav trim 0 1 2>/dev/null
			args1 := []string{"/dev/shm/mgmt_rec.wav", "trim", "0", "1"}
			cmd1 := exec.Command("/usr/bin/rec", args1...)
			// XXX: arecord stopped working on newer linux...
			// arecord -d 1 /dev/shm/mgmt_rec.wav 2>/dev/null
			//args1 := []string{"-d", "1", "/dev/shm/mgmt_rec.wav"}
			//cmd1 := exec.Command("/usr/bin/arecord", args1...)
			cmd1.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
				Pgid:    0,
			}
			// start the command
			if _, err := cmd1.Output(); err != nil {
				return errwrap.Wrapf(err, "cmd failed to run")
			}

			// sox -t .wav /dev/shm/mgmt_rec.wav -n stat 2>&1 | grep "Maximum amplitude" | cut -d ':' -f 2
			args2 := []string{"-t", ".wav", "/dev/shm/mgmt_rec.wav", "-n", "stat"}
			cmd2 := exec.Command("/usr/bin/sox", args2...)
			cmd2.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
				Pgid:    0,
			}

			// start the command
			out, err := cmd2.CombinedOutput() // data comes on stderr
			if err != nil {
				return errwrap.Wrapf(err, "cmd failed to run")
			}

			ratio, err := extract(out)
			if err != nil {
				return errwrap.Wrapf(err, "failed to extract")
			}

			result, err := visual(obj.symbol, int(obj.multiplier), obj.peak, ratio)
			if err != nil {
				return errwrap.Wrapf(err, "could not generate visual")
			}

			if obj.result != nil && *obj.result == result {
				continue // result didn't change
			}
			obj.result = &result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: *obj.result,
		}:
		case <-ctx.Done():
			return nil
		}
	}
}

func newTicker() *time.Ticker {
	return time.NewTicker(time.Duration(1) * time.Second)
}

func extract(data []byte) (float64, error) {
	const prefix = "Maximum amplitude:"
	str := string(data)
	lines := strings.Split(str, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		s := strings.TrimSpace(line[len(prefix):])
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		return f, nil
	}
	return 0, fmt.Errorf("could not extract any data")
}

func round(f float64) int {
	return int(f + math.Copysign(0.5, f))
}

// TODO: make this fancier
func visual(symbol string, multiplier int, peak, ratio float64) (string, error) {
	if ratio > 1 || ratio < 0 {
		return "", fmt.Errorf("invalid ratio of %f", ratio)
	}

	x := strings.Repeat(symbol, round(ratio*float64(multiplier)))
	if x == "" {
		x += symbol // add a minimum
	}
	if ratio > peak {
		x += " PEAK!!!"
	}
	return fmt.Sprintf("(%f):\n%s\n%s", ratio, x, x), nil
}
