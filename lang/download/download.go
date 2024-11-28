// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

// Package download is used for downloading language modules from git.
package download

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	git "github.com/go-git/go-git/v5"
)

// Downloader implements the Downloader interface. It provides a mechanism to
// pull down new code from the internet. This is usually done with git.
type Downloader struct {
	info *interfaces.DownloadInfo

	// Depth is the max recursion depth that we should descent to. A
	// negative value means infinite. This is usually the default.
	Depth int

	// Retry is the max number of retries we should run if we encounter a
	// network error. A negative value means infinite. The default is
	// usually zero.
	Retry int

	// TODO: add a retry backoff parameter
}

// Init initializes the downloader with some core structures we'll need.
func (obj *Downloader) Init(info *interfaces.DownloadInfo) error {
	obj.info = info
	return nil
}

// Get runs a single download of an import and stores it on disk.
// XXX: this should only touch the filesystem via obj.info.Fs, but that is not
// implemented at the moment, so we cheat and use the local fs directly. This is
// not disastrous, since we only run Get on a local fs, since we don't download
// to etcdfs directly with the downloader during a deploy. This is because we'd
// need to implement the afero.Fs -> billy.Filesystem mapping layer.
func (obj *Downloader) Get(info *interfaces.ImportData, modulesPath string) error {
	if info == nil {
		return fmt.Errorf("empty import information")
	}
	if info.URL == "" {
		return fmt.Errorf("can't clone from empty URL")
	}
	if modulesPath == "" || !strings.HasSuffix(modulesPath, "/") || !strings.HasPrefix(modulesPath, "/") {
		return fmt.Errorf("module path (`%s`) (must be an absolute dir)", modulesPath)
	}
	if stat, err := obj.info.Fs.Stat(modulesPath); err != nil || !stat.IsDir() {
		if err == nil {
			return fmt.Errorf("module path (`%s`) must be a dir", modulesPath)
		}
		if os.IsNotExist(err) {
			return fmt.Errorf("module path (`%s`) must exist", modulesPath)
		}
		return errwrap.Wrapf(err, "could not read module path (`%s`)", modulesPath)
	}

	if info.IsSystem || info.IsLocal {
		// NOTE: this doesn't prevent us from downloading from a remote
		// git repo that is actually a .git file path instead of HTTP...
		return fmt.Errorf("can only download remote repos")
	}
	// TODO: error early if we're provided *ImportData that we can't act on

	pull := false
	dir := modulesPath + info.Path // TODO: is this dir unique?
	isBare := false
	options := &git.CloneOptions{
		URL: info.URL,
		// TODO: do we want to add an option for infinite recursion here?
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		Progress:          os.Stdout,
	}

	msg := fmt.Sprintf("downloading `%s` to: `%s`", info.URL, dir)
	if obj.info.Noop {
		msg = "(noop) " + msg // add prefix
	}
	obj.info.Logf(msg)
	if obj.info.Debug {
		obj.info.Logf("info: `%+v`", info)
		obj.info.Logf("options: `%+v`", options)
	}
	if obj.info.Noop {
		return nil // done early
	}
	// FIXME: replace with:
	// `git.Clone(s storage.Storer, worktree billy.Filesystem, o *CloneOptions)`
	// that uses an `fs engine.Fs` wrapped to the git Filesystem interface:
	// `billyFs := desfacer.New(obj.info.Fs)`
	// TODO: repo, err := git.Clone(??? storage.Storer, billyFs, options)
	gitDir := path.Clean(dir)
	obj.info.Logf("cloning...")
	repo, err := git.PlainCloneContext(context.TODO(), gitDir, isBare, options)
	if err == git.ErrRepositoryAlreadyExists {
		repo, err = git.PlainOpen(gitDir)
		if err != nil {
			return errwrap.Wrapf(err, "can't open existing repo at: `%s`", dir)
		}
		obj.info.Logf("repo already exists!")

		if obj.info.Update {
			pull = true // make sure to pull latest...
		}
	} else if err != nil {
		return errwrap.Wrapf(err, "can't clone repo: `%s` to: `%s`", info.URL, dir)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return errwrap.Wrapf(err, "can't get working tree: `%s`", dir)
	}
	if worktree == nil {
		// FIXME: not sure how we're supposed to handle this scenario...
		return errwrap.Wrapf(err, "can't work with nil work tree for: `%s`", dir)
	}

	// TODO: do we need to checkout master first, before pulling?
	if pull {
		options := &git.PullOptions{
			// TODO: do we want to add an option for infinite recursion here?
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
			Progress:          os.Stdout,
		}
		obj.info.Logf("pulling...")
		err := worktree.PullContext(context.TODO(), options)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errwrap.Wrapf(err, "can't pull latest from: `%s`", info.URL)
		}
		if err == git.NoErrAlreadyUpToDate {
			obj.info.Logf("repo already up to date!")
		}
	}

	// TODO: checkout requested sha1/tag if one was specified...
	// if err := worktree.Checkout(opts *CheckoutOptions)

	// does the repo have a metadata file present? (we'll validate it later)
	if _, err := obj.info.Fs.Stat(dir + interfaces.MetadataFilename); err != nil {
		return errwrap.Wrapf(err, "could not read repo metadata file `%s` in its root", interfaces.MetadataFilename)
	}

	return nil
}
