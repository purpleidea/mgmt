// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package interfaces

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"

	"gopkg.in/yaml.v2"
)

const (
	// MetadataFilename is the filename for the metadata storage. This is
	// the ideal entry point for any running code.
	MetadataFilename = "metadata.yaml"

	// FileNameExtension is the filename extension used for languages files.
	FileNameExtension = "mcl" // alternate suggestions welcome!

	// DotFileNameExtension is the filename extension with a dot prefix.
	DotFileNameExtension = "." + FileNameExtension

	// MainFilename is the default filename for code to start running from.
	MainFilename = "main" + DotFileNameExtension

	// PathDirectory is the path directory name we search for modules in.
	PathDirectory = "path/"

	// FilesDirectory is the files directory name we include alongside
	// modules. It can store any useful files that we'd like.
	FilesDirectory = "files/"

	// ModuleDirectory is the default module directory name. It gets
	// appended to whatever the running prefix is or relative to the base
	// dir being used for deploys.
	ModuleDirectory = "modules/"
)

// Metadata is a data structure representing the module metadata. Since it can
// get moved around to different filesystems, it should only contain relative
// paths.
type Metadata struct {
	// Main is the path to the entry file where we start reading code.
	// Normally this is main.mcl or the value of the MainFilename constant.
	Main string `yaml:"main"`

	// Path is the relative path to the local module search path directory
	// that we should look in. This is similar to golang's vendor directory.
	// If a module wishes to include this directory, it's recommended that
	// it have the contained directory be a `git submodule` if possible.
	Path string `yaml:"path"`

	// Files is the location of the files/ directory which can contain some
	// useful additions that might get used in the modules. You can store
	// templates, or any other data that you'd like.
	// TODO: also allow storing files alongside the .mcl files in their dir!
	Files string `yaml:"files"`

	// License is the listed license of the module. Use the short names, eg:
	// LGPLv3+, or MIT.
	License string `yaml:"license"`

	// ParentPathBlock specifies whether we're allowed to search in parent
	// metadata file Path settings for modules. We always search in the
	// global path if we don't find others first. This setting defaults to
	// false, which is important because the downloader uses it to decide
	// where to put downloaded modules. It is similar to the equivalent of
	// a `require vendoring` flag in golang if such a thing existed. If a
	// module sets this to true, and specifies a Path value, then only that
	// path will be used as long as imports are present there. Otherwise it
	// will fall-back on the global modules directory. If a module sets this
	// to true, and does not specify a Path value, then the global modules
	// directory is automatically chosen for the import location for this
	// module. When this is set to true, in no scenario will an import come
	// from a directory other than the one specified here, or the global
	// modules directory. Module authors should use this sparingly when they
	// absolutely need a specific import vendored, otherwise they might
	// rouse the ire of module consumers. Keep in mind that you can specify
	// a Path directory, and include a git submodule in it, which will be
	// used by default, without specifying this option. In that scenario,
	// the consumer can decide to not recursively clone your submodule if
	// they wish to override it higher up in the module search locations.
	ParentPathBlock bool `yaml:"parentpathblock"`

	// Metadata stores a link to the parent metadata structure if it exists.
	Metadata *Metadata // this does *NOT* get a yaml struct tag

	// metadataPath stores the absolute path to this metadata file as it is
	// parsed. This is useful when we search upwards for parent Path values.
	metadataPath string // absolute path that this file was found in

	// TODO: is this needed anymore?
	defaultMain *string // set this to pick a default Main when decoding

	// bug395 is a flag to workaround the terrible yaml parser resetting all
	// the default struct field values when it finds an empty yaml document.
	// We set this value to have a default of true, which enables us to know
	// if the document was empty or not, and if so, then we know this struct
	// was emptied, so we should then return a new struct with all defaults.
	// See: https://github.com/go-yaml/yaml/issues/395 for more information.
	bug395 bool
}

// DefaultMetadata returns the default metadata that is used for absent values.
func DefaultMetadata() *Metadata {
	return &Metadata{ // the defaults
		Main: MainFilename, // main.mcl
		// This MUST be empty for a top-level default, because if it's
		// not, then an undefined Path dir at a lower level won't search
		// upwards to find a suitable path, and we'll nest forever...
		//Path: PathDirectory, // do NOT set this!
		Files: FilesDirectory, // files/
		//License: "", // TODO: ???

		bug395: true, // workaround, lol
	}
}

// SetAbsSelfPath sets the absolute directory path to this metadata file. This
// method is used on a built metadata file so that it can internally know where
// it is located.
func (obj *Metadata) SetAbsSelfPath(p string) error {
	obj.metadataPath = p
	return nil
}

// ToBytes marshals the struct into a byte array and returns it.
func (obj *Metadata) ToBytes() ([]byte, error) {
	return yaml.Marshal(obj) // TODO: obj or *obj ?
}

// NOTE: this is not currently needed, but here for reference.
//// MarshalYAML modifies the struct before it is used to build the raw output.
//func (obj *Metadata) MarshalYAML() (interface{}, error) {
//	// The Marshaler interface may be implemented by types to customize
//	// their behavior when being marshaled into a YAML document. The
//	// returned value is marshaled in place of the original value
//	// implementing Marshaler.
//
//	if obj.metadataPath == "" { // make sure metadataPath isn't saved!
//		return obj, nil
//	}
//	md := obj.Copy() // TODO: implement me
//	md.metadataPath = "" // if set, blank it out before save
//	return md, nil
//}

// UnmarshalYAML is the standard unmarshal method for this struct.
func (obj *Metadata) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type indirect Metadata // indirection to avoid infinite recursion
	def := DefaultMetadata()
	// support overriding
	if x := obj.defaultMain; x != nil {
		def.Main = *x
	}

	raw := indirect(*def) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = Metadata(raw) // restore from indirection with type conversion!
	return nil
}

// ParseMetadata reads from some input and returns a *Metadata struct that
// contains plausible values to be used.
func ParseMetadata(reader io.Reader) (*Metadata, error) {
	metadata := DefaultMetadata() // populate this
	//main := MainFilename // set a custom default here if you want
	//metadata.defaultMain = &main

	// does not work in all cases :/ (fails with EOF files, ioutil does not)
	//decoder := yaml.NewDecoder(reader)
	////decoder.SetStrict(true) // TODO: consider being strict?
	//if err := decoder.Decode(metadata); err != nil {
	//	return nil, errwrap.Wrapf(err, "can't parse metadata")
	//}
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read metadata")
	}
	if err := yaml.Unmarshal(b, metadata); err != nil {
		return nil, errwrap.Wrapf(err, "can't parse metadata")
	}

	if !metadata.bug395 { // workaround, lol
		// we must have gotten an empty document, so use a new default!
		metadata = DefaultMetadata()
	}

	// FIXME: search for unclean paths containing ../ or similar and error!

	if strings.HasPrefix(metadata.Main, "/") || strings.HasSuffix(metadata.Main, "/") {
		return nil, fmt.Errorf("the Main field must be a relative file path")
	}
	if metadata.Path != "" && (strings.HasPrefix(metadata.Path, "/") || !strings.HasSuffix(metadata.Path, "/")) {
		return nil, fmt.Errorf("the Path field must be undefined or be a relative dir path")
	}
	if metadata.Files != "" && (strings.HasPrefix(metadata.Files, "/") || !strings.HasSuffix(metadata.Files, "/")) {
		return nil, fmt.Errorf("the Files field must be undefined or be a relative dir path")
	}
	// TODO: add more validation

	return metadata, nil
}

// FindModulesPath returns an absolute path to the Path dir where modules can be
// found. This can vary, because the current metadata file might not specify a
// Path value, meaning we'd have to return the global modules path.
// Additionally, we can search upwards for a path if our metadata file allows
// this. It searches with respect to the calling base directory, and uses the
// ParentPathBlock field to determine if we're allowed to search upwards. It
// does logically without doing any filesystem operations.
func FindModulesPath(metadata *Metadata, base, modules string) (string, error) {
	ret := func(s string) (string, error) { // return helper function
		// don't return an empty string without an error!!!
		if s == "" {
			return "", fmt.Errorf("can't find a module path")
		}
		return s, nil
	}
	m := metadata // start
	b := base     // absolute base path current metadata file is in
	for m != nil {
		if m.metadataPath == "" { // a top-level module might be empty!
			return ret(modules) // so return this, there's nothing else!
		}
		if m.metadataPath != b { // these should be the same if no bugs!
			return "", fmt.Errorf("metadata inconsistency: `%s` != `%s`", m.metadataPath, b)
		}

		// does metadata specify where to look ?
		// search in the module specific space
		if m.Path != "" { // use this path, since it was specified!
			if !strings.HasSuffix(m.Path, "/") {
				return "", fmt.Errorf("metadata inconsistency: path `%s` has no trailing slash", m.Path)
			}
			return ret(b + m.Path) // join w/o cleaning trailing slash
		}

		// are we allowed to search incrementally upwards?
		if m.ParentPathBlock {
			break
		}

		// search upwards (search in parent dirs upwards recursively...)
		m = m.Metadata // might be nil
		if m != nil {
			b = m.metadataPath // get new parent path
		}
	}
	// by now we haven't found a metadata path, so we use the global path...
	return ret(modules) // often comes from an ENV or a default
}

// FindModulesPathList does what FindModulesPath does, except this function
// returns the entirely linear string of possible module locations until it gets
// to the root. This can be useful if you'd like to know which possible
// locations are valid, so that you can search through them to see if there is
// downloaded code available.
func FindModulesPathList(metadata *Metadata, base, modules string) ([]string, error) {
	found := []string{}
	ret := func(s []string) ([]string, error) { // return helper function
		// don't return an empty list without an error!!!
		if s == nil || len(s) == 0 {
			return nil, fmt.Errorf("can't find any module paths")
		}
		return s, nil
	}
	m := metadata // start
	b := base     // absolute base path current metadata file is in
	for m != nil {
		if m.metadataPath == "" { // a top-level module might be empty!
			return ret([]string{modules}) // so return this, there's nothing else!
		}
		if m.metadataPath != b { // these should be the same if no bugs!
			return nil, fmt.Errorf("metadata inconsistency: `%s` != `%s`", m.metadataPath, b)
		}

		// does metadata specify where to look ?
		// search in the module specific space
		if m.Path != "" { // use this path, since it was specified!
			if !strings.HasSuffix(m.Path, "/") {
				return nil, fmt.Errorf("metadata inconsistency: path `%s` has no trailing slash", m.Path)
			}
			p := b + m.Path          // join w/o cleaning trailing slash
			found = append(found, p) // add to list
		}

		// are we allowed to search incrementally upwards?
		if m.ParentPathBlock {
			break
		}

		// search upwards (search in parent dirs upwards recursively...)
		m = m.Metadata // might be nil
		if m != nil {
			b = m.metadataPath // get new parent path
		}
	}
	// add the global path to everything we've found...
	found = append(found, modules) // often comes from an ENV or a default
	return ret(found)
}
