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

package firstboot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util"

	"gopkg.in/yaml.v2"
)

const (
	// LockFilePath is where we store a lock file to prevent more than one
	// copy of this service running at the same time.
	// TODO: Is there a better place to put this?
	LockFilePath string = "/var/lib/mgmt/firstboot.lock"

	// ScriptsDir is the directory where firstboot scripts can be found. You
	// can put binaries in here too. Contents must be executable.
	ScriptsDir string = "/var/lib/mgmt/firstboot/"

	// StyleSuffix is what we append to executables to specify a style file.
	StyleSuffix = ".yaml"
)

// Start is the standalone entry program for the firstboot start component.
type Start struct {
	*cliUtil.FirstbootStartArgs // embedded config
	Config                      // embedded Config

	// Program is the name of this program, usually set at compile time.
	Program string

	// Version is the version of this program, usually set at compile time.
	Version string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Main runs everything for this setup item.
func (obj *Start) Main(ctx context.Context) error {
	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Run(ctx); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Start) Validate() error {
	if obj == nil {
		return fmt.Errorf("data is nil")
	}
	if obj.Program == "" {
		return fmt.Errorf("program is empty")
	}
	if obj.Version == "" {
		return fmt.Errorf("version is empty")
	}

	return nil
}

// Run performs the desired actions. This runs a list of scripts and then
// removes them. This is useful for a "firstboot" service. If there exists a
// file with the same name as an executable script or binary, but which has a
// .yaml extension, that will be parsed to look for command modifiers.
func (obj *Start) Run(ctx context.Context) error {
	lockFile := LockFilePath // default
	if s := obj.FirstbootStartArgs.LockFilePath; s != "" && !strings.HasSuffix(s, "/") {
		lockFile = s
	}

	// Ensure the directory exists.
	d := filepath.Dir(lockFile)
	if err := os.MkdirAll(d, 0750); err != nil {
		return fmt.Errorf("could not make lockfile dir at: %s", d)
	}

	// Make sure only one copy of this service is running at a time.
	unlock, err := util.NewFlock(lockFile).TryLock()
	if err != nil {
		return err // can't get lock
	}
	if unlock == nil {
		return fmt.Errorf("already running")
	}
	// now we're locked!
	defer unlock()

	scriptsDir := ScriptsDir // default
	if s := obj.FirstbootStartArgs.ScriptsDir; s != "" && strings.HasSuffix(s, "/") {
		scriptsDir = s
	}
	obj.Logf("scripts dir: %s", scriptsDir)

	// Loop through all the entries and execute what we can...
	entries, err := os.ReadDir(scriptsDir) // ([]os.DirEntry, error)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() { // skip dirs
			continue
		}
		if !entry.Type().IsRegular() { // skip weird things
			// TODO: We may wish to relax this constraint eventually.
			continue
		}

		// TODO: Why is entry.Type() always empty?
		//fmt.Printf("???: %+v\n", entry.Type())
		//if m := entry.Type(); m&0100 == 0 { // owner bit is not executable
		//	continue
		//}
		fi, err := entry.Info()
		if os.IsNotExist(err) {
			continue // we might have deleted a style file
		} else if err != nil {
			return err // TODO: continue instead?
		}
		if m := fi.Mode(); m&0100 == 0 { // owner bit is not executable
			continue
		}

		p := filepath.Clean(scriptsDir + entry.Name())
		obj.Logf("found: %s", p)

		styleFile := p + StyleSuffix // maybe it exists!

		style := &Style{ // set defaults here
			DoneDir: obj.FirstbootStartArgs.DoneDir,
		}

		// TODO: check style file is _not_ executable to avoid chains?
		data, err := os.ReadFile(styleFile)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err := yaml.Unmarshal(data, style); err != nil {
				return err
			}
			style.exists = true
		}

		// Found one!
		if err := obj.process(ctx, p, style); err != nil {

		}
	}

	return nil
}

// process runs the command, logs, and deletes the script if needed.
func (obj *Start) process(ctx context.Context, name string, style *Style) error {
	logOutput := ""
	if !style.NoLog {
		loggingDir := "/root/"
		if s := obj.FirstbootStartArgs.LoggingDir; s != "" && strings.HasSuffix(s, "/") {
			loggingDir = s
		}
		logOutput = fmt.Sprintf("%smgmt-firstboot-%v.log", loggingDir, time.Now().UnixNano())
	}
	if err := util.AppendFile(logOutput, []byte(name+"\n"), 0600); err != nil {
		obj.Logf("error: %v", err)
	}
	opts := &util.SimpleCmdOpts{
		Debug:     obj.Debug,
		Logf:      obj.Logf,
		LogOutput: logOutput,
	}
	args := []string{}
	err := util.SimpleCmd(ctx, name, args, opts)
	errStr := fmt.Sprintf("error: %v\n", err)
	if err := util.AppendFile(logOutput, []byte(errStr), 0600); err != nil {
		obj.Logf("error: %v", err)
	}

	if err != nil && style.KeepOnFail {
		return err
	}

	if style.DoneDir != "" && strings.HasSuffix(style.DoneDir, "/") {
		dest := func(p string) string { // dest path from input full path
			return style.DoneDir + filepath.Base(p)
		}
		if err := os.MkdirAll(style.DoneDir, 0750); err != nil { // convenience
			obj.Logf("error: %v", err)
			//return err // let the real final error be seen...
		}
		// Move files!
		if err := os.Rename(name, dest(name)); err != nil {
			obj.Logf("error: %v", err)
		} else {
			obj.Logf("moved: %s", name)
		}
		if style.exists {
			if err := os.Rename(name+StyleSuffix, dest(name+StyleSuffix)); err != nil {
				obj.Logf("error: %v", err)
			} else {
				obj.Logf("moved: %s", name+StyleSuffix)
			}
		}

		return err
	}

	// Remove files!
	if err := os.Remove(name); err != nil {
		obj.Logf("error: %v", err)
	} else {
		obj.Logf("removed: %s", name)
	}
	if style.exists {
		if err := os.Remove(name + StyleSuffix); err != nil {
			obj.Logf("error: %v", err)
		} else {
			obj.Logf("removed: %s", name+StyleSuffix)
		}
	}

	return err
}

// Style are some values that are used to specify how each command runs.
type Style struct {
	// TODO: Is this easy to implement?
	// DeleteFirst should be true to delete the script before it runs. This
	// is useful to prevent scenarios where a read-only filesystem would
	// cause the script to run again and again.
	//DeleteFirst bool `yaml:"delete-first"`

	// DoneDir specifies the dir to move files to instead of deleting them.
	DoneDir string `yaml:"done-dir"`

	// KeepOnFail should be true to preserve the script if it fails. This
	// means it will likely run again the next time the service runs.
	KeepOnFail bool `yaml:"keep-on-fail"`

	// NoLog should be true to skip logging the output of this command.
	NoLog bool `yaml:"no-log"`

	// exists specifies if the style file exists on disk. (And as a result,
	// should it be removed at the end?)
	exists bool
}
