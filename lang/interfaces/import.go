// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/engine"
)

// ImportData is the result of parsing a string import when it has not errored.
type ImportData struct {
	// Name is the original input that produced this struct. It is stored
	// here so that you can parse it once and pass this struct around
	// without having to include a copy of the original data if needed.
	Name string

	// Alias is the name identifier that should be used for this import.
	Alias string

	// IsSystem specifies that this is a system import.
	IsSystem bool

	// IsLocal represents if a module is either local or a remote import.
	IsLocal bool

	// IsFile represents if we're referring to an individual file or not.
	IsFile bool

	// Path represents the relative path to the directory that this import
	// points to. Since it specifies a directory, it will end with a
	// trailing slash which makes detection more obvious for other helpers.
	// If this points to a local import, that directory is probably not
	// expected to contain a metadata file, and it will be a simple path
	// addition relative to the current file this import was parsed from. If
	// this is a remote import, then it's likely that the file will be found
	// in a more distinct path, such as a search path that contains the full
	// fqdn of the import.
	// TODO: should system imports put something here?
	Path string

	// URL is the path that a `git clone` operation should use as the URL.
	// If it is a local import, then this is the empty value.
	URL string
}

// DownloadInfo is the set of input values passed into the Init method of the
// Downloader interface, so that it can have some useful information to use.
type DownloadInfo struct {
	// Fs is the filesystem to use for downloading to.
	Fs engine.Fs

	// Noop specifies if we should actually download or just fake it. The
	// one problem is that if we *don't* download something, then we can't
	// follow it to see if there's anything else to download.
	Noop bool

	// Sema specifies the max number of simultaneous downloads to run.
	Sema int

	// Update specifies if we should try and update existing downloaded
	// artifacts.
	Update bool

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Downloader is the interface that must be fulfilled to download modules.
// TODO: this should probably be in a more central package like the top-level
// GAPI package, and not contain the lang specific *ImportData struct. Since we
// aren't working on a downloader for any other frontend at the moment, we'll
// keep it here, and keep it less generalized for now. If we *really* wanted to
// generalize it, Get would be implemented as part of the *ImportData struct and
// there would be an interface it helped fulfill for the Downloader GAPI.
type Downloader interface {
	// Init initializes the downloader with some core structures we'll need.
	Init(*DownloadInfo) error

	// Get runs a single download of an import and stores it on disk.
	Get(*ImportData, string) error
}
