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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// KindTar is the kind string used to identify this resource.
	KindTar = "tar"
	// ParamTarState is the name of the state field parameter.
	ParamTarState = "state"
	// TarStateExists is the string that represents that the tar file should exist
	TarStateExists = "exists"
	// TarStateAbsent is the string that represents that the tar file should not exist.
	TarStateAbsent = "absent"
)

func init() {
	engine.RegisterResource(KindTar, func() engine.Res { return &TarRes{} })

	vars.RegisterResourceParams(KindTar, map[string]map[string]func() interfaces.Var{
		ParamTarState: {
			TarStateExists: func() interfaces.Var {
				return &types.StrValue{
					V: TarStateExists,
				}
			},
			TarStateAbsent: func() interfaces.Var {
				return &types.StrValue{
					V: TarStateAbsent,
				}
			},
		},
	})
}

// TarRes
type TarRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.GraphQueryable // allow others to query this res in the res graph

	init *engine.Init

	// Inputs are the input filepaths for the tar resource
	Inputs []string `lang:"inputs" yaml:"inputs"`

	// State is the desired state of the tar resource. It can either be 'exist' or 'absent'
	State string `lang:"state" yaml:"state"`

	// Compress is a flag to determine whether to compress the tar archive with
	// gzip or not. By default it is set to false (which won't compress the archive)
	Compress bool `lang:"compress" yaml:"compress"`

	// The path to local storage
	vardir string

	// The tar resource needs to determine if its input files are "dirty":
	// if so, checkApply will create a new tar resource.
	// The tar resource uses a hashmap to determine whether files are dirty or not.
	// Each key is an input file for the tar resource and
	// each value is a hash of the respective input file's content
	inputsHashMapFilepath string
}

// Default returns some sensible defaults for this resource.
func (obj *TarRes) Default() engine.Res {
	return &TarRes{}
}

// Validate if the params passed in are valid data.
func (obj *TarRes) Validate() error {
	if len(obj.Inputs) == 0 {
		return fmt.Errorf("expected one or more inputs")
	}
	if obj.Compress && !strings.HasSuffix(strings.ToLower(obj.Name()), ".gz") {
		return fmt.Errorf("expected a suffix of .gz, .Gz, or .gZ for compressed archive")
	}
	if obj.State != TarStateExists && obj.State != TarStateAbsent {
		return fmt.Errorf("the State is invalid")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *TarRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// enables Debug logs
	obj.init.Debug = false

	dir, err := obj.init.VarDir("tar")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.vardir = dir
	obj.inputsHashMapFilepath = path.Join(obj.vardir, "Inputs_hashmap.json")
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *TarRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TarRes) Watch() error {

	inputEvents := make(chan recwatch.Event)
	defer close(inputEvents)

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	exit := make(chan struct{})
	defer close(exit)

	// need to send events from inputs to the tar file watcher
	for _, inputFilepath := range obj.Inputs {
		stat, err := os.Stat(inputFilepath)
		if err != nil {
			return err
		}
		recurse := stat.IsDir()
		rwInput, err := recwatch.NewRecWatcher(inputFilepath, recurse)
		if err != nil {
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {

				var event recwatch.Event
				var ok bool
				var shutdown bool

				select {
				case event, ok = <-rwInput.Events():
				case <-exit:
					return
				}

				if !ok {
					err := fmt.Errorf("channel shutdown")
					event = recwatch.Event{Error: err}
					shutdown = true
				}

				select {
				// need to signal the tar file watcher that an input is dirty
				case inputEvents <- event:
					if shutdown {
						return
					}
				case <-exit:
					return
				}
			}
		}()
	}

	// when started, notify engine that we're running
	obj.init.Running()

	rwTar, err := recwatch.NewRecWatcher(obj.Name(), false)
	if err != nil {
		return err
	}
	defer rwTar.Close()

	var send = false
	for {

		select {

		// from resource
		case event, ok := <-rwTar.Events():
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug {
				// don't access event.Body if event.Error isn't nil
				obj.init.Logf("Event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		// from inputs
		case event, ok := <-inputEvents:
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s input watcher error", obj)
			}
			if obj.init.Debug {
				// don't access event.Body if event.Error isn't nil
				obj.init.Logf("input event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		if send {
			send = false
			obj.init.Event()
		}
	}
}

// CheckApply method for Tar resource
func (obj *TarRes) CheckApply(apply bool) (bool, error) {

	// Need the inputs hashmap if it doesn't exist to determine whether input files have been modified
	_, err := os.Stat(obj.inputsHashMapFilepath)
	if os.IsNotExist(err) {
		err = obj.initInputsHashMap()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not initialize vardir file for Inputs hashmap")
		}
	}

	content, err := os.ReadFile(obj.inputsHashMapFilepath)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not open inputs hashmap")
	}
	var inputsHashmap map[string][]byte
	if err := json.Unmarshal(content, &inputsHashmap); err != nil {
		return false, errwrap.Wrapf(err, "could not unmarshal Inputs hashmap")
	}

	// init or remove tar
	_, err = os.Stat(obj.Name())
	if obj.State == TarStateExists && os.IsNotExist(err) {
		obj.createTar()
	} else if obj.State == TarStateAbsent && !os.IsNotExist(err) {
		os.Remove(obj.Name())
	}

	// check
	checkOk := true
	// optimization: no need to check input files if the tar resource shouldn't exist
	if obj.State == TarStateExists {
		for _, inputPath := range obj.Inputs {

			checkOk, err = obj.checkFile(inputPath, inputsHashmap)
			if err != nil {
				return false, errwrap.Wrapf(err, "could not check file %s", inputPath)
			}
		}
	}

	// apply
	if !checkOk && obj.State == TarStateExists {
		err := obj.applyTar()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not apply tar")
		}
		checkOk = true
	}

	return checkOk, nil
}

func (obj *TarRes) applyTar() error {
	err := obj.createTar()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(obj.inputsHashMapFilepath)
	if err != nil {
		return err
	}

	var inputsHashmap map[string][]byte
	json.Unmarshal(content, &inputsHashmap)

	// Update inputs hashmap
	for _, inputPath := range obj.Inputs {
		obj.insertHash(inputPath, inputsHashmap)
	}

	return nil
}

// Checks if an input file is dirty
func (obj *TarRes) checkFile(filepath string, hashmap map[string][]byte) (bool, error) {
	file, err := os.Stat(filepath)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not get file stats for %s", filepath)
	}

	checkOK := true
	if file.IsDir() {
		files, err := os.ReadDir(filepath)
		if err != nil {
			return false, err
		}
		for _, file := range files {
			result, err := obj.checkFile(path.Join(filepath, file.Name()), hashmap)
			if err != nil {
				return false, err
			}
			checkOK = checkOK && result
			if obj.init.Debug {
				obj.init.Logf("checkOK for input %s: %t \n", path.Join(filepath, file.Name()), checkOK)
			}
		}
	} else {
		currentHash, err := obj.createHash(filepath)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not create hash from input file content")
		}
		oldHash := hashmap[filepath]
		if !bytes.Equal(oldHash, currentHash) {
			checkOK = false
			if obj.init.Debug {
				obj.init.Logf("oldHash %s != currentHash %s\n", oldHash, currentHash)
			}
		}
	}
	return checkOK, nil
}

func (obj *TarRes) initInputsHashMap() error {
	inputsHashmap := map[string][]byte{}

	for _, inputPath := range obj.Inputs {
		obj.insertHash(inputPath, inputsHashmap)
	}

	bytes, err := json.Marshal(inputsHashmap)
	if err != nil {
		return err
	}

	err = os.WriteFile(obj.inputsHashMapFilepath, bytes, 0666)
	if err != nil {
		return err
	}

	return nil
}

// Creates a hash from some input file's content
func (obj *TarRes) createHash(filepath string) ([]byte, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, 30*1024)
	hash := sha256.New()
	for {
		n, err := file.Read(buffer)

		if err == io.EOF {
			break
		}

		if err != nil {
			break
		}

		if n > 0 {
			_, err := hash.Write(buffer[:n])
			if err != nil {
				return nil, err
			}
		}
	}

	sum := hash.Sum(nil)
	return sum, nil
}

func (obj *TarRes) insertHash(filepath string, hashmap map[string][]byte) (map[string][]byte, error) {

	fileStat, err := os.Stat(filepath)
	if err != nil {
		return hashmap, err
	}

	if fileStat.IsDir() {
		files, err := os.ReadDir(filepath)
		if err != nil {
			return hashmap, err
		}
		for _, file := range files {
			obj.insertHash(path.Join(filepath, file.Name()), hashmap)
		}
	} else {
		hash, err := obj.createHash(filepath)
		if err != nil {
			return hashmap, err
		}
		hashmap[filepath] = hash
	}

	return hashmap, nil
}

func (obj *TarRes) createTar() error {

	tarfile, err := os.Create(obj.Name())
	if err != nil {
		return err
	}
	defer tarfile.Close()

	if obj.init.Debug {
		obj.init.Logf("writing to tar\n")
	}
	var tarWriter *tar.Writer
	if obj.Compress {
		gzipWriter := gzip.NewWriter(tarfile)
		tarWriter = tar.NewWriter(gzipWriter)
		defer gzipWriter.Close()
	} else {
		tarWriter = tar.NewWriter(tarfile)
	}
	defer tarWriter.Close()

	for _, filepath := range obj.Inputs {
		obj.writeToArchive(tarWriter, filepath)
	}

	return nil
}

func (obj *TarRes) writeToArchive(tarWriter *tar.Writer, filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// The `tar` command first archives the folder and only after does it archive the folder contents
	if stat.IsDir() {
		files, err := os.ReadDir(filepath)
		if err != nil {
			return err
		}
		for _, file := range files {
			obj.writeToArchive(tarWriter, path.Join(filepath, file.Name()))
		}
	}

	// We need to create a header to preserve the directory structure of the
	// input files
	header, err := tar.FileInfoHeader(stat, stat.Name())
	if err != nil {
		return err
	}

	// Need to remove the leading '/' to copy the same behaviour as 'tar'
	// Otherwise, the files are inacessible
	header.Name = strings.TrimPrefix(filepath, "/")
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	if _, err := io.Copy(tarWriter, file); err != nil {
		return err
	}

	return nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TarRes) Cmp(r engine.Res) error {
	// we can only compare TarRes to others of the same resource kind
	res, ok := r.(*TarRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if len(obj.Inputs) != len(res.Inputs) {
		return fmt.Errorf("the Inputs differ")
	}

	for i := range obj.Inputs {
		if obj.Inputs[i] != res.Inputs[i] {
			return fmt.Errorf("the Inputs differ")
		}
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	if obj.Compress != res.Compress {
		return fmt.Errorf("the Compress parameter differs")
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
