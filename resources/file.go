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
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global" // XXX: package mgmtmain instead?
	"github.com/purpleidea/mgmt/util"

	"gopkg.in/fsnotify.v1"
	//"github.com/go-fsnotify/fsnotify" // git master of "gopkg.in/fsnotify.v1"
)

func init() {
	gob.Register(&FileRes{})
}

// FileRes is a file and directory resource.
type FileRes struct {
	BaseRes   `yaml:",inline"`
	Path      string `yaml:"path"` // path variable (should default to name)
	Dirname   string `yaml:"dirname"`
	Basename  string `yaml:"basename"`
	Content   string `yaml:"content"` // FIXME: how do you describe: "leave content alone" - state = "create" ?
	Source    string `yaml:"source"`  // file path for source content
	State     string `yaml:"state"`   // state: exists/present?, absent, (undefined?)
	Recurse   bool   `yaml:"recurse"`
	Force     bool   `yaml:"force"`
	path      string // computed path
	isDir     bool   // computed isDir
	sha256sum string
	watcher   *fsnotify.Watcher
	watches   map[string]struct{}
}

// NewFileRes is a constructor for this resource. It also calls Init() for you.
func NewFileRes(name, path, dirname, basename, content, source, state string, recurse, force bool) *FileRes {
	obj := &FileRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Path:     path,
		Dirname:  dirname,
		Basename: basename,
		Content:  content,
		Source:   source,
		State:    state,
		Recurse:  recurse,
		Force:    force,
	}
	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *FileRes) Init() {
	obj.sha256sum = ""
	obj.watches = make(map[string]struct{})
	if obj.Path == "" { // use the name as the path default if missing
		obj.Path = obj.BaseRes.Name
	}
	obj.path = obj.GetPath()                     // compute once
	obj.isDir = strings.HasSuffix(obj.path, "/") // dirs have trailing slashes

	obj.BaseRes.kind = "File"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

// GetPath returns the actual path to use for this resource. It computes this
// after analysis of the Path, Dirname and Basename values. Dirs end with slash.
func (obj *FileRes) GetPath() string {
	d := util.Dirname(obj.Path)
	b := util.Basename(obj.Path)
	if obj.Dirname == "" && obj.Basename == "" {
		return obj.Path
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

// Validate reports any problems with the struct definition.
func (obj *FileRes) Validate() error {
	if obj.Dirname != "" && !strings.HasSuffix(obj.Dirname, "/") {
		return fmt.Errorf("Dirname must end with a slash.")
	}

	if strings.HasPrefix(obj.Basename, "/") {
		return fmt.Errorf("Basename must not start with a slash.")
	}

	if obj.Content != "" && obj.Source != "" {
		return fmt.Errorf("Can't specify both Content and Source.")
	}

	if obj.isDir && obj.Content != "" { // makes no sense
		return fmt.Errorf("Can't specify Content when creating a Dir.")
	}

	// XXX: should this specify that we create an empty directory instead?
	//if obj.Source == "" && obj.isDir {
	//	return fmt.Errorf("Can't specify an empty source when creating a Dir.")
	//}

	return nil
}

// addSubFolders is a helper that is used to add recursive dirs to the watches.
func (obj *FileRes) addSubFolders(p string) error {
	if !obj.Recurse {
		return nil // if we're not watching recursively, just exit early
	}
	// look at all subfolders...
	walkFn := func(path string, info os.FileInfo, err error) error {
		if global.DEBUG {
			log.Printf("%s[%s]: Walk: %s (%v): %v", obj.Kind(), obj.GetName(), path, info, err)
		}
		if err != nil {
			return nil
		}
		if info.IsDir() {
			obj.watches[path] = struct{}{} // add key
			err := obj.watcher.Add(path)
			if err != nil {
				return err // TODO: will this bubble up?
			}
		}
		return nil
	}
	err := filepath.Walk(p, walkFn)
	return err
}

// Watch is the primary listener for this resource and it outputs events.
// This one is a file watcher for files and directories.
// Modify with caution, it is probably important to write some test cases first!
// If the Watch returns an error, it means that something has gone wrong, and it
// must be restarted. On a clean exit it returns nil.
// FIXME: Also watch the source directory when using obj.Source !!!
func (obj *FileRes) Watch(processChan chan event.Event) error {
	if obj.IsWatching() {
		return nil // TODO: should this be an error?
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var startup bool
	Startup := func(block bool) <-chan time.Time {
		if block {
			return nil // blocks forever
			//return make(chan time.Time) // blocks forever
		}
		return time.After(time.Duration(500) * time.Millisecond) // 1/2 the resolution of converged timeout
	}

	var safename = path.Clean(obj.path) // no trailing slash

	var err error
	obj.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer obj.watcher.Close()

	patharray := util.PathSplit(safename) // tokenize the path
	var index = len(patharray)            // starting index
	var current string                    // current "watcher" location
	var deltaDepth int                    // depth delta between watcher and event
	var send = false                      // send event?
	var exit = false
	var dirty = false

	isDir := func(p string) bool {
		finfo, err := os.Stat(p)
		if err != nil {
			return false
		}
		return finfo.IsDir()
	}

	if obj.isDir {
		if err := obj.addSubFolders(safename); err != nil {
			return err
		}
	}
	for {
		current = strings.Join(patharray[0:index], "/")
		if current == "" { // the empty string top is the root dir ("/")
			current = "/"
		}
		if global.DEBUG {
			log.Printf("%s[%s]: Watching: %v", obj.Kind(), obj.GetName(), current) // attempting to watch...
		}
		// initialize in the loop so that we can reset on rm-ed handles
		err = obj.watcher.Add(current)
		if err != nil {
			if global.DEBUG {
				log.Printf("%s[%s]: watcher.Add(%v): Error: %v", obj.Kind(), obj.GetName(), current, err)
			}
			if err == syscall.ENOENT {
				index-- // usually not found, move up one dir
			} else if err == syscall.ENOSPC {
				// no space left on device, out of inotify watches
				// TODO: consider letting the user fall back to
				// polling if they hit this error very often...
				return fmt.Errorf("%s[%s]: Out of inotify watches: %v", obj.Kind(), obj.GetName(), err)
			} else if os.IsPermission(err) {
				return fmt.Errorf("%s[%s]: Permission denied to add a watch: %v", obj.Kind(), obj.GetName(), err)
			} else {
				return fmt.Errorf("Unknown %s[%s] error: %v", obj.Kind(), obj.GetName(), err)
			}
			index = int(math.Max(1, float64(index)))
			continue
		}

		obj.SetState(ResStateWatching) // reset
		select {
		case event := <-obj.watcher.Events:
			if global.DEBUG {
				log.Printf("%s[%s]: Watch(%s), Event(%s): %v", obj.Kind(), obj.GetName(), current, event.Name, event.Op)
			}
			cuuid.SetConverged(false) // XXX: technically i can detect if the event is erroneous or not first
			// the deeper you go, the bigger the deltaDepth is...
			// this is the difference between what we're watching,
			// and the event... doesn't mean we can't watch deeper
			if current == event.Name {
				deltaDepth = 0 // i was watching what i was looking for

			} else if util.HasPathPrefix(event.Name, current) {
				deltaDepth = len(util.PathSplit(current)) - len(util.PathSplit(event.Name)) // -1 or less

			} else if util.HasPathPrefix(current, event.Name) {
				deltaDepth = len(util.PathSplit(event.Name)) - len(util.PathSplit(current)) // +1 or more
				// if below me...
				if _, exists := obj.watches[event.Name]; exists {
					send = true
					dirty = true
					if event.Op&fsnotify.Remove == fsnotify.Remove {
						obj.watcher.Remove(event.Name)
						delete(obj.watches, event.Name)
					}
					if (event.Op&fsnotify.Create == fsnotify.Create) && isDir(event.Name) {
						obj.watcher.Add(event.Name)
						obj.watches[event.Name] = struct{}{}
						if err := obj.addSubFolders(event.Name); err != nil {
							return err
						}
					}
				}

			} else {
				// TODO different watchers get each others events!
				// https://github.com/go-fsnotify/fsnotify/issues/95
				// this happened with two values such as:
				// event.Name: /tmp/mgmt/f3 and current: /tmp/mgmt/f2
				continue
			}
			//log.Printf("The delta depth is: %v", deltaDepth)

			// if we have what we wanted, awesome, send an event...
			if event.Name == safename {
				//log.Println("Event!")
				// FIXME: should all these below cases trigger?
				send = true
				dirty = true

				if obj.isDir {
					if err := obj.addSubFolders(safename); err != nil {
						return err
					}
				}

				// file removed, move the watch upwards
				if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					//log.Println("Removal!")
					obj.watcher.Remove(current)
					index--
				}

				// we must be a parent watcher, so descend in
				if deltaDepth < 0 {
					obj.watcher.Remove(current)
					index++
				}

				// if safename starts with event.Name, we're above, and no event should be sent
			} else if util.HasPathPrefix(safename, event.Name) {
				//log.Println("Above!")

				if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					log.Println("Removal!")
					obj.watcher.Remove(current)
					index--
				}

				if deltaDepth < 0 {
					log.Println("Parent!")
					if util.PathPrefixDelta(safename, event.Name) == 1 { // we're the parent dir
						send = true
						dirty = true
					}
					obj.watcher.Remove(current)
					index++
				}

				// if event.Name startswith safename, send event, we're already deeper
			} else if util.HasPathPrefix(event.Name, safename) {
				//log.Println("Event2!")
				send = true
				dirty = true
			}

		case err := <-obj.watcher.Errors:
			cuuid.SetConverged(false)
			return fmt.Errorf("Unknown %s[%s] watcher error: %v", obj.Kind(), obj.GetName(), err)

		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}
			//dirty = false // these events don't invalidate state

		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
			continue

		case <-Startup(startup):
			cuuid.SetConverged(false)
			send = true
			dirty = true
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			startup = true // startup finished
			send = false
			// only invalid state on certain types of events
			if dirty {
				dirty = false
				obj.isStateOK = false // something made state dirty
			}
			if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
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
		return nil, fmt.Errorf("Path must be a directory.")
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
			return nil, fmt.Errorf("ReadDir: Unhandled error: %v", err)
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

// fileCheckApply is the CheckApply operation for a source and destination file.
// It can accept an io.Reader as the source, which can be a regular file, or it
// can be a bytes Buffer struct. It can take an input sha256 hash to use instead
// of computing the source data hash, and it returns the computed value if this
// function reaches that stage. As usual, it respects the apply action variable,
// and it symmetry with the main CheckApply function returns checkOK and error.
func (obj *FileRes) fileCheckApply(apply bool, src io.ReadSeeker, dst string, sha256sum string) (string, bool, error) {
	// TODO: does it make sense to switch dst to an io.Writer ?
	// TODO: use obj.Force when dealing with symlinks and other file types!
	if global.DEBUG {
		log.Printf("fileCheckApply: %s -> %s", src, dst)
	}

	srcFile, isFile := src.(*os.File)
	_, isBytes := src.(*bytes.Reader) // supports seeking!
	if !isFile && !isBytes {
		return "", false, fmt.Errorf("Can't open src as either file or buffer!")
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
			return "", false, fmt.Errorf("Non-regular src file: %s (%q)", srcStat.Name(), srcStat.Mode())
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

	dstStat, err := dstFile.Stat()
	if err != nil && dstExists {
		return "", false, err
	}

	if dstExists && dstStat.IsDir() { // oops, dst is a dir, and we want a file...
		if !apply {
			return "", false, nil
		}
		if !obj.Force {
			return "", false, fmt.Errorf("Can't force dir into file: %s", dst)
		}

		cleanDst := path.Clean(dst)
		if cleanDst == "" || cleanDst == "/" {
			return "", false, fmt.Errorf("Don't want to remove root!") // safety
		}
		// FIXME: respect obj.Recurse here...
		// there is a dir here, where we want a file...
		log.Printf("fileCheckApply: Removing (force): %s", cleanDst)
		if err := os.RemoveAll(cleanDst); err != nil { // dangerous ;)
			return "", false, err
		}
		dstExists = false // now it's gone!

	} else if err == nil {
		if !dstStat.Mode().IsRegular() {
			return "", false, fmt.Errorf("Non-regular dst file: %s (%q)", dstStat.Name(), dstStat.Mode())
		}
		if isFile && os.SameFile(srcStat, dstStat) { // same inode, we're done!
			return "", true, nil
		}
	}

	if dstExists { // if dst doesn't exist, no need to compare hashes
		// hash comparison (efficient because we can cache hash of content str)
		if sha256sum == "" { // cache is invalid
			hash := sha256.New()
			// TODO file existence test?
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
	if global.DEBUG {
		log.Printf("fileCheckApply: Apply: %s -> %s", src, dst)
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
	if global.DEBUG {
		log.Printf("fileCheckApply: Copy: %s -> %s", src, dst)
	}
	if n, err := io.Copy(dstFile, src); err != nil {
		return sha256sum, false, err
	} else if global.DEBUG {
		log.Printf("fileCheckApply: Copied: %v", n)
	}
	return sha256sum, false, dstFile.Sync()
}

// syncCheckApply is the CheckApply operation for a source and destination dir.
// It is recursive and can create directories directly, and files via the usual
// fileCheckApply method. It returns checkOK and error as is normally expected.
func (obj *FileRes) syncCheckApply(apply bool, src, dst string) (bool, error) {
	if global.DEBUG {
		log.Printf("syncCheckApply: %s -> %s", src, dst)
	}
	if src == "" || dst == "" {
		return false, fmt.Errorf("The src and dst must not be empty!")
	}

	var checkOK = true
	// TODO: handle ./ cases or ../ cases that need cleaning ?

	srcIsDir := strings.HasSuffix(src, "/")
	dstIsDir := strings.HasSuffix(dst, "/")

	if srcIsDir != dstIsDir {
		return false, fmt.Errorf("The src and dst must be both either files or directories.")
	}

	if !srcIsDir && !dstIsDir {
		if global.DEBUG {
			log.Printf("syncCheckApply: %s -> %s", src, dst)
		}
		fin, err := os.Open(src)
		if err != nil {
			if global.DEBUG && os.IsNotExist(err) { // if we get passed an empty src
				log.Printf("syncCheckApply: Missing src: %s", src)
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
	srcFiles, err := ReadDir(src)          // if src does not exist...
	if err != nil && !os.IsNotExist(err) { // an empty map comes out below!
		return false, err
	}
	dstFiles, err := ReadDir(dst)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	//log.Printf("syncCheckApply: srcFiles: %v", srcFiles)
	//log.Printf("syncCheckApply: dstFiles: %v", dstFiles)
	smartSrc := mapPaths(srcFiles)
	smartDst := mapPaths(dstFiles)

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
					if !obj.Force {
						return false, fmt.Errorf("Can't force file into dir: %s", absCleanDst)
					}
					if absCleanDst == "" || absCleanDst == "/" {
						return false, fmt.Errorf("Don't want to remove root!") // safety
					}
					log.Printf("syncCheckApply: Removing (force): %s", absCleanDst)
					if err := os.Remove(absCleanDst); err != nil {
						return false, err
					}
					delete(smartDst, relPathFile) // rm from purge list
				}

				if global.DEBUG {
					log.Printf("syncCheckApply: mkdir -m %s '%s'", fileInfo.Mode(), absDst)
				}
				if err := os.Mkdir(absDst, fileInfo.Mode()); err != nil {
					return false, err
				}
				checkOK = false // we did some work
			}
			// if we're a regular file, the recurse will create it
		}

		if global.DEBUG {
			log.Printf("syncCheckApply: Recurse: %s -> %s", absSrc, absDst)
		}
		if obj.Recurse {
			if c, err := obj.syncCheckApply(apply, absSrc, absDst); err != nil { // recurse
				return false, fmt.Errorf("syncCheckApply: Recurse failed: %v", err)
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
	// any files that now remain in smartDst need to be removed...
	for relPath, fileInfo := range smartDst {
		absSrc := src + relPath    // absolute dest (should not exist!)
		absDst := fileInfo.AbsPath // absolute path (should get removed)
		absCleanDst := path.Clean(absDst)
		if absCleanDst == "" || absCleanDst == "/" {
			return false, fmt.Errorf("Don't want to remove root!") // safety
		}

		// FIXME: respect obj.Recurse here...

		// NOTE: we could use os.RemoveAll instead of recursing, but I
		// think the symmetry is more elegant and correct here for now
		// Avoiding this is also useful if we had a recurse limit arg!
		if true { // switch
			log.Printf("syncCheckApply: Removing: %s", absCleanDst)
			if apply {
				if err := os.RemoveAll(absCleanDst); err != nil { // dangerous ;)
					return false, err
				}
				checkOK = false
			}
			continue
		}
		_ = absSrc
		//log.Printf("syncCheckApply: Recurse rm: %s -> %s", absSrc, absDst)
		//if c, err := obj.syncCheckApply(apply, absSrc, absDst); err != nil {
		//	return false, fmt.Errorf("syncCheckApply: Recurse rm failed: %v", err)
		//} else if !c { // don't let subsequent passes make this true
		//	checkOK = false
		//}
		//log.Printf("syncCheckApply: Removing: %s", absCleanDst)
		//if apply { // safety
		//	if err := os.Remove(absCleanDst); err != nil {
		//		return false, err
		//	}
		//	checkOK = false
		//}
	}

	return checkOK, nil
}

// contentCheckApply performs a CheckApply for the file existence and content.
func (obj *FileRes) contentCheckApply(apply bool) (checkOK bool, _ error) {
	log.Printf("%v[%v]: contentCheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.State == "absent" {
		if _, err := os.Stat(obj.path); os.IsNotExist(err) {
			// no such file or directory, but
			// file should be missing, phew :)
			return true, nil

		} else if err != nil { // what could this error be?
			return false, err
		}

		// state is not okay, no work done, exit, but without error
		if !apply {
			return false, nil
		}

		// apply portion
		if obj.path == "" || obj.path == "/" {
			return false, fmt.Errorf("Don't want to remove root!") // safety
		}
		log.Printf("contentCheckApply: Removing: %s", obj.path)
		// FIXME: respect obj.Recurse here...
		// TODO: add recurse limit here
		err := os.RemoveAll(obj.path) // dangerous ;)
		return false, err             // either nil or not
	}

	if obj.Source == "" { // do the obj.Content checks first...
		if obj.isDir { // TODO: should we create an empty dir this way?
			log.Fatal("XXX: Not implemented!") // XXX
		}

		bufferSrc := bytes.NewReader([]byte(obj.Content))
		sha256sum, checkOK, err := obj.fileCheckApply(apply, bufferSrc, obj.path, obj.sha256sum)
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

	checkOK, err := obj.syncCheckApply(apply, obj.Source, obj.path)
	if err != nil {
		log.Printf("syncCheckApply: Error: %v", err)
		return false, err
	}

	return checkOK, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *FileRes) CheckApply(apply bool) (checkOK bool, _ error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.isStateOK { // cache the state
		return true, nil
	}

	checkOK = true

	if c, err := obj.contentCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// TODO
	//if c, err := obj.chmodCheckApply(apply); err != nil {
	//	return false, err
	//} else if !c {
	//	checkOK = false
	//}

	// TODO
	//if c, err := obj.chownCheckApply(apply); err != nil {
	//	return false, err
	//} else if !c {
	//	checkOK = false
	//}

	// if we did work successfully, or are in a good state, then state is ok
	if apply || checkOK {
		obj.isStateOK = true
	}
	return checkOK, nil // w00t
}

// FileUUID is the UUID struct for FileRes.
type FileUUID struct {
	BaseUUID
	path string
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *FileUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*FileUUID)
	if !ok {
		return false
	}
	return obj.path == res.path
}

// FileResAutoEdges holds the state of the auto edge generator.
type FileResAutoEdges struct {
	data    []ResUUID
	pointer int
	found   bool
}

// Next returns the next automatic edge.
func (obj *FileResAutoEdges) Next() []ResUUID {
	if obj.found {
		log.Fatal("Shouldn't be called anymore!")
	}
	if len(obj.data) == 0 { // check length for rare scenarios
		return nil
	}
	value := obj.data[obj.pointer]
	obj.pointer++
	return []ResUUID{value} // we return one, even though api supports N
}

// Test gets results of the earlier Next() call, & returns if we should continue!
func (obj *FileResAutoEdges) Test(input []bool) bool {
	// if there aren't any more remaining
	if len(obj.data) <= obj.pointer {
		return false
	}
	if obj.found { // already found, done!
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		log.Fatal("Expecting a single value!")
	}
	if input[0] { // if a match is found, we're done!
		obj.found = true // no more to find!
		return false
	}
	return true // keep going
}

// AutoEdges generates a simple linear sequence of each parent directory from
// the bottom up!
func (obj *FileRes) AutoEdges() AutoEdge {
	var data []ResUUID                             // store linear result chain here...
	values := util.PathSplitFullReversed(obj.path) // build it
	_, values = values[0], values[1:]              // get rid of first value which is me!
	for _, x := range values {
		var reversed = true // cheat by passing a pointer
		data = append(data, &FileUUID{
			BaseUUID: BaseUUID{
				name:     obj.GetName(),
				kind:     obj.Kind(),
				reversed: &reversed,
			},
			path: x, // what matters
		}) // build list
	}
	return &FileResAutoEdges{
		data:    data,
		pointer: 0,
		found:   false,
	}
}

// GetUUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *FileRes) GetUUIDs() []ResUUID {
	x := &FileUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		path:     obj.path,
	}
	return []ResUUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *FileRes) GroupCmp(r Res) bool {
	_, ok := r.(*FileRes)
	if !ok {
		return false
	}
	// TODO: we might be able to group directory children into a single
	// recursive watcher in the future, thus saving fanotify watches
	return false // not possible atm
}

// Compare two resources and return if they are equivalent.
func (obj *FileRes) Compare(res Res) bool {
	switch res.(type) {
	case *FileRes:
		res := res.(*FileRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

		if obj.Name != res.Name {
			return false
		}
		if obj.path != res.Path {
			return false
		}
		if obj.Content != res.Content {
			return false
		}
		if obj.Source != res.Source {
			return false
		}
		if obj.State != res.State {
			return false
		}
		if obj.Recurse != res.Recurse {
			return false
		}
		if obj.Force != res.Force {
			return false
		}
	default:
		return false
	}
	return true
}

// CollectPattern applies the pattern for collection resources.
func (obj *FileRes) CollectPattern(pattern string) {
	// XXX: currently the pattern for files can only override the Dirname variable :P
	obj.Dirname = pattern // XXX: simplistic for now
}
