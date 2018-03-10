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

package integration

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	errwrap "github.com/pkg/errors"
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

// Code takes a code block as a backtick enclosed `heredoc` and removes any
// common indentation from each line. This helps inline code as strings to be
// formatted nicely without unnecessary indentation. It also drops the very
// first line of code if it has zero length.
func Code(code string) string {
	output := []string{}
	lines := strings.Split(code, "\n")
	var found bool
	var strip string // prefix to remove
	for i, x := range lines {
		if !found && len(x) > 0 {
			for j := 0; j < len(x); j++ {
				if x[j] != '\t' {
					break
				}
				strip += "\t"
			}
			// otherwise, there's no indentation
			found = true
		}
		if i == 0 && len(x) == 0 { // drop first line if it's empty
			continue
		}

		s := strings.TrimPrefix(x, strip)
		output = append(output, s)
	}

	return strings.Join(output, "\n")
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
