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

//go:build !darwin

package coresys

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/socketset"

	"golang.org/x/sys/unix"
)

const (
	// CPUCountFuncName is the name this fact is registered as. It's still a
	// Func Name because this is the name space the fact is actually using.
	CPUCountFuncName = "cpu_count"

	rtmGrps         = 0x1 // make me a multicast receiver
	socketFile      = "pipe.sock"
	cpuDevpathRegex = "/devices/system/cpu/cpu[0-9]"
)

func init() {
	funcs.ModuleRegister(ModuleName, CPUCountFuncName, func() interfaces.Func { return &CPUCount{} }) // must register the fact and name
}

// CPUCount is a fact that returns the current CPU count.
type CPUCount struct {
	init   *interfaces.Init
	result types.Value // last calculated output
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *CPUCount) String() string {
	return CPUCountFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *CPUCount) Validate() error {
	return nil
}

// Info returns static typing info about what the fact returns.
func (obj *CPUCount) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() int"),
	}
}

// Init runs startup code for this fact.
func (obj *CPUCount) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream starts a mainloop and runs Event when it's time to Call() again. It
// will first poll sysfs to get the initial cpu count, and then receives UEvents
// from the kernel as CPUs are added/removed.
func (obj CPUCount) Stream(ctx context.Context) error {
	ss, err := socketset.NewSocketSet(rtmGrps, socketFile, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}

	// waitgroup for netlink receive goroutine
	wg := &sync.WaitGroup{}
	defer ss.Close()
	// We must wait for the Shutdown() AND the select inside of SocketSet to
	// complete before we Close, since the unblocking in SocketSet is not a
	// synchronous operation.
	defer wg.Wait()
	defer ss.Shutdown() // close the netlink socket and unblock conn.receive()

	eventChan := make(chan *nlChanEvent) // updated in goroutine when we receive uevent
	closeChan := make(chan struct{})     // channel to unblock selects in goroutine
	defer close(closeChan)

	// wait for kernel to poke us about new device changes on the system
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)
		for {
			// XXX: This does *not* generate an initial event on
			// startup, so instead, use startChan below...
			uevent, err := ss.ReceiveUEvent() // calling Shutdown will stop this from blocking
			if obj.init.Debug {
				obj.init.Logf("sending uevent SEQNUM: %s", uevent.Data["SEQNUM"])
			}
			select {
			case eventChan <- &nlChanEvent{
				uevent: uevent,
				err:    err,
			}:
			case <-closeChan:
				return
			}
		}
	}()

	// streams must generate an initial event on startup
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!
	for {
		select {
		case <-startChan:
			startChan = nil // disable

		case event, ok := <-eventChan:
			if !ok {
				continue
			}
			if event.err != nil {
				return errwrap.Wrapf(event.err, "error receiving uevent")
			}
			if obj.init.Debug {
				obj.init.Logf("received uevent SEQNUM: %s", event.uevent.Data["SEQNUM"])
			}
			if !isCPUEvent(event.uevent) {
				continue
			}

		case <-ctx.Done():
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *CPUCount) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	count, err := getCPUCount() // TODO: ctx?
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get CPU count")
	}

	return &types.IntValue{
		V: count,
	}, nil

}

// getCPUCount looks in sysfs to get the number of CPUs that are online.
func getCPUCount() (int, error) {
	dat, err := os.ReadFile("/sys/devices/system/cpu/online")
	if err != nil {
		return 0, err
	}
	return parseCPUList(string(dat))
}

// Parses a line of the form X,Y,Z,... where X,Y,Z can be either a single CPU or
// a contiguous range of CPUs. e.g. "2,4-31,32-63". If there is an error parsing
// the line the function will return 0.
func parseCPUList(list string) (int, error) {
	var count int
	for _, rg := range strings.Split(list, ",") {
		cpuRange := strings.SplitN(rg, "-", 2)
		if len(cpuRange) == 1 {
			count++
		} else if len(cpuRange) == 2 {
			lo, err := strconv.Atoi(cpuRange[0])
			if err != nil {
				return 0, err
			}
			hi, err := strconv.Atoi(strings.TrimRight(cpuRange[1], "\n"))
			if err != nil {
				return 0, err
			}
			count += hi - lo + 1
		}
	}
	return count, nil
}

// When we receive a udev event, we filter only those that indicate a CPU is
// being added or removed, or being taken online or offline.
func isCPUEvent(event *socketset.UEvent) bool {
	if event.Subsystem != "cpu" {
		return false
	}
	// is this a valid cpu path in sysfs?
	m, err := regexp.MatchString(cpuDevpathRegex, event.Devpath)
	if !m || err != nil {
		return false
	}
	if event.Action == "add" || event.Action == "remove" || event.Action == "online" || event.Action == "offline" {
		return true
	}
	return false
}

// nlChanEvent defines the channel used to send netlink messages and errors to
// the event processing loop in Stream.
type nlChanEvent struct {
	uevent *socketset.UEvent
	err    error
}
