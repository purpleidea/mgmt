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

package util

import (
	"bytes"
	"os"
	"path"
)

// SyncFs is the part of a filesystem that SyncDirToDisk reads from. Each
// engine.Fs has these methods.
// TODO: ReadDir has the old afero signature. Change it to the io/fs one, which
// returns []fs.DirEntry, when engine.Fs gets that one.
type SyncFs interface {
	// ReadDir reads the named directory and returns a list of its entries
	// sorted by filename.
	ReadDir(dirname string) ([]os.FileInfo, error)

	// ReadFile reads the named file and returns its contents.
	ReadFile(name string) ([]byte, error)
}

// SyncDirToDisk syncs the contents of the src dir on the srcFs into the dst dir
// on our local disk, `rsync --delete src/ dst/` style. The dst dir must already
// exist. Anything in dst which isn't in src gets removed, and a file that's
// already the same is left alone, since unnecessary churn would cause
// unnecessary events. Each file is read whole into memory, and it's written
// with os.WriteFile, which isn't atomic. Each file and dir we make gets the
// mode of its source, plus what the owner needs to change it in a later sync.
// The mode of one that already exists isn't changed.
// XXX: verify it is correct
func SyncDirToDisk(srcFs SyncFs, src, dst string) error {
	srcEntries, err := srcFs.ReadDir(src)
	if err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(srcEntries))
	for _, e := range srcEntries {
		keep[e.Name()] = struct{}{}
	}
	dstEntries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range dstEntries {
		if _, ok := keep[e.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(path.Join(dst, e.Name())); err != nil {
			return err
		}
	}

	for _, e := range srcEntries {
		srcChild := path.Join(src, e.Name())
		dstChild := path.Join(dst, e.Name())
		dstInfo, _ := os.Stat(dstChild)

		if e.IsDir() {
			if dstInfo != nil && !dstInfo.IsDir() {
				if err := os.Remove(dstChild); err != nil {
					return err
				}
			}
			// The owner must be able to write in each dir we make,
			// or we couldn't sync into it, and eg: each dir in an
			// embedded fs is 0555.
			if err := os.MkdirAll(dstChild, e.Mode()|0700); err != nil {
				return err
			}
			if err := SyncDirToDisk(srcFs, srcChild, dstChild); err != nil {
				return err
			}
			continue
		}

		if dstInfo != nil && dstInfo.IsDir() {
			if err := os.RemoveAll(dstChild); err != nil {
				return err
			}
			dstInfo = nil
		}
		data, err := srcFs.ReadFile(srcChild)
		if err != nil {
			return err
		}
		if dstInfo != nil {
			if existing, err := os.ReadFile(dstChild); err == nil && bytes.Equal(existing, data) {
				continue
			}
		}
		// The owner must be able to write each file we make, or we
		// couldn't change it in a later sync, and eg: each file in an
		// embedded fs is 0444.
		if err := os.WriteFile(dstChild, data, e.Mode()|0200); err != nil {
			return err
		}
	}
	return nil
}
