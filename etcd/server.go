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

package etcd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	etcdUtil "github.com/purpleidea/mgmt/etcd/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcdTransport "go.etcd.io/etcd/client/pkg/v3/transport"
	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/server/v3/embed"
)

const (
	// MaxServerStartTimeout is the amount of time to wait for the server
	// to start before considering it a failure. If you hit this timeout,
	// let us know so that we can analyze the situation, and increase this
	// if necessary.
	MaxServerStartTimeout = 60 * time.Second

	// MaxServerCloseTimeout is the maximum amount of time we'll wait for
	// the server to close down. If it exceeds this, it's probably a bug.
	MaxServerCloseTimeout = 15 * time.Second

	// MaxServerRetries is the maximum number of times we can try to restart
	// the server if it fails on startup. This can help workaround some
	// timing bugs in etcd.
	MaxServerRetries = 5

	// ServerRetryWait is the amount of time to wait between retries.
	ServerRetryWait = 500 * time.Millisecond
)

// runServer kicks of a new embedded etcd server. It exits when the server shuts
// down. The exit can be triggered at any time by cancelling the ctx or if it
// exits due to some condition like an error.
func (obj *EmbdEtcd) runServer(ctx context.Context, newCluster bool, peerURLsMap etcdtypes.URLsMap) (reterr error) {
	// XXX: use a mutex here?
	obj.Logf("server: runServer: (newCluster=%t): %+v", newCluster, peerURLsMap)
	defer obj.Logf("server: runServer: done!")
	defer obj.serverExitsSignal.Send()
	dataDir := fmt.Sprintf("%s/", path.Join(obj.Prefix, "server"))
	if err := os.MkdirAll(dataDir, 0770); err != nil {
		return errwrap.Wrapf(err, "couldn't mkdir: %s", dataDir)
	}

	memberName := obj.Hostname

	// if no peer URLs exist, then starting a server is mostly only for some
	// testing, but etcd doesn't allow the value to be empty so we use this!
	peerURLs, err := etcdtypes.NewURLs([]string{"http://localhost:0"})
	if err != nil {
		return errwrap.Wrapf(err, "invalid URLs")
	}
	if len(obj.ServerURLs) > 0 {
		peerURLs = obj.ServerURLs
	}
	initialPeerURLsMap, err := etcdUtil.CopyURLsMap(peerURLsMap)
	if err != nil {
		return errwrap.Wrapf(err, "error copying URLsMap")
	}
	// add self to list if it's not already in there...
	if _, exists := peerURLsMap[memberName]; !exists {
		initialPeerURLsMap[memberName] = peerURLs
	}

	// TODO: do we need to copy?
	aPUrls := peerURLs
	if len(obj.AServerURLs) > 0 {
		aPUrls = obj.AServerURLs
	}
	// NOTE: this logic is similar to obj.curls()
	aCUrls := obj.ClientURLs
	if len(obj.AClientURLs) > 0 {
		aCUrls = obj.AClientURLs
	}

	// embed etcd
	cfg := embed.NewConfig()
	cfg.Name = memberName // hostname
	cfg.Dir = dataDir
	cfg.ListenPeerUrls = peerURLs
	cfg.ListenClientUrls = obj.ClientURLs
	cfg.AdvertisePeerUrls = aPUrls
	cfg.AdvertiseClientUrls = aCUrls
	cfg.StrictReconfigCheck = false // XXX: workaround https://github.com/etcd-io/etcd/issues/6305
	cfg.MaxTxnOps = DefaultMaxTxnOps
	cfg.Logger = "zap"
	//cfg.LogOutputs = []string{} // FIXME: add a way to pass in our logf func
	cfg.LogLevel = "error" // keep things quieter for now

	cfg.SocketOpts = etcdTransport.SocketOpts{
		//ReusePort: false, // SO_REUSEPORT
		ReuseAddress: true, // SO_REUSEADDR
	}

	cfg.InitialCluster = initialPeerURLsMap.String() // including myself!
	if newCluster {
		cfg.ClusterState = embed.ClusterStateFlagNew
	} else {
		cfg.ClusterState = embed.ClusterStateFlagExisting
	}
	//cfg.ForceNewCluster = newCluster // TODO: ?

	if err := cfg.Validate(); err != nil {
		return errwrap.Wrapf(err, "server config is invalid")
	}

	obj.Logf("server: starting...")
	// TODO: etcd panics with: `create wal error: no space left on device`
	// see: https://github.com/etcd-io/etcd/issues/10588
	defer func() {
		if r := recover(); r != nil { // magic panic catcher
			obj.Logf("server: panic: %s", r)
			reterr = fmt.Errorf("panic during start with: %s", r) // set named return err
		}
	}()
	// XXX: workaround: https://github.com/etcd-io/etcd/issues/10626
	// This runs when we see the nominate operation. This could also error
	// if this races to start up, and happens before the member add runs.
	count := 0
	for {
		obj.server, err = embed.StartEtcd(cfg)
		if err == nil {
			break
		}
		e := err.Error()
		// catch: error validating peerURLs ... member count is unequal
		if strings.HasPrefix(e, "error validating peerURLs") && strings.HasSuffix(e, "member count is unequal") {
			count++
			if count > MaxServerRetries {
				err = errwrap.Wrapf(err, "workaround retries (%d) exceeded", MaxServerRetries)
				break
			}
			obj.Logf("waiting %s for retry", ServerRetryWait.String())
			time.Sleep(ServerRetryWait)
			continue
		}
		break
	}
	defer func() {
		obj.server = nil // important because this is used as an isRunning flag
	}()
	if err != nil {
		// early debug logs in case something downstream blocks
		if obj.Debug {
			obj.Logf("server failing with: %+v", err)
		}
		return errwrap.Wrapf(err, "server start failed")
	}

	closedChan := make(chan struct{})
	defer func() {
		select {
		case <-time.After(MaxServerCloseTimeout):
			obj.Logf("server: close timeout of %s reached", MaxServerCloseTimeout.String())
		case <-closedChan:
		}
	}()
	defer func() {
		// no wg here, since we want to let it die on exit if need be...
		// XXX: workaround: https://github.com/etcd-io/etcd/issues/10600
		go func() {
			obj.server.Close() // this blocks until server has stopped
			close(closedChan)  // woo!
		}()
	}()
	defer obj.server.Server.Stop() // trigger a shutdown

	select {
	case <-obj.server.Server.ReadyNotify(): // we hang here if things are bad
		obj.Logf("server: ready") // it didn't hang!

	// TODO: should we wait for this notification elsewhere?
	case <-obj.server.Server.StopNotify(): // it's going down now...
		err := fmt.Errorf("received stop notification")
		obj.Logf("server: stopped: %v", err)
		return err

	case <-time.After(MaxServerStartTimeout):
		err := fmt.Errorf("start timeout of %s reached", MaxServerStartTimeout.String())
		obj.Logf("server: %v", err)
		return err
	}

	obj.serverID = uint64(obj.server.Server.ID()) // store member id for internal use
	defer func() {
		obj.serverID = 0 // reset
	}()
	//obj.addSelfState() // add to endpoints list so self client can connect!
	//obj.setEndpoints() // sync client with new endpoints
	//defer obj.setEndpoints()
	//defer obj.rmMemberState(obj.Hostname)

	obj.serverReadySignal.Send() // send a signal, and then reset the signal

	for {
		select {
		case err, ok := <-obj.server.Err():
			if !ok { // server shut down
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "server error")
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	//return nil // unreachable
}

// ServerReady returns a channel that closes when we're up and running. This
// process happens when calling runServer. If runServer is never called, this
// will never happen. It also returns a cancel/ack function which must be called
// once the signal is received or we are done watching it. This is because this
// is a cyclical signal which happens, and then gets reset as the server starts
// up, shuts down, and repeats the cycle. The cancel/ack function ensures that
// we only watch a signal when it's ready to be read, and only reset it when we
// are done watching it.
func (obj *EmbdEtcd) ServerReady() (<-chan struct{}, func()) {
	return obj.serverReadySignal.Subscribe()
}

// ServerExited returns a channel that closes when the server is destroyed. This
// process happens after runServer exits. If runServer is never called, this
// will never happen. It also returns a cancel/ack function which must be called
// once the signal is received or we are done watching it. This is because this
// is a cyclical signal which happens, and then gets reset as the server starts
// up, shuts down, and repeats the cycle. The cancel/ack function ensures that
// we only watch a signal when it's ready to be read, and only reset it when we
// are done watching it.
func (obj *EmbdEtcd) ServerExited() (<-chan struct{}, func()) {
	return obj.serverExitsSignal.Subscribe()
}
