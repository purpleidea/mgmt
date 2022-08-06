// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//go:build !root

package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"testing"

	"github.com/purpleidea/mgmt/util"
)

func TestInstance0(t *testing.T) {
	code := `
	import "sys"
	$root = sys.getenv("MGMT_TEST_ROOT")

	file "${root}/mgmt-hello-world" {
		content => "hello world from @purpleidea\n",
		state => $const.res.file.state.exists,
	}
	`
	m := Instance{
		Hostname: "h1", // arbitrary
		Preserve: true,
		Debug:    false, // TODO: set to true if not too wordy
		Logf: func(format string, v ...interface{}) {
			t.Logf("test: "+format, v...)
		},
	}
	if err := m.SimpleDeployLang(code); err != nil {
		t.Errorf("failed with: %+v", err)
		if output, err := m.CombinedOutput(); err == nil {
			t.Errorf("logs from instance:\n\n%s", output)
		}
		return
	}
	d := m.Dir()
	t.Logf("test ran in:\n%s", d)
	root := path.Join(d, RootDirectory)
	file := path.Join(root, "mgmt-hello-world")
	_, err := os.Stat(file)
	if err != nil {
		t.Errorf("could not find file: %+v", err)
		return
	}
}

func TestInstance1(t *testing.T) {
	type test struct { // an individual test
		name   string
		code   string // mcl code
		fail   bool
		expect map[string]string
	}
	testCases := []test{}

	{
		code := util.Code(`
		import "sys"
		$root = sys.getenv("MGMT_TEST_ROOT")

		file "${root}/mgmt-hello-world" {
			content => "hello world from @purpleidea\n",
			state => $const.res.file.state.exists,
		}
		`)
		testCases = append(testCases, test{
			name: "hello world",
			code: code,
			fail: false,
			expect: map[string]string{
				"mgmt-hello-world": "hello world from @purpleidea\n",
			},
		})
	}

	for index, tc := range testCases { // run all the tests
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			code, fail, expect := tc.code, tc.fail, tc.expect

			m := Instance{
				Hostname: "h1",
				Preserve: true,
				Debug:    false, // TODO: set to true if not too wordy
				Logf: func(format string, v ...interface{}) {
					t.Logf(fmt.Sprintf("test #%d: ", index)+format, v...)
				},
			}
			err := m.SimpleDeployLang(code)
			d := m.Dir()
			if d != "" {
				t.Logf("test ran in:\n%s", d)
			}

			if !fail && err != nil {
				t.Errorf("failed with: %+v", err)
				if output, err := m.CombinedOutput(); err == nil {
					t.Errorf("logs from instance:\n\n%s", output)
				}
				return
			}
			if fail && err == nil {
				t.Errorf("passed, expected fail")
				return
			}

			if expect == nil { // test done early
				return
			}

			files := []string{}
			for x := range expect {
				files = append(files, x)
			}
			sort.Strings(files) // loop in a deterministic order
			for _, f := range files {
				filename := path.Join(d, RootDirectory, f)
				b, err := ioutil.ReadFile(filename)
				if err != nil {
					t.Errorf("could not read file: `%s`", filename)
					continue
				}
				if expect[f] != string(b) {
					t.Errorf("file: `%s` did not match expected", f)
				}
			}
		})
	}
}

func TestCluster0(t *testing.T) {
	// TODO: implement a simple test for documentation purposes
}

func TestCluster1(t *testing.T) {
	type test struct { // an individual test
		name   string
		code   string // mcl code
		fail   bool
		hosts  []string
		expect map[string]map[string]string // hostname, file, contents
	}
	testCases := []test{}

	{
		code := util.Code(`
		import "sys"
		$root = sys.getenv("MGMT_TEST_ROOT")
		$host = sys.hostname()

		file "${root}/mgmt-hostname" {
			content => "i am ${host}\n",
			state => $const.res.file.state.exists,
		}
		`)
		testCases = append(testCases, test{
			name:  "simple pair",
			code:  code,
			fail:  false,
			hosts: []string{"h1", "h2"},
			expect: map[string]map[string]string{
				"h1": {
					"mgmt-hostname": "i am h1\n",
				},
				"h2": {
					"mgmt-hostname": "i am h2\n",
				},
			},
		})
	}
	{
		code := util.Code(`
		import "sys"
		$root = sys.getenv("MGMT_TEST_ROOT")
		$host = sys.hostname()

		file "${root}/mgmt-hostname" {
			content => "i am ${host}\n",
			state => $const.res.file.state.exists,
		}
		`)
		testCases = append(testCases, test{
			name:  "hello world",
			code:  code,
			fail:  false,
			hosts: []string{"h1", "h2", "h3"},
			expect: map[string]map[string]string{
				"h1": {
					"mgmt-hostname": "i am h1\n",
				},
				"h2": {
					"mgmt-hostname": "i am h2\n",
				},
				"h3": {
					"mgmt-hostname": "i am h3\n",
				},
			},
		})
	}

	for index, tc := range testCases { // run all the tests
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			code, fail, hosts, expect := tc.code, tc.fail, tc.hosts, tc.expect

			c := Cluster{
				Hostnames: hosts,
				Preserve:  true,
				Debug:     false, // TODO: set to true if not too wordy
				Logf: func(format string, v ...interface{}) {
					t.Logf(fmt.Sprintf("test #%d: ", index)+format, v...)
				},
			}
			err := c.SimpleDeployLang(code)
			if d := c.Dir(); d != "" {
				t.Logf("test ran in:\n%s", d)
			}
			instances := c.Instances()

			if !fail && err != nil {
				t.Errorf("failed with: %+v", err)
				for _, h := range hosts {
					if output, err := instances[h].CombinedOutput(); err == nil {
						t.Errorf("logs from instance `%s`:\n\n%s", h, output)
					}
				}
				return
			}
			if fail && err == nil {
				t.Errorf("passed, expected fail")
				return
			}

			if expect == nil { // test done early
				return
			}

			for _, h := range hosts {
				instance := instances[h]
				d := instance.Dir()
				hexpect, exists := expect[h]
				if !exists {
					continue
				}

				files := []string{}
				for x := range hexpect {
					files = append(files, x)
				}
				sort.Strings(files) // loop in a deterministic order
				for _, f := range files {
					filename := path.Join(d, RootDirectory, f)
					b, err := ioutil.ReadFile(filename)
					if err != nil {
						t.Errorf("could not read file: `%s`", filename)
						continue
					}
					if hexpect[f] != string(b) {
						t.Errorf("file: `%s` did not match expected", f)
					}
				}
			}
		})
	}
}
