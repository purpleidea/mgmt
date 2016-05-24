// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

const (
	PuppetYAMLBufferSize = 65535
)

func runPuppetCommand(cmd *exec.Cmd) ([]byte, error) {
	if DEBUG {
		log.Printf("Puppet: running command: %v", cmd)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Puppet: Error opening pipe to puppet command: %v", err)
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Puppet: Error opening error pipe to puppet command: %v", err)
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Puppet: Error starting puppet command: %v", err)
		return nil, err
	}

	// XXX: the current implementation is likely prone to fail
	// as soon as the YAML data overflows the buffer.
	data := make([]byte, PuppetYAMLBufferSize)
	var result []byte
	for err == nil {
		var count int
		count, err = stdout.Read(data)
		if err != nil && err != io.EOF {
			log.Printf("Puppet: Error reading YAML data from puppet: %v", err)
			return nil, err
		}
		// Slicing down to the number of actual bytes is important, the YAML parser
		// will choke on an oversized slice. http://stackoverflow.com/a/33726617/3356612
		result = append(result, data[0:count]...)
	}
	if DEBUG {
		log.Printf("Puppet: read %v bytes of data from puppet", len(result))
	}
	for scanner := bufio.NewScanner(stderr); scanner.Scan(); {
		log.Printf("Puppet: (output) %v", scanner.Text())
	}
	if err := cmd.Wait(); err != nil {
		log.Printf("Puppet: Error: puppet command did not complete: %v", err)
		return nil, err
	}

	return result, nil
}

func ParseConfigFromPuppet(puppetParam, puppetConf string) *GraphConfig {
	var puppetConfArg string
	if puppetConf != "" {
		puppetConfArg = "--config=" + puppetConf
	}

	var cmd *exec.Cmd
	if puppetParam == "agent" {
		cmd = exec.Command("puppet", "mgmtgraph", "print", puppetConfArg)
	} else if strings.HasSuffix(puppetParam, ".pp") {
		cmd = exec.Command("puppet", "mgmtgraph", "print", puppetConfArg, "--manifest", puppetParam)
	} else {
		cmd = exec.Command("puppet", "mgmtgraph", "print", puppetConfArg, "--code", puppetParam)
	}

	log.Println("Puppet: launching translator")

	var config GraphConfig
	if data, err := runPuppetCommand(cmd); err != nil {
		return nil
	} else if err := config.Parse(data); err != nil {
		log.Printf("Puppet: Error: Could not parse YAML output with Parse: %v", err)
		return nil
	}

	return &config
}

func PuppetInterval(puppetConf string) int {
	if DEBUG {
		log.Printf("Puppet: determining graph refresh interval")
	}
	var cmd *exec.Cmd
	if puppetConf != "" {
		cmd = exec.Command("puppet", "config", "print", "runinterval", "--config", puppetConf)
	} else {
		cmd = exec.Command("puppet", "config", "print", "runinterval")
	}

	log.Println("Puppet: inspecting runinterval configuration")

	interval := 1800
	data, err := runPuppetCommand(cmd)
	if err != nil {
		log.Printf("Puppet: could not determine configured run interval (%v), using default of %v", err, interval)
		return interval
	}
	result, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 0)
	if err != nil {
		log.Printf("Puppet: error reading numeric runinterval value (%v), using default of %v", err, interval)
		return interval
	}

	return int(result)
}
