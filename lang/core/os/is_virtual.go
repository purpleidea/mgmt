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
	"github.com/purpleidea/mgmt/util/errwrap"
)

// The Desktop Management Interface (DMI) is a framwork for exposing system data
// to software. We will make use of it to determine whether we're using a
// virtualization platform. dmiFilesSlice is a slice of file paths which might
// contain information related to the presence of a virtualization platform.
// This is where we might find the values present in virtualizationVendorSlice.
var dmiFilesSlice = []string{
	"/sys/class/dmi/id/product_name",
	"/sys/class/dmi/id/sys_vendor",
	"/sys/class/dmi/id/board_vendor",
	"/sys/class/dmi/id/bios_vendor",
	"/sys/class/dmi/id/product_version",
}

// virtualizationVendorSlice is a slice of strings that might be found in DMI
// related files (dmiFilesSlice) during the checks performed for the presence of
// virtualization.
var virtualizationVendorSlice = []string{
	"Amazon EC2",
	"Apple Virtualization",
	"BHYVE",
	"Bochs",
	"Google Computer Engine",
	"Hyper-V",
	"innotek GmbH",
	"KubeVirt",
	"KVM",
	"OpenStack",
	"Oracle Corporation",
	"Parallels",
	"QEMU",
	"VMware",
	"Xen",
}

func init() {
	simple.ModuleRegister(ModuleName, "is_virtual", &simple.Scaffold{
		T: types.NewType("func() bool"),
		F: IsVirtual,
	})
}

// IsVirtual is a shim for the isVirtual function
func IsVirtual(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := isVirtual(ctx)
	if err != nil {
		return nil, err
	}
	return &types.BoolValue{V: b}, nil
}

// isVirtual is a function that executes two types of checks: first, it checks
// whether we're running on Linux. If that's the case, we run checks related
// with the presence of virtualization platforms. If any of those checks returns
// true, then so does this function. Otherwise, it's assumed that it's not a
// virtualized environment.
func isVirtual(ctx context.Context) (bool, error) {
	// if we implement detection for OS other than Linux, this logic will have
	// to change
	if runtime.GOOS != "linux" {
		return false, errors.New("operating system is not Linux")
	}

	cpuInfoCheck, err1 := cpuInfoCheck(ctx)
	if err1 == nil && cpuInfoCheck {
		return true, nil
	}

	dmiFileCheck, err2 := dmiFileCheck(ctx)
	if err2 == nil && dmiFileCheck {
		return true, nil
	}

	if err1 != nil || err2 != nil {
		return false, errwrap.Append(err1, err2)
	}

	return false, nil

}

// In an x86 system, there's a check to detect virt envs. The Linux kernel adds
// the "hypervisor" flag to the CPU flags.
func cpuInfoCheck(ctx context.Context) (bool, error) {
	cpuInfo, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false, err
	}
	if strings.Contains(string(cpuInfo), "hypervisor") {
		return true, nil
	}
	return false, nil
}

// Check if any of the slices of virtualizationVendorSlice are present in any of
// the DMI files contained on dmiFilesSlice. If that's the case, then we return
// this function as true. This approach was inspired on systemd's work for a
// similar purpose.
// https://github.com/systemd/systemd/blob/main/src/basic/virt.c#L158
func dmiFileCheck(ctx context.Context) (bool, error) {
	for _, dmiFile := range dmiFilesSlice {
		dmiFileContent, err := os.ReadFile(dmiFile)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		} else if err != nil {
			continue
		}

		for _, vendor := range virtualizationVendorSlice {
			if strings.Contains(string(dmiFileContent), vendor) {
				return true, nil
			}
		}
	}

	return false, nil
}
