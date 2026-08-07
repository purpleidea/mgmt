// Mgmt
// Copyright (C) James Shubin and the project contributors
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

package resources

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	"golang.org/x/sys/unix"
)

func init() {
	engine.RegisterResource("exec", func() engine.Res { return &ExecRes{} })
}

var _ engine.EdgeableRes = &ExecRes{} // compile time check

const (
	// execCmdSignal is the default signal we send to a running command when
	// its context is cancelled. This is more correct since we're more of a
	// "service manager" than interactive tool, which would send SIGINT.
	execCmdSignal = syscall.SIGTERM

	// execCmdInterruptSignal is the signal we send to a running command
	// when the engine interrupts us. It's the last rung of the exit
	// sequence and the last thing we can do before a kill -9 of mgmt
	// itself, so it should be the one that a command can't catch or ignore.
	execCmdInterruptSignal = syscall.SIGKILL
)

// ExecRes is an exec resource for running commands.
//
// This resource attempts to minimise the effects of the execution environment,
// and, in particular, will start the new process with an empty environment (as
// would `execve` with an empty `envp` array). If you want the environment to
// inherit the mgmt process environment, you can import it from "sys" and use it
// with `env => sys.env()` in your exec resource.
type ExecRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Sendable

	init *engine.Init

	// Cmd is the command to run. If this is not specified, we use the name.
	// Remember that if you're not using `Shell` (the default) then adding
	// single quotes around args make them part of the actual values. IOW,
	// if your command is: "touch '/tmp/foo'", then (1) it probably won't be
	// able to find the "touch" command (use /usr/bin/touch instead) and (2)
	// the file won't be in the /tmp/ directory, it will be an oddly named
	// file that contains two single quotes, and it will likely error since
	// the dir path doesn't exist. In general, it's best to use the `Args`
	// field instead of including them here.
	// XXX: if not using shell, don't allow args here, force them to args!
	Cmd string `lang:"cmd" yaml:"cmd"`

	// Args is a list of args to pass to Cmd. This can be used *instead* of
	// passing the full command and args as a single string to Cmd. It can
	// only be used when a Shell is *not* specified. The advantage of this
	// is that you don't have to worry about escape characters.
	Args []string `lang:"args" yaml:"args"`

	// Cwd is the dir to run the command in. If empty, then this will use
	// the working directory of the calling process. (This process is mgmt,
	// not the process being run here.) Keep in mind that if you're running
	// this command as a user that does not have perms to the current
	// directory, you may wish to set this to `/` to avoid hitting an error
	// such as: `could not change directory to "/root": Permission denied`.
	Cwd string `lang:"cwd" yaml:"cwd"`

	// Shell is the (optional) shell to use to run the cmd. If you specify
	// this, then you can't use the Args parameter. Note that unless you use
	// absolute paths, or set the PATH variable, the shell might not be able
	// to find the program you're trying to run.
	Shell string `lang:"shell" yaml:"shell"`

	// Env allows the user to specify environment variables for script
	// execution. These are taken using a map of format of VAR_KEY -> value.
	// Omitting this value or setting it to an empty array will cause the
	// program to be run with an empty environment. These values are used
	// for every command. If there's a legitimate need to have different
	// environments for each command, then we'll split that out eventually.
	Env map[string]string `lang:"env" yaml:"env"`

	// CancelSignal is the signal to send to a running command to tell it to
	// stop. That happens when our context is cancelled via the second ^C
	// but it can also happen when the Meta:timeout expires. We default to
	// SIGTERM, but you may choose a different signal if you prefer such as
	// SIGINT.
	//
	// It is sent to every command that this resource runs, and to the whole
	// process group, so that a shell passes it on to its children. This
	// does not change what happens when the engine interrupts us, which is
	// always a SIGKILL.
	CancelSignal string `lang:"cancel_signal" yaml:"cancel_signal"`

	// WatchCmd is the command to run to detect event changes. Each line of
	// output from this command is treated as an event.
	WatchCmd string `lang:"watchcmd" yaml:"watchcmd"`

	// WatchCwd is the Cwd for the WatchCmd. See the docs for Cwd.
	WatchCwd string `lang:"watchcwd" yaml:"watchcwd"`

	// WatchShell is the Shell for the WatchCmd. See the docs for Shell.
	WatchShell string `lang:"watchshell" yaml:"watchshell"`

	// WatchFiles is a list of files that will be kept track of.
	WatchFiles []string `lang:"watchfiles" yaml:"watchfiles"`

	// IfCmd is the command that runs to guard against running the Cmd. If
	// this command succeeds, then Cmd *will not* be blocked from running.
	// If this command returns a non-zero result, then the Cmd will not be
	// run. Any error scenario or timeout will cause the resource to error.
	// There is *no* guarantee that this command will be ever run. For
	// example, if one of the Mtimes is newer, we won't be able to block the
	// main command from running, and this check might be skipped.
	IfCmd string `lang:"ifcmd" yaml:"ifcmd"`

	// IfCwd is the Cwd for the IfCmd. See the docs for Cwd.
	IfCwd string `lang:"ifcwd" yaml:"ifcwd"`

	// IfShell is the Shell for the IfCmd. See the docs for Shell.
	IfShell string `lang:"ifshell" yaml:"ifshell"`

	// IfEquals specifies that if the ifcmd returns zero, and that the
	// output matches this string, then it will guard against the Cmd
	// running. This can be the empty string. Remember to take into account
	// if the output includes a trailing newline or not. (Hint: it usually
	// does!)
	IfEquals *string `lang:"ifequals" yaml:"ifequals"`

	// IfEqualsStdout is like IfEquals, except that it only compares against
	// the stdout of the ifcmd, instead of against the combined stdout and
	// stderr. This is useful when the ifcmd might print warnings to stderr
	// which shouldn't factor into the comparison.
	IfEqualsStdout *string `lang:"ifequals_stdout" yaml:"ifequals_stdout"`

	// NIfCmd is the command that runs to guard against running the Cmd. If
	// this command succeeds, then Cmd *will* be blocked from running. If
	// this command returns a non-zero result, then the Cmd will be allowed
	// to run if not blocked by anything else. This is the opposite of the
	// IfCmd. There is *no* guarantee that this command will be ever run.
	// For example, if one of the Mtimes is newer, we won't be able to block
	// the main command from running, and this check might be skipped.
	NIfCmd string `lang:"nifcmd" yaml:"nifcmd"`

	// NIfCwd is the Cwd for the NIfCmd. See the docs for Cwd.
	NIfCwd string `lang:"nifcwd" yaml:"nifcwd"`

	// NIfShell is the Shell for the NIfCmd. See the docs for Shell.
	NIfShell string `lang:"nifshell" yaml:"nifshell"`

	// Creates is the absolute file path to check for before running the
	// main cmd. If this path exists, then the cmd will not run. More
	// precisely we attempt to `stat` the file, so it must succeed for a
	// skip. This also adds a watch on this path which re-checks things when
	// it changes. There is *no* guarantee that this check will be used if
	// for example one of the Mtimes is newer, we won't be able to block the
	// main command from running, and this check might be skipped.
	Creates string `lang:"creates" yaml:"creates"`

	// Mtimes is a list of files that will be kept track of. When any of the
	// mtimes is newer than the time the last command ran, then the command
	// will run again. This also adds a watch to each of these paths, and
	// will error if any of these files is missing. If any of these indicate
	// that the command needs running again, it will do so, even if it would
	// otherwise be blocked by IfCmd, NIfCmd, Creates and so on... Keep in
	// mind that use of this param may prevent IfCmd or others from running!
	// The reason it's okay to err on the side of causing a new exec of the
	// main command, is because they're supposed to be idempotent most of
	// the time, and at worst, they should be expensive, not catastrophic!
	// You may wish to combine this with `ifcmd => "/bin/false"` to prevent
	// the command running when the mtimes are not out of date, since this
	// only forces a run, it doesn't block a run.
	Mtimes []string `lang:"mtimes" yaml:"mtimes"`

	// DoneCmd is the command that runs after the regular Cmd runs
	// successfully. This is a useful pattern to avoid the shelling out to
	// bash simply to do `$cmd && echo done > /tmp/donefile`. If this
	// command errors, it behaves as if the normal Cmd had errored.
	DoneCmd string `lang:"donecmd" yaml:"donecmd"`

	// DoneCwd is the Cwd for the DoneCmd. See the docs for Cwd.
	DoneCwd string `lang:"donecwd" yaml:"donecwd"`

	// DoneShell is the Shell for the DoneCmd. See the docs for Shell.
	DoneShell string `lang:"doneshell" yaml:"doneshell"`

	// User is the (optional) user to use to execute the command. It is used
	// for any command being run.
	User string `lang:"user" yaml:"user"`

	// Group is the (optional) group to use to execute the command. It is
	// used for any command being run.
	Group string `lang:"group" yaml:"group"`

	// SendOutput is a value which can be sent for the Send/Recv Output
	// field if no value is available in the cache. This is used in very
	// specialized scenarios (particularly prototyping and unclean
	// environments) and should not be used routinely. It should be used
	// only in situations where we didn't produce our own sending values,
	// and there are none in the cache, and instead are relying on a runtime
	// mechanism to help us out. This can commonly occur if you wish to make
	// incremental progress when locally testing some code using Send/Recv,
	// but you are combining it with --tmp-prefix for other reasons.
	SendOutput *string `lang:"send_output" yaml:"send_output"`

	// SendStdout is like SendOutput but for stdout alone. See those docs.
	SendStdout *string `lang:"send_stdout" yaml:"send_stdout"`

	// SendStderr is like SendOutput but for stderr alone. See those docs.
	SendStderr *string `lang:"send_stderr" yaml:"send_stderr"`

	output *string // all cmd output, read only, do not set!
	stdout *string // the cmd stdout, read only, do not set!
	stderr *string // the cmd stderr, read only, do not set!

	dir           string // the path to local storage
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

// getCancelSignal returns the signal that we send to a running command when its
// context is cancelled.
func (obj *ExecRes) getCancelSignal() (syscall.Signal, error) {
	if obj.CancelSignal == "" {
		return execCmdSignal, nil // the default
	}

	s := obj.CancelSignal
	signal := unix.SignalNum(s)
	if signal != 0 {
		return signal, nil // success!
	}
	// it's not a signal that we know about, but be helpful in the error...

	s = strings.ToUpper(s) // did they forget to capitalize?
	if unix.SignalNum(s) != 0 {
		return 0, fmt.Errorf("invalid CancelSignal of: %q, did you mean: %q ?", obj.CancelSignal, s)
	}

	s = "SIG" + s // did they forget the prefix?
	if unix.SignalNum(s) != 0 {
		return 0, fmt.Errorf("invalid CancelSignal of: %q, did you mean: %q ?", obj.CancelSignal, s)
	}

	return 0, fmt.Errorf("invalid CancelSignal of: %q", obj.CancelSignal)
}

// validateUserGroup is just a small helper that is used by Validate().
func (obj *ExecRes) validateUserGroup() error {

	// Check that if a user or group is set, we are running as root, or
	// already running with the requested user/group.
	if obj.User == "" && obj.Group == "" {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return errwrap.Wrapf(err, "error looking up current user")
	}

	if currentUser.Uid == "0" {
		return nil // changing to any user is allowed since we're root!
	}
	//if currentUser.Gid == "0" { // XXX: Do we want to add this case too?
	//	return nil
	//}

	if obj.User != "" {
		uid, err := engineUtil.GetUID(obj.User)
		if err != nil {
			return errwrap.Wrapf(err, "error looking up uid for %s", obj.User)
		}
		if strconv.Itoa(uid) != currentUser.Uid {
			return fmt.Errorf("running as root is required if you want to use exec with a different user")
		}
	}
	if obj.Group != "" {
		gid, err := engineUtil.GetGID(obj.Group)
		if err != nil {
			return errwrap.Wrapf(err, "error looking up gid for %s", obj.Group)
		}
		if strconv.Itoa(gid) != currentUser.Gid {
			return fmt.Errorf("running as root is required if you want to use exec with a different group")
		}
	}

	return nil
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

	for key := range obj.Env {
		if err := isNameValid(key); err != nil {
			return errwrap.Wrapf(err, "invalid variable name")
		}
	}

	if _, err := obj.getCancelSignal(); err != nil {
		return err
	}

	for _, file := range obj.WatchFiles {
		if !strings.HasPrefix(file, "/") {
			return fmt.Errorf("the path (`%s`) in WatchFiles must be absolute", file)
		}
	}

	if obj.Creates != "" && !strings.HasPrefix(obj.Creates, "/") {
		return fmt.Errorf("the Creates param must be an absolute path")
	}

	for _, file := range obj.Mtimes {
		if !strings.HasPrefix(file, "/") {
			return fmt.Errorf("the path (`%s`) in Mtimes must be absolute", file)
		}
	}

	if err := obj.validateUserGroup(); err != nil {
		return err
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *ExecRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.dir = dir

	obj.interruptChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *ExecRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ExecRes) Watch(ctx context.Context) error {
	defer obj.wg.Wait()

	ioChan := make(chan *cmdOutput)
	filesChan := make(chan *recwatch.Event)

	var watchCmd *execCmd
	if obj.WatchCmd != "" {
		innerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cmd := &execCmd{
			Name:      "watchcmd",
			Command:   obj.WatchCmd,
			Shell:     obj.WatchShell,
			Env:       obj.Env,
			Interrupt: obj.interruptChan,
			Logf:      obj.init.Logf,
		}
		if err := cmd.Init(innerCtx); err != nil {
			return err
		}
		cmd.Dir = obj.WatchCwd // run program in pwd if ""

		watchCmd = cmd // store for errors

		// if we have a user and group, use them
		var err error
		if cmd.CancelSignal, err = obj.getCancelSignal(); err != nil {
			return err
		}
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return errwrap.Wrapf(err, "error while setting credential")
		}

		if ioChan, err = obj.cmdOutputRunner(innerCtx, cmd); err != nil {
			return errwrap.Wrapf(err, "error starting WatchCmd")
		}
	}

	fileList := []string{}
	fileList = append(fileList, obj.Mtimes...)
	fileList = append(fileList, obj.WatchFiles...)
	if obj.Creates != "" {
		fileList = append(fileList, obj.Creates)
	}
	for _, file := range fileList {
		recurse := strings.HasSuffix(file, "/") // check if it's a file or dir
		recWatcher, err := recwatch.NewRecWatcher(file, recurse)
		if err != nil {
			return err
		}
		defer recWatcher.Close()

		obj.wg.Add(1)
		go func() {
			defer obj.wg.Done()
			for {
				var files *recwatch.Event
				var ok bool
				var shutdown bool

				select {
				case files, ok = <-recWatcher.Events(): // receiving events
				case <-ctx.Done(): // unblock
					return
				}

				if !ok {
					err := fmt.Errorf("channel shutdown")
					files = &recwatch.Event{Error: err}
					shutdown = true
				}

				select {
				case filesChan <- files: // send events
					if shutdown { // optimization to free early
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case data, ok := <-ioChan:
			if !ok { // EOF
				// FIXME: add an "if watch command ends/crashes"
				// restart or generate error option
				return fmt.Errorf("reached EOF")
			}
			if err := data.err; err != nil {
				// We're the ones who stopped it, so how it died
				// isn't news that anyone needs to hear about.
				if e := watchCmd.stopped(); e != nil {
					return e
				}

				// error reading input or cmd failure
				exitStatus, e := watchCmd.exitStatus(err)
				if e != nil {
					return e
				}

				obj.init.Logf("watchcmd: %s", watchCmd)
				obj.init.Logf("watchcmd exited with: %d", exitStatus)
				return errwrap.Wrapf(err, "watchcmd errored")
			}

			// each time we get a line of output, we loop!
			if s := data.text; s == "" {
				obj.init.Logf("watch out empty!")
			} else {
				obj.init.Logf("watch out:")
				obj.init.Logf("%s", s)
			}
			if data.text == "" { // TODO: do we want to skip event?
				continue
			}

		case event, ok := <-filesChan:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected recwatch shutdown")
			}
			if event == nil {
				return fmt.Errorf("unexpected nil recwatch event")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// TODO: expand the IfCmd to be a list of commands
func (obj *ExecRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// If we receive a refresh signal, then the engine skips the IsStateOK()
	// check and this will run. It is still guarded by the IfCmd, but it can
	// have a chance to execute, and all without the check of obj.Refresh()!

	if err := obj.checkApplyReadCache(); err != nil {
		return false, err
	}

	forceRun := false
	var mtime time.Time
	if len(obj.Mtimes) > 0 {
		p := path.Join(obj.dir, "mtimes")
		fileInfo, err := os.Stat(p)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if err == nil {
			mtime = fileInfo.ModTime()
		}
		// otherwise mtime stays zero! (file doesn't exist yet)
	}
	for _, f := range obj.Mtimes {
		fileInfo, err := os.Stat(f)
		if err != nil {
			return false, err
		}

		m := fileInfo.ModTime()
		if m.After(mtime) { // if m is after mtime
			// Yes there could be some file with an mtime in the
			// future, but we don't need to worry about that
			// scenario, since we'll set the mtime to a time after
			// the command ran. (Same as when DoneCmd would run.)
			forceRun = true
			break
		}
	}

	if obj.IfCmd != "" && !forceRun { // if there is no onlyif check, we should just run
		cmd := &execCmd{
			Name:      "ifcmd",
			Command:   obj.IfCmd,
			Shell:     obj.IfShell,
			Env:       obj.Env,
			Interrupt: obj.interruptChan,
			Logf:      obj.init.Logf,
		}
		if err := cmd.Init(ctx); err != nil {
			return false, err
		}
		cmd.Dir = obj.IfCwd // run program in pwd if ""

		// if we have an user and group, use them
		var err error
		if cmd.CancelSignal, err = obj.getCancelSignal(); err != nil {
			return false, err
		}
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return false, errwrap.Wrapf(err, "error while setting credential")
		}

		if err := cmd.capture(); err != nil {
			return false, err
		}

		if err := cmd.run(); err != nil {
			// A command that we stopped never got to tell us
			// anything, so we must not read a decision into it.
			if e := cmd.stopped(); e != nil {
				return false, e
			}

			exitStatus, e := cmd.exitStatus(err)
			if e != nil {
				return false, e
			}
			out := cmd.output()

			obj.init.Logf("ifcmd: %s", cmd)
			obj.init.Logf("ifcmd exited with: %d, skipping cmd", exitStatus)
			if s := out.String(); s == "" {
				obj.init.Logf("ifcmd out empty!")
			} else {
				obj.init.Logf("ifcmd out:")
				obj.init.Logf("%s", s)
			}
			//if err := obj.checkApplyWriteCache(); err != nil {
			//	return false, err
			//}
			obj.safety()
			if err := obj.send(); err != nil {
				return false, err
			}
			return true, nil // don't run
		}
		out := cmd.output()
		s := out.String()
		if s == "" {
			obj.init.Logf("ifcmd out empty!")
		} else {
			obj.init.Logf("ifcmd out:")
			obj.init.Logf("%s", s)
		}
		if obj.IfEquals != nil && *obj.IfEquals == s {
			obj.init.Logf("ifequals matched")
			obj.safety()
			if err := obj.send(); err != nil {
				return false, err
			}
			return true, nil // don't run
		}
		if obj.IfEqualsStdout != nil && *obj.IfEqualsStdout == out.Stdout.String() {
			obj.init.Logf("ifequals stdout matched")
			obj.safety()
			if err := obj.send(); err != nil {
				return false, err
			}
			return true, nil // don't run
		}
	}

	if obj.NIfCmd != "" && !forceRun { // opposite of the ifcmd check
		cmd := &execCmd{
			Name:      "nifcmd",
			Command:   obj.NIfCmd,
			Shell:     obj.NIfShell,
			Env:       obj.Env,
			Interrupt: obj.interruptChan,
			Logf:      obj.init.Logf,
		}
		if err := cmd.Init(ctx); err != nil {
			return false, err
		}
		cmd.Dir = obj.NIfCwd // run program in pwd if ""

		// if we have an user and group, use them
		var err error
		if cmd.CancelSignal, err = obj.getCancelSignal(); err != nil {
			return false, err
		}
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return false, errwrap.Wrapf(err, "error while setting credential")
		}

		if err := cmd.capture(); err != nil {
			return false, err
		}

		err = cmd.run()
		out := cmd.output()
		if err == nil {
			obj.init.Logf("nifcmd: %s", cmd)
			obj.init.Logf("nifcmd exited with: %d, skipping cmd", 0)
			s := out.String()
			if s == "" {
				obj.init.Logf("nifcmd out empty!")
			} else {
				obj.init.Logf("nifcmd out:")
				obj.init.Logf("%s", s)
			}

			//if err := obj.checkApplyWriteCache(); err != nil {
			//	return false, err
			//}
			obj.safety()
			if err := obj.send(); err != nil {
				return false, err
			}
			return true, nil // don't run
		}

		// A command that we stopped never got to tell us anything, so
		// we must not read a decision into it.
		if e := cmd.stopped(); e != nil {
			return false, e
		}

		exitStatus, e := cmd.exitStatus(err)
		if e != nil {
			return false, e
		}

		obj.init.Logf("nifcmd: %s", cmd)
		obj.init.Logf("nifcmd exited with: %d, not skipping cmd", exitStatus)
		if s := out.String(); s == "" {
			obj.init.Logf("nifcmd out empty!")
		} else {
			obj.init.Logf("nifcmd out:")
			obj.init.Logf("%s", s)
		}

		//if obj.NIfEquals != nil && *obj.NIfEquals == s {
		//	obj.init.Logf("nifequals matched")
		//	return true, nil // don't run
		//}
	}

	if obj.Creates != "" && !forceRun { // gate the extra syscall
		if _, err := os.Stat(obj.Creates); err == nil {
			obj.init.Logf("creates file exists, skipping cmd")
			//if err := obj.checkApplyWriteCache(); err != nil {
			//	return false, err
			//}
			obj.safety()
			if err := obj.send(); err != nil {
				return false, err
			}
			return true, nil // don't run
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		//if err := obj.checkApplyWriteCache(); err != nil {
		//	return false, err
		//}
		//obj.safety()
		if err := obj.send(); err != nil {
			return false, err
		}
		return false, nil
	}

	// apply portion
	cmd := &execCmd{
		Name:      "cmd",
		Command:   obj.getCmd(),
		Shell:     obj.Shell,
		Args:      obj.Args,
		Env:       obj.Env,
		Interrupt: obj.interruptChan,
		Logf:      obj.init.Logf,
	}
	if err := cmd.Init(ctx); err != nil {
		return false, err
	}
	cmd.Dir = obj.Cwd // run program in pwd if ""

	// if we have a user and group, use them
	var err error
	if cmd.CancelSignal, err = obj.getCancelSignal(); err != nil {
		return false, err
	}
	if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
		return false, errwrap.Wrapf(err, "error while setting credential")
	}

	if err := cmd.capture(); err != nil {
		return false, err
	}

	obj.init.Logf("cmd: %s", cmd)
	if err := cmd.start(); err != nil {
		// We were already on our way out before this ever got going.
		if e := cmd.stopped(); e != nil {
			return false, e
		}
		return false, errwrap.Wrapf(err, "error starting cmd")
	}
	err = cmd.wait()
	out := cmd.output()

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

	// We stopped this command, so whatever it did in response doesn't tell
	// us that the resource is in the state that we wanted. This must come
	// before we look at how it exited, because a command which handles the
	// signal we sent is free to exit with a zero status, and if we took
	// that at face value we'd go on to run the DoneCmd and to save the
	// mtime and the cached output as if the work had actually been done. If
	// we ever want a command to be able to say that a graceful stop counts
	// as a success, then this is the place where that option would go.
	if e := cmd.stopped(); e != nil {
		return false, e
	}

	// process the err result from cmd, we process non-zero exits here too!
	if err != nil {
		// A command which something else in the system killed errors in
		// here, since we checked above that it wasn't us who did it.
		exitStatus, e := cmd.exitStatus(err)
		if e != nil {
			return false, e
		}

		// most commands error in this way
		if s := out.String(); s == "" {
			obj.init.Logf("exit status %d", exitStatus)
		} else {
			obj.init.Logf("cmd error: %s", s)
		}

		return false, errwrap.Wrapf(err, "cmd error") // exit status will be in the error
	}

	// TODO: if we printed the stdout while the command is running, this
	// would be nice, but it would require terminal log output that doesn't
	// interleave all the parallel parts which would mix it all up...
	if s := out.String(); s == "" {
		obj.init.Logf("cmd out empty!")
	} else {
		obj.init.Logf("cmd out:")
		obj.init.Logf("%s", s)
	}

	if obj.DoneCmd != "" {
		cmd := &execCmd{
			Name:      "donecmd",
			Command:   obj.DoneCmd,
			Shell:     obj.DoneShell,
			Env:       obj.Env,
			Interrupt: obj.interruptChan,
			Logf:      obj.init.Logf,
		}
		if err := cmd.Init(ctx); err != nil {
			return false, err
		}
		cmd.Dir = obj.DoneCwd // run program in pwd if ""

		// if we have an user and group, use them
		var err error
		if cmd.CancelSignal, err = obj.getCancelSignal(); err != nil {
			return false, err
		}
		if cmd.SysProcAttr.Credential, err = obj.getCredential(); err != nil {
			return false, errwrap.Wrapf(err, "error while setting credential")
		}

		if err := cmd.capture(); err != nil {
			return false, err
		}

		if err := cmd.run(); err != nil {
			// A command that we stopped never got to tell us
			// anything, so we must not read a decision into it.
			if e := cmd.stopped(); e != nil {
				return false, e
			}

			exitStatus, e := cmd.exitStatus(err)
			if e != nil {
				return false, e
			}
			out := cmd.output()

			obj.init.Logf("donecmd: %s", cmd)
			if s := out.String(); s == "" {
				obj.init.Logf("donecmd exit status %d", exitStatus)
			} else {
				obj.init.Logf("donecmd error: %s", s)
			}
			return false, errwrap.Wrapf(err, "cmd error") // exit status will be in the error
		}
		if s := cmd.output().String(); s == "" {
			obj.init.Logf("donecmd out empty!")
		} else {
			obj.init.Logf("donecmd out:")
			obj.init.Logf("%s", s)
		}
	}

	// Store the mtime as an mtime of last run time.
	if len(obj.Mtimes) > 0 {
		p := path.Join(obj.dir, "mtimes")
		f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return false, err
		}
		// If you don't write anything, this won't update the mtime of the file!
		if _, err := f.WriteString(time.Now().Format(time.RFC3339Nano) + "\n"); err != nil {
			return false, err
		}
		if err := f.Close(); err != nil {
			return false, err
		}
	}

	if err := obj.checkApplyWriteCache(); err != nil {
		return false, err
	}
	if err := obj.send(); err != nil {
		return false, err
	}

	// The state tracking is for exec resources that can't "detect" their
	// state, and assume it's invalid when the Watch() function triggers.
	// If we apply state successfully, we should reset it here so that we
	// know that we have applied since the state was set not ok by event!
	// This now happens automatically after the engine runs CheckApply().
	return false, nil // success
}

// send is a helper to avoid duplication of the same send operation.
func (obj *ExecRes) send() error {
	return obj.init.Send(&ExecSends{
		Output: obj.output,
		Stdout: obj.stdout,
		Stderr: obj.stderr,
	})
}

// safety is a helper function that populates the cached "send" values if they
// are empty. It must only be called right before actually sending any values,
// and right before CheckApply returns. It should be used only in situations
// where we didn't produce our own sending values, and there are none in the
// cache, and instead are relying on a runtime mechanism to help us out. This
// mechanism is useful as a backstop for when we're running in unclean
// scenarios.
func (obj *ExecRes) safety() {
	if x := obj.SendOutput; x != nil && obj.output == nil {
		s := *x // copy
		obj.output = &s
	}
	if x := obj.SendStdout; x != nil && obj.stdout == nil {
		s := *x // copy
		obj.stdout = &s
	}
	if x := obj.SendStderr; x != nil && obj.stderr == nil {
		s := *x // copy
		obj.stderr = &s
	}
}

// checkApplyReadCache is a helper to do all our reading from the cache.
func (obj *ExecRes) checkApplyReadCache() error {
	output, err := engineUtil.ReadData(path.Join(obj.dir, "output"))
	if err != nil {
		return err
	}
	obj.output = output

	stdout, err := engineUtil.ReadData(path.Join(obj.dir, "stdout"))
	if err != nil {
		return err
	}
	obj.stdout = stdout

	stderr, err := engineUtil.ReadData(path.Join(obj.dir, "stderr"))
	if err != nil {
		return err
	}
	obj.stderr = stderr

	return nil
}

// checkApplyWriteCache is a helper to do all our writing into the cache.
func (obj *ExecRes) checkApplyWriteCache() error {
	if _, err := engineUtil.WriteData(path.Join(obj.dir, "output"), obj.output); err != nil {
		return err
	}

	if _, err := engineUtil.WriteData(path.Join(obj.dir, "stdout"), obj.stdout); err != nil {
		return err
	}

	if _, err := engineUtil.WriteData(path.Join(obj.dir, "stderr"), obj.stderr); err != nil {
		return err
	}

	return nil
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
	if err := engineUtil.StrMapCmp(obj.Env, res.Env); err != nil {
		return errwrap.Wrapf(err, "the Env differs")
	}
	if obj.CancelSignal != res.CancelSignal {
		return fmt.Errorf("the CancelSignal differs")
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
	if err := engineUtil.StrListCmp(obj.WatchFiles, res.WatchFiles); err != nil {
		return errwrap.Wrapf(err, "the WatchFiles differ")
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
	if err := engineUtil.StrPtrCmp(obj.IfEquals, res.IfEquals); err != nil {
		return errwrap.Wrapf(err, "the IfEquals differs")
	}
	if err := engineUtil.StrPtrCmp(obj.IfEqualsStdout, res.IfEqualsStdout); err != nil {
		return errwrap.Wrapf(err, "the IfEqualsStdout differs")
	}

	if obj.NIfCmd != res.NIfCmd {
		return fmt.Errorf("the NIfCmd differs")
	}
	if obj.NIfCwd != res.NIfCwd {
		return fmt.Errorf("the NIfCwd differs")
	}
	if obj.NIfShell != res.NIfShell {
		return fmt.Errorf("the NIfShell differs")
	}

	if obj.Creates != res.Creates {
		return fmt.Errorf("the Creates differs")
	}
	if err := engineUtil.StrListCmp(obj.Mtimes, res.Mtimes); err != nil {
		return errwrap.Wrapf(err, "the Mtimes differ")
	}

	if obj.DoneCmd != res.DoneCmd {
		return fmt.Errorf("the DoneCmd differs")
	}
	if obj.DoneCwd != res.DoneCwd {
		return fmt.Errorf("the DoneCwd differs")
	}
	if obj.DoneShell != res.DoneShell {
		return fmt.Errorf("the DoneShell differs")
	}

	if obj.User != res.User {
		return fmt.Errorf("the User differs")
	}
	if obj.Group != res.Group {
		return fmt.Errorf("the Group differs")
	}

	if err := engineUtil.StrPtrCmp(obj.SendOutput, res.SendOutput); err != nil {
		return errwrap.Wrapf(err, "the SendOutput differs")
	}
	if err := engineUtil.StrPtrCmp(obj.SendStdout, res.SendStdout); err != nil {
		return errwrap.Wrapf(err, "the SendStdout differs")
	}
	if err := engineUtil.StrPtrCmp(obj.SendStderr, res.SendStderr); err != nil {
		return errwrap.Wrapf(err, "the SendStderr differs")
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
	Cmd      string
	WatchCmd string
	IfCmd    string
	NIfCmd   string
	DoneCmd  string
	// TODO: add more elements here
}

// ExecResAutoEdges holds the state of the auto edge generator.
type ExecResAutoEdges struct {
	edges   []engine.ResUID
	pointer int
}

// Next returns the next automatic edge.
func (obj *ExecResAutoEdges) Next() []engine.ResUID {
	if len(obj.edges) == 0 {
		return nil
	}
	value := obj.edges[obj.pointer]
	obj.pointer++
	return []engine.ResUID{value}
}

// Test gets results of the earlier Next() call, & returns if we should
// continue!
func (obj *ExecResAutoEdges) Test(input []bool) bool {
	if len(obj.edges) <= obj.pointer {
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		panic("Expecting a single value!")
	}
	return true // keep going
}

// AutoEdges returns the AutoEdge interface. In this case the systemd units.
func (obj *ExecRes) AutoEdges(ctx context.Context) (engine.AutoEdge, error) {
	var data []engine.ResUID
	var reversed = true

	for _, x := range obj.cmdFiles() {
		data = append(data, &PkgFileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: x, // what matters
		})
		data = append(data, &FileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: x,
		})
	}
	if obj.User != "" {
		data = append(data, &UserUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			name: obj.User,
		})
	}
	if obj.Group != "" {
		data = append(data, &GroupUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			name: obj.Group,
		})
	}

	return &ExecResAutoEdges{
		edges:   data,
		pointer: 0,
	}, nil
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *ExecRes) UIDs() []engine.ResUID {
	x := &ExecUID{
		BaseUID:  engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		Cmd:      obj.getCmd(),
		WatchCmd: obj.WatchCmd,
		IfCmd:    obj.IfCmd,
		NIfCmd:   obj.NIfCmd,
		DoneCmd:  obj.DoneCmd,
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

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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
	if obj.User == "" && obj.Group == "" {
		return nil, nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error looking up current user")
	}

	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error casting current UID to int")
	}
	gid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error casting current GID to int")
	}

	wantedUID := uid
	if obj.User != "" {
		wantedUID, err = engineUtil.GetUID(obj.User)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up uid for %s", obj.User)
		}
	}

	wantedGID := gid
	if obj.Group != "" {
		wantedGID, err = engineUtil.GetGID(obj.Group)
		if err != nil {
			return nil, errwrap.Wrapf(err, "error looking up gid for %s", obj.Group)
		}
	}

	// We are already are what we want, so no need to build the credentials.
	if wantedUID == uid && wantedGID == gid {
		return nil, nil
	}

	if uid != 0 { // XXX: add `&& gid != 0` or not?
		// Since we're not root, we've got to error, but this should be
		// caught in Validate first anyways.
		return nil, fmt.Errorf("running as root is required if you want to use exec with a different user/group")
	}

	//nolint:gosec // G115: uid/gid values resolved from the system are non-negative
	return &syscall.Credential{Uid: uint32(wantedUID), Gid: uint32(wantedGID)}, nil
}

// cmdFiles returns all the potential files/commands this command might need.
func (obj *ExecRes) cmdFiles() []string {
	var paths []string
	if obj.Shell != "" {
		paths = append(paths, obj.Shell)
	} else if sp := strings.Fields(obj.getCmd()); len(sp) > 0 {
		paths = append(paths, sp[0])
	}
	if obj.WatchShell != "" {
		paths = append(paths, obj.WatchShell)
	} else if sp := strings.Fields(obj.WatchCmd); len(sp) > 0 {
		paths = append(paths, sp[0])
	}
	if obj.IfShell != "" {
		paths = append(paths, obj.IfShell)
	} else if sp := strings.Fields(obj.IfCmd); len(sp) > 0 {
		paths = append(paths, sp[0])
	}
	if obj.NIfShell != "" {
		paths = append(paths, obj.NIfShell)
	} else if sp := strings.Fields(obj.NIfCmd); len(sp) > 0 {
		paths = append(paths, sp[0])
	}
	if obj.DoneShell != "" {
		paths = append(paths, obj.DoneShell)
	} else if sp := strings.Fields(obj.DoneCmd); len(sp) > 0 {
		paths = append(paths, sp[0])
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
// errors. It runs Start and Wait, and errors runtime things in the channel. If
// it can't start up the command, it will fail early. Once it's running, it will
// return the channel which can be used for the duration of the process.
// Cancelling the context merely unblocks the sending on the output channel, it
// does not Kill the cmd process. For that you must do it yourself elsewhere. It
// always reaps the process before the wg is done, so as to never leave a zombie
// behind, but keep in mind that reaping blocks until the process exits, so
// whoever kills it must not wait on the wg before doing the killing.
func (obj *ExecRes) cmdOutputRunner(ctx context.Context, cmd *execCmd) (chan *cmdOutput, error) {
	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating StdoutPipe for Cmd")
	}
	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating StderrPipe for Cmd")
	}
	// XXX: Can io.MultiReader when one of these is still open? Is there an
	// issue or race here about calling cmd.Wait() if only one of them dies?
	cmdReader := io.MultiReader(stdoutReader, stderrReader)
	scanner := bufio.NewScanner(cmdReader)
	if err := cmd.start(); err != nil {
		return nil, errwrap.Wrapf(err, "error starting Cmd")
	}

	ch := make(chan *cmdOutput)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch)
		waited := false
		defer func() {
			if waited {
				return
			}
			// always reap so we don't leave a zombie
			_ = cmd.wait()
		}()
		for scanner.Scan() {
			select {
			case ch <- &cmdOutput{text: scanner.Text()}: // blocks here ?
			case <-ctx.Done():
				return
			}
		}

		// on EOF, scanner.Err() will be nil
		reterr := scanner.Err()
		waited = true
		reterr = errwrap.Append(reterr, cmd.wait()) // always run Wait()
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

// execCmd is one command that this resource runs, and everything we need to
// signal it and to collect its output. Every command goes through here, so that
// they all behave the same way when we're asked to stop.
type execCmd struct {
	// Cmd is the underlying command, and it's built by Init. We embed it so
	// that the caller can set whatever we don't set for it, such as the dir
	// and the credential, exactly as it would on a bare command.
	*exec.Cmd

	// Name is what we call this command in log and error messages, eg the
	// "ifcmd" in "ifcmd exited with: 1".
	Name string

	// Command is the command to run.
	Command string

	// Shell is the shell to run Command with if we want one.
	Shell string

	// Args are the args for the command, and they may only be used when
	// there's no shell.
	Args []string

	// Env is the environment to run the command with.
	Env map[string]string

	// CancelSignal is the signal we send when the context is cancelled.
	CancelSignal syscall.Signal

	// Interrupt is closed when we must end right now. It's the last thing
	// that can happen before mgmt itself gets killed, so we kill the
	// command with a signal it can't catch, and it may well be left in a
	// partial state.
	Interrupt chan struct{}

	// Logf is used for logging.
	Logf func(format string, v ...interface{})

	// ctx is the context which cancels this command. We keep it so that we
	// can tell a command which we stopped from one which stopped on its
	// own.
	ctx context.Context

	// out is where we collect the output, if we were asked to collect it.
	out *splitWriter

	// pipes are the pipes which feed the above.
	pipes []*execPipe

	// mutex guards the three fields below it.
	mutex *sync.Mutex

	// started is true once the process exists, and finished is true once it
	// has been reaped. We may only signal it in between those two.
	started  bool
	finished bool

	// interrupted is true if we interrupted this command, and cancelled is
	// true if we signalled it because our context was cancelled. Either one
	// means that we're the reason it ended, which is the only thing that
	// tells us apart from a command which finished on its own.
	interrupted bool
	cancelled   bool

	// done is closed once the command has been reaped, and wg waits for the
	// goroutine which watches for an interrupt until then.
	done chan struct{}
	wg   *sync.WaitGroup
}

// Init builds the command from the params: the shell or the bare argv split,
// the sorted environment, its own process group, and the cancel behaviour.
// Nothing is started here, so the command must either be run or thrown away.
func (obj *execCmd) Init(ctx context.Context) error {
	var cmdName string
	var cmdArgs []string
	if obj.Shell == "" {
		// call without a shell
		// FIXME: are there still whitespace splitting issues?
		// TODO: we could make the split character user selectable...!
		split := strings.Fields(obj.Command)
		if len(split) == 0 { // avoid a panic on an all whitespace cmd
			return fmt.Errorf("the %s is empty", obj.Name)
		}
		cmdName = split[0]
		//d, _ := os.Getwd() // TODO: how does this ever error ?
		//cmdName = path.Join(d, cmdName)
		cmdArgs = split[1:]
		if len(obj.Args) > 0 {
			if len(split) != 1 { // should not happen
				return fmt.Errorf("validation error")
			}
			cmdArgs = obj.Args
		}
	} else {
		cmdName = obj.Shell // usually bash, or sh
		cmdArgs = []string{"-c", obj.Command}
	}

	obj.ctx = ctx
	obj.mutex = &sync.Mutex{}
	obj.done = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	obj.Cmd = exec.CommandContext(ctx, cmdName, cmdArgs...)

	envKeys := []string{}
	for key := range obj.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	cmdEnv := []string{}
	for _, k := range envKeys {
		cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
	}
	obj.Cmd.Env = cmdEnv

	// ignore signals sent to parent process (we're in our own group)
	obj.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// This runs when the context is cancelled. Returning nil from here is
	// how we tell os/exec that we did interrupt the command, which makes
	// Wait report the context error even if the command went on to exit
	// with a zero status.
	obj.Cmd.Cancel = func() error {
		err := obj.signal(obj.CancelSignal)
		if err != nil { // it was already gone, or we couldn't signal it
			return err
		}

		// We really did stop this one. Wait blocks until this has run,
		// so anyone who looks at this after it has returned sees this.
		obj.mutex.Lock()
		obj.cancelled = true
		obj.mutex.Unlock()

		return nil
	}

	return nil
}

// String returns the command and its args, for logging.
func (obj *execCmd) String() string {
	return strings.Join(obj.Cmd.Args, " ")
}

// capture collects the stdout and stderr of this command, so that we can look
// at them once it has finished. It must be called before the command is run.
//
// We make the pipes ourselves instead of handing os/exec an io.Writer and
// letting it do it, because when os/exec owns the pipes it makes Wait block
// until every write end has been closed. That includes the ones inherited by
// any descendant of our command which outlived it, so a command which
// backgrounds something and exits cleanly would block us forever. Since we have
// no timer which could break out of that, we do the copying ourselves, and we
// stop as soon as the command has been reaped.
func (obj *execCmd) capture() error {
	out := &splitWriter{}
	out.Init()

	// Since we hand os/exec two files, it hands them straight to the child
	// and does no copying of its own, which means none of what it documents
	// about the goroutines it would otherwise run applies to us. The two
	// which do the copying are ours, and what keeps the combined output
	// from interleaving badly is that the writers they feed share a lock.
	stdout, err := newExecPipe(out.Stdout)
	if err != nil {
		return err
	}
	stderr, err := newExecPipe(out.Stderr)
	if err != nil {
		stdout.cleanup()
		return err
	}

	obj.Cmd.Stdout = stdout.write
	obj.Cmd.Stderr = stderr.write
	obj.out = out
	obj.pipes = []*execPipe{stdout, stderr}

	return nil
}

// output returns the collected output. It's only valid after the command ran,
// and only if we asked to capture it.
func (obj *execCmd) output() *splitWriter {
	return obj.out
}

// run starts the command and waits for it to finish.
func (obj *execCmd) run() error {
	if err := obj.start(); err != nil {
		return err
	}
	return obj.wait()
}

// start starts the command. It refuses to start anything once we've been
// interrupted, since at that point we're on our way out and a new process is
// the last thing that anyone wants.
func (obj *execCmd) start() error {
	select {
	case <-obj.Interrupt:
		// We're the reason this never ran, so record it the same way we
		// would if we'd had to signal it and let stopped() do the rest.
		obj.mutex.Lock()
		obj.interrupted = true
		obj.mutex.Unlock()
		obj.cleanup()
		return fmt.Errorf("the resource was interrupted")
	default:
	}

	// We hold the lock across the start, because os/exec watches the
	// context from a goroutine which it launches in here, and that
	// goroutine could otherwise run our cancel before we've recorded that
	// there's something to signal, which would swallow the signal entirely.
	obj.mutex.Lock()
	err := obj.Cmd.Start()
	obj.started = err == nil
	obj.mutex.Unlock()
	if err != nil {
		obj.cleanup()
		return err
	}

	// An interrupt which arrived while we were starting is caught here too,
	// since the channel stays closed once it has been closed.
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		select {
		case <-obj.Interrupt:
			if err := obj.interrupt(); err != nil {
				obj.Logf("%s: error interrupting: %s", obj.Name, err)
			}
		case <-obj.done: // it finished on its own
		}
	}()

	// The child has its own copies of the write ends now, and ours have to
	// go, or the copying would never see the EOF which ends it.
	for _, pipe := range obj.pipes {
		pipe.closeWrite()
	}

	return nil
}

// wait waits for the command to finish, and it collects whatever output it
// produced. After this returns, the command is reaped and we must never signal
// it again.
func (obj *execCmd) wait() error {
	err := obj.Cmd.Wait()

	// The pid may be recycled from here on, so shut the door on signalling
	// before we do anything else.
	obj.mutex.Lock()
	obj.finished = true
	obj.mutex.Unlock()

	close(obj.done) // the interrupt watcher can stop now
	obj.wg.Wait()

	for _, pipe := range obj.pipes {
		if e := pipe.close(); e != nil {
			obj.Logf("%s: error closing pipe: %s", obj.Name, e)
		}
	}

	return err // this is what the caller has to classify
}

// signal sends a signal to the whole process group of this command, and not
// just to the command itself, because we start every command in its own group
// and a shell would otherwise leave its children running when it dies. It does
// nothing if the command hasn't started, or if it has already been reaped.
//
// This has to use the raw negative pid, because there is no pidfd for a process
// group: pidfd_send_signal addresses exactly one process, and the os.Process
// methods which do use a pidfd under the hood can't reach any of the
// descendants that we're aiming at here.
//
// TODO: A reaped pid can be recycled, so we could signal a stranger. The
// finished flag isn't enough, since os/exec reaps before it sets that.
func (obj *execCmd) signal(signal syscall.Signal) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if !obj.started || obj.finished {
		return os.ErrProcessDone // tells os/exec to ignore this
	}

	// The negative pid signals the whole process group instead.
	if err := syscall.Kill(-obj.Cmd.Process.Pid, signal); err != nil {
		if err == syscall.ESRCH { // it's already dead
			return os.ErrProcessDone // tells os/exec to ignore this
		}
		return err
	}

	return nil
}

// interrupt asks this command to end right now. It's the last thing we can do
// before mgmt itself gets killed, so the signal it sends is one that can't be
// caught or ignored, and the command may well be left in a partial state.
func (obj *execCmd) interrupt() error {
	err := obj.signal(execCmdInterruptSignal)
	if err == os.ErrProcessDone { // it finished on its own, nothing to do
		return nil
	}
	if err != nil {
		return err
	}

	// We really did kill this one, and we only record it now that we know
	// that, since a command which had already finished did all of its work
	// and must not be reported as one which we cut short.
	obj.mutex.Lock()
	obj.interrupted = true
	obj.mutex.Unlock()

	return nil
}

// stopped returns an error if this command didn't get to finish on its own,
// because we cancelled it, or interrupted it, or never let it start. It's
// important to check this before looking at how a command exited, since a
// command which handles our signal can exit with a zero status, and that must
// not be mistaken for the command having done what it was asked to do.
//
// We go by whether we actually signalled it, and not by whether our context is
// done, because a context which was cancelled once the command had already
// finished says nothing about that command: it did the work, and all the
// cancellation means is that we're on our way out now. Asking the context would
// throw away a completed run every time the two raced, and for the main command
// that would mean skipping the donecmd and the mtime for work that was done.
func (obj *execCmd) stopped() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.interrupted {
		return fmt.Errorf("%s was interrupted", obj.Name)
	}

	if obj.cancelled {
		if err := obj.ctx.Err(); err != nil {
			return err // the engine knows what this means
		}
		// Nothing but a done context can get us here, but we must never
		// report a command that we stopped as one which had its say.
		return fmt.Errorf("%s was cancelled", obj.Name)
	}

	if !obj.started {
		// It never ran, so whether that's a problem is the context's to
		// say. This is nil when we simply weren't asked to stop, which
		// leaves the real reason it wouldn't start to the caller.
		return obj.ctx.Err()
	}

	return nil
}

// exitStatus digs the exit status out of the error that running this command
// returned, so that the caller can tell an ordinary non-zero exit, which is an
// answer that some of our commands are allowed to give us, from the ways of
// dying which aren't an answer at all.
//
// A command which was killed by a signal has no exit status to speak of, and
// the -1 that we'd get for one must never reach a caller which is looking at
// this to decide something, since it would read exactly like an ordinary
// non-zero exit. It wasn't us who signalled it either, because that's checked
// before we get here, so it was something else in the system, such as the OOM
// killer or a segfault of its own making. We're only ever called after
// something went wrong, so a zero status would mean we misread things.
func (obj *execCmd) exitStatus(err error) (int, error) {
	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if !ok {
		// command failed in some bad way
		return 0, errwrap.Wrapf(err, "%s failed in some bad way", obj.Name)
	}
	pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
	wStatus, ok := pStateSys.(syscall.WaitStatus)
	if !ok {
		return 0, errwrap.Wrapf(err, "could not get exit status of %s", obj.Name)
	}

	//nolint:misspell // golang stdlib name (Signaled)
	if wStatus.Signaled() {
		return 0, errwrap.Wrapf(err, "%s was killed by signal: %s", obj.Name, wStatus.Signal())
	}

	exitStatus := wStatus.ExitStatus()
	if exitStatus == 0 {
		// i'm not sure if this could happen
		return 0, errwrap.Wrapf(err, "unexpected %s exit status of zero", obj.Name)
	}
	return exitStatus, nil
}

// cleanup throws away anything we built but never got to use. It must only be
// called if the command didn't start.
func (obj *execCmd) cleanup() {
	for _, pipe := range obj.pipes {
		pipe.cleanup()
	}
	obj.pipes = nil
}

// execPipe is one pipe which carries the output of a command back to us. We
// read from one end while the command writes into the other, because a command
// which produces more output than a pipe can hold would otherwise block forever
// waiting for someone to read it.
type execPipe struct {
	// read is our end of the pipe, and write is the end that we hand to the
	// command before closing our own copy of it.
	read  *os.File
	write *os.File

	// done is closed once the copying has finished, either because we hit
	// the EOF, or because we closed our end.
	done chan struct{}
}

// newExecPipe builds a pipe which copies everything it receives into w.
func newExecPipe(w io.Writer) (*execPipe, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating pipe")
	}

	obj := &execPipe{
		read:  read,
		write: write,
		done:  make(chan struct{}),
	}

	go func() {
		defer close(obj.done)
		_, _ = io.Copy(w, obj.read) // ends on EOF or on our close
	}()

	return obj, nil
}

// closeWrite closes our copy of the end that we gave to the command. Until this
// happens we're a writer on our own pipe, and the copying could never end.
func (obj *execPipe) closeWrite() {
	_ = obj.write.Close() // there's nothing useful we could do with this
}

// close finishes with this pipe, and it must only be called once the command
// has been reaped.
func (obj *execPipe) close() error {
	// If every write end of this pipe is closed, then the copying is
	// guaranteed to drain whatever is left and then stop on its own, so we
	// wait for it and we get all of the output. If some descendant of the
	// command outlived it and is still holding the write end, then waiting
	// would never end, and we take what we got instead. That case is
	// already a lost cause: the output we're missing belongs to a process
	// which escaped both the command and the process group we would have
	// killed it with.
	if hup, err := pipeHUP(obj.read); err == nil && hup {
		<-obj.done
	}

	err := obj.read.Close() // this unblocks the copying if it's still going
	<-obj.done              // now it's certainly over

	return err
}

// cleanup closes both ends of a pipe which never got used.
func (obj *execPipe) cleanup() {
	obj.closeWrite()
	_ = obj.read.Close()
	<-obj.done
}

// pipeHUP returns whether every write end of this pipe has been closed. When
// that's true, a read can't block on a writer which might never come back, so
// whatever is left to read is all that there will ever be.
func pipeHUP(file *os.File) (bool, error) {
	conn, err := file.SyscallConn()
	if err != nil {
		return false, err
	}

	var hup bool
	var reterr error
	if err := conn.Control(func(fd uintptr) {
		//nolint:gosec // G115: a file descriptor always fits in an int32
		fds := []unix.PollFd{{Fd: int32(fd)}}
		for {
			// POLLHUP is reported even though we ask for nothing.
			if _, err := unix.Poll(fds, 0); err != nil {
				if err == unix.EINTR {
					continue
				}
				reterr = err
				return
			}
			break
		}
		hup = fds[0].Revents&unix.POLLHUP != 0
	}); err != nil {
		return false, err
	}

	return hup, reterr
}

// isNameValid checks that environment variable name is valid.
func isNameValid(varName string) error {
	if varName == "" {
		return fmt.Errorf("variable name cannot be an empty string")
	}
	for i := range varName {
		c := varName[i]
		if i == 0 && '0' <= c && c <= '9' {
			return fmt.Errorf("variable name cannot begin with number")
		}
		if !(c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') {
			return fmt.Errorf("invalid character in variable name")
		}
	}
	return nil
}
