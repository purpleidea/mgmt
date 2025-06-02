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
	"os"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"google.golang.org/grpc"
)

const (
	defaultUser                  = "root"
	defaultSSHPort        uint16 = 22
	defaultEtcdPort       uint16 = 2379 // TODO: get this from etcd pkg
	defaultIDRsaPath             = "~/.ssh/id_rsa"
	defaultIDEd25519Path         = "~/.ssh/id_ed25519"
	defaultKnownHostsPath        = "~/.ssh/known_hosts"
)

// World is an implementation of the world API for etcd over SSH.
type World struct {
	// URL is the ssh server to connect to. Use the format, james@server:22
	// or similar. From there, we connect to each of the etcd Seeds, so the
	// ip's should be relative to this server.
	URL string

	// SSHID is the path to the ~/.ssh/id_rsa or ~/.ssh/id_ed25519 to use
	// for auth. If you omit this then this will look for your private key
	// in both of those default paths. If you specific a specific path, then
	// that will only be used. This will expand the ~/ and ~user/ style path
	// expansions.
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

	sshClient *ssh.Client

	cleanups []func() error
}

// sshKeyAuth is a helper function to get the ssh key auth struct needed.
func (obj *World) sshKeyAuth(sshID string) (ssh.AuthMethod, error) {
	if sshID == "" {
		return nil, fmt.Errorf("empty path specified")
	}

	// expand strings of the form: ~james/.ssh/id_rsa
	p, err := util.ExpandHome(sshID)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't find home directory")
	}
	if p == "" {
		return nil, fmt.Errorf("empty path specified")
	}
	// A public key may be used to authenticate against the server by using
	// an unencrypted PEM-encoded private key file. If you have an encrypted
	// private key, the crypto/x509 package can be used to decrypt it.
	key, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	// create the Signer for this private key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

// hostKeyCallback is a helper function to get the ssh callback function needed.
func (obj *World) hostKeyCallback() (ssh.HostKeyCallback, error) {
	// TODO: consider allowing a user-specified path in the future
	s := defaultKnownHostsPath // "~/.ssh/known_hosts"

	// expand strings of the form: ~james/.ssh/known_hosts
	p, err := util.ExpandHome(s)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't find home directory")
	}
	if p == "" {
		return nil, fmt.Errorf("empty path specified")
	}

	return knownhosts.New(p)
}

// Connect runs first.
func (obj *World) Connect(ctx context.Context, init *engine.WorldInit) error {
	obj.init = init
	obj.cleanups = []func() error{}

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

	addr := fmt.Sprintf("%s:%s", hostname, port)

	auths := []ssh.AuthMethod{}
	//auths = append(auths, ssh.Password("password")) // testing
	choices := []string{
		//obj.SSHID,
		defaultIDEd25519Path,
		defaultIDRsaPath, // "~/.ssh/id_rsa"
	}
	if obj.SSHID != "" {
		choices = []string{
			obj.SSHID,
		}
	}
	for _, p := range choices {
		if p == "" {
			continue
		}
		auth, err := obj.sshKeyAuth(p)
		if err != nil {
			//obj.init.Logf("can't get auth from: %s", p) // misleading
			continue
		}
		obj.init.Logf("found auth option in: %s", p)
		auths = append(auths, auth)
	}
	if len(auths) == 0 {
		return fmt.Errorf("no auth options available")
	}

	hostKeyCallback, err := obj.hostKeyCallback()
	if err != nil {
		return err
	}

	// SSH connection configuration
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: auths,
		//HostKeyCallback: ssh.InsecureIgnoreHostKey(), // testing
		HostKeyCallback: hostKeyCallback,
	}

	obj.init.Logf("ssh: %s@%s", user, addr)
	obj.sshClient, err = dialSSHWithContext(ctx, "tcp", addr, sshConfig)
	if err != nil {
		return err
	}
	obj.cleanups = append(obj.cleanups, func() error {
		e := obj.sshClient.Close()
		if obj.init.Debug && e != nil {
			obj.init.Logf("ssh client close error: %+v", e)
		}
		return e
	})

	// This runs repeatedly when etcd tries to reconnect.
	grpcWithContextDialerFunc := func(ctx context.Context, addr string) (net.Conn, error) {
		var reterr error
		for _, seed := range obj.Seeds { // first successful connect wins
			if addr != seedSSH[seed] {
				continue // not what we're expecting
			}
			obj.init.Logf("tunnel: %s", addr)

			tunnel, err := obj.sshClient.Dial("tcp", addr)
			if err != nil {
				reterr = err
				obj.init.Logf("ssh dial error: %v", err)
				continue
			}

			// TODO: do we need a mutex around adding these?
			obj.cleanups = append(obj.cleanups, func() error {
				e := tunnel.Close()
				if e == io.EOF { // XXX: why does this happen?
					return nil // ignore
				}
				if obj.init.Debug && e != nil {
					obj.init.Logf("ssh client close error: %+v", e)
				}
				return e
			})

			return tunnel, nil // connected successfully
		}

		if reterr != nil {
			return nil, reterr
		}
		return nil, fmt.Errorf("no ssh tunnels available") // TODO: better error message?
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: obj.Seeds,
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(grpcWithContextDialerFunc),
		},
	})
	if err != nil {
		return errwrap.Append(obj.cleanup(), err)
	}
	obj.cleanups = append(obj.cleanups, func() error {
		e := etcdClient.Close()
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

// CLeanup runs last.
func (obj *World) Cleanup() error {
	return obj.cleanup()
}

// dialSSHWithContext wraps ssh.Dial so that we can have a context to cancel.
func dialSSHWithContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ssh.NewClient(c, chans, reqs), nil
}
