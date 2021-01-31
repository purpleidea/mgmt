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

package lang

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Module logic: We want one single file to start from. That single file will
// have `import` or `append` statements in it. Those pull in everything else. To
// kick things off, we point directly at a metadata.yaml file path, which points
// to the main.mcl entry file. Alternatively, we could point directly to a
// main.mcl file, but then we wouldn't know to import additional special dirs
// like files/ that are listed in the metadata.yaml file. If we point to a
// directory, we could read a metadata.yaml file, and if missing, read a
// main.mcl file. The ideal way to start mgmt is by pointing to a metadata.yaml
// file. It's ideal because *it* could list the dir path for files/ or
// templates/ or other modules. If we start with a single foo.mcl file, the file
// itself won't have files/ however other files/ can come in from an import that
// contains a metadata.yaml file. Below we have the input parsers that implement
// some of this logic.

var (
	// inputOrder contains the correct running order of the input functions.
	inputOrder = []func(string, engine.Fs) (*ParsedInput, error){
		inputEmpty,
		inputStdin,
		inputMetadata,
		inputMcl,
		inputDirectory,
		inputCode,
		//inputFail,
	}
)

// ParsedInput is the output struct which contains all the information we need.
type ParsedInput struct {
	//activated bool // if struct is not nil we're activated
	Base     string   // base path (abs path with trailing slash)
	Main     []byte   // contents of main entry mcl code
	Files    []string // files and dirs to copy to fs (abs paths)
	Metadata *interfaces.Metadata
	Workers  []func(engine.Fs) error // copy files here that aren't listed!
}

// parseInput runs the list if input parsers to know how to run the lexer,
// parser, and so on... The fs input is the source filesystem to look in.
func parseInput(s string, fs engine.Fs) (*ParsedInput, error) {
	var err error
	var output *ParsedInput
	activated := false
	// i decided this was a cleaner way of input parsing than a big if-else!
	for _, fn := range inputOrder { // list of input detection functions
		output, err = fn(s, fs)
		if err != nil {
			return nil, err
		}
		if output != nil { // activated!
			activated = true
			break
		}
	}
	if !activated {
		return nil, fmt.Errorf("input is invalid")
	}

	return output, nil
}

// absify makes a path absolute if it's not already.
func absify(str string) (string, error) {
	if filepath.IsAbs(str) {
		return str, nil // done early!
	}
	x, err := filepath.Abs(str)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't get abs path for: `%s`", str)
	}
	if strings.HasSuffix(str, "/") { // if we started with a trailing slash
		x = dirify(x) // add it back because filepath.Abs() removes it!
	}
	return x, nil // success, we're absolute now
}

// dirify ensures path ends with a trailing slash, so that it's a dir. Don't
// call this on something that's not a dir! It just appends a trailing slash if
// one isn't already present.
func dirify(str string) string {
	if !strings.HasSuffix(str, "/") {
		return str + "/"
	}
	return str
}

// inputEmpty is a simple empty string contents check.
// TODO: perhaps we could have a default action here to run from /etc/ or /var/?
func inputEmpty(s string, _ engine.Fs) (*ParsedInput, error) {
	if s == "" {
		return nil, fmt.Errorf("input is empty")
	}
	return nil, nil // pass (this test never succeeds)
}

// inputStdin checks if we're looking at stdin.
func inputStdin(s string, fs engine.Fs) (*ParsedInput, error) {
	if s != "-" {
		return nil, nil // not us, but no error
	}

	// TODO: stdin passthrough is not implemented (should it be?)
	// TODO: this reads everything into memory, which isn't very efficient!

	// FIXME: check if we have a contained OsFs or not.
	//if fs != OsFs { // XXX: https://github.com/spf13/afero/issues/188
	//	return nil, errwrap.Wrapf("can't use stdin for: `%s`", fs.Name())
	//}

	// TODO: can this cause a problem if stdin is too large?
	// TODO: yes, we could pass a reader directly, but we'd
	// need to have a convention for it to get closed after
	// and we need to save it to disk for deploys to use it
	b, err := ioutil.ReadAll(os.Stdin) // doesn't need fs
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read in stdin")
	}

	return inputCode(string(b), fs) // recurse
}

// inputMetadata checks to see if we have a metadata file path.
func inputMetadata(s string, fs engine.Fs) (*ParsedInput, error) {
	// we've got a metadata.yaml file
	if !strings.HasSuffix(s, "/"+interfaces.MetadataFilename) {
		return nil, nil // not us, but no error
	}
	var err error
	if s, err = absify(s); err != nil { // s is now absolute
		return nil, err
	}

	// does metadata file exist?
	f, err := fs.Open(s)
	if err != nil {
		return nil, errwrap.Wrapf(err, "file: `%s` does not exist", s)
	}

	// parse metadata file and save it to the fs
	metadata, err := interfaces.ParseMetadata(f)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not parse metadata file")
	}

	// base path on local system of the metadata file, with trailing slash
	basePath := dirify(filepath.Dir(s)) // absolute dir
	m := basePath + metadata.Main       // absolute file

	// does main.mcl file exist? open the file read-only...
	fm, err := fs.Open(m)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read from file: `%s`", m)
	}
	defer fm.Close()             // we're done reading by the time this runs
	b, err := ioutil.ReadAll(fm) // doesn't need fs
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read in file: `%s`", m)
	}

	// files that we saw
	files := []string{
		s, // the metadata.yaml input file
		m, // the main.mcl file
	}

	// real files/ directory
	if metadata.Files != "" { // TODO: nil pointer instead?
		filesDir := basePath + metadata.Files
		if _, err := fs.Stat(filesDir); err == nil {
			files = append(files, filesDir)
		}
	}

	// set this path since we know the location (it is used to find modules)
	if err := metadata.SetAbsSelfPath(basePath); err != nil { // set metadataPath
		return nil, errwrap.Wrapf(err, "could not build metadata")
	}
	return &ParsedInput{
		Base:     basePath,
		Main:     b,
		Files:    files,
		Metadata: metadata,
		// no Workers needed, this is the ideal input
	}, nil
}

// inputMcl checks if we have a path to a *.mcl file?
func inputMcl(s string, fs engine.Fs) (*ParsedInput, error) {
	// TODO: a regexp here would be better
	if !strings.HasSuffix(s, interfaces.DotFileNameExtension) {
		return nil, nil // not us, but no error
	}
	var err error
	if s, err = absify(s); err != nil { // s is now absolute
		return nil, err
	}
	// does *.mcl file exist? open the file read-only...
	fm, err := fs.Open(s)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read from file: `%s`", s)
	}
	defer fm.Close()             // we're done reading by the time this runs
	b, err := ioutil.ReadAll(fm) // doesn't need fs
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read in file: `%s`", s)
	}

	// build and save a metadata file to fs
	metadata := &interfaces.Metadata{
		//Main: interfaces.MainFilename, // TODO: use standard name?
		Main: filepath.Base(s), // use the name of the input
	}
	byt, err := metadata.ToBytes()
	if err != nil {
		// probably a programming error
		return nil, errwrap.Wrapf(err, "can't built metadata file")
	}
	dst := "/" + interfaces.MetadataFilename // eg: /metadata.yaml
	workers := []func(engine.Fs) error{
		func(fs engine.Fs) error {
			err := gapi.CopyBytesToFs(fs, byt, dst)
			return errwrap.Wrapf(err, "could not copy metadata file to fs")
		},
	}
	return &ParsedInput{
		Base: dirify(filepath.Dir(s)), // base path with trailing slash
		Main: b,
		Files: []string{
			s, // the input .mcl file
		},
		Metadata: metadata,
		Workers:  workers,
	}, nil
}

// inputDirectory checks if we're given the path to a directory.
func inputDirectory(s string, fs engine.Fs) (*ParsedInput, error) {
	if !strings.HasSuffix(s, "/") {
		return nil, nil // not us, but no error
	}
	var err error
	if s, err = absify(s); err != nil { // s is now absolute
		return nil, err
	}
	// does dir exist?
	fi, err := fs.Stat(s)
	if err != nil {
		return nil, errwrap.Wrapf(err, "dir: `%s` does not exist", s)
	}
	if !fi.IsDir() {
		return nil, errwrap.Wrapf(err, "dir: `%s` is not a dir", s)
	}

	// try looking for a metadata file in the root
	md := s + interfaces.MetadataFilename // absolute file
	if _, err := fs.Stat(md); err == nil {
		if x, err := inputMetadata(md, fs); err != nil { // recurse
			return nil, err
		} else if x != nil {
			return x, nil // recursed successfully!
		}
	}

	// try looking for a main.mcl file in the root
	mf := s + interfaces.MainFilename // absolute file
	if _, err := fs.Stat(mf); err == nil {
		if x, err := inputMcl(mf, fs); err != nil { // recurse
			return nil, err
		} else if x != nil {
			return x, nil // recursed successfully!
		}
	}

	// no other options left, didn't activate!
	return nil, nil
}

// inputCode checks if this is raw code? (last possibility, try and run it).
func inputCode(s string, fs engine.Fs) (*ParsedInput, error) {
	if len(s) == 0 {
		// handle empty strings in a single place by recursing
		if x, err := inputEmpty(s, fs); err != nil { // recurse
			return nil, err
		} else if x != nil {
			return x, nil // recursed successfully!
		}
	}

	// check if code is `metadata.yaml`, which is obviously not correct here
	if s == interfaces.MetadataFilename {
		return nil, fmt.Errorf("unexpected raw code '%s'. Did you mean './%s'?",
			interfaces.MetadataFilename, interfaces.MetadataFilename)
	}

	wd, err := os.Getwd() // NOTE: not meaningful for stdin unless fs is an OsFs
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't get working dir")
	}

	// since by the time we run this stdin will be gone, and
	// we want this to work with deploys, we need to fake it
	// by saving the data and adding a default metadata file
	// so that everything is built in a logical input state.
	metadata := &interfaces.Metadata{ // default metadata
		Main: interfaces.MainFilename,
	}
	byt, err := metadata.ToBytes() // build a metadata file
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not build metadata file")
	}

	dst1 := "/" + interfaces.MetadataFilename // eg: /metadata.yaml
	dst2 := "/" + metadata.Main               // eg: /main.mcl
	b := []byte(s)                            // unfortunately we convert things back and forth :/

	workers := []func(engine.Fs) error{
		func(fs engine.Fs) error {
			err := gapi.CopyBytesToFs(fs, byt, dst1)
			return errwrap.Wrapf(err, "could not copy metadata file to fs")
		},
		func(fs engine.Fs) error {
			err := gapi.CopyBytesToFs(fs, b, dst2)
			return errwrap.Wrapf(err, "could not copy main file to fs")
		},
	}

	return &ParsedInput{
		Base:     dirify(wd),
		Main:     b,
		Files:    []string{}, // they're already copied in
		Metadata: metadata,
		Workers:  workers,
	}, nil
}

// inputFail fails, because we couldn't activate anyone. We might not need this.
//func inputFail(s string, _ engine.Fs) (*ParsedInput, error) {
//	return nil, fmt.Errorf("input is invalid") // fail (this test always succeeds)
//}
