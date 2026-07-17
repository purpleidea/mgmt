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

func TestParseEsphomeColor(t *testing.T) {
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
		red, green, blue, err := parseEsphomeColor(input)
		if err != nil {
			t.Fatalf("parseEsphomeColor(%q): %v", input, err)
		}
		if got := [3]float64{red, green, blue}; got != want {
			t.Fatalf("parseEsphomeColor(%q) = %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"", "chartreuse", "#12345", "#gg0000"} {
		if _, _, _, err := parseEsphomeColor(input); err == nil {
			t.Fatalf("parseEsphomeColor(%q) unexpectedly succeeded", input)
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

	res.Speed = 0
	if err := res.Validate(); err == nil {
		t.Fatalf("zero fan speed unexpectedly validated")
	}
	res.Speed = 50
	res.Direction = "sideways"
	if err := res.Validate(); err == nil {
		t.Fatalf("invalid fan direction unexpectedly validated")
	}
}

func TestEsphomeLightValidateAndCommand(t *testing.T) {
	res := (&EsphomeLightRes{}).Default().(*EsphomeLightRes)
	res.SetName("Status Light")
	res.Endpoint = "conveyor"
	res.State = esphomeStateOn
	res.Brightness = 0.5
	res.Color = "#ff8000"
	if err := res.Validate(); err != nil {
		t.Fatalf("valid light: %v", err)
	}
	command, err := res.command(true)
	if err != nil {
		t.Fatalf("light command: %v", err)
	}
	want := esphomeUtil.LightCommand{
		State: true, Brightness: 0.5, Red: 1,
		Green: float64(float32(128.0 / 255)), Blue: 0,
		HasBrightness: true, HasRGB: true,
	}
	if command != want {
		t.Fatalf("light command = %+v, want %+v", command, want)
	}

	res.Brightness = 1.1
	if err := res.Validate(); err == nil {
		t.Fatalf("out-of-range brightness unexpectedly validated")
	}
}
