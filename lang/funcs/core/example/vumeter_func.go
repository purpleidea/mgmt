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

package coreexample

import (
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

func init() {
	funcs.ModuleRegister(moduleName, "vumeter", func() interfaces.Func { return &VUMeterFunc{} }) // must register the func and name
}

// VUMeterFunc is a gimmic function to display a vu meter from the microphone.
type VUMeterFunc struct {
	init *interfaces.Init
	last types.Value // last value received to use for diff

	symbol     string
	multiplier int64
	peak       float64

	result string // last calculated output

	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *VUMeterFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *VUMeterFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  types.NewType("func(symbol str, multiplier int, peak float) str"),
	}
}

// Init runs some startup code for this function.
func (obj *VUMeterFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *VUMeterFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	ticker := newTicker()
	defer ticker.Stop()
	// FIXME: this goChan seems to work better than the ticker :)
	// this is because we have a ~1sec delay in capturing the value in exec
	goChan := make(chan struct{})
	close(goChan)
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

			obj.symbol = input.Struct()["symbol"].Str()
			obj.multiplier = input.Struct()["multiplier"].Int()
			obj.peak = input.Struct()["peak"].Float()

		//case <-ticker.C: // received the timer event
		case <-goChan:

			if obj.last == nil {
				continue // still waiting for input values
			}

			// arecord -d 1 /dev/shm/mgmt_rec.wav 2>/dev/null
			args1 := []string{"-d", "1", "/dev/shm/mgmt_rec.wav"}
			cmd1 := exec.Command("/usr/bin/arecord", args1...)
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

			if obj.result == result {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: obj.result,
		}:
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *VUMeterFunc) Close() error {
	close(obj.closeChan)
	return nil
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
