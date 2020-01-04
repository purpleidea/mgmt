// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package traits

import (
	"fmt"

	"github.com/purpleidea/mgmt/engine"
)

// Groupable contains a general implementation with most of the properties and
// methods needed to support autogrouping on resources. It may be used as a
// starting point to avoid re-implementing the straightforward methods.
type Groupable struct {
	// Xmeta is the stored meta. It should be called `meta` but it must be
	// public so that the `encoding/gob` package can encode it properly.
	Xmeta *engine.AutoGroupMeta

	isGrouped bool                  // am i contained within a group?
	grouped   []engine.GroupableRes // list of any grouped resources

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// AutoGroupMeta lets you get or set meta params for the automatic grouping
// trait.
func (obj *Groupable) AutoGroupMeta() *engine.AutoGroupMeta {
	if obj.Xmeta == nil { // set the defaults if previously empty
		obj.Xmeta = &engine.AutoGroupMeta{
			Disabled: false,
		}
	}
	return obj.Xmeta
}

// SetAutoGroupMeta lets you set all of the meta params for the automatic
// grouping trait in a single call.
func (obj *Groupable) SetAutoGroupMeta(meta *engine.AutoGroupMeta) {
	obj.Xmeta = meta
}

// GroupCmp compares two resources and decides if they're suitable for grouping.
// You'll probably want to override this method when implementing a resource...
// This base implementation assumes not, so override me!
func (obj *Groupable) GroupCmp(res engine.GroupableRes) error {
	return fmt.Errorf("the default grouping compare is not nil")
}

// GroupRes groups resource argument (res) into self.
func (obj *Groupable) GroupRes(res engine.GroupableRes) error {
	if l := len(res.GetGroup()); l > 0 {
		return fmt.Errorf("the `%s` resource already contains %d grouped resources", res, l)
	}
	if res.IsGrouped() {
		return fmt.Errorf("the `%s` resource is already grouped", res)
	}

	obj.grouped = append(obj.grouped, res)
	res.SetGrouped(true) // i am contained _in_ a group
	return nil
}

// IsGrouped determines if we are grouped.
func (obj *Groupable) IsGrouped() bool { // am I grouped?
	return obj.isGrouped
}

// SetGrouped sets a flag to tell if we are grouped.
func (obj *Groupable) SetGrouped(b bool) {
	obj.isGrouped = b
}

// GetGroup returns everyone grouped inside me.
func (obj *Groupable) GetGroup() []engine.GroupableRes {
	return obj.grouped
}

// SetGroup sets the grouped resources into me.
func (obj *Groupable) SetGroup(grouped []engine.GroupableRes) {
	obj.grouped = grouped
}
