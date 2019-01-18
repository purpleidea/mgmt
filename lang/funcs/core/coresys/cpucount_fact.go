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

package coresys

import (
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/socketset"

	errwrap "github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	rtmGrps         = 0x1 // make me a multicast receiver
	socketFile      = "pipe.sock"
	cpuDevpathRegex = "/devices/system/cpu/cpu[0-9]"
)

func init() {
	facts.ModuleRegister(moduleName, "cpu_count", func() facts.Fact { return &CPUCountFact{} }) // must register the fact and name
}

// CPUCountFact is a fact that returns the current CPU count.
type CPUCountFact struct {
	init      *facts.Init
	closeChan chan struct{}
}

// Info returns static typing info about what the fact returns.
func (obj *CPUCountFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("int"),
	}
}

// Init runs startup code for this fact. Initializes the closeChan and sets the
// facts.Init variable.
func (obj *CPUCountFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this fact has over time. It will
// first poll sysfs to get the initial cpu count, and then receives UEvents
// from the kernel as CPUs are added/removed.
func (obj CPUCountFact) Stream() error {
	defer close(obj.init.Output) // signal when we're done

	ss, err := socketset.NewSocketSet(rtmGrps, socketFile, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}

	// waitgroup for netlink receive goroutine
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer ss.Close()
	defer ss.Shutdown()

	eventChan := make(chan *nlChanEvent) // updated in goroutine when we receive uevent
	closeChan := make(chan struct{})     // channel to unblock selects in goroutine
	defer close(closeChan)

	// wait for kernel to poke us about new device changes on the system
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)
		for {
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

	startChan := make(chan struct{})
	close(startChan) // trigger the first event
	var cpuCount, newCount int64 = 0, -1
	for {
		select {
		case <-startChan:
			startChan = nil // disable
			newCount, err = getCPUCount()
			if err != nil {
				obj.init.Logf("Could not get initial CPU count. Setting to zero.")
			}
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
			if isCPUEvent(event.uevent) {
				newCount, err = getCPUCount()
				if err != nil {
					obj.init.Logf("could not getCPUCount: %e", err)
					continue
				}
			}
		case <-obj.closeChan:
			return nil
		}

		if newCount == cpuCount {
			continue
		}
		cpuCount = newCount

		select {
		case obj.init.Output <- &types.IntValue{
			V: cpuCount,
		}:
			// send
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs cleanup code for the fact and turns off the Stream.
func (obj *CPUCountFact) Close() error {
	close(obj.closeChan)
	return nil
}

// getCPUCount looks in sysfs to get the number of CPUs that are online.
func getCPUCount() (int64, error) {
	dat, err := ioutil.ReadFile("/sys/devices/system/cpu/online")
	if err != nil {
		return 0, err
	}
	return parseCPUList(string(dat))
}

// Parses a line of the form X,Y,Z,... where X,Y,Z can be either a single CPU or a
// contiguous range of CPUs. e.g. "2,4-31,32-63". If there is an error parsing
// the line the function will return 0.
func parseCPUList(list string) (int64, error) {
	var count int64
	for _, rg := range strings.Split(list, ",") {
		cpuRange := strings.SplitN(rg, "-", 2)
		if len(cpuRange) == 1 {
			count++
		} else if len(cpuRange) == 2 {
			lo, err := strconv.ParseInt(cpuRange[0], 10, 64)
			if err != nil {
				return 0, err
			}
			hi, err := strconv.ParseInt(strings.TrimRight(cpuRange[1], "\n"), 10, 64)
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
