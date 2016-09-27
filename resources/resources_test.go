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

package resources

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"testing"
)

func TestMiscEncodeDecode1(t *testing.T) {
	var err error
	//gob.Register( &NoopRes{} ) // happens in noop.go : init()
	//gob.Register( &FileRes{} ) // happens in file.go : init()
	// ...

	// encode
	var input interface{} = &FileRes{}
	b1 := bytes.Buffer{}
	e := gob.NewEncoder(&b1)
	err = e.Encode(&input) // pass with &
	if err != nil {
		t.Errorf("Gob failed to Encode: %v", err)
	}
	str := base64.StdEncoding.EncodeToString(b1.Bytes())

	// decode
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		t.Errorf("Base64 failed to Decode: %v", err)
	}
	b2 := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b2)
	err = d.Decode(&output) // pass with &
	if err != nil {
		t.Errorf("Gob failed to Decode: %v", err)
	}

	res1, ok := input.(Res)
	if !ok {
		t.Errorf("Input %v is not a Res", res1)
		return
	}
	res2, ok := output.(Res)
	if !ok {
		t.Errorf("Output %v is not a Res", res2)
		return
	}
	if !res1.Compare(res2) {
		t.Error("The input and output Res values do not match!")
	}
}

func TestMiscEncodeDecode2(t *testing.T) {
	var err error
	//gob.Register( &NoopRes{} ) // happens in noop.go : init()
	//gob.Register( &FileRes{} ) // happens in file.go : init()
	// ...

	// encode
	var input Res = &FileRes{}

	b64, err := ResToB64(input)
	if err != nil {
		t.Errorf("Can't encode: %v", err)
		return
	}

	output, err := B64ToRes(b64)
	if err != nil {
		t.Errorf("Can't decode: %v", err)
		return
	}

	res1, ok := input.(Res)
	if !ok {
		t.Errorf("Input %v is not a Res", res1)
		return
	}
	res2, ok := output.(Res)
	if !ok {
		t.Errorf("Output %v is not a Res", res2)
		return
	}
	if !res1.Compare(res2) {
		t.Error("The input and output Res values do not match!")
	}
}

func TestIFF(t *testing.T) {
	uuid := &BaseUUID{name: "/tmp/unit-test"}
	same := &BaseUUID{name: "/tmp/unit-test"}
	diff := &BaseUUID{name: "/tmp/other-file"}

	if !uuid.IFF(same) {
		t.Error("basic resource UUIDs with the same name should satisfy each other's IFF condition.")
	}

	if uuid.IFF(diff) {
		t.Error("basic resource UUIDs with different names should NOT satisfy each other's IFF condition.")
	}
}

func TestReadEvent(t *testing.T) {
	res := FileRes{}

	shouldExit := map[eventName]bool{
		eventStart:    false,
		eventPoke:     false,
		eventBackPoke: false,
		eventExit:     true,
	}
	shouldPoke := map[eventName]bool{
		eventStart:    true,
		eventPoke:     true,
		eventBackPoke: true,
		eventExit:     false,
	}

	for event := range shouldExit {
		exit, poke := res.ReadEvent(&Event{Name: event})
		if exit != shouldExit[event] {
			t.Errorf("resource.ReadEvent returned wrong exit flag for a %v event (%v, should be %v)",
				event, exit, shouldExit[event])
		}
		if poke != shouldPoke[event] {
			t.Errorf("resource.ReadEvent returned wrong poke flag for a %v event (%v, should be %v)",
				event, poke, shouldPoke[event])
		}
	}

	res.Init()
	res.SetWatching(true)

	// test result when a pause event is followed by start
	go res.SendEvent(eventStart, false, false)
	exit, poke := res.ReadEvent(&Event{Name: eventPause})
	if exit {
		t.Error("resource.ReadEvent returned wrong exit flag for a pause+start event (true, should be false)")
	}
	if poke {
		t.Error("resource.ReadEvent returned wrong poke flag for a pause+start event (true, should be false)")
	}

	// test result when a pause event is followed by exit
	go res.SendEvent(eventExit, false, false)
	exit, poke = res.ReadEvent(&Event{Name: eventPause})
	if !exit {
		t.Error("resource.ReadEvent returned wrong exit flag for a pause+start event (false, should be true)")
	}
	if poke {
		t.Error("resource.ReadEvent returned wrong poke flag for a pause+start event (true, should be false)")
	}

	// TODO: create a wrapper API around log, so that Fatals can be mocked and tested
}
