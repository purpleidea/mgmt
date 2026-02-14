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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("exec", func() engine.Res { return &ExecRes{} })
}

// ExecRes is an exec resource for running commands.
//
// This resource attempts to minimise the effects of the execution environment,
// and, in particular, will start the new process with an empty environment (as
// would `execve` with an empty `envp` array). If you want the environment to
// inherit the mgmt process' environment, you can import it from "sys" and use
// it with `env => sys.env()` in your exec resource.
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
	// this, then you can't use the Args parameter. Note that unless you
	// use absolute paths, or set the PATH variable, the shell might not be
	// able to find the program you're trying to run.
	Shell string `lang:"shell" yaml:"shell"`

	// Timeout is the number of seconds to wait before sending a Kill to the
	// running command. If the Kill is received before the process exits,
	// then this be treated as an error.
	Timeout uint64 `lang:"timeout" yaml:"timeout"`

	// Env allows the user to specify environment variables for script
	// execution. These are taken using a map of format of VAR_KEY -> value.
	// Omitting this value or setting it to an empty array will cause the
	// program to be run with an empty environment. These values are used
	// for every command. If there's a legitimate need to have different
	// environments for each command, then we'll split that out eventually.
	Env map[string]string `lang:"env" yaml:"env"`

	// WatchCmd is the command to run to detect event changes. Each line of
	// output from this command is treated as an event.
	WatchCmd string `lang:"watchcmd" yaml:"watchcmd"`

	// WatchCwd is the Cwd for the WatchCmd. See the docs for Cwd.
	WatchCwd string `lang:"watchcwd" yaml:"watchcwd"`

	// WatchFiles is a list of files that will be kept track of.
	WatchFiles []string `lang:"watchfiles" yaml:"watchfiles"`

	// WatchShell is the Shell for the WatchCmd. See the docs for Shell.
	WatchShell string `lang:"watchshell" yaml:"watchshell"`

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

	// check that, if a user or a group is set, we're running as root
	if obj.User != "" || obj.Group != "" {
		currentUser, err := user.Current()
		if err != nil {
			return errwrap.Wrapf(err, "error looking up current user")
		}
		if currentUser.Uid != "0" {
			return fmt.Errorf("running as root is required if you want to use exec with a different user/group")
		}
	}

	// check that environment variables' format is valid
	for key := range obj.Env {
		if err := isNameValid(key); err != nil {
			return errwrap.Wrapf(err, "invalid variable name")
		}
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
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ioChan := make(chan *cmdOutput)
	filesChan := make(chan recwatch.Event)

	var watchCmd *exec.Cmd
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

		innerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(innerCtx, cmdName, cmdArgs...)
		cmd.Dir = obj.WatchCwd // run program in pwd if ""

		envKeys := []string{}
		for key := range obj.Env {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)
		cmdEnv := []string{}
		for _, k := range envKeys {
			cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
		}
		cmd.Env = cmdEnv

		// ignore signals sent to parent process (we're in our own group)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}
		watchCmd = cmd // store for errors

		// if we have a user and group, use them
		var err error
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

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var files recwatch.Event
				var ok bool
				var shutdown bool

				select {
				case files, ok = <-recWatcher.Events(): // receiving events
				case <-ctx.Done(): // unblock
					return
				}

				if !ok {
					err := fmt.Errorf("channel shutdown")
					files = recwatch.Event{Error: err}
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

	obj.init.Running() // when started, notify engine that we're running

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
					return errwrap.Wrapf(err, "watchcmd failed in some bad way")
				}
				pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
				wStatus, ok := pStateSys.(syscall.WaitStatus)
				if !ok {
					return errwrap.Wrapf(err, "could not get exit status of watchcmd")
				}
				exitStatus := wStatus.ExitStatus()
				if exitStatus == 0 {
					// i'm not sure if this could happen
					return errwrap.Wrapf(err, "unexpected watchcmd exit status of zero")
				}

				obj.init.Logf("watchcmd: %s", strings.Join(watchCmd.Args, " "))
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

		case files, ok := <-filesChan:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected recwatch shutdown")
			}
			if err := files.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
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
		cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
		cmd.Dir = obj.IfCwd // run program in pwd if ""

		envKeys := []string{}
		for key := range obj.Env {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)
		cmdEnv := []string{}
		for _, k := range envKeys {
			cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
		}
		cmd.Env = cmdEnv

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
				return false, errwrap.Wrapf(err, "ifcmd failed in some bad way")
			}
			pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
			wStatus, ok := pStateSys.(syscall.WaitStatus)
			if !ok {
				return false, errwrap.Wrapf(err, "could not get exit status of ifcmd")
			}
			exitStatus := wStatus.ExitStatus()
			if exitStatus == 0 {
				// i'm not sure if this could happen
				return false, errwrap.Wrapf(err, "unexpected ifcmd exit status of zero")
			}

			obj.init.Logf("ifcmd: %s", strings.Join(cmd.Args, " "))
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
		s := out.String()
		if s == "" {
			obj.init.Logf("ifcmd out empty!")
		} else {
			obj.init.Logf("ifcmd out:")
			obj.init.Logf("%s", s)
		}
		if obj.IfEquals != nil && *obj.IfEquals == s {
			obj.init.Logf("ifequals matched")
			return true, nil // don't run
		}
	}

	if obj.NIfCmd != "" && !forceRun { // opposite of the ifcmd check
		var cmdName string
		var cmdArgs []string
		if obj.NIfShell == "" {
			// call without a shell
			// FIXME: are there still whitespace splitting issues?
			split := strings.Fields(obj.NIfCmd)
			cmdName = split[0]
			//d, _ := os.Getwd() // TODO: how does this ever error ?
			//cmdName = path.Join(d, cmdName)
			cmdArgs = split[1:]
		} else {
			cmdName = obj.NIfShell // usually bash, or sh
			cmdArgs = []string{"-c", obj.NIfCmd}
		}
		cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
		cmd.Dir = obj.NIfCwd // run program in pwd if ""

		envKeys := []string{}
		for key := range obj.Env {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)
		cmdEnv := []string{}
		for _, k := range envKeys {
			cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
		}
		cmd.Env = cmdEnv

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

		err = cmd.Run()
		if err == nil {
			obj.init.Logf("nifcmd: %s", strings.Join(cmd.Args, " "))
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

		exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
		if !ok {
			// command failed in some bad way
			return false, errwrap.Wrapf(err, "nifcmd failed in some bad way")
		}
		pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
		wStatus, ok := pStateSys.(syscall.WaitStatus)
		if !ok {
			return false, errwrap.Wrapf(err, "could not get exit status of nifcmd")
		}
		exitStatus := wStatus.ExitStatus()
		if exitStatus == 0 {
			// i'm not sure if this could happen
			return false, errwrap.Wrapf(err, "unexpected nifcmd exit status of zero")
		}

		obj.init.Logf("nifcmd: %s", strings.Join(cmd.Args, " "))
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
	var innerCtx context.Context
	var cancel context.CancelFunc
	if obj.Timeout > 0 { // cmd.Process.Kill() is called on timeout
		innerCtx, cancel = context.WithTimeout(ctx, time.Duration(obj.Timeout)*time.Second)
	} else { // zero timeout means no timer
		innerCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	cmd := exec.CommandContext(innerCtx, cmdName, cmdArgs...)
	cmd.Dir = obj.Cwd // run program in pwd if ""

	envKeys := []string{}
	for key := range obj.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	cmdEnv := []string{}
	for _, k := range envKeys {
		cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
	}
	cmd.Env = cmdEnv

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

	obj.init.Logf("cmd: %s", strings.Join(cmd.Args, " "))
	if err := cmd.Start(); err != nil {
		return false, errwrap.Wrapf(err, "error starting cmd")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.interruptChan:
			cancel()
		case <-innerCtx.Done():
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
			// most commands error in this way
			if s := out.String(); s == "" {
				obj.init.Logf("exit status %d", exitStatus)
			} else {
				obj.init.Logf("cmd error: %s", s)
			}

			return false, errwrap.Wrapf(err, "cmd error") // exit status will be in the error
		}
		sig := wStatus.Signal()

		// we get this on timeout, because ctx calls cmd.Process.Kill()
		if sig == syscall.SIGKILL {
			return false, errwrap.Wrapf(err, "cmd timeout, exit status: %d", exitStatus)
		}

		return false, errwrap.Wrapf(err, "unknown cmd error, signal: %s, exit status: %d", sig, exitStatus)

	} else if err != nil {
		return false, errwrap.Wrapf(err, "general cmd error")
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
		var cmdName string
		var cmdArgs []string
		if obj.DoneShell == "" {
			// call without a shell
			// FIXME: are there still whitespace splitting issues?
			split := strings.Fields(obj.DoneCmd)
			cmdName = split[0]
			//d, _ := os.Getwd() // TODO: how does this ever error ?
			//cmdName = path.Join(d, cmdName)
			cmdArgs = split[1:]
		} else {
			cmdName = obj.DoneShell // usually bash, or sh
			cmdArgs = []string{"-c", obj.DoneCmd}
		}
		cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
		cmd.Dir = obj.DoneCwd // run program in pwd if ""

		envKeys := []string{}
		for key := range obj.Env {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)
		cmdEnv := []string{}
		for _, k := range envKeys {
			cmdEnv = append(cmdEnv, k+"="+obj.Env[k])
		}
		cmd.Env = cmdEnv

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
				return false, errwrap.Wrapf(err, "donecmd failed in some bad way")
			}
			pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
			wStatus, ok := pStateSys.(syscall.WaitStatus)
			if !ok {
				return false, errwrap.Wrapf(err, "could not get exit status of donecmd")
			}
			exitStatus := wStatus.ExitStatus()
			if exitStatus == 0 {
				// i'm not sure if this could happen
				return false, errwrap.Wrapf(err, "unexpected donecmd exit status of zero")
			}

			obj.init.Logf("donecmd: %s", strings.Join(cmd.Args, " "))
			if s := out.String(); s == "" {
				obj.init.Logf("donecmd exit status %d", exitStatus)
			} else {
				obj.init.Logf("donecmd error: %s", s)
			}
			return false, errwrap.Wrapf(err, "cmd error") // exit status will be in the error
		}
		if s := out.String(); s == "" {
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
func (obj *ExecRes) AutoEdges() (engine.AutoEdge, error) {
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
// does not Kill the cmd process. For that you must do it yourself elsewhere.
func (obj *ExecRes) cmdOutputRunner(ctx context.Context, cmd *exec.Cmd) (chan *cmdOutput, error) {
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
		reterr = errwrap.Append(reterr, cmd.Wait()) // always run Wait()
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
