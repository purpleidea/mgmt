// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	sdbus "github.com/coreos/go-systemd/dbus"
	"github.com/coreos/go-systemd/unit"
	systemdUtil "github.com/coreos/go-systemd/util"
	fstab "github.com/deniswernert/go-fstab"
	"github.com/godbus/dbus"
	"golang.org/x/sys/unix"
)

func init() {
	engine.RegisterResource("mount", func() engine.Res { return &MountRes{} })
}

const (
	// procFilesystems is a file that lists all the valid filesystem types.
	procFilesystems = "/proc/filesystems"
	// procPath is the path to /proc/mounts which contains all active mounts.
	procPath = "/proc/mounts"
	// fstabPath is the path to the fstab file which defines mounts.
	fstabPath = "/etc/fstab"
	// fstabUmask is the umask (permissions) used to edit /etc/fstab.
	fstabUmask = 0644

	// getStatus64 is an ioctl command to get the status of file backed
	// loopback devices (i.e. iso file mounts.)
	getStatus64 = 0x4C05
	// loopFileUmask is the umask (permissions) used to read the loop file.
	loopFileUmask = 0660

	// devDisk is the path where disks and partitions can be found, organized
	// by uuid/label/path.
	devDisk = "/dev/disk/"
	// diskByUUID is the location of symlinks for devices by UUID.
	diskByUUID = devDisk + "by-uuid/"
	// diskByLabel is the location of symlinks for devices by label.
	diskByLabel = devDisk + "by-label/"
	// diskByUUID is the location of symlinks for partitions by UUID.
	diskByPartUUID = devDisk + "by-partuuid/"
	// diskByLabel is the location of symlinks for partitions by label.
	diskByPartLabel = devDisk + "by-partlabel/"

	// dbusSystemdService is the service to connect to systemd itself.
	dbusSystemd1Service = "org.freedesktop.systemd1"
	// dbusSystemd1Interface is the base systemd1 path.
	dbusSystemd1Path = "/org/freedesktop/systemd1"
	// dbusUnitPath is the dbus path where mount unit files are found.
	dbusUnitPath = dbusSystemd1Path + "/unit/"
	// dbusSystemd1Interface is the base systemd1 interface.
	dbusSystemd1Interface = "org.freedesktop.systemd1"
	// dbusMountInterface is used as an argument to filter dbus messages.
	dbusMountInterface = dbusSystemd1Interface + ".Mount"
	// dbusManagerInterface is the systemd manager interface used for
	// interfacing with systemd units.
	dbusManagerInterface = dbusSystemd1Interface + ".Manager"
	// dbusRestartUnit is the dbus method for restarting systemd units.
	dbusRestartUnit = dbusManagerInterface + ".RestartUnit"
	// dbusReloadSystemd is the dbus method for reloading systemd settings.
	// (i.e. systemctl daemon-reload)
	dbusReloadSystemd = dbusManagerInterface + ".Reload"
	// restartTimeout is the delay before restartUnit is assumed to have
	// failed.
	dbusRestartCtxTimeout = 10
	// dbusSignalJobRemoved is the name of the dbus signal that produces a
	// message when a dbus job is done (or has errored.)
	dbusSignalJobRemoved = "JobRemoved"
)

// MountRes is a systemd mount resource that adds/removes entries from
// /etc/fstab, and makes sure the defined device is mounted or unmounted
// accordingly. The mount point is set according to the resource's name.
type MountRes struct {
	traits.Base

	init *engine.Init

	// State must be exists ot absent. If absent, remaining fields are ignored.
	State   string            `yaml:"state"`
	Device  string            `yaml:"device"`  // location of the device or image
	Type    string            `yaml:"type"`    // the type of filesystem
	Options map[string]string `yaml:"options"` // mount options
	Freq    int               `yaml:"freq"`    // dump frequency
	PassNo  int               `yaml:"passno"`  // verification order

	mount *fstab.Mount // struct representing the mount
}

// Default returns some sensible defaults for this resource.
func (obj *MountRes) Default() engine.Res {
	return &MountRes{
		Options: defaultMntOps(),
	}
}

// Validate if the params passed in are valid data.
func (obj *MountRes) Validate() error {
	var err error

	// validate state
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be 'exists', or 'absent'")
	}

	// validate type
	fs, err := ioutil.ReadFile(procFilesystems)
	if err != nil {
		return errwrap.Wrapf(err, "error reading %s", procFilesystems)
	}
	fsSlice := strings.Fields(string(fs))
	for i, x := range fsSlice {
		if x == "nodev" {
			fsSlice = append(fsSlice[:i], fsSlice[i+1:]...)
		}
	}
	if obj.State != "absent" && !util.StrInList(obj.Type, fsSlice) {
		return fmt.Errorf("type must be a valid filesystem type (see /proc/filesystems)")
	}

	// validate mountpoint
	if strings.Contains(obj.Name(), "//") {
		return fmt.Errorf("double slashes are not allowed in resource name")
	}
	if err := unix.Access(obj.Name(), unix.R_OK); err != nil {
		return errwrap.Wrapf(err, "error validating mount point: %s", obj.Name())
	}

	// validate device
	device, err := evalSpec(obj.Device) // eval symlink
	if err != nil {
		return errwrap.Wrapf(err, "error evaluating spec: %s", obj.Device)
	}
	if err := unix.Access(device, unix.R_OK); err != nil {
		return errwrap.Wrapf(err, "error validating device: %s", device)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *MountRes) Init(init *engine.Init) error {
	obj.init = init //save for later

	obj.mount = &fstab.Mount{
		Spec:    obj.Device,
		File:    obj.Name(),
		VfsType: obj.Type,
		MntOps:  obj.Options,
		Freq:    obj.Freq,
		PassNo:  obj.PassNo,
	}
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *MountRes) Close() error {
	return nil
}

// Watch listens for signals from the mount unit associated with the resource.
// It also watch for changes to /etc/fstab, where mounts are defined.
func (obj *MountRes) Watch() error {
	// make sure systemd is running
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	// establish a godbus connection
	conn, err := util.SystemBusPrivateUsable()
	if err != nil {
		return errwrap.Wrapf(err, "error establishing dbus connection")
	}
	defer conn.Close()

	// add a dbus rule to watch signals from the mount unit.
	args := fmt.Sprintf("type='signal', path='%s', arg0='%s'",
		dbusUnitPath+sdbus.PathBusEscape(unit.UnitNamePathEscape((obj.Name()+".mount"))),
		dbusMountInterface,
	)
	if call := conn.BusObject().Call(engineUtil.DBusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error creating dbus call")
	}
	defer conn.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	ch := make(chan *dbus.Signal)
	defer close(ch)

	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	// watch the fstab file
	recWatcher, err := recwatch.NewRecWatcher(fstabPath, false)
	if err != nil {
		return err
	}
	// close the recwatcher when we're done
	defer recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	var send bool
	var done bool
	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok {
				if done {
					return nil
				}
				done = true
				continue
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown recwatcher error")
			}
			if obj.init.Debug {
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

			send = true

		case event, ok := <-ch:
			if !ok {
				if done {
					return nil
				}
				done = true
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("event: %+v", event)
			}

			send = true

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

// fstabCheckApply checks /etc/fstab for entries corresponding to the resource
// definition, and adds or deletes the entry as needed.
func (obj *MountRes) fstabCheckApply(apply bool) (bool, error) {
	exists, err := fstabEntryExists(fstabPath, obj.mount)
	if err != nil {
		return false, errwrap.Wrapf(err, "error checking if fstab entry exists")
	}

	// if everything is as it should be, we're done
	if (exists && obj.State == "exists") || (!exists && obj.State == "absent") {
		return true, nil
	}

	if !apply {
		return false, nil
	}
	obj.init.Logf("fstabCheckApply(%t)", apply)

	if obj.State == "exists" {
		if err := obj.fstabEntryAdd(fstabPath, obj.mount); err != nil {
			return false, errwrap.Wrapf(err, "error adding fstab entry: %+v", obj.mount)
		}
		return false, nil
	}
	if err := obj.fstabEntryRemove(fstabPath, obj.mount); err != nil {
		return false, errwrap.Wrapf(err, "error removing fstab entry: %+v", obj.mount)
	}
	return false, nil
}

// mountCheckApply checks if the defined resource is mounted, and mounts or
// unmounts it according to the defined state.
func (obj *MountRes) mountCheckApply(apply bool) (bool, error) {
	exists, err := mountExists(procPath, obj.mount)
	if err != nil {
		return false, errwrap.Wrapf(err, "error checking if mount exists")
	}

	// if everything is as it should be, we're done
	if (exists && obj.State == "exists") || (!exists && obj.State == "absent") {
		return true, nil
	}

	if !apply {
		return false, nil
	}
	obj.init.Logf("mountCheckApply(%t)", apply)

	if obj.State == "exists" {
		// Reload mounts from /etc/fstab by performing a `daemon-reload` and
		// restarting `local-fs.target` and `remote-fs.target` units.
		if err := mountReload(); err != nil {
			return false, errwrap.Wrapf(err, "error reloading /etc/fstab")
		}
		return false, nil // we're done
	}
	// unmount the device
	if err := unix.Unmount(obj.Name(), 0); err != nil { // 0 means no flags
		return false, errwrap.Wrapf(err, "error unmounting %s", obj.Name())
	}
	return false, nil
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *MountRes) CheckApply(apply bool) (bool, error) {
	checkOK := true

	if c, err := obj.fstabCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	if c, err := obj.mountCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil
}

// Cmp compares two resources and return if they are equivalent.
func (obj *MountRes) Cmp(r engine.Res) error {
	// we can only compare MountRes to others of the same resource kind
	res, ok := r.(*MountRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Type != res.Type {
		return fmt.Errorf("the Type differs")
	}
	if !strMapEq(obj.Options, res.Options) {
		return fmt.Errorf("the Options differ")
	}
	if obj.Freq != res.Freq {
		return fmt.Errorf("the Type differs")
	}
	if obj.PassNo != res.PassNo {
		return fmt.Errorf("the PassNo differs")
	}

	return nil
}

// MountUID is a unique resource identifier.
type MountUID struct {
	engine.BaseUID
	name string
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *MountUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*MountUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one although some resources can return multiple.
func (obj *MountRes) UIDs() []engine.ResUID {
	x := &MountUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *MountRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes MountRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*MountRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to MountRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = MountRes(raw) // restore from indirection with type conversion!
	return nil
}

// defaultMntOps returns a map that sets the default mount options for fstab
// mounts.
func defaultMntOps() map[string]string {
	return map[string]string{"defaults": ""}
}

// strMapEq returns true, if and only if the two provided maps are identical.
func strMapEq(x, y map[string]string) bool {
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if val, ok := x[k]; !ok || v != val {
			return false
		}
	}
	return true
}

// fstabEntryExists checks whether or not a given mount exists in the provided
// fstab file.
func fstabEntryExists(file string, mount *fstab.Mount) (bool, error) {
	mounts, err := fstab.ParseFile(file)
	if err != nil {
		return false, errwrap.Wrapf(err, "error parsing file: %s", file)
	}
	for _, m := range mounts {
		if m.Equals(mount) {
			return true, nil
		}
	}
	return false, nil
}

// fstabEntryAdd adds the given mount to the provided fstab file.
func (obj *MountRes) fstabEntryAdd(file string, mount *fstab.Mount) error {
	mounts, err := fstab.ParseFile(file)
	if err != nil {
		return errwrap.Wrapf(err, "error parsing file: %s", file)
	}
	for _, m := range mounts {
		// if the entry exists, we're done
		if m.Equals(mount) {
			return nil
		}
	}
	// mount does not exist so we need to add it
	mounts = append(mounts, mount)
	return obj.fstabWrite(file, mounts)
}

// fstabEntryRemove removes the given mount from the provided fstab file.
func (obj *MountRes) fstabEntryRemove(file string, mount *fstab.Mount) error {
	mounts, err := fstab.ParseFile(file)
	if err != nil {
		return errwrap.Wrapf(err, "error parsing file: %s", file)
	}
	for i, m := range mounts {
		// remove any entry with the defined mountpoint
		if m.File == mount.File {
			mounts = append(mounts[:i], mounts[i+1:]...)
		}
	}
	return obj.fstabWrite(file, mounts)
}

// fstabWrite generates an fstab file with the given mounts, and writes them to
// the provided fstab file.
func (obj *MountRes) fstabWrite(file string, mounts fstab.Mounts) error {
	// build the file contents
	contents := fmt.Sprintf("# Generated by %s at %d", obj.init.Program, time.Now().UnixNano()) + "\n"
	contents = contents + mounts.String() + "\n"
	// write the file
	if err := ioutil.WriteFile(file, []byte(contents), fstabUmask); err != nil {
		return errwrap.Wrapf(err, "error writing fstab file: %s", file)
	}
	return nil
}

// mountExists returns true, if a given mount exists in the given file
// (typically /proc/mounts.)
func mountExists(file string, mount *fstab.Mount) (bool, error) {
	var err error
	m := *mount // make a copy so we don't change the definition

	// resolve the device's symlink if there is one
	if m.Spec, err = evalSpec(mount.Spec); err != nil {
		return false, errwrap.Wrapf(err, "error evaluating spec: %s", mount.Spec)
	}

	// get all mounts
	mounts, err := fstab.ParseFile(file)
	if err != nil {
		return false, errwrap.Wrapf(err, "error parsing file: %s", file)
	}
	// check for the defined mount
	for _, p := range mounts {
		found, err := mountCompare(&m, p)
		if err != nil {
			return false, errwrap.Wrapf(err, "mounts could not be compared: %s and %s", mount.String(), p.String())
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// mountCompare compares two mounts. It is assumed that the first comes from a
// resource definition, and the second comes from /proc/mounts. It compares the
// two after resolving the loopback device's file path (if necessary,) and
// ignores freq and passno, as they may differ between the definition and
// /proc/mounts.
func mountCompare(def, proc *fstab.Mount) (bool, error) {
	if def.Equals(proc) {
		return true, nil
	}
	if def.File != proc.File {
		return false, nil
	}
	if def.Spec != "" {
		procSpec, err := loopFilePath(proc.Spec)
		if err != nil {
			return false, err
		}
		if def.Spec != procSpec {
			return false, nil
		}
	}
	if !strMapEq(def.MntOps, defaultMntOps()) && !strMapEq(def.MntOps, proc.MntOps) {
		return false, nil
	}
	if def.VfsType != "" && def.VfsType != proc.VfsType {
		return false, nil
	}
	return true, nil
}

// mountReload performs a daemon-reload and restarts fs-local.target and
// fs-remote.target, to let systemd mount any new entries in /etc/fstab.
func mountReload() error {
	// establish a godbus connection
	conn, err := util.SystemBusPrivateUsable()
	if err != nil {
		return errwrap.Wrapf(err, "error establishing dbus connection")
	}
	defer conn.Close()
	// systemctl daemon-reload
	call := conn.Object(dbusSystemd1Service, dbusSystemd1Path).Call(dbusReloadSystemd, 0)
	if call.Err != nil {
		return errwrap.Wrapf(call.Err, "error reloading systemd")
	}

	// systemctl restart local-fs.target
	if err := restartUnit(conn, "local-fs.target"); err != nil {
		return errwrap.Wrapf(err, "error restarting unit")
	}

	// systemctl restart remote-fs.target
	if err := restartUnit(conn, "remote-fs.target"); err != nil {
		return errwrap.Wrapf(err, "error restarting unit")
	}

	return nil
}

// restartUnit restarts the given dbus unit and waits for it to finish starting
// up. If restartTimeout is exceeded, it will return an error.
func restartUnit(conn *dbus.Conn, unit string) error {
	// timeout if we don't get the JobRemoved event
	ctx, cancel := context.WithTimeout(context.TODO(), dbusRestartCtxTimeout*time.Second)
	defer cancel()

	// Add a dbus rule to watch the systemd1 JobRemoved signal used to wait
	// until the restart job completes.
	args := fmt.Sprintf("type='signal', path='%s', interface='%s', member='%s', arg2='%s'",
		dbusSystemd1Path,
		dbusManagerInterface,
		dbusSignalJobRemoved,
		unit,
	)
	if call := conn.BusObject().Call(engineUtil.DBusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error creating dbus call")
	}
	defer conn.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	// channel for godbus connection
	ch := make(chan *dbus.Signal)
	defer close(ch)

	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	// restart the unit
	sd1 := conn.Object(dbusSystemd1Service, dbus.ObjectPath(dbusSystemd1Path))
	if call := sd1.Call(dbusRestartUnit, 0, unit, "fail"); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error restarting unit: %s", unit)
	}

	// wait for the job to be removed, indicating completion
	select {
	case event, ok := <-ch:
		if !ok {
			return fmt.Errorf("channel closed unexpectedly")
		}
		if event.Body[3] != "done" {
			return fmt.Errorf("unexpected job status: %s", event.Body[3])
		}
	case <-ctx.Done():
		return fmt.Errorf("restarting %s failed due to context timeout", unit)
	}
	return nil
}

// evalSpec resolves the device from the supplied spec, i.e. it follows the
// symlink, if any, from the provided uuid, label, or path.
func evalSpec(spec string) (string, error) {
	var path string
	m := &fstab.Mount{}
	m.Spec = spec

	switch m.SpecType() {
	case fstab.UUID:
		path = diskByUUID + m.SpecValue()
	case fstab.Label:
		path = diskByLabel + m.SpecValue()
	case fstab.PartUUID:
		path = diskByPartUUID + m.SpecValue()
	case fstab.PartLabel:
		path = diskByPartLabel + m.SpecValue()
	case fstab.Path:
		path = m.SpecValue()
	default:
		return "", fmt.Errorf("unexpected spec type: %v", m.SpecType())
	}

	return filepath.EvalSymlinks(path)
}

// loopFilePath returns the file path of the mounted filesystem image, backing
// the given loopback device.
func loopFilePath(spec string) (string, error) {
	// if it's not a loopback device, return the input
	if !strings.Contains(spec, "/dev/loop") {
		return spec, nil
	}
	info, err := getLoopInfo(spec)
	if err != nil {
		return "", errwrap.Wrapf(err, "error getting loop info")
	}
	// trim the extra null chars off the end of the filename
	return string(bytes.Trim(info.FileName[:], "\x00")), nil
}

// loopInfo is a datastructure that holds relevant information about a file
// backed loopback device. Code is based on freddierice/go-losetup.
type loopInfo struct {
	Device         uint64
	INode          uint64
	RDevice        uint64
	Offset         uint64
	SizeLimit      uint64
	Number         uint32
	EncryptType    uint32
	EncryptKeySize uint32
	Flags          uint32
	FileName       [64]byte
	CryptName      [64]byte
	EncryptKey     [32]byte
	Init           [2]uint64
}

// getLoopInfo returns a loopInfo struct containing information about the
// provided file backed loopback device.
func getLoopInfo(loop string) (*loopInfo, error) {
	// open the loop file
	f, err := os.OpenFile(loop, 0, loopFileUmask)
	if err != nil {
		return nil, fmt.Errorf("error opening %s: %s", loop, err)
	}
	defer f.Close()

	// deserialize the contents
	retInfo := &loopInfo{}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), getStatus64, uintptr(unsafe.Pointer(retInfo)))
	if errno == unix.ENXIO {
		return nil, fmt.Errorf("device not backed by a file")
	} else if errno != 0 {
		return nil, fmt.Errorf("error getting info about %s (errno: %d)", loop, errno)
	}

	return retInfo, nil
}
