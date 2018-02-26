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

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeployFailSyntaxError makes sure deploy errors when the lang files contains a syntax error
func TestDeployFailSyntaxError(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)
	defer m.StopBackground(t)

	m.RunBackground(t)
	m.WaitUntilIdle(t)

	// deploy lang file to the just started instance
	out, err := m.DeployLangFile(nil, "lang/syntaxerror.mcl")
	if err == nil {
		t.Fatal("deploy command did not fail")
	}
	assert.Contains(t, out, "could not set scope")
}
