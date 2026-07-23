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
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
	esphomeUtil "github.com/purpleidea/mgmt/util/esphome"
)

const (
	// esphomeKind is the base kind of the esphome family of resources.
	esphomeKind = "esphome"

	// esphomeConnectWait is how long the entity resources wait in
	// CheckApply for the shared session to become healthy, before giving
	// up. This tolerates the asynchronous connection startup.
	esphomeConnectWait = 15 * time.Second

	// esphomeCleanupTimeout is how long the entity resources wait when
	// trying to apply a safe state during Cleanup.
	esphomeCleanupTimeout = 10 * time.Second

	// esphomeStateOn is the on state of a switch.
	esphomeStateOn = "on"

	// esphomeStateOff is the off state of a switch.
	esphomeStateOff = "off"
)

var esphomeNamedColors = map[string]uint64{
	"black": 0x000000, "white": 0xffffff, "red": 0xff0000,
	"green": 0x00ff00, "blue": 0x0000ff, "yellow": 0xffff00,
	"cyan": 0x00ffff, "magenta": 0xff00ff, "orange": 0xff8000,
	"purple": 0x8000ff,
}

func init() {
	// NOTE: The endpoint kind string must match the bridge namespace that
	// the functions read from, which is why it uses that constant.
	engine.RegisterResource(esphomeUtil.BridgeNamespace, func() engine.Res { return &EsphomeEndpointRes{} })
	engine.RegisterResource(esphomeKind+":switch", func() engine.Res { return &EsphomeSwitchRes{} })
	engine.RegisterResource(esphomeKind+":number", func() engine.Res { return &EsphomeNumberRes{} })
	engine.RegisterResource(esphomeKind+":fan", func() engine.Res { return &EsphomeFanRes{} })
	engine.RegisterResource(esphomeKind+":light", func() engine.Res { return &EsphomeLightRes{} })
}

// EsphomeEndpointRes describes how to connect to one esphome device. It doesn't
// hold the device connection itself: it validates the params and publishes them
// over the local Bridge API for the esphome functions and the other esphome
// resources to consume. Its name is the uid that all of those consumers use as
// their endpoint param. Consumers which run before this resource has published
// (functions run before resources do!) simply see zero values until it does.
type EsphomeEndpointRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Host is the ip address or hostname of the esphome device.
	Host string `lang:"host" yaml:"host"`

	// Port is the port of the esphome native api. It defaults to 6053.
	Port int `lang:"port" yaml:"port"`

	// Key is the base64 encoded noise encryption key of the device, from
	// the `api: encryption: key:` field of the device yaml. It is required;
	// mgmt never silently downgrades a device connection to plaintext.
	Key string `lang:"key" yaml:"key"`

	// Password is the legacy api password. This auth mechanism was
	// deprecated and then removed from esphome, and it is currently not
	// supported. Prefer Key.
	Password string `lang:"password" yaml:"password"`

	// Interval selects how we watch the device for events. Zero means we
	// hold a persistent connection open, over which the device pushes
	// state changes to us natively as they happen. A positive value means
	// we poll instead: every interval seconds we connect, read a full
	// state snapshot, send any pending commands, and then disconnect.
	Interval uint32 `lang:"interval" yaml:"interval"`

	// Logs enables streaming the device logger into mgmt's logs. It is empty
	// by default. Valid levels are error, warn, info, config, debug, verbose,
	// and very_verbose.
	Logs string `lang:"logs" yaml:"logs"`
}

// connInfo builds the value that we publish over the bridge.
func (obj *EsphomeEndpointRes) connInfo() *esphomeUtil.ConnInfo {
	level, _ := esphomeUtil.NormalizeLogLevel(obj.Logs) // Validate reports errors
	return &esphomeUtil.ConnInfo{
		Host:     obj.Host,
		Port:     obj.Port,
		Key:      obj.Key,
		Password: obj.Password,
		Interval: obj.Interval,
		LogLevel: level,
		Logf: func(entry *esphomeUtil.LogEntry) {
			if entry == nil || obj.init == nil {
				return
			}
			for _, line := range strings.Split(entry.Message, "\n") {
				obj.init.Logf("device log [%s]: %s", entry.Level, line)
			}
		},
		ConnectLogf: func(format string, v ...interface{}) {
			if obj.init != nil {
				obj.init.Logf(format, v...)
			}
		},
	}
}

// Default returns some sensible defaults for this resource.
func (obj *EsphomeEndpointRes) Default() engine.Res {
	return &EsphomeEndpointRes{
		Port: esphomeUtil.DefaultPort,
	}
}

// Validate if the params passed in are valid data.
func (obj *EsphomeEndpointRes) Validate() error {
	if obj.Name() == "" {
		return fmt.Errorf("empty name")
	}
	if obj.Password != "" {
		return fmt.Errorf("legacy password auth is not supported, use the noise key")
	}
	if _, err := esphomeUtil.NormalizeLogLevel(obj.Logs); err != nil {
		return err
	}
	return obj.connInfo().Validate()
}

// Init runs some startup code for this resource.
func (obj *EsphomeEndpointRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. We
// unpublish our connection info so that the consumers disconnect and fall back
// to their zero values.
func (obj *EsphomeEndpointRes) Cleanup() error {
	if obj.init == nil {
		return nil
	}
	return obj.init.Local.BridgeSet(context.TODO(), esphomeUtil.BridgeNamespace, obj.Name(), nil)
}

// Watch is the primary listener for this resource and it outputs events. Our
// state lives in the in-memory bridge which only we write to, so after the
// startup event there is nothing to watch.
func (obj *EsphomeEndpointRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	return ctx.Err()
}

// CheckApply publishes our connection info over the bridge if it isn't already
// current.
func (obj *EsphomeEndpointRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	info := obj.connInfo()

	val, err := obj.init.Local.BridgeGet(ctx, esphomeUtil.BridgeNamespace, obj.Name())
	if err != nil {
		return false, err
	}
	if published, ok := val.(*esphomeUtil.ConnInfo); ok && published.Cmp(info) == nil {
		return true, nil // already published and current
	}

	if !apply {
		return false, nil
	}

	obj.init.Logf("publishing connection info for %s", info.Addr())
	if err := obj.init.Local.BridgeSet(ctx, esphomeUtil.BridgeNamespace, obj.Name(), info); err != nil {
		return false, err
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *EsphomeEndpointRes) Cmp(r engine.Res) error {
	res, ok := r.(*EsphomeEndpointRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	return obj.connInfo().Cmp(res.connInfo())
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *EsphomeEndpointRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes EsphomeEndpointRes // indirection to avoid infinite recursion

	def := obj.Default()                 // get the default
	res, ok := def.(*EsphomeEndpointRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to EsphomeEndpointRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = EsphomeEndpointRes(raw) // restore from indirection with type conversion!
	return nil
}

// esphomeBridgeConfigure reads the connection info that the named endpoint
// resource published (if any) and passes it into the shared session. A missing
// or unpublished endpoint configures the session with nil, which disconnects
// it, and returns nil info without an error.
func esphomeBridgeConfigure(ctx context.Context, init *engine.Init, session *esphomeUtil.Session, endpoint string) (*esphomeUtil.ConnInfo, error) {
	val, err := init.Local.BridgeGet(ctx, esphomeUtil.BridgeNamespace, endpoint)
	if err != nil {
		return nil, err
	}
	info, ok := val.(*esphomeUtil.ConnInfo)
	if !ok || info == nil {
		session.Configure(nil)
		return nil, nil
	}
	session.Configure(info)
	return info, nil
}

// esphomeWatch is the shared Watch mainloop of the esphome entity resources. It
// forwards both the bridge events (the endpoint resource publishing or
// unpublishing its connection info) and the session events (entity states
// changing, connects and disconnects) as resource events, which is what lets
// mgmt repair out-of-band changes made directly on the device.
func esphomeWatch(ctx context.Context, init *engine.Init, session *esphomeUtil.Session, endpoint string) error {
	bridgeCh, err := init.Local.BridgeWatch(ctx, esphomeUtil.BridgeNamespace, endpoint)
	if err != nil {
		return err
	}
	sessionCh, err := session.Watch(ctx)
	if err != nil {
		return err
	}

	if err := init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case _, ok := <-bridgeCh:
			if !ok {
				return nil
			}
			if _, err := esphomeBridgeConfigure(ctx, init, session, endpoint); err != nil {
				return err
			}

		case _, ok := <-sessionCh:
			if !ok {
				return nil
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := init.Event(ctx); err != nil {
			return err
		}
	}
}

// esphomeSessionReady configures the session from the bridge and waits for the
// device to be healthy. It errors if the endpoint isn't published or the device
// can't be reached, which makes the engine retry as per the resource retry
// metaparams.
func esphomeSessionReadyInfo(ctx context.Context, init *engine.Init, session *esphomeUtil.Session, endpoint string) (*esphomeUtil.ConnInfo, error) {
	info, err := esphomeBridgeConfigure(ctx, init, session, endpoint)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("endpoint `%s` is not available yet", endpoint)
	}
	if err := session.WaitConnected(ctx, esphomeConnectWait); err != nil {
		return nil, errwrap.Wrapf(err, "device `%s` is not connected", endpoint)
	}
	return info, nil
}

func esphomeSessionReady(ctx context.Context, init *engine.Init, session *esphomeUtil.Session, endpoint string) error {
	_, err := esphomeSessionReadyInfo(ctx, init, session, endpoint)
	return err
}

// EsphomeSwitchRes manages a switch entity on an esphome device, such as a gpio
// output pin, a relay, or an led. The name is the exact ESPHome entity name (or
// a legacy object_id), unless the id field overrides it. Because we subscribe
// to the device state, an out-of-band change (eg: someone toggling the switch
// from home assistant or the device web ui) generates an event, and mgmt
// repairs it.
type EsphomeSwitchRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Endpoint is the name of the esphome:endpoint resource which knows
	// how to connect to the device that this entity lives on.
	Endpoint string `lang:"endpoint" yaml:"endpoint"`

	// State is the desired state of the switch. It must be either `on` or
	// `off`.
	State string `lang:"state" yaml:"state"`

	// Id is the exact entity name or legacy object_id of the switch on the
	// device. It defaults to the name of this resource.
	Id string `lang:"id" yaml:"id"`

	session *esphomeUtil.Session
}

// getId returns the identifier of the entity we manage.
func (obj *EsphomeSwitchRes) getId() string {
	if obj.Id != "" {
		return obj.Id
	}
	return obj.Name()
}

// Default returns some sensible defaults for this resource.
func (obj *EsphomeSwitchRes) Default() engine.Res {
	return &EsphomeSwitchRes{}
}

// Validate if the params passed in are valid data.
func (obj *EsphomeSwitchRes) Validate() error {
	if obj.Endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}
	if obj.State != esphomeStateOn && obj.State != esphomeStateOff {
		return fmt.Errorf("state must be `%s` or `%s`", esphomeStateOn, esphomeStateOff)
	}
	if obj.getId() == "" {
		return fmt.Errorf("empty id")
	}
	return nil
}

// Init runs some startup code for this resource. Reserving the shared session
// is only in-memory bookkeeping: nothing connects until the endpoint resource
// publishes its connection info and someone passes it in.
func (obj *EsphomeSwitchRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.session = esphomeUtil.SessionReserve(obj.Endpoint)

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *EsphomeSwitchRes) Cleanup() error {
	if obj.session != nil {
		obj.session.Release()
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *EsphomeSwitchRes) Watch(ctx context.Context) error {
	return esphomeWatch(ctx, obj.init, obj.session, obj.Endpoint)
}

// CheckApply checks the cached entity state and commands the switch if needed.
func (obj *EsphomeSwitchRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if err := esphomeSessionReady(ctx, obj.init, obj.session, obj.Endpoint); err != nil {
		return false, err
	}

	desired := obj.State == esphomeStateOn

	state := obj.session.State(obj.getId())
	if state != nil && state.Domain == esphomeUtil.DomainSwitch && !state.Missing && state.Bool == desired {
		return true, nil // state is good
	}

	if !apply {
		return false, nil
	}

	obj.init.Logf("turning %s", obj.State)
	if err := obj.session.SetSwitch(ctx, obj.getId(), desired); err != nil {
		return false, err
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *EsphomeSwitchRes) Cmp(r engine.Res) error {
	res, ok := r.(*EsphomeSwitchRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Endpoint != res.Endpoint {
		return fmt.Errorf("the Endpoint differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Id != res.Id {
		return fmt.Errorf("the Id differs")
	}
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *EsphomeSwitchRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes EsphomeSwitchRes // indirection to avoid infinite recursion

	def := obj.Default()               // get the default
	res, ok := def.(*EsphomeSwitchRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to EsphomeSwitchRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = EsphomeSwitchRes(raw) // restore from indirection with type conversion!
	return nil
}

// EsphomeNumberRes manages a number entity on an esphome device, such as a
// setpoint or a motor speed. The name is the exact ESPHome entity name (or a
// legacy object_id), unless the id field overrides it.
//
// Because a number often drives something physical, this resource has an
// optional safety interlock for when the device becomes disconnected from us:
// see the stop and safe fields. Note that mgmt can only act while it is running
// and connected, so for full protection against a runaway load you must also
// configure a failsafe in the device firmware itself, for example an esphome
// script that stops the motor when `api.connected` has been false for too long.
// The device natively detects a dropped api connection quickly, so that pairs
// well with this resource.
type EsphomeNumberRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Endpoint is the name of the esphome:endpoint resource which knows
	// how to connect to the device that this entity lives on.
	Endpoint string `lang:"endpoint" yaml:"endpoint"`

	// Value is the desired value of the number entity.
	Value float64 `lang:"value" yaml:"value"`

	// Id is the exact entity name or legacy object_id of the number entity on
	// the device. It defaults to the name of this resource.
	Id string `lang:"id" yaml:"id"`

	// Stop enables the safety interlock when set to a positive number of
	// seconds. If the device was disconnected from us for at least this
	// long, then when the connection comes back, we first command the safe
	// value, and error instead of converging. With a retry metaparam the
	// resource then recovers and re-applies the desired value on the next
	// try; without one it stays safely stopped until a new graph runs.
	// When this is set, we also command the safe value when this resource
	// is removed. When using a polling endpoint, this must be comfortably
	// larger than the polling interval, since we only ever control the
	// device for a moment during each poll.
	Stop uint32 `lang:"stop" yaml:"stop"`

	// Safe is the value that the safety interlock commands. This is
	// usually the value which stops whatever the number drives.
	Safe float64 `lang:"safe" yaml:"safe"`

	session *esphomeUtil.Session
	// cleanupInfo keeps the last successfully used endpoint configuration so
	// cleanup can reconnect long enough to apply the safe value even when the
	// endpoint resource is removed first during a graph shutdown. The engine
	// serializes CheckApply and Cleanup for one resource, which protects this
	// field without a separate mutex. It can be stale after an unconfirmed
	// endpoint change, so the ordered live session is always preferred.
	cleanupInfo *esphomeUtil.ConnInfo

	// outageID remembers the last outage that we already handled, so that
	// each outage triggers the interlock at most once.
	outageID uint64
}

// getId returns the identifier of the entity we manage.
func (obj *EsphomeNumberRes) getId() string {
	if obj.Id != "" {
		return obj.Id
	}
	return obj.Name()
}

// Default returns some sensible defaults for this resource.
func (obj *EsphomeNumberRes) Default() engine.Res {
	return &EsphomeNumberRes{}
}

// Validate if the params passed in are valid data.
func (obj *EsphomeNumberRes) Validate() error {
	if obj.Endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}
	if obj.getId() == "" {
		return fmt.Errorf("empty id")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *EsphomeNumberRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.session = esphomeUtil.SessionReserve(obj.Endpoint)

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. If the
// safety interlock is enabled, we make a best effort to leave the entity at the
// safe value, since nobody will be managing it from now on.
func (obj *EsphomeNumberRes) Cleanup() error {
	if obj.session == nil {
		return nil
	}
	if obj.Stop > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), esphomeCleanupTimeout)
		defer cancel()
		if err := obj.session.SetNumberForCleanup(ctx, obj.cleanupInfo, obj.getId(), obj.Safe); err != nil {
			obj.init.Logf("could not apply the safe value on cleanup: %v", err)
		}
	}
	obj.session.Release()
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *EsphomeNumberRes) Watch(ctx context.Context) error {
	return esphomeWatch(ctx, obj.init, obj.session, obj.Endpoint)
}

// CheckApply checks the cached entity state and commands the number if needed.
// If the safety interlock is enabled and the device just came back from an
// outage which lasted too long, it commands the safe value and errors instead.
func (obj *EsphomeNumberRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	info, err := esphomeSessionReadyInfo(ctx, obj.init, obj.session, obj.Endpoint)
	if err != nil {
		return false, err
	}
	obj.cleanupInfo = info

	if obj.Stop > 0 {
		outage, id := obj.session.LastOutage()
		if id != obj.outageID && outage >= time.Duration(obj.Stop)*time.Second {
			if !apply {
				return false, nil
			}
			if err := obj.session.SetNumber(ctx, obj.getId(), obj.Safe); err != nil {
				return false, errwrap.Wrapf(err, "could not apply the safe value after an outage")
			}
			obj.outageID = id // each outage triggers at most once
			return false, fmt.Errorf("device was disconnected for %.1fs (stop is %ds), applied safe value %v", outage.Seconds(), obj.Stop, obj.Safe)
		}
		obj.outageID = id // a benign outage, or one we already handled
	}

	state := obj.session.State(obj.getId())
	if state != nil && state.Domain == esphomeUtil.DomainNumber && !state.Missing && state.Float == obj.Value {
		return true, nil // state is good
	}

	if !apply {
		return false, nil
	}

	obj.init.Logf("setting value to %v", obj.Value)
	if err := obj.session.SetNumber(ctx, obj.getId(), obj.Value); err != nil {
		return false, err
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *EsphomeNumberRes) Cmp(r engine.Res) error {
	res, ok := r.(*EsphomeNumberRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Endpoint != res.Endpoint {
		return fmt.Errorf("the Endpoint differs")
	}
	if obj.Value != res.Value {
		return fmt.Errorf("the Value differs")
	}
	if obj.Id != res.Id {
		return fmt.Errorf("the Id differs")
	}
	if obj.Stop != res.Stop {
		return fmt.Errorf("the Stop differs")
	}
	if obj.Safe != res.Safe {
		return fmt.Errorf("the Safe differs")
	}
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *EsphomeNumberRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes EsphomeNumberRes // indirection to avoid infinite recursion

	def := obj.Default()               // get the default
	res, ok := def.(*EsphomeNumberRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to EsphomeNumberRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = EsphomeNumberRes(raw) // restore from indirection with type conversion!
	return nil
}

// EsphomeFanRes manages an esphome fan entity. The hbridge fan platform makes
// this a useful abstraction for a reversible DC motor such as the conveyor
// demo. The name is the exact esphome entity name (or a legacy object_id),
// unless id overrides it.
//
// Stop is an mgmt-side recovery and cleanup interlock. It is not a substitute
// for a local firmware timeout, a current limit, guards, or a physical e-stop.
type EsphomeFanRes struct {
	traits.Base // add the base methods without re-implementation

	// init is used by the engine to pass in the internal structure.
	init *engine.Init

	// Endpoint is the name of the esphome:endpoint resource which knows how
	// to connect to the device that this entity lives on.
	Endpoint string `lang:"endpoint" yaml:"endpoint"`

	// State is the desired state of the fan. It must be on or off.
	State string `lang:"state" yaml:"state"`

	// Speed is the desired discrete speed level. CheckApply validates it
	// against the fan's advertised supported_speed_count.
	Speed int32 `lang:"speed" yaml:"speed"`

	// Direction is the desired fan direction. CheckApply errors clearly when
	// the entity does not advertise direction support.
	Direction string `lang:"direction" yaml:"direction"`

	// Id is the exact entity name or legacy object_id of the fan on the
	// device. It defaults to the name of this resource.
	Id string `lang:"id" yaml:"id"`

	// Stop enables the safety interlock when set to a positive number of
	// seconds. If the device was disconnected from us for at least this long,
	// then when the connection comes back, we first stop the fan and error
	// instead of converging. With a retry metaparam the resource then recovers
	// and re-applies the desired state on the next try; without one it stays
	// safely stopped until a new graph runs. When this is set, we also stop the
	// fan when this resource is removed. When using a polling endpoint, this
	// must be comfortably larger than the polling interval, since we only ever
	// control the device for a moment during each poll.
	Stop uint32 `lang:"stop" yaml:"stop"`

	// session is the shared connection to the endpoint.
	session *esphomeUtil.Session

	// outageID remembers the last outage that we already handled.
	outageID uint64
	// cleanupInfo keeps the last successfully used endpoint configuration so
	// cleanup can reconnect long enough to stop the fan even when the endpoint
	// resource is removed first during a graph shutdown. The engine serializes
	// CheckApply and Cleanup for one resource, which protects this field without
	// a separate mutex. It can be stale after an unconfirmed endpoint change, so
	// the ordered live session is always preferred.
	cleanupInfo *esphomeUtil.ConnInfo
}

// getId returns the identifier of the entity we manage.
func (obj *EsphomeFanRes) getId() string {
	if obj.Id != "" {
		return obj.Id
	}
	return obj.Name()
}

// command returns the desired native fan command. Stop commands deliberately
// omit optional capabilities so any managed fan can be stopped.
func (obj *EsphomeFanRes) command(on bool) esphomeUtil.FanCommand {
	return esphomeUtil.FanCommand{
		State: on, Speed: obj.Speed, Direction: obj.Direction,
		HasSpeed: on, HasDirection: on,
	}
}

// Default returns some sensible defaults for this resource.
func (obj *EsphomeFanRes) Default() engine.Res {
	return &EsphomeFanRes{Speed: 100, Direction: esphomeUtil.FanDirectionForward}
}

// Validate if the params passed in are valid data.
func (obj *EsphomeFanRes) Validate() error {
	if obj.Endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}
	if obj.State != esphomeStateOn && obj.State != esphomeStateOff {
		return fmt.Errorf("state must be `%s` or `%s`", esphomeStateOn, esphomeStateOff)
	}
	if obj.Speed < 1 || obj.Speed > 100 {
		return fmt.Errorf("speed must be between 1 and 100")
	}
	if obj.Direction != esphomeUtil.FanDirectionForward && obj.Direction != esphomeUtil.FanDirectionReverse {
		return fmt.Errorf("direction must be `%s` or `%s`", esphomeUtil.FanDirectionForward, esphomeUtil.FanDirectionReverse)
	}
	if obj.getId() == "" {
		return fmt.Errorf("empty id")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *EsphomeFanRes) Init(init *engine.Init) error {
	obj.init = init
	obj.session = esphomeUtil.SessionReserve(obj.Endpoint)
	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. If the
// safety interlock is enabled, we make a best effort to leave the fan stopped.
func (obj *EsphomeFanRes) Cleanup() error {
	if obj.session == nil {
		return nil
	}
	if obj.Stop > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), esphomeCleanupTimeout)
		defer cancel()
		if err := obj.session.SetFanForCleanup(ctx, obj.cleanupInfo, obj.getId(), obj.command(false)); err != nil {
			obj.init.Logf("could not stop the fan on cleanup: %v", err)
		}
	}
	obj.session.Release()
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *EsphomeFanRes) Watch(ctx context.Context) error {
	return esphomeWatch(ctx, obj.init, obj.session, obj.Endpoint)
}

// CheckApply checks the cached entity state and commands the fan if needed. If
// the safety interlock detects a long outage, it stops the fan and errors.
func (obj *EsphomeFanRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	info, err := esphomeSessionReadyInfo(ctx, obj.init, obj.session, obj.Endpoint)
	if err != nil {
		return false, err
	}
	obj.cleanupInfo = info

	if obj.Stop > 0 {
		outage, id := obj.session.LastOutage()
		if id != obj.outageID && outage >= time.Duration(obj.Stop)*time.Second {
			if !apply {
				return false, nil
			}
			if err := obj.session.SetFan(ctx, obj.getId(), obj.command(false)); err != nil {
				return false, errwrap.Wrapf(err, "could not stop the fan after an outage")
			}
			obj.outageID = id
			return false, fmt.Errorf("device was disconnected for %.1fs (stop is %ds), stopped fan", outage.Seconds(), obj.Stop)
		}
		obj.outageID = id
	}

	desired := obj.State == esphomeStateOn
	command := obj.command(desired)
	if err := obj.session.ValidateFanCommand(obj.getId(), command); err != nil {
		return false, err
	}
	state := obj.session.State(obj.getId())
	if state != nil && state.Domain == esphomeUtil.DomainFan && !state.Missing &&
		state.Bool == desired && (!desired || (state.Speed == obj.Speed && state.Direction == obj.Direction)) {
		return true, nil
	}
	if !apply {
		return false, nil
	}

	obj.init.Logf("turning fan %s at speed %d in the %s direction", obj.State, obj.Speed, obj.Direction)
	if err := obj.session.SetFan(ctx, obj.getId(), command); err != nil {
		return false, err
	}
	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *EsphomeFanRes) Cmp(r engine.Res) error {
	res, ok := r.(*EsphomeFanRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	if obj.Endpoint != res.Endpoint {
		return fmt.Errorf("the Endpoint differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Speed != res.Speed {
		return fmt.Errorf("the Speed differs")
	}
	if obj.Direction != res.Direction {
		return fmt.Errorf("the Direction differs")
	}
	if obj.Id != res.Id {
		return fmt.Errorf("the Id differs")
	}
	if obj.Stop != res.Stop {
		return fmt.Errorf("the Stop differs")
	}
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *EsphomeFanRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes EsphomeFanRes
	res, ok := obj.Default().(*EsphomeFanRes)
	if !ok {
		return fmt.Errorf("could not convert to EsphomeFanRes")
	}
	raw := rawRes(*res)
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*obj = EsphomeFanRes(raw)
	return nil
}

// EsphomeLightRes manages an RGB esphome light. Color accepts a small stable
// vocabulary of names or an exact #RRGGBB value; parseEsphomeColor is kept
// independent so this contract is straightforward to test.
type EsphomeLightRes struct {
	traits.Base // add the base methods without re-implementation

	// init is used by the engine to pass in the internal structure.
	init *engine.Init

	// Endpoint is the name of the esphome:endpoint resource which knows how
	// to connect to the device that this entity lives on.
	Endpoint string `lang:"endpoint" yaml:"endpoint"`

	// State is the desired state of the light. It must be on or off.
	State string `lang:"state" yaml:"state"`

	// Brightness is the desired normalized brightness in the range 0..1.
	Brightness float64 `lang:"brightness" yaml:"brightness"`

	// Color is a supported name or an exact #RRGGBB value. The entity must
	// advertise RGB color-mode support when the desired state is on.
	Color string `lang:"color" yaml:"color"`

	// Id is the exact entity name or legacy object_id of the light on the
	// device. It defaults to the name of this resource.
	Id string `lang:"id" yaml:"id"`

	// session is the shared connection to the endpoint.
	session *esphomeUtil.Session
}

// getId returns the identifier of the entity we manage.
func (obj *EsphomeLightRes) getId() string {
	if obj.Id != "" {
		return obj.Id
	}
	return obj.Name()
}

// parseEsphomeColor converts a stable color name or #RRGGBB value to native
// normalized RGB channels.
func parseEsphomeColor(value string) (float64, float64, float64, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	rgb, ok := esphomeNamedColors[normalized]
	if !ok {
		if len(normalized) != 7 || normalized[0] != '#' {
			return 0, 0, 0, fmt.Errorf("color must be a supported name or #RRGGBB")
		}
		var err error
		rgb, err = strconv.ParseUint(normalized[1:], 16, 24)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid color %q: %v", value, err)
		}
	}
	// Round through float32 because that is the native api wire type. This
	// makes the desired value exactly comparable with the received state.
	channel := func(shift uint) float64 {
		return float64(float32(float64((rgb>>shift)&0xff) / 255))
	}
	return channel(16), channel(8), channel(0), nil
}

// command returns the desired native light command. Off commands deliberately
// omit optional color capabilities so any managed light can be turned off.
func (obj *EsphomeLightRes) command(on bool) (esphomeUtil.LightCommand, error) {
	red, green, blue, err := parseEsphomeColor(obj.Color)
	if err != nil {
		return esphomeUtil.LightCommand{}, err
	}
	return esphomeUtil.LightCommand{
		State: on, Brightness: float64(float32(obj.Brightness)),
		Red: red, Green: green, Blue: blue,
		HasBrightness: on, HasRGB: on,
	}, nil
}

// Default returns some sensible defaults for this resource.
func (obj *EsphomeLightRes) Default() engine.Res {
	return &EsphomeLightRes{Brightness: 1, Color: "white"}
}

// Validate if the params passed in are valid data.
func (obj *EsphomeLightRes) Validate() error {
	if obj.Endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}
	if obj.State != esphomeStateOn && obj.State != esphomeStateOff {
		return fmt.Errorf("state must be `%s` or `%s`", esphomeStateOn, esphomeStateOff)
	}
	if math.IsNaN(obj.Brightness) || math.IsInf(obj.Brightness, 0) || obj.Brightness < 0 || obj.Brightness > 1 {
		return fmt.Errorf("brightness must be between 0 and 1")
	}
	if _, _, _, err := parseEsphomeColor(obj.Color); err != nil {
		return err
	}
	if obj.getId() == "" {
		return fmt.Errorf("empty id")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *EsphomeLightRes) Init(init *engine.Init) error {
	obj.init = init
	obj.session = esphomeUtil.SessionReserve(obj.Endpoint)
	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *EsphomeLightRes) Cleanup() error {
	if obj.session != nil {
		obj.session.Release()
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *EsphomeLightRes) Watch(ctx context.Context) error {
	return esphomeWatch(ctx, obj.init, obj.session, obj.Endpoint)
}

// CheckApply checks the cached entity state and commands the light if needed.
func (obj *EsphomeLightRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if err := esphomeSessionReady(ctx, obj.init, obj.session, obj.Endpoint); err != nil {
		return false, err
	}
	desired := obj.State == esphomeStateOn
	command, err := obj.command(desired)
	if err != nil {
		return false, err
	}
	if err := obj.session.ValidateLightCommand(obj.getId(), command); err != nil {
		return false, err
	}
	state := obj.session.State(obj.getId())
	if state != nil && state.Domain == esphomeUtil.DomainLight && !state.Missing && state.Bool == desired &&
		(!desired || (state.Brightness == command.Brightness && state.Red == command.Red &&
			state.Green == command.Green && state.Blue == command.Blue)) {
		return true, nil
	}
	if !apply {
		return false, nil
	}

	obj.init.Logf("turning light %s at brightness %v with color %s", obj.State, obj.Brightness, obj.Color)
	if err := obj.session.SetLight(ctx, obj.getId(), command); err != nil {
		return false, err
	}
	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *EsphomeLightRes) Cmp(r engine.Res) error {
	res, ok := r.(*EsphomeLightRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	if obj.Endpoint != res.Endpoint {
		return fmt.Errorf("the Endpoint differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Brightness != res.Brightness {
		return fmt.Errorf("the Brightness differs")
	}
	if obj.Color != res.Color {
		return fmt.Errorf("the Color differs")
	}
	if obj.Id != res.Id {
		return fmt.Errorf("the Id differs")
	}
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *EsphomeLightRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes EsphomeLightRes
	res, ok := obj.Default().(*EsphomeLightRes)
	if !ok {
		return fmt.Errorf("could not convert to EsphomeLightRes")
	}
	raw := rawRes(*res)
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*obj = EsphomeLightRes(raw)
	return nil
}
