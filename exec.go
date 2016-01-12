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
	"bytes"
	"log"
	"os/exec"
	"strings"
)

type ExecType struct {
	BaseType   `yaml:",inline"`
	State      string `yaml:"state"`      // state: exists/present?, absent, (undefined?)
	Cmd        string `yaml:"cmd"`        // the command to run
	Shell      string `yaml:"shell"`      // the (optional) shell to use to run the cmd
	Timeout    int    `yaml:"timeout"`    // the cmd timeout in seconds
	WatchCmd   string `yaml:"watchcmd"`   // the watch command to run
	WatchShell string `yaml:"watchshell"` // the (optional) shell to use to run the watch cmd
	IfCmd      string `yaml:"ifcmd"`      // the if command to run
	IfShell    string `yaml:"ifshell"`    // the (optional) shell to use to run the if cmd
	PollInt    int    `yaml:"pollint"`    // the poll interval for the ifcmd
	isStateOK  bool   // whether the state is okay based on events or not
}

func NewExecType(name, cmd, shell string, timeout int, watchcmd, watchshell, ifcmd, ifshell string, pollint int, state string) *ExecType {
	// FIXME if path = nil, path = name ...
	return &ExecType{
		BaseType: BaseType{
			Name:   name,
			events: make(chan Event),
			vertex: nil,
		},
		Cmd:        cmd,
		Shell:      shell,
		Timeout:    timeout,
		WatchCmd:   watchcmd,
		WatchShell: watchshell,
		IfCmd:      ifcmd,
		IfShell:    ifshell,
		PollInt:    pollint,
		State:      state,
	}
}

func (obj *ExecType) GetType() string {
	return "Exec"
}

// validate if the params passed in are valid data
// FIXME: where should this get called ?
func (obj *ExecType) Validate() bool {
	if obj.Cmd == "" { // this is the only thing that is really required
		return false
	}

	// if we have a watch command, then we don't poll with the if command!
	if obj.WatchCmd != "" && obj.PollInt > 0 {
		return false
	}

	return true
}

// wraps the scanner output in a channel
func (obj *ExecType) BufioChanScanner(scanner *bufio.Scanner) (chan string, chan error) {
	ch, errch := make(chan string), make(chan error)
	go func() {
		for scanner.Scan() {
			ch <- scanner.Text() // blocks here ?
			if e := scanner.Err(); e != nil {
				errch <- e // send any misc errors we encounter
				//break // TODO ?
			}
		}
		close(ch)
		errch <- scanner.Err() // eof or some err
		close(errch)
	}()
	return ch, errch
}

// Exec watcher
func (obj *ExecType) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	var send = false // send event?
	bufioch, errch := make(chan string), make(chan error)
	//vertex := obj.GetVertex()         // stored with SetVertex

	if obj.WatchCmd != "" {
		var cmdName string
		var cmdArgs []string
		if obj.WatchShell == "" {
			// call without a shell
			// FIXME: are there still whitespace splitting issues?
			split := strings.Fields(obj.WatchCmd)
			cmdName = split[0]
			//d, _ := os.Getwd() // TODO: how does this ever error ?
			//cmdName = path.Join(d, cmdName)
			cmdArgs = split[1:len(split)]
		} else {
			cmdName = obj.Shell // usually bash, or sh
			cmdArgs = []string{"-c", obj.WatchCmd}
		}
		cmd := exec.Command(cmdName, cmdArgs...)
		//cmd.Dir = "" // look for program in pwd ?

		cmdReader, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("%v[%v]: Error creating StdoutPipe for Cmd: %v", obj.GetType(), obj.GetName(), err)
			log.Fatal(err) // XXX: how should we handle errors?
		}
		scanner := bufio.NewScanner(cmdReader)

		defer cmd.Wait()         // XXX: is this necessary?
		defer cmd.Process.Kill() // TODO: is this necessary?
		if err := cmd.Start(); err != nil {
			log.Printf("%v[%v]: Error starting Cmd: %v", obj.GetType(), obj.GetName(), err)
			log.Fatal(err) // XXX: how should we handle errors?
		}

		bufioch, errch = obj.BufioChanScanner(scanner)
	}

	for {
		obj.SetState(typeWatching) // reset
		select {
		case text := <-bufioch:
			obj.SetConvergedState(typeConvergedNil)
			// each time we get a line of output, we loop!
			log.Printf("%v[%v]: Watch output: %s", obj.GetType(), obj.GetName(), text)
			if text != "" {
				send = true
			}

		case err := <-errch:
			obj.SetConvergedState(typeConvergedNil) // XXX ?
			if err == nil {                         // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				log.Printf("%v[%v]: Reached EOF", obj.GetType(), obj.GetName())
				return
			}
			log.Printf("%v[%v]: Error reading input?: %v", obj.GetType(), obj.GetName(), err)
			log.Fatal(err)
			// XXX: how should we handle errors?

		case event := <-obj.events:
			obj.SetConvergedState(typeConvergedNil)
			if ok := obj.ReadEvent(&event); !ok {
				return // exit
			}
			send = true

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(typeConvergedTimeout)
			obj.converged <- true
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.isStateOK = false // something made state dirty
			Process(obj)          // XXX: rename this function
		}
	}
}

// TODO: expand the IfCmd to be a list of commands
func (obj *ExecType) StateOK() bool {

	// if there is a watch command, but no if command, run based on state
	if b := obj.isStateOK; obj.WatchCmd != "" && obj.IfCmd == "" {
		obj.isStateOK = true // reset
		//if !obj.isStateOK { obj.isStateOK = true; return false }
		return b

		// if there is no watcher, but there is an onlyif check, run it to see
	} else if obj.IfCmd != "" { // && obj.WatchCmd == ""
		// there is a watcher, but there is also an if command
		//} else if obj.IfCmd != "" && obj.WatchCmd != "" {

		if obj.PollInt > 0 { // && obj.WatchCmd == ""
			// XXX have the Watch() command output onlyif poll events...
			// XXX we can optimize by saving those results for returning here
			// return XXX
		}

		var cmdName string
		var cmdArgs []string
		if obj.IfShell == "" {
			// call without a shell
			// FIXME: are there still whitespace splitting issues?
			split := strings.Fields(obj.IfCmd)
			cmdName = split[0]
			//d, _ := os.Getwd() // TODO: how does this ever error ?
			//cmdName = path.Join(d, cmdName)
			cmdArgs = split[1:len(split)]
		} else {
			cmdName = obj.IfShell // usually bash, or sh
			cmdArgs = []string{"-c", obj.IfCmd}
		}
		err := exec.Command(cmdName, cmdArgs...).Run()
		if err != nil {
			// TODO: check exit value
			return true // don't run
		}
		return false // just run

		// if there is no watcher and no onlyif check, assume we should run
	} else { // if obj.WatchCmd == "" && obj.IfCmd == "" {
		return false // just run
	}
}

func (obj *ExecType) Apply() bool {
	log.Printf("%v[%v]: Apply", obj.GetType(), obj.GetName())
	var cmdName string
	var cmdArgs []string
	if obj.Shell == "" {
		// call without a shell
		// FIXME: are there still whitespace splitting issues?
		// TODO: we could make the split character user selectable...!
		split := strings.Fields(obj.Cmd)
		cmdName = split[0]
		//d, _ := os.Getwd() // TODO: how does this ever error ?
		//cmdName = path.Join(d, cmdName)
		cmdArgs = split[1:len(split)]
	} else {
		cmdName = obj.Shell // usually bash, or sh
		cmdArgs = []string{"-c", obj.Cmd}
	}
	cmd := exec.Command(cmdName, cmdArgs...)
	//cmd.Dir = "" // look for program in pwd ?
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Start(); err != nil {
		log.Printf("%v[%v]: Error starting Cmd: %v", obj.GetType(), obj.GetName(), err)
		return false
	}

	timeout := obj.Timeout
	if timeout == 0 { // zero timeout means no timer, so disable it
		timeout = -1
	}
	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("%v[%v]: Error waiting for Cmd: %v", obj.GetType(), obj.GetName(), err)
			return false
		}

	case <-TimeAfterOrBlock(timeout):
		log.Printf("%v[%v]: Timeout waiting for Cmd", obj.GetType(), obj.GetName())
		//cmd.Process.Kill() // TODO: is this necessary?
		return false
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		log.Printf("Exec[%v]: Command output is empty!", obj.Name)
	} else {
		log.Printf("Exec[%v]: Command output is:", obj.Name)
		log.Printf(out.String())
	}
	// XXX: return based on exit value!!
	return true
}

func (obj *ExecType) Compare(typ Type) bool {
	switch typ.(type) {
	case *ExecType:
		typ := typ.(*ExecType)
		if obj.Name != typ.Name {
			return false
		}
		if obj.Cmd != typ.Cmd {
			return false
		}
		if obj.Shell != typ.Shell {
			return false
		}
		if obj.Timeout != typ.Timeout {
			return false
		}
		if obj.WatchCmd != typ.WatchCmd {
			return false
		}
		if obj.WatchShell != typ.WatchShell {
			return false
		}
		if obj.IfCmd != typ.IfCmd {
			return false
		}
		if obj.PollInt != typ.PollInt {
			return false
		}
		if obj.State != typ.State {
			return false
		}
	default:
		return false
	}
	return true
}
