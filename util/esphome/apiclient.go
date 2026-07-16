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

	apiclient "github.com/richard87/esphome-apiclient"
	"github.com/richard87/esphome-apiclient/pb"
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

	opts := []apiclient.Option{
		apiclient.WithClientInfo("mgmt"),
	}
	if info.Key != "" {
		opts = append(opts, apiclient.WithEncryptionKey(info.Key))
	}

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
		}
	})
	return err
}

// done returns a channel which closes when the connection dies.
func (obj *apiClientDriver) done() <-chan struct{} {
	return obj.client.Done()
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
