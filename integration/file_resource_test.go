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

// TestFileRes verifies file resource can create a file
func TestFileRes(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilIdle(t)

	// deploy
	m.DeployLang(t, `
	$root = getenv("MGMT_TEST_ROOT")
	file "${root}/file.txt" {
		content => "test",
	}
	`)
	m.WaitUntilConverged(t)

	// an empty file should have been created
	text, _ := m.WorkdirReadFromFile(t, "file.txt")
	assert.Equal(t, "test", text)
}

// TestFileResEmpty verifies file resource can create an empty file
func TestFileResEmpty(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilIdle(t)

	// deploy
	m.DeployLang(t, `
	$root = getenv("MGMT_TEST_ROOT")
	file "${root}/empty.txt" {
		content => "",
	}
	`)
	m.WaitUntilConverged(t)

	// an empty file should have been created
	text, _ := m.WorkdirReadFromFile(t, "empty.txt")
	assert.Equal(t, "", text)
}

// TestFileResEmptyCalculated verifies file resource works the same for string values as wel as calculated values
func TestFileResEmptyCalculated(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilIdle(t)

	// deploy
	m.DeployLang(t, `
	$root = getenv("MGMT_TEST_ROOT")
	file "${root}/empty.txt" {
		content => printf(""),
	}
	`)
	m.WaitUntilConverged(t)

	// an empty file should have been created
	text, _ := m.WorkdirReadFromFile(t, "empty.txt")
	assert.Equal(t, "", text)
}
