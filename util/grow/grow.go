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

// Package grow is a utility for growing storage.
package grow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/util"
)

const (
	// DevDir is where linux system devices are found.
	DevDir = "/dev/"
)

// Grow is a utility that grows the underlying partition, luks device (if
// encrypted) and then finally the partition. It makes many assumptions about
// the luks device being encrypted with an empty password, and that nobody uses
// lvm anymore. This utility is useful when provisioning new machines which
// don't get their maximum disk utilization by default. (All the Fedora machines
// whether physical or virtual seem to have this problem.)
//
// This whole utility should only be run by the Run entrypoint if you want to
// receive the benefit of the various options, such as Noop and Done. If you
// bypass those then you're on your own.
//
// Patches are welcome to expand the functionality of this software, however it
// was intended as the minimally viable piece of reliable code to solve the
// specific grow problem commonly seen, without adding unused features.
type Grow struct {
	// Noop is set to true to prevent any actual grow operation.
	Noop bool

	// Done specifies a file path to create once the operations complete
	// successfully. If this file exists, these operations won't run.
	Done string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Run the whole sequence. This is probably the only function you want to run.
func (obj *Grow) Run(ctx context.Context, mount string) (reterr error) {
	if obj.Done != "" {
		if !strings.HasPrefix(obj.Done, "/") {
			return fmt.Errorf("done must be absolute")
		}
		if strings.HasSuffix(obj.Done, "/") {
			return fmt.Errorf("done must be a file")
		}

		_, err := os.Stat(obj.Done)
		if err != nil && !os.IsNotExist(err) {
			return err // permission err
		}
		if err == nil { // exists!
			obj.Logf("done mode")
			return nil
		}
		defer func() {
			// Store error in case we have a problem here.
			reterr = os.WriteFile(obj.Done, []byte(fmt.Sprintf("%s\n", mount)), 0600)
		}()
	}

	fsList, err := obj.RunFindMnt(ctx, mount)
	if err != nil {
		return err
	}

	if len(fsList) == 0 {
		return fmt.Errorf("no mount found")
	}
	if len(fsList) != 1 {
		return fmt.Errorf("ambiguous mount found, expected one")
	}

	if target := fsList[0].Target; target != mount {
		return fmt.Errorf("unexpected target: %s", target)
	}

	fsType := fsList[0].FsType

	if err := obj.IsValidFsType(fsType); err != nil {
		return err
	}

	// If we're using LUKS, this will be something like:
	// /dev/mapper/luks-<UUID> but keep in mind that `cryptsetup isLuks` on
	// that device will be FALSE because that's the unencrypted
	// device-mapper device, not the actual encrypted block device.
	source := fsList[0].Source
	if source == "" {
		return fmt.Errorf("empty source")
	}
	if source == "/" {
		return fmt.Errorf("invalid source")
	}

	// Source is probably a device mapper device. Let's resolve it.
	// /usr/bin/realpath --relative-to /dev /dev/mapper/luks-<UUID>
	//
	// It might just be something like /dev/sda3 which means this is a noop.
	if !strings.HasPrefix(source, "/") {
		return fmt.Errorf("source is not absolute")
	}
	dev, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}

	// This should output something like /dev/dm-0 or maybe not if it's not
	// using LUKS.

	if !strings.HasPrefix(dev, DevDir) {
		return fmt.Errorf("unexpected resolved link of: %s", dev)
	}

	// This is now dm-0 if we're using luks/device mapper or maybe sda3.

	luks := "" // assume false
	if err := obj.IsDeviceMapper(dev); err == nil {

		// XXX: Is there a better way to resolve device-mapper? I know I
		// can cryptsetup status directly on the /dev/mapper/luks-<UUID>
		// but I'd like a more flexible approach where I do it stepwise.
		dm := strings.TrimPrefix(dev, DevDir)
		s := fmt.Sprintf("/sys/block/%s/slaves/", dm)
		dirs, err := os.ReadDir(s) // ([]DirEntry, error)
		if err != nil {
			return err
		}
		if l := len(dirs); l != 1 {
			return fmt.Errorf("unexpected number of device-mapper slaves: %d", l)
		}

		//luks = source // this would work
		luks = dev // this feels more accurate

		// This is the underlying device. You could also get this from
		// cryptsetup status $dev or $source but since the cryptsetup
		// command seems to be a bit sloppy in what it expects, we just
		// look up the device mapper stuff in the kernel /sys/ dirs...
		dev = DevDir + dirs[0].Name() // eg: /dev/ + nvme0n1p3
	}

	if obj.Debug {
		obj.Logf("source: %s", source)
		obj.Logf("luks: %s", luks)
		obj.Logf("dev: %s", dev)
	}

	if obj.Noop {
		obj.Logf("noop mode")
	}

	if err := obj.GrowPart(ctx, dev); err != nil {
		return err
	}

	if luks != "" {
		if err := obj.GrowLUKS(ctx, luks); err != nil {
			return err
		}
	}

	if err := obj.GrowFs(ctx, source, fsType); err != nil {
		return err
	}

	return nil
}

// GrowPart runs the "growpart" command from the "cloud-utils-growpart" package.
func (obj *Grow) GrowPart(ctx context.Context, part string) error {
	if part == "" {
		return fmt.Errorf("empty part")
	}

	base, part, err := obj.GrowPartArgs(part)
	if err != nil {
		return err
	}

	if obj.Debug {
		obj.Logf("base: %s", base)
		obj.Logf("part: %s", part)
	}

	cmd, err := exec.LookPath("growpart")
	if err != nil {
		return err
	}
	cmdArgs := []string{base, part}
	obj.Logf("cmd: %s %s", cmd, strings.Join(cmdArgs, " "))
	if obj.Noop {
		return nil
	}
	if err := util.SimpleCmd(ctx, cmd, cmdArgs, obj.cmdOpts()); err != nil {
		return err
	}

	// It can take a moment for the kernel to see the change. We can run
	// the partprobe command to hurry it up.
	cmd, err = exec.LookPath("partprobe")
	if err != nil {
		return err
	}
	cmdArgs = []string{base}
	obj.Logf("cmd: %s %s", cmd, strings.Join(cmdArgs, " "))
	return util.SimpleCmd(ctx, cmd, cmdArgs, obj.cmdOpts())
}

// GrowLUKS grows the luks device that has an empty password.
func (obj *Grow) GrowLUKS(ctx context.Context, luks string) error {
	if luks == "" {
		return fmt.Errorf("empty luks")
	}

	// XXX: this hack is necessary because cryptsetup is dumb
	empty := "/tmp/empty"
	if !obj.Noop {
		if err := os.WriteFile(empty, []byte{}, 0600); err != nil {
			return err
		}
	}

	cmd, err := exec.LookPath("cryptsetup")
	if err != nil {
		return err
	}
	cmdArgs := []string{"resize", fmt.Sprintf("--key-file=%s", empty), luks}
	obj.Logf("cmd: %s %s", cmd, strings.Join(cmdArgs, " "))
	if obj.Noop {
		return nil
	}
	return util.SimpleCmd(ctx, cmd, cmdArgs, obj.cmdOpts())
}

// GrowFs expands the filesystem while it's online.
func (obj *Grow) GrowFs(ctx context.Context, dev, fsType string) error {
	if dev == "" {
		return fmt.Errorf("empty dev")
	}

	if fsType == "ext4" {
		cmd, err := exec.LookPath("resize2fs")
		if err != nil {
			return err
		}
		cmdArgs := []string{dev}
		obj.Logf("cmd: %s %s", cmd, strings.Join(cmdArgs, " "))
		if obj.Noop {
			return nil
		}
		return util.SimpleCmd(ctx, cmd, cmdArgs, obj.cmdOpts())
	}

	if fsType == "xfs" {
		cmd, err := exec.LookPath("xfs_growfs")
		if err != nil {
			return err
		}
		cmdArgs := []string{dev}
		obj.Logf("cmd: %s %s", cmd, strings.Join(cmdArgs, " "))
		if obj.Noop {
			return nil
		}
		return util.SimpleCmd(ctx, cmd, cmdArgs, obj.cmdOpts())
	}

	// XXX: btrfs filesystem resize max /path

	return obj.IsValidFsType(fsType)
}

// GrowPartArgs get the special args needed for the `growpart` command.
func (obj *Grow) GrowPartArgs(dev string) (string, string, error) {
	if dev == "" {
		return "", "", fmt.Errorf("empty dev")
	}
	if !strings.HasPrefix(dev, DevDir) {
		return "", "", fmt.Errorf("missing prefix for dev in: %s", dev)
	}

	// Simple parsing for /dev/sda3 and friends... Walk backwards until we
	// encounter a non-number, and if it's a weird nvme thing, remove the p.
	i := len(dev) - 1
	for i >= 0 && dev[i] >= '0' && dev[i] <= '9' { // is a number
		i--
	}
	if i == len(dev)-1 {
		return "", "", fmt.Errorf("no partition number found")
	}
	base := dev[:i+1]
	part := dev[i+1:]

	// fancy parsing for /dev/nvme0n1p3 which has the "p".
	if strings.HasPrefix(dev, DevDir+"nvme") && dev[i] == 'p' {
		base = dev[:i]
	}
	if base == "" || part == "" {
		return "", "", fmt.Errorf("unexpected empty result")
	}

	return base, part, nil
}

// IsDeviceMapper runs a simple check to see if we're something like:
// `/dev/dm-0`.
func (obj *Grow) IsDeviceMapper(name string) error {
	if !strings.HasPrefix(name, "/dev/dm-") {
		return fmt.Errorf("missing device mapper prefix")
	}
	s := strings.TrimPrefix(name, "/dev/dm-")

	if _, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("missing device mapper number")
	}

	return nil
}

// IsValidFsType are the types of filesystems we currently know how to grow.
func (obj *Grow) IsValidFsType(fsType string) error {
	if fsType == "ext4" || fsType == "xfs" {
		return nil
	}
	// TODO: add btrfs support
	//if fsType == "btrfs" {
	//	return nil
	//}

	return fmt.Errorf("can't grow fstype: %s", fsType)
}

// RunFindMnt runs the linux util `findmnt` command.
func (obj *Grow) RunFindMnt(ctx context.Context, p string) ([]*MountInfo, error) {
	cmd, err := exec.LookPath("findmnt")
	if err != nil {
		return nil, err
	}
	cmdArgs := []string{"--json", "--all", "--mountpoint", p}
	b, err := util.SimpleCmdOut(ctx, cmd, cmdArgs, obj.cmdOpts())
	if err != nil {
		return nil, err
	}

	var st FindMnt
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}

	return st.Filesystems, nil
}

// cmdOpts is just a helper to return the same struct repeatedly.
func (obj *Grow) cmdOpts() *util.SimpleCmdOpts {
	logf := func(format string, v ...interface{}) {
		if !obj.Debug { // ignore the noisy "always on" log messages...
			return
		}
		obj.Logf(format, v...)
	}
	return &util.SimpleCmdOpts{
		Debug: obj.Debug,
		Logf:  logf,
	}
}

// FindMnt is the --json output of the `findmnt` command.
type FindMnt struct {
	Filesystems []*MountInfo `json:"filesystems"`
}

// MountInfo is the type of each entry in the FindMnt Filesystems field.
type MountInfo struct {
	Target  string `json:"target"`
	Source  string `json:"source"`
	FsType  string `json:"fstype"`
	Options string `json:"options"`
}
