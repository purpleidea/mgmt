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

package integration

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	binaryName = "mgmt"
)

// BinaryPath returns the full path to an mgmt binary. It expects that someone
// will run `make build` or something equivalent to produce a binary before this
// function runs.
func BinaryPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("can't determine binary path")
	}
	dir := filepath.Dir(file) // dir that this file is contained in
	root := filepath.Dir(dir) // we're in the parent dir to that

	return path.Join(root, binaryName), nil
}

// ParsePort parses a URL and returns the port that was found.
func ParsePort(input string) (int, error) {
	u, err := url.Parse(input)
	if err != nil {
		return 0, errwrap.Wrapf(err, "could not parse URL")
	}
	_, sport, err := net.SplitHostPort(u.Host)
	if err != nil {
		return 0, errwrap.Wrapf(err, "could not get port")
	}
	port, err := strconv.Atoi(sport)
	if err != nil {
		return 0, errwrap.Wrapf(err, "could not parse port")
	}
	return port, nil
}
