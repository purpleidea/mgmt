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
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	multierr "github.com/hashicorp/go-multierror"
)

func init() {
	engine.RegisterResource("exec", func() engine.Res { return &ExecRes{} })
}

// ExecRes is an exec resource for running commands.
type ExecRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Sendable

	init *engine.Init

	// Cmd is the command to run. If this is not specified, we use the name.
	Cmd string `yaml:"cmd"`
	// Args is a list of args to pass to Cmd. This can be used *instead* of
	// passing the full command and args as a single string to Cmd. It can
	// only be used when a Shell is *not* specified. The advantage of this
	// is that you don't have to worry about escape characters.
	Args []string `yaml:"args"`
	// Cmd is the dir to run the command in. If empty, then this will use
	// the working directory of the calling process. (This process is mgmt,
	// not the process being run here.)
	Cwd string `yaml:"cwd"`
	// Shell is the (optional) shell to use to run the cmd. If you specify
	// this, then you can't use the Args parameter.
	Shell string `yaml:"shell"`
	// Timeout is the number of seconds to wait before sending a Kill to the
	// running command. If the Kill is received before the process exits,
	// then this be treated as an error.
	Timeout uint64 `yaml:"timeout"`

	// Watch is the command to run to detect event changes. Each line of
	// output from this command is treated as an event.
	WatchCmd string `yaml:"watchcmd"`
	// WatchCwd is the Cwd for the WatchCmd. See the docs for Cwd.
	WatchCwd string `yaml:"watchcwd"`
	// WatchShell is the Shell for the WatchCmd. See the docs for Shell.
	WatchShell string `yaml:"watchshell"`

	// IfCmd is the command that runs to guard against running the Cmd. If
	// this command succeeds, then Cmd *will* be run. If this command
	// returns a non-zero result, then the Cmd will not be run. Any error
	// scenario or timeout will cause the resource to error.
	IfCmd string `yaml:"ifcmd"`
	// IfCwd is the Cwd for the IfCmd. See the docs for Cwd.
	IfCwd string `yaml:"ifcwd"`
	// IfShell is the Shell for the IfCmd. See the docs for Shell.
	IfShell string `yaml:"ifshell"`

	// User is the (optional) user to use to execute the command. It is used
	// for any command being run.
	User string `yaml:"user"`
	// Group is the (optional) group to use to execute the command. It is
	// used for any command being run.
	Group string `yaml:"group"`

	output *string // all cmd output, read only, do not set!
	stdout *string // the cmd stdout, read only, do not set!
	stderr *string // the cmd stderr, read only, do not set!

	interruptChan chan struct{}
	wg            *sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *ExecRes) Default() engine.Res {
	return &ExecRes{}
}

// getCmd returns the actual command to run. When Cmd is not specified, we use
// the Name.
func (obj *ExecRes) getCmd() string {
	if obj.Cmd != "" {
		return obj.Cmd
	}
	return obj.Name()
}

// Validate if the params passed in are valid data.
func (obj *ExecRes) Validate() error {
	if obj.getCmd() == "" { // this is the only thing that is really required
		return fmt.Errorf("the Cmd can't be empty")
	}

	split := strings.Fields(obj.getCmd())
	if len(obj.Args) > 0 && obj.Shell != "" {
		return fmt.Errorf("the Args param can't be used with a Shell")
	}
	if len(obj.Args) > 0 && len(split) > 1 {
		return fmt.Errorf("the Args param can't be used when Cmd has args")
	}

	// check that, if an user or a group is set, we're running as root
	if obj.User != "" || obj.Group != "" {
		currentUser, err := user.Current()
		if err != nil {
			return errwrap.Wrapf(err, "error looking up current user")
		}
		if currentUser.Uid != "0" {
			return fmt.Errorf("running as root is required if you want to use exec with a different user/group")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *ExecRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.interruptChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *ExecRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ExecRes) Watch() error {
	ioChan := make(chan *cmdOutput)
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
			cmdName = obj.WatchShell // usually bash, or sh
			cmdArgs = []string{"-c", obj.WatchCmd}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
		cmd.Dir = obj.WatchCwd // run program in pwd if ""
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

		if ioChan, err = obj.cmdOutputRunner(ctx, cmd); err != nil {
			return errwrap.Wrapf(err, "error starting WatchCmd")
		}
	}

	obj.init.Running() // when started, notify engine that we're running

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
				// error reading input or cmd failure
				exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
				if !ok {
					// command failed in some bad way
					return errwrap.Wrapf(err, "unknown error")
				}
				pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
				wStatus, ok := pStateSys.(syscall.WaitStatus)
				if !ok {
					return errwrap.Wrapf(err, "error running cmd")
				}
				exitStatus := wStatus.ExitStatus()
				obj.init.Logf("watchcmd exited with: %d", exitStatus)
				if exitStatus != 0 {
					return errwrap.Wrapf(err, "unexpected exit status of zero")
				}
				return err // i'm not sure if this could happen
			}

			// each time we get a line of output, we loop!
			if s := data.text; s == "" {
				obj.init.Logf("watch output is empty!")
			} else {
				obj.init.Logf("watch output is:")
				obj.init.Logf(s)
			}
			if data.text != "" {
				send = true
			}

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
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
		cmd.Dir = obj.IfCwd // run program in pwd if ""
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

		var out splitWriter
		out.Init()
		cmd.Stdout = out.Stdout
		cmd.Stderr = out.Stderr

		if err := cmd.Run(); err != nil {
			exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
			if !ok {
				// command failed in some bad way
				return false, err
			}
			pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
			wStatus, ok := pStateSys.(syscall.WaitStatus)
			if !ok {
				return false, errwrap.Wrapf(err, "error running cmd")
			}
			exitStatus := wStatus.ExitStatus()
			if exitStatus == 0 {
				return false, fmt.Errorf("unexpected exit status of zero")
			}

			obj.init.Logf("ifcmd exited with: %d", exitStatus)
			if s := out.String(); s == "" {
				obj.init.Logf("ifcmd output is empty!")
			} else {
				obj.init.Logf("ifcmd output is:")
				obj.init.Logf(s)
			}
			return true, nil // don't run
		}
		if s := out.String(); s == "" {
			obj.init.Logf("ifcmd output is empty!")
		} else {
			obj.init.Logf("ifcmd output is:")
			obj.init.Logf(s)
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
		split := strings.Fields(obj.getCmd())
		cmdName = split[0]
		//d, _ := os.Getwd() // TODO: how does this ever error ?
		//cmdName = path.Join(d, cmdName)
		cmdArgs = split[1:]
		if len(obj.Args) > 0 {
			if len(split) != 1 { // should not happen
				return false, fmt.Errorf("validation error")
			}
			cmdArgs = obj.Args
		}
	} else {
		cmdName = obj.Shell // usually bash, or sh
		cmdArgs = []string{"-c", obj.getCmd()}
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait() // this must be above the defer cancel() call
	var ctx context.Context
	var cancel context.CancelFunc
	if obj.Timeout > 0 { // cmd.Process.Kill() is called on timeout
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(obj.Timeout)*time.Second)
	} else { // zero timeout means no timer
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Dir = obj.Cwd // run program in pwd if ""
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.interruptChan:
			cancel()
		case <-ctx.Done():
			// let this exit
		}
	}()

	err = cmd.Wait() // we can unblock this with the timeout

	// save in memory for send/recv
	// we use pointers to strings to indicate if used or not
	if out.Stdout.Activity || out.Stderr.Activity {
		str := out.String()
		obj.output = &str
	}
	if out.Stdout.Activity {
		str := out.Stdout.String()
		obj.stdout = &str
	}
	if out.Stderr.Activity {
		str := out.Stderr.String()
		obj.stderr = &str
	}

	// process the err result from cmd, we process non-zero exits here too!
	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if err != nil && ok {
		pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
		wStatus, ok := pStateSys.(syscall.WaitStatus)
		if !ok {
			return false, errwrap.Wrapf(err, "error running cmd")
		}
		exitStatus := wStatus.ExitStatus()
		if !wStatus.Signaled() { // not a timeout or cancel (no signal)
			return false, errwrap.Wrapf(err, "cmd error, exit status: %d", exitStatus)
		}
		sig := wStatus.Signal()

		// we get this on timeout, because ctx calls cmd.Process.Kill()
		if sig == syscall.SIGKILL {
			return false, errwrap.Wrapf(err, "cmd timeout, exit status: %d", exitStatus)
		}

		return false, fmt.Errorf("unknown cmd error, signal: %s, exit status: %d", sig, exitStatus)

	} else if err != nil {
		return false, errwrap.Wrapf(err, "general cmd error")
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		obj.init.Logf("command output is empty!")
	} else {
		obj.init.Logf("command output is:")
		obj.init.Logf(s)
	}

	if err := obj.init.Send(&ExecSends{
		Output: obj.output,
		Stdout: obj.stdout,
		Stderr: obj.stderr,
	}); err != nil {
		return false, err
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
	// we can only compare ExecRes to others of the same resource kind
	res, ok := r.(*ExecRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Cmd != res.Cmd {
		return fmt.Errorf("the Cmd differs")
	}
	if len(obj.Args) != len(res.Args) {
		return fmt.Errorf("the Args differ")
	}
	for i, a := range obj.Args {
		if a != res.Args[i] {
			return fmt.Errorf("the Args differ at index: %d", i)
		}
	}
	if obj.Cwd != res.Cwd {
		return fmt.Errorf("the Cwd differs")
	}
	if obj.Shell != res.Shell {
		return fmt.Errorf("the Shell differs")
	}
	if obj.Timeout != res.Timeout {
		return fmt.Errorf("the Timeout differs")
	}

	if obj.WatchCmd != res.WatchCmd {
		return fmt.Errorf("the WatchCmd differs")
	}
	if obj.WatchCwd != res.WatchCwd {
		return fmt.Errorf("the WatchCwd differs")
	}
	if obj.WatchShell != res.WatchShell {
		return fmt.Errorf("the WatchShell differs")
	}

	if obj.IfCmd != res.IfCmd {
		return fmt.Errorf("the IfCmd differs")
	}
	if obj.IfCwd != res.IfCwd {
		return fmt.Errorf("the IfCwd differs")
	}
	if obj.IfShell != res.IfShell {
		return fmt.Errorf("the IfShell differs")
	}

	if obj.User != res.User {
		return fmt.Errorf("the User differs")
	}
	if obj.Group != res.Group {
		return fmt.Errorf("the Group differs")
	}

	return nil
}

// Interrupt is called to ask the execution of this resource to end early.
func (obj *ExecRes) Interrupt() error {
	close(obj.interruptChan)
	return nil
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
		Cmd:     obj.getCmd(),
		IfCmd:   obj.IfCmd,
		// TODO: add more params here
	}
	return []engine.ResUID{x}
}

// ExecSends is the struct of data which is sent after a successful Apply.
type ExecSends struct {
	// Output is the combined stdout and stderr of the command.
	Output *string `lang:"output"`
	// Stdout is the stdout of the command.
	Stdout *string `lang:"stdout"`
	// Stderr is the stderr of the command.
	Stderr *string `lang:"stderr"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *ExecRes) Sends() interface{} {
	return &ExecSends{
		Output: nil,
		Stdout: nil,
		Stderr: nil,
	}
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
	} else if cmdSplit := strings.Fields(obj.getCmd()); len(cmdSplit) > 0 {
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

// cmdOutput is the output struct of the cmdOutputRunner channel output. You
// should always check the error first. If it's nil, then you can assume the
// text data is good to use.
type cmdOutput struct {
	text string
	err  error
}

// cmdOutputRunner wraps the Cmd in with a StdoutPipe scanner and reads for
// errors. It runs Start and Wait, and errors runtime things in the channel.
// If it can't start up the command, it will fail early. Once it's running, it
// will return the channel which can be used for the duration of the process.
// Cancelling the context merely unblocks the sending on the output channel, it
// does not Kill the cmd process. For that you must do it yourself elsewhere.
func (obj *ExecRes) cmdOutputRunner(ctx context.Context, cmd *exec.Cmd) (chan *cmdOutput, error) {
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating StdoutPipe for Cmd")
	}
	scanner := bufio.NewScanner(cmdReader)
	if err := cmd.Start(); err != nil {
		return nil, errwrap.Wrapf(err, "error starting Cmd")
	}

	ch := make(chan *cmdOutput)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch)
		for scanner.Scan() {
			select {
			case ch <- &cmdOutput{text: scanner.Text()}: // blocks here ?
			case <-ctx.Done():
				return
			}
		}

		// on EOF, scanner.Err() will be nil
		reterr := scanner.Err()
		if err := cmd.Wait(); err != nil { // always run Wait()
			if reterr != nil {
				reterr = multierr.Append(reterr, err)
			} else {
				reterr = err
			}
		}
		// send any misc errors we encounter on the channel
		if reterr != nil {
			select {
			case ch <- &cmdOutput{err: reterr}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
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
