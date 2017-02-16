// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package resources

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

func init() {
	gob.Register(&ExecRes{})
}

// ExecRes is an exec resource for running commands.
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

// Default returns some sensible defaults for this resource.
func (obj *ExecRes) Default() Res {
	return &ExecRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *ExecRes) Validate() error {
	if obj.Cmd == "" { // this is the only thing that is really required
		return fmt.Errorf("Command can't be empty!")
	}

	// if we have a watch command, then we don't poll with the if command!
	if obj.WatchCmd != "" && obj.PollInt > 0 {
		return fmt.Errorf("Don't poll when we have a watch command.")
	}

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *ExecRes) Init() error {
	obj.BaseRes.kind = "Exec"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// BufioChanScanner wraps the scanner output in a channel.
func (obj *ExecRes) BufioChanScanner(scanner *bufio.Scanner) (chan string, chan error) {
	ch, errch := make(chan string), make(chan error)
	go func() {
		for scanner.Scan() {
			ch <- scanner.Text() // blocks here ?
			if e := scanner.Err(); e != nil {
				errch <- e // send any misc errors we encounter
				//break // TODO: ?
			}
		}
		close(ch)
		errch <- scanner.Err() // eof or some err
		close(errch)
	}()
	return ch, errch
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ExecRes) Watch(processChan chan *event.Event) error {
	var send = false // send event?
	var exit *error
	bufioch, errch := make(chan string), make(chan error)

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
			cmdArgs = split[1:]
		} else {
			cmdName = obj.Shell // usually bash, or sh
			cmdArgs = []string{"-c", obj.WatchCmd}
		}
		cmd := exec.Command(cmdName, cmdArgs...)
		//cmd.Dir = "" // look for program in pwd ?

		cmdReader, err := cmd.StdoutPipe()
		if err != nil {
			return errwrap.Wrapf(err, "Error creating StdoutPipe for Cmd")
		}
		scanner := bufio.NewScanner(cmdReader)

		defer cmd.Wait() // XXX: is this necessary?
		defer func() {
			// FIXME: without wrapping this in this func it panic's
			// when running examples/graph8d.yaml
			cmd.Process.Kill() // TODO: is this necessary?
		}()
		if err := cmd.Start(); err != nil {
			return errwrap.Wrapf(err, "Error starting Cmd")
		}

		bufioch, errch = obj.BufioChanScanner(scanner)
	}

	// notify engine that we're running
	if err := obj.Running(processChan); err != nil {
		return err // bubble up a NACK...
	}

	for {
		select {
		case text := <-bufioch:
			// each time we get a line of output, we loop!
			log.Printf("%s[%s]: Watch output: %s", obj.Kind(), obj.GetName(), text)
			if text != "" {
				send = true
			}

		case err := <-errch:
			if err == nil { // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				return fmt.Errorf("Reached EOF")
			}
			// error reading input?
			return errwrap.Wrapf(err, "Unknown error")

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// it is okay to invalidate the clean state on poke too
			obj.StateOK(false) // something made state dirty
			obj.Event(processChan)
		}
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// TODO: expand the IfCmd to be a list of commands
func (obj *ExecRes) CheckApply(apply bool) (checkOK bool, err error) {

	// if there is a watch command, but no if command, run based on state
	if obj.WatchCmd != "" && obj.IfCmd == "" {
		if obj.IsStateOK() { // FIXME: this is done by engine now...
			return true, nil
		}

		// if there is no watcher, but there is an onlyif check, run it to see
	} else if obj.IfCmd != "" { // && obj.WatchCmd == ""
		// there is a watcher, but there is also an if command
		//} else if obj.IfCmd != "" && obj.WatchCmd != "" {

		if obj.PollInt > 0 { // && obj.WatchCmd == ""
			// XXX: have the Watch() command output onlyif poll events...
			// XXX: we can optimize by saving those results for returning here
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
			cmdArgs = split[1:]
		} else {
			cmdName = obj.IfShell // usually bash, or sh
			cmdArgs = []string{"-c", obj.IfCmd}
		}
		err = exec.Command(cmdName, cmdArgs...).Run()
		if err != nil {
			// TODO: check exit value
			return true, nil // don't run
		}

		// if there is no watcher and no onlyif check, assume we should run
	} else { // if obj.WatchCmd == "" && obj.IfCmd == "" {
		// just run if state is dirty
		if obj.IsStateOK() { // FIXME: this is done by engine now...
			return true, nil
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%s[%s]: Apply", obj.Kind(), obj.GetName())
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
		cmdArgs = split[1:]
	} else {
		cmdName = obj.Shell // usually bash, or sh
		cmdArgs = []string{"-c", obj.Cmd}
	}
	cmd := exec.Command(cmdName, cmdArgs...)
	//cmd.Dir = "" // look for program in pwd ?
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Start(); err != nil {
		return false, errwrap.Wrapf(err, "Error starting Cmd")
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
			e := errwrap.Wrapf(err, "Error waiting for Cmd")
			return false, e
		}

	case <-util.TimeAfterOrBlock(timeout):
		//cmd.Process.Kill() // TODO: is this necessary?
		return false, fmt.Errorf("Timeout waiting for Cmd!")
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		log.Printf("%s[%s]: Command output is empty!", obj.Kind(), obj.GetName())

	} else {
		log.Printf("%s[%s]: Command output is:", obj.Kind(), obj.GetName())
		log.Printf(out.String())
	}
	// XXX: return based on exit value!!

	// The state tracking is for exec resources that can't "detect" their
	// state, and assume it's invalid when the Watch() function triggers.
	// If we apply state successfully, we should reset it here so that we
	// know that we have applied since the state was set not ok by event!
	// This now happens automatically after the engine runs CheckApply().
	return false, nil // success
}

// ExecUID is the UID struct for ExecRes.
type ExecUID struct {
	BaseUID
	Cmd   string
	IfCmd string
	// TODO: add more elements here
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *ExecUID) IFF(uid ResUID) bool {
	res, ok := uid.(*ExecUID)
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

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *ExecRes) AutoEdges() AutoEdge {
	// TODO: parse as many exec params to look for auto edges, for example
	// the path of the binary in the Cmd variable might be from in a pkg
	return nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *ExecRes) UIDs() []ResUID {
	x := &ExecUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		Cmd:     obj.Cmd,
		IfCmd:   obj.IfCmd,
		// TODO: add more params here
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *ExecRes) GroupCmp(r Res) bool {
	_, ok := r.(*ExecRes)
	if !ok {
		return false
	}
	return false // not possible atm
}

// Compare two resources and return if they are equivalent.
func (obj *ExecRes) Compare(res Res) bool {
	switch res.(type) {
	case *ExecRes:
		res := res.(*ExecRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

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

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *ExecRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes ExecRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*ExecRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to ExecRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = ExecRes(raw) // restore from indirection with type conversion!
	return nil
}
