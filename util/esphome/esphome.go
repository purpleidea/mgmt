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

// Package esphome is a client library for the esphome native api that is shared
// between the esphome resources and the esphome functions. It pools device
// connections so that any number of concurrent consumers which name the same
// endpoint uid share a single session, and as a result, a single real
// connection to the device. The wire protocol implementation is hidden behind a
// small internal driver interface so that the underlying library can be swapped
// out without changing any of the consumers.
package esphome

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

const (
	// BridgeNamespace is the namespace under which the esphome:endpoint
	// resource publishes its connection info with the local Bridge API. It
	// must match the kind string of that resource.
	BridgeNamespace = "esphome:endpoint"

	// DefaultPort is the default port of the esphome native api.
	DefaultPort = 6053

	// DomainBinarySensor is the entity domain for read-only boolean
	// entities such as gpio inputs and buttons wired as binary sensors.
	DomainBinarySensor = "binary_sensor"

	// DomainSensor is the entity domain for read-only numeric entities
	// such as adc inputs and temperature sensors.
	DomainSensor = "sensor"

	// DomainTextSensor is the entity domain for read-only string entities.
	DomainTextSensor = "text_sensor"

	// DomainSwitch is the entity domain for boolean outputs such as gpio
	// output pins and relays.
	DomainSwitch = "switch"

	// DomainNumber is the entity domain for writable numeric entities such
	// as setpoints and speeds.
	DomainNumber = "number"

	// DomainButton is the entity domain for momentary press-only entities.
	DomainButton = "button"

	// DomainFan is the entity domain for fans, including H-bridge motor
	// controllers exposed through esphome's fan abstraction.
	DomainFan = "fan"

	// DomainLight is the entity domain for lights.
	DomainLight = "light"

	// Fan directions used by the native api.
	FanDirectionForward = "forward"
	FanDirectionReverse = "reverse"

	// LightColorModeRGB is the native RGB color mode required by the current
	// light resource.
	LightColorModeRGB = "COLOR_MODE_RGB"

	// Log levels accepted by the esphome native api.
	LogLevelError       = "error"
	LogLevelWarn        = "warn"
	LogLevelInfo        = "info"
	LogLevelConfig      = "config"
	LogLevelDebug       = "debug"
	LogLevelVerbose     = "verbose"
	LogLevelVeryVerbose = "very_verbose"
)

// NormalizeLogLevel returns the canonical form of an esphome log level. An
// empty string disables device log streaming.
func NormalizeLogLevel(level string) (string, error) {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "", LogLevelError, LogLevelWarn, LogLevelInfo, LogLevelConfig,
		LogLevelDebug, LogLevelVerbose, LogLevelVeryVerbose:
		return level, nil
	case "warning":
		return LogLevelWarn, nil
	default:
		return "", fmt.Errorf("invalid log level: %s", level)
	}
}

// LogEntry is one message streamed from the device logger.
type LogEntry struct {
	Level   string
	Message string
}

// ConnInfo is the connection information that the esphome:endpoint resource
// publishes over the local Bridge API for the functions and other resources to
// consume. Everyone must treat a published value as immutable.
type ConnInfo struct {
	// Host is the ip address or hostname of the esphome device.
	Host string

	// Port is the port of the esphome native api. This is usually 6053.
	Port int

	// Key is the base64 encoded noise encryption key of the device. It is
	// required so callers cannot silently downgrade to plaintext.
	Key string

	// Password is the legacy api password. This auth mechanism was
	// deprecated and then removed from esphome, and it is currently not
	// supported by our driver. Prefer Key.
	Password string

	// Interval is how we watch for events. Zero means we hold a persistent
	// connection open, over which the device pushes state changes to us
	// natively. A positive value means we poll instead: every Interval
	// seconds we connect, read a full state snapshot, send any pending
	// commands, and then disconnect.
	Interval uint32

	// LogLevel enables native device log streaming at this level. Empty
	// disables it.
	LogLevel string

	// Logf receives device log entries. It is supplied by the endpoint
	// resource and deliberately excluded from Cmp: it doesn't affect the
	// wire connection identity.
	Logf func(*LogEntry)

	// ConnectLogf reports connection failures and retry delays. It is
	// supplied by the endpoint resource and deliberately excluded from Cmp:
	// it doesn't affect the wire connection identity.
	ConnectLogf func(format string, v ...interface{})
}

// Addr returns the host:port pair to dial.
func (obj *ConnInfo) Addr() string {
	return fmt.Sprintf("%s:%d", obj.Host, obj.Port)
}

// Validate reports whether the connection info is well formed.
func (obj *ConnInfo) Validate() error {
	if obj == nil {
		return fmt.Errorf("nil conn info")
	}
	if obj.Host == "" {
		return fmt.Errorf("empty host")
	}
	if obj.Port <= 0 || obj.Port > 65535 {
		return fmt.Errorf("invalid port: %d", obj.Port)
	}
	if obj.Key == "" {
		return fmt.Errorf("empty noise encryption key")
	}
	if obj.Key != "" && obj.Password != "" {
		return fmt.Errorf("key and password are mutually exclusive")
	}
	if obj.Key != "" {
		b, err := base64.StdEncoding.DecodeString(obj.Key)
		if err != nil {
			return fmt.Errorf("key is not valid base64: %v", err)
		}
		if len(b) != 32 {
			return fmt.Errorf("key must decode to 32 bytes, got: %d", len(b))
		}
	}
	if level, err := NormalizeLogLevel(obj.LogLevel); err != nil {
		return err
	} else if level != obj.LogLevel {
		return fmt.Errorf("log level must use canonical form: %s", level)
	}
	return nil
}

// Cmp compares two connection infos and returns an error describing the first
// difference if they aren't equivalent.
func (obj *ConnInfo) Cmp(info *ConnInfo) error {
	if obj == nil || info == nil {
		if obj == nil && info == nil {
			return nil
		}
		return fmt.Errorf("one conn info is nil")
	}
	if obj.Host != info.Host {
		return fmt.Errorf("the Host differs")
	}
	if obj.Port != info.Port {
		return fmt.Errorf("the Port differs")
	}
	if obj.Key != info.Key {
		return fmt.Errorf("the Key differs")
	}
	if obj.Password != info.Password {
		return fmt.Errorf("the Password differs")
	}
	if obj.Interval != info.Interval {
		return fmt.Errorf("the Interval differs")
	}
	if obj.LogLevel != info.LogLevel {
		return fmt.Errorf("the LogLevel differs")
	}
	return nil
}

// State is a snapshot of the last-known state of a single entity. Consumers
// must treat it as read-only.
type State struct {
	// Domain is the entity domain, eg: "binary_sensor" or "switch".
	Domain string

	// Bool holds the value for the binary_sensor and switch domains.
	Bool bool

	// Float holds the value for the sensor and number domains.
	Float float64

	// Str holds the value for the text_sensor domain.
	Str string

	// Speed is the discrete speed level of a fan.
	Speed int32

	// Direction is the forward/reverse direction of a fan.
	Direction string

	// Brightness and RGB hold normalized light values in the range 0..1.
	Brightness float64
	Red        float64
	Green      float64
	Blue       float64

	// Missing is true if the device reported this state as unknown.
	Missing bool
}

// FanCommand is the complete desired fan state sent by mgmt.
type FanCommand struct {
	// State is true when the fan should be on.
	State bool

	// Speed is the desired device-defined speed level.
	Speed int32

	// Direction is the desired forward or reverse direction.
	Direction string

	// HasSpeed and HasDirection select the optional capability-specific
	// fields. A stop command leaves both false so any fan can be stopped.
	HasSpeed     bool
	HasDirection bool
}

// LightCommand is the complete desired RGB light state sent by mgmt.
type LightCommand struct {
	// State is true when the light should be on.
	State bool

	// Brightness is the normalized brightness in the range 0..1.
	Brightness float64

	// Red, Green, and Blue are normalized color channels in the range 0..1.
	Red   float64
	Green float64
	Blue  float64

	// HasBrightness and HasRGB select optional capability-specific fields. An
	// off command leaves both false so any light can be turned off.
	HasBrightness bool
	HasRGB        bool
}

// EntityInfo describes one entity that a device advertises.
type EntityInfo struct {
	// Key is the numeric entity key used by the wire protocol.
	Key uint32

	// ObjectID is the legacy object_id. esphome 2026.7 and newer can leave
	// this empty, so consumers should also support Name.
	ObjectID string

	// Name is the entity name and the preferred identifier for current
	// esphome versions.
	Name string

	// Domain is the entity domain, eg: "binary_sensor" or "switch".
	Domain string

	// FanSupportsSpeed reports whether a fan accepts speed commands.
	FanSupportsSpeed bool

	// FanSupportedSpeedCount is the highest discrete speed level accepted
	// by a fan.
	FanSupportedSpeedCount int32

	// FanSupportsDirection reports whether a fan accepts direction commands.
	FanSupportsDirection bool

	// LightSupportedColorModes contains the native color modes advertised by
	// a light.
	LightSupportedColorModes []string
}

// EntityState is one state update as reported by the driver.
type EntityState struct {
	// Key is the numeric entity key used by the wire protocol.
	Key uint32

	// State is the reported state. The Domain field says which of the
	// value fields is meaningful.
	State
}

func init() {
	sessionMutex = &sync.Mutex{}
	sessionMap = make(map[string]*Session)
}

var (
	// sessionMutex is a mutex for locking the sessionMap.
	sessionMutex *sync.Mutex

	// sessionMap is a map from the endpoint uid to the shared session.
	sessionMap map[string]*Session
)

// SessionReserve returns the shared session for the given endpoint uid. It
// creates it on first use. Every call must be paired with exactly one call to
// Release on the returned session. The uid is the name of the corresponding
// esphome:endpoint resource, and the session does not connect anywhere until
// someone passes it that resource's published connection info via Configure.
func SessionReserve(uid string) *Session {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session, exists := sessionMap[uid]
	if !exists {
		session = newSession(uid)
		sessionMap[uid] = session
	}
	session.count++
	return session
}

// Release frees one reservation of this session. When the last reservation is
// released, the session disconnects and is removed from the pool.
func (obj *Session) Release() {
	sessionMutex.Lock()
	obj.count--
	if obj.count < 0 {
		sessionMutex.Unlock()
		// programming error
		panic("session count is negative")
	}
	last := obj.count == 0
	if last {
		delete(sessionMap, obj.uid)
	}
	sessionMutex.Unlock()

	if last {
		obj.cancel()
		obj.wg.Wait() // wait for the mainloop to shutdown
	}
}
