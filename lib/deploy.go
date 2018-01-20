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

package lib

import (
	"fmt"
	"log"
	"os"

	"github.com/purpleidea/mgmt/etcd"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/gapi"

	// these imports are so that GAPIs register themselves in init()
	_ "github.com/purpleidea/mgmt/hcl"
	_ "github.com/purpleidea/mgmt/lang"
	_ "github.com/purpleidea/mgmt/puppet"
	_ "github.com/purpleidea/mgmt/yamlgraph"
	_ "github.com/purpleidea/mgmt/yamlgraph2"

	"github.com/google/uuid"
	errwrap "github.com/pkg/errors"
	"github.com/urfave/cli"
	git "gopkg.in/src-d/go-git.v4"
)

const (
	// MetadataPrefix is the etcd prefix where all our fs superblocks live.
	MetadataPrefix = etcd.NS + "/fs"
	// StoragePrefix is the etcd prefix where all our fs data lives.
	StoragePrefix = etcd.NS + "/storage"
)

// deploy is the cli target to manage deploys to our cluster.
func deploy(c *cli.Context, name string, gapiObj gapi.GAPI) error {
	program, version := c.App.Name, c.App.Version
	var flags Flags
	if val, exists := c.App.Metadata["flags"]; exists {
		if f, ok := val.(Flags); ok {
			flags = f
		}
	}
	hello(program, version, flags) // say hello!

	var hash, pHash string
	if !c.GlobalBool("no-git") {
		wd, err := os.Getwd()
		if err != nil {
			return errwrap.Wrapf(err, "could not get current working directory")
		}
		repo, err := git.PlainOpen(wd)
		if err != nil {
			return errwrap.Wrapf(err, "could not open git repo")
		}

		head, err := repo.Head()
		if err != nil {
			return errwrap.Wrapf(err, "could not read git HEAD")
		}

		hash = head.Hash().String() // current commit id
		log.Printf("Deploy: Hash: %s", hash)

		lo := &git.LogOptions{
			From: head.Hash(),
		}
		commits, err := repo.Log(lo)
		if err != nil {
			return errwrap.Wrapf(err, "could not read git log")
		}
		if _, err := commits.Next(); err != nil { // skip over HEAD
			return errwrap.Wrapf(err, "could not read HEAD in git log") // weird!
		}
		commit, err := commits.Next()
		if err == nil { // errors are okay, we might be empty
			pHash = commit.Hash.String() // previous commit id
		}
		log.Printf("Deploy: Previous deploy hash: %s", pHash)
		if c.GlobalBool("force") {
			pHash = "" // don't check this :(
		}
		if hash == "" {
			return errwrap.Wrapf(err, "could not get git deploy hash")
		}
	}

	uniqueid := uuid.New() // panic's if it can't generate one :P

	etcdClient := &etcd.ClientEtcd{
		Seeds: c.GlobalStringSlice("seeds"), // endpoints
	}
	if err := etcdClient.Connect(); err != nil {
		return errwrap.Wrapf(err, "client connection error")
	}
	defer etcdClient.Destroy()

	// TODO: this was all implemented super inefficiently, fix up for perf!
	deploys, err := etcd.GetDeploys(etcdClient) // get previous deploys
	if err != nil {
		return errwrap.Wrapf(err, "error getting previous deploys")
	}
	// find the latest id
	var max uint64
	for i := range deploys {
		if i > max {
			max = i
		}
	}
	var id = max + 1 // next id
	log.Printf("Deploy: Previous deploy id: %d", max)

	etcdFs := &etcdfs.Fs{
		Client: etcdClient.GetClient(),
		// TODO: using a uuid is meant as a temporary measure, i hate them
		Metadata:   MetadataPrefix + fmt.Sprintf("/deploy/%d-%s", id, uniqueid),
		DataPrefix: StoragePrefix,
	}

	deploy, err := gapiObj.Cli(c, etcdFs)
	if err != nil {
		return errwrap.Wrapf(err, "cli parse error")
	}
	if deploy == nil { // not used
		return fmt.Errorf("not enough information specified")
	}

	// redundant
	deploy.Noop = c.GlobalBool("noop")
	deploy.Sema = c.GlobalInt("sema")

	str, err := deploy.ToB64()
	if err != nil {
		return errwrap.Wrapf(err, "encoding error")
	}

	// this nominally checks the previous git hash matches our expectation
	if err := etcd.AddDeploy(etcdClient, id, hash, pHash, &str); err != nil {
		return errwrap.Wrapf(err, "could not create deploy id `%d`", id)
	}
	log.Printf("Deploy: Success, id: %d", id)
	return nil
}
