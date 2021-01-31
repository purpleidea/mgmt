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

package resources

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("group", func() engine.Res { return &GroupRes{} })
}

const groupFile = "/etc/group"

// GroupRes is a user group resource.
type GroupRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	State string  `yaml:"state"` // state: exists, absent
	GID   *uint32 `yaml:"gid"`   // the group's gid

	recWatcher *recwatch.RecWatcher
}

// Default returns some sensible defaults for this resource.
func (obj *GroupRes) Default() engine.Res {
	return &GroupRes{}
}

// Validate if the params passed in are valid data.
func (obj *GroupRes) Validate() error {
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be 'exists' or 'absent'")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *GroupRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *GroupRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *GroupRes) Watch() error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(groupFile, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("Watching: %s", groupFile) // attempting to watch...
		}

		select {
		case event, ok := <-obj.recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("Event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply method for Group resource.
func (obj *GroupRes) CheckApply(apply bool) (bool, error) {
	obj.init.Logf("CheckApply(%t)", apply)

	// check if the group exists
	exists := true
	group, err := user.LookupGroup(obj.Name())
	if err != nil {
		if _, ok := err.(user.UnknownGroupError); !ok {
			return false, errwrap.Wrapf(err, "error looking up group")
		}
		exists = false
	}
	// if the group doesn't exist and should be absent, we are done
	if obj.State == "absent" && !exists {
		return true, nil
	}
	// if the group exists and no GID is specified, we are done
	if obj.State == "exists" && exists && obj.GID == nil {
		return true, nil
	}
	if exists && obj.GID != nil {
		// check if GID is taken
		lookupGID, err := user.LookupGroupId(strconv.Itoa(int(*obj.GID)))
		if err != nil {
			if _, ok := err.(user.UnknownGroupIdError); !ok {
				return false, errwrap.Wrapf(err, "error looking up GID")
			}
		}
		if lookupGID != nil && lookupGID.Name != obj.Name() {
			return false, fmt.Errorf("the requested GID belongs to another group")
		}
		// get the existing group's GID
		existingGID, err := strconv.ParseUint(group.Gid, 10, 32)
		if err != nil {
			return false, errwrap.Wrapf(err, "error casting existing GID")
		}
		// check if existing group has the wrong GID
		// if it is wrong groupmod will change it to the desired value
		if *obj.GID != uint32(existingGID) {
			obj.init.Logf("Inconsistent GID: %s", obj.Name())
		}
		// if the group exists and has the correct GID, we are done
		if obj.State == "exists" && *obj.GID == uint32(existingGID) {
			return true, nil
		}
	}

	if !apply {
		return false, nil
	}

	var cmdName string
	args := []string{obj.Name()}

	if obj.State == "exists" {
		if exists {
			obj.init.Logf("Modifying group: %s", obj.Name())
			cmdName = "groupmod"
		} else {
			obj.init.Logf("Adding group: %s", obj.Name())
			cmdName = "groupadd"
		}
		if obj.GID != nil {
			args = append(args, "-g", fmt.Sprintf("%d", *obj.GID))
		}
	}
	if obj.State == "absent" && exists {
		obj.init.Logf("Deleting group: %s", obj.Name())
		cmdName = "groupdel"
	}

	cmd := exec.Command(cmdName, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// open a pipe to get error messages from os/exec
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to initialize stderr pipe")
	}

	// start the command
	if err := cmd.Start(); err != nil {
		return false, errwrap.Wrapf(err, "cmd failed to start")
	}
	// capture any error messages
	slurp, err := ioutil.ReadAll(stderr)
	if err != nil {
		return false, errwrap.Wrapf(err, "error slurping error message")
	}
	// wait until cmd exits and return error message if any
	if err := cmd.Wait(); err != nil {
		return false, errwrap.Wrapf(err, "%s", slurp)
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *GroupRes) Cmp(r engine.Res) error {
	// we can only compare GroupRes to others of the same resource kind
	res, ok := r.(*GroupRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if (obj.GID == nil) != (res.GID == nil) {
		return fmt.Errorf("the GID differs")
	}
	if obj.GID != nil && res.GID != nil {
		if *obj.GID != *res.GID {
			return fmt.Errorf("the GID differs")
		}
	}
	return nil
}

// GroupUID is the UID struct for GroupRes.
type GroupUID struct {
	engine.BaseUID
	name string
	gid  *uint32
}

// AutoEdges returns the AutoEdge interface.
func (obj *GroupRes) AutoEdges() (engine.AutoEdge, error) {
	return nil, nil
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *GroupUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*GroupUID)
	if !ok {
		return false
	}
	if obj.gid != nil && res.gid != nil {
		if *obj.gid != *res.gid {
			return false
		}
	}
	if obj.name != "" && res.name != "" {
		if obj.name != res.name {
			return false
		}
	}
	return true
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *GroupRes) UIDs() []engine.ResUID {
	x := &GroupUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
		gid:     obj.GID,
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *GroupRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes GroupRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*GroupRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to GroupRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = GroupRes(raw) // restore from indirection with type conversion!
	return nil
}
