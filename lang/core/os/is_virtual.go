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

package coreos

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

var virtualizationVendorMap = map[string]string{
	"KVM":                    "KVM",
	"OpenStack":              "KVM",
	"KubeVirt":               "KVM",
	"Amazon EC2":             "Amazon",
	"QEMU":                   "QEMU",
	"VMware":                 "VMware",
	"VMW":                    "VMware",
	"innotek GmbH":           "Oracle",
	"VirtualBox":             "Oracle",
	"Oracle Corporation":     "Oracle",
	"Xen":                    "Xen",
	"Bochs":                  "Bochs",
	"Parallels":              "Parallels",
	"BHYVE":                  "BHYVE",
	"Hyper-V":                "Microsoft",
	"Apple Virtualization":   "Apple",
	"Google Computer Engine": "Google",
}

var dmiFilesSlice = []string{
	"/sys/class/dmi/id/product_name",
	"/sys/class/dmi/id/sys_vendor",
	"/sys/class/dmi/id/board_vendor",
	"/sys/class/dmi/id/bios_vendor",
	"/sys/class/dmi/id/product_version",
}

func init() {
	simple.ModuleRegister(ModuleName, "is_virtual", &simple.Scaffold{
		T: types.NewType("func() bool"),
		F: IsVirtual,
	})
}

// IsVirtual is a simple function that executes three types of checks: first, we
// check whether we're running on Linux. If that's the case, we run the two
// different checks we have, related with the presence of virtualization and
// containerization platforms. If any of those checks returns true, then so does
// this function; otherwise we assume that we're not in any of those contexts
func IsVirtual(ctx context.Context, input []types.Value) (types.Value, error) {
	// If we implement detection for OS other than Linux, this logic will have
	// to change
	opersys := runtime.GOOS
	if opersys != "linux" {
		return nil, errors.New("we're not running on Linux, exiting")
	}

	switch {
	case virtCheck():
		return &types.BoolValue{V: true}, nil
	case containerCheck():
		return &types.BoolValue{V: true}, nil
	default:
		return &types.BoolValue{V: false}, nil
	}
}

// We make use of systemd's work for detecting virtualization platforms, and
// check if any of the keys of virtualizationVendorMap is present on any of the
// DMI files present on dmiFilesSlice. If that's the case, then we return this
// func as true. https://github.com/systemd/systemd/blob/main/src/basic/virt.c
func virtCheck() bool {
	for _, dmiFile := range dmiFilesSlice {
		dmiFileContent, _ := os.ReadFile(dmiFile)
		for key := range virtualizationVendorMap {
			if strings.Contains(string(dmiFileContent), key) {
				return true
			}
		}
	}
	return false
}

// We're checking for docker, podman, systemd-nspawn, WSL by checking the
// presence and contents of files related with each of these contanerization
// platforms; if any of the checks passes, we return return this func as true
func containerCheck() bool {
	_, err := os.Stat("/run/.containerenv") // Podman
	if err == nil {
		return true
	}
	_, err = os.Stat("/.dockerenv") // Docker
	if err == nil {
		return true
	}
	wsl, _ := os.ReadFile("/proc/sys/kernel/osrelease") // WSL
	if strings.Contains(string(wsl), "Microsoft") || strings.Contains(string(wsl), "WSL") {
		return true
	}
	nspawn, _ := os.ReadFile("/run/systemd/container") // systemd-nspawn
	if strings.Contains(string(nspawn), "systemd-nspawn") {
		return true
	}
	return false
}
