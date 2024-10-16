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

// Package distro is for utilities that deal with os/distro things. Best to keep
// these all in one place so that adding this data happens all in the same file.
package distro

import (
	"context"
	"os"

	"github.com/ashcrow/osrelease"
)

const (
	// FamilyRedHat represents distros like Fedora and RHEL.
	FamilyRedHat = "redhat"

	// FamilyDebian represents distros like Debian and Ubuntu.
	FamilyDebian = "debian"

	// FamilyArchLinux represents primarily ArchLinux.
	FamilyArchLinux = "archlinux"

	// DistroDebian is the Debian distro.
	DistroDebian = "debian"

	// DistroFedora is the Fedora distro.
	DistroFedora = "fedora"
)

var (
	// MapDistroToBootstrapPackages is a map of distro to packages needed to
	// run our software.
	MapDistroToBootstrapPackages = map[string][]string{
		// TODO: add more values
		DistroDebian: {
			"libaugeas-dev",
			"libvirt-dev",
			"packagekit-tools",
		},
		DistroFedora: {
			"augeas-devel",
			"libvirt-devel",
			"PackageKit",
		},
	}
)

// DistroToBootstrapPackages returns the list of packages corresponding to the
// distro for bootstrapping. This returns false if the value doesn't exist.
func DistroToBootstrapPackages(distro string) ([]string, bool) {
	l, exists := MapDistroToBootstrapPackages[distro]
	return l, exists
}

// Family returns the distro family.
func Family(ctx context.Context) (string, error) {
	if b, err := IsFamilyRedHat(ctx); err != nil {
		return "", err
	} else if b {
		return FamilyRedHat, nil
	}
	if b, err := IsFamilyDebian(ctx); err != nil {
		return "", err
	} else if b {
		return FamilyDebian, nil
	}
	if b, err := IsFamilyArchLinux(ctx); err != nil {
		return "", err
	} else if b {
		return FamilyArchLinux, nil
	}
	return "", nil // unknown
}

// IsFamilyRedHat detects if the os family is redhat.
func IsFamilyRedHat(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/redhat-release")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsFamilyDebian detects if the os family is debian.
func IsFamilyDebian(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/debian_version")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsFamilyArchLinux detects if the os family is archlinux.
func IsFamilyArchLinux(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/arch-release")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Distro returns the distro name.
func Distro(ctx context.Context) (string, error) {
	output, err := parseOSRelease(ctx)
	if err != nil {
		return "", err
	}
	return output.ID, nil
}

// IsDistroDebian detects if the os distro is debian. (Not ubuntu!)
func IsDistroDebian(ctx context.Context) (bool, error) {
	output, err := parseOSRelease(ctx)
	if err != nil {
		return false, err
	}
	if output.ID == DistroDebian {
		return true, nil
	}
	return false, nil
}

// IsDistroFedora detects if the os distro is fedora.
func IsDistroFedora(ctx context.Context) (bool, error) {
	output, err := parseOSRelease(ctx)
	if err != nil {
		return false, err
	}
	if output.ID == DistroFedora {
		return true, nil
	}
	return false, nil
}

// IsDistroArchLinux detects if the os distro is archlinux.
func IsDistroArchLinux(ctx context.Context) (bool, error) {
	// TODO: Are there other distros in the archlinux family?
	return IsFamilyArchLinux(ctx)
}

// parseOSRelease is a simple helper function to parse the /etc/os-release file.
// TODO: We could probably implement our own cleaner parser eventually.
// TODO: Cache the result in a global if we don't care about changes.
func parseOSRelease(ctx context.Context) (*osrelease.OSRelease, error) {
	// TODO: use ctx around io operations
	output, err := osrelease.New()
	if err != nil {
		return nil, err
	}
	return &output, nil
}
