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

package esphome

import (
	"context"
	"fmt"
	"math"
	"strings"

	apiclient "github.com/flavio-fernandes/go-aioesphomeapi"
	"github.com/flavio-fernandes/go-aioesphomeapi/pb"
	"google.golang.org/protobuf/proto"
)

// apiClientDriver implements the driver interface with the esphome-apiclient
// library. We deliberately leave the library's built-in reconnect disabled,
// since the session owns the reconnect and polling logic.
type apiClientDriver struct {
	client *apiclient.Client
}

// newAPIClientDriver builds an unconnected driver.
func newAPIClientDriver() driver {
	return &apiClientDriver{}
}

// connect dials the device and performs the handshake.
func (obj *apiClientDriver) connect(ctx context.Context, info *ConnInfo) error {
	if info.Password != "" {
		// The legacy ConnectRequest auth flow was removed from esphome
		// in 2026.1, and our driver doesn't implement it either.
		return fmt.Errorf("legacy password auth is not supported, use the noise key")
	}
	if info.Key == "" {
		return fmt.Errorf("empty noise encryption key")
	}

	opts := []apiclient.Option{
		apiclient.WithClientInfo("mgmt"),
	}
	opts = append(opts, apiclient.WithEncryptionKey(info.Key))

	client, err := apiclient.DialWithContext(ctx, info.Addr(), dialTimeout, opts...)
	if err != nil {
		return err
	}
	obj.client = client
	return nil
}

// entities lists the entities that the device advertises.
func (obj *apiClientDriver) entities() ([]*EntityInfo, error) {
	if _, err := obj.client.ListEntities(); err != nil { // fills the registry
		return nil, err
	}
	registry := obj.client.Entities()

	result := []*EntityInfo{}
	for _, e := range registry.BinarySensors() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainBinarySensor})
	}
	for _, e := range registry.Sensors() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainSensor})
	}
	for _, e := range registry.TextSensors() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainTextSensor})
	}
	for _, e := range registry.Switches() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainSwitch})
	}
	for _, e := range registry.Numbers() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainNumber})
	}
	for _, e := range registry.Buttons() {
		result = append(result, &EntityInfo{Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainButton})
	}
	for _, e := range registry.Fans() {
		result = append(result, &EntityInfo{
			Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainFan,
			FanSupportsSpeed: e.SupportsSpeed, FanSupportedSpeedCount: e.SupportedSpeedCount,
			FanSupportsDirection: e.SupportsDirection,
		})
	}
	for _, e := range registry.Lights() {
		modes := make([]string, 0, len(e.SupportedColorModes))
		for _, mode := range e.SupportedColorModes {
			modes = append(modes, mode.String())
		}
		result = append(result, &EntityInfo{
			Key: e.Key, ObjectID: e.ObjectID, Name: e.Name, Domain: DomainLight,
			LightSupportedColorModes: modes,
		})
	}
	return result, nil
}

// subscribe asks the device to push state updates. The initial snapshot of
// every entity arrives right away, and changes stream in after that. A sensor
// value of NaN means the device doesn't have a valid reading, so we map it to
// the missing flag.
func (obj *apiClientDriver) subscribe(fn func(*EntityState)) error {
	_, err := obj.client.SubscribeStates(func(msg proto.Message) {
		switch m := msg.(type) {
		case *pb.BinarySensorStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain:  DomainBinarySensor,
				Bool:    m.State,
				Missing: m.MissingState,
			}})

		case *pb.SensorStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain:  DomainSensor,
				Float:   float64(m.State),
				Missing: m.MissingState || math.IsNaN(float64(m.State)),
			}})

		case *pb.TextSensorStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain:  DomainTextSensor,
				Str:     m.State,
				Missing: m.MissingState,
			}})

		case *pb.SwitchStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain: DomainSwitch,
				Bool:   m.State,
			}})

		case *pb.NumberStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain:  DomainNumber,
				Float:   float64(m.State),
				Missing: m.MissingState || math.IsNaN(float64(m.State)),
			}})

		case *pb.FanStateResponse:
			direction := FanDirectionForward
			if m.Direction == pb.FanDirection_FAN_DIRECTION_REVERSE {
				direction = FanDirectionReverse
			}
			fn(&EntityState{Key: m.Key, State: State{
				Domain:    DomainFan,
				Bool:      m.State,
				Speed:     m.SpeedLevel,
				Direction: direction,
			}})

		case *pb.LightStateResponse:
			fn(&EntityState{Key: m.Key, State: State{
				Domain:     DomainLight,
				Bool:       m.State,
				Brightness: float64(m.Brightness),
				Red:        float64(m.Red),
				Green:      float64(m.Green),
				Blue:       float64(m.Blue),
			}})
		}
	})
	return err
}

// subscribeLogs asks the device to stream its native logger output.
func (obj *apiClientDriver) subscribeLogs(level string, fn func(*LogEntry)) error {
	levels := map[string]pb.LogLevel{
		LogLevelError:       pb.LogLevel_LOG_LEVEL_ERROR,
		LogLevelWarn:        pb.LogLevel_LOG_LEVEL_WARN,
		LogLevelInfo:        pb.LogLevel_LOG_LEVEL_INFO,
		LogLevelConfig:      pb.LogLevel_LOG_LEVEL_CONFIG,
		LogLevelDebug:       pb.LogLevel_LOG_LEVEL_DEBUG,
		LogLevelVerbose:     pb.LogLevel_LOG_LEVEL_VERBOSE,
		LogLevelVeryVerbose: pb.LogLevel_LOG_LEVEL_VERY_VERBOSE,
	}
	pbLevel, exists := levels[level]
	if !exists {
		return fmt.Errorf("invalid log level: %s", level)
	}

	_, err := obj.client.SubscribeLogs(pbLevel, func(msg *pb.SubscribeLogsResponse) {
		name := strings.TrimPrefix(strings.ToLower(msg.Level.String()), "log_level_")
		fn(&LogEntry{
			Level:   name,
			Message: strings.TrimRight(string(msg.Message), "\r\n"),
		})
	})
	return err
}

// done returns a channel which closes when the connection dies.
func (obj *apiClientDriver) done() <-chan struct{} {
	return obj.client.Done()
}

// closeReason reports why the connection ended, or nil after a deliberate
// close. The library records the first sanitized, typed cause, so wrapping it
// keeps errors.Is and errors.As working and never exposes the noise key.
func (obj *apiClientDriver) closeReason() error {
	return obj.client.CloseReason()
}

// close tears the connection down.
func (obj *apiClientDriver) close() error {
	return obj.client.Close()
}

// setSwitch commands a switch entity by key.
func (obj *apiClientDriver) setSwitch(key uint32, on bool) error {
	return obj.client.SetSwitch(key, on)
}

// setNumber commands a number entity by key.
func (obj *apiClientDriver) setNumber(key uint32, value float64) error {
	return obj.client.SetNumber(key, float32(value))
}

// pressButton presses a button entity by key.
func (obj *apiClientDriver) pressButton(key uint32) error {
	return obj.client.PressButton(key)
}

func (obj *apiClientDriver) setFan(key uint32, command FanCommand) error {
	direction := pb.FanDirection_FAN_DIRECTION_FORWARD
	if command.Direction == FanDirectionReverse {
		direction = pb.FanDirection_FAN_DIRECTION_REVERSE
	}
	return obj.client.SetFan(key, apiclient.FanCommandOpts{
		HasState:      true,
		State:         command.State,
		HasSpeedLevel: command.HasSpeed,
		SpeedLevel:    command.Speed,
		HasDirection:  command.HasDirection,
		Direction:     direction,
	})
}

func (obj *apiClientDriver) setLight(key uint32, command LightCommand) error {
	return obj.client.SetLight(key, apiclient.LightCommandOpts{
		HasState:      true,
		State:         command.State,
		HasBrightness: command.HasBrightness,
		Brightness:    float32(command.Brightness),
		HasColorMode:  command.HasRGB,
		ColorMode:     pb.ColorMode_COLOR_MODE_RGB,
		HasRGB:        command.HasRGB,
		Red:           float32(command.Red),
		Green:         float32(command.Green),
		Blue:          float32(command.Blue),
	})
}
