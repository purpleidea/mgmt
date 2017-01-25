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
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/recwatch"

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
	Length        uint16  `yaml:"length"` // number of characters to return
	Saved         bool    // this caches the password in the clear locally
	CheckRecovery bool    // recovery from integrity checks by re-generating
	Password      *string // the generated password, read only, do not set!

	path       string // the path to local storage
	recWatcher *recwatch.RecWatcher
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

// Default returns some sensible defaults for this resource.
func (obj *PasswordRes) Default() Res {
	return &PasswordRes{
		Length: 64, // safe default
	}
}

// Validate if the params passed in are valid data.
func (obj *PasswordRes) Validate() error {
	return obj.BaseRes.Validate()
}

// Init generates a new password for this resource if one was not provided. It
// will save this into a local file. It will load it back in from previous runs.
func (obj *PasswordRes) Init() error {
	obj.BaseRes.kind = "Password" // must be set before using VarDir

	dir, err := obj.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.path = path.Join(dir, "password") // return a unique file

	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

func (obj *PasswordRes) read() (string, error) {
	file, err := os.Open(obj.path) // open a handle to read the file
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
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
	var c int
	if c, err = file.Write([]byte(password + newline)); err != nil {
		return c, errwrap.Wrapf(err, "can't write file")
	}
	return c, file.Sync()
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

	if !obj.Saved && length == 0 { // expecting an empty string
		return nil
	}
	if !obj.Saved && length != 0 { // should have no stored password
		return fmt.Errorf("Expected empty token only!")
	}

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

// Watch is the primary listener for this resource and it outputs events.
func (obj *PasswordRes) Watch(processChan chan *event.Event) error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(obj.path, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	// notify engine that we're running
	if err := obj.Running(processChan); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error
	for {
		select {
		// NOTE: this part is very similar to the file resource code
		case event, ok := <-obj.recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s[%s] watcher error", obj.Kind(), obj.GetName())
			}
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event(processChan)
		}
	}
}

// CheckApply method for Password resource. Does nothing, returns happy!
func (obj *PasswordRes) CheckApply(apply bool) (checkOK bool, err error) {

	var refresh = obj.Refresh() // do we have a pending reload to apply?
	var exists = true           // does the file (aka the token) exist?
	var generate bool           // do we need to generate a new password?
	var write bool              // do we need to write out to disk?

	password, err := obj.read() // password might be empty if just a token
	if err != nil {
		if !os.IsNotExist(err) {
			return false, errwrap.Wrapf(err, "unknown read error")
		}
		exists = false
	}

	if exists {
		if err := obj.check(password); err != nil {
			if !obj.CheckRecovery {
				return false, errwrap.Wrapf(err, "check failed")
			}
			log.Printf("%s[%s]: Integrity check failed", obj.Kind(), obj.GetName())
			generate = true // okay to build a new one
			write = true    // make sure to write over the old one
		}
	} else { // doesn't exist, write one
		write = true
	}

	// if we previously had !obj.Saved, and now we want it, we re-generate!
	if refresh || !exists || (obj.Saved && password == "") {
		generate = true
	}

	// stored password isn't consistent with memory
	if p := obj.Password; obj.Saved && (p != nil && *p != password) {
		write = true
	}

	if !refresh && exists && !generate && !write { // nothing to do, done!
		return true, nil
	}
	// a refresh was requested, the token doesn't exist, or the check failed

	if !apply {
		return false, nil
	}

	if generate {
		// we'll need to write this out...
		if obj.Saved || (!obj.Saved && password != "") {
			write = true
		}
		// generate the actual password
		var err error
		log.Printf("%s[%s]: Generating new password...", obj.Kind(), obj.GetName())
		if password, err = obj.generate(); err != nil { // generate one!
			return false, errwrap.Wrapf(err, "could not generate password")
		}
	}

	obj.Password = &password // save in memory

	var output string // the string to write out

	// if memory value != value on disk, save it
	if write {
		if obj.Saved { // save password as clear text
			// TODO: would it make sense to encrypt this password?
			output = password
		}
		// write either an empty token, or the password
		log.Printf("%s[%s]: Writing password token...", obj.Kind(), obj.GetName())
		if _, err := obj.write(output); err != nil {
			return false, errwrap.Wrapf(err, "can't write to file")
		}
	}

	return false, nil
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

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PasswordRes) UIDs() []ResUID {
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
		return false
	}
	return false // TODO: this is doable, but probably not very useful
	// TODO: it could be useful to group our tokens into a single write, and
	// as a result, we save inotify watches too!
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
		if obj.Length != res.Length {
			return false
		}
		// TODO: we *could* optimize by allowing CheckApply to move from
		// saved->!saved, by removing the file, but not likely worth it!
		if obj.Saved != res.Saved {
			return false
		}
		if obj.CheckRecovery != res.CheckRecovery {
			return false
		}
	default:
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *PasswordRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes PasswordRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*PasswordRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to PasswordRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = PasswordRes(raw) // restore from indirection with type conversion!
	return nil
}
