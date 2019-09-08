// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package graph

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ReverseFile is the file name in the resource state dir where any
	// reversal information is stored.
	ReverseFile = "reverse"

	// ReversePerm is the permissions mode used to create the ReverseFile.
	ReversePerm = 0600
)

// Reversals adds the reversals onto the loaded graph. This should happen last,
// and before Commit.
func (obj *Engine) Reversals() error {
	if obj.nextGraph == nil {
		return fmt.Errorf("there is no active graph to add reversals to")
	}

	// Initially get all of the reversals to seek out all possible errors.
	// XXX: The engine needs to know where data might have been stored if we
	// XXX: want to potentially allow alternate read/write paths, like etcd.
	// XXX: In this scenario, we'd have to store a token somewhere to let us
	// XXX: know to look elsewhere for the special ReversalList read method.
	data, err := obj.ReversalList() // (map[string]string, error)
	if err != nil {
		return errwrap.Wrapf(err, "the reversals had errors")
	}

	if len(data) == 0 {
		return nil // end early
	}

	resMatch := func(r1, r2 engine.Res) bool { // simple match on UID only!
		if r1.Kind() != r2.Kind() {
			return false
		}
		if r1.Name() != r2.Name() {
			return false
		}
		return true
	}
	resInList := func(needle engine.Res, haystack []engine.Res) bool {
		for _, res := range haystack {
			if resMatch(needle, res) {
				return true
			}
		}
		return false
	}

	if obj.Debug {
		obj.Logf("decoding %d reversals...", len(data))
	}
	resources := []engine.Res{}

	// do this in a sorted order so that it errors deterministically
	sorted := []string{}
	for key := range data {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		val := data[key]
		// XXX: replace this ResToB64 method with one that stores it in
		// a human readable format, in case someone wants to hack and
		// edit it manually.
		// XXX: we probably want this to be YAML, it works with the diff
		// too...
		r, err := engineUtil.B64ToRes(val)
		if err != nil {
			return errwrap.Wrapf(err, "error decoding res with UID: `%s`", key)
		}

		res, ok := r.(engine.ReversibleRes)
		if !ok {
			// this requirement is here to keep things simpler...
			return errwrap.Wrapf(err, "decoded res with UID: `%s` was not reversible", key)
		}

		matchFn := func(vertex pgraph.Vertex) (bool, error) {
			r, ok := vertex.(engine.Res)
			if !ok {
				return false, fmt.Errorf("not a Res")
			}
			if !resMatch(r, res) {
				return false, nil
			}
			return true, nil
		}

		// FIXME: not efficient, we could build a cache-map first
		vertex, err := obj.nextGraph.VertexMatchFn(matchFn) // (Vertex, error)
		if err != nil {
			return errwrap.Wrapf(err, "error searching graph for match")
		}
		if vertex != nil { // found one!
			continue // it doesn't need reversing yet
		}

		// TODO: check for (incompatible?) duplicates instead
		if resInList(res, resources) { // we've already got this one...
			continue
		}

		// We set this in two different places to be safe. It ensures
		// that we erase the reversal state file after we've used it.
		res.ReversibleMeta().Reversal = true // set this for later...

		resources = append(resources, res)
	}

	if len(resources) == 0 {
		return nil // end early
	}

	// Now that we've passed the chance of any errors, we modify the graph.
	obj.Logf("adding %d reversals...", len(resources))
	for _, res := range resources {
		obj.nextGraph.AddVertex(res)
	}
	// TODO: Do we want a way for stored reversals to add edges too?

	// It would be great to ensure we didn't add any loops here, but instead
	// of checking now, we'll move the check into the main loop.
	return nil
}

// ReversalList returns all the available pending reversal data on this host. It
// can then be decoded by whatever method is appropriate for.
func (obj *Engine) ReversalList() (map[string]string, error) {
	result := make(map[string]string) // some key to contents

	dir := obj.statePrefix() // loop through this dir...
	files, err := ioutil.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errwrap.Wrapf(err, "error reading list of state dirs")
	} else if err != nil {
		return result, nil // nothing found, no state dir exists yet
	}

	for _, x := range files {
		key := x.Name() // some uid for the resource
		file := path.Join(dir, key, ReverseFile)
		content, err := ioutil.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			return nil, errwrap.Wrapf(err, "could not read reverse file: %s", file)
		} else if err != nil {
			continue // file does not exist, skip
		}

		// file exists!
		str := string(content)
		result[key] = str // save
	}

	return result, nil
}

// ReversalInit performs the reversal initialization steps if necessary for this
// resource.
func (obj *State) ReversalInit() error {
	res, ok := obj.Vertex.(engine.ReversibleRes)
	if !ok {
		return nil // nothing to do
	}

	if res.ReversibleMeta().Disabled {
		return nil // nothing to do, reversal isn't enabled
	}

	// If the reversal is enabled, but we are the result of a previous
	// reversal, then this will overwrite that older reversal request, and
	// our resource should be designed to deal with that. This happens if we
	// return a reversible resource as the reverse of a resource that was
	// reversed. It's probably fairly rare.
	if res.ReversibleMeta().Reversal {
		obj.Logf("triangle reversal") // warn!
	}

	r, err := res.Reversed()
	if err != nil {
		return errwrap.Wrapf(err, "could not reverse: %s", res.String())
	}
	if r == nil {
		return nil // this can't be reversed, or isn't implemented here
	}

	// We set this in two different places to be safe. It ensures that we
	// erase the reversal state file after we've used it.
	r.ReversibleMeta().Reversal = true // set this for later...

	// XXX: replace this ResToB64 method with one that stores it in a human
	// readable format, in case someone wants to hack and edit it manually.
	// XXX: we probably want this to be YAML, it works with the diff too...
	str, err := engineUtil.ResToB64(r)
	if err != nil {
		return errwrap.Wrapf(err, "could not encode: %s", res.String())
	}

	// TODO: put this method on traits.Reversible as part of the interface?
	return obj.ReversalWrite(str, res.ReversibleMeta().Overwrite) // Store!
}

// ReversalClose performs the reversal shutdown steps if necessary for this
// resource.
func (obj *State) ReversalClose() error {
	res, ok := obj.Vertex.(engine.ReversibleRes)
	if !ok {
		return nil // nothing to do
	}

	// Don't check res.ReversibleMeta().Disabled because we're removing the
	// previous one. That value only applies if we're doing a new reversal.

	if !res.ReversibleMeta().Reversal {
		return nil // nothing to erase, we're not a reversal resource
	}

	if !obj.isStateOK { // did we successfully reverse?
		obj.Logf("did not complete reversal") // warn
		return nil
	}

	// TODO: put this method on traits.Reversible as part of the interface?
	return obj.ReversalDelete() // Erase our reversal instructions.
}

// ReversalWrite stores the reversal state information for this resource.
func (obj *State) ReversalWrite(str string, overwrite bool) error {
	dir, err := obj.varDir("") // private version
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir for reverse")
	}
	file := path.Join(dir, ReverseFile) // return a unique file

	content, err := ioutil.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return errwrap.Wrapf(err, "could not read reverse file: %s", file)
	}

	// file exists and we shouldn't overwrite if different
	if err == nil && !overwrite {
		// compare to existing file
		oldStr := string(content)
		if str != oldStr {
			obj.Logf("existing, pending, reversible resource exists")
			//obj.Logf("diff:")
			//obj.Logf("") // TODO: print the diff w/o and secret values
			return fmt.Errorf("existing, pending, reversible resource exists")
		}
	}

	return ioutil.WriteFile(file, []byte(str), ReversePerm)
}

// ReversalDelete removes the reversal state information for this resource.
func (obj *State) ReversalDelete() error {
	dir, err := obj.varDir("") // private version
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir for reverse")
	}
	file := path.Join(dir, ReverseFile) // return a unique file

	return errwrap.Wrapf(os.Remove(file), "could not remove reverse state file")
}
