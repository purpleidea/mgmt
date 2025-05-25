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
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("gzip", func() engine.Res { return &GzipRes{} })

	// const.res.gzip.level.no_compression = 0
	// const.res.gzip.level.best_speed = 1
	// const.res.gzip.level.best_compression = 9
	// const.res.gzip.level.default_compression = -1
	// const.res.gzip.level.huffman_only = -2
	vars.RegisterResourceParams("gzip", map[string]map[string]func() interfaces.Var{
		"level": {
			"no_compression": func() interfaces.Var {
				return &types.IntValue{
					V: gzip.NoCompression,
				}
			},
			"best_speed": func() interfaces.Var {
				return &types.IntValue{
					V: gzip.BestSpeed,
				}
			},
			"best_compression": func() interfaces.Var {
				return &types.IntValue{
					V: gzip.BestCompression,
				}
			},
			"default_compression": func() interfaces.Var {
				return &types.IntValue{
					V: gzip.DefaultCompression,
				}
			},
			"huffman_only": func() interfaces.Var {
				return &types.IntValue{
					V: gzip.HuffmanOnly,
				}
			},
		},
	})
}

// GzipRes is a resource that compresses a path or some raw data using gzip. The
// name of the resource is the path to the resultant compressed file. The input
// can either come from a file path if specified with Input or it looks at the
// Content field for raw data. It uses hashes to determine if something was
// changed, so as a result, this may not be suitable if you can create a sha256
// hash collision.
// TODO: support send/recv to send the output instead of writing to a file?
type GzipRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Path, which defaults to the name if not specified, represents the
	// destination path for the compressed file being created. It must be an
	// absolute path, and as a result must start with a slash. Since it is a
	// file, it must not end with a slash.
	Path string `lang:"path" yaml:"path"`

	// Input represents the input file to be compressed. It must be an
	// absolute path, and as a result must start with a slash. Since it is a
	// file, it must not end with a slash. If this is specified, we use it,
	// otherwise we use the Content parameter.
	Input *string `lang:"input" yaml:"input"`

	// Content is the raw data to compress. If Input is not specified, then
	// we use this parameter. If you forget to specify both of these, then
	// you will compress zero-length data!
	// TODO: If this is also empty should we just error at Validate?
	// FIXME: Do we need []byte here? Do we need a binary type?
	Content string `lang:"content" yaml:"content"`

	// Level is the compression level to use. If you change this, then the
	// file will get recompressed. The available values are:
	// const.res.gzip.level.no_compression, const.res.gzip.level.best_speed,
	// const.res.gzip.level.best_compression,
	// const.res.gzip.level.default_compression, and
	// const.res.gzip.level.huffman_only.
	Level int `lang:"level" yaml:"level"`

	// SendOnly specifies that we don't write the file to disk, and as a
	// result, the output is only be accessible by the send/recv mechanism.
	// TODO: Rename this?
	// TODO: Not implemented
	//SendOnly bool `lang:"sendonly" yaml:"sendonly"`

	// sha256sum is the hash of the content if it's using obj.Content here.
	sha256sum string

	// varDirPathInput is the path we use to store the content hash.
	varDirPathInput string

	// varDirPathOutput is the path we use to store the output file hash.
	varDirPathOutput string
}

// getPath returns the actual path to use for this resource. It computes this
// after analysis of the Path and Name.
func (obj *GzipRes) getPath() string {
	p := obj.Path
	if obj.Path == "" { // use the name as the path default if missing
		p = obj.Name()
	}
	return p
}

// Default returns some sensible defaults for this resource.
func (obj *GzipRes) Default() engine.Res {
	return &GzipRes{
		Level: gzip.DefaultCompression,
	}
}

// Validate if the params passed in are valid data.
func (obj *GzipRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("path is empty")
	}
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("path must be absolute")
	}
	if strings.HasSuffix(obj.getPath(), "/") {
		return fmt.Errorf("path must not end with a slash")
	}

	if obj.Input != nil {
		if !strings.HasPrefix(*obj.Input, "/") {
			return fmt.Errorf("input must be absolute")
		}
		if strings.HasSuffix(*obj.Input, "/") {
			return fmt.Errorf("input must not end with a slash")
		}
	}

	// This validation logic was observed in the gzip source code.
	if obj.Level < gzip.HuffmanOnly || obj.Level > gzip.BestCompression {
		return fmt.Errorf("invalid compression level: %d", obj.Level)
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *GzipRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	// return unique files
	obj.varDirPathInput = path.Join(dir, "input.sha256")
	obj.varDirPathOutput = path.Join(dir, "output.sha256")

	if obj.Input != nil {
		return nil
	}

	// This is all stuff that's done when we're using obj.Content instead...
	sha256sum, err := obj.hashContent(strings.NewReader(obj.Content))
	if err != nil {
		return err
	}
	obj.sha256sum = sha256sum

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *GzipRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *GzipRes) Watch(ctx context.Context) error {
	recurse := false // single file

	recWatcher, err := recwatch.NewRecWatcher(obj.getPath(), recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	var events chan recwatch.Event

	if obj.Input != nil {
		recWatcher, err := recwatch.NewRecWatcher(*obj.Input, recurse)
		if err != nil {
			return err
		}
		defer recWatcher.Close()
		events = recWatcher.Events()
	}

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				// TODO: Should this be an error? Previously it
				// was a `return nil`, and i'm not sure why...
				//return nil
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case event, ok := <-events:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return err
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

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// This is where we actually do the compression work when needed.
func (obj *GzipRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	h1, err := obj.hashFile(obj.getPath()) // output
	if err != nil {
		return false, err
	}

	h2, err := obj.readHashFile(obj.varDirPathOutput)
	if err != nil {
		return false, err
	}

	i1 := obj.sha256sum
	if obj.Input != nil {
		h, err := obj.hashFile(*obj.Input)
		if err != nil {
			return false, err
		}
		i1 = h
	}
	i1 = obj.levelPrefix() + i1 // add the level prefix so it is considered

	i2, err := obj.readHashFile(obj.varDirPathInput)
	if err != nil {
		return false, err
	}

	// We're cheating by computing this before we know if we errored!
	inputMatches := i1 == i2
	outputMatches := h1 == h2
	if err == nil && inputMatches && outputMatches {
		// If the two hashes match, we assume that the file is correct!
		// The file has to also exist of course...
		return true, nil
	}

	if !apply {
		return false, nil
	}

	fail := true // assume we have a failure

	defer func() {
		if !fail {
			return
		}
		// Don't leave a partial file lying around...
		obj.init.Logf("removing partial gzip file")
		err := os.Remove(obj.getPath())
		if err == nil || os.IsNotExist(err) {
			return
		}
		obj.init.Logf("error removing corrupt gzip file: %v", err)
	}()

	// FIXME: Do we instead want to write to a tmp file and do a move once
	// we finish writing to be atomic here and avoid partial corrupt files?
	// FIXME: Add a param called Atomic to specify that behaviour. It's
	// instant so that might be preferred as it might generate fewer events,
	// but there's a chance it's copying from obj.init.VarDir() to a
	// different filesystem.
	outputFile, err := os.Create(obj.getPath()) // io.Writer
	if err != nil {
		return false, err
	}
	//defer outputFile.Sync() // not needed?
	defer outputFile.Close()

	hash := sha256.New()

	// Write to both to avoid needing to wait for fsync to calculate hash!
	multiWriter := io.MultiWriter(outputFile, hash)

	gzipWriter, err := gzip.NewWriterLevel(multiWriter, obj.Level) // (*gzip.Writer, error)
	if err != nil {
		return false, err
	}

	var input io.Reader
	if obj.Input != nil {
		f, err := os.Open(*obj.Input) // io.Reader
		if err != nil && !os.IsNotExist(err) {
			// This is likely a permissions error.
			return false, err

		} else if err != nil {
			return false, err // File doesn't exist!
		}
		defer f.Close()
		input = f

	} else {
		input = strings.NewReader(obj.Content)
	}

	// Copy the input file into the writer, which writes it out compressed.
	count, err := io.Copy(gzipWriter, input) // dst, src
	if err != nil {
		gzipWriter.Close() // Might as well always close!
		return false, err
	}

	// NOTE: Must run this before hashing so that it includes the footer!
	if err := gzipWriter.Close(); err != nil {
		return false, err
	}
	sha256sum := hex.EncodeToString(hash.Sum(nil))

	obj.init.Logf("wrote %d gzipped bytes", count)

	// After gzip is successfully written, store the hashed input result.
	if !inputMatches {
		if err := os.WriteFile(obj.varDirPathInput, []byte(i1+"\n"), 0600); err != nil {
			return false, err
		}
	}

	// Also store the new hashed output result.
	if !outputMatches || h2 == "" { // If missing, we always write it out!
		if err := os.WriteFile(obj.varDirPathOutput, []byte(sha256sum+"\n"), 0600); err != nil {
			return false, err
		}
	}

	fail = false // defer can exit safely!

	return false, nil
}

// levelPrefix is a simple helper to add a level identifier for our hash.
func (obj *GzipRes) levelPrefix() string {
	return fmt.Sprintf("level:%d|", obj.Level)
}

// hashContent is a simple helper to run our hashing function.
func (obj *GzipRes) hashContent(handle io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile is a helper that returns the hash of the specified file. If the file
// doesn't exist, it returns the empty string. Otherwise it errors.
func (obj *GzipRes) hashFile(file string) (string, error) {
	f, err := os.Open(file) // io.Reader
	if err != nil && !os.IsNotExist(err) {
		// This is likely a permissions error.
		return "", err

	} else if err != nil {
		return "", nil // File doesn't exist!
	}

	defer f.Close()

	// File exists, lets hash it!

	return obj.hashContent(f)
}

// readHashFile reads the hashed value that we stored for the output file.
func (obj *GzipRes) readHashFile(file string) (string, error) {
	// TODO: Use io.ReadFull to avoid reading in a file that's too big!
	if expected, err := os.ReadFile(file); err != nil && !os.IsNotExist(err) { // ([]byte, error)
		// This is likely a permissions error?
		return "", err

	} else if err == nil {
		return strings.TrimSpace(string(expected)), nil
	}

	// File doesn't exist!
	return "", nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *GzipRes) Cmp(r engine.Res) error {
	// we can only compare GzipRes to others of the same resource kind
	res, ok := r.(*GzipRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}

	if (obj.Input == nil) != (res.Input == nil) { // xor
		return fmt.Errorf("the Input differs")
	}
	if obj.Input != nil && res.Input != nil {
		if *obj.Input != *res.Input { // compare the strings
			return fmt.Errorf("the contents of Input differ")
		}
	}

	if obj.Content != res.Content {
		return fmt.Errorf("the Content differs")
	}

	if obj.Level != res.Level {
		return fmt.Errorf("the Level differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *GzipRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes GzipRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*GzipRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to GzipRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = GzipRes(raw) // restore from indirection with type conversion!
	return nil
}
