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

package testpkg

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// Lgamma returns the natural logarithm and sign of Gamma(x), and two non-error
// results. Rejected by generator policy.
func Lgamma(x float64) (lgamma float64, sign int) {
	return math.Lgamma(x)
}

// AllKind mixes several allowed scalar types and returns allowed float64.
func AllKind(x int64, y string) float64 {
	if y == "" {
		return float64(x)
	}
	return float64(x) + 1.0
}

// ToUpper
func ToUpper(s string) string { return strings.ToUpper(s) }

// ToLower is excluded in the test via config (`Exclude: []string{"ToLower"},`).
func ToLower(s string) string { return strings.ToLower(s) }

// Max is a simple allowed signature with two float64 params.
func Max(x, y float64) float64 {
	if x > y {
		return x
	}
	return y
}

// WithError returns (string, error) and will be marked errorful=true.
func WithError(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	return s, nil
}

// WithErrorButNothingElse returns only error
func WithErrorButNothingElse(s string) error {
	if s == "" {
		return fmt.Errorf("empty")
	}
	return nil
}

// WithNothingElse returns nothing
func WithNothingElse(s string) { _ = s }

// Nextafter32 uses float32 in params/results, which is not in the allowlist.
func Nextafter32(x, y float32) (r float32) {
	return math.Nextafter32(x, y)
}

// WithInt exercises int/int64/bool/string mix
func WithInt(s float64, i int, x int64, j, k int, b bool, t string) string {
	if b {
		return fmt.Sprintf("%f-%d-%d-%d-%d-%t-%s", s, i, x, j, k, b, t)
	}
	return t
}

// SuperByte returns []byte. The generator keeps []byte so the template applies
// string([]byte).
func SuperByte(s []byte, t string) []byte {
	return append(append([]byte{}, s...), t...)
}

// Join is a variadic example (...string) represented as []string with
// Variadic=true.
func Join(elem ...string) string {
	return filepath.Join(elem...)
}
