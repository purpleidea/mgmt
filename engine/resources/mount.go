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

package resources

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	sdbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/coreos/go-systemd/v22/unit"
	systemdUtil "github.com/coreos/go-systemd/v22/util"
	"github.com/godbus/dbus/v5"
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

	// getStatus64 is an ioctl command to get the status of file backed
	// loopback devices (i.e. iso file mounts.)
	getStatus64 = 0x4C05
	// loopFileUmask is the umask (permissions) used to read the loop file.
	loopFileUmask = 0660

	// where we write new unit files
	systemdUnitsDir = "/etc/systemd/system"
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

// MountRes is a systemd mount resource that adds/removes mount units, and makes
// sure the defined device is mounted or unmounted accordingly. The mount point
// is set according to the resource's name.
type MountRes struct {
	traits.Base
	traits.Refreshable // needed because we embed a svc res
	traits.Recvable    // needed because we embed a file res

	init *engine.Init

	// State must be exists or absent. If absent, remaining fields are
	// ignored.
	State string `lang:"state" yaml:"state"`

	// Device is the location of the device or image.
	Device string `lang:"device" yaml:"device"`

	// Type of the filesystem.
	Type string `lang:"type" yaml:"type"`

	// Options are mount options.
	Options map[string]string `lang:"options" yaml:"options"`

	// wrapped file resource for the unit file
	unit *FileRes
	// wrapped svc resource for the systemd mount unit
	svc *SvcRes
	// the file name for the unit file
	unitPath string
	// the internal name of the mount unit
	unitName string
	// description to write into the unit file
	comment string
}

// Default returns some sensible defaults for this resource.
func (obj *MountRes) Default() engine.Res {
	return &MountRes{}
}

func (obj *MountRes) makeComposite() (*SvcRes, *FileRes, error) {
	if obj.unitName == "" {
		obj.setUnitNameAndPath()
	}
	res, err := engine.NewNamedResource("svc", obj.unitName)
	if err != nil {
		return nil, nil, err
	}
	svc := res.(*SvcRes)
	if obj.State == "exists" {
		svc.State = "running"
	} else {
		svc.State = "stopped"
	}
	svc.Type = "mount"

	res, err = engine.NewNamedResource("file", obj.unitPath)
	if err != nil {
		return nil, nil, err
	}
	file := res.(*FileRes)
	file.State = obj.State
	if file.State != "absent" {
		file.Content = obj.generateUnitFileContent()
	}

	return svc, file, nil
}

func (obj *MountRes) generateUnitFileContent() *string {
	type_line := ""
	if obj.Type != "" {
		type_line = fmt.Sprintf("Type=%s\n", obj.Type)
	}
	options_line := ""
	if obj.Options != nil && len(obj.Options) > 0 {
		options_line = fmt.Sprintf("Options=%s\n", obj.optionsString())
	}
	// TODO: this could potentially be cleaner through unit.Serialize
	content := fmt.Sprintf("%s\n[Mount]\nWhat=%s\nWhere=%s\n%s%s",
		obj.comment, obj.Device, obj.Name(), type_line, options_line)
	return &content
}

// setUnitNameAndPath filles the helper fields of the MountRes objects according
// to its name
func (obj *MountRes) setUnitNameAndPath() error {
	obj.unitName = unit.UnitNamePathEscape(obj.Name())
	obj.unitPath = systemdUnitsDir + "/" + obj.unitName + ".mount"
	return nil
}

func (obj *MountRes) optionsString() string {
	options := []string{}
	for option, value := range obj.Options {
		options = append(options, fmt.Sprintf("%s=%s", option, value))
	}
	return strings.Join(options, ",")
}

// Validate if the params passed in are valid data.
func (obj *MountRes) Validate() error {
	var err error

	// validate state
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be 'exists', or 'absent'")
	}

	// validate type
	fs, err := os.ReadFile(procFilesystems)
	if err != nil {
		return errwrap.Wrapf(err, "error reading %s", procFilesystems)
	}
	fsSlice := strings.Fields(string(fs))
	for i, x := range fsSlice {
		if x == "nodev" {
			fsSlice = append(fsSlice[:i], fsSlice[i+1:]...)
		}
	}
	if obj.Type != "" && obj.State != "absent" && !util.StrInList(obj.Type, fsSlice) {
		return fmt.Errorf("type must be a valid filesystem type (see /proc/filesystems)")
	}

	// validate mountpoint
	if strings.Contains(obj.Name(), "//") {
		return fmt.Errorf("double slashes are not allowed in resource name")
	}
	if strings.Index(obj.Name(), "/") != 0 {
		return fmt.Errorf("mount point must be an absolute path")
	}

	// validate device
	if obj.Device == "" {
		if obj.State != "absent" {
			return fmt.Errorf("device is mandatory unless state => absent")
		}
	} else if obj.Device == "tmpfs" {
		if obj.Type != "tmpfs" {
			return fmt.Errorf("need to specify type tmpfs when mounting tmpfs")
		}
	} else if err := unix.Access(obj.Device, unix.R_OK); err != nil {
		return errwrap.Wrapf(err, "error validating device: %s", obj.Device)
	}

	svc, file, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in validate")
	}
	if err := svc.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded svc: %s", svc)
	}
	if err := file.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded unit file: %s", svc)
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *MountRes) Init(init *engine.Init) error {
	obj.init = init //save for later
	obj.comment = fmt.Sprintf("# Created by %s from Mount['%s']", init.Program, obj.Name())
	svc, file, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	obj.svc = svc
	obj.unit = file

	// TODO: we could build a new init that adds a prefix to the logger...
	if e := obj.svc.Init(init); e != nil {
		return e
	}
	return obj.unit.Init(init)
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *MountRes) Cleanup() error {
	if obj.svc != nil {
		return obj.svc.Cleanup()
	}
	if obj.unit != nil {
		return obj.unit.Cleanup()
	}
	return nil
}

// Watch listens for signals from the mount unit associated with the resource.
// It also watch for changes to the file defining the mount.
func (obj *MountRes) Watch(ctx context.Context) error {
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
		dbusUnitPath+sdbus.PathBusEscape(obj.unitName),
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

	obj.init.Running() // when started, notify engine that we're running

	// // watch the unit file
	// recWatcher, err := recwatch.NewRecWatcher(obj.unitPath, false)
	// if err != nil {
	// 	return err
	// }
	// // close the recwatcher when we're done
	// defer recWatcher.Close()

	var send bool
	var done bool
	for {
		select {
		//case event, ok := <-recWatcher.Events():
		//	if !ok {
		//		if done {
		//			return nil
		//		}
		//		done = true
		//		continue
		//	}
		//	if err := event.Error; err != nil {
		//		return errwrap.Wrapf(err, "unknown recwatcher error")
		//	}
		//	if obj.init.Debug {
		//		obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
		//	}

		//	send = true

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

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *MountRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// this resource depends on systemd to ensure that it's running
	if !systemdUtil.IsRunningSystemd() {
		return false, fmt.Errorf("systemd is not running")
	}
	checkOK := true

	if obj.unit == nil {
		return false, fmt.Errorf("unit file resource was not initialized")
	}
	if obj.svc == nil {
		return false, fmt.Errorf("svc resource was not initialized")
	}

	// start with the file
	if ok, err := obj.unit.CheckApply(ctx, apply); err != nil {
		return false, errwrap.Wrapf(err, "could not sync unit file")
	} else if !ok {
		checkOK = false
	}

	obj.init.Logf("finished CheckApply on unit file (return %v)", checkOK)

	if !checkOK {
		err := systemdReload(ctx)
		if err != nil {
			obj.init.Logf("systemd daemon-reload failed (warning): %v", err)
		} else {
			obj.init.Logf("systemd reloaded")
		}
	}

	// now the service
	if ok, err := obj.svc.CheckApply(ctx, apply); err != nil {
		return false, errwrap.Wrapf(err, "could not sync svc resource")
	} else if !ok {
		checkOK = false
	}

	obj.init.Logf("finished CheckApply on unit svc (return %v)", checkOK)

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

// mountReload performs a daemon-reload and restarts fs-local.target and
// fs-remote.target, to let systemd mount any new entries in /etc/fstab.
func systemdReload(ctx context.Context) error {
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

	return nil
}
