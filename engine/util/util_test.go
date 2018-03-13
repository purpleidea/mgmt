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

package util

import (
	"os/user"
	"strconv"
	"testing"
)

func TestUnknownGroup(t *testing.T) {
	gid, err := GetGID("unknowngroup")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", gid)
	}
}

func TestUnknownUser(t *testing.T) {
	uid, err := GetUID("unknownuser")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", uid)
	}
}

func TestCurrentUserGroupByName(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via username and group name)
	if uid, err = GetUID(userObj.Username); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	// macOS users do not have a group with their name on it, so not assuming this here
	group, err := user.LookupGroupId(currentGID)
	if err != nil {
		t.Errorf("failed to lookup group by id: %s", currentGID)
	}
	if gid, err = GetGID(group.Name); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}

func TestCurrentUserGroupById(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via uid and gid)
	if uid, err = GetUID(currentUID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	if gid, err = GetGID(currentGID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}
