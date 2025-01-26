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

package interfaces

import (
	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
)

// MembersResult returns the expect result (including possibly an error) from a
// WatchMembers operation.
type MembersResult struct {

	// Members is the list of members found in this result.
	Members []*Member

	// Err represents an error. If this is not nil, don't touch the other
	// data in this struct.
	Err error
}

// Member is our internal copy of the etcd member struct as found here:
// https://godocs.io/github.com/coreos/etcd/etcdserver/etcdserverpb#Member but
// which uses native types where possible.
type Member struct {
	// ID is unique in the cluster for each member.
	ID uint64

	// Name for the member which if not not started will be an empty string.
	Name string

	// IsLeader tells which member is leading the cluster. Expect this to
	// change as time goes on.
	// XXX: add when new version of etcd supports this
	//IsLeader bool

	// PeerURLs is the list of addresses peers servers can connect to.
	PeerURLs etcdtypes.URLs

	// ClientURLs is the list of addresses that clients can connect to. If
	// the member is not started, then this will be a zero length list.
	ClientURLs etcdtypes.URLs
}
