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

// DOCS: https://godoc.org/github.com/coreos/go-systemd/dbus

package resources

import (
	"context"
	"fmt"
	"os/user"
	"path"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	systemd "github.com/coreos/go-systemd/v22/dbus" // change namespace
	systemdUtil "github.com/coreos/go-systemd/v22/util"
	"github.com/godbus/dbus/v5" // namespace collides with systemd wrapper
)

func init() {
	engine.RegisterResource("svc", func() engine.Res { return &SvcRes{} })
}

// The SystemdUnitMode* constants do the following from the docs:
//
// The mode needs to be one of replace, fail, isolate, ignore-dependencies,
// ignore-requirements. If "replace" the call will start the unit and its
// dependencies, possibly replacing already queued jobs that conflict with this.
// If "fail" the call will start the unit and its dependencies, but will fail if
// this would change an already queued job. If "isolate" the call will start the
// unit in question and terminate all units that aren't dependencies of it. If
// "ignore-dependencies" it will start a unit but ignore all its dependencies.
// If "ignore-requirements" it will start a unit but only ignore the requirement
// dependencies. It is not recommended to make use of the latter two options.
const (
	SystemdUnitModeReplace            = "replace"
	SystemdUnitModeFail               = "fail"
	SystemdUnitModeIsolate            = "isolate"
	SystemdUnitModeIgnoreDependencies = "ignore-dependencies"
	SystemdUnitModeIgnoreRequirements = "ignore-requirements"
)

// The SystemdUnitResult* constants do the following from the docs:
//
// If the provided channel is non-nil, a result string will be sent to it upon
// job completion: one of done, canceled, timeout, failed, dependency, skipped.
// "done" indicates successful execution of a job. "canceled" indicates that a
// job has been canceled before it finished execution. "timeout" indicates that
// the job timeout was reached. "failed" indicates that the job failed.
// "dependency" indicates that a job this job has been depending on failed and
// the job hence has been removed too. "skipped" indicates that a job was
// skipped because it didn't apply to the units current state.
const (
	SystemdUnitResultDone       = "done"
	SystemdUnitResultCanceled   = "canceled"
	SystemdUnitResultTimeout    = "timeout"
	SystemdUnitResultFailed     = "failed"
	SystemdUnitResultDependency = "dependency"
	SystemdUnitResultSkipped    = "skipped"
)

// SvcRes is a service resource for systemd units.
type SvcRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Groupable
	traits.Refreshable

	init *engine.Init

	// State is the desired state for this resource. Valid values include:
	// running, stopped, and undefined (empty string).
	State string `lang:"state" yaml:"state"`

	// Startup specifies what should happen on startup. Values can be:
	// enabled, disabled, and undefined (empty string).
	Startup string `lang:"startup" yaml:"startup"`

	// Session specifies if this is for a system service (false) or a user
	// session specific service (true).
	Session bool `lang:"session" yaml:"session"` // user session (true) or system?
}

// Default returns some sensible defaults for this resource.
func (obj *SvcRes) Default() engine.Res {
	return &SvcRes{}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *SvcRes) Validate() error {
	if obj.State != "running" && obj.State != "stopped" && obj.State != "" {
		return fmt.Errorf("state must be either `running` or `stopped` or undefined")
	}
	if obj.Startup != "enabled" && obj.Startup != "disabled" && obj.Startup != "" {
		return fmt.Errorf("startup must be either `enabled` or `disabled` or undefined")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *SvcRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *SvcRes) Cleanup() error {
	return nil
}

// svc is a helper that returns the systemd name.
func (obj *SvcRes) svc() string {
	return fmt.Sprintf("%s.service", obj.Name())
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *SvcRes) Watch(ctx context.Context) error {
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // make sure we always close any below ctx just in case!

	var conn *systemd.Conn
	var err error
	if obj.Session {
		conn, err = systemd.NewUserConnectionContext(ctx) // user session
	} else {
		// we want NewSystemConnectionContext but New... falls back to this
		conn, err = systemd.NewWithContext(ctx) // needs root access
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to systemd")
	}
	defer conn.Close()

	// if we share the bus with others, we will get each others messages!!
	var bus *dbus.Conn
	if obj.Session {
		bus, err = util.SessionBusPrivateUsable()
	} else {
		bus, err = util.SystemBusPrivateUsable()
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()

	// NOTE: I guess it's not the worst-case scenario if we drop signal or
	// if it fills up and we block. Whichever way the upstream implements it
	// we'll have a back log of signals to loop through which is just fine.
	chBus := make(chan *dbus.Signal, 10) // TODO: what size if any?
	defer close(chBus)                   // NOTE: closing a chan that contains a value is ok
	bus.Signal(chBus)
	defer bus.RemoveSignal(chBus) // not needed here, but nice for symmetry

	// Legacy way to do this matching...
	//method := "org.freedesktop.DBus.AddMatch"
	//flags := dbus.Flags(0)
	//args := []interface{}{"type='signal',interface='org.freedesktop.systemd1.Manager',member='Reloading'"}
	//call := bus.BusObject().CallWithContext(ctx, method, flags, args...) // *dbus.Call
	//if err := call.Err; err != nil {
	//	return errwrap.Wrapf(err, "failed to connect signal on bus")
	//}
	matchOptions := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
		dbus.WithMatchMember("Reloading"),
	}
	if err := bus.AddMatchSignalContext(ctx, matchOptions...); err != nil {
		return errwrap.Wrapf(err, "failed to add match signal on bus")
	}
	defer func() {
		// On shutdown, we prefer to give this a chance to run. If we
		// use the main ctx, then it will error because ctx cancelled.
		ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
		defer cancel()
		if err := bus.RemoveMatchSignalContext(ctx, matchOptions...); err != nil {
			obj.init.Logf("failed to remove match signal on bus: %+v", err)
		}
	}()

	obj.init.Running() // when started, notify engine that we're running

	svc := obj.svc() // systemd name

	set := conn.NewSubscriptionSet() // no error should be returned
	// XXX: dynamic bugs: https://github.com/coreos/go-systemd/issues/474
	set.Add(svc) // it's okay if the svc doesn't exist yet
	chSub, chSubErr := set.Subscribe()
	//defer close(chSub) // cannot close receive-only channel
	//defer close(chSubErr) // cannot close receive-only channel

	//chSubClosed := false
	//chSubErrClosed := false
	for {
		//if chSubClosed && chSubErrClosed {
		//
		//}

		if obj.init.Debug {
			obj.init.Logf("watching...")
		}
		select {

		case sig, ok := <-chBus:
			if !ok {
				chBus = nil
				return fmt.Errorf("unexpected close") // we close this one!
			}
			if obj.init.Debug {
				obj.init.Logf("sig: %+v", sig)
			}

			// This event happens if we `systemctl daemon-reload` or
			// if `systemctl enable/disable <svc>` is run. For both
			// of these situations we seem to always get two events.
			// The first seems to have `Body:[true]`, and the second
			// has `Body:[false]`.

			// https://pkg.go.dev/github.com/godbus/dbus/v5#Signal
			//eg: &{Sender::1.287 Path:/org/freedesktop/systemd1 Name:org.freedesktop.systemd1.Manager.Reloading Body:[false] Sequence:7}
			if sig.Name != "org.freedesktop.systemd1.Manager.Reloading" {
				// not for us
				continue
			}

			if len(sig.Body) == 0 {
				// does this ever happen? send a signal for now
				obj.init.Logf("daemon reload with empty body")
				break // break out of select and send event now
			}

			if len(sig.Body) > 1 {
				// does this ever happen? send a signal for now
				obj.init.Logf("daemon reload with big body")
				break // break out of select and send event now
			}

			b, ok := sig.Body[0].(bool)
			if !ok {
				// does this ever happen? send a signal for now
				obj.init.Logf("daemon reload with badly typed body")
				break // break out of select and send event now
			}

			// We do all of this annoying parsing to cut our event
			// count by half, since these signals seem to come in
			// pairs. We skip the "true" one that comes first.
			if b {
				if obj.init.Debug {
					obj.init.Logf("skipping daemon-reload start")
				}
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("daemon reload") // success!
			}

		case event, ok := <-chSub:
			if !ok {
				chSub = nil
				//chSubClosed = true
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("event: %+v", event)
			}

			// The value returned is a map in case we monitor many.
			unitStatus, ok := event[svc]
			if !ok { // not me
				continue
			}

			if unitStatus == nil {
				if obj.init.Debug {
					obj.init.Logf("service stopped")
				}
				break // break out of select and send event now
			}

			msg := ""
			switch event[svc].ActiveState { // string
			case "active":
				msg = "service started"
			case "inactive":
				msg = "service stopped"
			case "reloading":
				msg = "service reloading"
			case "failed":
				msg = "service failed"
			case "activating":
				msg = "service activating"
			case "deactivating":
				msg = "service deactivating"
			default:
				return fmt.Errorf("unknown service state: %s", event[svc].ActiveState)
			}
			if obj.init.Debug {
				obj.init.Logf("%s", msg)
			}

		case err, ok := <-chSubErr:
			if !ok {
				chSubErr = nil
				//chSubErrClosed = true
				continue
			}
			if err == nil {
				obj.init.Logf("unexpected nil error")
				continue
			}
			return errwrap.Wrapf(err, "unknown error")

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *SvcRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if !systemdUtil.IsRunningSystemd() {
		return false, fmt.Errorf("systemd is not running")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // make sure we always close any below ctx just in case!

	var conn *systemd.Conn
	var err error
	if obj.Session {
		conn, err = systemd.NewUserConnectionContext(ctx) // user session
	} else {
		// we want NewSystemConnectionContext but New... falls back to this
		conn, err = systemd.NewWithContext(ctx) // needs root access
	}
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to connect to systemd")
	}
	defer conn.Close()

	// if we share the bus with others, we will get each others messages!!
	//var bus *dbus.Conn
	//if obj.Session {
	//	bus, err = util.SessionBusPrivateUsable()
	//} else {
	//	bus, err = util.SystemBusPrivateUsable()
	//}
	//if err != nil {
	//	return errwrap.Wrapf(err, "failed to connect to bus")
	//}
	//defer bus.Close()

	svc := obj.svc() // systemd name

	loadState, err := conn.GetUnitPropertyContext(ctx, svc, "LoadState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get load state")
	}

	// NOTE: we have to compare variants with other variants, they are really strings...
	notFound := (loadState.Value == dbus.MakeVariant("not-found"))
	if notFound {
		return false, fmt.Errorf("failed to find svc: %s", svc)
	}

	//conn.GetUnitPropertiesContexts(svc)
	activeState, err := conn.GetUnitPropertyContext(ctx, svc, "ActiveState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get active state")
	}

	running := (activeState.Value == dbus.MakeVariant("active"))
	stateOK := ((obj.State == "") || (obj.State == "running" && running) || (obj.State == "stopped" && !running))

	startupState, err := conn.GetUnitPropertyContext(ctx, svc, "UnitFileState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get unit file state")
	}

	enabled := (startupState.Value == dbus.MakeVariant("enabled"))
	disabled := (startupState.Value == dbus.MakeVariant("disabled"))
	startupOK := ((obj.Startup == "") || (obj.Startup == "enabled" && enabled) || (obj.Startup == "disabled" && disabled))

	// NOTE: if this svc resource is embedded as a composite resource inside
	// of another resource using a technique such as `makeComposite()`, then
	// the Init of the embedded resource is traditionally passed through and
	// identical to the parent's Init. As a result, the data matches what is
	// expected from the parent. (So this luckily turns out to be actually a
	// thing that does help, although it is important to add the Refreshable
	// trait to the parent resource, or we'll panic when we call this line.)
	// It might not be recommended to use the Watch method without a thought
	// to what actually happens when we would run Send(), and other methods.
	refresh := obj.init.Refresh() // do we have a pending reload to apply?

	if stateOK && startupOK && !refresh {
		return true, nil // we are in the correct state
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion

	if !startupOK && obj.Startup != "" {
		files := []string{svc} // the svc represented in a list
		if obj.Startup == "enabled" {
			_, _, err = conn.EnableUnitFilesContext(ctx, files, false, true)
		} else if obj.Startup == "disabled" {
			_, err = conn.DisableUnitFilesContext(ctx, files, false)
		} else {
			// pass
		}
		if err != nil {
			return false, errwrap.Wrapf(err, "unable to change startup status")
		}
		if obj.Startup == "enabled" {
			obj.init.Logf("service enabled")
		} else if obj.Startup == "disabled" {
			obj.init.Logf("service disabled")
		}
	}

	// XXX: do we need to use a buffered channel here?
	result := make(chan string, 1) // catch result information
	defer close(result)
	var status string
	var ok bool

	if !stateOK && obj.State != "" {
		if obj.State == "running" {
			_, err = conn.StartUnitContext(ctx, svc, SystemdUnitModeFail, result)
		} else if obj.State == "stopped" {
			_, err = conn.StopUnitContext(ctx, svc, SystemdUnitModeFail, result)
		} else { // skip through this section
			// TODO: should we do anything here instead?
			result <- "" // chan is buffered, so won't block
		}
		if err != nil {
			return false, errwrap.Wrapf(err, "unable to change running status")
		}
		if refresh {
			obj.init.Logf("skipping reload, due to pending start/stop")
		}
		refresh = false // We did a start or stop, so a reload is not needed.

		// TODO: Should we permanenty error after a long timeout here?
		for {
			warn := true // warn once
			select {
			case status, ok = <-result:
				if !ok {
					return false, fmt.Errorf("unexpected closed channel during start/stop")
				}
				break

			case <-time.After(10 * time.Second):
				if warn {
					obj.init.Logf("service start/stop is slow...")
				}
				warn = false
				continue

			case <-ctx.Done():
				return false, ctx.Err()
			}
			break // don't loop forever
		}

		switch status {
		case "":
			// pass

		case SystemdUnitResultDone:
			if obj.State == "running" {
				obj.init.Logf("service started")
			} else if obj.State == "stopped" {
				obj.init.Logf("service stopped")
			}

		case SystemdUnitResultCanceled:
			// TODO: should this be context.Canceled?
			return false, fmt.Errorf("operation cancelled")

		case SystemdUnitResultTimeout:
			return false, fmt.Errorf("operation timed out")

		case SystemdUnitResultFailed:
			return false, fmt.Errorf("svc failed (selinux?)")

		default:
			return false, fmt.Errorf("unknown systemd return string: %s", status)
		}
	}

	if !refresh { // Do we need to reload the service?
		return false, nil // success
	}

	if obj.init.Debug {
		obj.init.Logf("reloading...")
	}

	// From: https://www.freedesktop.org/software/systemd/man/latest/org.freedesktop.systemd1.html
	// If a service is restarted that isn't running, it will be started
	// unless the "Try" flavor is used in which case a service that isn't
	// running is not affected by the restart. The "ReloadOrRestart" flavors
	// attempt a reload if the unit supports it and use a restart otherwise.
	if _, err := conn.ReloadOrTryRestartUnitContext(ctx, svc, SystemdUnitModeFail, result); err != nil {
		return false, errwrap.Wrapf(err, "failed to reload unit")
	}

	// TODO: Should we permanenty error after a long timeout here?
	for {
		warn := true // warn once
		select {
		case status, ok = <-result:
			if !ok {
				return false, fmt.Errorf("unexpected closed channel during reload")
			}
			break

		case <-time.After(10 * time.Second):
			if warn {
				obj.init.Logf("service start/stop is slow...")
			}
			warn = false
			continue

		case <-ctx.Done():
			return false, ctx.Err()
		}
		break // don't loop forever
	}

	switch status {
	case "":
		// pass

	case SystemdUnitResultDone:
		obj.init.Logf("service reloaded")

	case SystemdUnitResultCanceled:
		// TODO: should this be context.Canceled?
		return false, fmt.Errorf("operation cancelled")

	case SystemdUnitResultTimeout:
		return false, fmt.Errorf("operation timed out")

	case SystemdUnitResultFailed:
		return false, fmt.Errorf("svc reload failed (selinux?)")

	default:
		return false, fmt.Errorf("unknown systemd return string: %v", status)
	}

	return false, nil // success
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *SvcRes) Cmp(r engine.Res) error {
	// we can only compare SvcRes to others of the same resource kind
	res, ok := r.(*SvcRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Startup != res.Startup {
		return fmt.Errorf("the Startup differs")
	}
	if obj.Session != res.Session {
		return fmt.Errorf("the Session differs")
	}

	return nil
}

// SvcUID is the UID struct for SvcRes.
type SvcUID struct {
	// NOTE: there is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	engine.BaseUID
	name    string // the svc name
	session bool   // user session
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *SvcUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*SvcUID)
	if !ok {
		return false
	}
	if obj.name != res.name {
		return false
	}
	if obj.session != res.session {
		return false
	}
	return true
}

// SvcResAutoEdges holds the state of the auto edge generator.
type SvcResAutoEdges struct {
	data    []engine.ResUID
	pointer int
	found   bool
}

// Next returns the next automatic edge.
func (obj *SvcResAutoEdges) Next() []engine.ResUID {
	if obj.found {
		panic("shouldn't be called anymore!")
	}
	if len(obj.data) == 0 { // check length for rare scenarios
		return nil
	}
	value := obj.data[obj.pointer]
	obj.pointer++
	return []engine.ResUID{value} // we return one, even though api supports N
}

// Test gets results of the earlier Next() call, & returns if we should
// continue!
func (obj *SvcResAutoEdges) Test(input []bool) bool {
	// if there aren't any more remaining
	if len(obj.data) <= obj.pointer {
		return false
	}
	if obj.found { // already found, done!
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		panic("expecting a single value")
	}
	if input[0] { // if a match is found, we're done!
		obj.found = true // no more to find!
		return false
	}
	return true // keep going
}

// SvcResAutoEdgesCron holds the state of the svc -> cron auto edge generator.
type SvcResAutoEdgesCron struct {
	unit    string // target unit
	session bool   // user session
}

// Next returns the next automatic edge.
func (obj *SvcResAutoEdgesCron) Next() []engine.ResUID {
	// XXX: should this be true if SvcRes State == "stopped"?
	reversed := false
	value := &CronUID{
		BaseUID: engine.BaseUID{
			Kind:     "CronRes",
			Reversed: &reversed,
		},
		unit:    obj.unit,    // target unit
		session: obj.session, // user session
	}
	return []engine.ResUID{value} // we return one, even though api supports N
}

// Test takes the output of the last call to Next() and outputs true if we
// should continue.
func (obj *SvcResAutoEdgesCron) Test([]bool) bool {
	return false // only get one svc -> cron edge
}

// AutoEdges returns the AutoEdge interface. In this case, systemd unit file
// resources and cron (systemd-timer) resources.
func (obj *SvcRes) AutoEdges() (engine.AutoEdge, error) {
	var data []engine.ResUID
	var svcFiles []string

	svc := obj.svc() // systemd name

	svcFiles = []string{
		// root svc
		fmt.Sprintf("/etc/systemd/system/%s", svc),     // takes precedence
		fmt.Sprintf("/usr/lib/systemd/system/%s", svc), // pkg default
	}
	if obj.Session {
		// user svc
		u, err := user.Current()
		if err != nil {
			return nil, errwrap.Wrapf(err, "error getting current user")
		}
		if u.HomeDir == "" {
			return nil, fmt.Errorf("user has no home directory")
		}
		svcFiles = []string{
			path.Join(u.HomeDir, "/.config/systemd/user/", svc),
		}
	}
	for _, x := range svcFiles {
		var reversed = true
		data = append(data, &FileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: x, // what matters
		})
	}

	fileEdge := &FileResAutoEdges{
		data:    data,
		pointer: 0,
		found:   false,
	}
	cronEdge := &SvcResAutoEdgesCron{
		session: obj.Session,
		unit:    svc,
	}

	return engineUtil.AutoEdgeCombiner(fileEdge, cronEdge)
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *SvcRes) UIDs() []engine.ResUID {
	x := &SvcUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),  // svc name
		session: obj.Session, // user session
	}
	return []engine.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
//func (obj *SvcRes) GroupCmp(r engine.GroupableRes) error {
//	_, ok := r.(*SvcRes)
//	if !ok {
//		return fmt.Errorf("resource is not the same kind")
//	}
//	// TODO: depending on if the systemd service api allows batching, we
//	// might be able to build this, although not sure how useful it is...
//	// it might just eliminate parallelism by bunching up the graph
//	return fmt.Errorf("not possible at the moment")
//}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *SvcRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes SvcRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*SvcRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to SvcRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = SvcRes(raw) // restore from indirection with type conversion!
	return nil
}
