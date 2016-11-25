// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"math/big"
	"os"
	"path"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/event"

	errwrap "github.com/pkg/errors"
)

func init() {
	gob.Register(&PasswordRes{})
}

const (
	alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	newline  = "\n" // something not in alphabet that TrimSpace can trim
)

// PasswordRes is a no-op resource that returns a random password string.
type PasswordRes struct {
	BaseRes `yaml:",inline"`
	// FIXME: is uint16 too big?
	Length   uint16  `yaml:"length"` // number of characters to return
	Password *string // the generated password

	path string // the path to local storage
}

// NewPasswordRes is a constructor for this resource. It also calls Init() for you.
func NewPasswordRes(name string, length uint16) (*PasswordRes, error) {
	obj := &PasswordRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Length: length,
	}
	return obj, obj.Init()
}

func (obj *PasswordRes) read() (string, error) {
	file, err := os.Open(obj.path) // open a handle to read the file
	if err != nil {
		return "", errwrap.Wrapf(err, "could not read password")
	}
	defer file.Close()
	data := make([]byte, obj.Length+uint16(len(newline))) // data + newline
	if _, err := file.Read(data); err != nil {
		return "", errwrap.Wrapf(err, "could not read from file")
	}
	return strings.TrimSpace(string(data)), nil
}

func (obj *PasswordRes) write(password string) (int, error) {
	file, err := os.Create(obj.path) // open a handle to create the file
	if err != nil {
		return -1, errwrap.Wrapf(err, "can't create file")
	}
	defer file.Close()
	return file.Write([]byte(password + newline))
}

// generate generates a new password.
func (obj *PasswordRes) generate() (string, error) {
	max := len(alphabet) - 1 // last index
	output := ""

	// FIXME: have someone verify this is cryptographically secure & correct
	for i := uint16(0); i < obj.Length; i++ {
		big, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
		if err != nil {
			return "", errwrap.Wrapf(err, "could not generate password")
		}
		ix := big.Int64()
		output += string(alphabet[ix])
	}

	if output == "" { // safety against empty passwords
		return "", fmt.Errorf("password is empty")
	}

	if uint16(len(output)) != obj.Length { // safety against weird bugs
		return "", fmt.Errorf("password length is too short") // bug!
	}

	return output, nil
}

// check validates a stored password string
func (obj *PasswordRes) check(value string) error {
	length := uint16(len(value))
	if length != obj.Length {
		return fmt.Errorf("String length is not %d", obj.Length)
	}
Loop:
	for i := uint16(0); i < length; i++ {
		for j := 0; j < len(alphabet); j++ {
			if value[i] == alphabet[j] {
				continue Loop
			}
		}
		// we couldn't find that character, so error!
		return fmt.Errorf("Invalid character `%s`", string(value[i]))
	}
	return nil
}

// Init generates a new password for this resource if one was not provided. It
// will save this into a local file. It will load it back in from previous runs.
func (obj *PasswordRes) Init() error {
	// XXX: eventually store a hash instead of the plain text! we might want
	// to generate a new value on fresh run if the downstream resource needs
	// an update (triggers a backpoke?) this is a POC for send/recv for now.
	obj.BaseRes.kind = "Password" // must be set before using VarDir

	dir, err := obj.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}

	obj.path = path.Join(dir, "password") // return a unique file
	password := ""
	if _, err := os.Stat(obj.path); err != nil { // probably doesn't exist
		if !os.IsNotExist(err) {
			return errwrap.Wrapf(err, "unknown stat error")
		}

		// generate password and store it in the file
		if obj.Password != nil {
			password = *obj.Password // reuse what we've got
		} else {
			var err error
			if password, err = obj.generate(); err != nil { // generate one!
				return errwrap.Wrapf(err, "could not init password")
			}
		}

		// store it to disk
		if _, err := obj.write(password); err != nil {
			return errwrap.Wrapf(err, "can't write to file")
		}

	} else { // must exist already!

		password, err := obj.read()
		if err != nil {
			return errwrap.Wrapf(err, "could not read password")
		}
		if err := obj.check(password); err != nil {
			return errwrap.Wrapf(err, "check failed")
		}

		if p := obj.Password; p != nil && *p != password {
			// stored password isn't consistent with memory
			if _, err := obj.write(*p); err != nil {
				return errwrap.Wrapf(err, "consistency overwrite failed")
			}
			password = *p // use the copy from the resource
		}
	}

	obj.Password = &password // save in memory

	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Validate if the params passed in are valid data.
// FIXME: where should this get called ?
func (obj *PasswordRes) Validate() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PasswordRes) Watch(processChan chan event.Event) error {
	if obj.IsWatching() {
		return nil // TODO: should this be an error?
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuid := obj.converger.Register()
	defer cuid.Unregister()

	var startup bool
	Startup := func(block bool) <-chan time.Time {
		if block {
			return nil // blocks forever
			//return make(chan time.Time) // blocks forever
		}
		return time.After(time.Duration(500) * time.Millisecond) // 1/2 the resolution of converged timeout
	}

	var send = false // send event?
	var exit = false
	for {
		obj.SetState(ResStateWatching) // reset
		select {
		case event := <-obj.Events():
			cuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}

		case <-cuid.ConvergedTimer():
			cuid.SetConverged(true) // converged!
			continue

		case <-Startup(startup):
			cuid.SetConverged(false)
			send = true
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			startup = true // startup finished
			send = false
			if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// CheckApply method for Password resource. Does nothing, returns happy!
func (obj *PasswordRes) CheckApply(apply bool) (checkOK bool, err error) {
	return true, nil
}

// PasswordUID is the UID struct for PasswordRes.
type PasswordUID struct {
	BaseUID
	name string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *PasswordRes) AutoEdges() AutoEdge {
	return nil
}

// GetUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PasswordRes) GetUIDs() []ResUID {
	x := &PasswordUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *PasswordRes) GroupCmp(r Res) bool {
	_, ok := r.(*PasswordRes)
	if !ok {
		// NOTE: technically we could group a noop into any other
		// resource, if that resource knew how to handle it, although,
		// since the mechanics of inter-kind resource grouping are
		// tricky, avoid doing this until there's a good reason.
		return false
	}
	return true // noop resources can always be grouped together!
}

// Compare two resources and return if they are equivalent.
func (obj *PasswordRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare PasswordRes to others of the same resource
	case *PasswordRes:
		res := res.(*PasswordRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

		if obj.Name != res.Name {
			return false
		}
	default:
		return false
	}
	return true
}
