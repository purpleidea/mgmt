// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package engine

import (
	"fmt"
	"strconv"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"golang.org/x/time/rate"
)

// DefaultMetaParams are the defaults that are used for undefined metaparams.
// Don't modify this variable. Use .Copy() if you'd like some for yourself.
var DefaultMetaParams = &MetaParams{
	Noop:  false,
	Retry: 0,
	Delay: 0,
	Poll:  0,        // defaults to watching for events
	Limit: rate.Inf, // defaults to no limit
	Burst: 0,        // no burst needed on an infinite rate
	//Sema:  []string{},
	Rewatch: true,
	Realize: false, // true would be more awesome, but unexpected for users
}

// MetaRes is the interface a resource must implement to support meta params.
// All resources must implement this.
type MetaRes interface {
	// MetaParams lets you get or set meta params for the resource.
	MetaParams() *MetaParams

	// SetMetaParams lets you set all of the meta params for the resource in
	// a single call.
	SetMetaParams(*MetaParams)
}

// MetaParams provides some meta parameters that apply to every resource.
type MetaParams struct {
	// Noop specifies that no changes should be made by the resource. It
	// relies on the individual resource implementation, and can't protect
	// you from a poorly or maliciously implemented resource.
	Noop bool `yaml:"noop"`

	// NOTE: there are separate Watch and CheckApply retry and delay values,
	// but I've decided to use the same ones for both until there's a proper
	// reason to want to do something differently for the Watch errors.

	// Retry is the number of times to retry on error. Use -1 for infinite.
	Retry int16 `yaml:"retry"`

	// Delay is the number of milliseconds to wait between retries.
	Delay uint64 `yaml:"delay"`

	// Poll is the number of seconds between poll intervals. Use 0 to Watch.
	Poll uint32 `yaml:"poll"`

	// Limit is the number of events per second to allow through.
	Limit rate.Limit `yaml:"limit"`

	// Burst is the number of events to allow in a burst.
	Burst int `yaml:"burst"`

	// Sema is a list of semaphore ids in the form `id` or `id:count`. If
	// you don't specify a count, then 1 is assumed. The sema of `foo` which
	// has a count equal to 1, is different from a sema named `foo:1` which
	// also has a count equal to 1, but is a different semaphore.
	Sema []string `yaml:"sema"`

	// Rewatch specifies whether we re-run the Watch worker during a swap
	// if it has errored. When doing a GraphCmp to swap the graphs, if this
	// is true, and this particular worker has errored, then we'll remove it
	// and add it back as a new vertex, thus causing it to run again. This
	// is different from the Retry metaparam which applies during the normal
	// execution. It is only when this is exhausted that we're in permanent
	// worker failure, and only then can we rely on this metaparam.
	Rewatch bool `yaml:"rewatch"`

	// Realize ensures that the resource is guaranteed to converge at least
	// once before a potential graph swap removes or changes it. This
	// guarantee is useful for fast changing graphs, to ensure that the
	// brief creation of a resource is seen. This guarantee does not prevent
	// against the engine quitting normally, and it can't guarantee it if
	// the resource is blocked because of a failed pre-requisite resource.
	// XXX: Not implemented!
	Realize bool `yaml:"realize"`
}

// Cmp compares two AutoGroupMeta structs and determines if they're equivalent.
func (obj *MetaParams) Cmp(meta *MetaParams) error {
	if obj.Noop != meta.Noop {
		return fmt.Errorf("values for Noop are different")
	}
	// XXX: add a one way cmp like we used to have ?
	//if obj.Noop != meta.Noop {
	//	// obj is the existing res, res is the *new* resource
	//	// if we go from no-noop -> noop, we can re-use the obj
	//	// if we go from noop -> no-noop, we need to regenerate
	//	if obj.Noop { // asymmetrical
	//		return fmt.Errorf("values for Noop are different") // going from noop to no-noop!
	//	}
	//}

	if obj.Retry != meta.Retry {
		return fmt.Errorf("values for Retry are different")
	}
	if obj.Delay != meta.Delay {
		return fmt.Errorf("values for Delay are different")
	}
	if obj.Poll != meta.Poll {
		return fmt.Errorf("values for Poll are different")
	}
	if obj.Limit != meta.Limit {
		return fmt.Errorf("values for Limit are different")
	}
	if obj.Burst != meta.Burst {
		return fmt.Errorf("values for Burst are different")
	}

	if err := util.SortedStrSliceCompare(obj.Sema, meta.Sema); err != nil {
		return errwrap.Wrapf(err, "values for Sema are different")
	}

	if obj.Rewatch != meta.Rewatch {
		return fmt.Errorf("values for Rewatch are different")
	}
	if obj.Realize != meta.Realize {
		return fmt.Errorf("values for Realize are different")
	}

	return nil
}

// Validate runs some validation on the meta params.
func (obj *MetaParams) Validate() error {
	if obj.Burst == 0 && !(obj.Limit == rate.Inf) { // blocked
		return fmt.Errorf("permanently limited (rate != Inf, burst = 0)")
	}

	for _, s := range obj.Sema {
		if s == "" {
			return fmt.Errorf("semaphore is empty")
		}
		if _, err := strconv.Atoi(s); err == nil { // standalone int
			return fmt.Errorf("semaphore format is invalid")
		}
	}

	return nil
}

// Copy copies this struct and returns a new one.
func (obj *MetaParams) Copy() *MetaParams {
	sema := []string{}
	if obj.Sema != nil {
		sema = make([]string, len(obj.Sema))
		copy(sema, obj.Sema)
	}
	return &MetaParams{
		Noop:    obj.Noop,
		Retry:   obj.Retry,
		Delay:   obj.Delay,
		Poll:    obj.Poll,
		Limit:   obj.Limit, // FIXME: can we copy this type like this? test me!
		Burst:   obj.Burst,
		Sema:    sema,
		Rewatch: obj.Rewatch,
		Realize: obj.Realize,
	}
}

// UnmarshalYAML is the custom unmarshal handler for the MetaParams struct. It
// is primarily useful for setting the defaults.
// TODO: this is untested
func (obj *MetaParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawMetaParams MetaParams            // indirection to avoid infinite recursion
	raw := rawMetaParams(*DefaultMetaParams) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = MetaParams(raw) // restore from indirection with type conversion!
	return nil
}
