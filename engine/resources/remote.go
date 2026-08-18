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

package resources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	distroUtil "github.com/purpleidea/mgmt/util/distro"
	"github.com/purpleidea/mgmt/util/errwrap"
	passwordUtil "github.com/purpleidea/mgmt/util/password"
	"github.com/purpleidea/mgmt/util/sshutil"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func init() {
	engine.RegisterResource("remote", func() engine.Res { return &RemoteRes{} })

	remotePasswordStdinLock = make(chan struct{}, 1) // init (once for all remotes)
}

const (
	// remoteUserPrefix is the private directory beneath the remote user's
	// home which contains non-cached remote state.
	remoteUserPrefix = ".mgmt/remote/"

	// remoteCachePrefix is the directory for cached remote state.
	// TODO: should this be configurable?
	remoteCachePrefix = "/var/lib/mgmt/remote/"

	// remoteUIDLength is the number of hex chars used for the working
	// directory uid.
	remoteUIDLength = 10

	// remotePidFilename is the file (inside the remote working directory)
	// which stores the pid of the remote process that we spawned.
	remotePidFilename = "pid"

	// remoteLogFilename is the file (inside the remote working directory)
	// which collects the output of the remote process that we spawned.
	remoteLogFilename = "mgmt.log"

	// remoteExitFilename is the file (inside the remote working directory)
	// which stores the exit status of a remote converged-exit process.
	remoteExitFilename = "exit"

	// remoteDepsFilename is the file (inside the remote working directory)
	// which stores the generated dependency installation script.
	remoteDepsFilename = "deps.sh"

	// remotePrefixDirname is the directory (inside the remote working
	// directory) which the remote process uses as its working prefix.
	remotePrefixDirname = "prefix"

	// remoteCheckInterval is the polling interval that Watch uses to check
	// if a persistent remote process is still in the expected state.
	remoteCheckInterval = 30 * time.Second

	// remoteExitTimeout is the number of seconds that we wait for the
	// remote process to exit after a SIGINT, before we escalate and send it
	// an unblockable SIGKILL.
	remoteExitTimeout = 30 // seconds

	// remoteStartupRetries is the number of one second interval checks we
	// make while waiting for the remote process to start up successfully.
	remoteStartupRetries = 3

	// remoteShutdownTimeout bounds the time we spend stopping a transient
	// remote process when this resource is shutting down.
	remoteShutdownTimeout = 60 * time.Second

	remoteDefaultUser              = "root"           // default user
	remoteDefaultPort       uint16 = 22               // default port
	remoteDefaultTunnelAddr        = "127.0.0.1:2379" // default tunnel listen addr

	// RemoteStateRunning is the state which represents that the remote
	// process should be running.
	RemoteStateRunning = "running"

	// RemoteStateStopped is the state which represents that the remote
	// process should not be running.
	RemoteStateStopped = "stopped"
)

var (
	// remotePasswordStdinLock is a global lock around reading a password
	// from stdin, in the chance that you have multiple `remote` resources
	// trying to all read from the same time. You don't want to interleave
	// and accidentally send the wrong password to the wrong remote server!
	// XXX: We should have a feature that lets any resource read user input
	remotePasswordStdinLock chan struct{}
)

// RemoteRes provides facilities for remote execution over SSH as a resource.
// This lets us maintain the architectural robustness of our current design, and
// combine it with the ability to run it with an "agent-less"-like approach for
// bootstrapping, initial hand-off after provisioning, and remote configuration
// in environments with more restrictive installation requirements. The notable
// advantage of this being built as a resource, is that its use is programmatic
// which comes with fun possibilities. In general the following sequence is run:
//
// 1. connect to remote host
// 2. make a working directory
// 3. copy over the mgmt binary
// 4. optionally tunnel the etcd client connection back through SSH
// 5. run it!
// 6. on shutdown, optionally stop it (see the Transient field)
// 7. close tunnels
// 8. clean up
// 9. disconnect
//
// The spawned process connects as a client to our etcd world backend, and as a
// result it receives the currently active deploy the same way any other member
// of the cluster would, so there is no need to copy over any code or graphs.
// That cluster might be served by the host running this resource, or it might
// be a bigger cluster which we are ourselves only a client of. This resource
// uses the EndpointsWorld API to figure out which addresses to hand out.
//
// One advantage of this agent-less approach, is that while multiple of these
// remote mgmt transient agents are running, they can still exchange data and
// converge together without directly connecting, since they all tunnel through
// to the etcd server running on the first initiator or connect to a
// pre-existing etcd instance.
//
// Another advantage and key feature of this being a resource instead of as part
// of the core, is that we can make programmatic connections! In other words, we
// can first check if we're running mgmt on "host A", if not, ssh into it, and
// then if we're on "hostA ", connect to "host B", and then fan out to multiple
// hosts from there. Very useful for traversing firewalls or using your slow
// home link to connect into a jump server and then fan out quickly and also
// hierarchically from there.
//
// The unique `Name` field should represent a URL connect string to be used to
// reach the remote host. To override this and for details of the format, use
// the `Remote` field.
type RemoteRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Remote represents the remote host to connect to, specified in a
	// url.Parse compatible format such as: `ssh://user@host:22`. This value
	// will override the value specified in the `Name` variable. It can be
	// preferable to specify the value here if it's liable to change in a
	// non pertinent, equivalent way. Eg from: ssh://host:22 to: ssh://host.
	// This field _can_ also contain a password to use, but this is not
	// recommended for security reasons. If you wish to allow this, then you
	// must also set the AllowCleartextPassword field.
	Remote string `lang:"remote" yaml:"remote"`

	// Hostname is the unique hostname the remote process identifies itself
	// as to the rest of the cluster. If it is not specified, it will be
	// read from that host. Be attentive here, as the hostname a host
	// advertises itself as, may not be what your expect. Some advertise the
	// fqdn, and some do not.
	Hostname string `lang:"hostname" yaml:"hostname"`

	// State is the desired state of the remote process. It must be either
	// `running` or `stopped`. It defaults to `running`. If you specify
	// `stopped` it will actively *start* an SSH connection to ensure the
	// remote mgmt is actually stopped. Most of the time, instead of
	// enforcing a stop, you probably just want to do `running` with the
	// `transient` param set to true. When the transient resource returns,
	// you should not normally have a remaining remote process running.
	State string `lang:"state" yaml:"state"`

	// SSHID is the path to the SSH private key to use for authentication.
	// If you omit this (nil) then it will scan your ~/.ssh/ directory for
	// any usable private keys. If you specify the empty string, then key
	// based authentication will not be used. If you specify a specific
	// path, then that will be used. It will expand the ~/ and ~user/ style
	// path expansions.
	SSHID *string `lang:"ssh_id" yaml:"ssh_id"`

	// HostKey is the key part (which is already base64 encoded) from a
	// known_hosts file, representing the host we're connecting to. If this
	// is specified, then it is checked first, otherwise (and additionally)
	// we look for the host in your ~/.ssh/known_hosts file. You can find
	// this key by running the ssh-keyscan command.
	HostKey string `lang:"hostkey" yaml:"hostkey"`

	// Interactive specifies if we allow interactive auth prompting. It is
	// not recommended to use this unless you're okay with your program
	// blocking on password input at a possibly unexpected time. If you set
	// this, you might also want to consider setting InteractiveTimeout.
	// FIXME: "allowing interactivity" should be a meta param
	Interactive bool `lang:"interactive" yaml:"interactive"`

	// InteractiveTimeout specifies the number of seconds to wait on the
	// interactive password callback. Defaults to 0 which means infinite.
	InteractiveTimeout uint `lang:"interactive_timeout" yaml:"interactive_timeout"`

	// MaxPasswordTries is the number of interactive password tries allowed.
	// If value is <= 0, it will retry indefinitely.
	MaxPasswordTries int `lang:"max_password_tries" yaml:"max_password_tries"`

	// AllowCleartextPassword must be set to true if you want to allow a
	// cleartext password in the URL argument of either the Name or the
	// Remote field. This is obviously not secure and should only be used
	// for debugging, as this string will end up in logs and all over
	// memory.
	AllowCleartextPassword bool `lang:"allow_cleartext_password" yaml:"allow_cleartext_password"`

	// Caching specifies whether we should try and cache the binary remotely
	// to avoid having to re-copy it when needed. This also causes the
	// remote working directory to be preserved on shutdown. This defaults
	// to true.
	Caching bool `lang:"caching" yaml:"caching"`

	// Transient specifies whether the launched remote process is expected
	// to exit on its own, either because ConvergerTimeout is set or because
	// there's another termination mechanism such as if an exit resource is
	// present. When true, this resource holds the connection (and the
	// tunnel, if any) open and blocks until the child exits. An exit of the
	// parent will propagate through SSH and into this child.
	//
	// When false, the child is launched and runs continuously without being
	// bound to the parent. This must be true when Tunnelled is true, since
	// the tunnel must close when this resource shuts down. This defaults to
	// true.
	Transient bool `lang:"transient" yaml:"transient"`

	// Tunnelled specifies whether the network connections should be
	// tunnelled back through SSH or if the remote server should connect
	// directly. You should avoid this unless necessary as it adds overhead.
	// Use this if you are routing over the internet and don't have a flat,
	// local layer 2/3 network topology. Transient must be true because the
	// child can only reach the cluster through the tunnel, which closes
	// when this resource shuts down. Against my better judgement, this
	// defaults to true.
	Tunnelled bool `lang:"tunnelled" yaml:"tunnelled"`

	// TunnelAddr is the address that the reverse SSH tunnel listens on, on
	// the remote host, and which the remote process connects to as its
	// seed. It defaults to 127.0.0.1:2379 to match the etcd defaults. If
	// you specify a port of zero, then a free port is chosen at random by
	// the remote host. This is only used when Tunnelled is true.
	TunnelAddr string `lang:"tunnel_addr" yaml:"tunnel_addr"`

	// InstallDeps specifies whether we should run a generated script on the
	// remote host which installs the runtime dependencies of this binary
	// (such as the augeas and libvirt libraries) before we start the remote
	// process. This usually requires root permissions there. This does not
	// default to true, and you should avoid using it if possible, because
	// it is generally more elegant to ensure this kind of dependency is
	// installed during provisioning. That should be done automatically for
	// you if you are using any of the integrated mgmt provisioning tools.
	InstallDeps bool `lang:"install_deps" yaml:"install_deps"`

	// ConvergerTimeout specifies how many seconds of inactivity indicate
	// that the remote process has converged. When specified, the remote
	// process exits after convergence, and this resource waits for it
	// before reporting that it has converged. When unspecified, it inherits
	// the process-wide converger timeout only if this process is also
	// configured to exit after convergence. This requires Transient to be
	// true, since it makes the child exit on its own.
	ConvergerTimeout *int `lang:"converger_timeout" yaml:"converger_timeout"`

	world   engine.EndpointsWorld // world API for endpoint discovery
	urlInfo *urlInfo              // parsed remote connection info (use after Init)

	auth []ssh.AuthMethod // list of auth for ssh

	ssh      *ssh.Client  // ssh client object
	sftp     *sftp.Client // sftp client object
	listener net.Listener // ssh tunnel listener

	// tunnelURL is the seed URL that the remote process uses to get back
	// through the tunnel. It is set in Watch before the first event, and
	// only read afterwards in CheckApply, so it doesn't need any locking.
	tunnelURL string

	// sshInWatch specifies whether Watch holds the persistent SSH
	// connection that supervises the remote process and which CheckApply
	// reuses. This is only the case for a non-transient, running resource
	// that isn't polling. Otherwise CheckApply connects on-demand, and for
	// a transient child it also owns the tunnel and blocks until the child
	// exits.
	sshInWatch bool

	cachedHostname string // cached remote hostname if looked up over ssh

	interruptChan chan struct{}

	wg *sync.WaitGroup // sync group for tunnel go routines

	// conns tracks the open tunnel connections, so that tunnelClose can
	// close them all and unblock the copy goroutines. The mutex guards it.
	connsMutex *sync.Mutex
	conns      map[net.Conn]struct{}

	remoteUID    string // deterministic uid used in the remote working path
	remotewdSafe bool   // true after the remote path passes safety checks

	remotewd    string // path to remote working directory
	execpath    string // path to remote mgmt binary
	pidpath     string // path to remote pid file
	logpath     string // path to remote log file
	exitpath    string // path to remote exit status file
	depspath    string // path to remote deps install script
	childPrefix string // path to remote working prefix for the child
}

// Default returns some sensible defaults for this resource.
func (obj *RemoteRes) Default() engine.Res {
	return &RemoteRes{
		State:      RemoteStateRunning,
		Caching:    true,
		Transient:  true,
		Tunnelled:  true,
		TunnelAddr: remoteDefaultTunnelAddr,
	}
}

// remote returns the remote connection string to use. It defaults to using the
// Name variable, but falls back to the Remote parameter if specified.
func (obj *RemoteRes) remote() string {
	if obj.Remote != "" {
		return obj.Remote
	}
	return obj.Name()
}

// logf is a safe wrapper around the init Logf function, so that the helpers
// which use it can also be run in Validate, which runs before Init.
func (obj *RemoteRes) logf(format string, v ...interface{}) {
	if obj.init == nil {
		return
	}
	obj.init.Logf(format, v...)
}

// helper returns the common SSH utility helper struct.
func (obj *RemoteRes) helper() *sshutil.Helper {
	debug := false
	if obj.init != nil {
		debug = obj.init.Debug
	}
	return &sshutil.Helper{
		Debug: debug,
		Logf:  obj.logf,
	}
}

// urlParse parses a raw URL and returns a struct with safe values and defaults.
func (obj *RemoteRes) urlParse(rawURL string) (*urlInfo, error) {
	s := rawURL
	// the url.Parse function parses incorrectly without a scheme prefix :/
	// this also makes `host` and `host:22` equivalent to the ssh:// forms
	if !strings.Contains(s, "://") {
		s = "ssh://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't parse rawURL")
	}
	// TODO: consider supporting other remote execution schemes in the future
	if u.Scheme != "ssh" {
		return nil, fmt.Errorf("unknown remote scheme: %s", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("empty hostname")
	}

	port := remoteDefaultPort // default
	if rawPort := u.Port(); rawPort != "" {
		v, err := strconv.ParseUint(rawPort, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("can't parse port: %s", rawPort)
		}
		port = uint16(v)
	}

	user := remoteDefaultUser // default
	if x := u.User.Username(); x != "" {
		user = x
	}

	var pass *string
	if p, exists := u.User.Password(); exists {
		obj.logf("warning: plaintext password is being used")
		pass = &p
	}

	return &urlInfo{
		Scheme: u.Scheme,
		Host:   host,
		Port:   port,
		User:   user,
		Pass:   pass,
	}, nil
}

// convergerTimeout returns the explicitly configured timeout, or inherits the
// process-wide timeout when this process is itself configured to converge and
// exit. The latter makes this behaviour propagate through nested remotes.
func (obj *RemoteRes) convergerTimeout() *int {
	if !obj.Transient {
		return nil // only a transient child is expected to converge and exit
	}
	if obj.ConvergerTimeout != nil {
		return obj.ConvergerTimeout
	}
	if obj.init == nil || obj.init.Converger == nil {
		return nil
	}
	timeout := obj.init.Converger.TimeoutSeconds()
	if !obj.init.Converger.ExitsOnConverged() || timeout < 0 {
		return nil
	}
	return &timeout
}

// Validate if the params passed in are valid data.
func (obj *RemoteRes) Validate() error {
	if obj.remote() == "" {
		return fmt.Errorf("empty Remote")
	}
	urlInfo, err := obj.urlParse(obj.remote())
	if err != nil {
		return errwrap.Wrapf(err, "can't parse Remote")
	}
	if urlInfo.Pass != nil && !obj.AllowCleartextPassword {
		return fmt.Errorf("cleartext password found in URL")
	}

	if obj.State != RemoteStateRunning && obj.State != RemoteStateStopped {
		return fmt.Errorf("invalid State: %s", obj.State)
	}
	if obj.ConvergerTimeout != nil && *obj.ConvergerTimeout < 0 {
		return fmt.Errorf("the ConvergerTimeout cannot be negative")
	}
	maxInteractiveTimeout := uint64(math.MaxInt64 / int64(time.Second))
	if uint64(obj.InteractiveTimeout) > maxInteractiveTimeout {
		return fmt.Errorf("the InteractiveTimeout is too large")
	}

	if !obj.Transient && obj.Tunnelled {
		return fmt.Errorf("the Transient param must be true when Tunnelled is true")
	}

	// XXX: is this really needed?
	if obj.ConvergerTimeout != nil && !obj.Transient {
		return fmt.Errorf("the Transient param must be true when ConvergerTimeout is set")
	}

	if obj.Tunnelled && obj.TunnelAddr != "" {
		if _, _, err := net.SplitHostPort(obj.TunnelAddr); err != nil {
			return errwrap.Wrapf(err, "can't parse TunnelAddr")
		}
	}

	if obj.HostKey != "" {
		if _, err := obj.helper().KnownHostsKey(obj.HostKey); err != nil {
			return errwrap.Wrapf(err, "invalid known_hosts key")
		}
	}

	// TODO: is it okay that we run this once and then throw it away?
	// NOTE: The context is unused here, it propagates through for running
	// the auth result, but that doesn't happen here, result is discarded.
	if _, err := obj.getAuth(context.Background(), urlInfo); err != nil {
		return errwrap.Wrapf(err, "can't build auth methods")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *RemoteRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	var err error
	obj.urlInfo, err = obj.urlParse(obj.remote())
	if err != nil { // should not happen, previously done in Validate()
		return errwrap.Wrapf(err, "can't parse Remote")
	}

	world, ok := obj.init.World.(engine.EndpointsWorld)
	if !ok {
		return fmt.Errorf("world backend does not support the EndpointsWorld interface")
	}
	obj.world = world

	if obj.init.Program == "" {
		return fmt.Errorf("program name is empty")
	}

	// Watch supervises a persistent child over a held connection. A
	// transient child is owned by CheckApply (which blocks on it), a
	// stopped resource needs no held connection, and polling skips Watch
	// entirely.
	obj.sshInWatch = obj.MetaParams().Poll == 0 && obj.State == RemoteStateRunning && !obj.Transient

	// We use a deterministic remote working directory, so that we can find
	// the pid file of a previously started process again, and so that the
	// cached binary is found in the same place on subsequent runs. It is
	// deterministic on the resource identity, and not on which host runs
	// it, so that a replacement initiator converges on the same child.
	sum := sha256.Sum256([]byte(obj.Kind() + ":" + obj.Name()))
	obj.remoteUID = hex.EncodeToString(sum[:])[0:remoteUIDLength]

	obj.interruptChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}
	obj.connsMutex = &sync.Mutex{}
	obj.conns = make(map[net.Conn]struct{})

	return nil
}

// initRemotePaths securely creates the remote working directory and sets all of
// the paths beneath it. Non-cached state lives under the remote user's
// canonical home, instead of a predictable path in a shared temporary
// directory. Each path component that we own is verified not to be a symlink
// and is restricted to the SSH principal.
func (obj *RemoteRes) initRemotePaths(ctx context.Context, client remotePathClient) error {
	obj.remotewdSafe = false

	cachePrefix := path.Clean(remoteCachePrefix)
	base := path.Dir(cachePrefix)
	components := []string{path.Base(cachePrefix), obj.remoteUID}
	if !obj.Caching {
		wd, err := client.Getwd()
		if err != nil {
			return errwrap.Wrapf(err, "can't get remote home directory")
		}
		base = path.Clean(wd)
		components = append(strings.Split(path.Clean(remoteUserPrefix), "/"), obj.remoteUID)
	} else {
		info, err := client.Lstat(base)
		if os.IsNotExist(err) {
			if err := client.Mkdir(base); err != nil {
				return errwrap.Wrapf(err, "can't create remote cache base: %s", base)
			}
			info, err = client.Lstat(base)
		}
		if err != nil {
			return errwrap.Wrapf(err, "can't inspect remote cache base: %s", base)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("remote cache base is a symlink: %s", base)
		}
		if !info.IsDir() {
			return fmt.Errorf("remote cache base is not a directory: %s", base)
		}
	}

	remotewd, err := secureRemoteDir(ctx, client, base, components...)
	if err != nil {
		return errwrap.Wrapf(err, "can't initialize remote working directory")
	}
	obj.remotewd = remotewd
	obj.execpath = path.Join(obj.remotewd, obj.init.Program)
	obj.pidpath = path.Join(obj.remotewd, remotePidFilename)
	obj.logpath = path.Join(obj.remotewd, remoteLogFilename)
	obj.exitpath = path.Join(obj.remotewd, remoteExitFilename)
	obj.depspath = path.Join(obj.remotewd, remoteDepsFilename)
	obj.childPrefix = path.Join(obj.remotewd, remotePrefixDirname)
	obj.remotewdSafe = true

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. A
// transient child is stopped by CheckApply, which holds the connection open
// until the child exits or we shut down, so there is nothing to do here.
func (obj *RemoteRes) Cleanup() error {
	return nil
}

// connect is used for the common connect and disconnect code that runs in
// CheckApply. The returned func contains the disconnect routines to run after a
// successful connection. On error, this cleans up anything that it opened and
// returns a nil cleanup func. This function is not thread-safe, and we assume
// it's only run from one caller for resource.
func (obj *RemoteRes) connect(ctx context.Context) (func() error, error) {
	var err error
	obj.auth, err = obj.getAuth(ctx, obj.urlInfo)
	if err != nil { // should not happen, previously done in Validate()
		return nil, errwrap.Wrapf(err, "can't build auth methods")
	}

	// connect
	if err := obj.sshConnect(ctx); err != nil {
		obj.auth = nil
		return nil, err
	}

	if err := obj.sftpConnect(ctx); err != nil {
		return nil, err
	}
	if err := obj.sftpOperation(ctx, func() error {
		return obj.initRemotePaths(ctx, obj.sftp)
	}); err != nil {
		var errs error
		errs = errwrap.Append(errs, err)
		errs = errwrap.Append(errs, obj.sshClose())
		errs = errwrap.Append(errs, obj.sftpClose())
		return nil, errs
	}

	cleanup := func() error {
		var errs error
		// Close the raw transport first so SFTP shutdown cannot block
		// on a peer which has stopped reading.
		errs = errwrap.Append(errs, obj.sshClose())
		errs = errwrap.Append(errs, obj.sftpClose())
		return errs
	}
	return cleanup, nil
}

// getAuth returns a list of possible client authentication methods for SSH.
func (obj *RemoteRes) getAuth(ctx context.Context, urlInfo *urlInfo) ([]ssh.AuthMethod, error) {
	helper := obj.helper()
	auth := []ssh.AuthMethod{}

	if urlInfo.Pass != nil {
		if !obj.AllowCleartextPassword {
			return nil, fmt.Errorf("cleartext password found in URL")
		}

		auth = append(auth, ssh.Password(*urlInfo.Pass)) // secret!
	}

	// get ssh key auth if available
	if obj.SSHID == nil {
		// scan the default ssh dir for any usable private keys
		signers, err := helper.KeySigners()
		if err != nil {
			// Default key discovery is optional when another usable
			// auth method was explicitly configured.
			if len(auth) == 0 && !obj.Interactive {
				return nil, errwrap.Wrapf(err, "could not scan for SSH keys")
			}
			obj.logf("could not scan for optional SSH keys: %v", err)
		} else if len(signers) > 0 {
			auth = append(auth, ssh.PublicKeys(signers...)) // add all
		}

	} else if *obj.SSHID != "" {
		// expand strings of the form: ~james/.ssh/id_ed25519
		p, err := util.ExpandHome(*obj.SSHID)
		if err != nil {
			return nil, errwrap.Wrapf(err, "can't find home directory")
		}
		if p == "" {
			return nil, fmt.Errorf("empty path specified")
		}
		signer, err := helper.KeySigner(p)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not get SSH key auth")
		}
		auth = append(auth, ssh.PublicKeys(signer)) // add one
	}

	// TODO: do we really want to do this on a program that runs as daemon?
	// TODO: detect if running with stdin perhaps, and only do it then?
	if obj.Interactive {
		genCb := obj.passwordCallback(ctx, urlInfo.User, urlInfo.Host)
		cb := ssh.PasswordCallback(genCb)
		authMethod := ssh.RetryableAuthMethod(cb, obj.MaxPasswordTries)
		auth = append(auth, authMethod)
	}

	if len(auth) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	return auth, nil
}

// passwordCallback is a function which returns an interactive callback.
func (obj *RemoteRes) passwordCallback(ctx context.Context, user, host string) func() (string, error) {
	cb := func() (string, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		select {
		case remotePasswordStdinLock <- struct{}{}:
			defer func() { <-remotePasswordStdinLock }()

		case <-ctx.Done():
			return "", ctx.Err()
		}

		// Include a timeout if we promiscuously added interactive mode!
		if obj.InteractiveTimeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, time.Duration(obj.InteractiveTimeout)*time.Second) //nolint:gosec // G115: Validate bounds the conversion
			defer cancel()
		}

		obj.init.Logf("prompting for %s@%s password...", user, host)
		password, err := passwordUtil.ReadPasswordCtx(ctx) // []byte, error
		if err != nil {
			// returning an error will cancel the N retries on this
			return "", err
		}
		return string(password), nil
	}
	return cb
}

// sshConnect opens the SSH connection. Must not be called before Init.
func (obj *RemoteRes) sshConnect(ctx context.Context) error {
	addr := net.JoinHostPort(obj.urlInfo.Host, strconv.FormatUint(uint64(obj.urlInfo.Port), 10))

	obj.init.Logf("ssh connect %s", addr)

	helper := obj.helper()

	var pubKey ssh.PublicKey // known hosts key
	if obj.HostKey != "" {
		k, err := helper.KnownHostsKey(obj.HostKey)
		if err != nil { // should not happen, previously done in Validate()
			return errwrap.Wrapf(err, "invalid known_hosts key")
		}
		pubKey = k
	}

	algorithms := ssh.SupportedAlgorithms()
	preferredAlgoOrder := algorithms.HostKeys // the defaults
	// TODO: anyone got a problem with classic rsa?
	preferredAlgoOrder = append(preferredAlgoOrder, ssh.KeyAlgoRSA) // big keys are okay

	// The resource decides which known_hosts file to use, and we pass the
	// same value everywhere it's read so that host key algorithm
	// prioritization and verification can never disagree. Empty uses the
	// default location.
	// TODO: expose this as a resource parameter
	knownHosts := sshutil.DefaultKnownHostsPath

	// Learn which host key types we already trust for this host so that we
	// negotiate a key we can actually verify, the same way OpenSSH does.
	knownTypes, err := helper.KnownHostsAlgorithms(addr, knownHosts)
	if err != nil {
		obj.init.Logf("known_hosts algorithms: %v", err)
	}

	config := &ssh.ClientConfig{
		User: obj.urlInfo.User,
		// you must pass in at least one implementation of AuthMethod
		Auth: obj.auth,

		// Required! Verify against HostKey and/or ~/.ssh/known_hosts.
		HostKeyCallback: helper.HostKeyCallback(pubKey, knownHosts),

		// Prefer algorithms matching the configured host key and the
		// key types we already trust for this host in known_hosts.
		HostKeyAlgorithms: helper.PrioritizeHostKeyAlgorithms(preferredAlgoOrder, pubKey, knownTypes...),
	}

	obj.ssh, err = sshutil.DialSSHWithContext(ctx, "tcp", addr, config)
	if err != nil {
		return errwrap.Wrapf(err, "can't dial")
	}
	return nil
}

// sshClose closes the main SSH connection.
func (obj *RemoteRes) sshClose() error {
	//obj.init.Logf("ssh disconnect") // avoid being precise since unpaired
	obj.init.Logf("disconnect")
	// NOTE: We don't clear obj.urlInfo here (even though it may contain a
	// password) since we may need it to reconnect, and the same data is
	// kept in the public struct fields anyways.
	obj.auth = nil // built on demand in connect
	return obj.ssh.Close()
}

// sftpConnect opens the SFTP connection. Must not be called before sshConnect.
func (obj *RemoteRes) sftpConnect(ctx context.Context) error {
	obj.init.Logf("sftp connect...")

	type result struct {
		client *sftp.Client
		err    error
	}
	wg := &sync.WaitGroup{}
	resultChan := make(chan result, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		client, err := sftp.NewClient(obj.ssh)
		resultChan <- result{client: client, err: err}
	}()

	select {
	case result := <-resultChan:
		wg.Wait() // the result send happens just before the goroutine exits
		if result.err != nil {
			closeErr := obj.sshClose()
			if result.client != nil {
				closeErr = errwrap.Append(closeErr, result.client.Close())
			}
			return errwrap.Append(result.err, closeErr)
		}
		if err := ctx.Err(); err != nil {
			closeErr := obj.sshClose()
			closeErr = errwrap.Append(closeErr, result.client.Close())
			return errwrap.Append(err, closeErr)
		}
		obj.sftp = result.client
		return nil

	case <-ctx.Done():
		// This connection is not visible to Watch or CheckApply until
		// connect returns so closing it here only cancels this attempt.
		closeErr := obj.sshClose()
		result := <-resultChan // join the goroutine before returning
		wg.Wait()
		if result.client != nil {
			closeErr = errwrap.Append(closeErr, result.client.Close())
		}
		return errwrap.Append(ctx.Err(), closeErr)
	}
}

// sftpClose closes the main SFTP connection.
func (obj *RemoteRes) sftpClose() error {
	// This doesn't interleave correctly with ssh disconnect, so might as
	// well avoid logging the subtly confusing message.
	//obj.init.Logf("sftp disconnect")
	return obj.sftp.Close()
}

// sftpOperation bounds an SFTP operation with the caller's context. The SFTP
// client uses background contexts internally, so cancellation closes the SSH
// connection to interrupt any blocked request and force the resource to
// reconnect. The operation goroutine is owned and joined by this method.
func (obj *RemoteRes) sftpOperation(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	result := make(chan error, 1)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		result <- operation()
	}()

	select {
	case err := <-result:
		wg.Wait()
		return err

	case <-ctx.Done():
		// The SSH client's Close reaches the raw net.Conn, which
		// unblocks SFTP reads and writes even when the SFTP client's
		// Close is blocked.
		if obj.ssh != nil {
			_ = obj.ssh.Close()
		}
		<-result
		wg.Wait()
		return ctx.Err()
	}
}

// Watch is the primary listener for this resource and it outputs events. For a
// persistent (non-transient) running child, it holds the SSH connection,
// watches for endpoint changes in the cluster, and periodically checks that the
// remote process is still alive so that a crashed child gets restarted. For a
// transient or stopped resource there is nothing to supervise here, since
// CheckApply owns the connection (and, for a transient child, the tunnel), so
// we just send the initial event and wait for shutdown. Note that when the poll
// meta param is being used, this method doesn't run at all, and the connection
// instead happens on-demand in CheckApply.
func (obj *RemoteRes) Watch(ctx context.Context) error {
	if !obj.sshInWatch {
		if err := obj.init.Event(ctx); err != nil {
			return err
		}
		<-ctx.Done() // wait for the engine to signal shutdown
		return ctx.Err()
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait() // must happen after the deferred ssh close unblocks us

	cleanup, err := obj.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// watch for the ssh connection dying, so that we can report it and let
	// the engine retry us to reconnect...
	sshDied := make(chan error, 1) // buffered so the goroutine never blocks
	wg.Add(1)
	go func() {
		defer wg.Done()
		sshDied <- obj.ssh.Wait() // returns when the connection closes
	}()

	endpointsCtx, endpointsCancel := context.WithCancel(ctx)
	endpointsChan, err := obj.world.WatchEndpoints(endpointsCtx)
	if err != nil {
		endpointsCancel()
		return errwrap.Wrapf(err, "can't watch endpoints")
	}
	// This Watch invocation owns the endpoint subwatch context. Cancel it
	// on every exit so a retry cannot leave the previous watcher alive.
	defer endpointsCancel()

	// periodically check up on the remote process...
	ticker := time.NewTicker(remoteCheckInterval)
	defer ticker.Stop()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	var lastRunning *bool
	for {
		select {
		case err, ok := <-endpointsChan:
			if !ok {
				// world is shutting down, we should soon be too
				endpointsChan = nil
				continue
			}
			if err != nil {
				return errwrap.Wrapf(err, "endpoints watch failed")
			}
			if obj.init.Debug {
				obj.init.Logf("endpoints changed")
			}
			// fall through to send an event

		case err, ok := <-sshDied:
			select {
			case <-ctx.Done(): // it died because we're shutting down
				return ctx.Err()
			default:
			}
			if !ok || err == nil {
				err = fmt.Errorf("connection closed")
			}
			return errwrap.Wrapf(err, "ssh connection died")

		case <-ticker.C:
			running, err := obj.checkRunning(ctx)
			if err != nil {
				return err
			}
			changed := lastRunning != nil && *lastRunning != running
			lastRunning = &running
			if !changed {
				continue // nothing changed, skip this event
			}
			// fall through to send an event

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// localEndpoint returns the first current local endpoint to use as the target
// of a forwarded tunnel connection.
func (obj *RemoteRes) localEndpoint(ctx context.Context) (string, error) {
	endpoints, err := obj.world.LocalEndpoints(ctx)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't get local endpoints")
	}
	if len(endpoints) < 1 {
		return "", fmt.Errorf("need at least one local endpoint to tunnel")
	}
	// TODO: do something less arbitrary about which one we pick?
	dialURL := cleanURL(endpoints[0]) // arbitrarily pick the first one
	if dialURL == "" {
		return "", fmt.Errorf("can't parse local endpoint: %s", endpoints[0])
	}
	return dialURL, nil
}

// tunnel initiates the reverse SSH tunnel. It listens on the remote host at the
// TunnelAddr address, and forwards those connections to the first current local
// endpoint of our world backend. The remote process uses the remote side of
// this tunnel as its seed URL. The listener runs until tunnelClose is called.
func (obj *RemoteRes) tunnel(ctx context.Context) error {
	dialURL, err := obj.localEndpoint(ctx)
	if err != nil {
		return err
	}

	addr := obj.TunnelAddr
	if addr == "" {
		addr = remoteDefaultTunnelAddr
	}

	// reverse `ssh -R` listener to listen on the remote host
	obj.listener, err = obj.ssh.Listen("tcp", addr) // remote
	if err != nil {
		return errwrap.Wrapf(err, "can't listen on remote host")
	}
	// read back the address in case a port of zero picked a random one
	obj.tunnelURL = fmt.Sprintf("http://%s", obj.listener.Addr().String())
	obj.init.Logf("tunnel: %s -> %s", obj.tunnelURL, dialURL)

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		for {
			conn, err := obj.listener.Accept()
			if err != nil {
				if ctx.Err() == nil { // not a normal shutdown
					obj.init.Logf("tunnel listener: %v", err)
				}
				return
			}
			if err := obj.forward(ctx, conn); err != nil {
				// drop this connection, but keep on listening
				err = errwrap.Append(err, conn.Close())
				obj.init.Logf("%v", err)
			}
		}
	}()
	return nil
}

// forward proxies one accepted tunnel connection to the local endpoint. Both
// halves of the pair are tracked, and when either copy direction finishes, both
// ends get closed, which unblocks the copy running in the other way.
func (obj *RemoteRes) forward(ctx context.Context, remoteConn net.Conn) error {
	dialURL, err := obj.localEndpoint(ctx)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{}
	localConn, err := dialer.DialContext(ctx, "tcp", dialURL) // local
	if err != nil {
		return errwrap.Wrapf(err, "local dial error")
	}
	obj.connTrack(remoteConn, localConn)

	cp := func(writer, reader net.Conn) {
		// Copy copies from src to dst until either EOF is reached on
		// src or an error occurs. It returns the number of bytes copied
		// and the first error encountered while copying, if any.
		// Note: src/dst are backwards in golang as compared to cp, lol!
		n, err := io.Copy(writer, reader) // from reader to writer
		if err != nil && obj.init.Debug {
			// this is a normal error when connections get closed
			obj.init.Logf("io.Copy error: %s", err)
		}
		if obj.init.Debug {
			obj.init.Logf("io.Copy finished: %d", n)
		}
		// close both ends so the reverse copy direction unblocks too
		obj.connClose(writer, reader)
	}
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		cp(remoteConn, localConn)
	}()
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		cp(localConn, remoteConn)
	}()

	return nil // success!
}

// connTrack registers tunnel connections so that tunnelClose can close them.
func (obj *RemoteRes) connTrack(conns ...net.Conn) {
	obj.connsMutex.Lock()
	defer obj.connsMutex.Unlock()
	for _, c := range conns {
		obj.conns[c] = struct{}{}
	}
}

// connClose closes and forgets tunnel connections. It is safe to call it more
// than once with the same connections.
func (obj *RemoteRes) connClose(conns ...net.Conn) {
	obj.connsMutex.Lock()
	defer obj.connsMutex.Unlock()
	for _, c := range conns {
		if _, exists := obj.conns[c]; !exists {
			continue // it was already closed
		}
		delete(obj.conns, c)
		c.Close() //nolint:gosec // G104: close only unblocks the paired copy
	}
}

// connCloseAll closes any tunnel connections that are still open.
func (obj *RemoteRes) connCloseAll() {
	obj.connsMutex.Lock()
	defer obj.connsMutex.Unlock()
	for c := range obj.conns {
		delete(obj.conns, c)
		c.Close() //nolint:gosec // G104: best-effort close unblocks copy goroutines
	}
}

// tunnelClose causes any currently connected Tunnel to shutdown.
func (obj *RemoteRes) tunnelClose() error {
	err := obj.listener.Close()
	obj.connCloseAll() // unblock the copy goroutines
	obj.wg.Wait()      // wait for everyone to close
	obj.listener = nil
	return err
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *RemoteRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// When Watch isn't holding a connection for us (a transient or stopped
	// resource, or polling), CheckApply owns it for this call. For a
	// transient child, we block below until it exits so the tunnel and
	// connection stay up for exactly as long as the child needs them.
	if !obj.sshInWatch {
		cleanup, err := obj.connect(ctx)
		if err != nil {
			return false, err
		}
		defer cleanup()
	}

	running, err := obj.checkRunning(ctx)
	if err != nil {
		return false, err
	}

	if obj.State == RemoteStateStopped {
		if !running {
			return true, nil
		}
		if !apply {
			return false, nil
		}
		obj.init.Logf("stopping...")
		if err := obj.execExit(ctx); err != nil {
			return false, errwrap.Wrapf(err, "can't stop remote process")
		}
		return false, nil // we made a change
	}

	// obj.State == RemoteStateRunning

	// A running child needs restarting if its binary is stale, or if it is
	// tunnelled, because the tunnel it seeded through belonged to a
	// previous connection which is now gone and can't be re-attached to.
	if running {
		restart := obj.Tunnelled
		if !restart {
			same, err := obj.binaryMatches(ctx)
			if err != nil {
				return false, errwrap.Wrapf(err, "can't compare remote binary")
			}
			restart = !same
		}
		if restart {
			if !apply {
				return false, nil
			}
			obj.init.Logf("restarting remote process...")
			if err := obj.execExit(ctx); err != nil {
				return false, errwrap.Wrapf(err, "can't stop remote process")
			}
			running = false
		}
	}

	started := false
	if !running {
		if !apply {
			return false, nil
		}
		// The tunnel must be up before we launch, since the child seeds
		// through it. It's torn down when CheckApply returns, which for
		// a transient child is after it exits below.
		if obj.Tunnelled {
			if err := obj.tunnel(ctx); err != nil {
				return false, errwrap.Wrapf(err, "can't set up tunnel")
			}
			defer obj.tunnelClose()
		}
		if err := obj.start(ctx); err != nil {
			return false, err
		}
		started = true
	}

	if !obj.Transient {
		return !started, nil // false only if we just started it
	}

	// A transient child is expected to exit on its own (eg: converged-exit
	// or an in-graph exit resource) but if it's still running and we didn't
	// just start it, we aren't converged yet, but we mustn't block during a
	// check.
	if !started && !apply { // XXX: verify this check
		return false, nil
	}

	wg := &sync.WaitGroup{} // move higher if needed
	defer wg.Wait()

	// On the way out, remove the remote working directory unless we're
	// caching it. A crash (or a failed stop) is deliberately left in place
	// so a retry can reuse it and its log can be inspected.
	tidy := false
	defer func() {
		if !tidy || obj.Caching {
			return
		}
		cleanCtx := ctx       // by default we wait on the main ctx
		if ctx.Err() != nil { // if it already closed, wait a bit longer
			var cancel context.CancelFunc
			cleanCtx, cancel = context.WithTimeout(context.Background(), remoteShutdownTimeout)
			defer cancel()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				select {
				case <-obj.interruptChan:
				case <-cleanCtx.Done():
				}
			}()
		}
		if err := obj.sftpClean(cleanCtx); err != nil {
			obj.init.Logf("could not clean up: %v", err) // best effort
		}
	}()

	// Block until the child exits, or until we're shut down.
	waitErr := obj.waitConvergedExit(ctx)
	if waitErr != nil && ctx.Err() == nil {
		return false, waitErr // it exited badly, let the engine retry
	}
	if ctx.Err() != nil {
		// We're shutting down before the child exited, so stop it with
		// a fresh context while the connection is still open.
		stopCtx, cancel := context.WithTimeout(context.Background(), remoteShutdownTimeout)
		defer cancel()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			select {
			case <-obj.interruptChan:
			case <-stopCtx.Done():
			}
		}()
		obj.init.Logf("transient shutdown...")
		if err := obj.execExit(stopCtx); err != nil {
			return false, errwrap.Wrapf(err, "can't stop remote process")
		}
		tidy = true
		return false, ctx.Err()
	}
	tidy = true
	return true, nil // it exited cleanly, we're converged
}

// start pushes the binary, optionally installs its dependencies, and launches
// the remote process.
func (obj *RemoteRes) start(ctx context.Context) error {
	if err := obj.sftpPush(ctx); err != nil {
		return errwrap.Wrapf(err, "can't push binary")
	}
	if obj.InstallDeps {
		if err := obj.installDeps(ctx); err != nil {
			return errwrap.Wrapf(err, "can't install dependencies")
		}
	}
	seeds, err := obj.getSeeds(ctx)
	if err != nil {
		return errwrap.Wrapf(err, "can't determine seeds")
	}
	if err := obj.exec(ctx, seeds); err != nil {
		return errwrap.Wrapf(err, "can't start remote process")
	}
	return nil
}

// checkRunning returns whether the remote process that we spawned (or a
// previous incarnation of us spawned) is currently running. It checks that the
// pid found in our remote pid file is alive and is executing our exact remote
// binary, so that a recycled pid belonging to another process doesn't fool us.
func (obj *RemoteRes) checkRunning(ctx context.Context) (bool, error) {
	cmd := util.Code(fmt.Sprintf(`
		pid="$(cat %s 2>/dev/null)" || exit 1
		case "$pid" in ''|0|*[!0-9]*) exit 1;; esac
		expected="$(readlink -f %s 2>/dev/null)" || exit 1
		actual="$(readlink "/proc/$pid/exe" 2>/dev/null)" || exit 1
		test "$actual" = "$expected"
	`, shellescape(obj.pidpath), shellescape(obj.execpath)))
	if _, err := obj.simpleRun(ctx, cmd); err == nil {
		return true, nil
	} else if _, ok := err.(*ssh.ExitError); !ok {
		return false, errwrap.Wrapf(err, "can't check remote process")
	}
	return false, nil // clean exit error means it's not running
}

// binaryMatches returns whether the remote executable is identical to the
// executable which is running this resource.
func (obj *RemoteRes) binaryMatches(ctx context.Context) (bool, error) {
	selfpath, err := os.Executable()
	if err != nil {
		return false, errwrap.Wrapf(err, "can't get executable path")
	}
	return obj.sftpHash(ctx, selfpath, obj.execpath)
}

// getSeeds returns the seeds argument that the remote process should use to
// connect back to our cluster. When tunnelled, this is the remote side of our
// tunnel. Otherwise it's the list of advertised endpoints of the cluster, which
// the remote host needs to be able to route to.
func (obj *RemoteRes) getSeeds(ctx context.Context) (string, error) {
	if obj.Tunnelled {
		if obj.tunnelURL == "" {
			return "", fmt.Errorf("tunnel is not up")
		}
		return obj.tunnelURL, nil
	}

	m, err := obj.world.AdvertisedEndpoints(ctx)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't get advertised endpoints")
	}

	seen := make(map[string]struct{})
	seeds := []string{}
	for _, urls := range m {
		for _, s := range urls {
			u, err := url.Parse(s)
			if err != nil {
				continue // skip unparseable entries
			}
			// 0.0.0.0 and :: are unspecified wildcard addresses
			// used for listening, not concrete destinations the
			// remote host can dial.
			h := u.Hostname()
			if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
				continue
			}
			if _, exists := seen[s]; exists {
				continue // skip duplicates
			}
			seen[s] = struct{}{}
			seeds = append(seeds, s)
		}
	}
	sort.Strings(seeds) // determinism

	if len(seeds) == 0 {
		return "", fmt.Errorf("no routable endpoints found, use Tunnelled or advertise client urls")
	}
	return strings.Join(seeds, ","), nil
}

// sftpPush is a function for the sftp protocol to create the remote working
// directory and copy over the binary to run. If an identical binary is already
// present, then the copy is skipped.
func (obj *RemoteRes) sftpPush(ctx context.Context) error {
	if !obj.remotewdSafe {
		return fmt.Errorf("remote working directory is not safe")
	}

	// we run local operations first so that remote clean up is easier...
	selfpath, err := os.Executable()
	if err != nil {
		return errwrap.Wrapf(err, "can't get executable path")
	}
	obj.init.Logf("self executable is: %s", selfpath)

	obj.init.Logf("remote working directory is: %s", obj.remotewd)

	var same bool
	if err := obj.sftpOperation(ctx, func() error {
		_, err := obj.sftp.Stat(obj.execpath)
		return err
	}); err == nil {
		same, _ = obj.sftpHash(ctx, selfpath, obj.execpath) // ignore errors
	} else if ctx.Err() != nil {
		return ctx.Err()
	}
	if same {
		obj.init.Logf("skipping binary copy, file was cached")
	} else {
		// XXX: can we display some sort of progress indicator?
		obj.init.Logf("copying binary, please be patient...")
		if err := obj.sftpOperation(ctx, func() error {
			resultChan, cancelFunc := obj.simpleCopy(selfpath, obj.execpath)
			defer cancelFunc()
			select {
			case result, ok := <-resultChan:
				if !ok || result == nil {
					return fmt.Errorf("copy was cancelled")
				}
				if result.err != nil {
					return errwrap.Wrapf(result.err, "error copying binary")
				}
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}); err != nil {
			return err
		}
	}

	// make file executable; don't cache this in case it didn't ever happen
	// TODO: do we want the group or other bits set?
	if err := obj.sftpOperation(ctx, func() error {
		return obj.sftp.Chmod(obj.execpath, 0770)
	}); err != nil {
		return errwrap.Wrapf(err, "can't set file mode bits")
	}

	return nil
}

// sftpHash hashes a local file, and compares that hash to the result of a
// remote hashing command run on the second file path.
func (obj *RemoteRes) sftpHash(ctx context.Context, local, remote string) (bool, error) {
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
	if obj.init.Debug {
		obj.init.Logf("sha256sum: %s", sha256sum)
	}

	// We run a remote hashing command, instead of reading the file in over
	// the wire and hashing it ourselves, because assuming symmetric
	// bandwidth, that would defeat the point of caching it altogether!
	cmd := fmt.Sprintf("sha256sum %s", shellescape(remote))
	out, err := obj.simpleRun(ctx, cmd)
	if err != nil {
		return false, errwrap.Wrapf(err, "error running remote hash command")
	}

	s := strings.Split(out, " ") // sha256sum returns: hash + filename
	if len(s) > 0 && s[0] == sha256sum {
		return true, nil
	}
	return false, nil // files were different
}

// sftpClean removes the remote working directory and everything in it.
func (obj *RemoteRes) sftpClean(ctx context.Context) error {
	if err := obj.sftpOperation(ctx, func() error {
		p, err := obj.validateRemoteWorkDir(ctx, obj.sftp)
		if err != nil {
			return err
		}
		return obj.sftp.RemoveAll(p)
	}); err != nil {
		return err
	}
	obj.remotewdSafe = false
	return nil
}

// validateRemoteWorkDir checks that the final path is still the directory which
// initRemotePaths established. The private parent hierarchy prevents another
// account from replacing it after initialization.
func (obj *RemoteRes) validateRemoteWorkDir(ctx context.Context, client remotePathClient) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !obj.remotewdSafe {
		return "", fmt.Errorf("remote working directory is not safe")
	}
	p := strings.TrimSuffix(obj.remotewd, "/")
	info, err := client.Lstat(p)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't inspect remote working directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("remote working directory is a symlink: %s", p)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("remote working path is not a directory: %s", p)
	}
	return p, nil
}

// installDeps pushes a generated script to the remote host which installs the
// runtime dependencies of this binary, and then runs it.
func (obj *RemoteRes) installDeps(ctx context.Context) error {
	// Starting the binary is the cheapest and most accurate way to check if
	// all of its linked runtime dependencies are already present.
	cmd := fmt.Sprintf("%s --version", shellescape(obj.execpath))
	if _, err := obj.simpleRun(ctx, cmd); err == nil {
		return nil
	} else if _, ok := err.(*ssh.ExitError); !ok {
		return errwrap.Wrapf(err, "can't check remote binary dependencies")
	}

	script := obj.depsScript()

	if err := obj.sftpOperation(ctx, func() error {
		f, err := obj.sftp.Create(obj.depspath)
		if err != nil {
			return errwrap.Wrapf(err, "can't create deps script")
		}
		if _, err := f.Write(script); err != nil {
			writeErr := errwrap.Wrapf(err, "can't write deps script")
			return errwrap.Append(writeErr, f.Close())
		}
		if err := f.Close(); err != nil {
			return errwrap.Wrapf(err, "can't close deps script")
		}
		if err := obj.sftp.Chmod(obj.depspath, 0770); err != nil {
			return errwrap.Wrapf(err, "can't set file mode bits")
		}
		return nil
	}); err != nil {
		return err
	}

	obj.init.Logf("installing dependencies...")
	out, err := obj.simpleRun(ctx, shellescape(obj.depspath))
	if err != nil {
		return errwrap.Wrapf(err, "error installing dependencies: %s", out)
	}
	return nil
}

// depsScript generates the script which installs our runtime dependencies.
// TODO: should the dependency list be configurable?
func (obj *RemoteRes) depsScript() []byte {
	fedoraPackages, _ := distroUtil.ToBootstrapPackages(distroUtil.DistroFedora)
	debianPackages, _ := distroUtil.ToBootstrapPackages(distroUtil.DistroDebian)

	return []byte(util.Code(fmt.Sprintf(`
		#!/bin/sh
		# generated remote dependency installation script for mgmt
		if command -v dnf >/dev/null 2>&1; then
			exec dnf install -y %s
		fi
		if command -v yum >/dev/null 2>&1; then
			exec yum install -y %s
		fi
		if command -v apt-get >/dev/null 2>&1; then
			exec apt-get install -y %s
		fi
		echo "no supported package manager found" >&2
		exit 1
	`, strings.Join(fedoraPackages, " "), strings.Join(fedoraPackages, " "), strings.Join(debianPackages, " "))))
}

// exec starts the remote process. It is started in its own session so that it
// survives our disconnection, and its pid is stored in the remote pid file so
// that we can find it again later. The process output goes to a remote log.
func (obj *RemoteRes) exec(ctx context.Context, seeds string) error {
	if obj.execpath == "" {
		return fmt.Errorf("must have a binary path to execute")
	}

	h, err := obj.hostname(ctx)
	if err != nil {
		return err
	}

	args := []string{
		"--hostname", shellescape(h),
		"--seeds", shellescape(seeds),
		"--prefix", shellescape(obj.childPrefix),
	}
	convergerTimeout := obj.convergerTimeout()
	if convergerTimeout != nil {
		args = append(args,
			"--converger-timeout", shellescape(strconv.Itoa(*convergerTimeout)),
			"--converged-exit",
		)
	}
	// We don't pass --noop here. The active deploy carries its no-op
	// setting to every cluster member, including this child. No-op also
	// prevents the launcher from reaching this apply path when the child is
	// not running.

	// setsid starts mgmt in a new session and process group, separate from
	// the short-lived SSH shell. Combined with the redirected standard file
	// descriptors, this lets mgmt keep running after that SSH session
	// closes. We capture the pid from inside the new session, because
	// whether the setsid tool forks before exec'ing depends on whether it
	// is already a process group leader, which varies with the shell that
	// runs it, so the $! of the setsid process itself is not reliable.
	runCmd := fmt.Sprintf(
		"%s run %s empty > %s 2>&1 < /dev/null",
		shellescape(obj.execpath),
		strings.Join(args, " "),
		shellescape(obj.logpath),
	)
	wrapper := fmt.Sprintf(`%s & echo $! > %s`, runCmd, shellescape(obj.pidpath))
	prologue := ""
	if obj.Transient {
		// Record the child's exit status so that CheckApply's wait can
		// differentiate a clean exit from a crash so that we can know
		// what caused the child to exit.
		exitTmpPath := obj.exitpath + ".tmp"
		wrapper = fmt.Sprintf(
			`%s & pid=$!; echo "$pid" > %s; wait "$pid"; status=$?; echo "$status" > %s; mv %s %s`,
			runCmd,
			shellescape(obj.pidpath),
			shellescape(exitTmpPath),
			shellescape(exitTmpPath),
			shellescape(obj.exitpath),
		)
		prologue = fmt.Sprintf("echo running > %s; ", shellescape(obj.exitpath))
	}
	cmd := fmt.Sprintf(
		"%ssetsid sh -c %s > /dev/null 2>&1 < /dev/null &",
		prologue,
		shellescape(wrapper),
	)
	obj.init.Logf("running: %s", cmd)
	if out, err := obj.simpleRun(ctx, cmd); err != nil {
		return errwrap.Wrapf(err, "error starting remote process: %s", out)
	}
	if obj.Transient {
		return nil // CheckApply waits for and confirms a transient child
	}

	// give it a few moments, and verify it actually started successfully
	running := false
	for i := 0; i < remoteStartupRetries && !running; i++ {
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		var err error
		if running, err = obj.checkRunning(ctx); err != nil {
			return err
		}
	}
	if !running {
		// grab some log output to give the user a hint about why...
		cmd := fmt.Sprintf("tail -n 20 %s", shellescape(obj.logpath))
		out, _ := obj.simpleRun(ctx, cmd) // best effort
		return fmt.Errorf("remote process exited early: %s", out)
	}

	return nil
}

// convergedExitStatus returns the status of a remote converged-exit process.
// The exists result reports whether the status file exists, and exited reports
// whether it contains a final numeric exit status instead of "running".
func (obj *RemoteRes) convergedExitStatus(ctx context.Context) (int, bool, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, false, err
	}

	var out []byte
	exists := true
	if err := obj.sftpOperation(ctx, func() error {
		f, err := obj.sftp.Open(obj.exitpath)
		if err != nil {
			if os.IsNotExist(err) {
				exists = false
				return nil
			}
			return errwrap.Wrapf(err, "can't read remote exit status")
		}
		out, err = io.ReadAll(f)
		if err != nil {
			readErr := errwrap.Wrapf(err, "can't read remote exit status")
			return errwrap.Append(readErr, f.Close())
		}
		if err := f.Close(); err != nil {
			return errwrap.Wrapf(err, "can't close remote exit status")
		}
		return nil
	}); err != nil {
		return 0, exists, false, err
	}
	if !exists {
		return 0, false, false, nil
	}

	s := strings.TrimSpace(string(out))
	if s == "running" {
		return 0, true, false, nil
	}
	status, err := strconv.Atoi(s)
	if err != nil {
		return 0, true, false, errwrap.Wrapf(err, "invalid remote exit status: %s", s)
	}
	return status, true, true, nil
}

// waitConvergedExit waits for a remote converged-exit process to finish. It
// verifies that the process starts and remains alive until its wrapper records
// the final status, so a broken launcher does not leave CheckApply waiting
// forever on a "running" status file.
func (obj *RemoteRes) waitConvergedExit(ctx context.Context) error {
	started := false
	notRunningChecks := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	tailLog := func() string {
		cmd := fmt.Sprintf("tail -n 20 %s", shellescape(obj.logpath))
		out, _ := obj.simpleRun(ctx, cmd) // best effort
		return out
	}

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}

		status, _, exited, err := obj.convergedExitStatus(ctx)
		if err != nil {
			return err
		}
		if exited {
			if status == 0 {
				return nil
			}
			return fmt.Errorf("remote process exited with status %d: %s", status, tailLog())
		}

		running, err := obj.checkRunning(ctx)
		if err != nil {
			return err
		}
		if running {
			started = true
			notRunningChecks = 0
			continue
		}

		notRunningChecks++
		if notRunningChecks < remoteStartupRetries {
			// The wrapper may still be publishing the final status.
			continue
		}
		if started {
			return fmt.Errorf("remote process exited without reporting status: %s", tailLog())
		}
		return fmt.Errorf("remote process exited early: %s", tailLog())
	}
}

// execExit stops the remote process if it is running. It sends a SIGINT (^C)
// signal to it, waits for the process to exit, and if it doesn't exit promptly,
// it gets sent a force kill.
func (obj *RemoteRes) execExit(ctx context.Context) error {
	running, err := obj.checkRunning(ctx)
	if err != nil {
		return err
	}
	if !running {
		return nil // nothing to do
	}

	// SIGINT and wait for a graceful exit, escalating to SIGKILL after the
	// timeout. We re-check the executable identity before every signal and
	// while waiting, so that PID reuse can't make us signal another
	// process. A failed INT is tolerated, since the process may have
	// already exited between the identity check and the signal being sent.
	cmd := util.Code(fmt.Sprintf(`
		pid="$(cat %s)" || exit 1
		case "$pid" in ''|0|*[!0-9]*) exit 1;; esac
		expected="$(readlink -f %s 2>/dev/null)" || exit 1
		is_child() {
			actual="$(readlink "/proc/$pid/exe" 2>/dev/null)" || return 1
			test "$actual" = "$expected"
		}
		is_child || exit 0
		kill -INT "$pid" 2>/dev/null
		for i in $(seq %d); do
			is_child || exit 0
			sleep 1
		done
		is_child || exit 0
		kill -KILL "$pid" 2>/dev/null
		sleep 1
		! is_child
	`, shellescape(obj.pidpath), shellescape(obj.execpath), remoteExitTimeout))
	if out, err := obj.simpleRun(ctx, cmd); err != nil {
		return errwrap.Wrapf(err, "error stopping remote process: %s", out)
	}

	_ = obj.sftpOperation(ctx, func() error {
		return obj.sftp.Remove(obj.pidpath) // remove the stale pid file
	}) // best effort
	return nil
}

// simpleRun is a simple helper for running commands in new sessions. It returns
// the raw error from the session, so that callers can differentiate a command
// which exited non-zero (*ssh.ExitError) from a transport failure.
func (obj *RemoteRes) simpleRun(ctx context.Context, cmd string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// this calls NewSession and does everything in its own session :)
	session, err := obj.ssh.NewSession() // not the main session!
	if err != nil {
		return "", errwrap.Wrapf(err, "failed to create session")
	}
	defer session.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	type result struct {
		out []byte
		err error
	}
	wg := &sync.WaitGroup{}
	resultChan := make(chan result, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		out, err := session.CombinedOutput(cmd)
		resultChan <- result{out: out, err: err}
	}()

	select {
	case result := <-resultChan:
		wg.Wait() // the result send happens just before the goroutine exits
		return string(result.out), result.err

	case <-ctx.Done():
		// Closing this session interrupts CombinedOutput without
		// closing the persistent SSH client which is owned by Watch.
		session.Close()        //nolint:gosec // G104: preserve the context cancellation error
		result := <-resultChan // join the goroutine before returning
		wg.Wait()
		return string(result.out), ctx.Err()
	}
}

// hostname returns the hostname that is used as the UUID name for the remote
// host. If the Hostname param is specified, that is used, otherwise it is found
// automatically by querying the host.
// FIXME: listen for events if the remote hostname changes and update...
func (obj *RemoteRes) hostname(ctx context.Context) (string, error) {
	if obj.Hostname != "" {
		return obj.Hostname, nil
	}

	if obj.cachedHostname != "" {
		return obj.cachedHostname, nil
	}

	h, err := obj.simpleRun(ctx, "hostname") // run `hostname` command on remote!
	if err != nil {
		return "", errwrap.Wrapf(err, "error running hostname command")
	}
	h = strings.TrimSpace(h) // trim any trailing newline
	if h == "" {
		return "", fmt.Errorf("hostname is empty")
	}
	if strings.ContainsAny(h, " \t\n") {
		return "", fmt.Errorf("hostname contains whitespace: %s", h)
	}
	obj.cachedHostname = h // cache to avoid future expensive lookups
	return h, nil
}

// simpleCopy is a simple helper function that runs a local -> remote sftp copy.
// It requires that an existing sftp session is already open before running it.
// It returns a chan with the result status, and a cancel function. When a
// result is produced (whether on success or error) a result will be sent on the
// channel. If you wish to unblock this channel and stop waiting, you can run
// the returned cancel function. You should always run the cancel function to
// ensure things are cleaned up properly after use. It is safe to call multiple
// times. In particular, running cancel will cause an ongoing copy to unblock,
// which might be what you want if you don't want to wait for it to finish.
func (obj *RemoteRes) simpleCopy(src, dst string) (<-chan *simpleCopyResult, func()) {
	wg := &sync.WaitGroup{}
	closeChan := make(chan struct{})
	once := &sync.Once{}
	mutex := &sync.Mutex{}

	f1Close := func() {} // noop for now
	f2Close := func() {}
	closed := false

	resultChan := make(chan *simpleCopyResult)
	cancelFunc := func() {
		defer wg.Wait()
		defer once.Do(func() {
			close(closeChan)
		})

		// close any copy operations that are in progress...
		// TODO: we probably only need to shutdown one of them, but
		// which one should we shutdown? close both for now...
		mutex.Lock()
		f1Close()
		f2Close()
		closed = true
		mutex.Unlock()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(resultChan)

		// open a handle to read the file
		mutex.Lock()
		if closed {
			return
		}
		f1, err1 := os.Open(src)
		f1Close = func() { // update the close function as is needed
			if err1 != nil {
				return // skip close if failed to open
			}
			// TODO: add a sync.Once if a double Close is unsafe
			f1.Close() //nolint:gosec // G104: cancellation close only unblocks io.Copy
		}
		mutex.Unlock()
		if err1 != nil {
			select {
			case resultChan <- &simpleCopyResult{n: -1, err: err1}:
			case <-closeChan:
			}
			return
		}
		defer f1Close()

		// open a handle to create the file
		mutex.Lock()
		if closed {
			return
		}
		f2, err2 := obj.sftp.Create(dst)
		f2Close = func() {
			if err2 != nil {
				return // skip close if failed to open
			}
			// TODO: add a sync.Once if a double Close is unsafe
			f2.Close() //nolint:gosec // G104: cancellation close only unblocks io.Copy
		}
		mutex.Unlock()
		if err2 != nil {
			select {
			case resultChan <- &simpleCopyResult{n: -1, err: err2}:
			case <-closeChan:
			}
			return
		}
		defer f2Close()

		// the actual copy, this might take time...
		// this can get unblocked by closing the file descriptors
		n, err := io.Copy(f2, f1) // dst, src -> n, error
		if err != nil {
			select {
			case resultChan <- &simpleCopyResult{n: n, err: errwrap.Wrapf(err, "can't copy to remote path")}:
			case <-closeChan:
			}
			return
		}
		if n <= 0 {
			select {
			case resultChan <- &simpleCopyResult{n: n, err: fmt.Errorf("zero bytes copied")}:
			case <-closeChan:
			}
			return
		}
		select {
		case resultChan <- &simpleCopyResult{n: n}:
		case <-closeChan:
		}
	}()
	return resultChan, cancelFunc
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *RemoteRes) Cmp(r engine.Res) error {
	// we can only compare RemoteRes to others of the same resource kind
	res, ok := r.(*RemoteRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	// parse before Cmp so that "host1" vs. "host1:22" are equivalent
	u1, err := obj.urlParse(obj.remote())
	if err != nil { // should not happen, previously done in Validate()
		return errwrap.Wrapf(err, "can't parse Remote param")
	}
	u2, err := res.urlParse(res.remote())
	if err != nil { // should not happen, previously done in Validate()
		return errwrap.Wrapf(err, "can't parse Remote param")
	}
	if err := u1.Cmp(u2); err != nil {
		return errwrap.Wrapf(err, "the Remote param differs")
	}

	if obj.Hostname != res.Hostname {
		return fmt.Errorf("the Hostname param differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State param differs")
	}

	if (obj.SSHID == nil) != (res.SSHID == nil) { // xor
		return fmt.Errorf("the SSHID param differs")
	}
	if obj.SSHID != nil && res.SSHID != nil {
		if *obj.SSHID != *res.SSHID {
			return fmt.Errorf("the SSHID param differs")
		}
	}
	if obj.HostKey != res.HostKey {
		return fmt.Errorf("the HostKey param differs")
	}

	if obj.Interactive != res.Interactive {
		return fmt.Errorf("the Interactive param differs")
	}
	if obj.InteractiveTimeout != res.InteractiveTimeout {
		return fmt.Errorf("the InteractiveTimeout param differs")
	}
	if obj.MaxPasswordTries != res.MaxPasswordTries {
		return fmt.Errorf("the MaxPasswordTries param differs")
	}
	if obj.AllowCleartextPassword != res.AllowCleartextPassword {
		return fmt.Errorf("the AllowCleartextPassword param differs")
	}

	if obj.Caching != res.Caching {
		return fmt.Errorf("the Caching param differs")
	}
	if obj.Transient != res.Transient {
		return fmt.Errorf("the Transient param differs")
	}
	if obj.Tunnelled != res.Tunnelled {
		return fmt.Errorf("the Tunnelled param differs")
	}
	if obj.TunnelAddr != res.TunnelAddr {
		return fmt.Errorf("the TunnelAddr param differs")
	}

	if obj.InstallDeps != res.InstallDeps {
		return fmt.Errorf("the InstallDeps param differs")
	}
	if (obj.ConvergerTimeout == nil) != (res.ConvergerTimeout == nil) { // xor
		return fmt.Errorf("the ConvergerTimeout param differs")
	}
	if obj.ConvergerTimeout != nil && res.ConvergerTimeout != nil {
		if *obj.ConvergerTimeout != *res.ConvergerTimeout {
			return fmt.Errorf("the ConvergerTimeout param differs")
		}
	}

	return nil
}

// Interrupt is called to ask the execution of this resource to end early.
func (obj *RemoteRes) Interrupt() error {
	close(obj.interruptChan)
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *RemoteRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes RemoteRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*RemoteRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to RemoteRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = RemoteRes(raw) // restore from indirection with type conversion!
	return nil
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *RemoteRes) UIDs() []engine.ResUID {
	x := &RemoteUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// RemoteUID is the UID struct for RemoteRes.
type RemoteUID struct {
	engine.BaseUID
	name string
}

// simpleCopyResult is the result of a completed SFTP copy.
type simpleCopyResult struct {
	n   int64
	err error
}

// remotePathClient contains the SFTP operations needed to securely initialize a
// remote working directory.
type remotePathClient interface {
	Getwd() (string, error)
	Lstat(string) (os.FileInfo, error)
	Mkdir(string) error
	Chmod(string, os.FileMode) error
}

// urlInfo is the result of parsing a URL, with appropriate, safe, defaults.
type urlInfo struct {
	Scheme string // eg: ssh
	Host   string
	Port   uint16 // eg: 22
	User   string
	Pass   *string // value if set
}

// Cmp compares this urlInfo to another.
func (obj *urlInfo) Cmp(urlInfo *urlInfo) error {
	if obj.Scheme != urlInfo.Scheme {
		return fmt.Errorf("the Scheme differs")
	}
	if obj.Host != urlInfo.Host {
		return fmt.Errorf("the Host differs")
	}
	if obj.Port != urlInfo.Port {
		return fmt.Errorf("the Port differs")
	}
	if obj.User != urlInfo.User {
		return fmt.Errorf("the User differs")
	}

	if (obj.Pass == nil) != (urlInfo.Pass == nil) { // xor
		return fmt.Errorf("the Pass state differs")
	}
	if obj.Pass != nil && urlInfo.Pass != nil {
		if *obj.Pass != *urlInfo.Pass { // compare the strings
			return fmt.Errorf("the Pass differs")
		}
	}

	return nil
}

// shellescape quotes a string so that it is safe to interpolate into a shell
// command that we run on the remote host.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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

// secureRemoteDir creates a private directory hierarchy beneath a trusted
// absolute base. Existing path components must be real directories, never
// symlinks. The base itself must not be writable by group or other users, so
// that another account can't replace the private hierarchy after validation.
func secureRemoteDir(ctx context.Context, client remotePathClient, base string, components ...string) (string, error) {
	base = path.Clean(base)
	if !path.IsAbs(base) {
		return "", fmt.Errorf("remote base path is not absolute: %s", base)
	}

	info, err := client.Lstat(base)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't inspect remote base directory: %s", base)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("remote base directory is a symlink: %s", base)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("remote base path is not a directory: %s", base)
	}
	if info.Mode().Perm()&0022 != 0 {
		return "", fmt.Errorf("remote base directory is writable by another user: %s", base)
	}

	p := base
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if component == "" || component == "." || component == ".." || strings.Contains(component, "/") {
			return "", fmt.Errorf("invalid remote path component: %s", component)
		}

		p = path.Join(p, component)
		info, err := client.Lstat(p)
		if os.IsNotExist(err) {
			if err := client.Mkdir(p); err != nil {
				return "", errwrap.Wrapf(err, "can't create remote directory: %s", p)
			}
			info, err = client.Lstat(p)
		}
		if err != nil {
			return "", errwrap.Wrapf(err, "can't inspect remote directory: %s", p)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("remote directory is a symlink: %s", p)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("remote path is not a directory: %s", p)
		}
		if err := client.Chmod(p, 0700); err != nil {
			return "", errwrap.Wrapf(err, "can't secure remote directory: %s", p)
		}
	}

	return p + "/", nil
}
