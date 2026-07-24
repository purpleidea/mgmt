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

//go:build linux

package corenet

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/vishvananda/netlink"
)

// networkEventStream emits an initial event and then emits an event whenever a
// network link or address changes. Subscriptions are established before the
// initial event so a change cannot occur between the initial call and watch.
func networkEventStream(ctx context.Context, event func(context.Context) error) error {
	done := make(chan struct{})
	linkEvents := make(chan netlink.LinkUpdate)
	addrEvents := make(chan netlink.AddrUpdate)
	errors := make(chan error, 1)
	errorCallback := func(err error) {
		select {
		case errors <- err:
		default:
		}
	}

	linkOptions := netlink.LinkSubscribeOptions{
		ErrorCallback: errorCallback,
	}
	if err := netlink.LinkSubscribeWithOptions(linkEvents, done, linkOptions); err != nil {
		return errwrap.Wrapf(err, "could not subscribe to network link events")
	}

	addrOptions := netlink.AddrSubscribeOptions{
		ErrorCallback: errorCallback,
	}
	if err := netlink.AddrSubscribeWithOptions(addrEvents, done, addrOptions); err != nil {
		close(done)
		for range linkEvents {
		}
		return errwrap.Wrapf(err, "could not subscribe to network address events")
	}

	defer func() {
		close(done)
		for linkEvents != nil || addrEvents != nil {
			select {
			case _, ok := <-linkEvents:
				if !ok {
					linkEvents = nil
				}
			case _, ok := <-addrEvents:
				if !ok {
					addrEvents = nil
				}
			}
		}
	}()

	if err := event(ctx); err != nil {
		return err
	}

	for {
		select {
		case _, ok := <-linkEvents:
			if !ok {
				return networkEventSubscriptionClosed("link", errors)
			}

		case _, ok := <-addrEvents:
			if !ok {
				return networkEventSubscriptionClosed("address", errors)
			}

		case err := <-errors:
			return errwrap.Wrapf(err, "network event subscription failed")

		case <-ctx.Done():
			return nil
		}

		if err := event(ctx); err != nil {
			return err
		}
	}
}

// networkEventSubscriptionClosed returns the receiver error when available.
func networkEventSubscriptionClosed(kind string, errors <-chan error) error {
	select {
	case err := <-errors:
		return errwrap.Wrapf(err, "network event subscription failed")
	default:
		return fmt.Errorf("network %s event subscription closed", kind)
	}
}
