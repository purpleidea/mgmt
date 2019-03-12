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
	"bytes"
	"context"
	"fmt"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	sdbus "github.com/coreos/go-systemd/dbus"
	"github.com/coreos/go-systemd/unit"
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus"
)

const (
	// OnCalendar is a systemd-timer trigger, whose behaviour is defined in
	// 'man systemd-timer', and whose format is defined in the 'Calendar
	// Events' section of 'man systemd-time'.
	OnCalendar = "OnCalendar"
	// OnActiveSec is a systemd-timer trigger, whose behaviour is defined in
	// 'man systemd-timer', and whose format is a time span as defined in
	// 'man systemd-time'.
	OnActiveSec = "OnActiveSec"
	// OnBootSec is a systemd-timer trigger, whose behaviour is defined in
	// 'man systemd-timer', and whose format is a time span as defined in
	// 'man systemd-time'.
	OnBootSec = "OnBootSec"
	// OnStartupSec is a systemd-timer trigger, whose behaviour is defined in
	// 'man systemd-timer', and whose format is a time span as defined in
	// 'man systemd-time'.
	OnStartupSec = "OnStartupSec"
	// OnUnitActiveSec is a systemd-timer trigger, whose behaviour is defined
	// in 'man systemd-timer', and whose format is a time span as defined in
	// 'man systemd-time'.
	OnUnitActiveSec = "OnUnitActiveSec"
	// OnUnitInactiveSec is a systemd-timer trigger, whose behaviour is defined
	// in 'man systemd-timer', and whose format is a time span as defined in
	// 'man systemd-time'.
	OnUnitInactiveSec = "OnUnitInactiveSec"

	// ctxTimeout is the delay, in seconds, before the calls to restart or stop
	// the systemd unit will error due to timeout.
	ctxTimeout = 30
)

func init() {
	engine.RegisterResource("cron", func() engine.Res { return &CronRes{} })
}

// CronRes is a systemd-timer cron resource.
type CronRes struct {
	traits.Base
	traits.Edgeable
	traits.Recvable
	traits.Refreshable // needed because we embed a svc res

	init *engine.Init

	// Unit is the name of the systemd service unit. It is only necessary to
	// set if you want to specify a service with a different name than the
	// resource.
	Unit string `yaml:"unit"`
	// State must be 'exists' or 'absent'.
	State string `yaml:"state"`

	// Session, if true, creates the timer as the current user, rather than
	// root. The service it points to must also be a user unit. It defaults to
	// false.
	Session bool `yaml:"session"`

	// Trigger is the type of timer. Valid types are 'OnCalendar',
	// 'OnActiveSec'. 'OnBootSec'. 'OnStartupSec'. 'OnUnitActiveSec', and
	// 'OnUnitInactiveSec'. For more information see 'man systemd.timer'.
	Trigger string `yaml:"trigger"`
	// Time must be used with all triggers. For 'OnCalendar', it must be in
	// the format defined in 'man systemd-time' under the heading 'Calendar
	// Events'. For all other triggers, time should be a valid time span as
	// defined in 'man systemd-time'
	Time string `yaml:"time"`

	// AccuracySec is the accuracy of the timer in systemd-time time span
	// format. It defaults to one minute.
	AccuracySec string `yaml:"accuracysec"`
	// RandomizedDelaySec delays the timer by a randomly selected, evenly
	// distributed amount of time between 0 and the specified time value. The
	// value must be a valid systemd-time time span.
	RandomizedDelaySec string `yaml:"randomizeddelaysec"`

	// Persistent, if true, means the time when the service unit was last
	// triggered is stored on disk. When the timer is activated, the service
	// unit is triggered immediately if it would have been triggered at least
	// once during the time when the timer was inactive. It defaults to false.
	Persistent bool `yaml:"persistent"`
	// WakeSystem, if true, will cause the system to resume from suspend,
	// should it be suspended and if the system supports this. It defaults to
	// false.
	WakeSystem bool `yaml:"wakesystem"`
	// RemainAfterElapse, if true, means an elapsed timer will stay loaded, and
	// its state remains queriable. If false, an elapsed timer unit that cannot
	// elapse anymore is unloaded. It defaults to true.
	RemainAfterElapse bool `yaml:"remainafterelapse"`

	file       *FileRes             // nested file resource
	recWatcher *recwatch.RecWatcher // recwatcher for nested file
}

// Default returns some sensible defaults for this resource.
func (obj *CronRes) Default() engine.Res {
	return &CronRes{
		State:             "exists",
		RemainAfterElapse: true,
	}
}

// makeComposite creates a pointer to a FileRes. The pointer is used to
// validate and initialize the nested file resource and to apply the file state
// in CheckApply.
func (obj *CronRes) makeComposite() (*FileRes, error) {
	p, err := obj.UnitFilePath()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error generating unit file path")
	}
	res, err := engine.NewNamedResource("file", p)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating nested file resource")
	}
	file, ok := res.(*FileRes)
	if !ok {
		return nil, fmt.Errorf("error casting fileres")
	}
	file.State = obj.State
	if obj.State != "absent" {
		s := obj.unitFileContents()
		file.Content = &s
	}
	return file, nil
}

// Validate if the params passed in are valid data.
func (obj *CronRes) Validate() error {
	// validate state
	if obj.State != "absent" && obj.State != "exists" {
		return fmt.Errorf("state must be 'absent' or 'exists'")
	}

	// validate trigger
	if obj.State == "absent" && obj.Trigger == "" {
		return nil // if trigger is undefined we can't make a unit file
	}
	if obj.Trigger == "" || obj.Time == "" {
		return fmt.Errorf("trigger and must be set together")
	}
	if obj.Trigger != OnCalendar &&
		obj.Trigger != OnActiveSec &&
		obj.Trigger != OnBootSec &&
		obj.Trigger != OnStartupSec &&
		obj.Trigger != OnUnitActiveSec &&
		obj.Trigger != OnUnitInactiveSec {

		return fmt.Errorf("invalid trigger")
	}

	// TODO: Validate time (regex?)

	// validate nested file
	file, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in validate")
	}
	if err := file.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded file: %s", obj.file)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *CronRes) Init(init *engine.Init) error {
	var err error
	obj.init = init // save for later

	obj.file, err = obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	return obj.file.Init(init)
}

// Close is run by the engine to clean up after the resource is done.
func (obj *CronRes) Close() error {
	if obj.file != nil {
		return obj.file.Close()
	}
	return nil
}

// Watch for state changes and sends a message to the bus if there is a change.
func (obj *CronRes) Watch() error {
	var bus *dbus.Conn
	var err error

	// this resource depends on systemd
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	// create a private message bus
	if obj.Session {
		bus, err = util.SessionBusPrivateUsable()
	} else {
		bus, err = util.SystemBusPrivateUsable()
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()

	// dbus addmatch arguments for the timer unit
	args := []string{}
	args = append(args, "type='signal'")
	args = append(args, "interface='org.freedesktop.systemd1.Manager'")
	args = append(args, "eavesdrop='true'")
	args = append(args, fmt.Sprintf("arg2='%s.timer'", obj.Name()))

	// match dbus messsages
	if call := bus.BusObject().Call(engineUtil.DBusAddMatch, 0, strings.Join(args, ",")); call.Err != nil {
		return err
	}
	defer bus.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	// channels for dbus signal
	dbusChan := make(chan *dbus.Signal)
	defer close(dbusChan)
	bus.Signal(dbusChan)
	defer bus.RemoveSignal(dbusChan) // not needed here, but nice for symmetry

	p, err := obj.UnitFilePath()
	if err != nil {
		return errwrap.Wrapf(err, "error generating unit file path")
	}
	// recwatcher for the systemd-timer unit file
	obj.recWatcher, err = recwatch.NewRecWatcher(p, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		select {
		case event := <-dbusChan:
			// process dbus events
			if obj.init.Debug {
				obj.init.Logf("%+v", event)
			}
			send = true

		case event, ok := <-obj.recWatcher.Events():
			// process unit file recwatch events
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", obj)
			}
			if obj.init.Debug {
				obj.init.Logf("Event(%s): %v", event.Body.Name, event.Body.Op)
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

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *CronRes) CheckApply(apply bool) (checkOK bool, err error) {
	ok := true
	// use the embedded file resource to apply the correct state
	if c, err := obj.file.CheckApply(apply); err != nil {
		return false, errwrap.Wrapf(err, "nested file failed")
	} else if !c {
		ok = false
	}
	// check timer state and apply the defined state if needed
	if c, err := obj.unitCheckApply(apply); err != nil {
		return false, errwrap.Wrapf(err, "unitCheckApply error")
	} else if !c {
		ok = false
	}
	return ok, nil
}

// unitCheckApply checks the state of the systemd-timer unit and, if apply is
// true, applies the defined state.
func (obj *CronRes) unitCheckApply(apply bool) (checkOK bool, err error) {
	var conn *sdbus.Conn
	var godbusConn *dbus.Conn

	// this resource depends on systemd to ensure that it's running
	if !systemdUtil.IsRunningSystemd() {
		return false, fmt.Errorf("systemd is not running")
	}
	// go-systemd connection
	if obj.Session {
		conn, err = sdbus.NewUserConnection()
	} else {
		conn, err = sdbus.New() // system bus
	}
	if err != nil {
		return false, errwrap.Wrapf(err, "error making go-systemd dbus connection")
	}
	defer conn.Close()

	// get the load state and active state of the timer unit
	loadState, err := conn.GetUnitProperty(fmt.Sprintf("%s.timer", obj.Name()), "LoadState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get load state")
	}
	activeState, err := conn.GetUnitProperty(fmt.Sprintf("%s.timer", obj.Name()), "ActiveState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get active state")
	}
	// check the timer unit state
	if obj.State == "absent" && loadState.Value == dbus.MakeVariant("not-found") {
		return true, nil
	}
	if obj.State == "exists" && activeState.Value == dbus.MakeVariant("active") {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	// systemctl daemon-reload
	if err := conn.Reload(); err != nil {
		return false, errwrap.Wrapf(err, "error reloading daemon")
	}

	// context for stopping/restarting the unit
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout*time.Second)
	defer cancel()

	// godbus connection for stopping/restarting the unit
	if obj.Session {
		godbusConn, err = util.SessionBusPrivateUsable()
	} else {
		godbusConn, err = util.SystemBusPrivateUsable()
	}
	if err != nil {
		return false, errwrap.Wrapf(err, "error making godbus connection")
	}
	defer godbusConn.Close()

	// stop or restart the unit
	if obj.State == "absent" {
		return false, engineUtil.StopUnit(ctx, godbusConn, fmt.Sprintf("%s.timer", obj.Name()))
	}
	return false, engineUtil.RestartUnit(ctx, godbusConn, fmt.Sprintf("%s.timer", obj.Name()))
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *CronRes) Cmp(r engine.Res) error {
	res, ok := r.(*CronRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.State != res.State {
		return fmt.Errorf("state differs: %s vs %s", obj.State, res.State)
	}
	if obj.Trigger != res.Trigger {
		return fmt.Errorf("trigger differs: %s vs %s", obj.Trigger, res.Trigger)
	}
	if obj.Time != res.Time {
		return fmt.Errorf("time differs: %s vs %s", obj.Time, res.Time)
	}
	if obj.AccuracySec != res.AccuracySec {
		return fmt.Errorf("accuracysec differs: %s vs %s", obj.AccuracySec, res.AccuracySec)
	}
	if obj.RandomizedDelaySec != res.RandomizedDelaySec {
		return fmt.Errorf("randomizeddelaysec differs: %s vs %s", obj.RandomizedDelaySec, res.RandomizedDelaySec)
	}
	if obj.Unit != res.Unit {
		return fmt.Errorf("unit differs: %s vs %s", obj.Unit, res.Unit)
	}
	if obj.Persistent != res.Persistent {
		return fmt.Errorf("persistent differs: %t vs %t", obj.Persistent, res.Persistent)
	}
	if obj.WakeSystem != res.WakeSystem {
		return fmt.Errorf("wakesystem differs: %t vs %t", obj.WakeSystem, res.WakeSystem)
	}
	if obj.RemainAfterElapse != res.RemainAfterElapse {
		return fmt.Errorf("remainafterelapse differs: %t vs %t", obj.RemainAfterElapse, res.RemainAfterElapse)
	}
	return obj.file.Cmp(r)
}

// CronUID is a unique resource identifier.
type CronUID struct {
	// NOTE: There is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	engine.BaseUID

	unit    string // name of target unit
	session bool   // user session
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *CronUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*CronUID)
	if !ok {
		return false
	}
	if obj.unit != res.unit {
		return false
	}
	if obj.session != res.session {
		return false
	}
	return true
}

// AutoEdges returns the AutoEdge interface.
func (obj *CronRes) AutoEdges() (engine.AutoEdge, error) {
	return nil, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one although some resources can return multiple.
func (obj *CronRes) UIDs() []engine.ResUID {
	unit := fmt.Sprintf("%s.service", obj.Name())
	if obj.Unit != "" {
		unit = obj.Unit
	}
	uids := []engine.ResUID{
		&CronUID{
			BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
			unit:    unit,        // name of target unit
			session: obj.Session, // user session
		},
	}
	if file, err := obj.makeComposite(); err == nil {
		uids = append(uids, file.UIDs()...) // add the file uid if we can
	}
	return uids
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *CronRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes CronRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*CronRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to CronRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = CronRes(raw) // restore from indirection with type conversion!
	return nil
}

// UnitFilePath returns the path to the systemd-timer unit file.
func (obj *CronRes) UnitFilePath() (string, error) {
	// root timer
	if !obj.Session {
		return fmt.Sprintf("/etc/systemd/system/%s.timer", obj.Name()), nil
	}
	// user timer
	u, err := user.Current()
	if err != nil {
		return "", errwrap.Wrapf(err, "error getting current user")
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("user has no home directory")
	}
	return path.Join(u.HomeDir, "/.config/systemd/user/", fmt.Sprintf("%s.timer", obj.Name())), nil
}

// unitFileContents returns the contents of the unit file representing the
// CronRes struct.
func (obj *CronRes) unitFileContents() string {
	u := []*unit.UnitOption{}

	// [Unit]
	u = append(u, &unit.UnitOption{Section: "Unit", Name: "Description", Value: "timer generated by mgmt"})
	// [Timer]
	u = append(u, &unit.UnitOption{Section: "Timer", Name: obj.Trigger, Value: obj.Time})
	if obj.AccuracySec != "" {
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "AccuracySec", Value: obj.AccuracySec})
	}
	if obj.RandomizedDelaySec != "" {
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "RandomizedDelaySec", Value: obj.RandomizedDelaySec})
	}
	if obj.Unit != "" {
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "Unit", Value: obj.Unit})
	}
	if obj.Persistent != false { // defaults to false
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "Persistent", Value: "true"})
	}
	if obj.WakeSystem != false { // defaults to false
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "WakeSystem", Value: "true"})
	}
	if obj.RemainAfterElapse != true { // defaults to true
		u = append(u, &unit.UnitOption{Section: "Timer", Name: "RemainAfterElapse", Value: "false"})
	}
	// [Install]
	u = append(u, &unit.UnitOption{Section: "Install", Name: "WantedBy", Value: "timers.target"})

	buf := new(bytes.Buffer)
	buf.ReadFrom(unit.Serialize(u))
	return buf.String()
}
