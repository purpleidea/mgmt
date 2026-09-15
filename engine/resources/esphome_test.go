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

//go:build !root

package resources

import (
	"testing"

	esphomeUtil "github.com/purpleidea/mgmt/util/esphome"
)

func TestParseEsphomeColour(t *testing.T) {
	tests := map[string][3]float64{
		"red":     {1, 0, 0},
		" GREEN ": {0, 1, 0},
		"#0000ff": {0, 0, 1},
		"#804020": {
			float64(float32(128.0 / 255)),
			float64(float32(64.0 / 255)),
			float64(float32(32.0 / 255)),
		},
	}
	for input, want := range tests {
		red, green, blue, err := parseEsphomeColour(input)
		if err != nil {
			t.Fatalf("parseEsphomeColour(%q): %v", input, err)
		}
		if got := [3]float64{red, green, blue}; got != want {
			t.Fatalf("parseEsphomeColour(%q) = %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"", "chartreuse", "#12345", "#gg0000"} {
		if _, _, _, err := parseEsphomeColour(input); err == nil {
			t.Fatalf("parseEsphomeColour(%q) unexpectedly succeeded", input)
		}
	}
}

func TestEsphomeFanValidate(t *testing.T) {
	res := (&EsphomeFanRes{}).Default().(*EsphomeFanRes)
	res.SetName("Conveyor Motor")
	res.Endpoint = "conveyor"
	res.State = esphomeStateOn
	if err := res.Validate(); err != nil {
		t.Fatalf("valid fan: %v", err)
	}

	for _, speed := range []int32{0, 101, -101} {
		res.Speed = speed
		if err := res.Validate(); err == nil {
			t.Fatalf("fan speed %d unexpectedly validated", speed)
		}
	}

	// The sign of the speed is the direction, so a negative speed must
	// command the same level as its positive twin, but the other way.
	res.Speed = 50
	if err := res.Validate(); err != nil {
		t.Fatalf("valid forward fan: %v", err)
	}
	forward := res.command(true)
	res.Speed = -50
	if err := res.Validate(); err != nil {
		t.Fatalf("valid reverse fan: %v", err)
	}
	reverse := res.command(true)

	if forward.Speed != 50 || forward.Direction != esphomeUtil.FanDirectionForward {
		t.Fatalf("speed 50 gave %d in the %s direction", forward.Speed, forward.Direction)
	}
	if reverse.Speed != 50 || reverse.Direction != esphomeUtil.FanDirectionReverse {
		t.Fatalf("speed -50 gave %d in the %s direction", reverse.Speed, reverse.Direction)
	}
}

func TestEsphomeLightValidateAndCommand(t *testing.T) {
	res := (&EsphomeLightRes{}).Default().(*EsphomeLightRes)
	res.SetName("Status Light")
	res.Endpoint = "conveyor"
	res.State = esphomeStateOn
	res.Brightness = 0.5
	res.Colour = "#ff8000"
	if err := res.Validate(); err != nil {
		t.Fatalf("valid light: %v", err)
	}
	command, err := res.command(true)
	if err != nil {
		t.Fatalf("light command: %v", err)
	}
	// An empty effect asks for the device's own name for "no effect", since
	// the empty string is not something the api will accept.
	want := esphomeUtil.LightCommand{
		State: true, Brightness: 0.5, Red: 1,
		Green: float64(float32(128.0 / 255)), Blue: 0,
		Effect:        esphomeUtil.LightEffectNone,
		HasBrightness: true, HasRGB: true, HasEffect: true,
	}
	if command != want {
		t.Fatalf("light command = %+v, want %+v", command, want)
	}

	// An effect rides along with the colour, and an off command carries
	// neither so that any light can still be turned off.
	res.Effect = "Rainbow"
	command, err = res.command(true)
	if err != nil {
		t.Fatalf("light effect command: %v", err)
	}
	if !command.HasEffect || command.Effect != "Rainbow" {
		t.Fatalf("light effect command = %+v, want the Rainbow effect", command)
	}
	command, err = res.command(false)
	if err != nil {
		t.Fatalf("light off command: %v", err)
	}
	if command.HasEffect {
		t.Fatalf("light off command = %+v, want no effect selection", command)
	}

	res.Brightness = 1.1
	if err := res.Validate(); err == nil {
		t.Fatalf("out-of-range brightness unexpectedly validated")
	}
}
