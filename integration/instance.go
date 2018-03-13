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

package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/recwatch"

	errwrap "github.com/pkg/errors"
)

const (
	// RootDirectory is the directory that is exposed in the per instance
	// directory which can be used by that instance safely.
	RootDirectory = "root"

	// PrefixDirectory is the directory that is exposed in the per instance
	// directory which is used for the mgmt prefix.
	PrefixDirectory = "prefix"

	// ConvergedStatusFile is the name of the file which is used for the
	// converged status tracking.
	ConvergedStatusFile = "csf.txt"

	// StdoutStderrFile is the name of the file which is used for the
	// command output.
	StdoutStderrFile = "stdoutstderr.txt"

	// longTimeout is a high bound of time we're willing to wait for events.
	// If we exceed this timeout, then it's likely we are blocked somewhere.
	longTimeout = 60 // seconds

	// convergedTimeout is the number of seconds we wait for our instance to
	// remain unchanged to be considered as converged.
	convergedTimeout = 15 // seconds

	// dirMode is the the mode used when making directories.
	dirMode = 0755

	// fileMode is the the mode used when making files.
	fileMode = 0644
)

// Instance represents a single running mgmt instance. It is a building block
// that can be used to run standalone tests, or combined to run clustered tests.
type Instance struct {
	// Hostname is a unique identifier for this instance.
	Hostname string

	// Preserve prevents the runtime output from being explicitly deleted.
	// This is helpful for running analysis or tests on the output.
	Preserve bool

	// Debug enables more verbosity.
	Debug bool

	// dir is the directory where all files will be written under.
	dir string

	tmpPrefixDirectory   string
	testRootDirectory    string
	convergedStatusFile  string
	convergedStatusIndex int

	cmd *exec.Cmd

	clientURL string // set when launched with run
	serverURL string
}

// Init runs some initialization for this instance. It errors if the struct was
// populated in an invalid way, or if it can't initialize correctly.
func (obj *Instance) Init() error {
	if obj.Hostname == "" {
		return fmt.Errorf("must specify a hostname")
	}

	// create temporary directory to use during testing
	if obj.dir == "" {
		var err error
		obj.dir, err = ioutil.TempDir("", fmt.Sprintf("mgmt-integration-%s-", obj.Hostname))
		if err != nil {
			return errwrap.Wrapf(err, "can't create temporary directory")
		}
	}

	tmpPrefix := path.Join(obj.dir, PrefixDirectory)
	if err := os.MkdirAll(tmpPrefix, dirMode); err != nil {
		return errwrap.Wrapf(err, "can't create prefix directory")
	}
	obj.tmpPrefixDirectory = tmpPrefix

	testRootDirectory := path.Join(obj.dir, RootDirectory)
	if err := os.MkdirAll(testRootDirectory, dirMode); err != nil {
		return errwrap.Wrapf(err, "can't create instance root directory")
	}
	obj.testRootDirectory = testRootDirectory

	obj.convergedStatusFile = path.Join(obj.dir, ConvergedStatusFile)

	return nil
}

// Close cleans up after we're done with this instance.
func (obj *Instance) Close() error {
	if !obj.Preserve {
		if obj.dir == "" || obj.dir == "/" {
			panic("obj.dir is set to a dangerous path")
		}
		if err := os.RemoveAll(obj.dir); err != nil { // dangerous ;)
			return errwrap.Wrapf(err, "can't remove instance dir")
		}
	}
	obj.Kill() // safety
	return nil
}

// Run launches the instance. It returns an error if it was unable to launch.
func (obj *Instance) Run(seeds []*Instance) error {
	if obj.cmd != nil {
		return fmt.Errorf("an instance is already running")
	}

	if len(seeds) == 0 {
		// set so that Deploy can know where to connect
		// also set so that we can allow others to find us and connect
		obj.clientURL = "http://127.0.0.1:2379"
		obj.serverURL = "http://127.0.0.1:2380"
	} else {
		// pick next available pair of ports
		var maxClientPort, maxServerPort int
		for _, instance := range seeds {
			clientPort, err := ParsePort(instance.clientURL)
			if err != nil {
				return errwrap.Wrapf(err, "could not parse client URL from `%s`", instance.Hostname)
			}
			if clientPort > maxClientPort {
				maxClientPort = clientPort
			}

			serverPort, err := ParsePort(instance.serverURL)
			if err != nil {
				return errwrap.Wrapf(err, "could not parse server URL from `%s`", instance.Hostname)
			}
			if serverPort > maxServerPort {
				maxServerPort = serverPort
			}
		}
		if maxClientPort+2 == maxServerPort || maxClientPort == maxServerPort+2 {
			return fmt.Errorf("port conflict found")
		}

		obj.clientURL = fmt.Sprintf("http://127.0.0.1:%d", maxClientPort+2) // odd
		obj.serverURL = fmt.Sprintf("http://127.0.0.1:%d", maxServerPort+2) // even
	}

	cmdName, err := BinaryPath()
	if err != nil {
		return err
	}
	cmdArgs := []string{
		"run", // mode
		fmt.Sprintf("--hostname=%s", obj.Hostname),
		fmt.Sprintf("--client-urls=%s", obj.clientURL),
		fmt.Sprintf("--server-urls=%s", obj.serverURL),
		fmt.Sprintf("--prefix=%s", obj.tmpPrefixDirectory),
		fmt.Sprintf("--converged-timeout=%d", convergedTimeout),
		"--converged-timeout-no-exit",
		fmt.Sprintf("--converged-status-file=%s", obj.convergedStatusFile),
	}
	if len(seeds) > 0 {
		urls := []string{}
		for _, instance := range seeds {
			if instance.cmd == nil {
				return fmt.Errorf("instance `%s` has not started yet", instance.Hostname)
			}
			urls = append(urls, instance.clientURL)
		}
		s := fmt.Sprintf("--seeds=%s", urls[0])
		// TODO: we could just add all the seeds instead...
		//s := fmt.Sprintf("--seeds=%s", strings.Join(urls, ","))
		cmdArgs = append(cmdArgs, s)
	}
	obj.cmd = exec.Command(cmdName, cmdArgs...)
	obj.cmd.Env = []string{
		fmt.Sprintf("MGMT_TEST_ROOT=%s", obj.testRootDirectory),
	}

	// output file for storing logs
	file, err := os.Create(path.Join(obj.dir, StdoutStderrFile))
	if err != nil {
		return errwrap.Wrapf(err, "error creating log file")
	}
	defer file.Close()
	obj.cmd.Stdout = file
	obj.cmd.Stderr = file

	if err := obj.cmd.Start(); err != nil {
		return errwrap.Wrapf(err, "error starting mgmt")
	}

	return nil
}

// Kill the process immediately. This is a `kill -9` for if things get stuck.
func (obj *Instance) Kill() error {
	if obj.cmd == nil {
		return nil // already dead
	}
	if obj.cmd.Process == nil {
		return nil // nothing running
	}

	// cause a stack dump first if we can
	_ = obj.cmd.Process.Signal(syscall.SIGQUIT)

	return obj.cmd.Process.Kill()
}

// Quit sends a friendly shutdown request to the process. You can specify a
// context if you'd like to exit earlier. If you trigger an early exit with the
// context, then this will end up running a `kill -9` so it can return.
func (obj *Instance) Quit(ctx context.Context) error {
	if obj.cmd == nil {
		return fmt.Errorf("no process is running")
	}
	if err := obj.cmd.Process.Signal(os.Interrupt); err != nil {
		return errwrap.Wrapf(err, "could not send interrupt signal")
	}

	var err error
	wg := &sync.WaitGroup{}
	done := make(chan error)
	wg.Add(1)
	go func() {
		defer wg.Done()
		done <- obj.cmd.Wait()
		close(done)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case err = <-done:
		case <-ctx.Done():
			obj.Kill() // should cause the Wait() to exit
			err = ctx.Err()
		}
	}()

	wg.Wait()
	obj.cmd = nil
	return err
}

// Wait until the first converged state we hit. It is not necessary to use the
// `--converged-timeout` option with mgmt for this to work. It tracks this via
// the `--converged-status-file` option which can be used to track the varying
// convergence status.
func (obj *Instance) Wait(ctx context.Context) error {
	//if obj.cmd == nil { // TODO: should we include this?
	//	return fmt.Errorf("no process is running")
	//}

	recurse := false
	recWatcher, err := recwatch.NewRecWatcher(obj.convergedStatusFile, recurse)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch file")
	}
	defer recWatcher.Close()
	startup := make(chan struct{})
	close(startup)
	for {
		select {
		// FIXME: instead of sending one event here, the recwatch
		// library should send one initial event at startup...
		case <-startup:
			startup = nil
			// send an initial event

		case event, ok := <-recWatcher.Events():
			if !ok {
				return fmt.Errorf("file watcher shut down")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "error event received")
			}

			// send event...

		case <-ctx.Done():
			return ctx.Err()
		}

		contents, err := ioutil.ReadFile(obj.convergedStatusFile)
		if err != nil {
			continue // file might not exist yet, wait for an event
		}
		raw := strings.Split(string(contents), "\n")
		lines := []string{}
		for _, x := range raw {
			if x == "" { // drop blank lines!
				continue
			}
			lines = append(lines, x)
		}

		if c := len(lines); c < obj.convergedStatusIndex {
			return fmt.Errorf("file is missing lines or was truncated, got: %d", c)
		}

		var converged bool
		for i := obj.convergedStatusIndex; i < len(lines); i++ {
			obj.convergedStatusIndex = i + 1 // new max
			line := lines[i]
			if line == "true" { // converged!
				converged = true
			}
		}
		if converged {
			return nil
		}
	}
}

// DeployLang deploys some code to the cluster.
func (obj *Instance) DeployLang(code string) error {
	//if obj.cmd == nil { // TODO: should we include this?
	//	return fmt.Errorf("no process is running")
	//}

	filename := path.Join(obj.dir, "deploy.mcl")
	data := []byte(code)
	if err := ioutil.WriteFile(filename, data, fileMode); err != nil {
		return err
	}

	cmdName, err := BinaryPath()
	if err != nil {
		return err
	}
	cmdArgs := []string{
		"deploy", // mode
		"--no-git",
		"--seeds", obj.clientURL,
		"lang", "--lang", filename,
	}
	cmd := exec.Command(cmdName, cmdArgs...)
	if err := cmd.Run(); err != nil {
		return errwrap.Wrapf(err, "can't run deploy")
	}
	return nil
}

// Dir returns the dir where the instance can write to. You should only use this
// after Init has been called, or it won't have been created and determined yet.
func (obj *Instance) Dir() string {
	return obj.dir
}

// CombinedOutput returns the logged output from the instance.
func (obj *Instance) CombinedOutput() (string, error) {
	contents, err := ioutil.ReadFile(path.Join(obj.dir, StdoutStderrFile))
	if err != nil {
		return "", err
	}
	return string(contents), nil
}
