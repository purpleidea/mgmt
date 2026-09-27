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

package resources

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("password", func() engine.Res { return &PasswordRes{} })
}

const (
	newline = "\n" // the standard newline
)

// PasswordRes is a no-op resource that generates a random password string. It
// sends the generated password to other resources via send/recv, and also
// publishes it over the local Bridge API so that it can be read back into the
// language with the res/password value function, for example:
// password.value("name") <|> "hunter2". That function returns a catchable error
// until we've generated a password, which lets the except operator supply a
// fallback in the meantime.
type PasswordRes struct {
	traits.Base // add the base methods without re-implementation
	// TODO: it could be useful to group our tokens into a single write, and
	// as a result, we save inotify watches too!
	//traits.Groupable // TODO: this is doable, but probably not very useful
	traits.Refreshable
	traits.Sendable

	init *engine.Init

	// Length is the number of characters to return. If you choose 0, then
	// it is an error.
	// FIXME: is uint16 too big?
	Length uint16 `lang:"length" yaml:"length"`

	// Alphabet lets you specify the list of characters to use when
	// generating a password. You may not include the newline. If you have
	// duplicates, then statistics dictates that they will be more frequent
	// in your password, we do not prevent this. If you do not specify this
	// or if it is empty, then we will use a default "simple" alphabet.
	Alphabet string `lang:"alphabet" yaml:"alphabet"`

	// Write stores the password in the clear on disk. Without this your
	// password will be ephemeral for the run of this resource. Once it is
	// graph swapped away and back, that password will be gone. This also
	// happens when you restart the mgmt process. If you want a password
	// which is generated once and then kept across runs, set this to true.
	Write bool `lang:"write" yaml:"write"`

	// Newline spits out a newline at the end of the password. Useful if we
	// are using send/recv to write it to a file, or when passing to a tool
	// that expects a newline termination.
	Newline bool `lang:"newline" yaml:"newline"`

	// CheckRecovery specifies that we should recover from, regenerate, and
	// carry on casually without erroring the resource if the "check"
	// facility fails. This can happen when loading a saved password from
	// disk which is not of the expected length. In this case, we'd discard
	// the old saved password and create a new one without erroring. This is
	// useful if you re-use a password resource and are changing the length.
	CheckRecovery bool `lang:"check_recovery" yaml:"check_recovery"`

	password string // when not stored on disk
	path     string // the path to local storage
}

// Default returns some sensible defaults for this resource.
func (obj *PasswordRes) Default() engine.Res {
	return &PasswordRes{
		Length: 64, // safe default
	}
}

// Validate if the params passed in are valid data.
func (obj *PasswordRes) Validate() error {
	if obj.Length == 0 {
		return fmt.Errorf("password length is 0") // not allowed for security
	}
	if strings.Contains(obj.Alphabet, newline) {
		return fmt.Errorf("the Alphabet contains a newline")
	}

	return nil
}

// Init runs some startup code for this resource. It generates a new password
// for this resource if one was not provided. It will save this into a local
// file. It will load it back in from previous runs.
func (obj *PasswordRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.path = path.Join(dir, "password") // return a unique file

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *PasswordRes) Cleanup() error {
	// Unpublish our password from the local Bridge API so that any consumer
	// of the res/password value function reverts to erroring (and can pick
	// up its except fallback) once we're no longer running.
	return obj.init.Local.BridgeSet(context.Background(), obj.Kind(), obj.Name(), nil)
}

// publish sends the generated password to the local Bridge API so that the
// res/password value function can read it back. We use our resource kind as the
// namespace and our name as the uid. We publish the exact same value that we
// send over send/recv (so it honours the Newline param) to avoid the surprise
// of the function and send/recv disagreeing about our output. An empty password
// is never published, so that consumers keep erroring (and can use the except
// operator to fall back) until we've actually generated one.
func (obj *PasswordRes) publish(ctx context.Context, password string) error {
	if password == "" {
		return nil // nothing valid to publish yet
	}
	return obj.init.Local.BridgeSet(ctx, obj.Kind(), obj.Name(), password)
}

// read is a helper to read the data from disk. This is similar to an engineUtil
// function named ReadData but is kept separate for safety anyways.
func (obj *PasswordRes) read() (string, error) {
	file, err := os.Open(obj.path) // open a handle to read the file
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", errwrap.Wrapf(err, "could not read from file")
	}
	return strings.TrimSuffix(string(data), newline), nil
}

// write is a helper to store the data on disk. This is similar to an engineUtil
// function named WriteData but is kept separate for safety anyways.
func (obj *PasswordRes) write(password string) (int, error) {
	uid, gid, err := engineUtil.GetUIDGID()
	if err != nil {
		return -1, err
	}

	// Chmod it before we write the secret data.
	file, err := os.OpenFile(obj.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return -1, errwrap.Wrapf(err, "can't create file")
	}
	defer file.Close()

	// Chown it before we write the secret data.
	if err := file.Chown(uid, gid); err != nil {
		return -1, err
	}

	c, err := file.WriteString(password + newline)
	if err != nil {
		return c, errwrap.Wrapf(err, "can't write file")
	}
	return c, file.Sync()
}

// alphabet returns the alphabet we use for this resource.
func (obj *PasswordRes) alphabet() string {
	if obj.Alphabet != "" {
		return obj.Alphabet
	}

	return util.RandomStringSimpleAlphabet
}

// generate generates a new password.
func (obj *PasswordRes) generate() (string, error) {
	output, err := util.RandomStringAlphabet(obj.Length, obj.alphabet())
	if err != nil {
		return "", errwrap.Wrapf(err, "could not generate password")
	}

	if output == "" { // safety against empty passwords
		return "", fmt.Errorf("password is empty")
	}

	return output, nil
}

// check validates a stored password string
func (obj *PasswordRes) check(value string) error {
	length := len(value)

	if length == 0 { // invalid password
		return fmt.Errorf("password is empty")
	}

	if length != int(obj.Length) {
		return fmt.Errorf("string length is not %d", obj.Length)
	}
	alphabet := obj.alphabet()
Loop:
	for i := 0; i < length; i++ {
		for j := 0; j < len(alphabet); j++ {
			if value[i] == alphabet[j] {
				continue Loop
			}
		}
		// we couldn't find that character, so error!
		return fmt.Errorf("invalid character `%s`", string(value[i]))
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PasswordRes) Watch(ctx context.Context) error {
	if !obj.Write {
		if err := obj.init.Event(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done(): // closed by the engine to signal shutdown
		}

		return ctx.Err()
	}

	recWatcher, err := recwatch.NewRecWatcher(ctx, obj.path, false)
	if err != nil {
		return err
	}
	defer recWatcher.Cleanup()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		// NOTE: this part is very similar to the file resource code
		case event, ok := <-recWatcher.Events():
			if ctx.Err() != nil {
				return ctx.Err() // engine is shutting us down
			}
			if !ok { // channel shutdown
				return nil
			}
			if event == nil {
				// programming error
				return fmt.Errorf("unexpected nil recwatch event")
			}
			if err := event.Error; err != nil { // might be context.Canceled
				return err
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply method for Password resource. Does nothing, returns happy!
func (obj *PasswordRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	var refresh = obj.init.Refresh() // do we have a pending reload to apply?
	var exists bool                  // do we already have a password?
	var generate bool                // do we need to generate a new password?

	if obj.Write {
		obj.password = ""           // reset in case there's stale data
		password, err := obj.read() // password might be empty if just a token
		if err != nil && !os.IsNotExist(err) {
			return false, errwrap.Wrapf(err, "unknown read error")
		}
		if err == nil {
			obj.password = password // load
			exists = true
		}
	} else {
		// An ephemeral password lives in memory for the lifetime of
		// this resource so don't generate a new one on each CheckApply.
		exists = obj.password != ""
	}

	if exists {
		if err := obj.check(obj.password); err != nil {
			if !obj.CheckRecovery {
				return false, errwrap.Wrapf(err, "check failed")
			}
			obj.init.Logf("integrity check failed")
			generate = true // okay to build a new one
		}
	}

	if refresh || !exists || obj.password == "" {
		generate = true
	}

	if !refresh && exists && !generate { // nothing to do, done!
		p := obj.password
		if obj.Newline {
			p += newline
		}
		if err := obj.init.Send(&PasswordSends{
			Password: &p,
		}); err != nil {
			return false, err
		}
		if err := obj.publish(ctx, p); err != nil {
			return false, err
		}
		return true, nil
	}
	// a refresh was requested, the token doesn't exist, or the check failed

	if !apply {
		p := obj.password
		if obj.Newline {
			p += newline
		}
		if err := obj.init.Send(&PasswordSends{
			Password: &p, // XXX: arbitrary since we're in noop mode
		}); err != nil {
			return false, err
		}
		return false, nil
	}

	if generate {
		// generate the actual password
		obj.init.Logf("generating new password...")
		password, err := obj.generate()
		if err != nil { // generate one!
			return false, errwrap.Wrapf(err, "could not generate password")
		}
		obj.password = password // store
	}

	if obj.Write {
		// TODO: would it make sense to encrypt this password?
		if _, err := obj.write(obj.password); err != nil {
			return false, errwrap.Wrapf(err, "can't write to file")
		}
	}

	// send
	p := obj.password
	if obj.Newline {
		p += newline
	}
	if err := obj.init.Send(&PasswordSends{
		Password: &p,
	}); err != nil {
		return false, err
	}
	if err := obj.publish(ctx, p); err != nil {
		return false, err
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *PasswordRes) Cmp(r engine.Res) error {
	// we can only compare PasswordRes to others of the same resource kind
	res, ok := r.(*PasswordRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Length != res.Length {
		return fmt.Errorf("the Length differs")
	}
	if obj.Alphabet != res.Alphabet {
		return fmt.Errorf("the Alphabet differs")
	}
	if obj.Write != res.Write {
		return fmt.Errorf("the Write differs")
	}
	if obj.Newline != res.Newline {
		return fmt.Errorf("the Newline differs")
	}
	if obj.CheckRecovery != res.CheckRecovery {
		return fmt.Errorf("the CheckRecovery differs")
	}

	return nil
}

// PasswordUID is the UID struct for PasswordRes.
type PasswordUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *PasswordRes) UIDs() []engine.ResUID {
	x := &PasswordUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// PasswordSends is the struct of data which is sent after a successful Apply.
type PasswordSends struct {
	// Password is the generated password being sent.
	Password *string `lang:"password"`
	// Hashing is the algorithm used for this password. Empty is plain text.
	Hashing string // TODO: implement me
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *PasswordRes) Sends() interface{} {
	return &PasswordSends{
		Password: nil,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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
