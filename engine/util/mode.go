// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package util

import (
	"fmt"
	"os"
	"strings"
)

// Constant bytes for the who (u, g, or o) and the what (r, w, x, s, or t).
const (
	ModeUser  uint32 = 64
	ModeGroup uint32 = 8
	ModeOther uint32 = 1

	ModeRead  uint32 = 4
	ModeWrite uint32 = 2
	ModeExec  uint32 = 1

	ModeSetU   uint32 = 4
	ModeSetG   uint32 = 2
	ModeSticky uint32 = 1
)

// modeIsValidWho checks that only the characters 'u', 'g', 'o' are in the who
// string. It expects that 'a' was expanded to "ugo" already and will return
// false if not.
func modeIsValidWho(who string) bool {
	for _, w := range []string{"u", "g", "o"} {
		who = strings.Replace(who, w, "", -1) // TODO: use ReplaceAll in 1.12
	}
	return len(who) == 0
}

// modeIsValidWhat checks that only the valid mode characters are in the what
// string ('r', 'w', 'x', 's', 't').
func modeIsValidWhat(what string) bool {
	for _, w := range []string{"r", "w", "x", "s", "t"} {
		what = strings.Replace(what, w, "", -1) // TODO: use ReplaceAll in 1.12
	}
	return len(what) == 0
}

// modeAssigned executes an assigment symbolic mode string (u=r). It clears out
// any bits for every subject in who and then assigns the specified modes in
// what.
func modeAssigned(who, what string, from os.FileMode) (os.FileMode, error) {
	// Clear out any users defined in 'who'.
	for _, w := range who {
		switch w {
		case 'u':
			from = from &^ os.FileMode(448) // 111000000 in bytes
			from = from &^ os.ModeSetuid
		case 'g':
			from = from &^ os.FileMode(56) // 111000 in bytes
			from = from &^ os.ModeSetgid
		case 'o':
			from = from &^ os.FileMode(7) // 111 in bytes
		}
	}

	for _, c := range what {
		switch c {
		case 'r':
			m := modeValueFrom(who, ModeRead)
			if from&m == 0 {
				from = from | m
			}
		case 'w':
			m := modeValueFrom(who, ModeWrite)
			if from&m == 0 {
				from = from | m
			}
		case 'x':
			m := modeValueFrom(who, ModeExec)
			if from&m == 0 {
				from = from | m
			}
		case 's':
			for _, w := range who {
				switch w {
				case 'u':
					from = from | os.ModeSetuid
				case 'g':
					from = from | os.ModeSetgid
				}
			}
		case 't':
			from = from | os.ModeSticky
		default:
			return os.FileMode(0), fmt.Errorf("invalid character %q", c)
		}
	}

	return from, nil
}

// modeAdded executes an addition symbolic mode string (u+x) and will add the
// bits requested in what if not present.
func modeAdded(who, what string, from os.FileMode) (os.FileMode, error) {
	for _, c := range what {
		switch c {
		case 'r':
			m := modeValueFrom(who, ModeRead)
			if from&m == 0 {
				from = from | m
			}
		case 'w':
			m := modeValueFrom(who, ModeWrite)
			if from&m == 0 {
				from = from | m
			}
		case 'x':
			m := modeValueFrom(who, ModeExec)
			if from&m == 0 {
				from = from | m
			}
		case 's':
			for _, w := range who {
				switch w {
				case 'u':
					from = from | os.ModeSetuid
				case 'g':
					from = from | os.ModeSetgid
				}
			}
		case 't':
			from = from | os.ModeSticky
		default:
			return os.FileMode(0), fmt.Errorf("invalid character %q", c)
		}
	}

	return from, nil
}

// modeSubtracted executes an subtraction symbolic mode string (u+x) and will
// removethe bits requested in what if present.
func modeSubtracted(who, what string, from os.FileMode) (os.FileMode, error) {
	for _, c := range what {
		switch c {
		case 'r':
			m := modeValueFrom(who, ModeRead)
			if from&m != 0 {
				from = from &^ m
			}
		case 'w':
			m := modeValueFrom(who, ModeWrite)
			if from&m != 0 {
				from = from &^ m
			}
		case 'x':
			m := modeValueFrom(who, ModeExec)
			if from&m != 0 {
				from = from &^ m
			}
		case 's':
			for _, w := range who {
				switch w {
				case 'u':
					if from&os.ModeSetuid != 0 {
						from = from &^ os.ModeSetuid
					}
				case 'g':
					if from&os.ModeSetgid != 0 {
						from = from &^ os.ModeSetgid
					}
				}
			}
		case 't':
			if from&os.ModeSticky != 0 {
				from = from | os.ModeSticky
			}
		default:
			return os.FileMode(0), fmt.Errorf("invalid character %q", c)
		}
	}

	return from, nil
}

// modeValueFrom will return the bits requested for the mode in the correct
// possitions for the specified subjects in who.
func modeValueFrom(who string, modeType uint32) os.FileMode {
	i := uint32(0)
	for _, w := range who {
		switch w {
		case 'u':
			i += ModeUser * uint32(modeType)
		case 'g':
			i += ModeGroup * uint32(modeType)
		case 'o':
			i += ModeOther * uint32(modeType)
		}
	}

	return os.FileMode(i)
}

// ParseSymbolicModes parses a slice of symbolic mode strings. By default it
// will only accept the assignment input (=), but if allowAssign is true, then
// all symbolic mode strings (=, +, -) can be used as well.
//
// Symbolic mode is expected to be a string of who (user, group, other) then the
// operation (=, +, -) then the change (read, write, execute, setuid, setgid,
// sticky).
//
// Eg: ug=rw
//
// If you repeat yourself in the slice (eg. u=rw,u=w) ParseSymbolicModes will
// fail with an error.
func ParseSymbolicModes(modes []string, from os.FileMode, allowAssign bool) (os.FileMode, error) {
	symModes := make([]struct {
		mode, who, what string

		parse func(who, what string, from os.FileMode) (os.FileMode, error)
	}, len(modes))

	for i, mode := range modes {
		symModes[i].mode = mode

		// If string contains '=' and no '+/-' it is safe to guess it is
		// an assign.
		if strings.Contains(mode, "=") && !strings.ContainsAny(mode, "+-") {
			m := strings.Split(mode, "=")
			if len(m) != 2 {
				return os.FileMode(0), fmt.Errorf("only a single %q is allowed but found %d", "=", len(m))
			}
			symModes[i].who = m[0]
			symModes[i].what = m[1]
			symModes[i].parse = modeAssigned
			continue

		} else if strings.Contains(mode, "+") && !strings.ContainsAny(mode, "=-") && allowAssign {
			m := strings.Split(mode, "+")
			if len(m) != 2 {
				return os.FileMode(0), fmt.Errorf("only a single %q is allowed but found %d", "+", len(m))
			}
			symModes[i].who = m[0]
			symModes[i].what = m[1]
			symModes[i].parse = modeAdded
			continue

		} else if strings.Contains(mode, "-") && !strings.ContainsAny(mode, "=+") && allowAssign {
			m := strings.Split(mode, "-")
			if len(m) != 2 {
				return os.FileMode(0), fmt.Errorf("only a single %q is allowed but found %d", "-", len(m))
			}
			symModes[i].who = m[0]
			symModes[i].what = m[1]
			symModes[i].parse = modeSubtracted
			continue
		}

		return os.FileMode(0), fmt.Errorf("%s is not a valid a symbolic mode", symModes[i].mode)
	}

	// Validate input and verify the slice of symbolic modes does not
	// contain redundancy.
	seen := make(map[rune]struct{}) // validate that subjects are not duplicated
	for i := range symModes {
		if strings.ContainsRune(symModes[i].who, 'a') || symModes[i].who == "" {
			// If 'a' or empty who (implicit 'a') is called and
			// there are more symbolic modes in the slice then it
			// must be a repetition.
			if len(symModes) > 1 {
				return os.FileMode(0), fmt.Errorf("subject was repeated: each subject (u,g,o) is only accepted once")
			}

			symModes[i].who = "ugo"
		}

		if !modeIsValidWhat(symModes[i].what) || !modeIsValidWho(symModes[i].who) {
			return os.FileMode(0), fmt.Errorf("unexpected character assignment in %s", symModes[i].mode)
		}

		for _, w := range symModes[i].who {
			if _, ok := seen[w]; ok {
				return os.FileMode(0), fmt.Errorf("subject was repeated: only define each subject (u,g,o) once")
			}
			seen[w] = struct{}{}
		}
	}

	// Parse each symbolic mode accumulatively onto the file mode.
	for _, m := range symModes {
		var err error
		from, err = m.parse(m.who, m.what, from)
		if err != nil {
			return os.FileMode(0), err
		}
	}

	return from, nil
}
