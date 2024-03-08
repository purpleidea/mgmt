// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

var (
	test TestSpec = TestSpec{
		Test: TestDescription{
			Title: "uninitialized",
		},
	}
	templ       *template.Template
	baseDir     string
	mgmtRunning bool
	stopMgmt    chan struct{}
	mgmtOutput  chan string
	lookups     map[string]string
	wg          sync.WaitGroup
)

type LaunchStep struct {
	Command string
}

type ConvergeStep struct {
	Time   int
	Checks []map[string]string
}

type InputStep struct {
	Target  string
	Content string
}

type CleanupStep struct {
	Steps []map[string]string
}

type TestStep struct {
	Launch    LaunchStep
	Converge  ConvergeStep
	Input     InputStep
	Cleanup   CleanupStep
	Terminate bool
}

type TestSpec struct {
	Test     TestDescription
	Timeline []TestStep
}

type TestDescription struct {
	Title  string
	Inputs map[string]string
	Sudo   bool
	Vars   map[string]string
}

func fail(message string, reason error) {
	if reason != nil {
		fmt.Fprintf(os.Stderr, "FAIL [%s] %s: %s\n", test.Test.Title, message, reason)
	} else {
		fmt.Fprintf(os.Stderr, "FAIL [%s] %s\n", test.Test.Title, message)
	}
	cleanup()
	os.Exit(1)
}

func skip(message string) {
	fmt.Printf("SKIP [%s] %s\n", test.Test.Title, message)
	cleanup()
	os.Exit(0)
}

func initialize() {
	var err error
	baseDir, err = os.MkdirTemp("", "mgmtest-")
	if err != nil {
		fail("could not create temporary directory", err)
	}
	lookups = make(map[string]string)

	if test.Test.Sudo {
		checksudo()
	}

	for input, class := range test.Test.Inputs {
		if _, exists := test.Test.Vars[input]; exists {
			fail(fmt.Sprintf("input %s has the same name as a variable", input), nil)
		}
		var pattern string
		switch class {
		case "langfile":
			pattern = input+"*.mcl"
		case "yamlfile":
			pattern = input+"*.yaml"
		default:
			fail(fmt.Sprintf("input %s specifies invalid type %s", input, class), nil)
		}
		if tempfile, err := os.CreateTemp(baseDir, pattern); err != nil {
			fail(fmt.Sprintf("could not create temp file for input %s", input), err)
		} else {
			lookups[input] = tempfile.Name()
			tempfile.Close() // just reserve the name
		}
	}

	for variable, value := range test.Test.Vars {
		lookups[variable] = value
	}

	templ = template.New(test.Test.Title)
}

func checksudo() {
	timer := time.AfterFunc(time.Second, func() { skip("sudo disabled") })
	if err := exec.Command("/usr/bin/sudo", "-A", "/usr/bin/true").Run() ; err != nil {
		fail("sudo test failed", err)
	}
	timer.Stop()
	fmt.Println("sudo test was successful")
}

// Evaluate template expressions, or fail
func expand(text string) string {
	templ, err := templ.Parse(text)
	if err != nil {
		fail(fmt.Sprintf("could not expand template '%s'", text), err)
	}
	var buf bytes.Buffer
	err = templ.Execute(&buf, lookups)
	if err != nil {
		fail(fmt.Sprintf("template execution failed for '%s'", text), err)
	}
	return buf.String()
}

func launch(step LaunchStep) {
	command := expand(step.Command)
	if test.Test.Sudo {
		command = "sudo -A " + command
	}
	fmt.Printf("launching via '%s'\n", command)

	tokens := strings.Split(command, " ")
	cmd := exec.Command(tokens[0], tokens[1:]...)

	if test.Test.Sudo {
		// new process group, otherwise sudo will not forward SIGINT
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	}

	//output, err := cmd.CombinedOutput()
	//fmt.Printf("MGMT OUTPUT\n\n%s\n\n", output)
	//fail("bailing out", err)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fail("could not open mgmt stderr for reading", err)
	}
	if err := cmd.Start(); err != nil {
		fail("launching mgmt failed", err)
	}
	mgmtOutput = make(chan string)
	stopMgmt = make(chan struct{})
	go func() {
		wg.Add(1)
		defer wg.Done()
		defer close(mgmtOutput)
		defer fmt.Println("stopping mgmt listener")
		buf := make([]byte, 1024)
		for {
			count, e := stderr.Read(buf)
			if count > 0 {
				output := fmt.Sprintf("%s", buf)
				select {
				case mgmtOutput <- output:
				case <-stopMgmt:
					return
				}
			}
			if e == io.EOF {
				fmt.Printf("mgmt was terminated\n%s\n", buf)
				return
			} else if e != nil {
				fail("error reading mgmt output", e)
			}
		}
	}()
	go func() {
		wg.Add(1)
		// not thread safe
		mgmtRunning = true
		<-stopMgmt
		mgmtRunning = false
		fmt.Println("signaling the mgmt process")
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			fail("unable to terminate mgmt process", err)
		} else {
			fmt.Println("killed mgmt")
		}
		wg.Done()
	}()
}

func writeInput(step InputStep) {
	content := expand(step.Content)
	if err := os.WriteFile(lookups[step.Target], []byte(content), 0666); err != nil {
		fail(fmt.Sprintf("could not write to input '%s'", step.Target), err)
	}
	fmt.Printf("wrote content to input %s:\n%s\n", step.Target, content)
}

func converge(step ConvergeStep) {
	convergeTime := time.Duration(step.Time) * time.Second
	fmt.Printf("converging in %v, running %v checks\n", convergeTime, len(step.Checks))
	start := time.Now()

	ding := make(chan struct{})
	sendDing := func() { ding <- struct{}{} }
	closeDing := func() { close(ding) }
	convergeTimer := time.AfterFunc(convergeTime, sendDing)
	timeoutTimer := time.AfterFunc(60*time.Second, closeDing)

	var output []string

loop:
	for {
		select {
		case chunk, ok := <-mgmtOutput:
			if !ok {
				fail("mgmtOutput channel closed somehow", nil)
			}
			if !convergeTimer.Reset(convergeTime) {
				// eep, just fired the converge signal, never mind
				convergeTimer.Stop()
			}
			output = append(output, chunk)
		case _, ok := <-ding:
			convergeTimer.Stop()
			timeoutTimer.Stop()
			if ok {
				finish := time.Now()
				for _, chunk := range output {
					fmt.Print(chunk)
				}
				fmt.Printf("converged for %v after %v\n", convergeTime, finish.Sub(start))
				break loop
			} else {
				fail("converge ran into the 60 second timeout", nil)
			}
		}
	}

	for _, step := range step.Checks {
		runShell(step)
	}
}

func runShell(spec map[string]string) {
	var (
		command string
		name    string
		expect  int
		ok      bool
		rc      int
	)
	if command, ok = spec["shell"]; !ok {
		fail(fmt.Sprintf("no shell command specified: %+v", spec), nil)
	}
	command = expand(command)
	if name, ok = spec["name"]; !ok {
		name = fmt.Sprintf("unnamed command [%s]", command)
	} else {
		name = expand(name)
	}
	if expectS, found := spec["result"]; found {
		var err error
		if expect, err = strconv.Atoi(expectS); err != nil {
			fail(fmt.Sprintf("shell expect must be integer, not '%s'", expectS), err)
		}
	}

	err := exec.Command("/bin/bash", "-c", command).Run()
	if err == nil {
		rc = 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else {
		fail("error running: %v", err)
	}
	if rc != expect {
		fail(fmt.Sprintf("(%s) command %s should have returned %v not %v", name, command, expect, rc), nil)
	}
	fmt.Printf(" -- %s: running '%s' (rc: %v) OK\n", name, command, rc)
}

func terminate() {
	fmt.Println("terminating mgmt ...")
	// panic if mgmt not running
	close(stopMgmt)
	wg.Wait()
	fmt.Println("mgmt fully stopped")
}

func run() {
	for _, step := range test.Timeline {
		if step.Launch.Command != "" {
			launch(step.Launch)
		} else if step.Converge.Time != 0 {
			converge(step.Converge)
		} else if step.Input.Target != "" {
			writeInput(step.Input)
		} else if len(step.Cleanup.Steps) > 0 {
			for _, cleanUpStep := range step.Cleanup.Steps {
				runShell(cleanUpStep)
			}
		} else if step.Terminate {
			terminate()
		} else {
			fail(fmt.Sprintf("no handler for timeline item %v", step), nil)
		}
	}
}

func cleanup() {
	fmt.Println("running the cleanup routine")
	if mgmtRunning {
		fmt.Println("closing the stop chan")
		close(stopMgmt)
	}
	if baseDir != "" {
		fmt.Printf("removing %v\n", baseDir)
		os.RemoveAll(baseDir)
	}
	wg.Wait()
}

func loadyaml() {
	if len(os.Args) != 2 {
		fail("no test was specified", nil)
	}
	definition := os.Args[1]
	yamlData, err := os.ReadFile(definition)
	if err != nil {
		fail("could not load test definition", err)
	}

	err = yaml.Unmarshal(yamlData, &test)
	if err != nil {
		fail("failed to unmarshal yaml", err)
	}
}

func main() {
	loadyaml()
	initialize()
	run()
	cleanup()
}
