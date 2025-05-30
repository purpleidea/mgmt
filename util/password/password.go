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

// Package password has some utility functions for dealing with misc passwords.
// XXX: Please note, the "github.com/tredoe/osutil/user/crypt/sha512_crypt"
// dependency is slightly suspicious and we should investigate it further to
// make sure there are no supply chain issues with it.
package password

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	sha512Crypt "github.com/tredoe/osutil/user/crypt/sha512_crypt"
	"golang.org/x/sys/unix"
)

const (
	// StdPrompt is the usual text that we would use to ask for a password.
	StdPrompt = "Password: "

	// XXX: these two are different on BSD, and were taken from:
	// golang.org/x/term/term_unix_other.go
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)

// ReadPassword reads a password from stdin and returns the result. It hides the
// display of the password typed. For more options try ReadPasswordCtxFdPrompt
// instead. If interrupted by an uncaught signal during read, then this can bork
// your terminal. It's best to use a version with a context instead.
func ReadPassword() ([]byte, error) {
	return ReadPasswordCtxFdPrompt(context.Background(), int(os.Stdin.Fd()), StdPrompt)
}

// ReadPasswordCtx reads a password from stdin and returns the result. It hides
// the display of the password typed. It cancels reading when the context
// closes. For more options try ReadPasswordCtxFdPrompt instead. If interrupted
// by an uncaught signal during read, then this can bork your terminal.
func ReadPasswordCtx(ctx context.Context) ([]byte, error) {
	return ReadPasswordCtxFdPrompt(ctx, int(os.Stdin.Fd()), StdPrompt)
}

// ReadPasswordCtxPrompt reads a password stdin and returns the result. It hides
// the display of the password typed. It cancels reading when the context
// closes. If specified, it will prompt the user with the prompt message. If
// interrupted by an uncaught signal during read, then this can bork your
// terminal.
func ReadPasswordCtxPrompt(ctx context.Context, prompt string) ([]byte, error) {
	return ReadPasswordCtxFdPrompt(ctx, int(os.Stdin.Fd()), prompt)
}

// ReadPasswordCtxFdPrompt reads a password from the file descriptor and returns
// the result. It hides the display of the password typed. It cancels reading
// when the context closes. If specified, it will prompt the user with the
// prompt message. If interrupted by an uncaught signal during read, then this
// can bork your terminal.
func ReadPasswordCtxFdPrompt(ctx context.Context, fd int, prompt string) ([]byte, error) {

	// XXX: https://github.com/golang/go/issues/24842
	if err := syscall.SetNonblock(fd, true); err != nil {
		return nil, err
	}
	defer syscall.SetNonblock(fd, false) // TODO: is this necessary?
	file := os.NewFile(uintptr(fd), "")  // XXX: name?

	// We do some term magic to not print the password. This is taken from:
	// golang.org/x/term/term_unix.go:readPassword
	termios, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	newState := *termios
	newState.Lflag &^= unix.ECHO
	newState.Lflag |= unix.ICANON | unix.ISIG
	newState.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &newState); err != nil {
		return nil, err
	}
	defer unix.IoctlSetTermios(fd, ioctlWriteTermios, termios)

	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		file.SetReadDeadline(time.Now())
	}()

	if prompt != "" {
		fmt.Print(prompt) // prints because we only turned off echo on fd
	}

	// This previously didn't pass through the deadline. This is taken from:
	// golang.org/x/term/terminal.go:readPasswordLine
	var buf [1]byte
	var ret []byte
	for {
		n, err := file.Read(buf[:]) // unblocks on SetReadDeadline(now)
		if n > 0 {
			switch buf[0] {
			case '\b':
				if len(ret) > 0 {
					ret = ret[:len(ret)-1]
				}
			case '\n':
				if runtime.GOOS != "windows" {
					return ret, nil
				}
				// otherwise ignore \n
			case '\r': // lol
				if runtime.GOOS == "windows" {
					return ret, nil
				}
				// otherwise ignore \r
			default:
				ret = append(ret, buf[0])
			}
			continue
		}
		if e := ctx.Err(); errors.Is(err, os.ErrDeadlineExceeded) && e != nil {
			return nil, e
		}
		if err != nil {
			if err == io.EOF && len(ret) > 0 {
				return ret, nil
			}
			return ret, err // XXX: why ret and not nil?
		}
	}
}

// SaltedSHA512Password is meant to generate Ulrich Drepper's salted SHA512 unix
// crypt password hash as seen at https://www.akkadia.org/drepper/SHA-crypt.txt
// and used for /etc/shadow and anaconda kickstart files. Please read the
// following disclaimer carefully before using this: XXX: I have no idea if I am
// doing this correctly, and I have no idea if the library I am using is doing
// this correctly. Please check! Also: https://github.com/golang/go/issues/21865
func SaltedSHA512Password(password []byte) (string, error) {
	crypter := sha512Crypt.New()
	saltObj := sha512Crypt.GetSalt() // New() builds us one, this is another
	crypter.SetSalt(saltObj)
	salt := saltObj.GenerateWRounds(sha512Crypt.SaltLenMax, sha512Crypt.RoundsDefault)
	// NOTE: We could pass an empty salt and the above should happen, but we
	// wouldn't notice it if the API changed and no longer did this magic...
	hash, err := crypter.Generate(password, salt)
	if err != nil {
		return "", err
	}
	if len(hash) != 106 { // XXX: the rounds=? stuff feels not part of spec
		return "", fmt.Errorf("password was not generated correctly")
	}
	if !strings.HasPrefix(hash, sha512Crypt.MagicPrefix) {
		// programming error
		return "", fmt.Errorf("magic prefix was missing")
	}

	return hash, nil
}
