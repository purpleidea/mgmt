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
	"log"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("exec", func() Res { return &ExecRes{} })
}

// ExecRes is an exec resource for running commands.
type ExecRes struct {
	BaseRes    `yaml:",inline"`
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

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *ExecRes) Init() error {
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
func (obj *ExecRes) Watch() error {
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

		bufioch, errch = obj.BufioChanScanner(scanner)
	}

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	for {
		select {
		case text := <-bufioch:
			// each time we get a line of output, we loop!
			log.Printf("%s: Watch output: %s", obj, text)
			if text != "" {
				send = true
				obj.StateOK(false) // something made state dirty
			}

		case err := <-errch:
			if err == nil { // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				return fmt.Errorf("reached EOF")
			}
			// error reading input?
			return errwrap.Wrapf(err, "unknown error")

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
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
	log.Printf("%s: Apply", obj)
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
			e := errwrap.Wrapf(err, "error running cmd")
			return false, e
		}
		return false, fmt.Errorf("cmd error, exit status: %d", wStatus.ExitStatus())

	} else if err != nil {
		e := errwrap.Wrapf(err, "general cmd error")
		return false, e
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		log.Printf("%s: Command output is empty!", obj)

	} else {
		log.Printf("%s: Command output is:", obj)
		log.Printf(out.String())
	}

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

// ExecResAutoEdges holds the state of the auto edge generator.
type ExecResAutoEdges struct {
	edges []ResUID
}

// Next returns the next automatic edge.
func (obj *ExecResAutoEdges) Next() []ResUID {
	return obj.edges
}

// Test gets results of the earlier Next() call, & returns if we should continue!
func (obj *ExecResAutoEdges) Test(input []bool) bool {
	return false // Never keep going
	// TODO: We could return false if we find as many edges as the number of different path in cmdFiles()
}

// AutoEdges returns the AutoEdge interface. In this case the systemd units.
func (obj *ExecRes) AutoEdges() (AutoEdge, error) {
	var data []ResUID
	for _, x := range obj.cmdFiles() {
		var reversed = true
		data = append(data, &PkgFileUID{
			BaseUID: BaseUID{
				Name:     obj.GetName(),
				Kind:     obj.GetKind(),
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
func (obj *ExecRes) UIDs() []ResUID {
	x := &ExecUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
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
func (obj *ExecRes) Compare(r Res) bool {
	// we can only compare ExecRes to others of the same resource kind
	res, ok := r.(*ExecRes)
	if !ok {
		return false
	}
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
		gid, err = GetGID(obj.Group)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up gid for %s", obj.Group)
		}
	}

	if obj.User != "" {
		uid, err = GetUID(obj.User)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up uid for %s", obj.User)
		}
	}

	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}, nil
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
func (sw *splitWriter) Init() {
	if sw.initialized {
		panic("splitWriter is already initialized")
	}
	sw.mutex = &sync.Mutex{}
	sw.Stdout = &wrapWriter{
		Mutex:  sw.mutex,
		Buffer: &sw.stdout,
		Output: &sw.output,
	}
	sw.Stderr = &wrapWriter{
		Mutex:  sw.mutex,
		Buffer: &sw.stderr,
		Output: &sw.output,
	}
	sw.initialized = true
}

// String returns the contents of the combined output buffer.
func (sw *splitWriter) String() string {
	if !sw.initialized {
		panic("splitWriter is not initialized")
	}
	return sw.output.String()
}

// wrapWriter is a simple writer which is used internally by splitWriter.
type wrapWriter struct {
	Mutex    *sync.Mutex
	Buffer   *bytes.Buffer // stdout or stderr
	Output   *bytes.Buffer // combined output
	Activity bool          // did we get any writes?
}

// Write writes to both bytes buffers with a parent lock to mix output safely.
func (w *wrapWriter) Write(p []byte) (int, error) {
	// TODO: can we move the lock to only guard around the Output.Write ?
	w.Mutex.Lock()
	defer w.Mutex.Unlock()
	w.Activity = true
	i, err := w.Buffer.Write(p) // first write
	if err != nil {
		return i, err
	}
	return w.Output.Write(p) // shared write
}

// String returns the contents of the unshared buffer.
func (w *wrapWriter) String() string {
	return w.Buffer.String()
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
