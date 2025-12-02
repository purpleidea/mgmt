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
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("line", func() engine.Res { return &LineRes{} })
}

const (
	// LineStateExists is the string that represents that the line should be
	// present.
	LineStateExists = "exists"

	// LineStateAbsent is the string that represents that the line should
	// not exist.
	LineStateAbsent = "absent"
)

// LineRes is a simple resource that adds or removes a line of text from a file.
// For more complicated control over the file, use the regular File resource.
type LineRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// File is the absolute path to the file that we are managing. If this
	// contains a ~user/foo/ or ~/foo/ type directory, then expand it.
	// TODO: Allow the Name to be something like ${path}:some-contents ?
	File string `lang:"file" yaml:"file"`

	// State specifies the desired state of the line. It can be either
	// `exists` or `absent`. If you do not specify this, we will not be able
	// to create or remove a line.
	State string `lang:"state" yaml:"state"`

	// Content specifies the line contents to add or remove. If this is
	// empty, then it does nothing.
	Content string `lang:"content" yaml:"content"`

	// Trim specifies that we will trim any whitespace from the beginning
	// and end of the content. This makes it easier to pass in data from a
	// file that ends with a newline, and avoid adding an unnecessary blank.
	Trim bool `lang:"trim" yaml:"trim"`

	// TODO: consider adding top or bottom insertion preferences?
	// TODO: consider adding duplicate removal preferences?
}

// getContent is a simple helper to apply the trim field to the content.
func (obj *LineRes) getContent() string {
	if !obj.Trim {
		return obj.Content
	}
	return strings.TrimSpace(obj.Content)
}

// getFile is a helper to get the actual file to use.
func (obj *LineRes) getFile() (string, error) {
	return util.ExpandHome(obj.File)
}

// Default returns some sensible defaults for this resource.
func (obj *LineRes) Default() engine.Res {
	return &LineRes{}
}

// Validate if the params passed in are valid data.
func (obj *LineRes) Validate() error {

	if !strings.HasPrefix(obj.File, "/") && !strings.HasPrefix(obj.File, "~/") {
		return fmt.Errorf("the File must be absolute")
	}
	if strings.HasSuffix(obj.File, "/") {
		return fmt.Errorf("the File must not end with a slash")
	}

	if obj.State != LineStateExists && obj.State != LineStateAbsent {
		return fmt.Errorf("the State is invalid")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *LineRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *LineRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *LineRes) Watch(ctx context.Context) error {
	f, err := obj.getFile()
	if err != nil {
		return err
	}

	recWatcher, err := recwatch.NewRecWatcher(f, false)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	for {
		if obj.init.Debug {
			obj.init.Logf("watching: %s", f) // attempting to watch...
		}

		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply method for Value resource. Does nothing, returns happy!
func (obj *LineRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.getContent() == "" { // special case
		return true, nil // done early
	}

	exists, err := obj.check(ctx)
	if err != nil {
		return false, err
	}

	if obj.State == LineStateExists && exists {
		return true, nil
	}
	if obj.State == LineStateAbsent && !exists {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if obj.State == LineStateAbsent { // remove
		obj.init.Logf("removing line")
		return obj.remove(ctx)
	}

	//if obj.State == LineStateExists { // add
	//}
	obj.init.Logf("adding line")
	return obj.add(ctx)
}

// check returns true if it found a match. false otherwise. It errors if
// something went permanently wrong. If the file doesn't exist, this returns
// false.
func (obj *LineRes) check(ctx context.Context) (bool, error) {
	matchLines := strings.Split(obj.getContent(), "\n")
	f, err := obj.getFile()
	if err != nil {
		return false, err
	}

	file, err := os.Open(f)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	// XXX: make a streaming version of this function without this cache
	var fileLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		fileLines = append(fileLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}

	// XXX: add tests to make sure this is correct
	for i := 0; i <= len(fileLines)-len(matchLines); i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		match := true
		for j := 0; j < len(matchLines); j++ {
			if fileLines[i+j] != matchLines[j] {
				match = false
				break
			}
		}
		if match {
			return true, nil // end early, we found a match!
		}
	}

	return false, nil
}

// remove returns true if it did nothing. false if it removed a match. It errors
// if something went permanently wrong.
func (obj *LineRes) remove(ctx context.Context) (bool, error) {
	matchLines := strings.Split(obj.getContent(), "\n")
	f, err := obj.getFile()
	if err != nil {
		return false, err
	}

	file, err := os.Open(f)
	if err != nil {
		return false, err
	}

	var fileLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		fileLines = append(fileLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		file.Close() // don't leak
		return false, err
	}
	file.Close() // close before we eventually write

	// check if the last line ends with a newline
	nl := ""
	if len(fileLines) > 0 && strings.HasSuffix(fileLines[len(fileLines)-1], "\n") {
		nl = "\n"
	}

	// XXX: add tests to make sure this is correct
	var newLines []string
	i := 0
	count := 0
	for i < len(fileLines) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		match := true
		if i+len(matchLines) <= len(fileLines) {
			for j := 0; j < len(matchLines); j++ {
				if fileLines[i+j] != matchLines[j] {
					match = false
					break
				}
			}
		} else {
			match = false
		}

		if match {
			i += len(matchLines)     // skip over the matched block
			count += len(matchLines) // count the skips
		} else {
			newLines = append(newLines, fileLines[i])
			i++
		}
	}

	if count == 0 {
		return true, nil // nothing removed!
	}

	// write out the updated file
	output := strings.Join(newLines, "\n") + nl // preserve newline at EOF
	return false, os.WriteFile(f, []byte(output), 0600)
}

// add returns true if it did nothing. false if it add a line. It errors if
// something went permanently wrong. It's not strictly required for it to avoid
// adding duplicates, but it's a nice feature, hence why it can return true.
// TODO: add at beginning or at end of file?
// XXX: do the duplicate check at the same time?
func (obj *LineRes) add(ctx context.Context) (bool, error) {
	f, err := obj.getFile()
	if err != nil {
		return false, err
	}

	file, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return false, err
	}
	defer file.Close()

	if _, err := file.WriteString(obj.getContent() + "\n"); err != nil {
		return false, err
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *LineRes) Cmp(r engine.Res) error {
	// we can only compare LineRes to others of the same resource kind
	res, ok := r.(*LineRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.File != res.File {
		return fmt.Errorf("the File field differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State field differs")
	}
	if obj.Content != res.Content {
		return fmt.Errorf("the Content field differs")
	}
	// TODO: We could technically compare obj.getContent() instead...
	if obj.Trim != res.Trim {
		return fmt.Errorf("the Trim field differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *LineRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes LineRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*LineRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to LineRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = LineRes(raw) // restore from indirection with type conversion!
	return nil
}
