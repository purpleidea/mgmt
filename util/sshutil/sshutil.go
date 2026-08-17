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

// Package sshutil has some utility functions for dealing with SSH clients that
// are shared between the various consumers of the golang SSH implementation.
package sshutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	// DefaultSSHDir is the default directory to scan for private keys.
	DefaultSSHDir = "~/.ssh/"

	// DefaultKnownHostsPath is the default known_hosts file location.
	DefaultKnownHostsPath = "~/.ssh/known_hosts"
)

// Helper is a struct of common SSH client operations which want a logger. Fill
// in the fields and then use the methods.
type Helper struct {
	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// KeySigners gets a list of possible key signers found in the default SSH
// directory. These are used to get the available types of the keys, and the
// auth methods.
func (obj *Helper) KeySigners() ([]ssh.Signer, error) {
	sshDir, err := util.ExpandHome(DefaultSSHDir)
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

		signer, err := obj.KeySigner(p)
		if err != nil {
			obj.Logf("%s", err)
			continue
		}

		signers = append(signers, signer)
	}

	return signers, nil
}

// KeySigner returns a single signer from an absolute path.
func (obj *Helper) KeySigner(p string) (ssh.Signer, error) {
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
			return nil, fmt.Errorf("password required for key file: %s", p)
		}

		return nil, fmt.Errorf("key file parsing error: %s", err)
	}

	obj.Logf("found auth option in: %s", p)

	// return the Signer for this private key
	return signer, nil
}

// isPossiblePrivateKeyFile determines if we've found a private key file.
func (obj *Helper) isPossiblePrivateKeyFile(p string) error {
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

// PrioritizeHostKeyAlgorithms returns the host key algorithms that we tell the
// server that we support. The order matters, because the server returns only
// one host key. The preferred key must be a known server key, not an unrelated
// key used for client authentication. The optional knownTypes are host key
// types (for example from the known_hosts file) that we already trust for this
// host; prioritizing them makes the handshake negotiate a key we can actually
// verify, the same way OpenSSH does.
func (obj *Helper) PrioritizeHostKeyAlgorithms(allHostKeyAlgos []string, hostKey ssh.PublicKey, knownTypes ...string) []string {
	preferred := []string{}
	// An explicitly configured host key is the strongest signal, so it goes
	// first, followed by whatever we found in the known_hosts file.
	if hostKey != nil {
		preferred = append(preferred, hostKeyTypeAlgorithms(hostKey.Type())...)
	}
	for _, t := range knownTypes {
		preferred = append(preferred, hostKeyTypeAlgorithms(t)...)
	}

	rank := make(map[string]int, len(preferred))
	for i, t := range preferred {
		if _, exists := rank[t]; exists {
			continue // keep the first (highest) rank for this algo
		}
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

// KnownHostsKey takes a known_hosts key entry (just the base64 key part) and
// turns it into the ssh.PublicKey needed for HostKeyCallback. This excerpt was
// taken from: x/crypto/ssh:keys.go:func parseAuthorizedKey
func (obj *Helper) KnownHostsKey(hostkey string) (ssh.PublicKey, error) {
	key := make([]byte, base64.StdEncoding.DecodedLen(len(hostkey)))
	n, err := base64.StdEncoding.Decode(key, []byte(hostkey))
	if err != nil {
		// Make it easier to spot this common error...
		s := err.Error()
		m := "illegal base64 data at input byte "
		if strings.HasPrefix(s, m) {
			if d, e := strconv.Atoi(s[len(m):]); e == nil {
				obj.Logf("error: %v", err)
				obj.Logf("host key: %s", hostkey)
				obj.Logf("location: %s^", strings.Repeat(" ", d))
			}
		}
		return nil, err
	}
	key = key[:n]
	return ssh.ParsePublicKey(key)
}

// HostKeyCallback is a helper function to get the ssh callback function needed.
// If a nil hostkey is specified, then it only checks the known_hosts file,
// otherwise it checks that key first. The knownHosts path selects the file to
// check against; an empty string uses the default location.
func (obj *Helper) HostKeyCallback(hostkey ssh.PublicKey, knownHosts string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		obj.Logf("server host key type: %s", key.Type())
		obj.Logf("host key fingerprint: %s", ssh.FingerprintSHA256(key))

		// First try our one known key if it exists.
		if hostkey != nil {
			fn := ssh.FixedHostKey(hostkey)
			if fn(hostname, remote, key) == nil {
				if obj.Debug {
					obj.Logf("matched key")
				}
				return nil // found it!
			}
			obj.Logf("did not match known key: %s", ssh.FingerprintSHA256(hostkey))
		}

		p, err := knownHostsPath(knownHosts)
		if err != nil {
			return err
		}

		fn, err := knownhosts.New(p)
		if err != nil {
			return err
		}
		obj.Logf("trying known_hosts file at: %s", p)
		err = fn(hostname, remote, key)
		if err == nil {
			obj.Logf("host key matched")
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

// KnownHostsAlgorithms returns the host key types that we already trust for
// this host, based on the entries in the known_hosts file. This lets us
// prioritize the host key algorithms we offer during the handshake, the same
// way OpenSSH prefers the key types it has already recorded for a host, so that
// we negotiate a host key we can actually verify. The addr should be in
// host:port form. The knownHosts path selects the file to read; an empty string
// uses the default location. If the host is not found (or there is no
// known_hosts file), an empty list is returned without error.
func (obj *Helper) KnownHostsAlgorithms(addr string, knownHosts string) ([]string, error) {
	p, err := knownHostsPath(knownHosts)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		return nil, nil // no known_hosts file, nothing to prioritize
	}

	fn, err := knownhosts.New(p)
	if err != nil {
		return nil, err
	}

	// The knownhosts callback doesn't expose the trusted key types
	// directly, so we probe it with a throwaway key. When the host is
	// present but the key doesn't match, the callback returns a KeyError
	// whose Want field lists every key we already trust for this host
	// (with proper hashed-host matching). That's how we learn which host
	// key types to prioritize.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}

	// The callback requires a non-nil remote whose String() is host:port.
	// We reuse addr since checkAddr prefers the (non-empty) hostname
	// anyways.
	err = fn(addr, &fakeAddr{addr: addr}, signer.PublicKey())
	if err == nil {
		return nil, nil // the throwaway key matched somehow; nothing to do
	}
	ke, ok := err.(*knownhosts.KeyError)
	if !ok || len(ke.Want) == 0 {
		return nil, nil // host not found in known_hosts
	}

	types := []string{}
	seen := make(map[string]struct{})
	for _, kk := range ke.Want {
		t := kk.Key.Type()
		if _, exists := seen[t]; exists {
			continue
		}
		seen[t] = struct{}{}
		types = append(types, t)
	}
	return types, nil
}

// fakeAddr is a net.Addr backed by a plain host:port string. It's used to probe
// the knownhosts callback, which needs a non-nil remote addr.
type fakeAddr struct {
	addr string
}

// Network implements the net.Addr interface.
func (obj *fakeAddr) Network() string { return "tcp" }

// String implements the net.Addr interface.
func (obj *fakeAddr) String() string { return obj.addr }

// hostKeyTypeAlgorithms expands a host key type (as found on a server key or in
// a known_hosts entry) into the host-key algorithm names we should prefer for
// it. An RSA key can be used with either secure SHA-2 algorithm, which we
// prefer first, but we also keep the legacy SHA-1 ssh-rsa algorithm as a
// fallback since it's still used widely.
func hostKeyTypeAlgorithms(t string) []string {
	switch t {
	case ssh.KeyAlgoRSA:
		return []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA}
	case ssh.CertAlgoRSAv01:
		return []string{ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01, ssh.CertAlgoRSAv01}
	default:
		return []string{t}
	}
}

// knownHostsPath resolves the known_hosts file path, expanding strings of the
// form ~james/.ssh/known_hosts. An empty input falls back to the default
// location. The caller decides the path so that everything which reads
// known_hosts (verification and host key algorithm prioritization) agrees.
func knownHostsPath(knownHosts string) (string, error) {
	s := knownHosts
	if s == "" {
		s = DefaultKnownHostsPath // "~/.ssh/known_hosts"
	}
	p, err := util.ExpandHome(s)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't find home directory for known_hosts file")
	}
	if p == "" {
		return "", fmt.Errorf("empty known_hosts path specified")
	}
	return p, nil
}

// DialSSHWithContext wraps ssh.Dial so that we can have a context to cancel.
func DialSSHWithContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	// NewClientConn performs a synchronous handshake without accepting a
	// context. This watcher is owned and joined by this function; closing
	// the raw connection unblocks every handshake read or write.
	handshakeDone := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	close(handshakeDone)
	<-watchDone
	if err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}

	return ssh.NewClient(c, chans, reqs), nil
}
