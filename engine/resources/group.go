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

package resources

import (
	"context"
	"fmt"
	"os/user"
	"strconv"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("group", func() engine.Res { return &GroupRes{} })
}

// GroupRes is a user group resource.
type GroupRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	// State is `exists` or `absent`.
	State string `lang:"state" yaml:"state"`

	// GID is the group's gid.
	GID *uint32 `lang:"gid" yaml:"gid"`
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

	if err := util.ValidUser(obj.Name()); err != nil { // groups too
		return fmt.Errorf("group contains invalid character(s)")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *GroupRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *GroupRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *GroupRes) Watch(ctx context.Context) error {
	recWatcher, err := recwatch.NewRecWatcher(util.EtcGroupFile, false)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		if obj.init.Debug {
			obj.init.Logf("watching: %s", util.EtcGroupFile) // attempting to watch...
		}

		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply method for Group resource.
func (obj *GroupRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	user := defaultGroupFuncs // shadows os/user inside this function

	// check if the group exists
	exists := true
	group, err := user.LookupGroup(obj.Name())
	if err != nil {
		if !isUnknownGroup(err) {
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
	// Only enforce GID uniqueness when we plan to create or modify the
	// group. For state=absent with a missing group we returned above and
	// for state=absent with an existing group, we're about to delete it, so
	// a clash on the (about-to-be-released) GID is not our concern.
	if obj.State == "exists" && obj.GID != nil {
		// check if the GID is already taken by a different group
		lookupGID, err := user.LookupGroupId(strconv.Itoa(int(*obj.GID)))
		if err != nil && !isUnknownGroupID(err) {
			return false, errwrap.Wrapf(err, "error looking up GID")
		}
		if err == nil && lookupGID.Name != obj.Name() {
			return false, fmt.Errorf("the requested GID belongs to another group")
		}
	}
	// if the group already exists, compare its GID with the one we want
	if obj.State == "exists" && exists && obj.GID != nil {
		existingGID, err := strconv.ParseUint(group.Gid, 10, 32)
		if err != nil {
			return false, errwrap.Wrapf(err, "error casting existing GID")
		}
		// if the group exists and has the correct GID, we are done
		if *obj.GID == uint32(existingGID) {
			return true, nil
		}
		// otherwise groupmod will change it to the desired value
		obj.init.Logf("Inconsistent GID: %s", obj.Name())
	}

	if !apply {
		return false, nil
	}

	var cmdName string
	var args []string

	if obj.State == "exists" {
		if exists {
			obj.init.Logf("Modifying group: %s", obj.Name())
			cmdName = "groupmod"
		} else {
			obj.init.Logf("Adding group: %s", obj.Name())
			cmdName = "groupadd"
		}
		if obj.GID != nil {
			args = append(args, "-g", strconv.FormatUint(uint64(*obj.GID), 10))
		}
	}
	if obj.State == "absent" && exists {
		obj.init.Logf("Deleting group: %s", obj.Name())
		cmdName = "groupdel"
	}

	args = append(args, obj.Name())

	if err := user.RunCmd(ctx, cmdName, args); err != nil {
		return false, err
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

// isUnknownGroup reports whether err is the os/user "group not found" error.
func isUnknownGroup(err error) bool {
	_, ok := err.(user.UnknownGroupError)
	return ok
}

// isUnknownGroupID reports whether err is the os/user "GID not found" error.
func isUnknownGroupID(err error) bool {
	_, ok := err.(user.UnknownGroupIdError)
	return ok
}

// groupFuncs bundles the os/user and command-runner entry points that
// CheckApply uses, behind func-typed fields. Shadowing `user` inside CheckApply
// with a value of this type swaps the whole bundle at once, which lets tests
// serve lookups from memory and capture the command that would be run.
type groupFuncs struct {
	LookupGroup   func(name string) (*user.Group, error)
	LookupGroupId func(gid string) (*user.Group, error)
	RunCmd        func(ctx context.Context, cmdName string, args []string) error
}

// defaultGroupFuncs is the production wiring of groupFuncs. RunCmd reuses the
// stderr-capturing exec helper from the engine util library.
var defaultGroupFuncs = groupFuncs{
	LookupGroup:   user.LookupGroup,
	LookupGroupId: user.LookupGroupId,
	RunCmd:        engineUtil.RunCmd,
}
