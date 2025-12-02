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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"golang.org/x/crypto/ssh"
)

func init() {
	engine.RegisterResource("ssh:authorized_key", func() engine.Res { return &SSHAuthorizedKeyRes{} })
}

// SSHAuthorizedKeyRes is a resource for managing entries in the
// ~/.ssh/authorized_keys file. It's better to use this rather than the line
// resource, because individual fields can be overridden if we export and then
// collect this. The name of this resource is the entry to add in the file. It
// can be overridden by the content field or a combination of the keytype, key,
// and an optional comment field. Those are mututally exclusive with content.
// Whichever form is used, it will be validated to check the contents are legal.
// If you use this to try and add either a comment line or an empty line, it
// will error. If you pass in each param individually, instead of as Name or
// with the Content field, then if there is a mismatch between the actual key
// and the specified type, then this will error. If you don't pass things
// individually, then we won't detect this at the moment, but this behaviour may
// change in the future.
type SSHAuthorizedKeyRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	// State is either exists or absent.
	State string `lang:"state" yaml:"state"`

	// File is the authorized_keys file to edit. By default, if unspecified,
	// this used the file at ~/.ssh/authorized_keys for the given user. You
	// can specify the user field instead of this to have that specific user
	// expanded. The ~user/ patterns will be expanded here. This takes
	// precedence over the User param (for determining the path) if that is
	// specified.
	File string `lang:"file" yaml:"file"`

	// User specifies the expansion for the file path lookup. If user is
	// specified, then the dir (if created with the Mkdir option) and the
	// authorized_keys file (if it needs to be created) will be owned by
	// this user. If the File param is specified, then this is only used for
	// ownership changes.
	User string `lang:"user" yaml:"user"`

	// Content is the line to add to the file. It will get parsed and
	// cleaned up before it is used.
	Content string `lang:"content" yaml:"content"`

	// Options are the list options for the authorized key entry. Some
	// options are single words, others take a parameter. This can't be
	// specified if you also specify Content.
	Options []string `lang:"options" yaml:"options"`

	// Type is the type of the ssh key. Common values are "ssh-ed25519" and
	// "ssh-rsa". This can't be specified if you also specify Content.
	Type string `lang:"type" yaml:"type"`

	// Key is the base64 encoded key. This can't be specified if you also
	// specify Content.
	Key string `lang:"key" yaml:"key"`

	// Comment is a comment to be stored on this line. This can't be
	// specified if you also specify Content. When specifying Type and Key,
	// specifying this is optional.
	Comment string `lang:"comment" yaml:"comment"`

	// Mkdir will make the .ssh/ directory if it doesn't already exist. This
	// only makes the immediate directory, this is not `mkdir -p`, and it
	// only does so if the HOME directory (parent of the .ssh/) already
	// exists. This never removes the .ssh/ directory if state is absent.
	// This defaults to true.
	Mkdir bool `lang:"mkdir" yaml:"mkdir"`

	// XXX: Add a "Magic" param, which if true, would edit any entry instead
	// of just adding a new one. It would do so by determining which old
	// line to edit by looking for one with the same "comment". To find that
	// line, the line resource would need a "Find" param, which takes a
	// match function that we pass in. Since this is a composite resource,
	// we can pass in the golang function directly, only future engines will
	// be able to use that field from mcl. This feature would allow us to
	// edit anything but the comment. This is useful for key/type rotation.

	// XXX: Should we allow specifying "Content" and some of the: Type, Key,
	// or Comment fields to effectively let a resource be used as a "parser"
	// by having a full field come in, and one resource collection we switch
	// out one or more of those fields while leaving the others unchanged...

	// We're using the line resource to build the resources because it's all
	// done through line.
	line *LineRes
}

// Default returns some sensible defaults for this resource.
func (obj *SSHAuthorizedKeyRes) Default() engine.Res {
	return &SSHAuthorizedKeyRes{
		Mkdir: true,
	}
}

// parseLine checks if an authorized_keys line is valid and if so, returns a
// clean version. This strips any trailing newlines. Comments are not considered
// valid and any that are prefixed will be removed. This is a helper function.
func (obj *SSHAuthorizedKeyRes) parseLine(s string) (string, error) {
	line := strings.TrimSpace(s)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", fmt.Errorf("comment or empty line")
	}

	pubKey, comment, options, rest, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return "", err
	}
	if len(rest) > 0 {
		return "", fmt.Errorf("leftover data in line")
	}
	// The parser will ignore the specified key type and read the type it
	// thinks directly out of the key. If they don't match, then error here.
	// XXX: We only notice this if you pass in each value individually.
	if obj.Type != "" && pubKey.Type() != obj.Type {
		return "", fmt.Errorf("type does not match key")
	}

	key := base64.StdEncoding.EncodeToString(pubKey.Marshal())
	out := fmt.Sprintf("%s %s", pubKey.Type(), key)
	if comment != "" {
		out += " " + comment
	}
	if len(options) > 0 {
		out = strings.Join(options, ",") + " " + out
	}

	return out, nil
}

// getLine returns the actual line that we want to use when editing the file.
func (obj *SSHAuthorizedKeyRes) getLine() (string, error) {
	if obj.Content != "" {
		return obj.parseLine(obj.Content)
	}

	if obj.Type != "" && obj.Key != "" {
		// FIXME: We glue these together, and then parse them apart
		// instead of parsing each separately first, which might be
		// better, but would be more code to write at the moment...

		out := fmt.Sprintf("%s %s", obj.Type, obj.Key)
		if obj.Comment != "" {
			out += " " + obj.Comment
		}
		if len(obj.Options) > 0 {
			out = strings.Join(obj.Options, ",") + " " + out
		}
		return obj.parseLine(out)
	}

	return obj.parseLine(obj.Name())
}

// getFile returns the path to the file that we want to edit.
func (obj *SSHAuthorizedKeyRes) getFile() (string, error) {
	f := "~/.ssh/authorized_keys" // default
	if obj.User != "" {
		f = fmt.Sprintf("~%s/.ssh/authorized_keys", obj.User)
	}
	if obj.File != "" { // takes precedence over user
		f = obj.File
	}

	// If we're exporting this, we can't expand this in Init because the
	// user won't necessarily exist or have the right $HOME directory...
	// Therefore we only expand on CheckApply/Watch when it's being used.
	//p, err := util.ExpandHome(f)
	//if err != nil {
	//	return "", err
	//}
	return f, nil
}

// makeComposite creates a pointer to a LineRes. The pointer is used to validate
// and initialize the nested resource.
func (obj *SSHAuthorizedKeyRes) makeComposite() (*LineRes, error) {
	f, err := obj.getFile()
	if err != nil {
		return nil, err
	}
	s, err := obj.getLine()
	if err != nil {
		return nil, err
	}

	res, err := engine.NewNamedResource("line", obj.Name())
	if err != nil {
		return nil, err
	}
	line := res.(*LineRes)

	line.File = f
	line.State = obj.State
	line.Content = s
	// XXX: possible future improvement
	//if obj.Magic {
	//	line.Find = func(ctx context.Context, line string) (string, error) {
	//		if strings.HasSuffix(line, " " + obj.getComment()) {
	//			return true, nil
	//		}
	//		return false, nil
	//	}
	//}

	return line, nil
}

// Validate if the params passed in are valid data.
func (obj *SSHAuthorizedKeyRes) Validate() error {
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be 'exists' or 'absent'")
	}

	//if obj.File != "" && obj.User != "" {
	//	return fmt.Errorf("can't use File and User params at the same time")
	//}
	if obj.User != "" {
		if err := util.ValidUser(obj.User); err != nil {
			return err
		}
	}

	if obj.Content != "" && len(obj.Options) > 0 {
		return fmt.Errorf("can't use Content and Options param at the same time")
	}
	if obj.Content != "" && obj.Type != "" {
		return fmt.Errorf("can't use Content and Type param at the same time")
	}
	if obj.Content != "" && obj.Key != "" {
		return fmt.Errorf("can't use Content and Key param at the same time")
	}
	if obj.Content != "" && obj.Comment != "" { // TODO: relax this if doing Magic ?
		return fmt.Errorf("can't use Content and Comment param at the same time")
	}

	if obj.Type != "" && obj.Key == "" {
		return fmt.Errorf("must use Key param when using Type param")
	}
	if obj.Type == "" && obj.Key != "" {
		return fmt.Errorf("must use Type param when using Key param")
	}

	// Validate content from whichever method specifies the values.
	if _, err := obj.getLine(); err != nil {
		return err
	}

	res, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in validate")
	}
	if err := res.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded line: %s", res)
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *SSHAuthorizedKeyRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	res, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	obj.line = res

	newInit := obj.init.Copy()
	newInit.Send = func(interface{}) error { // override so line can't send
		return nil
	}
	newInit.Logf = func(format string, v ...interface{}) {
		obj.init.Logf("line: "+format, v...) // TODO: good enough for now
	}

	return obj.line.Init(newInit)
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *SSHAuthorizedKeyRes) Cleanup() error {
	if obj.line != nil {
		return obj.line.Cleanup()
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *SSHAuthorizedKeyRes) Watch(ctx context.Context) error {
	return obj.line.Watch(ctx)
}

// fileCheckApply creates the .ssh/ directory if needed.
func (obj *SSHAuthorizedKeyRes) fileCheckApply(ctx context.Context, apply bool, file string) (bool, error) {
	if !obj.Mkdir {
		return true, nil
	}
	if obj.State != "exists" {
		return true, nil
	}

	dir := filepath.Dir(file) + "/" // */.ssh/
	d := filepath.Base(dir) + "/"   // .ssh/
	if d != ".ssh/" {               // can't do anything if it's not this pattern...
		return true, nil
	}

	if _, err := os.Stat(dir); err == nil {
		return true, nil // already exists
	} else if err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "could not stat dir")
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// create the empty directory
	if err := os.Mkdir(dir, 0700); err != nil {
		return false, err
	}
	obj.init.Logf("mkdir %s", dir)

	return false, obj.chown(dir)
}

// CheckApply method for SSHAuthorizedKey resource.
func (obj *SSHAuthorizedKeyRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	//obj.init.Logf("%s", obj.line.?) // "check"

	checkOK := true

	p, err := util.ExpandHome(obj.line.File)
	if err != nil {
		return false, err
	}

	if c, err := obj.fileCheckApply(ctx, apply, p); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// Check if file exists before we run the line resource...
	exists := false
	if _, err := os.Stat(p); err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "could not stat file")
	} else if err == nil {
		exists = true
	}

	if c, err := obj.line.CheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
		// We only chown if we probably created the file. Because we
		// don't want to potentially fight with a file resource which is
		// doing more precisely what the user asked for.
		if !exists { // we must have created it, so chown it...
			if err := obj.chown(p); err != nil {
				return false, err
			}
		}
	}

	if !checkOK {
		//obj.init.Logf("%s", obj.line.?) // "apply"
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *SSHAuthorizedKeyRes) Cmp(r engine.Res) error {
	// we can only compare SSHAuthorizedKeyRes to others of the same resource kind
	res, ok := r.(*SSHAuthorizedKeyRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	if obj.File != res.File {
		return fmt.Errorf("the File differs")
	}
	if obj.User != res.User {
		return fmt.Errorf("the User differs")
	}

	if obj.Content != res.Content {
		return fmt.Errorf("the Content differs")
	}

	if (obj.Options == nil) != (res.Options == nil) {
		return fmt.Errorf("the Options differ")
	}
	if obj.Options != nil && res.Options != nil {
		if len(obj.Options) != len(res.Options) {
			return fmt.Errorf("the Options differ")
		}
		for i := range obj.Options {
			if obj.Options[i] != res.Options[i] {
				return fmt.Errorf("the Options differ at index: %d", i)
			}
		}
	}

	if obj.Type != res.Type {
		return fmt.Errorf("the Type differs")
	}
	if obj.Key != res.Key {
		return fmt.Errorf("the Key differs")
	}
	if obj.Comment != res.Comment {
		return fmt.Errorf("the Comment differs")
	}

	if obj.Mkdir != res.Mkdir {
		return fmt.Errorf("the Mkdir differs")
	}

	// TODO: why is res.line ever nil?
	if (obj.line == nil) != (res.line == nil) { // xor
		return fmt.Errorf("the line differs")
	}
	if obj.line != nil && res.line != nil {
		if err := obj.line.Cmp(res.line); err != nil {
			return errwrap.Wrapf(err, "the line differs")
		}
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *SSHAuthorizedKeyRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes SSHAuthorizedKeyRes // indirection to avoid infinite recursion

	def := obj.Default()                  // get the default
	res, ok := def.(*SSHAuthorizedKeyRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to SSHAuthorizedKeyRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = SSHAuthorizedKeyRes(raw) // restore from indirection with type conversion!
	return nil
}

// chown is a general utility to chown the path in question.
func (obj *SSHAuthorizedKeyRes) chown(p string) error {
	if obj.User == "" {
		return nil // can't do anything
	}

	fileInfo, err := os.Stat(p)
	if err != nil { // if the file does not exist, it's correct to error!
		return err
	}

	stUnix, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		// not unix
		return fmt.Errorf("can't set Owner or Group on this platform")
	}

	var expectedUID, expectedGID int

	if obj.User != "" {
		expectedUID, err = engineUtil.GetUID(obj.User)
		if err != nil {
			return err
		}
	} else {
		// nothing specified, no changes to be made, expect same as actual
		expectedUID = int(stUnix.Uid)
	}
	//if obj.Group != "" {
	//	expectedGID, err = engineUtil.GetGID(obj.Group)
	//	if err != nil {
	//		return false, err
	//	}
	//} else {
	//	// nothing specified, no changes to be made, expect same as actual
	expectedGID = int(stUnix.Gid)
	//}

	// nothing to do
	if int(stUnix.Uid) == expectedUID && int(stUnix.Gid) == expectedGID {
		return nil
	}

	obj.init.Logf("chown %s", p)
	return os.Chown(p, expectedUID, expectedGID)
}
