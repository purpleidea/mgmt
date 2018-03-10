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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"testing"
)

func TestInstance0(t *testing.T) {
	code := `
	$root = getenv("MGMT_TEST_ROOT")

	file "${root}/mgmt-hello-world" {
		content => "hello world from @purpleidea\n",
		state => "exists",
	}
	`
	m := Instance{
		Hostname: "h1", // arbitrary
		Preserve: true,
	}
	if err := m.SimpleDeployLang(code); err != nil {
		t.Errorf("failed with: %+v", err)
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
	values := []test{}

	{
		code := Code(`
		$root = getenv("MGMT_TEST_ROOT")

		file "${root}/mgmt-hello-world" {
			content => "hello world from @purpleidea\n",
			state => "exists",
		}
		`)
		values = append(values, test{
			name: "hello world",
			code: code,
			fail: false,
			expect: map[string]string{
				"mgmt-hello-world": "hello world from @purpleidea\n",
			},
		})
	}

	for index, test := range values { // run all the tests
		t.Run(fmt.Sprintf("test #%d (%s)", index, test.name), func(t *testing.T) {
			code, fail, expect := test.code, test.fail, test.expect

			m := Instance{
				Hostname: "h1",
				Preserve: true,
			}
			err := m.SimpleDeployLang(code)
			d := m.Dir()
			if d != "" {
				t.Logf("test ran in:\n%s", d)
			}

			if !fail && err != nil {
				t.Errorf("failed with: %+v", err)
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
	values := []test{}

	{
		code := Code(`
		$root = getenv("MGMT_TEST_ROOT")

		file "${root}/mgmt-hostname" {
			content => "i am ${hostname()}\n",
			state => "exists",
		}
		`)
		values = append(values, test{
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
		code := Code(`
		$root = getenv("MGMT_TEST_ROOT")

		file "${root}/mgmt-hostname" {
			content => "i am ${hostname()}\n",
			state => "exists",
		}
		`)
		values = append(values, test{
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

	for index, test := range values { // run all the tests
		t.Run(fmt.Sprintf("test #%d (%s)", index, test.name), func(t *testing.T) {
			code, fail, hosts, expect := test.code, test.fail, test.hosts, test.expect

			c := Cluster{
				Hostnames: hosts,
				Preserve:  true,
			}
			err := c.SimpleDeployLang(code)
			if d := c.Dir(); d != "" {
				t.Logf("test ran in:\n%s", d)
			}

			if !fail && err != nil {
				t.Errorf("failed with: %+v", err)
				return
			}
			if fail && err == nil {
				t.Errorf("passed, expected fail")
				return
			}

			if expect == nil { // test done early
				return
			}

			instances := c.Instances()
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
