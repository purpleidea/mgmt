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
	parent    engine.GroupableRes   // resource i am grouped inside of

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

// GroupRes groups resource argument (res) into self. Callers of this method
// should probably also run SetParent.
func (obj *Groupable) GroupRes(res engine.GroupableRes) error {
	// We can keep this check with hierarchical grouping by adding in the
	// Kind test which we seen inside... If they're all the same, then we
	// can't do it. But if they're dissimilar, then it's okay to group!
	if l := len(res.GetGroup()); l > 0 {
		kind := res.Kind()
		ok := true // assume okay for now
		for _, r := range res.GetGroup() {
			if r.Kind() == kind {
				ok = false // non-hierarchical grouping, error!
			}
		}
		// XXX: Why is it not acceptable to allow hierarchical grouping,
		// AND self-kind grouping together? For example, group
		// http:server:flag with another flag, and then group that group
		// inside http:server!
		if !ok && false { // XXX: disabled this check for now...
			return fmt.Errorf("the `%s` resource already contains %d grouped resources", res, l)
		}
	}
	// XXX: Do we need to disable this to support hierarchical grouping?
	//if res.IsGrouped() {
	//	return fmt.Errorf("the `%s` resource is already grouped", res)
	//}

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

// SetGroup sets the grouped resources into me. Callers of this method should
// probably also run SetParent.
func (obj *Groupable) SetGroup(grouped []engine.GroupableRes) {
	obj.grouped = grouped
}

// Parent returns the parent groupable resource that I am inside of.
func (obj *Groupable) Parent() engine.GroupableRes {
	return obj.parent
}

// SetParent tells a particular grouped resource who their parent is.
func (obj *Groupable) SetParent(res engine.GroupableRes) {
	obj.parent = res
}
