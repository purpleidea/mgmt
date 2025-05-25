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
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("tar", func() engine.Res { return &TarRes{} })

	// const.res.tar.format.unknown = 0
	// const.res.tar.format.ustar = 2
	// const.res.tar.format.pax = 4
	// const.res.tar.format.gnu = 8
	vars.RegisterResourceParams("tar", map[string]map[string]func() interfaces.Var{
		"format": {
			"unknown": func() interfaces.Var {
				return &types.IntValue{
					V: int64(tar.FormatUnknown),
				}
			},
			"ustar": func() interfaces.Var {
				return &types.IntValue{
					V: int64(tar.FormatUSTAR),
				}
			},
			"pax": func() interfaces.Var {
				return &types.IntValue{
					V: int64(tar.FormatPAX),
				}
			},
			"gnu": func() interfaces.Var {
				return &types.IntValue{
					V: int64(tar.FormatGNU),
				}
			},
		},
	})
}

// TarRes is a resource that archives a number of paths using tar, thus
// combining them into a single file. The name of the resource is the path to
// the resultant archive file. The input comes from a list of paths which can be
// either files or directories or both. Directories are added recursively of
// course. This uses hashes to determine if something was changed, so as a
// result, this may not be suitable if you can create a sha256 hash collision.
// TODO: support send/recv to send the output instead of writing to a file?
type TarRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Path, which defaults to the name if not specified, represents the
	// destination path for the compressed file being created. It must be an
	// absolute path, and as a result must start with a slash. Since it is a
	// file, it must not end with a slash.
	Path string `lang:"path" yaml:"path"`

	// Inputs represents the list of files to be compressed. They must each
	// be absolute paths of either single files or directories, and as a
	// result, each must start with a slash. Directories must end with a
	// slash and files must not for standard behaviour. As a special
	// exception, if you omit the trailing slash on a directory path, then
	// this will include that directory name as a prefix. This is similar to
	// how rsync chooses if it copies in the base directory or not.
	Inputs []string `lang:"inputs" yaml:"inputs"`

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
func (obj *TarRes) getPath() string {
	p := obj.Path
	if obj.Path == "" { // use the name as the path default if missing
		p = obj.Name()
	}
	return p
}

// Default returns some sensible defaults for this resource.
func (obj *TarRes) Default() engine.Res {
	return &TarRes{
		Format: int(tar.FormatUnknown), // TODO: will this let it auto-choose?
	}
}

// Validate if the params passed in are valid data.
func (obj *TarRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("path is empty")
	}
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("path must be absolute")
	}
	if strings.HasSuffix(obj.getPath(), "/") {
		return fmt.Errorf("path must not end with a slash")
	}

	for i, x := range obj.Inputs {
		if !strings.HasPrefix(x, "/") {
			return fmt.Errorf("input #%d must be absolute", i)
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *TarRes) Init(init *engine.Init) error {
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
func (obj *TarRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TarRes) Watch(ctx context.Context) error {
	recurse := false // single (output) file
	recWatcher, err := recwatch.NewRecWatcher(obj.getPath(), recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	chanList := []<-chan recwatch.Event{}
	for _, x := range obj.Inputs {
		fi, err := os.Stat(x)
		if err != nil {
			return err
		}
		//recurse := strings.HasSuffix(x, "/") // recurse for dirs
		recurse := fi.IsDir()
		recWatcher, err := recwatch.NewRecWatcher(x, recurse)
		if err != nil {
			return err
		}
		defer recWatcher.Close()
		ch := recWatcher.Events()
		chanList = append(chanList, ch)
	}
	events := recwatch.MergeChannels(chanList...)

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

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
// This is where we actually do the archiving into a tar file work when needed.
func (obj *TarRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	h1, err := obj.hashFile(obj.getPath()) // output
	if err != nil {
		return false, err
	}

	h2, err := obj.readHashFile(obj.varDirPathOutput, true)
	if err != nil {
		return false, err
	}

	isDirCache := make(map[string]bool)

	i1 := ""
	i1 = obj.formatPrefix() + "\n" // add the prefix so it is considered
	for _, x := range obj.Inputs {
		fi, err := os.Stat(x)
		if err != nil {
			return false, err
		}
		isDirCache[x] = fi.IsDir() // cache

		//if !strings.HasSuffix(x, "/") // not dir
		if !fi.IsDir() {
			h, err := obj.hashFile(x)
			if err != nil {
				return false, err
			}
			i1 += x + "|" + h + "\n"

			continue
		}

		// Must be a directory...

		fileSystem := os.DirFS(x)
		fn := func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == "." { // special case for root
					i1 += x + "|" + "\n"
					return nil
				}
				// hash the dir itself too (eg: empty dirs!)
				i1 += x + path + "/" + "|" + "\n"
				return nil
			}

			// file
			h, err := obj.hashFile(x + path)
			if err != nil {
				return err
			}
			i1 += x + path + "|" + h + "\n"

			return nil
		}
		if err := fs.WalkDir(fileSystem, ".", fn); err != nil {
			return false, err
		}
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

	for _, x := range obj.Inputs {
		isDir, exists := isDirCache[x]
		if !exists {
			// programming error
			return false, fmt.Errorf("is dir cache miss")
		}

		if isDir {
			// If strings.HasSuffix(x, "/") is true, it's the normal
			// way and prefix will be empty.
			ix := strings.LastIndex(x, "/")
			prefix := x[ix+1:]
			if prefix != "" {
				prefix += "/" // add the separator
			}
			fsys := os.DirFS(x) // fs.FS
			// TODO: formerly tarWriter.AddFS(fsys) // buggy!
			if err := obj.addFS(tarWriter, fsys, prefix); err != nil {
				return false, errwrap.Wrapf(err, "error writing: %s", x)
			}
			continue
		}

		// Must be a file...

		f, err := os.Open(x) // io.Reader
		if err != nil && !os.IsNotExist(err) {
			// This is likely a permissions error.
			return false, err

		} else if err != nil {
			return false, err // File doesn't exist!
		}
		defer f.Close() // Also close this below to free memory earlier.

		fileInfo, err := f.Stat()
		if err != nil {
			return false, err
		}

		// If fileInfo describes a symlink, tar.FileInfoHeader records
		// "link" as the link target.
		link := ""                                        // TODO: I have no idea if we want to use this.
		header, err := tar.FileInfoHeader(fileInfo, link) // (*tar.Header, error)
		if err != nil {
			return false, err
		}
		// Since fs.FileInfo's Name method only returns the base name of
		// the file it describes, it may be necessary to modify
		// Header.Name to provide the full path name of the file.
		// header.Name = name // TODO: edit this if needed
		header.Format = tar.Format(obj.Format)

		if err := tarWriter.WriteHeader(header); err != nil {
			return false, errwrap.Wrapf(err, "error writing: %s", header.Name)
		}

		// Copy the input file into the writer, which archives it out.
		count, err := io.Copy(tarWriter, f) // dst, src
		if err != nil {
			return false, err
		}
		_ = count
		// TODO: add better logging if we can see tarWriter.AddFs too!
		//obj.init.Logf("wrote %d archived bytes of: %s", count, header.Name)

		if err := f.Close(); err != nil { // free earlier than defer!
			return false, err
		}
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
func (obj *TarRes) formatPrefix() string {
	return fmt.Sprintf("format:%d|%s", obj.Format, tar.Format(obj.Format))
}

// hashContent is a simple helper to run our hashing function.
func (obj *TarRes) hashContent(handle io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile is a helper that returns the hash of the specified file. If the file
// doesn't exist, it returns the empty string. Otherwise it errors.
func (obj *TarRes) hashFile(file string) (string, error) {
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
func (obj *TarRes) readHashFile(file string, trim bool) (string, error) {
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
func (obj *TarRes) addFS(tw *tar.Writer, fsys fs.FS, prefix string) error {
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
			return fmt.Errorf("tar: cannot add non-regular file")
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = prefix + name
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

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TarRes) Cmp(r engine.Res) error {
	// we can only compare TarRes to others of the same resource kind
	res, ok := r.(*TarRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}

	if len(obj.Inputs) != len(res.Inputs) {
		return fmt.Errorf("the number of Inputs differs")
	}
	for i, x := range obj.Inputs {
		if input := res.Inputs[i]; x != input {
			return fmt.Errorf("the input at index %d differs", i)
		}
	}

	if obj.Format != res.Format {
		return fmt.Errorf("the Format differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *TarRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TarRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*TarRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TarRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TarRes(raw) // restore from indirection with type conversion!
	return nil
}
