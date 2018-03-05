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

// TestFileFunc verifies the file function works as expected
func TestFileFunc(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilConverged(t)

	// create template file to be read
	m.WorkdirWriteToFile(t, "file0.txt.tmpl", "This is a template file with some value: {{.somevalue}}")

	// deploy
	m.DeployLangFile(t, "lang/file0.mcl")
	m.WaitUntilConverged(t)

	text, _ := m.WorkdirReadFromFile(t, "file0.txt")
	assert.Equal(t, "This is a template file with some value: somevalue", text)
}

// TestFileFuncStream verifies the file function updates when template is changed
func TestFileFuncStream(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilConverged(t)

	// create template file to be read
	m.WorkdirWriteToFile(t, "file0.txt.tmpl", "This is a template file with some value: {{.somevalue}}")

	m.DeployLangFile(t, "lang/file0.mcl")
	m.WaitUntilConverged(t)

	text, _ := m.WorkdirReadFromFile(t, "file0.txt")
	assert.Equal(t, "This is a template file with some value: somevalue", text)

	// change template file to be read
	m.WorkdirWriteToFile(t, "file0.txt.tmpl", "This is a template file with some other value: {{.somevalue}}")
	m.WaitUntilConverged(t)

	text2, _ := m.WorkdirReadFromFile(t, "file0.txt")
	assert.Equal(t, "This is a template file with some other value: somevalue", text2)
}

// TestFileFuncNonExist verifies if the file function can cope with non-existing files
func TestFileFuncNonExist(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)
	m.WaitUntilConverged(t)

	// deploy
	m.DeployLangFile(t, "lang/file0.mcl")
	m.WaitUntilConverged(t)

	var text string

	// TODO: file doesn't get created because file() hasn't returned output yet, inconsistent with behaviour regarding to files that go out of existence.
	// text, _ = m.WorkdirReadFromFile(t, "file0.txt")
	// assert.Equal(t, "", text)

	// create template file to be read
	m.WorkdirWriteToFile(t, "file0.txt.tmpl", "This is a template file with some value: {{.somevalue}}")
	m.WaitUntilConverged(t)

	text, _ = m.WorkdirReadFromFile(t, "file0.txt")
	assert.Equal(t, "This is a template file with some value: somevalue", text)

	m.WorkdirRemoveFile(t, "file0.txt.tmpl")
	m.WaitUntilConverged(t)

	text, _ = m.WorkdirReadFromFile(t, "file0.txt")
	assert.Equal(t, "", text)

}
