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

package util

import (
	"fmt"
)

const (
	// MaxHostnameLength is the maximum number of chars in a valid hostname.
	// This matches the usual DNS limit for a fully qualified domain name.
	MaxHostnameLength = 253
)

// ValidHostname validates a hostname which is used as the unique identifier for
// a host. It doesn't technically need to be the actual hostname of the machine,
// but it must be safe to use as a component in an etcd key, since many key
// prefixes contain it. In particular it must not contain a slash (which would
// escape its key prefix) and it must not be the star string (which is reserved
// as a catch-all in the exported resources keyspace).
func ValidHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname is empty")
	}
	if len(hostname) > MaxHostnameLength {
		return fmt.Errorf("hostname is longer than %d chars", MaxHostnameLength)
	}

	alnum := 0
	for _, c := range hostname {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			alnum++
		case c == '.' || c == '-' || c == '_':
			// permitted separators
		default:
			return fmt.Errorf("hostname contains invalid char: %q", c)
		}
	}
	if alnum == 0 { // catches ".", "..", "-" and similar
		return fmt.Errorf("hostname must contain an alphanumeric char")
	}
	if c := hostname[0]; c == '.' || c == '-' {
		return fmt.Errorf("hostname must not begin with: %q", c)
	}

	return nil
}
