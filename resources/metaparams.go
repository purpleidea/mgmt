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
	"golang.org/x/time/rate"
)

// MetaParams is a struct will all params that apply to every resource.
type MetaParams struct {
	AutoEdge  bool `yaml:"autoedge"`  // metaparam, should we generate auto edges?
	AutoGroup bool `yaml:"autogroup"` // metaparam, should we auto group?
	Noop      bool `yaml:"noop"`
	// NOTE: there are separate Watch and CheckApply retry and delay values,
	// but I've decided to use the same ones for both until there's a proper
	// reason to want to do something differently for the Watch errors.
	Retry int16      `yaml:"retry"` // metaparam, number of times to retry on error. -1 for infinite
	Delay uint64     `yaml:"delay"` // metaparam, number of milliseconds to wait between retries
	Poll  uint32     `yaml:"poll"`  // metaparam, number of seconds between poll intervals, 0 to watch
	Limit rate.Limit `yaml:"limit"` // metaparam, number of events per second to allow through
	Burst int        `yaml:"burst"` // metaparam, number of events to allow in a burst
	Sema  []string   `yaml:"sema"`  // metaparam, list of semaphore ids (id | id:count)
}

// UnmarshalYAML is the custom unmarshal handler for the MetaParams struct. It
// is primarily useful for setting the defaults.
func (obj *MetaParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawMetaParams MetaParams           // indirection to avoid infinite recursion
	raw := rawMetaParams(DefaultMetaParams) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = MetaParams(raw) // restore from indirection with type conversion!
	return nil
}

// DefaultMetaParams are the defaults to be used for undefined metaparams.
var DefaultMetaParams = MetaParams{
	AutoEdge:  true,
	AutoGroup: true,
	Noop:      false,
	Retry:     0,        // TODO: is this a good default?
	Delay:     0,        // TODO: is this a good default?
	Poll:      0,        // defaults to watching for events
	Limit:     rate.Inf, // defaults to no limit
	Burst:     0,        // no burst needed on an infinite rate // TODO: is this a good default?
	//Sema:      []string{},
}
