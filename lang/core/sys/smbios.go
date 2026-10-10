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

package coresys

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

var typeSmbiosOemStrings = types.NewType("[]str")

func init() {
	simple.ModuleRegister("sys.smbios", "oem_strings", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType(fmt.Sprintf("func() %s", typeSmbiosOemStrings)),
		F: SmbiosOemStrings,
	})
}

func SmbiosOemStrings(ctx context.Context, input []types.Value) (types.Value, error) {
	const smbios11 = "/sys/firmware/dmi/entries/11-0/raw"
	data, err := os.ReadFile(smbios11)

	if err != nil {
		return nil, errwrap.Wrapf(err, "Cannot read smbios type 11. Failed to read file, %s\n", smbios11)
	}

	// From DTMF DSP0134_3.9.0 spec:
	// OEM Strings (Type 11)
	// Header is
	// * type number (1byte)
	// * length (1byte, always 0x05)
	// * handle (2 bytes, don't need it right now)
	// * count (1 byte)
	//
	// count == number of strings in this type11 entry.

	// Do some checks to make sure we're really reading the correct entry.
	if data[0] != 11 {
		return nil, fmt.Errorf("smbios: Got unexpected value in header. 'Type' field must be 11, but got %d\n", data[0])
	}
	if data[1] != 5 {
		return nil, fmt.Errorf("smbios: Got unexpected value in header. 'Length' field must be 5, but got %d\n", data[0])
	}

	offset := 5 // offset of first string position
	count := data[4]

	entries := types.NewList(typeSmbiosOemStrings)
	for i := range count {
		// Look for the nul byte that ends this strings.
		end := offset + bytes.IndexByte(data[offset:], 0)
		if end == -1 {
			return nil, fmt.Errorf("smbios: On string entry %d, didn't find a trailing nul.", i)
		}

		str := string(data[offset:end])
		fmt.Printf("Got string [%d:%d]: %v\n", offset, end, str)
		offset = end + 1
		entries.Add(&types.StrValue{V: str})
	}

	return entries, nil
}
