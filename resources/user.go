// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/purpleidea/mgmt/recwatch"

	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("user", func() Res { return &UserRes{} })
}

const passwdFile = "/etc/passwd"

// UserRes is a user account resource.
type UserRes struct {
	BaseRes           `yaml:",inline"`
	State             string   `yaml:"state"`             // state: exists, absent
	UID               *uint32  `yaml:"uid"`               // uid must be unique unless AllowDuplicateUID is true
	GID               *uint32  `yaml:"gid"`               // gid of the user's primary group
	Group             *string  `yaml:"group"`             // name of the user's primary group
	Groups            []string `yaml:"groups"`            // list of supplemental groups
	HomeDir           *string  `yaml:"homedir"`           // path to the user's home directory
	AllowDuplicateUID bool     `yaml:"allowduplicateuid"` // allow duplicate uid

	recWatcher *recwatch.RecWatcher
}

// Default returns some sensible defaults for this resource.
func (obj *UserRes) Default() Res {
	return &UserRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *UserRes) Validate() error {
	const whitelist string = "_abcdefghijklmnopqrstuvwxyz0123456789"

	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be 'exists' or 'absent'")
	}
	if obj.GID != nil && obj.Group != nil {
		return fmt.Errorf("cannot use both GID and Group")
	}
	if obj.Group != nil {
		if *obj.Group == "" {
			return fmt.Errorf("group cannot be empty string")
		}
		for _, char := range *obj.Group {
			if !strings.Contains(whitelist, string(char)) {
				return fmt.Errorf("group contains invalid character(s)")
			}
		}
	}
	if obj.Groups != nil {
		for _, group := range obj.Groups {
			if group == "" {
				return fmt.Errorf("group cannot be empty string")
			}
			for _, char := range group {
				if !strings.Contains(whitelist, string(char)) {
					return fmt.Errorf("groups list contains invalid character(s)")
				}
			}
		}
	}
	return obj.BaseRes.Validate()
}

// Init initializes the resource.
func (obj *UserRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *UserRes) Watch() error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(passwdFile, false)
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
			log.Printf("%s: Watching: %s", obj, passwdFile) // attempting to watch...
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

// CheckApply method for User resource.
func (obj *UserRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply(%t)", obj, apply)

	var exists = true
	usr, err := user.Lookup(obj.GetName())
	if err != nil {
		if _, ok := err.(user.UnknownUserError); !ok {
			return false, errwrap.Wrapf(err, "error looking up user")
		}
		exists = false
	}

	if obj.AllowDuplicateUID == false && obj.UID != nil {
		existingUID, err := user.LookupId(strconv.Itoa(int(*obj.UID)))
		if err != nil {
			if _, ok := err.(user.UnknownUserIdError); !ok {
				return false, errwrap.Wrapf(err, "error looking up UID")
			}
		} else if existingUID.Username != obj.GetName() {
			return false, fmt.Errorf("the requested UID is already taken")
		}
	}

	if obj.State == "absent" && !exists {
		return true, nil
	}

	if usercheck := true; exists && obj.State == "exists" {
		intUID, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return false, errwrap.Wrapf(err, "error casting UID to int")
		}
		intGID, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return false, errwrap.Wrapf(err, "error casting GID to int")
		}
		if obj.UID != nil && int(*obj.UID) != intUID {
			usercheck = false
		}
		if obj.GID != nil && int(*obj.GID) != intGID {
			usercheck = false
		}
		if obj.HomeDir != nil && *obj.HomeDir != usr.HomeDir {
			usercheck = false
		}
		if usercheck {
			return true, nil
		}
	}

	if !apply {
		return false, nil
	}

	var cmdName string
	var args []string
	if obj.State == "exists" {
		if exists {
			cmdName = "usermod"
			log.Printf("%s: Modifying user: %s", obj, obj.GetName())
		} else {
			cmdName = "useradd"
			log.Printf("%s: Adding user: %s", obj, obj.GetName())
		}
		if obj.AllowDuplicateUID {
			args = append(args, "--non-unique")
		}
		if obj.UID != nil {
			args = append(args, "-u", fmt.Sprintf("%d", *obj.UID))
		}
		if obj.GID != nil {
			args = append(args, "-g", fmt.Sprintf("%d", *obj.GID))
		}
		if obj.Group != nil {
			args = append(args, "-g", *obj.Group)
		}
		if obj.Groups != nil {
			args = append(args, "-G", strings.Join(obj.Groups, ","))
		}
		if obj.HomeDir != nil {
			args = append(args, "-d", *obj.HomeDir)
		}
	}
	if obj.State == "absent" {
		cmdName = "userdel"
		log.Printf("%s: Deleting user: %s", obj, obj.GetName())
	}

	args = append(args, obj.GetName())

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

// UserUID is the UID struct for UserRes.
type UserUID struct {
	BaseUID
	name string
}

// UserResAutoEdges holds the state of the auto edge generator.
type UserResAutoEdges struct {
	UIDs    []ResUID
	pointer int
}

// AutoEdges returns edges from the user resource to each group found in
// its definition. The groups can be in any of the three applicable fields
// (GID, Group and Groups.) If the user exists, reversed ensures the edge
// goes from group to user, and if the user is absent the edge goes from
// user to group. This ensures that we don't add users to groups that
// don't exist or delete groups before we delete their members.
func (obj *UserRes) AutoEdges() (AutoEdge, error) {
	var result []ResUID
	var reversed bool
	if obj.State == "exists" {
		reversed = true
	}
	if obj.GID != nil {
		result = append(result, &GroupUID{
			BaseUID: BaseUID{
				Reversed: &reversed,
			},
			gid: obj.GID,
		})
	}
	if obj.Group != nil {
		result = append(result, &GroupUID{
			BaseUID: BaseUID{
				Reversed: &reversed,
			},
			name: *obj.Group,
		})
	}
	for _, group := range obj.Groups {
		result = append(result, &GroupUID{
			BaseUID: BaseUID{
				Reversed: &reversed,
			},
			name: group,
		})
	}
	return &UserResAutoEdges{
		UIDs:    result,
		pointer: 0,
	}, nil
}

// Next returns the next automatic edge.
func (obj *UserResAutoEdges) Next() []ResUID {
	if len(obj.UIDs) == 0 {
		return nil
	}
	value := obj.UIDs[obj.pointer]
	obj.pointer++
	return []ResUID{value}
}

// Test gets results of the earlier Next() call, & returns if we should continue.
func (obj *UserResAutoEdges) Test(input []bool) bool {
	if len(obj.UIDs) <= obj.pointer {
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		log.Fatal("Expecting a single value!")
	}
	return true // keep going
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *UserRes) UIDs() []ResUID {
	x := &UserUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *UserRes) GroupCmp(r Res) bool {
	_, ok := r.(*UserRes)
	if !ok {
		return false
	}
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *UserRes) Compare(r Res) bool {
	// we can only compare UserRes to others of the same resource kind
	res, ok := r.(*UserRes)
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
	if (obj.UID == nil) != (res.UID == nil) {
		return false
	}
	if obj.UID != nil && res.UID != nil {
		if *obj.UID != *res.UID {
			return false
		}
	}
	if (obj.GID == nil) != (res.GID == nil) {
		return false
	}
	if obj.GID != nil && res.GID != nil {
		if *obj.GID != *res.GID {
			return false
		}
	}
	if (obj.Groups == nil) != (res.Groups == nil) {
		return false
	}
	if obj.Groups != nil && res.Groups != nil {
		if len(obj.Groups) != len(res.Groups) {
			return false
		}
		objGroups := obj.Groups
		resGroups := res.Groups
		sort.Strings(objGroups)
		sort.Strings(resGroups)
		for i := range objGroups {
			if objGroups[i] != resGroups[i] {
				return false
			}
		}
	}
	if (obj.HomeDir == nil) != (res.HomeDir == nil) {
		return false
	}
	if obj.HomeDir != nil && res.HomeDir != nil {
		if *obj.HomeDir != *obj.HomeDir {
			return false
		}
	}
	if obj.AllowDuplicateUID != res.AllowDuplicateUID {
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *UserRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes UserRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*UserRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to UserRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = UserRes(raw) // restore from indirection with type conversion!
	return nil
}
