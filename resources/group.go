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

package resources

import (
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/purpleidea/mgmt/recwatch"

	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("group", func() Res { return &GroupRes{} })
}

const groupFile = "/etc/group"

// GroupRes is a user group resource.
type GroupRes struct {
	BaseRes `yaml:",inline"`
	State   string  `yaml:"state"` // state: exists, absent
	GID     *uint32 `yaml:"gid"`   // the group's gid

	recWatcher *recwatch.RecWatcher
}

// Default returns some sensible defaults for this resource.
func (obj *GroupRes) Default() Res {
	return &GroupRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *GroupRes) Validate() error {
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("State must be 'exists' or 'absent'")
	}
	return obj.BaseRes.Validate()
}

// Init initializes the resource.
func (obj *GroupRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *GroupRes) Watch() error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(groupFile, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error

	for {
		if obj.debug {
			log.Printf("%s: Watching: %s", obj, groupFile) // attempting to watch...
		}

		select {
		case event, ok := <-obj.recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", obj)
			}
			if obj.debug { // don't access event.Body if event.Error isn't nil
				log.Printf("%s: Event(%s): %v", obj, event.Body.Name, event.Body.Op)
			}
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
			//obj.StateOK(false) // dirty // these events don't invalidate state
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply method for Group resource.
func (obj *GroupRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply(%t)", obj, apply)

	// check if the group exists
	exists := true
	group, err := user.LookupGroup(obj.GetName())
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
		if lookupGID != nil && lookupGID.Name != obj.GetName() {
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
			log.Printf("%s: Inconsistent GID: %s", obj, obj.GetName())
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
	args := []string{obj.GetName()}

	if obj.State == "exists" {
		if exists {
			log.Printf("%s: Modifying group: %s", obj, obj.GetName())
			cmdName = "groupmod"
		} else {
			log.Printf("%s: Adding group: %s", obj, obj.GetName())
			cmdName = "groupadd"
		}
		if obj.GID != nil {
			args = append(args, "-g", fmt.Sprintf("%d", *obj.GID))
		}
	}
	if obj.State == "absent" && exists {
		log.Printf("%s: Deleting group: %s", obj, obj.GetName())
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

// GroupUID is the UID struct for GroupRes.
type GroupUID struct {
	BaseUID
	name string
	gid  *uint32
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *GroupUID) IFF(uid ResUID) bool {
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

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *GroupRes) UIDs() []ResUID {
	x := &GroupUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
		gid:     obj.GID,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *GroupRes) GroupCmp(r Res) bool {
	_, ok := r.(*GroupRes)
	if !ok {
		return false
	}
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *GroupRes) Compare(r Res) bool {
	// we can only compare GroupRes to others of the same resource kind
	res, ok := r.(*GroupRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}
	if obj.State != res.State {
		return false
	}
	if (obj.GID == nil) != (res.GID == nil) {
		return false
	}
	if obj.GID != nil && res.GID != nil {
		if *obj.GID != *res.GID {
			return false
		}
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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
