// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

// Package arch is for utilities that deal with cpu architectures. Best to keep
// these all in one place so that adding this data happens all in the same file.
package arch

var (
	// Any is a special value meaning noarch or any arch.
	Any = "*"

	// MapPackageKitArchToGoArch contains the mapping from PackageKit arch
	// to GOARCH.
	MapPackageKitArchToGoArch = map[string]string{
		// TODO: add more values
		// noarch
		"noarch": "ANY", // as seen in Fedora
		"any":    "ANY", // as seen in ArchLinux
		"all":    "ANY", // as seen in Debian
		// fedora
		"x86_64":  "amd64",
		"aarch64": "arm64",
		// debian, from: https://www.debian.org/ports/
		"amd64": "amd64",
		"arm64": "arm64",
		"i386":  "386",
		"i486":  "386",
		"i586":  "386",
		"i686":  "386",
	}

	// MapGoArchToVirtBuilderArch is a map of GOARCH to virt-builder format.
	MapGoArchToVirtBuilderArch = map[string]string{
		// TODO: add more values
		"386":   "i686",
		"amd64": "x86_64",
		//"arm": "armv7l", // TODO: is this correct?
		"arm64": "aarch64",
		//"s390x": "?", // TODO: add me
		//"?": "ppc64le", // TODO: add me
		//"?": "ppc64", // TODO: add me
		//"mips64": "?", // TODO: add me
		//"mips64le": "?", // TODO: add me
		//"ppc64": "?", // TODO: add me
		//"ppc64le": "?", // TODO: add me
	}
)

// GoArchToVirtBuilderArch returns the virt-builder arch corresponding to the
// golang GOARCH value. This returns false if the value doesn't exist.
func GoArchToVirtBuilderArch(goarch string) (string, bool) {
	s, exists := MapGoArchToVirtBuilderArch[goarch]
	return s, exists
}
