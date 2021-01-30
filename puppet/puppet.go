// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

// Package puppet provides the integration entrypoint for the puppet language.
package puppet

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/yamlgraph"
)

const (
	// PuppetYAMLBufferSize is the maximum buffer size for the yaml input data
	PuppetYAMLBufferSize = 65535
)

func (obj *GAPI) runPuppetCommand(cmd *exec.Cmd) ([]byte, error) {
	if obj.data.Debug {
		obj.data.Logf("running command: %v", cmd)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error opening pipe to puppet command")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error opening error pipe to puppet command")
	}

	if err := cmd.Start(); err != nil {
		return nil, errwrap.Wrapf(err, "error starting puppet command")
	}

	// XXX: the current implementation is likely prone to fail
	// as soon as the YAML data overflows the buffer.
	data := make([]byte, PuppetYAMLBufferSize)
	var result []byte
	for err == nil {
		var count int
		count, err = stdout.Read(data)
		if err != nil && err != io.EOF {
			obj.data.Logf("error reading YAML data from puppet: %v", err)
			return nil, err
		}
		// Slicing down to the number of actual bytes is important, the YAML parser
		// will choke on an oversized slice. http://stackoverflow.com/a/33726617/3356612
		result = append(result, data[0:count]...)
	}
	if obj.data.Debug {
		obj.data.Logf("read %d bytes of data from puppet", len(result))
	}
	for scanner := bufio.NewScanner(stderr); scanner.Scan(); {
		obj.data.Logf("(output) %v", scanner.Text())
	}
	if err := cmd.Wait(); err != nil {
		return nil, errwrap.Wrapf(err, "error waiting for puppet command to complete")
	}

	return result, nil
}

// ParseConfigFromPuppet returns the graph configuration structure from the mode
// and input values, including possibly some file and directory paths.
func (obj *GAPI) ParseConfigFromPuppet() (*yamlgraph.GraphConfig, error) {
	var args []string
	switch obj.Mode {
	case "agent":
		args = []string{"mgmtgraph", "print"}
	case "file":
		args = []string{"mgmtgraph", "print", "--manifest", obj.puppetFile}
	case "string":
		args = []string{"mgmtgraph", "print", "--code", obj.puppetString}
	case "dir":
		// TODO: run the code from the obj.puppetDir directory path
		return nil, fmt.Errorf("not implemented") // XXX: not implemented
	default:
		panic(fmt.Sprintf("%s: unhandled case: %s", Name, obj.Mode))
	}

	if obj.puppetConf != "" {
		args = append(args, "--config="+obj.puppetConf)
	}

	cmd := exec.Command("puppet", args...)

	obj.data.Logf("launching translator")

	var config yamlgraph.GraphConfig
	if data, err := obj.runPuppetCommand(cmd); err != nil {
		return nil, errwrap.Wrapf(err, "could not run puppet command")
	} else if err := config.Parse(data); err != nil {
		return nil, errwrap.Wrapf(err, "could not parse YAML output")
	}

	return &config, nil
}

// RefreshInterval returns the graph refresh interval from the puppet
// configuration.
func (obj *GAPI) refreshInterval() int {
	if obj.data.Debug {
		obj.data.Logf("determining graph refresh interval")
	}
	var cmd *exec.Cmd
	if obj.puppetConf != "" {
		cmd = exec.Command("puppet", "config", "print", "runinterval", "--config", obj.puppetConf)
	} else {
		cmd = exec.Command("puppet", "config", "print", "runinterval")
	}

	obj.data.Logf("inspecting runinterval configuration")

	interval := 1800
	data, err := obj.runPuppetCommand(cmd)
	if err != nil {
		obj.data.Logf("could not determine configured run interval (%v), using default of %v", err, interval)
		return interval
	}
	result, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 0)
	if err != nil {
		obj.data.Logf("error reading numeric runinterval value (%v), using default of %v", err, interval)
		return interval
	}

	return int(result)
}
