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
	"errors"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine/resources/packagekit"
)

func TestNilList1(t *testing.T) {
	var x []string
	if x != nil { // we have this expectation for obj.fileList in pkg
		t.Errorf("list should have been nil, was: %+v", x)
	}
	x = []string{} // empty list
	if x == nil {
		t.Errorf("list should have been empty, was: %+v", x)
	}
}

func TestPkgHigherVersionError(t *testing.T) {
	pkErr := &packagekit.PkError{
		Code:    packagekit.PkErrorEnumPackageAlreadyInstalled,
		Details: "higher version is already installed",
	}
	err := (&PkgRes{State: "4.5.1-21.fc42"}).packageAlreadyInstalledError(pkErr)
	if err == nil {
		t.Fatalf("expected higher version error")
	}
	if !strings.Contains(err.Error(), "allowdowngrade") {
		t.Errorf("expected allowdowngrade hint, got: %v", err)
	}
	var wrapped *packagekit.PkError
	if !errors.As(err, &wrapped) {
		t.Errorf("expected wrapped PackageKit error")
	}
}

func TestPkgHigherVersionErrorSkipped(t *testing.T) {
	testCases := []struct {
		name string
		res  *PkgRes
		err  error
	}{
		{
			name: "allow downgrade",
			res:  &PkgRes{State: "4.5.1-21.fc42", AllowDowngrade: true},
			err:  &packagekit.PkError{Code: packagekit.PkErrorEnumPackageAlreadyInstalled},
		},
		{
			name: "non-version state",
			res:  &PkgRes{State: PkgStateInstalled},
			err:  &packagekit.PkError{Code: packagekit.PkErrorEnumPackageAlreadyInstalled},
		},
		{
			name: "different packagekit error",
			res:  &PkgRes{State: "4.5.1-21.fc42"},
			err:  &packagekit.PkError{Code: packagekit.PkErrorEnumPackageNotFound},
		},
		{
			name: "non packagekit error",
			res:  &PkgRes{State: "4.5.1-21.fc42"},
			err:  errors.New("plain error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.res.packageAlreadyInstalledError(tc.err); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPkgTransactionFlags(t *testing.T) {
	testCases := []struct {
		name string
		res  *PkgRes
		flag uint64
		want bool
	}{
		{
			name: "version without downgrade",
			res:  &PkgRes{State: "4.5.1-21.fc42"},
			flag: packagekit.PkTransactionFlagEnumAllowDowngrade,
			want: false,
		},
		{
			name: "version with downgrade",
			res:  &PkgRes{State: "4.5.1-21.fc42", AllowDowngrade: true},
			flag: packagekit.PkTransactionFlagEnumAllowDowngrade,
			want: true,
		},
		{
			name: "installed with downgrade",
			res:  &PkgRes{State: PkgStateInstalled, AllowDowngrade: true},
			flag: packagekit.PkTransactionFlagEnumAllowDowngrade,
			want: false,
		},
		{
			name: "trusted by default",
			res:  &PkgRes{State: PkgStateInstalled},
			flag: packagekit.PkTransactionFlagEnumOnlyTrusted,
			want: true,
		},
		{
			name: "untrusted allowed",
			res:  &PkgRes{State: PkgStateInstalled, AllowUntrusted: true},
			flag: packagekit.PkTransactionFlagEnumOnlyTrusted,
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flags := tc.res.packageTransactionFlags()
			if got := flags&tc.flag == tc.flag; got != tc.want {
				t.Errorf("unexpected flag state: got %t, want %t", got, tc.want)
			}
		})
	}
}
