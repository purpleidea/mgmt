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
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	archUtil "github.com/purpleidea/mgmt/util/arch"
	distroUtil "github.com/purpleidea/mgmt/util/distro"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("virt:builder", func() engine.Res { return &VirtBuilderRes{} })

	if !strings.HasPrefix(VirtBuilderBinDir, "/") {
		panic("the VirtBuilderBinDir does not start with a slash")
	}
	if !strings.HasSuffix(VirtBuilderBinDir, "/") {
		panic("the VirtBuilderBinDir does not end with a slash")
	}
	virtBuilderMutex = &sync.Mutex{}
}

var (
	// virtBuilderMutex is a mutex for virt:builder res global operations.
	virtBuilderMutex *sync.Mutex
)

const (
	// VirtBuilderCmdPath is the path to the virt-builder binary. For now,
	// this is the same on any Linux machine, so it's a constant.
	VirtBuilderCmdPath = "/usr/bin/virt-builder"

	// VirtBuilderBinDir is the directory to copy our binary to in the vm.
	VirtBuilderBinDir = "/usr/local/bin/"
)

// VirtBuilderRes is a resource for building virtual machine images. It is based
// on the amazing virt-builder tool which is part of the guestfs suite of tools.
// TODO: Add autoedges with the virt resource disk path!
type VirtBuilderRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Output is the full absolute file path where the image will be
	// created. If this file exists, then no action will be performed.
	// TODO: Consider adding a "overwrite" type mechanism in the future,
	// when we can find a safe way to do so.
	Output string `lang:"output" yaml:"output"`

	// OSVersion specifies which distro and version to use for installation.
	// You will need to pick from the output of `virt-builder --list`.
	OSVersion string `lang:"os_version" yaml:"os_version"`

	// Arch specifies the CPU architecture to use for this machine. You will
	// need to pick from the output of `virt-builder --list`. Note that not
	// all OSVersion+Arch combinations may exist.
	Arch string `lang:"arch" yaml:"arch"`

	// Hostname for the new machine.
	Hostname string `lang:"hostname" yaml:"hostname"`

	// Format is the disk image format. You likely want "raw" or "qcow2".
	Format string `lang:"format" yaml:"format"`

	// Size is the disk size of the new virtual machine in bytes.
	Size int `lang:"size" yaml:"size"`

	// Packages is the list of packages to install. If Bootstrap is true,
	// then it will add additional packages that we install if needed.
	Packages []string `lang:"packages" yaml:"packages"`

	// Update specifies that we should update the installed packages during
	// image build. This defaults to true.
	Update bool `lang:"update" yaml:"update"`

	// SelinuxRelabel specifies that we should do an selinux relabel on the
	// final image. This defaults to true.
	SelinuxRelabel bool `lang:"selinux_relabel" yaml:"selinux_relabel"`

	// NoSetup can be set to true to disable trying to install the package
	// for the virt-builder binary.
	NoSetup bool `lang:"no_setup" yaml:"no_setup"`

	// SSHKeys is a list of additional keys to add to the machine. This is
	// not a map because you may wish to add more than one to that user
	// account.
	SSHKeys []*SSHKeyInfo `lang:"ssh_keys" yaml:"ssh_keys"`

	// RootSSHInject disables installing the root ssh key into the new vm.
	// If one is not present, then nothing is done.	This defaults to true.
	RootSSHInject bool `lang:"root_ssh_inject" yaml:"root_ssh_inject"`

	// RootPasswordSelector is a string in the virt-builder format. See the
	// manual page "USERS AND PASSWORDS" section for more information.
	RootPasswordSelector string `lang:"root_password_selector" yaml:"root_password_selector"`

	// Bootstrap can be set to false to disable any automatic bootstrapping
	// of running the mgmt binary on first boot. If this is set, we will
	// attempt to copy the mgmt binary in, and then run it. This also adds
	// additional packages to install which are needed to bootstrap mgmt.
	// This defaults to true.
	// TODO: This does not yet support multi or cross arch.
	Bootstrap bool `lang:"bootstrap" yaml:"bootstrap"`

	// Seeds is a list of default etcd client endpoints to connect to. If
	// you specify this, you must also set Bootstrap to true. These should
	// likely be http URL's like: http://127.0.0.1:2379 or similar.
	Seeds []string `lang:"seeds" yaml:"seeds"`

	// Mkdir creates these directories in the guests. This happens before
	// CopyIn runs. Directories must be absolute and end with a slash. Any
	// intermediate directories are created, similar to how `mkdir -p`
	// works.
	Mkdir []string `lang:"mkdir" yaml:"mkdir"`

	// CopyIn is a list of local paths to copy into the machine dest. The
	// dest directory must exist for this to work. Use Mkdir if you need to
	// make a directory, since that step happens earlier. All paths must be
	// absolute, and directories must end with a slash. This happens before
	// the RunCmd stage in case you want to create something to be used
	// there.
	CopyIn []*CopyIn `lang:"copy_in" yaml:"copy_in"`

	// RunCmd is a sequence of commands + args (one set per list item) to
	// run in the build environment. These happen after the CopyIn stage.
	RunCmd []string `lang:"run_cmd" yaml:"run_cmd"`

	// FirstbootCmd is a sequence of commands + args (one set per list item)
	// to run once on first boot.
	// TODO: Consider replacing this with the mgmt firstboot mechanism for
	// consistency between this platform and other platforms that might not
	// support the excellent libguestfs version of those scripts. (Make the
	// logs look more homogeneous.)
	FirstbootCmd []string `lang:"firstboot_cmd" yaml:"firstboot_cmd"`

	// LogOutput logs the output of running this command to a file in the
	// special $vardir directory. It defaults to true. Keep in mind that if
	// you let virt-builder choose the password randomly, it will be output
	// in these logs in cleartext!
	LogOutput bool `lang:"log_output" yaml:"log_output"`

	// Tweaks adds some random tweaks to work around common bugs. This
	// defaults to true.
	Tweaks bool `lang:"tweaks" yaml:"tweaks"`

	varDir string
}

// getOutput returns the output filename of the image that we plan to build. If
// the Output field is set, we use that, otherwise we use the Name.
func (obj *VirtBuilderRes) getOutput() string {
	if obj.Output != "" {
		return obj.Output
	}
	return obj.Name()
}

// getDistro returns the distro of the guest that we want to use. If not
// specified, or if we don't have a mapping for it, we return the empty string.
func (obj *VirtBuilderRes) getDistro() string {
	ix := strings.Index(obj.OSVersion, "-") // fedora- or debian-
	if ix == -1 {
		return "" // os version is not supported
	}

	return obj.OSVersion[0:ix] // everything before the dash, eg: fedora
}

// getArch returns the architecture that we want to use. If not specified, or if
// we don't have a mapping for it, we return the empty string.
func (obj *VirtBuilderRes) getArch() string {
	if obj.Arch != "" {
		return obj.Arch
	}

	defaultArch, exists := archUtil.GoArchToVirtBuilderArch(runtime.GOARCH)
	if !exists {
		return ""
	}

	return defaultArch
}

// getBinaryPath returns the path to the binary of this program.
func (obj *VirtBuilderRes) getBinaryPath() (string, error) {
	p1, err := os.Executable()
	if err != nil {
		return "", err
	}

	p2, err := filepath.EvalSymlinks(p1)
	if err != nil {
		return "", err
	}

	p3, err := filepath.Abs(p2)
	if err != nil {
		return "", err
	}
	return p3, nil
}

// getGuestfs returns the package to install for our os so that we can run
// virt-builder.
func (obj *VirtBuilderRes) getGuestfs() ([]string, error) {
	// TODO: Improve this function as things evolve.
	distro, err := distroUtil.Distro(context.TODO()) // what is this resource running in?
	if err != nil {
		return nil, nil
	}

	packages, exists := distroUtil.ToGuestfsPackages(distro)
	if !exists {
		// TODO: patches welcome!
		return nil, fmt.Errorf("os/version is not supported")
	}

	return packages, nil
}

// getDeps returns a list of packages to install for the specific os-version so
// that we can easily run mgmt.
func (obj *VirtBuilderRes) getDeps() ([]string, error) {
	// TODO: Improve this function as things evolve.
	distro := obj.getDistro()
	if distro == "" {
		return nil, fmt.Errorf("os version is not supported")
	}
	packages, exists := distroUtil.ToBootstrapPackages(distro)
	if !exists {
		// TODO: patches welcome!
		return nil, fmt.Errorf("os version is not supported")
	}

	return packages, nil
}

// Default returns some sensible defaults for this resource.
func (obj *VirtBuilderRes) Default() engine.Res {
	return &VirtBuilderRes{
		Update:               true,
		SelinuxRelabel:       true,
		RootSSHInject:        true,
		RootPasswordSelector: "disabled",
		Bootstrap:            true,
		LogOutput:            true,
		Tweaks:               true,
	}
}

// Validate reports any problems with the struct definition.
func (obj *VirtBuilderRes) Validate() error {
	if !strings.HasPrefix(obj.getOutput(), "/") {
		return fmt.Errorf("output must be absolute and start with slash")
	}

	if obj.OSVersion == "" {
		return fmt.Errorf("must specify OSVersion")
	}
	// TODO: Check if OSVersion+Arch is in list of virt-builder --list --list-format json
	// TODO: Make this check inexpensive by caching the result in $vardir?

	if obj.Hostname != "" {
		// TODO: Validate hostname chars
	}

	if obj.Format != "" {
		if obj.Format != "raw" && obj.Format != "qcow2" {
			return fmt.Errorf("format must be 'raw' or 'qcow2'")
		}
	}

	if obj.Size < 0 {
		return fmt.Errorf("size must be positive")
	}

	for _, x := range obj.Packages {
		if x == "" {
			return fmt.Errorf("a package cannot be the empty string")
		}
		if strings.Contains(x, ",") {
			return fmt.Errorf("a package cannot contain a comma")
		}
	}

	for _, x := range obj.SSHKeys {
		if err := x.Validate(); err != nil {
			return err
		}
	}

	for _, x := range obj.Seeds {
		if x == "" {
			return fmt.Errorf("empty seed")
		}
		if _, err := url.Parse(x); err != nil { // it's so rare this fails
			return err
		}
	}

	for _, x := range obj.Mkdir {
		if x == "" {
			return fmt.Errorf("empty Mkdir entry")
		}
		if !strings.HasPrefix(x, "/") {
			return fmt.Errorf("the Mkdir entry must be absolute")
		}
		if !strings.HasSuffix(x, "/") {
			return fmt.Errorf("the Mkdir entry must be a directory")
		}
	}
	for _, x := range obj.CopyIn {
		if err := x.Validate(); err != nil {
			return err
		}
	}
	for _, x := range obj.RunCmd {
		if x == "" {
			return fmt.Errorf("empty RunCmd entry")
		}
	}
	for _, x := range obj.FirstbootCmd {
		if x == "" {
			return fmt.Errorf("empty FirstbootCmd entry")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *VirtBuilderRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	if obj.LogOutput {
		varDir, err := obj.init.VarDir("")
		if err != nil {
			return errwrap.Wrapf(err, "could not get VarDir in Init()")
		}
		obj.varDir = varDir
	}

	_, err := os.Stat(VirtBuilderCmdPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		return nil
	}

	if obj.NoSetup { // done early
		return nil
	}

	// Try to get the packages for the binary...
	p, err := obj.getGuestfs()
	if err != nil {
		return err
	}

	virtBuilderMutex.Lock()
	defer virtBuilderMutex.Unlock()

	// Try to install the binary...
	for _, x := range p {
		obj.init.Logf("installing: %s", x)
		if err := InstallOnePackage(context.TODO(), x); err != nil {
			return err
		}
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *VirtBuilderRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// one watches the on disk filename if it creates one, as well as the runtime
// value the kernel has stored!
func (obj *VirtBuilderRes) Watch(ctx context.Context) error {
	recurse := false // single file
	recWatcher, err := recwatch.NewRecWatcher(obj.getOutput(), recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return err
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *VirtBuilderRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	if _, err := os.Stat(obj.getOutput()); err != nil && !os.IsNotExist(err) {
		return false, err // probably some permissions error

	} else if err == nil {
		return true, nil // if file exists, we're done (for safety!)
	}

	// File does not exist, therefore we can continue!

	if !apply {
		return false, nil
	}

	cmdName := VirtBuilderCmdPath
	cmdArgs := []string{obj.OSVersion, "--output", obj.getOutput()}

	if arch := obj.getArch(); arch != "" {
		args := []string{"--arch", arch}
		cmdArgs = append(cmdArgs, args...)
	}

	if obj.Hostname != "" {
		args := []string{"--hostname", obj.Hostname}
		cmdArgs = append(cmdArgs, args...)
	}

	if obj.Format != "" {
		args := []string{"--format", obj.Format}
		cmdArgs = append(cmdArgs, args...)
	}

	if obj.Size > 0 {
		// size in bytes if it ends with a `b`
		args := []string{"--size", strconv.Itoa(obj.Size) + "b"}
		cmdArgs = append(cmdArgs, args...)
	}

	extraPackages := []string{}
	if obj.Bootstrap {
		p, err := obj.getDeps()
		if err != nil {
			return false, err
		}
		extraPackages = append(extraPackages, p...)
	}

	if len(obj.Packages) > 0 || len(extraPackages) > 0 {
		packages := []string{} // I think the ordering _may_ matter.
		packages = append(packages, obj.Packages...)
		packages = append(packages, extraPackages...)
		args := []string{"--install", strings.Join(packages, ",")}
		cmdArgs = append(cmdArgs, args...)
	}

	// XXX: Tweak for debian grub-pc bug:
	// https://www.mail-archive.com/guestfs@lists.libguestfs.org/msg00062.html
	if obj.Tweaks && obj.Update && obj.getDistro() == "debian" {
		args := []string{"--run-command", "apt-mark hold grub-pc"}
		cmdArgs = append(cmdArgs, args...)
	}
	if obj.Update {
		arg := "--update"
		cmdArgs = append(cmdArgs, arg)
	}
	if obj.Tweaks && obj.Update && obj.getDistro() == "debian" {
		args := []string{"--firstboot-command", "apt-mark unhold grub-pc"}
		cmdArgs = append(cmdArgs, args...)
	}

	if obj.SelinuxRelabel {
		arg := "--selinux-relabel"
		cmdArgs = append(cmdArgs, arg)
	}

	for _, x := range obj.SSHKeys {
		args := []string{"--ssh-inject", x.SSHInjectLine()}
		cmdArgs = append(cmdArgs, args...)
	}

	if obj.RootSSHInject {
		args := []string{"--ssh-inject", "root"}
		cmdArgs = append(cmdArgs, args...)
	}

	// TODO: consider changing this behaviour to get password from send/recv
	if obj.RootPasswordSelector != "" {
		passwordArgs := []string{"--root-password", obj.RootPasswordSelector}
		cmdArgs = append(cmdArgs, passwordArgs...)
	}

	if obj.Bootstrap {
		p, err := obj.getBinaryPath()
		if err != nil {
			return false, err
		}
		args1 := []string{"--mkdir", VirtBuilderBinDir, "--copy-in", p + ":" + VirtBuilderBinDir} // LOCALPATH:REMOTEDIR
		cmdArgs = append(cmdArgs, args1...)

		// TODO: bootstrap mgmt based on the deploy method this ran with
		// TODO: --tmp-prefix ? --module-path ?
		// TODO: add an alternate handoff method to run a bolus of code?
		if len(obj.Seeds) > 0 {
			m := filepath.Join(VirtBuilderBinDir, filepath.Base(p)) // mgmt full path
			setupSvc := []string{
				m,       // mgmt
				"setup", // setup command
				"svc",   // TODO: pull from a const?
				"--install",
				//"--start", // we're in pre-boot env right now
				"--enable", // start on first boot!
				fmt.Sprintf("--binary-path=%s", m),
				"--no-server", // TODO: hardcode this for now
				//fmt.Sprintf("--seeds=%s", strings.Join(obj.Seeds, ",")),
			}
			for _, seed := range obj.Seeds {
				// TODO: validate each seed?
				s := fmt.Sprintf("--seeds=%s", seed)
				setupSvc = append(setupSvc, s)
			}

			setupSvcCmd := strings.Join(setupSvc, " ")
			args := []string{"--run-command", setupSvcCmd} // cmd must be a single string
			cmdArgs = append(cmdArgs, args...)
		}
	}

	for _, x := range obj.Mkdir {
		args := []string{"--mkdir", x}
		cmdArgs = append(cmdArgs, args...)
	}
	for _, x := range obj.CopyIn {
		args := []string{"--copy-in", x.Path + ":" + x.Dest} // LOCALPATH:REMOTEDIR
		cmdArgs = append(cmdArgs, args...)
	}
	for _, x := range obj.RunCmd {
		args := []string{"--run-command", x}
		cmdArgs = append(cmdArgs, args...)
	}
	for _, x := range obj.FirstbootCmd {
		args := []string{"--firstboot-command", x}
		cmdArgs = append(cmdArgs, args...)
	}

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)

	// ignore signals sent to parent process (we're in our own group)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// Capture stdout and stderr together. Same as CombinedOutput() method.
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b

	obj.init.Logf("running: %s", strings.Join(cmd.Args, " "))
	start := time.Now().UnixNano()
	if err := cmd.Start(); err != nil {
		return false, errwrap.Wrapf(err, "error starting cmd")
	}

	err := cmd.Wait() // we can unblock this with the timeout
	out := b.String()

	p := path.Join(obj.varDir, fmt.Sprintf("%d.log", start))
	if obj.LogOutput {
		if err := os.WriteFile(p, b.Bytes(), 0600); err != nil {
			obj.init.Logf("unable to store log: %v", err)
		}
	}

	if err == nil {
		obj.init.Logf("built image successfully!")
		return false, nil // success!
	}

	// Delete partial/invalid/corrupt image.
	if err := os.Remove(obj.getOutput()); err != nil && !os.IsNotExist(err) {
		// Can't delete the file :/
		// XXX: This permanently breaks our resource since subsequent
		// runs will see it as in a valid state. Maybe we should add a
		// $vardir file telling future CheckApply runs that we need to
		// do a delete first? But if we can't write that file things are
		// bad anyways.
		obj.init.Logf("permanent error, can't delete partial file: %s", obj.getOutput())
		return false, errwrap.Wrapf(err, "permanent error, can't delete partial file: %s", obj.getOutput())
	} else if err == nil {
		obj.init.Logf("deleted partial output")
	}

	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if !ok {
		// command failed in some bad way
		return false, errwrap.Wrapf(err, "cmd failed in some bad way")
	}
	pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
	wStatus, ok := pStateSys.(syscall.WaitStatus)
	if !ok {
		return false, errwrap.Wrapf(err, "could not get exit status of cmd")
	}
	exitStatus := wStatus.ExitStatus()
	if exitStatus == 0 {
		// i'm not sure if this could happen
		return false, errwrap.Wrapf(err, "unexpected cmd exit status of zero")
	}

	obj.init.Logf("cmd: %s", strings.Join(cmd.Args, " "))
	if out == "" {
		obj.init.Logf("cmd exit status %d", exitStatus)
	} else {
		obj.init.Logf("cmd error:\n%s", out) // newline because it's long
	}
	return false, errwrap.Wrapf(err, "cmd error") // exit status will be in the error
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *VirtBuilderRes) Cmp(r engine.Res) error {
	// we can only compare VirtBuilderRes to others of the same resource kind
	res, ok := r.(*VirtBuilderRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Output != res.Output {
		return fmt.Errorf("the Output differs")
	}
	if obj.OSVersion != res.OSVersion {
		return fmt.Errorf("the OSVersion value differs")
	}
	if obj.Arch != res.Arch {
		return fmt.Errorf("the Arch value differs")
	}
	if obj.Hostname != res.Hostname {
		return fmt.Errorf("the Hostname value differs")
	}
	if obj.Format != res.Format {
		return fmt.Errorf("the Format value differs")
	}
	if obj.Size != res.Size {
		return fmt.Errorf("the Size value differs")
	}

	if len(obj.Packages) != len(res.Packages) {
		return fmt.Errorf("the number of Packages differs")
	}
	for i, x := range obj.Packages {
		if pkg := res.Packages[i]; x != pkg {
			return fmt.Errorf("the package at index %d differs", i)
		}
	}

	if obj.Update != res.Update {
		return fmt.Errorf("the Update value differs")
	}
	if obj.SelinuxRelabel != res.SelinuxRelabel {
		return fmt.Errorf("the SelinuxRelabel value differs")
	}

	if obj.NoSetup != res.NoSetup {
		return fmt.Errorf("the NoSetup value differs")
	}

	if len(obj.SSHKeys) != len(res.SSHKeys) {
		return fmt.Errorf("the number of SSHKeys differs")
	}
	for i, x := range obj.SSHKeys {
		if err := res.SSHKeys[i].Cmp(x); err != nil {
			return errwrap.Wrapf(err, "the ssh key at index %d differs", i)
		}
	}

	if obj.RootSSHInject != res.RootSSHInject {
		return fmt.Errorf("the RootSSHInject value differs")
	}
	if obj.RootPasswordSelector != res.RootPasswordSelector {
		return fmt.Errorf("the RootPasswordSelector value differs")
	}
	if obj.Bootstrap != res.Bootstrap {
		return fmt.Errorf("the Bootstrap value differs")
	}

	if len(obj.Seeds) != len(res.Seeds) {
		return fmt.Errorf("the number of Seeds differs")
	}
	for i, x := range obj.Seeds {
		if seed := res.Seeds[i]; x != seed {
			return fmt.Errorf("the seed at index %d differs", i)
		}
	}

	if len(obj.Mkdir) != len(res.Mkdir) {
		return fmt.Errorf("the number of Mkdir entries differs")
	}
	for i, x := range obj.Mkdir {
		if s := res.Mkdir[i]; x != s {
			return fmt.Errorf("the Mkdir entry at index %d differs", i)
		}
	}
	if len(obj.CopyIn) != len(res.CopyIn) {
		return fmt.Errorf("the number of CopyIn structs differ")
	}
	for i, x := range obj.CopyIn {
		if err := res.CopyIn[i].Cmp(x); err != nil {
			return errwrap.Wrapf(err, "the copy in struct at index %d differs", i)
		}
	}
	if len(obj.RunCmd) != len(res.RunCmd) {
		return fmt.Errorf("the number of RunCmd entries differs")
	}
	for i, x := range obj.RunCmd {
		if s := res.RunCmd[i]; x != s {
			return fmt.Errorf("the RunCmd entry at index %d differs", i)
		}
	}
	if len(obj.FirstbootCmd) != len(res.FirstbootCmd) {
		return fmt.Errorf("the number of FirstbootCmd entries differs")
	}
	for i, x := range obj.FirstbootCmd {
		if s := res.FirstbootCmd[i]; x != s {
			return fmt.Errorf("the FirstbootCmd entry at index %d differs", i)
		}
	}

	if obj.LogOutput != res.LogOutput {
		return fmt.Errorf("the LogOutput value differs")
	}
	if obj.Tweaks != res.Tweaks {
		return fmt.Errorf("the Tweaks value differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *VirtBuilderRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes VirtBuilderRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*VirtBuilderRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to VirtBuilderRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = VirtBuilderRes(raw) // restore from indirection with type conversion!
	return nil
}

// SSHKeyInfo stores information about user SSH keys. It is used to add entries
// to the authorized_keys file in the target vm.
type SSHKeyInfo struct {
	// User is the user account we want to add this key to.
	User string `lang:"user" yaml:"user"`

	// Type of SSH key. Usually "ssh-rsa" for example.
	Type string `lang:"type" yaml:"type"`

	// Key is the base64 encoded key.
	Key string `lang:"key" yaml:"key"`

	// Comment is a short comment about this entry.
	Comment string `lang:"comment" yaml:"comment"`
}

// AuthorizedKeyLine returns the valid line for the authorized_keys entry. Make
// sure you've run Validate before using this.
func (obj *SSHKeyInfo) AuthorizedKeyLine() string {
	comment := obj.Comment
	if comment == "" {
		comment = "comment" // TODO: Put something useful like user@hostname?
	}
	return fmt.Sprintf("%s %s %s", obj.Type, obj.Key, obj.Comment)
}

// SSHInjectLine returns the valid arg for the --ssh-inject command. Make sure
// you've run Validate before using this.
func (obj *SSHKeyInfo) SSHInjectLine() string {
	user := obj.User
	if user == "" {
		user = "root" // default user
	}
	return fmt.Sprintf("%s:string:%s", user, obj.AuthorizedKeyLine()) // USER:string:KEY_STRING
}

// Validate reports any problems with the struct definition.
func (obj *SSHKeyInfo) Validate() error {
	if obj == nil {
		return fmt.Errorf("nil obj")
	}
	//if obj.User == "" { // root if not specified
	//	return fmt.Errorf("empty User")
	//}
	if obj.Type == "" {
		return fmt.Errorf("empty Type")
	}
	if obj.Key == "" {
		return fmt.Errorf("empty Key")
	}
	//if obj.Comment == "" { // generate one if not specified
	//	return fmt.Errorf("empty Comment")
	//}

	// TODO: Check key type is a valid algorithm?
	// TODO: Check key length and format is sane for key type?

	check := func(s string) error {
		fn := func(r rune) bool {
			return unicode.IsSpace(r)
		}
		if strings.ContainsFunc(s, fn) {
			return fmt.Errorf("white space chars found")
		}
		return nil
	}

	if err := check(obj.User); err != nil {
		return errwrap.Wrapf(err, "problem with User")
	}
	if err := check(obj.Type); err != nil {
		return errwrap.Wrapf(err, "problem with Type")
	}
	if err := check(obj.Key); err != nil {
		return errwrap.Wrapf(err, "problem with Key")
	}
	if err := check(obj.Comment); err != nil {
		return errwrap.Wrapf(err, "problem with Comment")
	}

	return nil
}

// Cmp compares two of these and returns an error if they are not equivalent.
func (obj *SSHKeyInfo) Cmp(x *SSHKeyInfo) error {
	//if (obj == nil) != (x == nil) { // xor
	//	return fmt.Errorf("we differ") // redundant
	//}
	if obj == nil || x == nil {
		// special case since we want to error if either is nil
		return fmt.Errorf("can't cmp if nil")
	}

	if obj.User != x.User {
		return fmt.Errorf("the User differs")
	}
	if obj.Type != x.Type {
		return fmt.Errorf("the Type differs")
	}
	if obj.Key != x.Key {
		return fmt.Errorf("the Key differs")
	}
	if obj.Comment != x.Comment {
		return fmt.Errorf("the Comment differs")
	}

	return nil
}

// CopyIn is a list of local paths to copy into the machine dest.
type CopyIn struct {
	// Path is the local file or directory that we want to copy in.
	// TODO: Add autoedges
	Path string `lang:"path" yaml:"path"`

	// Dest is the destination dir that the path gets copied into. This
	// directory must exist.
	Dest string `lang:"dest" yaml:"dest"`
}

// Validate reports any problems with the struct definition.
func (obj *CopyIn) Validate() error {
	if obj == nil {
		return fmt.Errorf("nil obj")
	}
	if obj.Path == "" {
		return fmt.Errorf("empty Path")
	}
	if !strings.HasPrefix(obj.Path, "/") {
		return fmt.Errorf("the Path must be absolute")
	}
	if obj.Dest == "" {
		return fmt.Errorf("empty Dest")
	}
	if !strings.HasPrefix(obj.Dest, "/") {
		return fmt.Errorf("the Dest must be absolute")
	}
	if !strings.HasSuffix(obj.Dest, "/") {
		return fmt.Errorf("the dest must be a directory")
	}

	return nil
}

// Cmp compares two of these and returns an error if they are not equivalent.
func (obj *CopyIn) Cmp(x *CopyIn) error {
	//if (obj == nil) != (x == nil) { // xor
	//	return fmt.Errorf("we differ") // redundant
	//}
	if obj == nil || x == nil {
		// special case since we want to error if either is nil
		return fmt.Errorf("can't cmp if nil")
	}

	if obj.Path != x.Path {
		return fmt.Errorf("the Path differs")
	}
	if obj.Dest != x.Dest {
		return fmt.Errorf("the Dest differs")
	}

	return nil
}
