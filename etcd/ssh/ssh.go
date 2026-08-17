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

// Package ssh transports etcd traffic over SSH to provide a special World API.
package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/sshutil"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

const (
	defaultUser                       = "root"
	defaultSSHPort             uint16 = 22
	defaultSSHHostKeyFieldName        = "hostkey" // querystring field name
	defaultEtcdPort            uint16 = 2379      // TODO: get this from etcd pkg
	allowRSA                          = true      // are big keys okay?

	// closeTimeout bounds how long we wait for the etcd client to close
	// during cleanup before we force the underlying ssh transport down to
	// unblock it. The grpc graceful close can otherwise block forever if
	// the connection is genuinely dead and can never drain. See
	// closeClient.
	closeTimeout = 60 * time.Second
)

// World is an implementation of the world API for etcd over SSH.
type World struct {
	// URL is the ssh server to connect to. Use the format, james@server:22
	// or similar. From there, we connect to each of the etcd Seeds, so the
	// ip's should be relative to this server. If you pass in a ?hostkey=
	// query string parameter, you can specify a base64, known_hosts key to
	// use for confirmation that you're connecting to the right host.
	// Without this, it will look in your ~/.ssh/known_hosts file which may
	// not necessarily exist yet, and without it connection is impossible.
	// You can find the key by running the ssh-keyscan command. It can also
	// be read from the HostKey parameter, which avoids you needing to
	// urlencode it here.
	URL string

	// HostKey is the key part (which is already base64 encoded) from a
	// known_hosts file, representing the host we're connecting to. If this
	// is specified, then it overrides looking for it in the URL.
	HostKey string

	// SSHID is the private key path for SSH client auth to URL. If empty,
	// Connect scans defaultSSHDir for id_* private keys. This will expand
	// the ~/ and ~user/ style path expansions.
	SSHID string

	// Seeds are the list of etcd endpoints to connect to.
	Seeds []string

	// NS is the etcd namespace to use.
	NS string

	MetadataPrefix string    // expected metadata prefix
	StoragePrefix  string    // storage prefix for etcdfs storage
	StandaloneFs   engine.Fs // store an fs here for local usage
	GetURI         func() string

	*etcd.World

	init *engine.WorldInit

	cleanups []func() error

	// sshMutex guards sshCleanups, which is mutated by the grpc dialer on
	// every (re)connect and read by closeSSH on shutdown.
	sshMutex    *sync.Mutex
	sshCleanups []func() error
}

// Connect runs first.
func (obj *World) Connect(ctx context.Context, init *engine.WorldInit) error {
	obj.init = init
	obj.cleanups = []func() error{}
	obj.sshMutex = &sync.Mutex{}
	obj.sshCleanups = nil

	helper := &sshutil.Helper{
		Debug: obj.init.Debug,
		Logf:  obj.init.Logf,
	}

	if len(obj.Seeds) == 0 {
		return fmt.Errorf("at least one seed is required")
	}
	seedSSH := make(map[string]string)
	for _, seed := range obj.Seeds {
		u, err := url.Parse(seed)
		if err != nil {
			return err
		}
		hostname := u.Hostname()
		if hostname == "" {
			return fmt.Errorf("empty hostname")
		}
		port := strconv.Itoa(int(defaultSSHPort)) // default
		if s := u.Port(); s != "" {
			port = s
		}
		addr := fmt.Sprintf("%s:%s", hostname, port)
		if s := u.Scheme; s != "http" && s != "https" {
			return fmt.Errorf("invalid scheme: %s", s)
		}
		seedSSH[seed] = addr // remove the scheme!
	}
	if l := len(obj.Seeds) - len(seedSSH); l != 0 {
		return fmt.Errorf("found %d duplicate tunnels", l)
	}

	// XXX: If we're using SSH, we should really have a list of SSH
	// endpoints, but a localhost identifier of one etcd jump from each...
	// Having the {N, M} set would be possibly too complicated. The point is
	// that the list of SSH endpoints is the useful thing when over SSH.
	s := obj.URL
	scheme := "ssh://"
	// the url.Parse function parses incorrectly without a scheme prefix :/
	if !strings.HasPrefix(s, scheme) {
		s = scheme + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	user := defaultUser
	if s := u.User.Username(); s != "" {
		user = s
	}
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("empty hostname")
	}
	port := strconv.Itoa(int(defaultSSHPort)) // default
	if s := u.Port(); s != "" {
		port = s
	}

	// TODO: Should we read out a list of these, one for each key type?
	base64Key := u.Query().Get(defaultSSHHostKeyFieldName) // urlencode me!
	if obj.HostKey != "" {                                 // override
		base64Key = obj.HostKey
	}
	var pubKey ssh.PublicKey // known hosts key
	if base64Key != "" {
		k, err := helper.KnownHostsKey(base64Key)
		if err != nil {
			return errwrap.Wrapf(err, "invalid known_hosts key")
		}
		pubKey = k
	}

	sshAddr := fmt.Sprintf("%s:%s", hostname, port)

	// Client authentication key types available...
	authKeyTypes := []string{
		//ssh.KeyAlgoED25519, // "ssh-ed25519"
		//ssh.KeyAlgoRSA,     // "ssh-rsa"
	}
	auths := []ssh.AuthMethod{}
	//auths = append(auths, ssh.Password("password")) // testing

	if obj.SSHID != "" {
		p, err := util.ExpandHome(obj.SSHID)
		if err != nil {
			return errwrap.Wrapf(err, "can't find home directory")
		}
		if p == "" {
			return fmt.Errorf("empty path specified")
		}

		signer, err := helper.KeySigner(p)
		if err != nil {
			return err
		}
		typ := signer.PublicKey().Type()
		authKeyTypes = append(authKeyTypes, typ)
		auths = append(auths, ssh.PublicKeys(signer)) // add one
	}

	if len(auths) == 0 {
		signers, err := helper.KeySigners()
		if err != nil {
			return err
		}
		for _, signer := range signers {
			typ := signer.PublicKey().Type()
			authKeyTypes = append(authKeyTypes, typ)
		}
		// TODO: should the order of the signers matter?
		if len(signers) > 0 {
			auths = append(auths, ssh.PublicKeys(signers...)) // add all
		}
	}

	if len(auths) == 0 {
		return fmt.Errorf("no auth options available")
	}

	obj.init.Logf("found %d available key types: %s", len(authKeyTypes), strings.Join(authKeyTypes, ", "))

	algorithms := ssh.SupportedAlgorithms()
	preferredAlgoOrder := algorithms.HostKeys // the defaults
	if allowRSA {
		preferredAlgoOrder = append(preferredAlgoOrder, ssh.KeyAlgoRSA)
	}

	// Learn which host key types we already trust for this host so that we
	// negotiate a key we can actually verify, the same way OpenSSH does.
	knownTypes, err := helper.KnownHostsAlgorithms(sshAddr, sshutil.DefaultKnownHostsPath)
	if err != nil {
		obj.init.Logf("known_hosts algorithms: %v", err)
	}
	hostKeyAlgorithms := helper.PrioritizeHostKeyAlgorithms(preferredAlgoOrder, pubKey, knownTypes...)
	obj.init.Logf("supported algos: %s", strings.Join(hostKeyAlgorithms, ", "))

	// SSH connection configuration
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: auths,
		//HostKeyCallback: ssh.InsecureIgnoreHostKey(), // testing
		HostKeyCallback: helper.HostKeyCallback(pubKey, sshutil.DefaultKnownHostsPath),

		// This is the list of host key algorithms that this SSH client
		// will offer to the SSH server when it says hello. This can be
		// different from what a normal terminal SSH client might do,
		// which means you might not get the right SSH host key algo
		// offered back to you, so make sure you provide what it's
		// asking for. Maybe we need to make this configurable by the
		// user.
		HostKeyAlgorithms: hostKeyAlgorithms,
	}

	// This runs repeatedly when etcd tries to reconnect.
	grpcWithContextDialerFunc := func(ctx context.Context, addr string) (net.Conn, error) {

		var reterr error
		for _, seed := range obj.Seeds { // first successful connect wins
			if addr != seedSSH[seed] {
				continue // not what we're expecting
			}

			// Cleanup previous connection...
			if err := obj.closeSSH(); err != nil {
				obj.init.Logf("error cleaning on reconnect: %+v", err)
			}

			obj.init.Logf("ssh: %s@%s", user, sshAddr)
			sshClient, err := sshutil.DialSSHWithContext(ctx, "tcp", sshAddr, sshConfig)
			if err != nil {
				reterr = err
				obj.init.Logf("ssh dial error: %v", err)
				continue
			}
			obj.addSSHCleanup(func() error {
				e := sshClient.Close()
				if obj.init.Debug && e != nil {
					obj.init.Logf("ssh client close error: %+v", e)
				}
				return e
			})

			obj.init.Logf("tunnel: %s", addr)
			tunnel, err := sshClient.Dial("tcp", addr)
			if err != nil {
				reterr = err
				obj.init.Logf("ssh tunnel error: %v", err)
				continue
			}

			obj.addSSHCleanup(func() error {
				e := tunnel.Close()
				if e == io.EOF { // XXX: why does this happen?
					return nil // ignore
				}
				if obj.init.Debug && e != nil {
					obj.init.Logf("ssh tunnel close error: %+v", e)
				}
				return e
			})

			obj.init.Logf("ssh tunnel connected")
			// We hand grpc a wrapped conn whose Close also closes
			// the ssh client. An ssh channel's Close does not
			// unblock a pending Read, which breaks the net.Conn
			// contract that grpc's transport shutdown relies on.
			// (See tunnelConn.)
			return &tunnelConn{Conn: tunnel, client: sshClient}, nil
		}

		if reterr != nil {
			return nil, reterr
		}
		return nil, fmt.Errorf("no ssh tunnels available") // TODO: better error message?
	}

	grpcConnectParams := grpc.ConnectParams{
		Backoff: backoff.DefaultConfig,
		//MinConnectTimeout: ???
	}
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: obj.Seeds,
		DialOptions: []grpc.DialOption{
			grpc.WithConnectParams(grpcConnectParams),
			grpc.WithContextDialer(grpcWithContextDialerFunc),
		},
	})
	if err != nil {
		return errwrap.Append(obj.cleanup(), err)
	}
	obj.cleanups = append(obj.cleanups, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		e := obj.closeClient(ctx, etcdClient)
		if obj.init.Debug && e != nil {
			obj.init.Logf("etcd client close error: %+v", e)
		}
		return e
	})

	c := client.NewClientFromNamespaceStr(etcdClient, obj.NS)

	obj.World = &etcd.World{
		// TODO: Pass through more data if the struct for etcd changes.
		Client:         c,
		MetadataPrefix: obj.MetadataPrefix,
		StoragePrefix:  obj.StoragePrefix,
		StandaloneFs:   obj.StandaloneFs,
		GetURI:         obj.GetURI,
	}
	if err := obj.World.Connect(ctx, init); err != nil {
		return errwrap.Append(obj.cleanup(), err)
	}
	obj.cleanups = append(obj.cleanups, func() error {
		e := obj.World.Cleanup()
		if obj.init.Debug && e != nil {
			obj.init.Logf("world close error: %+v", e)
		}
		return e
	})

	return nil
}

// closeClient closes the etcd client, bounded by ctx. The etcd and grpc Close
// methods are not context-aware and can block forever on a genuinely dead
// connection: the grpc reader is parked in a raw conn.Read that only a conn
// close (not a context cancellation) can interrupt. So if ctx is done before
// Close returns, we tear down the ssh transport ourselves to unblock the reader
// and then wait for Close to actually return. The close goroutine is always
// joined, never abandoned.
func (obj *World) closeClient(ctx context.Context, etcdClient *clientv3.Client) error {
	errch := make(chan error, 1) // buffered so the goroutine can't leak
	go func() {
		errch <- etcdClient.Close()
	}()
	select {
	case err := <-errch:
		return err // closed cleanly within the deadline
	case <-ctx.Done():
	}
	// Close is stuck; force the ssh transport down to unblock the grpc
	// reader, then wait for Close to actually return.
	if err := obj.closeSSH(); err != nil && obj.init.Debug {
		obj.init.Logf("ssh force close error: %+v", err)
	}
	return <-errch
}

// addSSHCleanup registers a "close" action for the current ssh connection.
func (obj *World) addSSHCleanup(fn func() error) {
	obj.sshMutex.Lock()
	defer obj.sshMutex.Unlock()
	obj.sshCleanups = append(obj.sshCleanups, fn)
}

// closeSSH tears down the current ssh client and tunnel, if any. It runs on
// reconnect (to drop the stale connection) and on shutdown (to unblock a stuck
// etcd client Close, see closeClient). It is safe to call more than once.
func (obj *World) closeSSH() error {
	obj.sshMutex.Lock()
	defer obj.sshMutex.Unlock()
	var errs error
	for i := len(obj.sshCleanups) - 1; i >= 0; i-- { // reverse
		if err := obj.sshCleanups[i](); err != nil {
			errs = errwrap.Append(errs, err)
		}
	}
	obj.sshCleanups = nil // clean
	return errs
}

// cleanup performs all the "close" actions either at the very end or as we go.
func (obj *World) cleanup() error {
	var errs error
	for i := len(obj.cleanups) - 1; i >= 0; i-- { // reverse
		f := obj.cleanups[i]
		if err := f(); err != nil {
			errs = errwrap.Append(errs, err)
		}
	}
	obj.cleanups = nil // clean
	return errs
}

// Cleanup runs last.
func (obj *World) Cleanup() error {
	return obj.cleanup()
}

// LocalEndpoints overrides the embedded etcd.World implementation, because the
// endpoints which our client uses are only reachable through our own SSH
// tunnel, and as a result they would be misleading to anyone who wants to make
// a direct connection to them, such as the remote resource tunnelling code.
func (obj *World) LocalEndpoints(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("local endpoints are tunnelled over ssh, they are not directly dialable")
}

// tunnelConn is the net.Conn that we hand to grpc for an ssh tunnel. It ties
// the tunnel channel and its dedicated ssh client together so that closing the
// conn also closes the client. This is necessary because an ssh channel's Close
// does not unblock a goroutine already blocked in the channel's Read, which
// violates the net.Conn contract. The grpc transport relies on that contract to
// shut down: its Close calls conn.Close and then waits, without a timeout, for
// its reader goroutine to exit. Without this, an etcd client Close over an ssh
// tunnel can block forever (e.g. on program shutdown), because the reader stays
// parked until the remote happens to send something. Closing the ssh client
// tears down the mux and unblocks all channel reads. Each tunnel has its own
// dedicated ssh client, so closing it here is safe.
type tunnelConn struct {
	net.Conn // the ssh tunnel channel

	client *ssh.Client // the ssh client backing this tunnel
}

// Close closes the ssh tunnel and then the ssh client that backs it.
func (obj *tunnelConn) Close() error {
	err := obj.Conn.Close()
	if err == io.EOF { // XXX: why does this happen?
		err = nil // ignore
	}
	if e := obj.client.Close(); e != nil && err == nil {
		err = e
	}
	return err
}
