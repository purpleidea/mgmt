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
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	"github.com/spf13/afero"
)

func init() {
	engine.RegisterResource("deploy:tar", func() engine.Res { return &DeployTar{} })
}

// DeployTar is a resource that archives a deploy filesystem using tar, thus
// combining them into a single file. The name of the resource is the path to
// the resultant archive file. The input comes from the current deploy. This
// uses hashes to determine if something was changed, so as a result, this may
// not be suitable if you can create a sha256 hash collision.
// TODO: support send/recv to send the output instead of writing to a file?
// TODO: This resource is very similar to the tar resource. Update that one if
// this changes, or consider porting this to use that as a composite resource.
// TODO: consider using a `deploy.get_archive()` function to make a .tar, and a
// file resource to store those contents on disk with whatever mode we want...
type DeployTar struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Path, which defaults to the name if not specified, represents the
	// destination path for the compressed file being created. It must be an
	// absolute path, and as a result must start with a slash. Since it is a
	// file, it must not end with a slash.
	Path string `lang:"path" yaml:"path"`

	// Format is the header format to use. If you change this, then the
	// file will get rearchived. The strange thing is that it seems the
	// header format is stored for each individual file. The available
	// values are: const.res.tar.format.unknown, const.res.tar.format.ustar,
	// const.res.tar.format.pax, and const.res.tar.format.gnu which have
	// values of 0, 2, 4, and 8 respectively.
	Format int `lang:"format" yaml:"format"`

	// SendOnly specifies that we don't write the file to disk, and as a
	// result, the output is only be accessible by the send/recv mechanism.
	// TODO: Rename this?
	// TODO: Not implemented
	//SendOnly bool `lang:"sendonly" yaml:"sendonly"`

	// varDirPathInput is the path we use to store the content hash.
	varDirPathInput string

	// varDirPathOutput is the path we use to store the output file hash.
	varDirPathOutput string
}

// getPath returns the actual path to use for this resource. It computes this
// after analysis of the Path and Name.
func (obj *DeployTar) getPath() string {
	p := obj.Path
	if obj.Path == "" { // use the name as the path default if missing
		p = obj.Name()
	}
	return p
}

// Default returns some sensible defaults for this resource.
func (obj *DeployTar) Default() engine.Res {
	return &DeployTar{
		Format: int(tar.FormatUnknown), // TODO: will this let it auto-choose?
	}
}

// Validate if the params passed in are valid data.
func (obj *DeployTar) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("path is empty")
	}
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("path must be absolute")
	}
	if strings.HasSuffix(obj.getPath(), "/") {
		return fmt.Errorf("path must not end with a slash")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *DeployTar) Init(init *engine.Init) error {
	obj.init = init // save for later

	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	// return unique files
	obj.varDirPathInput = path.Join(dir, "input.sha256")
	obj.varDirPathOutput = path.Join(dir, "output.sha256")

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DeployTar) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DeployTar) Watch(ctx context.Context) error {
	recurse := false // single (output) file
	recWatcher, err := recwatch.NewRecWatcher(obj.getPath(), recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	if err := obj.init.Running(ctx); err != nil { return err } // when started, notify engine that we're running

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

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if err := obj.init.Event(ctx); err != nil { return err } // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// This is where we actually do the archiving into a tar file work when needed.
func (obj *DeployTar) CheckApply(ctx context.Context, apply bool) (bool, error) {
	uri := obj.init.World.URI() // request each time to ensure it's fresh!

	filesystem, err := obj.init.World.Fs(uri) // open the remote file system
	if err != nil {
		return false, errwrap.Wrapf(err, "can't load code from file system `%s`", uri)
	}

	h1, err := obj.hashFile(obj.getPath()) // output
	if err != nil {
		return false, err
	}

	h2, err := obj.readHashFile(obj.varDirPathOutput, true)
	if err != nil {
		return false, err
	}

	i1 := ""
	i1 = obj.formatPrefix() + "\n" // add the prefix so it is considered

	// TODO: use standard filesystem API's when we can make them work!
	//fsys := afero.NewIOFS(filesystem)

	if err := afero.Walk(filesystem, "/", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path == "/" { // special case for root
				i1 += path + "|" + "\n"
				return nil
			}
			// hash the dir itself too (eg: empty dirs!)
			i1 += path + "/" + "|" + "\n"
			return nil
		}

		h, err := obj.hashFileAferoFs(filesystem, path)
		if err != nil {
			return err
		}
		i1 += path + "|" + h + "\n"
		return nil

	}); err != nil {
		return false, err
	}

	i2, err := obj.readHashFile(obj.varDirPathInput, false)
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
		obj.init.Logf("removing partial tar file")
		err := os.Remove(obj.getPath())
		if err == nil || os.IsNotExist(err) {
			return
		}
		obj.init.Logf("error removing corrupt tar file: %v", err)
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

	tarWriter := tar.NewWriter(multiWriter) // (*tar.Writer, error)
	defer tarWriter.Close()                 // Might as well always close if we error early!

	// TODO: formerly tarWriter.AddFS(fsys) // buggy!
	if err := obj.addAferoFs(tarWriter, filesystem); err != nil {
		return false, errwrap.Wrapf(err, "error writing fs")
	}

	// NOTE: Must run this before hashing so that it includes the footer!
	if err := tarWriter.Close(); err != nil {
		return false, err
	}
	sha256sum := hex.EncodeToString(hash.Sum(nil))

	// TODO: add better logging counts if we can see tarWriter.AddFs too!
	//obj.init.Logf("wrote %d files into archive", ?)
	obj.init.Logf("wrote tar archive")

	// After tar is successfully written, store the hashed input result.
	if !inputMatches {
		if err := os.WriteFile(obj.varDirPathInput, []byte(i1), 0600); err != nil {
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

// formatPrefix is a simple helper to add a format identifier for our hash.
func (obj *DeployTar) formatPrefix() string {
	return fmt.Sprintf("format:%d|%s", obj.Format, tar.Format(obj.Format))
}

// hashContent is a simple helper to run our hashing function.
func (obj *DeployTar) hashContent(handle io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile is a helper that returns the hash of the specified file. If the file
// doesn't exist, it returns the empty string. Otherwise it errors.
func (obj *DeployTar) hashFile(file string) (string, error) {
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

// hashFileAferoFs is a helper that returns the hash of the specified file with
// an Afero fs. If the file doesn't exist, it returns the empty string.
// Otherwise it errors.
func (obj *DeployTar) hashFileAferoFs(fsys afero.Fs, file string) (string, error) {
	f, err := fsys.Open(file) // io.Reader
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
func (obj *DeployTar) readHashFile(file string, trim bool) (string, error) {
	// TODO: Use io.ReadFull to avoid reading in a file that's too big!
	if expected, err := os.ReadFile(file); err != nil && !os.IsNotExist(err) { // ([]byte, error)
		// This is likely a permissions error?
		return "", err

	} else if err == nil {
		if trim {
			return strings.TrimSpace(string(expected)), nil
		}
		return string(expected), nil
	}

	// File doesn't exist!
	return "", nil
}

// addFS is an edited copy of archive/tar's *Writer.AddFs function. This version
// correctly adds the directories too! https://github.com/golang/go/issues/69459
func (obj *DeployTar) addFS(tw *tar.Writer, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// TODO: Handle symlinks when fs.ReadLinkFS is available. (#49580)
		if !info.Mode().IsRegular() && !info.Mode().IsDir() {
			return fmt.Errorf("deploy:tar: cannot add non-regular file")
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = name
		h.Format = tar.Format(obj.Format)
		if d.IsDir() {
			h.Name += "/" // dir
		}

		if err := tw.WriteHeader(h); err != nil {
			return err
		}

		if d.IsDir() {
			return nil // no contents to copy in
		}

		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// addAferoFs is an edited copy of archive/tar's *Writer.AddFs function but for
// the deprecated Afero.Fs API. This version correctly adds the directories too!
// https://github.com/golang/go/issues/69459
func (obj *DeployTar) addAferoFs(tw *tar.Writer, fsys afero.Fs) error {
	return afero.Walk(fsys, "/", func(name string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if name == "/" {
			return nil
		}
		// TODO: Handle symlinks when fs.ReadLinkFS is available. (#49580)
		if !info.Mode().IsRegular() && !info.Mode().IsDir() {
			return fmt.Errorf("deploy:tar: cannot add non-regular file")
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = name
		h.Format = tar.Format(obj.Format)
		if info.IsDir() {
			h.Name += "/" // dir
		}

		if err := tw.WriteHeader(h); err != nil {
			return err
		}

		if info.IsDir() {
			return nil // no contents to copy in
		}

		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DeployTar) Cmp(r engine.Res) error {
	// we can only compare DeployTar to others of the same resource kind
	res, ok := r.(*DeployTar)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}

	if obj.Format != res.Format {
		return fmt.Errorf("the Format differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DeployTar) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DeployTar // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*DeployTar) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DeployTar")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DeployTar(raw) // restore from indirection with type conversion!
	return nil
}
