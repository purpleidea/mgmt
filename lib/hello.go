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

package lib

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/coreos/pkg/capnslog"
)

func hello(program, version string, flags Flags) {
	var start = time.Now().UnixNano()

	var logFlags int
	if flags.Debug || true { // TODO: remove || true
		logFlags = log.LstdFlags | log.Lshortfile
	}
	logFlags = (logFlags - log.Ldate) // remove the date for now
	log.SetFlags(logFlags)

	// un-hijack from capnslog...
	// XXX: move this to the etcd package when new version deprecates capnslog
	log.SetOutput(os.Stderr)
	if flags.Verbose {
		capnslog.SetFormatter(capnslog.NewLogFormatter(os.Stderr, "(etcd) ", logFlags))
	} else {
		capnslog.SetFormatter(capnslog.NewNilFormatter())
	}

	if program == "" {
		program = "<unknown>"
	}
	fmt.Println(fmt.Sprintf("This is: %s, version: %s", program, version))
	fmt.Println("Copyright (C) 2013-2019+ James Shubin and the project contributors")
	fmt.Println("Written by James Shubin <james@shubin.ca> and the project contributors")
	log.Printf("main: start: %v", start)
}
