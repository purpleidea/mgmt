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

package util

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// RandomStringSimpleAlphabet contains the characters used by
// RandomStringSimple.
const RandomStringSimpleAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomStringSimple returns a cryptographically secure random string of the
// requested length. It only uses the ASCII characters a-z and A-Z.
func RandomStringSimple(length uint16) (string, error) {
	return randomStringSimple(rand.Reader, length)
}

func randomStringSimple(reader io.Reader, length uint16) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("random reader is nil")
	}

	limit := big.NewInt(int64(len(RandomStringSimpleAlphabet)))
	var output strings.Builder
	output.Grow(int(length))
	for i := uint16(0); i < length; i++ {
		value, err := rand.Int(reader, limit)
		if err != nil {
			return "", errwrap.Wrapf(err, "could not generate random string")
		}
		output.WriteByte(RandomStringSimpleAlphabet[value.Int64()])
	}

	return output.String(), nil
}
