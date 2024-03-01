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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/etcd/deployer"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/gapi"
	emptyGAPI "github.com/purpleidea/mgmt/gapi/empty"
	langGAPI "github.com/purpleidea/mgmt/lang/gapi"
	"github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/util/errwrap"
	yamlGAPI "github.com/purpleidea/mgmt/yamlgraph"

	"github.com/pborman/uuid"
	git "gopkg.in/src-d/go-git.v4"
)

// DeployArgs is the CLI parsing structure and type of the parsed result. This
// particular one contains all the common flags for the `deploy` subcommand
// which all frontends can use.
type DeployArgs struct {
	Seeds []string `arg:"--seeds,env:MGMT_SEEDS" help:"default etc client endpoint"`
	Noop  bool     `arg:"--noop" help:"globally force all resources into no-op mode"`
	Sema  int      `arg:"--sema" default:"-1" help:"globally add a semaphore to all resources with this lock count"`
	NoGit bool     `arg:"--no-git" help:"don't look at git commit id for safe deploys"`
	Force bool     `arg:"--force" help:"force a new deploy, even if the safety chain would break"`

	DeployEmpty *emptyGAPI.Args `arg:"subcommand:empty" help:"deploy empty payload"`
	DeployLang  *langGAPI.Args  `arg:"subcommand:lang" help:"deploy lang (mcl) payload"`
	DeployYaml  *yamlGAPI.Args  `arg:"subcommand:yaml" help:"deploy yaml graph payload"`
}

// Run executes the correct subcommand. It errors if there's ever an error. It
// returns true if we did activate one of the subcommands. It returns false if
// we did not. This information is used so that the top-level parser can return
// usage or help information if no subcommand activates. This particular Run is
// the run for the main `deploy` subcommand. This always requires a frontend to
// deploy to the cluster, but if you don't want a graph, you can use the `empty`
// frontend. The engine backend is agnostic to which frontend is deployed, in
// fact, you can deploy with multiple different frontends, one after another, on
// the same engine.
func (obj *DeployArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	var name string
	var args interface{}
	if cmd := obj.DeployEmpty; cmd != nil {
		name = emptyGAPI.Name
		args = cmd
	}
	if cmd := obj.DeployLang; cmd != nil {
		name = langGAPI.Name
		args = cmd
	}
	if cmd := obj.DeployYaml; cmd != nil {
		name = yamlGAPI.Name
		args = cmd
	}

	// XXX: workaround https://github.com/alexflint/go-arg/issues/239
	if l := len(obj.Seeds); name == "" && l > 1 {
		elem := obj.Seeds[l-2] // second to last element
		if elem == emptyGAPI.Name || elem == langGAPI.Name || elem == yamlGAPI.Name {
			return false, cliUtil.CliParseError(cliUtil.MissingEquals) // consistent errors
		}
	}

	fn, exists := gapi.RegisteredGAPIs[name]
	if !exists {
		return false, nil // did not activate
	}
	gapiObj := fn()

	program, version := data.Program, data.Version
	Logf := func(format string, v ...interface{}) {
		log.Printf("deploy: "+format, v...)
	}

	// TODO: consider adding a timeout based on an args.Timeout flag ?
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	cliUtil.Hello(program, version, data.Flags) // say hello!
	defer Logf("goodbye!")

	var hash, pHash string
	if !obj.NoGit {
		wd, err := os.Getwd()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not get current working directory")
		}
		repo, err := git.PlainOpen(wd)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not open git repo")
		}

		head, err := repo.Head()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not read git HEAD")
		}

		hash = head.Hash().String() // current commit id
		Logf("hash: %s", hash)

		lo := &git.LogOptions{
			From: head.Hash(),
		}
		commits, err := repo.Log(lo)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not read git log")
		}
		if _, err := commits.Next(); err != nil { // skip over HEAD
			return false, errwrap.Wrapf(err, "could not read HEAD in git log") // weird!
		}
		commit, err := commits.Next()
		if err == nil { // errors are okay, we might be empty
			pHash = commit.Hash.String() // previous commit id
		}
		Logf("previous deploy hash: %s", pHash)
		if obj.Force {
			pHash = "" // don't check this :(
		}
		if hash == "" {
			return false, errwrap.Wrapf(err, "could not get git deploy hash")
		}
	}

	uniqueid := uuid.New() // panic's if it can't generate one :P

	etcdClient := client.NewClientFromSeedsNamespace(
		obj.Seeds, // endpoints
		lib.NS,
	)
	if err := etcdClient.Init(); err != nil {
		return false, errwrap.Wrapf(err, "client Init failed")
	}
	defer func() {
		err := errwrap.Wrapf(etcdClient.Close(), "client Close failed")
		if err != nil {
			// TODO: cause the final exit code to be non-zero
			Logf("client cleanup error: %+v", err)
		}
	}()

	simpleDeploy := &deployer.SimpleDeploy{
		Client: etcdClient,
		Debug:  data.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			Logf("deploy: "+format, v...)
		},
	}
	if err := simpleDeploy.Init(); err != nil {
		return false, errwrap.Wrapf(err, "deploy Init failed")
	}
	defer func() {
		err := errwrap.Wrapf(simpleDeploy.Close(), "deploy Close failed")
		if err != nil {
			// TODO: cause the final exit code to be non-zero
			Logf("deploy cleanup error: %+v", err)
		}
	}()

	// get max id (from all the previous deploys)
	max, err := simpleDeploy.GetMaxDeployID(ctx)
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting max deploy id")
	}
	// find the latest id
	var id = max + 1 // next id
	Logf("previous max deploy id: %d", max)

	etcdFs := &etcdfs.Fs{
		Client: etcdClient,
		// TODO: using a uuid is meant as a temporary measure, i hate them
		Metadata:   lib.MetadataPrefix + fmt.Sprintf("/deploy/%d-%s", id, uniqueid),
		DataPrefix: lib.StoragePrefix,

		Debug: data.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			Logf("fs: "+format, v...)
		},
	}

	info := &gapi.Info{
		Args: args,
		Flags: &gapi.Flags{
			Noop: obj.Noop,
			Sema: obj.Sema,
			//Update: obj.Update,
		},

		Fs:    etcdFs,
		Debug: data.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			log.Printf("cli: "+format, v...)
		},
	}

	deploy, err := gapiObj.Cli(info)
	if err != nil {
		return false, cliUtil.CliParseError(err) // consistent errors
	}
	if deploy == nil { // not used
		return false, fmt.Errorf("not enough information specified")
	}

	// redundant
	deploy.Noop = obj.Noop
	deploy.Sema = obj.Sema

	str, err := deploy.ToB64()
	if err != nil {
		return false, errwrap.Wrapf(err, "encoding error")
	}

	// this nominally checks the previous git hash matches our expectation
	if err := simpleDeploy.AddDeploy(ctx, id, hash, pHash, &str); err != nil {
		return false, errwrap.Wrapf(err, "could not create deploy id `%d`", id)
	}
	Logf("success, id: %d", id)
	return true, nil
}