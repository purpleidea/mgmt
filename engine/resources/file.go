// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource(KindFile, func() engine.Res { return &FileRes{} })

	// const.res.file.state.exists = "exists"
	// const.res.file.state.absent = "absent"
	vars.RegisterResourceParams(KindFile, map[string]map[string]func() interfaces.Var{
		ParamFileState: {
			FileStateExists: func() interfaces.Var {
				return &types.StrValue{
					V: FileStateExists,
				}
			},
			FileStateAbsent: func() interfaces.Var {
				return &types.StrValue{
					V: FileStateAbsent,
				}
			},
			// TODO: consider removing this field entirely
			"undefined": func() interfaces.Var {
				return &types.StrValue{
					V: FileStateUndefined, // empty string
				}
			},
		},
	})
}

const (
	// KindFile is the kind string used to identify this resource.
	KindFile = "file"
	// ParamFileState is the name of the state field parameter.
	ParamFileState = "state"
	// FileStateExists is the string that represents that the file should be
	// present.
	FileStateExists = "exists"
	// FileStateAbsent is the string that represents that the file should
	// not exist.
	FileStateAbsent = "absent"
	// FileStateUndefined means the file state has not been specified.
	// TODO: consider moving to *string and express this state as a nil.
	FileStateUndefined = ""

	// FileModeAllowAssign specifies whether we only use ugo=rwx style
	// assignment (false) or if we also allow ugo+-rwx style too (true). I
	// think that it's possibly illogical to allow imperative mode
	// specifiers in a declarative language, so let's leave it off for now.
	FileModeAllowAssign = false
)

// FileRes is a file and directory resource. Dirs are defined by names ending in
// a slash.
type FileRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.GraphQueryable // allow others to query this res in the res graph
	//traits.Groupable // TODO: implement this
	traits.Recvable
	traits.Reversible

	init *engine.Init

	// Path, which defaults to the name if not specified, represents the
	// destination path for the file or directory being managed. It must be
	// an absolute path, and as a result must start with a slash.
	Path     string `lang:"path" yaml:"path"`
	Dirname  string `lang:"dirname" yaml:"dirname"`   // override the path dirname
	Basename string `lang:"basename" yaml:"basename"` // override the path basename

	// State specifies the desired state of the file. It can be either
	// `exists` or `absent`. If you do not specify this, we will not be able
	// to create or remove a file if it might be logical for another
	// param to require that. Instead it will error. This means that this
	// field is not implied by specifying some content or a mode.
	State string `lang:"state" yaml:"state"`

	// Content specifies the file contents to use. If this is nil, they are
	// left undefined. It cannot be combined with the Source or Fragments
	// parameters.
	Content *string `lang:"content" yaml:"content"`
	// Source specifies the source contents for the file resource. It cannot
	// be combined with the Content or Fragments parameters. It must be an
	// absolute path, and it can point to a file or a directory. If it
	// points to a file, then that will will be copied throuh directly. If
	// it points to a directory, then it will copy the directory "rsync
	// style" onto the file destination. As a result, if this is a file,
	// then the main file res must be a file, and if it is a directory, then
	// this must be a directory. To meaningfully copy a full directory, you
	// also need to specify the Recurse parameter, which is currently
	// required. If you want an existing dir to be turned into a file (or
	// vice-versa) instead of erroring, then you'll also need to specify the
	// Force parameter. If source is undefined and the file path is a
	// directory, then a directory will be created. If left undefined, and
	// combined with the Purge option too, then any unmanaged file in this
	// dir will be removed.
	Source string `lang:"source" yaml:"source"`
	// Fragments specifies that the file is built from a list of individual
	// files. If one of the files is a directory, then the list of files in
	// that directory are the fragments to combine. Multiple of these can be
	// used together, although most simple cases will probably only either
	// involve a single directory path or a fixed list of individual files.
	// All paths are absolute and as a result must start with a slash. The
	// directories (if any) must end with a slash as well. This cannot be
	// combined with the Content or Source parameters. If a file with param
	// is reversed, the reversed file is one that has `Content` set instead.
	// Automatic edges will be added from these fragments. This currently
	// isn't recursive in that if a fragment is a directory, this only
	// searches one level deep at the moment.
	Fragments []string `lang:"fragments" yaml:"fragments"`

	// Owner specifies the file owner. You can specify either the string
	// name, or a string representation of the owner integer uid.
	Owner string `lang:"owner" yaml:"owner"`
	// Group specifies the file group. You can specify either the string
	// name, or a string representation of the group integer gid.
	Group string `lang:"group" yaml:"group"`
	// Mode is the mode of the file as a string representation of the octal
	// form or symbolic form.
	Mode    string `lang:"mode" yaml:"mode"`
	Recurse bool   `lang:"recurse" yaml:"recurse"`
	Force   bool   `lang:"force" yaml:"force"`
	// Purge specifies that when true, any unmanaged file in this file
	// directory will be removed. As a result, this file resource must be a
	// directory. This isn't particularly meaningful if you don't also set
	// Recurse to true. This doesn't work with Content or Fragments.
	Purge bool `lang:"purge" yaml:"purge"`

	sha256sum string
}

// getPath returns the actual path to use for this resource. It computes this
// after analysis of the Path, Dirname and Basename values. Dirs end with slash.
// TODO: memoize the result if this seems important.
func (obj *FileRes) getPath() string {
	p := obj.Path
	if obj.Path == "" { // use the name as the path default if missing
		p = obj.Name()
	}

	d := util.Dirname(p)
	b := util.Basename(p)
	if obj.Dirname == "" && obj.Basename == "" {
		return p
	}
	if obj.Dirname == "" {
		return d + obj.Basename
	}
	if obj.Basename == "" {
		return obj.Dirname + b
	}
	// if obj.dirname != "" && obj.basename != ""
	return obj.Dirname + obj.Basename
}

// isDir is a helper function to specify whether the path should be a dir.
func (obj *FileRes) isDir() bool {
	return strings.HasSuffix(obj.getPath(), "/") // dirs have trailing slashes
}

// mode returns the file permission specified on the graph. It doesn't handle
// the case where the mode is not specified. The caller should check obj.Mode is
// not empty.
func (obj *FileRes) mode() (os.FileMode, error) {
	if n, err := strconv.ParseInt(obj.Mode, 8, 32); err == nil {
		return os.FileMode(n), nil
	}

	// Try parsing symbolically by first getting the files current mode.
	stat, err := os.Stat(obj.getPath())
	if err != nil {
		return os.FileMode(0), errwrap.Wrapf(err, "failed to get the current file mode")
	}

	modes := strings.Split(obj.Mode, ",")
	m, err := engineUtil.ParseSymbolicModes(modes, stat.Mode(), FileModeAllowAssign)
	if err != nil {
		return os.FileMode(0), errwrap.Wrapf(err, "mode should be an octal number or symbolic mode (%s)", obj.Mode)
	}

	return os.FileMode(m), nil
}

// Default returns some sensible defaults for this resource.
func (obj *FileRes) Default() engine.Res {
	return &FileRes{
		//State: FileStateUndefined, // the default must be undefined!
	}
}

// Validate reports any problems with the struct definition.
func (obj *FileRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("path is empty")
	}

	if obj.Dirname != "" && !strings.HasSuffix(obj.Dirname, "/") {
		return fmt.Errorf("dirname must end with a slash")
	}

	if strings.HasPrefix(obj.Basename, "/") {
		return fmt.Errorf("basename must not start with a slash")
	}

	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("resultant path must be absolute")
	}

	if obj.State != FileStateExists && obj.State != FileStateAbsent && obj.State != FileStateUndefined {
		return fmt.Errorf("the State is invalid")
	}

	isContent := obj.Content != nil
	isSrc := obj.Source != ""
	isFrag := len(obj.Fragments) > 0
	if (isContent && isSrc) || (isSrc && isFrag) || (isFrag && isContent) {
		return fmt.Errorf("can only specify one of Content, Source, and Fragments")
	}

	if obj.State == FileStateAbsent && (isContent || isSrc || isFrag) {
		return fmt.Errorf("can't specify file Content, Source, or Fragments when State is %s", FileStateAbsent)
	}

	// The path and Source must either both be dirs or both not be.
	srcIsDir := strings.HasSuffix(obj.Source, "/")
	if isSrc && (obj.isDir() != srcIsDir) {
		return fmt.Errorf("the path and Source must either both be dirs or both not be")
	}

	if obj.isDir() && (isContent || isFrag) { // makes no sense
		return fmt.Errorf("can't specify Content or Fragments when creating a Dir")
	}

	// TODO: is this really a requirement that we want to enforce?
	if isSrc && obj.isDir() && srcIsDir && !obj.Recurse {
		return fmt.Errorf("you'll want to Recurse when you have a Source dir to copy")
	}
	// TODO: do we want to enforce this sort of thing?
	if obj.Purge && !obj.Recurse {
		return fmt.Errorf("you'll want to Recurse when you have a Purge to do")
	}

	if isSrc && !obj.isDir() && !srcIsDir && obj.Recurse {
		return fmt.Errorf("you can't recurse when copying a single file")
	}

	for _, frag := range obj.Fragments {
		// absolute paths begin with a slash
		if !strings.HasPrefix(frag, "/") {
			return fmt.Errorf("the frag (`%s`) isn't an absolute path", frag)
		}
	}

	if obj.Purge && (isContent || isFrag) {
		return fmt.Errorf("can't combine Purge with Content or Fragments")
	}
	// XXX: should this work with obj.Purge && obj.Source != "" or not?
	//if obj.Purge && obj.Source != "" {
	//	return fmt.Errorf("can't Purge when Source is specified")
	//}

	// TODO: should we silently ignore these errors or include them?
	//if obj.State == FileStateAbsent && obj.Owner != "" {
	//	return fmt.Errorf("can't specify Owner for an absent file")
	//}
	//if obj.State == FileStateAbsent && obj.Group != "" {
	//	return fmt.Errorf("can't specify Group for an absent file")
	//}
	if obj.Owner != "" || obj.Group != "" {
		fileInfo, err := os.Stat("/") // pick root just to do this test
		if err != nil {
			return fmt.Errorf("can't stat root to get system information")
		}
		_, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("can't set Owner or Group on this platform")
		}
	}
	if _, err := engineUtil.GetUID(obj.Owner); obj.Owner != "" && err != nil {
		return err
	}

	if _, err := engineUtil.GetGID(obj.Group); obj.Group != "" && err != nil {
		return err
	}

	// TODO: should we silently ignore this error or include it?
	//if obj.State == FileStateAbsent && obj.Mode != "" {
	//	return fmt.Errorf("can't specify Mode for an absent file")
	//}
	if obj.Mode != "" {
		if _, err := obj.mode(); err != nil {
			return err
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *FileRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.sha256sum = ""

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *FileRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// one is a file watcher for files and directories. Modify with caution, it is
// probably important to write some test cases first! If the Watch returns an
// error, it means that something has gone wrong, and it must be restarted. On a
// clean exit it returns nil.
func (obj *FileRes) Watch() error {
	// TODO: chan *recwatch.Event instead?
	inputEvents := make(chan recwatch.Event)
	defer close(inputEvents)

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	exit := make(chan struct{})
	// TODO: should this be after (later in the file) than the `defer recWatcher.Close()` ?
	// TODO: should this be after (later in the file) the `defer recWatcher.Close()` ?
	defer close(exit)

	recWatcher, err := recwatch.NewRecWatcher(obj.getPath(), obj.Recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	// watch the various inputs to this file resource too!
	if obj.Source != "" {
		// This block is virtually identical to the below one.
		recurse := strings.HasSuffix(obj.Source, "/") // isDir
		rw, err := recwatch.NewRecWatcher(obj.Source, recurse)
		if err != nil {
			return err
		}
		defer rw.Close()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// TODO: *recwatch.Event instead?
				var event recwatch.Event
				var ok bool
				var shutdown bool
				select {
				case event, ok = <-rw.Events(): // recv
				case <-exit: // unblock
					return
				}

				if !ok {
					err := fmt.Errorf("channel shutdown")
					event = recwatch.Event{Error: err}
					shutdown = true
				}

				select {
				case inputEvents <- event: // send
					if shutdown { // optimization to free early
						return
					}
				case <-exit: // unblock
					return
				}
			}
		}()
	}
	for _, frag := range obj.Fragments {
		// This block is virtually identical to the above one.
		recurse := false // TODO: is it okay for depth==1 dirs?
		//recurse := strings.HasSuffix(frag, "/") // isDir
		rw, err := recwatch.NewRecWatcher(frag, recurse)
		if err != nil {
			return err
		}
		defer rw.Close()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// TODO: *recwatch.Event instead?
				var event recwatch.Event
				var ok bool
				var shutdown bool
				select {
				case event, ok = <-rw.Events(): // recv
				case <-exit: // unblock
					return
				}

				if !ok {
					err := fmt.Errorf("channel shutdown")
					event = recwatch.Event{Error: err}
					shutdown = true
				}

				select {
				case inputEvents <- event: // send
					if shutdown { // optimization to free early
						return
					}
				case <-exit: // unblock
					return
				}
			}
		}()
	}

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("watching: %s", obj.getPath()) // attempting to watch...
		}

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
			send = true

		case event, ok := <-inputEvents:
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s input watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("input event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// fileCheckApply is the CheckApply operation for a source and destination file.
// It can accept an io.Reader as the source, which can be a regular file, or it
// can be a bytes Buffer struct. It can take an input sha256 hash to use instead
// of computing the source data hash, and it returns the computed value if this
// function reaches that stage. As usual, it respects the apply action variable,
// and has some symmetry with the main CheckApply function.
func (obj *FileRes) fileCheckApply(apply bool, src io.ReadSeeker, dst string, sha256sum string) (string, bool, error) {
	// TODO: does it make sense to switch dst to an io.Writer ?
	// TODO: use obj.Force when dealing with symlinks and other file types!
	if obj.init.Debug {
		obj.init.Logf("fileCheckApply: %v -> %s", src, dst)
	}

	srcFile, isFile := src.(*os.File)
	_, isBytes := src.(*bytes.Reader) // supports seeking!
	if !isFile && !isBytes {
		return "", false, fmt.Errorf("can't open src as either file or buffer")
	}

	var srcStat os.FileInfo
	if isFile {
		var err error
		srcStat, err = srcFile.Stat()
		if err != nil {
			return "", false, err
		}
		// TODO: deal with symlinks
		if !srcStat.Mode().IsRegular() { // can't copy non-regular files or dirs
			return "", false, fmt.Errorf("non-regular src file: %s (%q)", srcStat.Name(), srcStat.Mode())
		}
	}

	dstFile, err := os.Open(dst)
	if err != nil && !os.IsNotExist(err) { // ignore ErrNotExist errors
		return "", false, err
	}
	dstClose := func() error {
		return dstFile.Close() // calling this twice is safe :)
	}
	defer dstClose()
	dstExists := !os.IsNotExist(err)

	// Optimization: we shouldn't be making the file, it happens in
	// stateCheckApply, but we skip doing it there in order to do it here,
	// unless we're undefined, and then we shouldn't force it!
	if !dstExists && obj.State == FileStateUndefined {
		return "", false, err
	}

	dstStat, err := dstFile.Stat()
	if err != nil && dstExists {
		return "", false, err
	}

	if dstExists && dstStat.IsDir() { // oops, dst is a dir, and we want a file...
		if !obj.Force {
			return "", false, fmt.Errorf("can't force dir into file: %s", dst)
		}
		if !apply {
			return "", false, nil
		}

		cleanDst := path.Clean(dst)
		if cleanDst == "" || cleanDst == "/" {
			return "", false, fmt.Errorf("don't want to remove root") // safety
		}
		// FIXME: respect obj.Recurse here...
		// there is a dir here, where we want a file...
		obj.init.Logf("fileCheckApply: removing (force): %s", cleanDst)
		if err := os.RemoveAll(cleanDst); err != nil { // dangerous ;)
			return "", false, err
		}
		dstExists = false // now it's gone!

	} else if err == nil {
		if !dstStat.Mode().IsRegular() {
			return "", false, fmt.Errorf("non-regular dst file: %s (%q)", dstStat.Name(), dstStat.Mode())
		}
		if isFile && os.SameFile(srcStat, dstStat) { // same inode, we're done!
			return "", true, nil
		}
	}

	if dstExists { // if dst doesn't exist, no need to compare hashes
		// hash comparison (efficient because we can cache hash of content str)
		if sha256sum == "" { // cache is invalid
			hash := sha256.New()
			// TODO: file existence test?
			if _, err := io.Copy(hash, src); err != nil {
				return "", false, err
			}
			sha256sum = hex.EncodeToString(hash.Sum(nil))
			// since we re-use this src handler below, it is
			// *critical* to seek to 0, or we'll copy nothing!
			if n, err := src.Seek(0, 0); err != nil || n != 0 {
				return sha256sum, false, err
			}
		}

		// dst hash
		hash := sha256.New()
		if _, err := io.Copy(hash, dstFile); err != nil {
			return "", false, err
		}
		if h := hex.EncodeToString(hash.Sum(nil)); h == sha256sum {
			return sha256sum, true, nil // same!
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return sha256sum, false, nil
	}
	if obj.init.Debug {
		obj.init.Logf("fileCheckApply: apply: %v -> %s", src, dst)
	}

	dstClose() // unlock file usage so we can write to it
	dstFile, err = os.Create(dst)
	if err != nil {
		return sha256sum, false, err
	}
	defer dstFile.Close() // TODO: is this redundant because of the earlier defered Close() ?

	if isFile { // set mode because it's a new file
		if err := dstFile.Chmod(srcStat.Mode()); err != nil {
			return sha256sum, false, err
		}
	}

	// TODO: attempt to reflink with Splice() and int(file.Fd()) as input...
	// syscall.Splice(rfd int, roff *int64, wfd int, woff *int64, len int, flags int) (n int64, err error)

	// TODO: should we offer a way to cancel the copy on ^C ?
	if obj.init.Debug {
		obj.init.Logf("fileCheckApply: copy: %v -> %s", src, dst)
	}
	if n, err := io.Copy(dstFile, src); err != nil {
		return sha256sum, false, err
	} else if obj.init.Debug {
		obj.init.Logf("fileCheckApply: copied: %v", n)
	}
	return sha256sum, false, dstFile.Sync()
}

// dirCheckApply is the CheckApply operation for an empty directory.
func (obj *FileRes) dirCheckApply(apply bool) (bool, error) {
	// check if the path exists and is a directory
	fileInfo, err := os.Stat(obj.getPath())
	if err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "stat error on file resource")
	}

	if err == nil && fileInfo.IsDir() {
		return true, nil // already a directory, nothing to do
	}
	if err == nil && !fileInfo.IsDir() && !obj.Force {
		return false, fmt.Errorf("can't force file into dir: %s", obj.getPath())
	}

	if !apply {
		return false, nil
	}

	// the path exists and is not a directory
	// delete the file if force is given
	if err == nil && !fileInfo.IsDir() {
		obj.init.Logf("dirCheckApply: removing (force): %s", obj.getPath())
		if err := os.Remove(obj.getPath()); err != nil {
			return false, err
		}
	}

	// create the empty directory
	var mode os.FileMode
	if obj.Mode != "" {
		mode, err = obj.mode()
		if err != nil {
			return false, err
		}
	} else {
		mode = os.ModePerm
	}

	if obj.Force {
		// FIXME: respect obj.Recurse here...
		// TODO: add recurse limit here
		return false, os.MkdirAll(obj.getPath(), mode)
	}

	return false, os.Mkdir(obj.getPath(), mode)
}

// syncCheckApply is the CheckApply operation for a source and destination dir.
// It is recursive and can create directories directly, and files via the usual
// fileCheckApply method. It returns checkOK and error as is normally expected.
// If excludes is specified, none of those files there will be deleted by this,
// with the exception that a sync *can* convert a file to a dir, or vice-versa.
func (obj *FileRes) syncCheckApply(apply bool, src, dst string, excludes []string) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("syncCheckApply: %s -> %s", src, dst)
	}
	// an src of "" is now supported, if dst is a dir
	if dst == "" {
		return false, fmt.Errorf("the src and dst must not be empty")
	}

	checkOK := true
	// TODO: handle ./ cases or ../ cases that need cleaning ?

	srcIsDir := strings.HasSuffix(src, "/")
	dstIsDir := strings.HasSuffix(dst, "/")

	if srcIsDir != dstIsDir && src != "" {
		return false, fmt.Errorf("the src and dst must be both either files or directories")
	}
	if src == "" && !dstIsDir {
		return false, fmt.Errorf("dst must be a dir if we have an empty src")
	}

	if !srcIsDir && !dstIsDir && src != "" {
		if obj.init.Debug {
			obj.init.Logf("syncCheckApply: %s -> %s", src, dst)
		}
		fin, err := os.Open(src)
		if err != nil {
			if obj.init.Debug && os.IsNotExist(err) { // if we get passed an empty src
				obj.init.Logf("syncCheckApply: missing src: %s", src)
			}
			return false, err
		}

		_, checkOK, err := obj.fileCheckApply(apply, fin, dst, "")
		if err != nil {
			fin.Close()
			return false, err
		}
		return checkOK, fin.Close()
	}

	// else: if srcIsDir && dstIsDir

	smartSrc := make(map[string]FileInfo)
	if src != "" {
		srcFiles, err := ReadDir(src)          // if src does not exist...
		if err != nil && !os.IsNotExist(err) { // an empty map comes out below!
			return false, err
		}
		smartSrc = mapPaths(srcFiles)
		obj.init.Logf("syncCheckApply: srcFiles: %v", srcFiles)
	}

	dstFiles, err := ReadDir(dst)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	smartDst := mapPaths(dstFiles)
	obj.init.Logf("syncCheckApply: dstFiles: %v", dstFiles)

	for relPath, fileInfo := range smartSrc {
		absSrc := fileInfo.AbsPath // absolute path
		absDst := dst + relPath    // absolute dest

		if _, exists := smartDst[relPath]; !exists {
			if fileInfo.IsDir() {
				if !apply { // only checking and not identical!
					return false, nil
				}

				// file exists, but we want a dir: we need force
				// we check for the file w/o the smart dir slash
				relPathFile := strings.TrimSuffix(relPath, "/")
				if _, ok := smartDst[relPathFile]; ok {
					absCleanDst := path.Clean(absDst)
					// TODO: can we fail this before `!apply`?
					if !obj.Force {
						return false, fmt.Errorf("can't force file into dir: %s", absCleanDst)
					}
					if absCleanDst == "" || absCleanDst == "/" {
						return false, fmt.Errorf("don't want to remove root") // safety
					}
					obj.init.Logf("syncCheckApply: removing (force): %s", absCleanDst)
					if err := os.Remove(absCleanDst); err != nil {
						return false, err
					}
					delete(smartDst, relPathFile) // rm from purge list
				}

				if obj.init.Debug {
					obj.init.Logf("syncCheckApply: mkdir -m %s '%s'", fileInfo.Mode(), absDst)
				}
				if err := os.Mkdir(absDst, fileInfo.Mode()); err != nil {
					return false, err
				}
				checkOK = false // we did some work
			}
			// if we're a regular file, the recurse will create it
		}

		if obj.init.Debug {
			obj.init.Logf("syncCheckApply: recurse: %s -> %s", absSrc, absDst)
		}
		if obj.Recurse {
			if c, err := obj.syncCheckApply(apply, absSrc, absDst, excludes); err != nil { // recurse
				return false, errwrap.Wrapf(err, "syncCheckApply: recurse failed")
			} else if !c { // don't let subsequent passes make this true
				checkOK = false
			}
		}
		if !apply && !checkOK { // check failed, and no apply to do, so exit!
			return false, nil
		}
		delete(smartDst, relPath) // rm from purge list
	}

	if !apply && len(smartDst) > 0 { // we know there are files to remove!
		return false, nil // so just exit now
	}

	// isExcluded specifies if the path is part of an excluded path. For
	// example, if we exclude /tmp/foo/bar from deletion, then we don't want
	// to delete /tmp/foo/bar *or* /tmp/foo/ *or* /tmp/ b/c they're parents.
	isExcluded := func(p string) bool {
		for _, x := range excludes {
			if util.HasPathPrefix(x, p) {
				return true
			}
		}
		return false
	}

	// any files that now remain in smartDst need to be removed...
	for relPath, fileInfo := range smartDst {
		absSrc := src + relPath    // absolute dest (should not exist!)
		absDst := fileInfo.AbsPath // absolute path (should get removed)
		absCleanDst := path.Clean(absDst)
		if absCleanDst == "" || absCleanDst == "/" {
			return false, fmt.Errorf("don't want to remove root") // safety
		}

		// FIXME: respect obj.Recurse here...

		// NOTE: we could use os.RemoveAll instead of recursing, but I
		// think the symmetry is more elegant and correct here for now
		// Avoiding this is also useful if we had a recurse limit arg!
		if true { // switch
			if isExcluded(absDst) { // skip removing excluded files
				continue
			}
			obj.init.Logf("syncCheckApply: removing: %s", absCleanDst)
			if apply {
				if err := os.RemoveAll(absCleanDst); err != nil { // dangerous ;)
					return false, err
				}
				checkOK = false
			}
			continue
		}
		_ = absSrc
		//obj.init.Logf("syncCheckApply: recurse rm: %s -> %s", absSrc, absDst)
		//if c, err := obj.syncCheckApply(apply, absSrc, absDst, excludes); err != nil {
		//	return false, errwrap.Wrapf(err, "syncCheckApply: recurse rm failed")
		//} else if !c { // don't let subsequent passes make this true
		//	checkOK = false
		//}
		//if isExcluded(absDst) { // skip removing excluded files
		//	continue
		//}
		//obj.init.Logf("syncCheckApply: removing: %s", absCleanDst)
		//if apply { // safety
		//	if err := os.Remove(absCleanDst); err != nil {
		//		return false, err
		//	}
		//	checkOK = false
		//}
	}

	return checkOK, nil
}

// stateCheckApply performs a CheckApply of the file state to create or remove
// an empty file or directory.
func (obj *FileRes) stateCheckApply(apply bool) (bool, error) {
	if obj.State == FileStateUndefined { // state is not specified
		return true, nil
	}

	_, err := os.Stat(obj.getPath())

	if err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "could not stat file")
	}

	if obj.State == FileStateAbsent && os.IsNotExist(err) {
		return true, nil
	}

	if obj.State == FileStateExists && err == nil {
		return true, nil
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	if obj.State == FileStateAbsent { // remove
		p := obj.getPath()
		if p == "" {
			// programming error?
			return false, fmt.Errorf("can't remove empty path") // safety
		}
		if p == "/" {
			return false, fmt.Errorf("don't want to remove root") // safety
		}
		obj.init.Logf("stateCheckApply: removing: %s", p)
		// FIXME: respect obj.Recurse here...
		// TODO: add recurse limit here
		err := os.RemoveAll(p) // dangerous ;)
		return false, err      // either nil or not
	}

	// we need to make a file or a directory now

	if obj.isDir() {
		return obj.dirCheckApply(apply)
	}

	// Optimization: we shouldn't even look at obj.Content here, but we can
	// skip this empty file creation here since we know we're going to be
	// making it there anyways. This way we save the extra fopen noise.
	if obj.Content != nil || len(obj.Fragments) > 0 {
		return false, nil // pretend we actually made it
	}

	// Create an empty file to ensure one exists. Don't O_TRUNC it, in case
	// one is magically created right after our exists test. The chmod used
	// is what is used by the os.Create function.
	// TODO: is using O_EXCL okay?
	f, err := os.OpenFile(obj.getPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return false, errwrap.Wrapf(err, "problem creating empty file")
	}
	if err := f.Close(); err != nil {
		return false, errwrap.Wrapf(err, "problem closing empty file")
	}

	return false, nil // defer the Content != nil work to later...
}

// contentCheckApply performs a CheckApply for the file content.
func (obj *FileRes) contentCheckApply(apply bool) (bool, error) {
	obj.init.Logf("contentCheckApply(%t)", apply)

	// content is not defined, leave it alone...
	if obj.Content == nil {
		return true, nil
	}

	// Actually write the file. This is similar to fragmentsCheckApply.
	bufferSrc := bytes.NewReader([]byte(*obj.Content))
	sha256sum, checkOK, err := obj.fileCheckApply(apply, bufferSrc, obj.getPath(), obj.sha256sum)
	if sha256sum != "" { // empty values mean errored or didn't hash
		// this can be valid even when the whole function errors
		obj.sha256sum = sha256sum // cache value
	}
	if err != nil {
		return false, err
	}
	// if no err, but !ok, then...
	return checkOK, nil // success
}

// sourceCheckApply performs a CheckApply for the file source.
func (obj *FileRes) sourceCheckApply(apply bool) (bool, error) {
	obj.init.Logf("sourceCheckApply(%t)", apply)

	// source is not defined, leave it alone...
	if obj.Source == "" && !obj.Purge {
		return true, nil
	}

	excludes := []string{}

	// If we're running a purge, do it here.
	if obj.Purge {
		graph, err := obj.init.FilteredGraph()
		if err != nil {
			return false, errwrap.Wrapf(err, "can't read filtered graph")
		}
		for _, vertex := range graph.Vertices() {
			res, ok := vertex.(engine.Res)
			if !ok {
				// programming error
				return false, fmt.Errorf("not a Res")
			}
			if res.Kind() != KindFile {
				continue // only interested in files
			}
			if res.Name() == obj.Name() {
				continue // skip me!
			}
			fileRes, ok := res.(*FileRes)
			if !ok {
				// programming error
				return false, fmt.Errorf("not a FileRes")
			}
			p := fileRes.getPath() // if others use it, make public!
			if !util.HasPathPrefix(p, obj.getPath()) {
				continue
			}
			excludes = append(excludes, p)
		}
	}
	if obj.init.Debug {
		obj.init.Logf("syncCheckApply: excludes: %+v", excludes)
	}

	// XXX: should this work with obj.Purge && obj.Source != "" or not?
	checkOK, err := obj.syncCheckApply(apply, obj.Source, obj.getPath(), excludes)
	if err != nil {
		obj.init.Logf("syncCheckApply: error: %v", err)
		return false, err
	}

	return checkOK, nil
}

// fragmentsCheckApply performs a CheckApply for the file fragments.
func (obj *FileRes) fragmentsCheckApply(apply bool) (bool, error) {
	obj.init.Logf("fragmentsCheckApply(%t)", apply)

	// fragments is not defined, leave it alone...
	if len(obj.Fragments) == 0 {
		return true, nil
	}

	content := ""
	// TODO: In the future we could have a flag that merges and then sorts
	// all the individual files in each directory before they are combined.
	for _, frag := range obj.Fragments {
		// It's a single file. Add it to what we're building...
		if isDir := strings.HasSuffix(frag, "/"); !isDir {
			out, err := ioutil.ReadFile(frag)
			if err != nil {
				return false, errwrap.Wrapf(err, "could not read file fragment")
			}
			content += string(out)
			continue
		}

		// We're a dir, peer inside...
		files, err := ioutil.ReadDir(frag)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not read fragment directory")
		}
		// TODO: Add a sort and filter option so that we can choose the
		// way we iterate through this directory to build out the file.
		for _, file := range files {
			if file.IsDir() { // skip recursive solutions for now...
				continue
			}
			f := path.Join(frag, file.Name())
			out, err := ioutil.ReadFile(f)
			if err != nil {
				return false, errwrap.Wrapf(err, "could not read directory file fragment")
			}
			content += string(out)
		}
	}

	// Actually write the file. This is similar to contentCheckApply.
	bufferSrc := bytes.NewReader([]byte(content))
	// NOTE: We pass in an invalidated sha256sum cache since we don't cache
	// all the individual files, and it could all change without us knowing.
	// TODO: Is the sha256sum caching even having an effect at all here ???
	sha256sum, checkOK, err := obj.fileCheckApply(apply, bufferSrc, obj.getPath(), "")
	if sha256sum != "" { // empty values mean errored or didn't hash
		// this can be valid even when the whole function errors
		obj.sha256sum = sha256sum // cache value
	}
	if err != nil {
		return false, err
	}
	// if no err, but !ok, then...
	return checkOK, nil // success
}

// chownCheckApply performs a CheckApply for the file ownership.
func (obj *FileRes) chownCheckApply(apply bool) (bool, error) {
	obj.init.Logf("chownCheckApply(%t)", apply)

	if obj.Owner == "" && obj.Group == "" {
		// no owner or group specified, everything is ok
		return true, nil
	}

	fileInfo, err := os.Stat(obj.getPath())
	// TODO: is this a sane behaviour that we want to preserve?
	// If the file does not exist and we are in noop mode, do not throw an
	// error.
	//if os.IsNotExist(err) && !apply {
	//	return false, nil
	//}
	if err != nil { // if the file does not exist, it's correct to error!
		return false, err
	}

	stUnix, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok { // this check is done in Validate, but it's done here again...
		// not unix
		return false, fmt.Errorf("can't set Owner or Group on this platform")
	}

	var expectedUID, expectedGID int

	if obj.Owner != "" {
		expectedUID, err = engineUtil.GetUID(obj.Owner)
		if err != nil {
			return false, err
		}
	} else {
		// nothing specified, no changes to be made, expect same as actual
		expectedUID = int(stUnix.Uid)
	}
	if obj.Group != "" {
		expectedGID, err = engineUtil.GetGID(obj.Group)
		if err != nil {
			return false, err
		}
	} else {
		// nothing specified, no changes to be made, expect same as actual
		expectedGID = int(stUnix.Gid)
	}

	// nothing to do
	if int(stUnix.Uid) == expectedUID && int(stUnix.Gid) == expectedGID {
		return true, nil
	}

	// not clean, but don't apply
	if !apply {
		return false, nil
	}

	return false, os.Chown(obj.getPath(), expectedUID, expectedGID)
}

// chmodCheckApply performs a CheckApply for the file permissions.
func (obj *FileRes) chmodCheckApply(apply bool) (bool, error) {
	obj.init.Logf("chmodCheckApply(%t)", apply)

	if obj.Mode == "" {
		// no mode specified, everything is ok
		return true, nil
	}

	mode, err := obj.mode() // get the desired mode
	if err != nil {
		return false, err
	}

	fileInfo, err := os.Stat(obj.getPath())
	if err != nil { // if the file does not exist, it's correct to error!
		return false, err
	}

	// nothing to do
	if fileInfo.Mode() == mode {
		return true, nil
	}

	// not clean but don't apply
	if !apply {
		return false, nil
	}

	return false, os.Chmod(obj.getPath(), mode)
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *FileRes) CheckApply(apply bool) (bool, error) {
	// NOTE: all send/recv change notifications *must* be processed before
	// there is a possibility of failure in CheckApply. This is because if
	// we fail (and possibly run again) the subsequent send->recv transfer
	// might not have a new value to copy, and therefore we won't see this
	// notification of change. Therefore, it is important to process these
	// promptly, if they must not be lost, such as for cache invalidation.
	if val, exists := obj.init.Recv()["Content"]; exists && val.Changed {
		// if we received on Content, and it changed, invalidate the cache!
		obj.init.Logf("contentCheckApply: invalidating sha256sum of `Content`")
		obj.sha256sum = "" // invalidate!!
	}

	checkOK := true

	// Run stateCheckApply before contentCheckApply, sourceCheckApply, and
	// fragmentsCheckApply.
	if c, err := obj.stateCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}
	if c, err := obj.contentCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}
	if c, err := obj.sourceCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}
	if c, err := obj.fragmentsCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	if c, err := obj.chownCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}
	if c, err := obj.chmodCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil // w00t
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FileRes) Cmp(r engine.Res) error {
	// we can only compare FileRes to others of the same resource kind
	res, ok := r.(*FileRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	// We don't need to compare Path, Dirname or Basename-- we only care if
	// the resultant path is different or not.
	if obj.getPath() != res.getPath() {
		return fmt.Errorf("the Path differs")
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	if (obj.Content == nil) != (res.Content == nil) { // xor
		return fmt.Errorf("the Content differs")
	}
	if obj.Content != nil && res.Content != nil {
		if *obj.Content != *res.Content { // compare the strings
			return fmt.Errorf("the contents of Content differ")
		}
	}
	if obj.Source != res.Source {
		return fmt.Errorf("the Source differs")
	}
	if len(obj.Fragments) != len(res.Fragments) {
		return fmt.Errorf("the number of Fragments differs")
	}
	for i, x := range obj.Fragments {
		if frag := res.Fragments[i]; x != frag {
			return fmt.Errorf("the fragment at index %d differs", i)
		}
	}

	if obj.Owner != res.Owner {
		return fmt.Errorf("the Owner differs")
	}
	if obj.Group != res.Group {
		return fmt.Errorf("the Group differs")
	}
	// TODO: when we start to allow alternate representations for the mode,
	// ensure that we compare in the same format. Eg: `ug=rw` == `0660`.
	if obj.Mode != res.Mode {
		return fmt.Errorf("the Mode differs")
	}

	if obj.Recurse != res.Recurse {
		return fmt.Errorf("the Recurse option differs")
	}
	if obj.Force != res.Force {
		return fmt.Errorf("the Force option differs")
	}
	if obj.Purge != res.Purge {
		return fmt.Errorf("the Purge option differs")
	}

	return nil
}

// FileUID is the UID struct for FileRes.
type FileUID struct {
	engine.BaseUID
	path string
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *FileUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*FileUID)
	if !ok {
		return false
	}
	return obj.path == res.path
}

// FileResAutoEdges holds the state of the auto edge generator.
type FileResAutoEdges struct {
	// We do all of these first...
	frags []engine.ResUID
	fdone bool

	// Then this is the second part...
	data    []engine.ResUID
	pointer int
	found   bool
}

// Next returns the next automatic edge.
func (obj *FileResAutoEdges) Next() []engine.ResUID {
	// We do all of these first...
	if !obj.fdone && len(obj.frags) > 0 {
		return obj.frags // return them all at the same time
	}

	// Then this is the second part...
	if obj.found {
		panic("Shouldn't be called anymore!")
	}
	if len(obj.data) == 0 { // check length for rare scenarios
		return nil
	}
	value := obj.data[obj.pointer]
	obj.pointer++
	return []engine.ResUID{value} // we return one, even though api supports N
}

// Test gets results of the earlier Next() call, & returns if we should
// continue!
func (obj *FileResAutoEdges) Test(input []bool) bool {
	// We do all of these first...
	if !obj.fdone && len(obj.frags) > 0 {
		obj.fdone = true // mark as done
		return true      // keep going
	}

	// Then this is the second part...
	// if there aren't any more remaining
	if len(obj.data) <= obj.pointer {
		return false
	}
	if obj.found { // already found, done!
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		panic("Expecting a single value!")
	}
	if input[0] { // if a match is found, we're done!
		obj.found = true // no more to find!
		return false
	}
	return true // keep going
}

// AutoEdges generates a simple linear sequence of each parent directory from
// the bottom up!
func (obj *FileRes) AutoEdges() (engine.AutoEdge, error) {
	var data []engine.ResUID // store linear result chain here...
	// don't use any memoization run in Init (this gets called before Init)
	values := util.PathSplitFullReversed(obj.getPath())
	_, values = values[0], values[1:] // get rid of first value which is me!
	for _, x := range values {
		var reversed = true // cheat by passing a pointer
		data = append(data, &FileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: x, // what matters
		}) // build list
	}

	// Ensure any file or dir fragments come first.
	frags := []engine.ResUID{}
	for _, frag := range obj.Fragments {
		var reversed = true // cheat by passing a pointer
		frags = append(frags, &FileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: frag, // what matters
		}) // build list

	}

	return &FileResAutoEdges{
		frags:   frags,
		data:    data,
		pointer: 0,
		found:   false,
	}, nil
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *FileRes) UIDs() []engine.ResUID {
	x := &FileUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		path:    obj.getPath(),
	}
	return []engine.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
//func (obj *FileRes) GroupCmp(r engine.GroupableRes) error {
//	_, ok := r.(*FileRes)
//	if !ok {
//		return fmt.Errorf("resource is not the same kind")
//	}
//	// TODO: we might be able to group directory children into a single
//	// recursive watcher in the future, thus saving fanotify watches
//	return fmt.Errorf("not possible at the moment")
//}

// CollectPattern applies the pattern for collection resources.
func (obj *FileRes) CollectPattern(pattern string) {
	// XXX: currently the pattern for files can only override the Dirname variable :P
	obj.Dirname = pattern // XXX: simplistic for now
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FileRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FileRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*FileRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FileRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FileRes(raw) // restore from indirection with type conversion!
	return nil
}

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *FileRes) Copy() engine.CopyableRes {
	var content *string
	if obj.Content != nil { // copy the string contents, not the pointer...
		s := *obj.Content
		content = &s
	}
	fragments := []string{}
	for _, frag := range obj.Fragments {
		fragments = append(fragments, frag)
	}
	return &FileRes{
		Path:      obj.Path,
		Dirname:   obj.Dirname,
		Basename:  obj.Basename,
		State:     obj.State, // TODO: if this becomes a pointer, copy the string!
		Content:   content,
		Source:    obj.Source,
		Fragments: fragments,
		Owner:     obj.Owner,
		Group:     obj.Group,
		Mode:      obj.Mode,
		Recurse:   obj.Recurse,
		Force:     obj.Force,
		Purge:     obj.Purge,
	}
}

// Reversed returns the "reverse" or "reciprocal" resource. This is used to
// "clean" up after a previously defined resource has been removed.
func (obj *FileRes) Reversed() (engine.ReversibleRes, error) {
	// NOTE: Previously, we did some more complicated management of reversed
	// properties. For example, we could add mode and state even when they
	// weren't originally specified. This code has now been simplified to
	// avoid this complexity, because it's not really necessary, and it is
	// somewhat illogical anyways.

	// TODO: reversing this could be tricky, since we'd store it all
	if obj.isDir() { // XXX: limit this error to a defined state or content?
		return nil, fmt.Errorf("can't reverse a dir yet")
	}

	cp, err := engine.ResCopy(obj)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not copy")
	}
	rev, ok := cp.(engine.ReversibleRes)
	if !ok {
		return nil, fmt.Errorf("not reversible")
	}
	rev.ReversibleMeta().Disabled = true // the reverse shouldn't run again

	res, ok := cp.(*FileRes)
	if !ok {
		return nil, fmt.Errorf("copied res was not our kind")
	}

	// these are already copied in, and we don't need to change them...
	//res.Path = obj.Path
	//res.Dirname = obj.Dirname
	//res.Basename = obj.Basename

	if obj.State == FileStateExists {
		res.State = FileStateAbsent
	}
	if obj.State == FileStateAbsent {
		res.State = FileStateExists
	}

	// If we've specified content, we might need to restore the original, OR
	// if we're removing the file with a `state => "absent"`, save it too...
	// We do this whether we specified content with Content or w/ Fragments.
	// The `res.State != FileStateAbsent` check is an optional optimization.
	if ((obj.Content != nil || len(obj.Fragments) > 0) || obj.State == FileStateAbsent) && res.State != FileStateAbsent {
		content, err := ioutil.ReadFile(obj.getPath())
		if err != nil && !os.IsNotExist(err) {
			return nil, errwrap.Wrapf(err, "could not read file for reversal storage")
		}
		res.Content = nil
		if err == nil {
			str := string(content)
			res.Content = &str // set contents
		}
	}
	if res.State == FileStateAbsent { // can't specify content when absent!
		res.Content = nil
	}

	//res.Source = "" // XXX: what should we do with this?
	if obj.Source != "" {
		return nil, fmt.Errorf("can't reverse with Source yet")
	}

	// We suck in the previous file contents above when Fragments is used...
	// This is basically the very same code path as when we reverse Content.
	// TODO: Do we want to do it this way or is there a better reverse path?
	if len(obj.Fragments) > 0 {
		res.Fragments = []string{}
	}

	// There is a race if the operating system is adding/changing/removing
	// the file between the ioutil.Readfile at the top and here. If there is
	// a discrepancy between the two, then you might get an unexpected
	// reverse, but in reality, your perspective is pretty absurd. This is a
	// user error, and not an issue we actually care about, afaict.
	fileInfo, err := os.Stat(obj.getPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, errwrap.Wrapf(err, "could not stat file for reversal information")
	}
	res.Owner = ""
	res.Group = ""
	res.Mode = ""
	if err == nil {
		stUnix, ok := fileInfo.Sys().(*syscall.Stat_t)
		// XXX: add a !ok error scenario or some alternative?
		if ok { // if not, this isn't unix
			if obj.Owner != "" {
				res.Owner = strconv.FormatInt(int64(stUnix.Uid), 10) // Uid is a uint32
			}
			if obj.Group != "" {
				res.Group = strconv.FormatInt(int64(stUnix.Gid), 10) // Gid is a uint32
			}
		}

		// TODO: use Mode().String() when we support full rwx style mode specs!
		if obj.Mode != "" {
			res.Mode = fmt.Sprintf("%#o", fileInfo.Mode().Perm()) // 0400, 0777, etc.
		}
	}

	// these are already copied in, and we don't need to change them...
	//res.Recurse = obj.Recurse
	//res.Force = obj.Force

	return res, nil
}

// GraphQueryAllowed returns nil if you're allowed to query the graph. This
// function accepts information about the requesting resource so we can
// determine the access with some form of fine-grained control.
func (obj *FileRes) GraphQueryAllowed(opts ...engine.GraphQueryableOption) error {
	options := &engine.GraphQueryableOptions{} // default options
	options.Apply(opts...)                     // apply the options
	if options.Kind != KindFile {
		return fmt.Errorf("only other files can access my information")
	}
	return nil
}

// smartPath adds a trailing slash to the path if it is a directory.
func smartPath(fileInfo os.FileInfo) string {
	smartPath := fileInfo.Name() // absolute path
	if fileInfo.IsDir() {
		smartPath += "/" // add a trailing slash for dirs
	}
	return smartPath
}

// FileInfo is an enhanced variant of the traditional os.FileInfo struct. It can
// store both the absolute and the relative paths (when built from our ReadDir),
// and those two paths contain a trailing slash when they refer to a directory.
type FileInfo struct {
	os.FileInfo        // embed
	AbsPath     string // smart variant
	RelPath     string // smart variant
}

// ReadDir reads a directory path, and returns a list of enhanced FileInfo's.
func ReadDir(path string) ([]FileInfo, error) {
	if !strings.HasSuffix(path, "/") { // dirs have trailing slashes
		return nil, fmt.Errorf("path must be a directory")
	}
	output := []FileInfo{} // my file info
	fileInfos, err := ioutil.ReadDir(path)
	if os.IsNotExist(err) {
		return output, err // return empty list
	}
	if err != nil {
		return nil, err
	}
	for _, fi := range fileInfos {
		abs := path + smartPath(fi)
		rel, err := filepath.Rel(path, abs) // NOTE: calls Clean()
		if err != nil {                     // shouldn't happen
			return nil, errwrap.Wrapf(err, "unhandled error in ReadDir")
		}
		if fi.IsDir() {
			rel += "/" // add a trailing slash for dirs
		}
		x := FileInfo{
			FileInfo: fi,
			AbsPath:  abs,
			RelPath:  rel,
		}
		output = append(output, x)
	}
	return output, nil
}

// smartMapPaths adds a trailing slash to every path that is a directory. It
// returns the data as a map where the keys are the smart paths and where the
// values are the original os.FileInfo entries.
func mapPaths(fileInfos []FileInfo) map[string]FileInfo {
	paths := make(map[string]FileInfo)
	for _, fileInfo := range fileInfos {
		paths[fileInfo.RelPath] = fileInfo
	}
	return paths
}
