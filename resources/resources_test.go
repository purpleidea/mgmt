// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"testing"
)

func TestCompare1(t *testing.T) {
	r1 := &NoopRes{}
	r2 := &NoopRes{}
	r3 := &FileRes{}

	if !r1.Compare(r2) || !r2.Compare(r1) {
		t.Error("The two resources do not match!")
	}

	if r1.Compare(r3) || r3.Compare(r1) {
		t.Error("The two resources should not match!")
	}
}

func TestCompare2(t *testing.T) {
	r1 := &NoopRes{
		BaseRes: BaseRes{
			Name: "noop1",
			Kind: "noop",
			MetaParams: MetaParams{
				Noop: true,
			},
		},
	}
	r2 := &NoopRes{
		BaseRes: BaseRes{
			Name: "noop1", // same name
			Kind: "noop",
			MetaParams: MetaParams{
				Noop: false, // different noop
			},
		},
	}

	if !r2.Compare(r1) { // going from noop(false) -> noop(true) is okay!
		t.Error("The two resources do not match!")
	}

	if r1.Compare(r2) { // going from noop(true) -> noop(false) is not okay!
		t.Error("The two resources should not match!")
	}
}

func TestIFF(t *testing.T) {
	uid := &BaseUID{Name: "/tmp/unit-test"}
	same := &BaseUID{Name: "/tmp/unit-test"}
	diff := &BaseUID{Name: "/tmp/other-file"}

	if !uid.IFF(same) {
		t.Error("basic resource UIDs with the same name should satisfy each other's IFF condition.")
	}

	if uid.IFF(diff) {
		t.Error("basic resource UIDs with different names should NOT satisfy each other's IFF condition.")
	}
}

func TestReadEvent(t *testing.T) {
	//res := FileRes{}

	//shouldExit := map[event.Kind]bool{
	//	event.EventStart:    false,
	//	event.EventPoke:     false,
	//	event.EventBackPoke: false,
	//	event.EventExit:     true,
	//}
	//shouldPoke := map[event.Kind]bool{
	//	event.EventStart:    true,
	//	event.EventPoke:     true,
	//	event.EventBackPoke: true,
	//	event.EventExit:     false,
	//}

	//for ev := range shouldExit {
	//	exit, poke := res.ReadEvent(&event.Event{Kind: ev})
	//	if exit != shouldExit[ev] {
	//		t.Errorf("resource.ReadEvent returned wrong exit flag for a %v event (%v, should be %v)",
	//			ev, exit, shouldExit[ev])
	//	}
	//	if poke != shouldPoke[ev] {
	//		t.Errorf("resource.ReadEvent returned wrong poke flag for a %v event (%v, should be %v)",
	//			ev, poke, shouldPoke[ev])
	//	}
	//}

	//res.Init()
	//res.SetWatching(true)

	// test result when a pause event is followed by start
	//go res.SendEvent(event.EventStart, nil)
	//exit, poke := res.ReadEvent(&event.Event{Kind: event.EventPause})
	//if exit {
	//	t.Error("resource.ReadEvent returned wrong exit flag for a pause+start event (true, should be false)")
	//}
	//if poke {
	//	t.Error("resource.ReadEvent returned wrong poke flag for a pause+start event (true, should be false)")
	//}

	// test result when a pause event is followed by exit
	//go res.SendEvent(event.EventExit, nil)
	//exit, poke = res.ReadEvent(&event.Event{Kind: event.EventPause})
	//if !exit {
	//	t.Error("resource.ReadEvent returned wrong exit flag for a pause+start event (false, should be true)")
	//}
	//if poke {
	//	t.Error("resource.ReadEvent returned wrong poke flag for a pause+start event (true, should be false)")
	//}

	// TODO: create a wrapper API around log, so that Fatals can be mocked and tested
}
