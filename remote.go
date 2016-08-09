// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// This set of structs and methods are for running mgmt remotely over SSH. This
// gives us the architectural robustness of our current design, combined with
// the ability to run it with an "agent-less" approach for bootstrapping, and
// in environments with more restrictive installation requirements. In general
// the following sequence is run:
//
//	1) connect to remote host
//	2) make temporary directory
//	3) copy over the mgmt binary and graph definition
//	4) tunnel tcp connections for etcd
//	5) run it!
//	6) finish and quit
//	7) close tunnels
//	8) clean up
//	9) disconnect
//
// The main advantage of this agent-less approach, is while multiple of these
// remote mgmt transient agents are running, they can still exchange data and
// converge together without directly connecting, since they all tunnel through
// the etcd server running on the initiator.
package main // TODO: make this a separate ssh package

// TODO: running with two identical remote endpoints over a slow connection, eg:
// --remote file1.yaml --remote file1.yaml
// where we ^C when both file copies are running seems to deadlock the process.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/howeyc/gopass"
	"github.com/kardianos/osext"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// FIXME: should this dir be in /var/ instead?
	formatPattern                        = "/tmp/mgmt.%s/"                        // remote format, to match `mktemp`
	formatChars                          = "abcdefghijklmnopqrstuvwxyz0123456789" // chars for fmt string // TODO: what does mktemp use?
	maxCollisions                        = 13                                     // number of tries to try making a unique remote directory
	defaultUser                          = "mgmt"                                 // default user
	defaultPort                   uint16 = 22                                     // default port
	maxPasswordTries                     = 3                                      // max number of interactive password tries
	nonInteractivePasswordTimeout        = 5 * 2                                  // five minutes
)

// The SSH struct is the unit building block for a single remote SSH connection.
type SSH struct {
	host string           // remote host to connect to
	port uint16           // remote port to connect to (usually 22)
	user string           // username to connect with
	auth []ssh.AuthMethod // list of auth for ssh

	file       string   // the graph definition file to run
	clientURLs []string // list of urls where the local server is listening
	remoteURLs []string // list of urls where the remote server connects to
	noop       bool     // whether to run the remote process with --noop
	noWatch    bool     // whether to run the remote process with --no-watch

	caching bool   // whether to try and cache the copy of the binary
	prefix  string // location we're allowed to put data on the remote server

	client   *ssh.Client  // client object
	sftp     *sftp.Client // sftp object
	listener net.Listener // remote listener
	session  *ssh.Session // session for exec
	f1       *os.File     // file object for SftpCopy source
	f2       *sftp.File   // file object for SftpCopy destination

	wg      sync.WaitGroup // sync group for tunnel go routines
	lock    sync.Mutex     // mutex to avoid exit races
	exiting bool           // flag to let us know if we're exiting

	remotewd string // path to remote working directory
	execpath string // path to remote mgmt binary
	filepath string // path to remote file config
}

// Connect kicks off the SSH connection.
func (obj *SSH) Connect() error {
	config := &ssh.ClientConfig{
		User: obj.user,
		// you must pass in at least one implementation of AuthMethod
		Auth: obj.auth,
	}
	var err error
	obj.client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", obj.host, obj.port), config)
	if err != nil {
		return fmt.Errorf("Can't dial: %s", err.Error()) // Error() returns a string
	}
	return nil
}

// Close cleans up after the main SSH connection.
func (obj *SSH) Close() error {
	if obj.client == nil {
		return nil
	}
	return obj.client.Close()
}

// The Sftp function uses the sftp protocol to create a remote dir and copy over
// the binary to run. On error the string represents the path to the remote dir.
func (obj *SSH) Sftp() error {
	var err error

	if obj.client == nil {
		return fmt.Errorf("Not dialed!")
	}
	// this check is needed because the golang path.Base function is weird!
	if strings.HasSuffix(obj.file, "/") {
		return fmt.Errorf("File must not be a directory.")
	}

	// we run local operations first so that remote clean up is easier...
	selfpath := ""
	if selfpath, err = osext.Executable(); err != nil {
		return fmt.Errorf("Can't get executable path: %v", err)
	}
	log.Printf("Remote: Self executable is: %s", selfpath)

	// this calls NewSession and does everything in its own session :)
	obj.sftp, err = sftp.NewClient(obj.client)
	if err != nil {
		return err
	}

	// TODO: make the path configurable to deal with /tmp/ mounted noexec?
	tmpdir := func() string {
		return fmt.Sprintf(formatPattern, fmtUUID(10)) // eg: /tmp/mgmt.abcdefghij/
	}
	var ready bool
	obj.remotewd = ""
	if obj.caching && obj.prefix != "" {
		// try and make the parent dir, just in case...
		obj.sftp.Mkdir(obj.prefix)                     // ignore any errors
		obj.remotewd = path.Join(obj.prefix, "remote") // eg: /var/lib/mgmt/remote/
		if fileinfo, err := obj.sftp.Stat(obj.remotewd); err == nil {
			if fileinfo.IsDir() {
				ready = true
			}
		}
	} else {
		obj.remotewd = tmpdir()
	}

	for i := 0; true; {
		// NOTE: since fmtUUID is deterministic, if we don't clean up
		// previous runs, we may get the same paths generated, and here
		// they will conflict.
		if err := obj.sftp.Mkdir(obj.remotewd); err != nil {
			// TODO: if we could determine if this was a "file
			// already exists" error, we could break now!
			// https://github.com/pkg/sftp/issues/131
			//if status, ok := err.(*sftp.StatusError); ok {
			//	log.Printf("Code: %v, %v", status.Code, status.Error())
			//	if status.Code == ??? && obj.caching {
			//		break
			//	}
			//}
			if ready { // dir already exists
				break
			}

			i++ // count number of times we've tried
			e := fmt.Errorf("Can't make tmp directory: %s", err)
			log.Println(e)
			if i >= maxCollisions {
				log.Printf("Remote: Please clean up the remote dir: %s", obj.remotewd)
				return e
			}
			if obj.caching { // maybe /var/lib/mgmt/ is read-only.
				obj.remotewd = tmpdir()
			}
			continue // try again, unlucky conflict!
		}
		log.Printf("Remote: Remotely created: %s", obj.remotewd)
		break
	}

	obj.execpath = path.Join(obj.remotewd, program) // program is a compile time string from main.go
	log.Printf("Remote: Remote path is: %s", obj.execpath)

	var same bool
	if obj.caching {
		same, _ = obj.SftpHash(selfpath, obj.execpath) // ignore errors
	}
	if same {
		log.Println("Remote: Skipping binary copy, file was cached.")
	} else {
		log.Println("Remote: Copying binary, please be patient...")
		_, err = obj.SftpCopy(selfpath, obj.execpath)
		if err != nil {
			// TODO: cleanup
			return fmt.Errorf("Error copying binary: %s", err)
		}
	}

	if obj.exitCheck() {
		return nil
	}

	// make file executable; don't cache this in case it didn't ever happen
	// TODO: do we want the group or other bits set?
	if err := obj.sftp.Chmod(obj.execpath, 0770); err != nil {
		return fmt.Errorf("Can't set file mode bits!")
	}

	// copy graph file
	// TODO: should future versions use torrent for this copy and updates?
	obj.filepath = path.Join(obj.remotewd, path.Base(obj.file)) // same filename
	log.Println("Remote: Copying graph definition...")
	_, err = obj.SftpGraphCopy()
	if err != nil {
		// TODO: cleanup
		return fmt.Errorf("Error copying graph: %s", err)
	}

	return nil
}

// SftpGraphCopy is a helper function used for re-copying the graph definition.
func (obj *SSH) SftpGraphCopy() (int64, error) {
	if obj.filepath == "" {
		return -1, fmt.Errorf("Sftp session isn't ready yet!")
	}
	return obj.SftpCopy(obj.file, obj.filepath)
}

// SftpCopy is a simple helper function that runs a local -> remote sftp copy.
func (obj *SSH) SftpCopy(src, dst string) (int64, error) {
	if obj.sftp == nil {
		return -1, fmt.Errorf("Sftp session is not active!")
	}
	var err error
	// TODO: add a check to make sure we don't run two copies of this
	// function at the same time! they both would use obj.f1 and obj.f2

	obj.f1, err = os.Open(src) // open a handle to read the file
	if err != nil {
		return -1, err
	}
	defer obj.f1.Close()

	if obj.exitCheck() {
		return -1, nil
	}

	obj.f2, err = obj.sftp.Create(dst) // open a handle to create the file
	if err != nil {
		return -1, err
	}
	defer obj.f2.Close()

	if obj.exitCheck() {
		return -1, nil
	}

	// the actual copy, this might take time...
	n, err := io.Copy(obj.f2, obj.f1) // dst, src -> n, error
	if err != nil {
		return n, fmt.Errorf("Can't copy to remote path: %v", err)
	}
	if n <= 0 {
		return n, fmt.Errorf("Zero bytes copied!")
	}
	return n, nil
}

// SftpHash hashes a local file, and compares that hash to the result of a
// remote hashing command run on the second file path.
func (obj *SSH) SftpHash(local, remote string) (bool, error) {
	// TODO: we could run both hash operations in parallel! :)
	hash := sha256.New()
	f, err := os.Open(local)
	if err != nil {
		return false, err
	}
	defer f.Close()

	if _, err := io.Copy(hash, f); err != nil {
		return false, err
	}
	sha256sum := hex.EncodeToString(hash.Sum(nil))
	//log.Printf("sha256sum: %s", sha256sum)

	// We run a remote hashing command, instead of reading the file in over
	// the wire and hashing it ourselves, because assuming symmetric
	// bandwidth, that would defeat the point of caching it altogether!
	cmd := fmt.Sprintf("sha256sum '%s'", remote)
	out, err := obj.simpleRun(cmd)
	if err != nil {
		return false, err
	}

	s := strings.Split(out, " ") // sha256sum returns: hash + filename
	if s[0] == sha256sum {
		return true, nil
	}
	return false, nil // files were different
}

// SftpClean cleans up the mess and closes the connection from the sftp work.
func (obj *SSH) SftpClean() error {
	if obj.sftp == nil {
		return nil
	}

	// TODO: if this runs before we ever use f1 or f2 it could be a panic!
	// TODO: fix this possible? panic if we ever end up caring about it...
	// close any copy operations that are in progress...
	obj.f1.Close() // TODO: we probably only need to shutdown one of them,
	obj.f2.Close() // but which one should we shutdown? close both for now

	// clean up the graph definition in obj.remotewd
	err := obj.sftp.Remove(obj.filepath)

	// if we're not caching+sha1sum-ing, then also remove the rest
	if !obj.caching {
		if e := obj.sftp.Remove(obj.execpath); e != nil {
			err = e
		}
		if e := obj.sftp.Remove(obj.remotewd); e != nil {
			err = e
		}
	}

	if e := obj.sftp.Close(); e != nil {
		err = e
	}

	// TODO: return all errors when we have a better error struct
	return err
}

// Tunnel initiates the reverse SSH tunnel. You can .Wait() on the returned
// sync WaitGroup to know when the tunnels have closed completely.
func (obj *SSH) Tunnel() error {
	var err error

	if len(obj.clientURLs) < 1 {
		return fmt.Errorf("Need at least one client URL to tunnel!")
	}
	if len(obj.remoteURLs) < 1 {
		return fmt.Errorf("Need at least one remote URL to tunnel!")
	}

	// TODO: do something less arbitrary about which one we pick?
	url := cleanURL(obj.remoteURLs[0]) // arbitrarily pick the first one
	// reverse `ssh -R` listener to listen on the remote host
	obj.listener, err = obj.client.Listen("tcp", url) // remote
	if err != nil {
		return fmt.Errorf("Can't listen on remote host: %s", err)
	}

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		for {
			conn, err := obj.listener.Accept()
			if err != nil {
				// a Close() will trigger an EOF "error" here!
				if err == io.EOF {
					return
				}
				log.Printf("Remote: Error accepting on remote host: %s", err)
				return // FIXME: return or continue?
			}
			// XXX: pass in wg to this method and to its children?
			if f := obj.forward(conn); f != nil {
				// TODO: is this correct?
				defer f.Close() // close the remote connection
			} else {
				// TODO: is this correct?
				// close the listener since it is useless now
				obj.listener.Close()
			}
		}
	}()
	return nil
}

// forward is a helper function to make the tunnelling code more readable.
func (obj *SSH) forward(remoteConn net.Conn) net.Conn {
	// TODO: validate URL format?
	// TODO: do something less arbitrary about which one we pick?
	url := cleanURL(obj.clientURLs[0])     // arbitrarily pick the first one
	localConn, err := net.Dial("tcp", url) // local
	if err != nil {
		log.Printf("Remote: Local dial error: %s", err)
		return nil // seen as an error...
	}

	cp := func(writer, reader net.Conn) {
		// Copy copies from src to dst until either EOF is reached on
		// src or an error occurs. It returns the number of bytes copied
		// and the first error encountered while copying, if any.
		// Note: src & dst are backwards in golang as compared to cp, lol!
		n, err := io.Copy(writer, reader) // from reader to writer
		if err != nil {
			log.Printf("Remote: io.Copy error: %s", err)
			// FIXME: what should we do here???
		}
		if DEBUG {
			log.Printf("Remote: io.Copy finished: %d", n)
		}
	}
	go cp(remoteConn, localConn)
	go cp(localConn, remoteConn)

	return localConn // success!
}

// TunnelClose causes any currently connected Tunnel to shutdown.
func (obj *SSH) TunnelClose() error {
	if obj.listener != nil {
		err := obj.listener.Close()
		obj.wg.Wait() // wait for everyone to close
		obj.listener = nil
		return err
	}
	return nil
}

// Exec runs the binary on the remote server.
func (obj *SSH) Exec() error {
	if obj.execpath == "" {
		return fmt.Errorf("Must have a binary path to execute!")
	}
	if obj.filepath == "" {
		return fmt.Errorf("Must have a graph definition to run!")
	}

	var err error
	obj.session, err = obj.client.NewSession()
	if err != nil {
		return fmt.Errorf("Failed to create session: %s", err.Error())
	}
	defer obj.session.Close()

	var b combinedWriter
	obj.session.Stdout = &b
	obj.session.Stderr = &b

	// TODO: do something less arbitrary about which one we pick?
	url := cleanURL(obj.remoteURLs[0])                           // arbitrarily pick the first one
	seeds := fmt.Sprintf("--no-server --seeds 'http://%s'", url) // XXX: escape dangerous untrusted input?
	file := fmt.Sprintf("--file '%s'", obj.filepath)             // XXX: escape dangerous untrusted input!
	args := []string{seeds, file}
	if obj.noop {
		args = append(args, "--noop")
	}
	if obj.noWatch {
		args = append(args, "--no-watch")
	}

	// TODO: add --converged-timeout support for group

	cmd := fmt.Sprintf("%s run %s", obj.execpath, strings.Join(args, " "))
	log.Printf("Remote: Running: %s", cmd)
	if err := obj.session.Run(cmd); err != nil {
		// The returned error is nil if the command runs, has no
		// problems copying stdin, stdout, and stderr, and exits with a
		// zero exit status. If the remote server does not send an exit
		// status, an error of type *ExitMissingError is returned. If
		// the command completes unsuccessfully or is interrupted by a
		// signal, the error is of type *ExitError. Other error types
		// may be returned for I/O problems.
		if e, ok := err.(*ssh.ExitError); ok {
			if sig := e.Waitmsg.Signal(); sig != "" {
				log.Printf("Remote: Exit signal: %s", sig)
			}
			log.Printf("Remote: Error: Output...\n%s", b.PrefixedString("|\t"))
			return fmt.Errorf("Exited (%d) with: %s", e.Waitmsg.ExitStatus(), e.Error())

		} else if e, ok := err.(*ssh.ExitMissingError); ok {
			return fmt.Errorf("Exit code missing: %s", e.Error())
		}
		// TODO: catch other types of errors here...
		return fmt.Errorf("Failed for unknown reason: %s", err.Error())
	}
	log.Printf("Remote: Output...\n%s", b.PrefixedString("|\t"))
	return nil
}

// simpleRun is a simple helper for running commands in new sessions.
func (obj *SSH) simpleRun(cmd string) (string, error) {
	session, err := obj.client.NewSession() // not the main session!
	if err != nil {
		return "", fmt.Errorf("Failed to create session: %s", err.Error())
	}
	defer session.Close()
	var out []byte
	if out, err = session.CombinedOutput(cmd); err != nil {
		return string(out), fmt.Errorf("Error running command: %s", err)
	}
	return string(out), nil
}

// ExecExit sends a SIGINT (^C) signal to the remote process, and waits for the
// process to exit.
func (obj *SSH) ExecExit() error {
	if obj.session == nil {
		return nil
	}
	// Signal sends the given signal to the remote process.
	// FIXME: this doesn't work, see: https://github.com/golang/go/issues/16597
	// FIXME: additionally, a disconnect leaves the remote process running! :(
	if err := obj.session.Signal(ssh.SIGINT); err != nil {
		log.Printf("Remote: Signal: Error: %s", err)
	}

	// FIXME: workaround: force a signal!
	if _, err := obj.simpleRun(fmt.Sprintf("killall -SIGINT %s", program)); err != nil { // FIXME: low specificity
		log.Printf("Remote: Failed to send SIGINT: %s", err.Error())
	}

	// emergency timeout...
	go func() {
		// try killing the process more violently
		time.Sleep(10 * time.Second)
		//obj.session.Signal(ssh.SIGKILL)
		cmd := fmt.Sprintf("killall -SIGKILL %s", program) // FIXME: low specificity
		obj.simpleRun(cmd)
	}()

	// FIXME: workaround: wait (spin lock) until process quits cleanly...
	cmd := fmt.Sprintf("while killall -0 %s 2> /dev/null; do sleep 1s; done", program) // FIXME: low specificity
	if _, err := obj.simpleRun(cmd); err != nil {
		return fmt.Errorf("Error waiting: %s", err)
	}

	return nil
}

// Go kicks off the entire sequence of one SSH connection.
func (obj *SSH) Go() error {
	if obj.exitCheck() {
		return nil
	}

	// connect
	log.Println("Remote: Connect...")
	if err := obj.Connect(); err != nil {
		return fmt.Errorf("Remote: SSH errored with: %v", err)
	}
	defer obj.Close()

	if obj.exitCheck() {
		return nil
	}

	// sftp
	log.Println("Remote: Sftp...")
	defer obj.SftpClean()
	if err := obj.Sftp(); err != nil {
		return fmt.Errorf("Remote: Sftp errored with: %v", err)
	}

	if obj.exitCheck() {
		return nil
	}

	// tunnel
	log.Println("Remote: Tunnelling...")
	if err := obj.Tunnel(); err != nil { // non-blocking
		log.Printf("Remote: Tunnel errored with: %v", err)
		return err
	}
	defer obj.TunnelClose()

	if obj.exitCheck() {
		return nil
	}

	// exec
	log.Println("Remote: Exec...")
	if err := obj.Exec(); err != nil {
		log.Printf("Remote: Exec errored with: %v", err)
		return err
	}

	log.Println("Remote: Done!")
	return nil
}

// exitCheck is a helper function which stops additional stages from running if
// we detect that a Stop() action has been called.
func (obj *SSH) exitCheck() bool {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	if obj.exiting {
		return true // prevent from continuing to the next stage
	}
	return false
}

// Stop shuts down any SSH in progress as safely and quickly as possible.
func (obj *SSH) Stop() error {
	obj.lock.Lock()
	obj.exiting = true // don't spawn new steps once this flag is set!
	obj.lock.Unlock()

	// TODO: return all errors when we have a better error struct
	var e error
	// go through each stage in reverse order and request an exit
	if err := obj.ExecExit(); e == nil && err != nil { // waits for program to exit
		e = err
	}
	if err := obj.TunnelClose(); e == nil && err != nil {
		e = err
	}

	// TODO: match errors due to stop signal and ignore them!
	if err := obj.SftpClean(); e == nil && err != nil {
		e = err
	}
	if err := obj.Close(); e == nil && err != nil {
		e = err
	}
	return e
}

// The Remotes struct manages a set of SSH connections.
// TODO: rename this to something more logical
type Remotes struct {
	clientURLs   []string // list of urls where the local server is listening
	remoteURLs   []string // list of urls where the remote server connects to
	noop         bool     // whether to run in noop mode
	remotes      []string // list of remote graph definition files to run
	fileWatch    chan string
	cConns       uint16 // number of concurrent ssh connections, zero means unlimited
	interactive  bool   // allow interactive prompting
	sshPrivIdRsa string // path to ~/.ssh/id_rsa
	caching      bool   // whether to try and cache the copy of the binary
	prefix       string // folder prefix to use for misc storage

	wg        sync.WaitGroup  // keep track of each running SSH connection
	lock      sync.Mutex      // mutex for access to sshmap
	sshmap    map[string]*SSH // map to each SSH struct with the remote as the key
	exiting   bool            // flag to let us know if we're exiting
	semaphore Semaphore       // counting semaphore to limit concurrent connections
}

// The NewRemotes function builds a Remotes struct.
func NewRemotes(clientURLs, remoteURLs []string, noop bool, remotes []string, fileWatch chan string, cConns uint16, interactive bool, sshPrivIdRsa string, caching bool, prefix string) *Remotes {
	return &Remotes{
		clientURLs:   clientURLs,
		remoteURLs:   remoteURLs,
		noop:         noop,
		remotes:      StrRemoveDuplicatesInList(remotes),
		fileWatch:    fileWatch,
		cConns:       cConns,
		interactive:  interactive,
		sshPrivIdRsa: sshPrivIdRsa,
		caching:      caching,
		prefix:       prefix,
		sshmap:       make(map[string]*SSH),
		semaphore:    NewSemaphore(int(cConns)),
	}
}

// NewSSH is a helper function that does the initial parsing into an SSH obj.
// It takes as input the path to a graph definition file.
func (obj *Remotes) NewSSH(file string) (*SSH, error) {
	// first do the parsing...
	config := ParseConfigFromFile(file)
	if config == nil {
		return nil, fmt.Errorf("Remote: Error parsing remote graph: %s", file)
	}
	if config.Remote == "" {
		return nil, fmt.Errorf("Remote: No remote endpoint in the graph: %s", file)
	}

	// do the url parsing...
	u, err := url.Parse(config.Remote)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "" && u.Scheme != "ssh" {
		return nil, fmt.Errorf("Unknown remote scheme: %s", u.Scheme)
	}

	host := ""
	port := defaultPort // default
	x := strings.Split(u.Host, ":")
	if c := len(x); c == 0 || c > 2 { // need one or two chunks
		return nil, fmt.Errorf("Can't parse host pattern: %s", u.Host)
	} else if c == 2 {
		v, err := strconv.ParseUint(x[1], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("Can't parse port: %s", x[1])
		}
		port = uint16(v)
	}
	host = x[0]
	if host == "" {
		return nil, fmt.Errorf("Empty hostname!")
	}

	user := defaultUser // default
	if x := u.User.Username(); x != "" {
		user = x
	}
	auth := []ssh.AuthMethod{}
	if secret, b := u.User.Password(); b {
		auth = append(auth, ssh.Password(secret))
	}

	// get ssh key auth if available
	if a, err := obj.sshKeyAuth(); err == nil {
		auth = append(auth, a)
	}

	// if there are no auth methods available, add interactive to be helpful
	if len(auth) == 0 || obj.interactive {
		auth = append(auth, ssh.RetryableAuthMethod(ssh.PasswordCallback(obj.passwordCallback(user, host)), maxPasswordTries))
	}

	if len(auth) == 0 {
		return nil, fmt.Errorf("No authentication methods available!")
	}

	return &SSH{
		host:       host,
		port:       port,
		user:       user,
		auth:       auth,
		file:       file,
		clientURLs: obj.clientURLs,
		remoteURLs: obj.remoteURLs,
		noop:       obj.noop,
		noWatch:    obj.fileWatch == nil,
		caching:    obj.caching,
		prefix:     obj.prefix,
	}, nil
}

// sshKeyAuth is a helper function to get the ssh key auth struct needed
func (obj *Remotes) sshKeyAuth() (ssh.AuthMethod, error) {
	if obj.sshPrivIdRsa == "" {
		return nil, fmt.Errorf("Empty path specified!")
	}
	p := ""
	// TODO: this doesn't match strings of the form: ~james/.ssh/id_rsa
	if strings.HasPrefix(obj.sshPrivIdRsa, "~/") {
		usr, err := user.Current()
		if err != nil {
			log.Printf("Remote: Can't find home directory automatically.")
			return nil, err
		}
		p = path.Join(usr.HomeDir, obj.sshPrivIdRsa[len("~/"):])
	}
	if p == "" {
		return nil, fmt.Errorf("Empty path specified!")
	}
	// A public key may be used to authenticate against the server by using
	// an unencrypted PEM-encoded private key file. If you have an encrypted
	// private key, the crypto/x509 package can be used to decrypt it.
	key, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

// passwordCallback is a function which returns the appropriate type of callback.
func (obj *Remotes) passwordCallback(user, host string) func() (string, error) {
	timeout := nonInteractivePasswordTimeout // default
	if obj.interactive {                     // return after a timeout if not interactive
		timeout = -1 // unlimited when we asked for interactive mode!
	}
	cb := func() (string, error) {
		passchan := make(chan string)
		failchan := make(chan error)

		go func() {
			log.Printf("Remote: Prompting for %s@%s password...", user, host)
			fmt.Printf("Password: ")
			password, err := gopass.GetPasswd()
			if err != nil { // on ^C or getch() error
				// returning an error will cancel the N retries on this
				failchan <- err
				return
			}
			passchan <- string(password)
		}()

		// wait for password, but include a timeout if we promiscuously
		// added the interactive mode
		select {
		case p := <-passchan:
			return p, nil
		case e := <-failchan:
			return "", e
		case <-TimeAfterOrBlock(timeout):
			return "", fmt.Errorf("Interactive timeout reached!")
		}
	}
	return cb
}

// The Run method of the Remotes struct kicks it all off. It is usually run from
// a go routine.
func (obj *Remotes) Run() {
	// kick off the file change notifications
	if obj.fileWatch != nil {
		go func() {
			for {
				f, more := <-obj.fileWatch // read from channel
				if !more {
					return
				}
				obj.lock.Lock()
				sshobj, exists := obj.sshmap[f]
				if !exists || sshobj == nil {
					continue // skip, this hasn't happened yet
				}
				// NOTE: if this errors because the session isn't
				// ready yet, it's fine, because we haven't copied
				// the file yet, so the update notification isn't
				// wasted, in fact, it's premature and redundant.
				if _, err := sshobj.SftpGraphCopy(); err == nil { // push new copy
					log.Printf("Remote: Copied over new graph definition: %s", f)
				} // ignore errors
				obj.lock.Unlock()
			}
		}()
	}

	// the semaphore provides the max simultaneous connection limit
	for _, f := range obj.remotes {
		if obj.cConns != 0 {
			obj.semaphore.P(1) // take one
		}
		obj.lock.Lock()
		if obj.exiting {
			return
		}
		sshobj, err := obj.NewSSH(f)
		if err != nil {
			log.Printf("Remote: Error: %s", err)
			continue
		}
		obj.sshmap[f] = sshobj // save a reference

		obj.wg.Add(1)
		go func() {
			if obj.cConns != 0 {
				defer obj.semaphore.V(1)
			}
			defer obj.wg.Done()
			if err := sshobj.Go(); err != nil {
				log.Printf("Remote: Error: %s", err)
			}
		}()
		obj.lock.Unlock()
	}
}

// The Exit method causes as much of the Remotes struct to shutdown as quickly
// and as cleanly as possible. It only returns once everything is shutdown.
func (obj *Remotes) Exit() {
	obj.lock.Lock()
	obj.exiting = true // don't spawn new ones once this flag is set!
	obj.lock.Unlock()
	for _, f := range obj.remotes {
		sshobj, exists := obj.sshmap[f]
		if !exists || sshobj == nil {
			continue
		}

		// TODO: should we run these as go routines?
		if err := sshobj.Stop(); err != nil {
			log.Printf("Remote: Error stopping: %s", err)
		}
	}

	obj.wg.Wait() // wait for everyone to exit
}

// fmtUUID makes a random string of length n, it is not cryptographically safe.
// This function actually usually generates the same sequence of random strings
// each time the program is run, which makes repeatability of this code easier.
func fmtUUID(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = formatChars[rand.Intn(len(formatChars))]
	}
	return string(b)
}

// cleanURL removes the scheme and leaves just the host:port combination.
func cleanURL(s string) string {
	x := s
	if !strings.Contains(s, "://") {
		x = "ssh://" + x
	}
	// the url.Parse returns "" for u.Host if given "hostname:22" as input.
	u, err := url.Parse(x)
	if err != nil {
		return ""
	}
	return u.Host
}

// Semaphore is a counting semaphore.
type Semaphore chan struct{}

func NewSemaphore(size int) Semaphore {
	return make(Semaphore, size)
}

// P acquires n resources.
func (s Semaphore) P(n int) {
	e := struct{}{}
	for i := 0; i < n; i++ {
		s <- e // acquire one
	}
}

// V releases n resources.
func (s Semaphore) V(n int) {
	for i := 0; i < n; i++ {
		<-s // release one
	}
}

// combinedWriter mimics what the ssh.CombinedOutput command does.
type combinedWriter struct {
	b  bytes.Buffer
	mu sync.Mutex
}

// The Write method writes to the bytes buffer with a lock to mix output safely.
func (w *combinedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

// The String function returns the contents of the buffer.
func (w *combinedWriter) String() string {
	return w.b.String()
}

// The PrefixedString returns the contents of the buffer with the prefix
// appended to every line.
func (w *combinedWriter) PrefixedString(prefix string) string {
	return prefix + strings.TrimSuffix(strings.Replace(w.String(), "\n", "\n"+prefix, -1), prefix)
}
