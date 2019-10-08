package resources

import (
	"fmt"
	"log"
	"strings"
)

// symbolicMode calculates the filemodes from the symbolic file input
func symbolicMode(p string) (mode string, err error) {
	var octal int64
	var out int64
	m := parse(p)
	for _, v := range m {
		v = string(v)
		switch v[0] {
		default:
			err = fmt.Errorf("error")
			return v, err
		case 'g':
			switch v[2] {
			case 'r':
				octal = groupRead
			case 'w':
				octal = groupWrite
			case 'x':
				octal = groupExecute
			}
		case 'o':
			switch v[2] {
			case 'r':
				octal = otherRead
			case 'w':
				octal = otherWrite
			case 'x':
				octal = otherExecute
			}
		case 'u':
			switch v[2] {

			case 'r':
				octal = userRead
			case 'w':
				octal = userWrite
			case 'x':
				octal = userExecute
			}
		}
		out = out + octal
		if err != nil {
			log.Fatal("Invalid perm")

		}
	}

	mode = fmt.Sprintf("%#04o", out)
	fmt.Println(mode)
	return mode, err
}

// parse returns individual file modes from a string
func parse(symbolic string) []string {
	var p []string
	for _, v := range strings.Split(symbolic, ",") {
		v = strings.ReplaceAll(v, "a", "ugo")
		s := strings.Split(v, "=")
		for _, w := range s[0] {
			for _, q := range s[1] {
				c := string(w) + "=" + string(q) + "\n"
				p = append(p, c)
			}
		}
	}
	p = unique(p)
	return p
}

// unique removes repeating filemodes
func unique(p []string) []string {
	k := make(map[string]bool)
	l := []string{}
	for _, i := range p {
		if _, v := k[i]; !v {
			k[i] = true
			l = append(l, i)
		}
	}
	return l
}

// octal constants
const (
	defRead    = 04
	defWrite   = 02
	defExecute = 01
	userShift  = 6
	groupShift = 3
	otherShift = 0

	userRead     = defRead << userShift
	userWrite    = defWrite << userShift
	userExecute  = defExecute << userShift
	groupRead    = defRead << groupShift
	groupWrite   = defWrite << groupShift
	groupExecute = defExecute << groupShift
	otherRead    = defRead << otherShift
	otherWrite   = defWrite << otherShift
	otherExecute = defExecute << otherShift
)
