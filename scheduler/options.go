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

package scheduler

import (
	"net/url"
	"strconv"
)

const (
	// DefaultStrategy is the strategy to use if none has been specified.
	DefaultStrategy = "rr"

	// DefaultMaxCount is the maximum number of hosts to schedule on if not
	// specified.
	DefaultMaxCount = 1 // TODO: what is the logical value to choose? +Inf?

	// DefaultPersist is the recommended default persist value.
	DefaultPersist = false // best default

	// DefaultSessionTTL is the number of seconds to wait before a dead or
	// unresponsive host is removed from the scheduled pool.
	DefaultSessionTTL = 10 // seconds
)

// Option is a type that can be used to configure the scheduler.
type Option func(*Options) error

// Options represents the different possible configurable options. Not all
// options necessarily work for each scheduler strategy algorithm.
type Options struct {
	Strategy    string
	MaxCount    *int
	Persist     bool
	SessionTTL  int // TODO: should this be *int to know when it's set?
	HostsFilter []string
	// TODO: add more options
}

// Defaults returns a struct of scheduler options with all the defaults set.
func Defaults() *Options {
	maxCount := DefaultMaxCount
	return &Options{ // default scheduler options
		Strategy: DefaultStrategy,
		MaxCount: &maxCount,
		// If Persist is false, then on host disconnection, that hosts
		// entry will immediately expire, and the scheduler will react
		// instantly and remove that host entry from the list. If this
		// is true, or if the host closes without a clean shutdown, it
		// will take the TTL number of seconds to remove the key. This
		// can be set using the concurrency.WithTTL option to Session.
		Persist:    DefaultPersist,
		SessionTTL: DefaultSessionTTL,
	}
}

// Load restores struct fields from a string. It respects any defaults that are
// already set in the existing struct.
func (obj *Options) Load(s string) error {
	if s == "" {
		return nil
	}

	values, err := url.ParseQuery(s)
	if err != nil {
		return err
	}

	get := func(key string) (string, bool) { // helper
		if !values.Has(key) {
			return "", false
		}
		return values.Get(key), true
	}

	if v, ok := get("strategy"); ok {
		if _, err := Lookup(v); err != nil {
			return err
		}
		obj.Strategy = v
	}

	if v, ok := get("maxCount"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		obj.MaxCount = &n
	}

	if v, ok := get("persist"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return err
		}
		obj.Persist = b
	}

	if v, ok := get("sessionTTL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		obj.SessionTTL = n
	}

	// supports both repeated keys and single values
	if vs, ok := values["hostsFilter"]; ok {
		// eg: hostsFilter=a&hostsFilter=b
		obj.HostsFilter = make([]string, 0, len(vs))
		for _, v := range vs {
			if v != "" {
				obj.HostsFilter = append(obj.HostsFilter, v)
			}
		}
	}

	return nil
}

// Save stores the struct contents as a string.
func (obj *Options) Save() string {
	values := url.Values{}

	values.Set("strategy", obj.Strategy)
	if n := obj.MaxCount; n != nil {
		values.Set("maxCount", strconv.Itoa(*n))
	}
	values.Set("persist", strconv.FormatBool(obj.Persist))
	values.Set("sessionTTL", strconv.Itoa(obj.SessionTTL))
	// encode with repeated keys?
	for _, h := range obj.HostsFilter {
		values.Add("hostsFilter", h)
	}
	// NOTE: Remember to add entries here if we add new fields.

	return values.Encode()
}

// Merge two scheduler options structs together. This is kind of arbitrary since
// we expect for most options that the individual hosts all specify the same
// options, so if they differ, it's valid undefined behaviour and we can do
// anything, but at least try to roughly merge them in some sane way.
func (obj *Options) Merge(opts *Options) {
	if s := opts.Strategy; s != "" {
		//if _, err := Lookup(s); err != nil {
		//	panic(err) // we already know it should be valid
		//}
		obj.Strategy = s // simpler way
	}

	if d := opts.MaxCount; d != nil {
		obj.MaxCount = d
	}

	if opts.Persist {
		obj.Persist = true
	}

	if d := opts.SessionTTL; d != 0 {
		obj.SessionTTL = d
	}

	if h := opts.HostsFilter; len(h) > 0 {
		obj.HostsFilter = h
	}
}

// StrategyKind sets the scheduler strategy used.
func StrategyKind(kind string) Option {
	return func(so *Options) error {
		if _, err := Lookup(kind); err != nil {
			return err
		}
		so.Strategy = kind
		return nil
	}
}

// MaxCount is the maximum number of hosts that should get simultaneously
// scheduled.
func MaxCount(maxCount int) Option {
	return func(so *Options) error {
		if maxCount > 0 {
			so.MaxCount = &maxCount
		}
		return nil
	}
}

// Persist specifies whether we persist the key when exiting cleanly. This then
// depends on the TTL.
func Persist(persist bool) Option {
	return func(so *Options) error {
		so.Persist = persist
		return nil
	}
}

// SessionTTL is the amount of time to delay before expiring a key on abrupt
// host disconnect or if Persist is true.
func SessionTTL(sessionTTL int) Option {
	return func(so *Options) error {
		if sessionTTL > 0 {
			so.SessionTTL = sessionTTL
		}
		return nil
	}
}

// HostsFilter specifies a manual list of hosts, to use as a subset of whatever
// was auto-discovered.
// XXX: think more about this idea...
func HostsFilter(hosts []string) Option {
	return func(so *Options) error {
		so.HostsFilter = hosts
		return nil
	}
}
