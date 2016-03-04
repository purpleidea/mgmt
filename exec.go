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
	"errors"
	"log"
	"os/exec"
	"strings"
)

type ExecRes struct {
	BaseRes    `yaml:",inline"`
	State      string `yaml:"state"`      // state: exists/present?, absent, (undefined?)
	Cmd        string `yaml:"cmd"`        // the command to run
	Shell      string `yaml:"shell"`      // the (optional) shell to use to run the cmd
	Timeout    int    `yaml:"timeout"`    // the cmd timeout in seconds
	WatchCmd   string `yaml:"watchcmd"`   // the watch command to run
	WatchShell string `yaml:"watchshell"` // the (optional) shell to use to run the watch cmd
	IfCmd      string `yaml:"ifcmd"`      // the if command to run
	IfShell    string `yaml:"ifshell"`    // the (optional) shell to use to run the if cmd
	PollInt    int    `yaml:"pollint"`    // the poll interval for the ifcmd
}

func NewExecRes(name, cmd, shell string, timeout int, watchcmd, watchshell, ifcmd, ifshell string, pollint int, state string) *ExecRes {
	obj := &ExecRes{
		BaseRes: BaseRes{
			Name: name,
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
	obj.Init()
	return obj
}

func (obj *ExecRes) Init() {
	obj.BaseRes.kind = "Exec"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

// validate if the params passed in are valid data
// FIXME: where should this get called ?
func (obj *ExecRes) Validate() bool {
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
func (obj *ExecRes) BufioChanScanner(scanner *bufio.Scanner) (chan string, chan error) {
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
func (obj *ExecRes) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	var send = false // send event?
	var exit = false
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
			log.Printf("%v[%v]: Error creating StdoutPipe for Cmd: %v", obj.Kind(), obj.GetName(), err)
			log.Fatal(err) // XXX: how should we handle errors?
		}
		scanner := bufio.NewScanner(cmdReader)

		defer cmd.Wait() // XXX: is this necessary?
		defer func() {
			// FIXME: without wrapping this in this func it panic's
			// when running examples/graph8d.yaml
			cmd.Process.Kill() // TODO: is this necessary?
		}()
		if err := cmd.Start(); err != nil {
			log.Printf("%v[%v]: Error starting Cmd: %v", obj.Kind(), obj.GetName(), err)
			log.Fatal(err) // XXX: how should we handle errors?
		}

		bufioch, errch = obj.BufioChanScanner(scanner)
	}

	for {
		obj.SetState(resStateWatching) // reset
		select {
		case text := <-bufioch:
			obj.SetConvergedState(resConvergedNil)
			// each time we get a line of output, we loop!
			log.Printf("%v[%v]: Watch output: %s", obj.Kind(), obj.GetName(), text)
			if text != "" {
				send = true
			}

		case err := <-errch:
			obj.SetConvergedState(resConvergedNil) // XXX ?
			if err == nil {                        // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				log.Printf("%v[%v]: Reached EOF", obj.Kind(), obj.GetName())
				return
			}
			log.Printf("%v[%v]: Error reading input?: %v", obj.Kind(), obj.GetName(), err)
			log.Fatal(err)
			// XXX: how should we handle errors?

		case event := <-obj.events:
			obj.SetConvergedState(resConvergedNil)
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
			}

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(resConvergedTimeout)
			obj.converged <- true
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// it is okay to invalidate the clean state on poke too
			obj.isStateOK = false // something made state dirty
			Process(obj)          // XXX: rename this function
		}
	}
}

// TODO: expand the IfCmd to be a list of commands
func (obj *ExecRes) CheckApply(apply bool) (stateok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	// if there is a watch command, but no if command, run based on state
	if obj.WatchCmd != "" && obj.IfCmd == "" {
		if obj.isStateOK {
			return true, nil
		}

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
			return true, nil // don't run
		}

		// if there is no watcher and no onlyif check, assume we should run
	} else { // if obj.WatchCmd == "" && obj.IfCmd == "" {
		// just run if state is dirty
		if obj.isStateOK {
			return true, nil
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v[%v]: Apply", obj.Kind(), obj.GetName())
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

	if err = cmd.Start(); err != nil {
		log.Printf("%v[%v]: Error starting Cmd: %v", obj.Kind(), obj.GetName(), err)
		return false, err
	}

	timeout := obj.Timeout
	if timeout == 0 { // zero timeout means no timer, so disable it
		timeout = -1
	}
	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	select {
	case err = <-done:
		if err != nil {
			log.Printf("%v[%v]: Error waiting for Cmd: %v", obj.Kind(), obj.GetName(), err)
			return false, err
		}

	case <-TimeAfterOrBlock(timeout):
		log.Printf("%v[%v]: Timeout waiting for Cmd", obj.Kind(), obj.GetName())
		//cmd.Process.Kill() // TODO: is this necessary?
		return false, errors.New("Timeout waiting for Cmd!")
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

	// the state tracking is for exec resources that can't "detect" their
	// state, and assume it's invalid when the Watch() function triggers.
	// if we apply state successfully, we should reset it here so that we
	// know that we have applied since the state was set not ok by event!
	obj.isStateOK = true // reset
	return false, nil    // success
}

type ExecUUID struct {
	BaseUUID
	Cmd   string
	IfCmd string
	// TODO: add more elements here
}

// if and only if they are equivalent, return true
// if they are not equivalent, return false
func (obj *ExecUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*ExecUUID)
	if !ok {
		return false
	}
	if obj.Cmd != res.Cmd {
		return false
	}
	// TODO: add more checks here
	//if obj.Shell != res.Shell {
	//	return false
	//}
	//if obj.Timeout != res.Timeout {
	//	return false
	//}
	//if obj.WatchCmd != res.WatchCmd {
	//	return false
	//}
	//if obj.WatchShell != res.WatchShell {
	//	return false
	//}
	if obj.IfCmd != res.IfCmd {
		return false
	}
	//if obj.PollInt != res.PollInt {
	//	return false
	//}
	//if obj.State != res.State {
	//	return false
	//}
	return true
}

func (obj *ExecRes) AutoEdges() AutoEdge {
	// TODO: parse as many exec params to look for auto edges, for example
	// the path of the binary in the Cmd variable might be from in a pkg
	return nil
}

// include all params to make a unique identification of this object
func (obj *ExecRes) GetUUIDs() []ResUUID {
	x := &ExecUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		Cmd:      obj.Cmd,
		IfCmd:    obj.IfCmd,
		// TODO: add more params here
	}
	return []ResUUID{x}
}

func (obj *ExecRes) Compare(res Res) bool {
	switch res.(type) {
	case *ExecRes:
		res := res.(*ExecRes)
		if obj.Name != res.Name {
			return false
		}
		if obj.Cmd != res.Cmd {
			return false
		}
		if obj.Shell != res.Shell {
			return false
		}
		if obj.Timeout != res.Timeout {
			return false
		}
		if obj.WatchCmd != res.WatchCmd {
			return false
		}
		if obj.WatchShell != res.WatchShell {
			return false
		}
		if obj.IfCmd != res.IfCmd {
			return false
		}
		if obj.PollInt != res.PollInt {
			return false
		}
		if obj.State != res.State {
			return false
		}
	default:
		return false
	}
	return true
}
