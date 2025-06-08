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
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	defaultUser                       = "root"
	defaultSSHPort             uint16 = 22
	defaultSSHHostKeyFieldName        = "hostkey" // querystring field name
	defaultEtcdPort            uint16 = 2379      // TODO: get this from etcd pkg
	defaultSSHDir                     = "~/.ssh/"
	defaultKnownHostsPath             = "~/.ssh/known_hosts"
	allowRSA                          = true // are big keys okay?
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

	// SSHID is the path to the ~/.ssh/id_??? key to use for auth. If you
	// omit this then this will look for your private key in all possible
	// paths. If you specific a specific path, then only that will be used.
	// This will expand the ~/ and ~user/ style path expansions.
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

// keySigners gets a list of possible key signers. These are used to get the
// available types of the keys, and the auth methods.
func (obj *World) keySigners() ([]ssh.Signer, error) {
	sshDir, err := util.ExpandHome(defaultSSHDir)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't find home directory")
	}
	if sshDir == "" {
		return nil, fmt.Errorf("empty path found")
	}

	files, err := os.ReadDir(sshDir)
	if err != nil {
		return nil, err
	}

	signers := []ssh.Signer{}
	// XXX: Should we aim to pull the keys out by order of preference?
	for _, file := range files {
		p := filepath.Join(sshDir, file.Name())

		if file.IsDir() || obj.isPossiblePrivateKeyFile(p) != nil {
			continue
		}

		signer, err := obj.keySigner(p)
		if err != nil {
			obj.init.Logf("%s", err)
			continue
		}

		signers = append(signers, signer)
	}

	return signers, nil
}

// keySigner returns a single signer from an absolute path.
func (obj *World) keySigner(p string) (ssh.Signer, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("key file error: %s", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty key file at: %s", p)
	}

	// A public key may be used to authenticate against the server by using
	// an unencrypted PEM-encoded private key file. If you have an encrypted
	// private key, the crypto/x509 package can be used to decrypt it.
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			return nil, fmt.Errorf("password required for key file:: %s", p)
		}

		return nil, fmt.Errorf("key file parsing error: %s", err)
	}

	obj.init.Logf("found auth option in: %s", p)

	// return the Signer for this private key
	return signer, nil
}

// isPossiblePrivateKeyFile determines if we've found a private key file.
func (obj *World) isPossiblePrivateKeyFile(p string) error {

	b := filepath.Base(p)
	//d := filepath.Dir(p) // no trailing slash :(

	if !strings.HasPrefix(b, "id_") {
		return fmt.Errorf("keys start with id_???")
	}

	if strings.HasSuffix(b, ".pub") {
		return fmt.Errorf("this is a public key")
	}

	if _, err := os.Stat(p + ".pub"); err != nil {
		return fmt.Errorf("matching public key is inaccessible")
	}

	// TODO: should we rule out anything else?

	return nil
}

// prioritizeHostKeyAlgorithms returns the host key algorithms that we tell the
// server that we support. The order matters, because this ordering will let the
// server know which we can authenticate against. Once we send a list, the
// server then only returns a single one, so it's important that we sort this
// list properly with what we have available at the very top.
func (obj *World) prioritizeHostKeyAlgorithms(allHostKeyAlgos, keyTypes []string) []string {
	rank := make(map[string]int, len(keyTypes))
	for i, t := range keyTypes {
		rank[t] = i
	}

	sorted := make([]string, len(allHostKeyAlgos))
	copy(sorted, allHostKeyAlgos)

	sort.SliceStable(sorted, func(i, j int) bool {
		rankI, okI := rank[sorted[i]]
		rankJ, okJ := rank[sorted[j]]

		switch {
		case okI && okJ:
			return rankI < rankJ
		case okI:
			return true
		case okJ:
			return false
		default:
			return false
		}
	})

	return sorted
}

// knownHostsKey takes a known_hosts key entry (just the base64 key part) and
// turns it into the ssh.PublicKey needed for hostKeyCallback. This excerpt was
// taken from: x/crypto/ssh:keys.go:func parseAuthorizedKey
func (obj *World) knownHostsKey(hostkey string) (ssh.PublicKey, error) {
	key := make([]byte, base64.StdEncoding.DecodedLen(len(hostkey)))
	n, err := base64.StdEncoding.Decode(key, []byte(hostkey))
	if err != nil {
		// Make it easier to spot this common error...
		s := err.Error()
		m := "illegal base64 data at input byte "
		if strings.HasPrefix(s, m) {
			if d, e := strconv.Atoi(s[len(m):]); e == nil {
				obj.init.Logf("error: %v", err)
				obj.init.Logf("host key: %s", hostkey)
				obj.init.Logf("location: %s^", strings.Repeat(" ", d))
			}
		}
		return nil, err
	}
	key = key[:n]
	return ssh.ParsePublicKey(key)
}

// hostKeyCallback is a helper function to get the ssh callback function needed.
// func (obj *World) hostKeyCallback() (ssh.HostKeyCallback, error) {
func (obj *World) hostKeyCallback(hostkey ssh.PublicKey) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		obj.init.Logf("server host key type: %s", key.Type())
		obj.init.Logf("host key fingerprint: %s", ssh.FingerprintSHA256(key))

		// First try our one known key if it exists.
		if hostkey != nil {
			fn := ssh.FixedHostKey(hostkey)
			if fn(hostname, remote, key) == nil {
				obj.init.Logf("matched key")
				return nil // found it!
			}
			obj.init.Logf("did not match known key: %s", ssh.FingerprintSHA256(hostkey))
		}

		// TODO: consider allowing a user-specified path in the future
		s := defaultKnownHostsPath // "~/.ssh/known_hosts"

		// expand strings of the form: ~james/.ssh/known_hosts
		p, err := util.ExpandHome(s)
		if err != nil {
			return errwrap.Wrapf(err, "can't find home directory for known_hosts file")
		}
		if p == "" {
			return fmt.Errorf("empty known_hosts path specified")
		}

		fn, err := knownhosts.New(p)
		if err != nil {
			return err
		}
		obj.init.Logf("trying known_hosts file at: %s", p)
		err = fn(hostname, remote, key)
		if err == nil {
			obj.init.Logf("host key matched")
			return nil
		}

		ke, ok := err.(*knownhosts.KeyError) // give a better error?
		if !ok || len(ke.Want) == 0 {
			return err
		}

		// Based on what we initially have in our ~/.ssh/ dir, our ssh
		// client offers keys to the server differently, and the server
		// replies with up to one of our acceptable choices. If none are
		// available, then this error message is weird, so we do all
		// this to make it clearer.
		types := []string{}
		for _, kk := range ke.Want { // known keys
			typ := kk.Key.Type()
			types = append(types, typ)

			// We found what the server offered, error normally...
			if key.Type() == typ {
				return err
			}
		}

		return fmt.Errorf("no known_hosts entry matching type, have: %s", strings.Join(types, ", "))
	}
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

	// TODO: Should we read out a list of these, one for each key type?
	base64Key := u.Query().Get(defaultSSHHostKeyFieldName) // urlencode me!
	if obj.HostKey != "" {                                 // override
		base64Key = obj.HostKey
	}
	var pubKey ssh.PublicKey // known hosts key
	if base64Key != "" {
		k, err := obj.knownHostsKey(base64Key)
		if err != nil {
			return errwrap.Wrapf(err, "invalid known_hosts key")
		}
		pubKey = k
	}

	addr := fmt.Sprintf("%s:%s", hostname, port)

	// Preference order of keys I have available...
	keyTypes := []string{
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

		signer, err := obj.keySigner(p)
		if err != nil {
			return err
		}
		typ := signer.PublicKey().Type()
		keyTypes = append(keyTypes, typ)
		auths = append(auths, ssh.PublicKeys(signer)) // add one
	}

	if len(auths) == 0 {
		signers, err := obj.keySigners()
		if err != nil {
			return err
		}
		for _, signer := range signers {
			typ := signer.PublicKey().Type()
			keyTypes = append(keyTypes, typ)
		}
		// TODO: should the order of the signers matter?
		if len(signers) > 0 {
			auths = append(auths, ssh.PublicKeys(signers...)) // add all
		}
	}

	if len(auths) == 0 {
		return fmt.Errorf("no auth options available")
	}

	obj.init.Logf("found %d available key types: %s", len(keyTypes), strings.Join(keyTypes, ", "))

	algorithms := ssh.SupportedAlgorithms()
	preferredAlgoOrder := algorithms.HostKeys // the defaults
	if allowRSA {
		preferredAlgoOrder = append(preferredAlgoOrder, ssh.KeyAlgoRSA)
	}
	obj.init.Logf("supported algos: %s", strings.Join(preferredAlgoOrder, ", "))

	// SSH connection configuration
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: auths,
		//HostKeyCallback: ssh.InsecureIgnoreHostKey(), // testing
		HostKeyCallback: obj.hostKeyCallback(pubKey),

		// This is the list of host key algorithms that this SSH client
		// will offer to the SSH server when it says hello. This can be
		// different from what a normal terminal SSH client might do,
		// which means you might not get the right SSH host key algo
		// offered back to you, so make sure you provide what it's
		// asking for. Maybe we need to make this configurable by the
		// user.
		//HostKeyAlgorithms: algorithms.HostKeys,
		HostKeyAlgorithms: obj.prioritizeHostKeyAlgorithms(preferredAlgoOrder, keyTypes),
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
