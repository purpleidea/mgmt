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

package election

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	"go.etcd.io/etcd/client/v3/concurrency"
)

// Observer is an alternative to the *concurrency.Election Observe functionality
// except that it runs Campaign automatically.
type Observer struct {
	// Unique name of this host which we use as the token to identify us.
	Hostname string

	// Session is a built Session object which this depends on. Close it
	// when done as you normally would.
	Session *concurrency.Session

	// Path is the unique election path used in etcd. It does not include
	// any magic /_mgmt/ prefix, so you'll have to add that yourself.
	Path string // fmt.Sprintf("%s%selection", obj.Client.GetNamespace(), obj.Prefix)

	// ResignTimeout is how long to wait when resigning during exit. The ctx
	// does not skip over this duration.
	ResignTimeout time.Duration

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Observe is the same as the concurrency package election.Observe call except
// that it also runs an election.Campaign autonomously for you.
func (obj *Observer) Observe(ctx context.Context) <-chan *Result {
	ch := make(chan *Result)
	var reterr error
	var camerr error
	go func() {
		defer close(ch)
		defer func() {
			select {
			case ch <- &Result{
				Err: reterr,
			}:
			case <-ctx.Done():
			}
		}()
		defer func() {
			reterr = errwrap.Append(reterr, camerr)
		}()
		ctx, cancel := context.WithCancel(ctx) // wrap so we can cancel watchers!

		// create an election object
		// we need to add on the client namespace here since the special /_mgmt/
		// prefix isn't added when we go directly through this API...
		election := concurrency.NewElection(obj.Session, obj.Path)
		electionChan := election.Observe(ctx)
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), obj.ResignTimeout)
			defer cancel()

			// If we're not the leader, this is a harmless noop.
			if err := election.Resign(ctx); err != nil {
				// lock only needed if we do this elsewhere concurrently
				reterr = errwrap.Append(reterr, err)
			}
		}()

		wg := &sync.WaitGroup{}
		defer wg.Wait() // can't run Resign and Campaign concurrently!
		defer cancel()

		elected := "" // who we "assume" is elected
		startup := make(chan struct{})
		var running chan struct{} // is campaign running?

		fn := func(ctx context.Context) {
			running = make(chan struct{})
			wg.Add(1)
			go func() {
				defer close(running)
				defer wg.Done()
				obj.Logf("starting campaign...")
				// Campaign blocks if we're not elected, and it
				// returns instantly if we're already elected.
				if err := election.Campaign(ctx, obj.Hostname); err != nil {
					camerr = err
					cancel()
				}
			}()
		}

		obj.Logf("checking for existing leader...")
		leaderResult, err := election.Leader(ctx)
		if err != nil && err != concurrency.ErrElectionNoLeader {
			if obj.Debug {
				obj.Logf("leader information error: %v", err)
			}
			reterr = err
			return
		}
		if err == concurrency.ErrElectionNoLeader {
			fn(ctx) // start campaign
		}
		if err == nil {
			elected = string(leaderResult.Kvs[0].Value)
			obj.Logf("leader information: %s", elected)
			close(startup)
		}

		for {
			// NOTE: At least *someone* has to Campaign initially...
			select {
			case <-startup:
				//startup = make(chan struct{}) // reset
				startup = nil // once

			// new election result
			// XXX: does this produce an initial value at startup?
			case val, ok := <-electionChan:
				if obj.Debug {
					obj.Logf("electionChan(%t): %+v", ok, val)
				}
				if !ok {
					if obj.Debug {
						obj.Logf("elections stream shutdown...")
					}
					reterr = fmt.Errorf("election shutdown")
					return
				}

				elected = string(val.Kvs[0].Value)
				if obj.Debug {
					obj.Logf("elected: %s", elected)
				}

			case <-ctx.Done():
				return
			}

			if elected != "" { // initially we don't send yet
				select {
				case ch <- &Result{
					Val: elected,
				}:
				case <-ctx.Done():
					return
				}
			}

			// I'm already elected, wait for electionChan to change
			// before I try to campaign again...
			if elected == obj.Hostname {
				continue // don't campaign again right now
			}

			select {
			case <-running: // pass through and start again
			default:
				continue
			}

			fn(ctx) // start campaign
		}
	}()

	return ch
}

// Result stores an error, or if it's nil, then our elected result.
type Result struct {
	Val string
	Err error
}
