// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/gob"
	"log"
	"time"
)

func init() {
	gob.Register(&TimerRes{})
}

// TimerRes is a timer resource for time based events.
type TimerRes struct {
	BaseRes  `yaml:",inline"`
	Interval int `yaml:"interval"` // Interval : Interval between runs
}

// TimerUUID is the UUID struct for TimerRes.
type TimerUUID struct {
	BaseUUID
	name string
}

// NewTimerRes creates a new TimerRes.
func NewTimerRes(name string, interval int) *TimerRes {
	obj := &TimerRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Interval: interval,
	}
	obj.Init()
	return obj
}

func (obj *TimerRes) Init() {
	obj.BaseRes.kind = "Timer"
	obj.BaseRes.Init() // call base init, b/c we're overrriding
}

// Validate the params that are passed to TimerRes
// Currently we are getting only an interval in seconds
// which gets validated by go compiler
func (obj *TimerRes) Validate() bool {
	return true
}

func (obj *TimerRes) Watch(processChan chan Event) {
	if obj.IsWatching() {
		return
	}

	// Create a time.Ticker for the given interval
	ticker := time.NewTicker(time.Duration(obj.Interval) * time.Second)
	defer ticker.Stop()

	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var send = false

	for {
		obj.SetState(resStateWatching)
		select {
		case <-ticker.C: // received the timer event
			send = true
			log.Printf("%v[%v]: received tick", obj.Kind(), obj.GetName())
		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, _ := obj.ReadEvent(&event); exit {
				return
			}
		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true)
			continue
		}
		if send {
			send = false
			obj.isStateOK = false
			resp := NewResp()
			processChan <- Event{eventNil, resp, "timer ticked", true}
			resp.ACKWait()
		}
	}
}

func (obj *TimerRes) GetUUIDs() []ResUUID {
	x := &TimerUUID{
		BaseUUID: BaseUUID{
			name: obj.GetName(),
			kind: obj.Kind(),
		},
		name: obj.Name,
	}
	return []ResUUID{x}
}

// The AutoEdges method returns the AutoEdges. In this case none are used.
func (obj *TimerRes) AutoEdges() AutoEdge {
	return nil
}

func (obj *TimerRes) Compare(res Res) bool {
	switch res.(type) {
	case *TimerRes:
		res := res.(*TimerRes)
		if !obj.BaseRes.Compare(res) {
			return false
		}
		if obj.Name != res.Name {
			return false
		}
		if obj.Interval != res.Interval {
			return false
		}
	default:
		return false
	}
	return true
}

// CheckApply method for Timer resource. Does nothing, returns happy!
func (obj *TimerRes) CheckApply(apply bool) (bool, error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)
	return true, nil // state is always okay
}

func (obj *TimerRes) CollectPatten(pattern string) {
	return
}
