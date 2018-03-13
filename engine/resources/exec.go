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

package resources

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

func init() {
	engine.RegisterResource("exec", func() engine.Res { return &ExecRes{} })
}

// ExecRes is an exec resource for running commands.
type ExecRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	Cmd        string  `yaml:"cmd"`        // the command to run
	Shell      string  `yaml:"shell"`      // the (optional) shell to use to run the cmd
	Timeout    int     `yaml:"timeout"`    // the cmd timeout in seconds
	WatchCmd   string  `yaml:"watchcmd"`   // the watch command to run
	WatchShell string  `yaml:"watchshell"` // the (optional) shell to use to run the watch cmd
	IfCmd      string  `yaml:"ifcmd"`      // the if command to run
	IfShell    string  `yaml:"ifshell"`    // the (optional) shell to use to run the if cmd
	User       string  `yaml:"user"`       // the (optional) user to use to execute the command
	Group      string  `yaml:"group"`      // the (optional) group to use to execute the command
	Output     *string // all cmd output, read only, do not set!
	Stdout     *string // the cmd stdout, read only, do not set!
	Stderr     *string // the cmd stderr, read only, do not set!

	wg *sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *ExecRes) Default() engine.Res {
	return &ExecRes{}
}

// Validate if the params passed in are valid data.
func (obj *ExecRes) Validate() error {
	if obj.Cmd == "" { // this is the only thing that is really required
		return fmt.Errorf("command can't be empty")
	}

	// check that, if an user or a group is set, we're running as root
	if obj.User != "" || obj.Group != "" {
		currentUser, err := user.Current()
		if err != nil {
			return errwrap.Wrapf(err, "error looking up current user")
		}
		if currentUser.Uid != "0" {
			return errwrap.Errorf("running as root is required if you want to use exec with a different user/group")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *ExecRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.wg = &sync.WaitGroup{}

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *ExecRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ExecRes) Watch() error {
	ioChan := make(chan *bufioOutput)
	defer obj.wg.Wait()

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
		// ignore signals sent to parent process (we're in our own group)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}

		// if we have a user and group, use them
		var err error
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return errwrap.Wrapf(err, "error while setting credential")
		}

		cmdReader, err := cmd.StdoutPipe()
		if err != nil {
			return errwrap.Wrapf(err, "error creating StdoutPipe for Cmd")
		}
		scanner := bufio.NewScanner(cmdReader)

		defer cmd.Wait() // wait for the command to exit before return!
		defer func() {
			// FIXME: without wrapping this in this func it panic's
			// when running certain graphs... why?
			cmd.Process.Kill() // shutdown the Watch command on exit
		}()
		if err := cmd.Start(); err != nil {
			return errwrap.Wrapf(err, "error starting Cmd")
		}

		ioChan = obj.bufioChanScanner(scanner)
	}

	// notify engine that we're running
	if err := obj.init.Running(); err != nil {
		return err // exit if requested
	}

	var send = false // send event?
	for {
		select {
		case data, ok := <-ioChan:
			if !ok { // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				return fmt.Errorf("reached EOF")
			}
			if err := data.err; err != nil {
				// error reading input?
				return errwrap.Wrapf(err, "unknown error")
			}

			// each time we get a line of output, we loop!
			obj.init.Logf("watch output: %s", data.text)
			if data.text != "" {
				send = true
				obj.init.Dirty() // dirty
			}

		case event, ok := <-obj.init.Events:
			if !ok {
				return nil
			}
			if err := obj.init.Read(event); err != nil {
				return err
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			if err := obj.init.Event(); err != nil {
				return err // exit if requested
			}
		}
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// TODO: expand the IfCmd to be a list of commands
func (obj *ExecRes) CheckApply(apply bool) (bool, error) {
	// If we receive a refresh signal, then the engine skips the IsStateOK()
	// check and this will run. It is still guarded by the IfCmd, but it can
	// have a chance to execute, and all without the check of obj.Refresh()!

	if obj.IfCmd != "" { // if there is no onlyif check, we should just run

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
		cmd := exec.Command(cmdName, cmdArgs...)
		// ignore signals sent to parent process (we're in our own group)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}

		// if we have an user and group, use them
		var err error
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return false, errwrap.Wrapf(err, "error while setting credential")
		}

		if err := cmd.Run(); err != nil {
			// TODO: check exit value
			return true, nil // don't run
		}

	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	obj.init.Logf("Apply")
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
	// ignore signals sent to parent process (we're in our own group)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// if we have a user and group, use them
	var err error
	if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
		return false, errwrap.Wrapf(err, "error while setting credential")
	}

	var out splitWriter
	out.Init()
	// from the docs: "If Stdout and Stderr are the same writer, at most one
	// goroutine at a time will call Write." so we trick it here!
	cmd.Stdout = out.Stdout
	cmd.Stderr = out.Stderr

	if err := cmd.Start(); err != nil {
		return false, errwrap.Wrapf(err, "error starting cmd")
	}

	timeout := obj.Timeout
	if timeout == 0 { // zero timeout means no timer, so disable it
		timeout = -1
	}
	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	select {
	case e := <-done:
		err = e // store

	case <-util.TimeAfterOrBlock(timeout):
		cmd.Process.Kill() // TODO: check error?
		return false, fmt.Errorf("timeout for cmd")
	}

	// save in memory for send/recv
	// we use pointers to strings to indicate if used or not
	if out.Stdout.Activity || out.Stderr.Activity {
		str := out.String()
		obj.Output = &str
	}
	if out.Stdout.Activity {
		str := out.Stdout.String()
		obj.Stdout = &str
	}
	if out.Stderr.Activity {
		str := out.Stderr.String()
		obj.Stderr = &str
	}

	// process the err result from cmd, we process non-zero exits here too!
	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if err != nil && ok {
		pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
		wStatus, ok := pStateSys.(syscall.WaitStatus)
		if !ok {
			return false, errwrap.Wrapf(err, "error running cmd")
		}
		return false, fmt.Errorf("cmd error, exit status: %d", wStatus.ExitStatus())

	} else if err != nil {
		return false, errwrap.Wrapf(err, "general cmd error")
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		obj.init.Logf("Command output is empty!")

	} else {
		obj.init.Logf("Command output is:")
		obj.init.Logf(out.String())
	}

	// The state tracking is for exec resources that can't "detect" their
	// state, and assume it's invalid when the Watch() function triggers.
	// If we apply state successfully, we should reset it here so that we
	// know that we have applied since the state was set not ok by event!
	// This now happens automatically after the engine runs CheckApply().
	return false, nil // success
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *ExecRes) Cmp(r engine.Res) error {
	if !obj.Compare(r) {
		return fmt.Errorf("did not compare")
	}
	return nil
}

// Compare two resources and return if they are equivalent.
func (obj *ExecRes) Compare(r engine.Res) bool {
	// we can only compare ExecRes to others of the same resource kind
	res, ok := r.(*ExecRes)
	if !ok {
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
	if obj.IfShell != res.IfShell {
		return false
	}
	if obj.User != res.User {
		return false
	}
	if obj.Group != res.Group {
		return false
	}

	return true
}

// ExecUID is the UID struct for ExecRes.
type ExecUID struct {
	engine.BaseUID
	Cmd   string
	IfCmd string
	// TODO: add more elements here
}

// ExecResAutoEdges holds the state of the auto edge generator.
type ExecResAutoEdges struct {
	edges []engine.ResUID
}

// Next returns the next automatic edge.
func (obj *ExecResAutoEdges) Next() []engine.ResUID {
	return obj.edges
}

// Test gets results of the earlier Next() call, & returns if we should continue!
func (obj *ExecResAutoEdges) Test(input []bool) bool {
	return false // never keep going
	// TODO: we could return false if we find as many edges as the number of different path's in cmdFiles()
}

// AutoEdges returns the AutoEdge interface. In this case the systemd units.
func (obj *ExecRes) AutoEdges() (engine.AutoEdge, error) {
	var data []engine.ResUID
	for _, x := range obj.cmdFiles() {
		var reversed = true
		data = append(data, &PkgFileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: x, // what matters
		})
	}
	return &ExecResAutoEdges{
		edges: data,
	}, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *ExecRes) UIDs() []engine.ResUID {
	x := &ExecUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		Cmd:     obj.Cmd,
		IfCmd:   obj.IfCmd,
		// TODO: add more params here
	}
	return []engine.ResUID{x}
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

// getCredential returns the correct *syscall.Credential if an User and Group
// are set.
func (obj *ExecRes) getCredential() (*syscall.Credential, error) {
	var uid, gid int
	var err error
	var currentUser *user.User
	if currentUser, err = user.Current(); err != nil {
		return nil, errwrap.Wrapf(err, "error looking up current user")
	}
	if currentUser.Uid != "0" {
		// since we're not root, we've got nothing to do
		return nil, nil
	}

	if obj.Group != "" {
		gid, err = engineUtil.GetGID(obj.Group)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up gid for %s", obj.Group)
		}
	}

	if obj.User != "" {
		uid, err = engineUtil.GetUID(obj.User)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up uid for %s", obj.User)
		}
	}

	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}, nil
}

// cmdFiles returns all the potential files/commands this command might need.
func (obj *ExecRes) cmdFiles() []string {
	var paths []string
	if obj.Shell != "" {
		paths = append(paths, obj.Shell)
	} else if cmdSplit := strings.Fields(obj.Cmd); len(cmdSplit) > 0 {
		paths = append(paths, cmdSplit[0])
	}
	if obj.WatchShell != "" {
		paths = append(paths, obj.WatchShell)
	} else if watchSplit := strings.Fields(obj.WatchCmd); len(watchSplit) > 0 {
		paths = append(paths, watchSplit[0])
	}
	if obj.IfShell != "" {
		paths = append(paths, obj.IfShell)
	} else if ifSplit := strings.Fields(obj.IfCmd); len(ifSplit) > 0 {
		paths = append(paths, ifSplit[0])
	}
	return paths
}

// bufioOutput is the output struct of the bufioChanScanner channel output.
type bufioOutput struct {
	text string
	err  error
}

// bufioChanScanner wraps the scanner output in a channel.
func (obj *ExecRes) bufioChanScanner(scanner *bufio.Scanner) chan *bufioOutput {
	ch := make(chan *bufioOutput)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch)
		for scanner.Scan() {
			ch <- &bufioOutput{text: scanner.Text()} // blocks here ?
		}
		// on EOF, scanner.Err() will be nil
		if err := scanner.Err(); err != nil {
			ch <- &bufioOutput{err: err} // send any misc errors we encounter
		}
	}()
	return ch
}

// splitWriter mimics what the ssh.CombinedOutput command does, but stores the
// the stdout and stderr separately. This is slightly tricky because we don't
// want the combined output to be interleaved incorrectly. It creates sub writer
// structs which share the same lock and a shared output buffer.
type splitWriter struct {
	Stdout *wrapWriter
	Stderr *wrapWriter

	stdout      bytes.Buffer // just the stdout
	stderr      bytes.Buffer // just the stderr
	output      bytes.Buffer // combined output
	mutex       *sync.Mutex
	initialized bool // is this initialized?
}

// Init initializes the splitWriter.
func (obj *splitWriter) Init() {
	if obj.initialized {
		panic("splitWriter is already initialized")
	}
	obj.mutex = &sync.Mutex{}
	obj.Stdout = &wrapWriter{
		Mutex:  obj.mutex,
		Buffer: &obj.stdout,
		Output: &obj.output,
	}
	obj.Stderr = &wrapWriter{
		Mutex:  obj.mutex,
		Buffer: &obj.stderr,
		Output: &obj.output,
	}
	obj.initialized = true
}

// String returns the contents of the combined output buffer.
func (obj *splitWriter) String() string {
	if !obj.initialized {
		panic("splitWriter is not initialized")
	}
	return obj.output.String()
}

// wrapWriter is a simple writer which is used internally by splitWriter.
type wrapWriter struct {
	Mutex    *sync.Mutex
	Buffer   *bytes.Buffer // stdout or stderr
	Output   *bytes.Buffer // combined output
	Activity bool          // did we get any writes?
}

// Write writes to both bytes buffers with a parent lock to mix output safely.
func (obj *wrapWriter) Write(p []byte) (int, error) {
	// TODO: can we move the lock to only guard around the Output.Write ?
	obj.Mutex.Lock()
	defer obj.Mutex.Unlock()
	obj.Activity = true
	i, err := obj.Buffer.Write(p) // first write
	if err != nil {
		return i, err
	}
	return obj.Output.Write(p) // shared write
}

// String returns the contents of the unshared buffer.
func (obj *wrapWriter) String() string {
	return obj.Buffer.String()
}
