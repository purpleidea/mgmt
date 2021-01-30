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

package integration

import (
	"context"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// SimpleDeployLang is a helper method that takes a struct and runs a sequence
// of methods on it. This particular helper starts up an instance, deploys some
// code, and then shuts down. Both after initially starting up, and after
// deploy, it waits for the instance to converge before running the next step.
func (obj *Instance) SimpleDeployLang(code string) error {
	if err := obj.Init(); err != nil {
		return errwrap.Wrapf(err, "could not init instance")
	}
	defer obj.Close() // clean up working directories

	// run the program
	if err := obj.Run(nil); err != nil {
		return errwrap.Wrapf(err, "mgmt could not start")
	}
	defer obj.Kill() // do a kill -9

	// wait for an internal converge signal as a baseline
	{
		ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		defer cancel()
		if err := obj.Wait(ctx); err != nil { // wait to get a converged signal
			return errwrap.Wrapf(err, "mgmt wait failed") // timeout expired
		}
	}

	// push a deploy
	if err := obj.DeployLang(code); err != nil {
		return errwrap.Wrapf(err, "mgmt could not deploy")
	}

	// wait for an internal converge signal
	{
		ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		defer cancel()
		if err := obj.Wait(ctx); err != nil { // wait to get a converged signal
			return errwrap.Wrapf(err, "mgmt wait failed") // timeout expired
		}
	}

	// press ^C
	{
		ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		defer cancel()
		if err := obj.Quit(ctx); err != nil {
			if err == context.DeadlineExceeded {
				return errwrap.Wrapf(err, "mgmt blocked on exit")
			}
			return errwrap.Wrapf(err, "mgmt exited with error")
		}
	}

	return nil
}

// SimpleDeployLang is a helper method that takes a struct representing a
// cluster and runs a sequence of methods on it. This particular helper starts
// up a series of instances linearly, deploys some code, and then shuts down.
// Both after initially starting up, after peering each instance, and after
// deploy, it waits for the instance to converge before running the next step.
func (obj *Cluster) SimpleDeployLang(code string) error {
	if err := obj.Init(); err != nil {
		return errwrap.Wrapf(err, "could not init instance")
	}
	defer obj.Close() // clean up working directories

	// start the cluster
	if err := obj.RunLinear(); err != nil {
		return errwrap.Wrapf(err, "mgmt could not start")
	}
	defer obj.Kill() // do a kill -9

	// wait for an internal converge signal as a baseline
	// FIXME: add this wait if we remove it from RunLinear
	//{
	//	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(longTimeout*len(obj.Hostnames))*time.Second)
	//	defer cancel()
	//	if err := obj.Wait(ctx); err != nil { // wait to get a converged signal
	//		return errwrap.Wrapf(err, "mgmt initial wait failed") // timeout expired
	//	}
	//}

	// push a deploy
	if err := obj.DeployLang(code); err != nil {
		return errwrap.Wrapf(err, "mgmt could not deploy")
	}

	// wait for an internal converge signal
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(longTimeout*len(obj.Hostnames))*time.Second)
		defer cancel()
		if err := obj.Wait(ctx); err != nil { // wait to get a converged signal
			return errwrap.Wrapf(err, "mgmt post-deploy wait failed") // timeout expired
		}
	}

	// press ^C
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(longTimeout*len(obj.Hostnames))*time.Second)
		defer cancel()
		if err := obj.Quit(ctx); err != nil {
			if err == context.DeadlineExceeded {
				return errwrap.Wrapf(err, "mgmt blocked on exit")
			}
			return errwrap.Wrapf(err, "mgmt exited with error")
		}
	}

	return nil
}
