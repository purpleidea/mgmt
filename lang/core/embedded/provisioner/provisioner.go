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

//go:build !noembedded_provisioner

package coreprovisioner

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"strings"

	"github.com/purpleidea/mgmt/cli"
	"github.com/purpleidea/mgmt/entry"
	"github.com/purpleidea/mgmt/lang/embedded"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification/fastsolver"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/password"
)

const (
	// ModuleName is the prefix given to all the functions in this module.
	ModuleName = "provisioner"

	// Version is the version number of this module.
	Version = "v0.0.1"

	// Frontend is the name of the GAPI to run.
	Frontend = "lang"
)

// NOTE: Grouped like shown is better, but you _can_ do it separately...
// Remember to add more patterns for nested child folders!
//
//go:embed metadata.yaml main.mcl files/*
var fs embed.FS // grouped is better

// NOTE: Separate as it's part of a different API and has a different function.
//
//go:embed top.mcl
var top []byte

// localArgs is our struct which is used to modify the CLI parser.
type localArgs struct {
	// Interface is the local ethernet interface to provision from. It will
	// be determined automatically if not specified.
	Interface *string `arg:"--interface" help:"local ethernet interface to provision from" func:"cli_interface"` // eg: enp0s31f6 or eth0

	// Network is the ip network with cidr that we want to use for the
	// provisioner.
	Network *string `arg:"--network" help:"network with cidr to use" func:"cli_network"` // eg: 192.168.42.0/24

	// Router is the ip for this machine with included cidr. It must exist
	// in the chosen network.
	Router *string `arg:"--router" help:"router ip for this machine with cidr" func:"cli_router"` // eg: 192.168.42.1/24

	// DNS are the list of upstream DNS servers to use during this process.
	DNS []string `arg:"--dns,separate" help:"upstream dns servers to use" func:"cli_dns"` // eg: ["8.8.8.8", "1.1.1.1"]

	// Proxy specifies that we use proxy dhcp on an existing network, where
	// another dhcp server gives out the leases, and we only send the
	// network boot program. The network must be empty, and our ip is the
	// address of the interface, unless the router is specified. With a mac
	// address, the ip is optional, and if it's set, then the host is only
	// provisioned if the other dhcp server gave it that ip. Without a mac
	// address, you must specify either ProxyRange or ProxyForce.
	Proxy bool `arg:"--proxy" help:"use proxy dhcp with an existing dhcp server" func:"cli_proxy"`

	// ProxyRange is the network with cidr that the other dhcp server must
	// give a uefi host an ip in, for it to be provisioned with proxy dhcp.
	// This is only used without a mac address.
	ProxyRange string `arg:"--proxy-range" help:"with --proxy, only provision hosts that get an ip in this network" func:"cli_proxy_range"` // eg: 192.168.1.200/29

	// ProxyForce allows us to provision any uefi host on the whole existing
	// network with proxy dhcp. This will erase the disk of every host which
	// tries to boot from the network, so be careful! This is only used
	// without a mac address.
	ProxyForce bool `arg:"--proxy-force" help:"with --proxy, provision any host on the whole network" func:"-"` // cli only

	// Prefix is a directory to store some provisioner specific state such
	// as cached distro packages. It can be safely deleted. If you don't
	// specify this value, one will be chosen automatically.
	Prefix *string `arg:"--prefix" help:"local XDG_CACHE_HOME path" func:"cli_prefix"` // eg: ~/.cache/mgmt/provisioner/

	// Firewalld will automatically open the required ports for being a
	// provisioner. By default this is enabled, but it can be disabled if
	// you use a different firewall system.
	Firewalld bool `arg:"--firewalld" default:"true" help:"should we open firewalld on our provisioner" func:"cli_firewalld"`

	// repo

	// Distro specifies the distribution to use. Currently `fedora` and
	// `debian` are supported.
	Distro string `arg:"--distro" default:"fedora" help:"distribution to use" func:"cli_distro"`

	// Version is the distribution version. This is a string, not an int.
	// For debian, this is the major version number, not the codename.
	Version string `arg:"--dversion" help:"distribution version" func:"cli_version"` // eg: "38" or "13"

	// Arch is the distro architecture to use. Only x86_64 and aarch64 are
	// currently supported. Patches welcome. We use these names for every
	// distro, even if the distro itself uses a different name for it.
	Arch string `arg:"--arch" default:"x86_64" help:"architecture to use" func:"cli_arch"`

	// Flavour describes a flavour of distribution to provision. The value
	// and what it does is highly dependent on the distro you specified. The
	// default is set automatically depending on your distro variable. Both
	// fedora and debian support `Workstation` and `Server`.
	Flavour *string `arg:"--flavour" help:"flavour of distribution" func:"cli_flavour"` // eg: "Workstation" or "Server"

	// Mirror is the mirror to provision from. Pick one that supports both
	// rsync AND https if you want the most capable provisioner features. A
	// list for fedora is at: https://admin.fedoraproject.org/mirrormanager/
	// eg: https://mirror.csclub.uwaterloo.ca/fedora/ for example. This
	// points to: https://download.fedoraproject.org/pub/fedora/linux/ by
	// default if unspecified, because it will automatically translate to a
	// local mirror near you. If you have issues with the installer exiting,
	// and 404's or similar issues happening, specify a mirror here. Some
	// users can get inconsistent repository views if the particular
	// location they connect from picks different mirrors. For debian, this
	// defaults to: https://deb.debian.org/debian/ and the security updates
	// always come from: https://deb.debian.org/debian-security/ instead.
	// TODO: Do we need to do a special step of checking the signature of
	// the initrd or vmlinuz or the install.img file we first load?
	Mirror string `arg:"--mirror" help:"https mirror for proxy provisioning" func:"cli_mirror"`

	// Rsync is the rsync to sync from. Pick one that supports both rsync
	// AND https if you want the most capable provisioner features. A list
	// for fedora is at: https://admin.fedoraproject.org/mirrormanager/ eg:
	// rsync://mirror.csclub.uwaterloo.ca/fedora-enchilada/linux/releases/
	// for examples. Be advised that this option will likely pull down over
	// 100GiB per os/arch/version combination. Consider only using `mirror`.
	Rsync string `arg:"--rsync" help:"rsync mirror for full synchronization" func:"cli_rsync"`

	// host

	// Mac is the mac address of the host that we'd like to provision. If
	// you omit this, than we will attempt to provision any computer which
	// asks.
	Mac *net.HardwareAddr `arg:"--mac" help:"mac address to provision" func:"cli_mac"`

	// IP is the address of the host to provision. It must include the /cidr
	// and be contained in the above network that was specified. With proxy
	// dhcp, it's optional, and it's the ip that the other dhcp server must
	// give to the host for it to be provisioned.
	IP *string `arg:"--ip" help:"ip address with cidr of the host to provision" func:"cli_ip"` // eg: "192.168.42.114/24"

	// Bios should be set true if you want to provision legacy machines.
	Bios bool `arg:"--bios" help:"should we use bios or uefi" func:"cli_bios"`

	// IPXE should be true if you want the host to boot through ipxe, which
	// then loads everything else over http, which is much faster than tftp.
	// This is needed for debian 12 with proxy dhcp. Stock ipxe isn't
	// signed, so secure boot must be disabled on the host.
	IPXE bool `arg:"--ipxe" help:"boot through ipxe, which needs secure boot disabled" func:"cli_ipxe"`

	// Password is an `openssl passwd -6` salted password. If you don't
	// specify this, you will be prompted to enter the actual unhashed
	// password, and it will be salted and hashed for you.
	Password *string `arg:"--password" help:"the 'openssl passwd -6' salted password" func:"-"` // skip auto func gen

	// Part is the magic partitioning scheme to use. At the moment you can
	// either specify `plain` or `btrfs`. The default empty string will use
	// the `plain` scheme.
	Part string `arg:"--part" help:"partitioning scheme, read manual for details" func:"cli_part"` // eg: empty string for plain

	// LUKS specifies that we're encrypting the volumes, and using an empty
	// password! This is done so that you can non-interactively boot for the
	// first time and run your automation tool. You should set the password
	// after first boot, either manually or with automation tooling!
	LUKS bool `arg:"--luks" default:"true" help:"add an empty LUKS password (change it after first boot)" func:"cli_luks"`

	// Packages are a list of additional distro packages to install. It's up
	// to the user to make sure they exist and don't conflict with each
	// other or the base installation packages.
	Packages []string `arg:"--packages,separate" help:"list of additional distro packages to install" func:"cli_packages"`

	// HandoffExec specifies that we want to handoff to this machine by
	// running a single exec on firstboot. Usually an `mgmt run` command.
	HandoffExec string `arg:"--handoff-exec" help:"exec command to run on firstboot" func:"cli_handoff_exec"` // eg: mgmt run ...

	// HandoffCode specifies that we want to handoff to this machine with a
	// static code deploy bolus. This is useful for isolated, one-time runs.
	HandoffCode string `arg:"--handoff-code" help:"code dir to handoff to host" func:"cli_handoff_code"` // eg: /etc/mgmt/

	// HandoffModulePath is the code handoff module path dir that we use.
	// It's an absolute path on this local machine. (The provisioner!)
	HandoffModulePath string `arg:"--handoff-module-path" help:"module path dir to use for handoff" func:"cli_handoff_module_path"` // eg: /etc/mgmt/modules/

	// HandoffHostname specifies that we want to handoff a hostname to set
	// on this machine. This is useful to make initial code handoff easier.
	HandoffHostname string `arg:"--handoff-hostname" help:"hostname to handoff to host" func:"cli_handoff_hostname"` // eg: server1

	// BmcURI specifies the BMC connect string we want to use for this host.
	// This is a giant driver://user:password@host:port/whatever URL...
	BmcURI string `arg:"--bmc-uri" help:"bmc connect string to use for this host" func:"cli_bmc_uri"`

	// Check tells the compiler to stop after the check stage. This is used
	// for testing.
	Check bool `arg:"--check" help:"stop after check stage"`
}

// provisioner is our cli parser translator and general frontend object.
type provisioner struct {
	init *entry.Init

	// localArgs is a stored reference to the localArgs config struct that
	// is used in the API of the command line parsing library. After it adds
	// our flags and executes it, the resultant parsed values will be made
	// available here where we've stored a copy.
	localArgs *localArgs

	// salted password
	password string
}

// Init implements the Initable interface which lets us collect some data and
// handles from our caller.
func (obj *provisioner) Init(init *entry.Init) error {
	obj.init = init // store some data/handles including logf

	return nil
}

// Customize implements the Customizable interface which lets us manipulate the
// CLI.
func (obj *provisioner) Customize(a interface{}) (*cli.RunArgs, error) {
	//if obj.init.Debug {
	//	obj.init.Logf("got: %T: %+v\n", a, a) // parent Args
	//}
	ctx := context.TODO()

	runArgs, ok := a.(*cli.RunArgs)
	if !ok {
		// programming error?
		return nil, fmt.Errorf("received invalid struct of type: %T", a)
	}

	libConfig := runArgs.Config
	//var name string
	var args interface{}
	if cmd := runArgs.RunLang; cmd != nil {
		//name = cliUtil.LookupSubcommand(obj, cmd) // "lang" // reflect.Value.Interface: cannot return value obtained from unexported field or method
		args = cmd
	}
	//if name == "" {
	//	return nil, fmt.Errorf("no frontend activated")
	//}
	if args == nil {
		return nil, fmt.Errorf("no frontend activated")
	}
	//if obj.init.Debug {
	//	obj.init.Logf("got: %T: %+v\n", args, args) // parent Args
	//}

	if obj.localArgs == nil {
		// programming error
		return nil, fmt.Errorf("could not convert/access our struct")
	}
	//localArgs := *obj.localArgs // optional

	// Add custom defaults, and improve some as well.

	if s := obj.localArgs.Interface; s == nil || *obj.localArgs.Interface == "" {
		devices, err := util.GetPhysicalEthernetDevices()
		if err != nil {
			return nil, err
		}
		if i := len(devices); i == 0 || i > 1 {
			return nil, fmt.Errorf("couldn't guess ethernet device, got %d", i)
		}
		dev := devices[0]
		obj.localArgs.Interface = &dev
	}
	obj.init.Logf("interface: %+v", *obj.localArgs.Interface)

	if s := obj.localArgs.Network; s == nil {
		x := "192.168.42.0/24"
		if obj.localArgs.Proxy {
			x = "" // proxy dhcp is for an existing network
		}
		obj.localArgs.Network = &x
	}
	// An empty network means that we're on an existing network, where we
	// don't do any dhcp ourselves. Our router ip is then our address on it,
	// which comes from the interface, unless it's specified.
	existing := *obj.localArgs.Network == ""

	if s := obj.localArgs.Router; s == nil {
		x := "192.168.42.1/24"
		if existing {
			x = "" // use the address of the interface
		}
		obj.localArgs.Router = &x
	}
	if s := obj.localArgs.IP; s == nil {
		x := "192.168.42.13/24"
		if existing {
			x = "" // we don't give out any leases
		}
		obj.localArgs.IP = &x
	}

	if err := obj.checkProxy(existing); err != nil {
		return nil, err
	}

	if existing {
		for _, s := range []string{*obj.localArgs.Router, *obj.localArgs.IP} {
			if s == "" {
				continue
			}
			if _, _, err := net.ParseCIDR(s); err != nil {
				return nil, err
			}
		}
	} else {
		_, netIPnet, err := net.ParseCIDR(*obj.localArgs.Network)
		if err != nil {
			return nil, err
		}
		routerIP, _, err := net.ParseCIDR(*obj.localArgs.Router)
		if err != nil {
			return nil, err
		}
		if !netIPnet.Contains(routerIP) {
			return nil, fmt.Errorf("network %s does not contain %s", *obj.localArgs.Network, *obj.localArgs.Router)
		}
		hostIP, _, err := net.ParseCIDR(*obj.localArgs.IP)
		if err != nil {
			return nil, err
		}
		// The ip is only used for a host, which needs a mac address.
		if obj.localArgs.Mac != nil && !netIPnet.Contains(hostIP) {
			return nil, fmt.Errorf("network %s does not contain %s", *obj.localArgs.Network, *obj.localArgs.IP)
		}
	}

	// TODO: add more validation

	if p := obj.localArgs.Prefix; p != nil {
		if strings.HasPrefix(*p, "~") {
			expanded, err := util.ExpandHome(*p)
			if err != nil {
				return nil, err
			}
			obj.localArgs.Prefix = &expanded
		}
	}
	if obj.localArgs.Prefix == nil || *obj.localArgs.Prefix == "" { // pick a default
		user, err := user.Current()
		if err != nil {
			return nil, errwrap.Wrapf(err, "can't get current user")
		}

		xdg := os.Getenv("XDG_CACHE_HOME")
		// Ensure there is a / at the end of the directory path.
		if xdg != "" && !strings.HasSuffix(xdg, "/") {
			xdg = xdg + "/"
		}
		if xdg == "" && user.HomeDir != "" {
			xdg = fmt.Sprintf("%s/.cache/%s/", user.HomeDir, obj.init.Data.Program)
		}

		xdg += fmt.Sprintf("%s/", ModuleName) // pick a dir for this tool
		obj.localArgs.Prefix = &xdg
	}
	obj.init.Logf("cache prefix: %+v", *obj.localArgs.Prefix)

	if obj.localArgs.Mac == nil {
		mac := net.HardwareAddr([]byte{}) // will print empty string
		obj.localArgs.Mac = &mac
	}

	if obj.localArgs.Distro == "" {
		return nil, fmt.Errorf("distro was not specified")
	}
	if d := obj.localArgs.Distro; d != "fedora" && d != "debian" { // TODO: add other distros!
		return nil, fmt.Errorf("only fedora and debian are currently supported")
	}

	if obj.localArgs.Distro == "fedora" && obj.localArgs.Version == "" {
		version, err := util.LatestFedoraVersion(ctx, obj.localArgs.Arch) // get a default for fedora
		if err != nil {
			return nil, err
		}
		obj.localArgs.Version = version
	}
	if obj.localArgs.Distro == "debian" && obj.localArgs.Version == "" {
		version, err := util.LatestDebianVersion(ctx) // get a default for debian
		if err != nil {
			return nil, err
		}
		obj.localArgs.Version = version
	}
	if obj.localArgs.Version == "" {
		return nil, fmt.Errorf("distro version was not specified")
	}

	if obj.localArgs.Arch == "" {
		obj.localArgs.Arch = "x86_64"
	}

	if obj.localArgs.Flavour == nil || *obj.localArgs.Flavour == "" {
		flavour := "Workstation" // set a default for fedora and debian
		obj.localArgs.Flavour = &flavour
	}
	flavour := *obj.localArgs.Flavour

	if flavour != strings.Title(flavour) {
		return nil, fmt.Errorf("distro flavour should be in Title case")
	}

	if obj.localArgs.Distro == "fedora" && obj.localArgs.Mirror == "" {
		obj.localArgs.Mirror = util.FedoraDownloadURL // default
		// This will auto-resolve once we get going.
		m, err := util.GetFedoraDownloadURL(ctx)
		if err == nil {
			obj.localArgs.Mirror = m
		}
	}
	if obj.localArgs.Distro == "debian" && obj.localArgs.Mirror == "" {
		obj.localArgs.Mirror = util.DebianDownloadURL // a CDN, no need to resolve
	}

	if obj.localArgs.IPXE && obj.localArgs.Bios {
		return nil, fmt.Errorf("--ipxe can't be used with --bios")
	}
	// The debian 12 grub doesn't look at our proxy dhcp offer, so it can't
	// find its config, but it works with ipxe or with debian 13 and newer.
	if obj.localArgs.Proxy && obj.localArgs.Distro == "debian" && obj.localArgs.Version == "12" && !obj.localArgs.IPXE {
		return nil, fmt.Errorf("debian 12 needs --ipxe with --proxy, or use a newer debian")
	}

	obj.init.Logf("distro uid: %s%s-%s", obj.localArgs.Distro, obj.localArgs.Version, obj.localArgs.Arch)
	if obj.localArgs.IPXE {
		obj.init.Logf("ipxe: true (secure boot must be disabled on the host)")
	}
	obj.init.Logf("flavour: %+v", flavour)
	obj.init.Logf("mirror: %+v", obj.localArgs.Mirror)
	if len(obj.localArgs.Packages) > 0 {
		obj.init.Logf("packages: %+v", strings.Join(obj.localArgs.Packages, ","))
	}

	obj.init.Logf("luks: %t", obj.localArgs.LUKS)

	modulePathArgs := []string{}
	if p := obj.localArgs.HandoffModulePath; p != "" {
		if strings.HasPrefix(p, "~") {
			expanded, err := util.ExpandHome(p)
			if err != nil {
				return nil, err
			}
			obj.localArgs.HandoffModulePath = expanded
		}

		// Make path absolute.
		if !strings.HasPrefix(obj.localArgs.HandoffModulePath, "/") {
			dir, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			dir = dir + "/" // dir's should have a trailing slash!
			obj.localArgs.HandoffModulePath = dir + obj.localArgs.HandoffModulePath
		}

		// Does this path actually exist?
		if _, err := os.Stat(obj.localArgs.HandoffModulePath); err != nil {
			return nil, err
		}

		s := fmt.Sprintf("--module-path=%s", obj.localArgs.HandoffModulePath)
		modulePathArgs = append(modulePathArgs, s)
	}

	if p := obj.localArgs.HandoffCode; p != "" {
		if strings.HasPrefix(p, "~") {
			expanded, err := util.ExpandHome(p)
			if err != nil {
				return nil, err
			}
			obj.localArgs.HandoffCode = expanded
		}

		// Make path absolute.
		if !strings.HasPrefix(obj.localArgs.HandoffCode, "/") {
			dir, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			dir = dir + "/" // dir's should have a trailing slash!
			obj.localArgs.HandoffCode = dir + obj.localArgs.HandoffCode
		}

		// Does this path actually exist?
		if _, err := os.Stat(obj.localArgs.HandoffCode); err != nil {
			return nil, err
		}

		binary, err := util.ExecutablePath() // path to mgmt binary
		if err != nil {
			return nil, err
		}

		// Type check this path before we provision?
		out := ""
		cmdOpts := &util.SimpleCmdOpts{
			Debug: true,
			Logf: func(format string, v ...interface{}) {
				// XXX: HACK to make output more beautiful!
				errorText := "cli parse error: "
				s := fmt.Sprintf(format+"\n", v...)
				for _, x := range strings.Split(s, "\n") {
					if !strings.HasPrefix(x, errorText) {
						continue
					}
					out = strings.TrimPrefix(x, errorText)
				}
			},
		}
		// TODO: Add a --quiet flag instead of the above filter hack.
		cmdArgs := []string{"check", "--tmp-prefix", "lang", "--skip-fmt"}
		cmdArgs = append(cmdArgs, modulePathArgs...)
		cmdArgs = append(cmdArgs, obj.localArgs.HandoffCode)
		if err := util.SimpleCmd(ctx, binary, cmdArgs, cmdOpts); err != nil {
			return nil, fmt.Errorf("handoff code didn't type check: %s", out)
		}

		obj.init.Logf("handoff: %s", obj.localArgs.HandoffCode)
		if len(modulePathArgs) > 0 {
			obj.init.Logf("handoff module path: %s", obj.localArgs.HandoffModulePath)
		}
	}
	if h := obj.localArgs.HandoffHostname; h != "" {
		obj.init.Logf("handoff hostname: %s", h)
	}

	// Do this last to let others fail early b/c this has user interaction.
	if obj.localArgs.Password == nil {
		b, err := password.ReadPasswordCtxPrompt(ctx, "["+ModuleName+"] password: ")
		if err != nil {
			return nil, err
		}
		fmt.Printf("\n") // leave space after the prompt
		// XXX: I have no idea if I am doing this correctly, and I have
		// no idea if the library is doing this correctly. Please check!
		// XXX: erase values: https://github.com/golang/go/issues/21865
		hash, err := password.SaltedSHA512Password(b) // internally salted
		if err != nil {
			return nil, err
		}
		obj.password = hash // store
	} else if p := *obj.localArgs.Password; p == "-" {
		// XXX: pull from a file or something else if we choose this
		return nil, fmt.Errorf("not implemented")
	} else if len(p) != 106 { // salted length should be 106 chars AIUI
		return nil, fmt.Errorf("password must be salted with openssl passwd -6")
	} else {
		obj.password = p // salted
	}

	if obj.localArgs.Check {
		obj.init.Logf("stopping after check stage...")
		runArgs.RunLang.CheckOnly = true
		runArgs.RunLang.CheckUnify = true
	}

	name := fastsolver.Name
	// TODO: Remove these optimizations when the solver is faster overall.
	runArgs.RunLang.UnifySolver = &name
	//runArgs.RunLang.UnifyOptimizations = []string{
	//	fastsolver.TODO,
	//}
	libConfig.TmpPrefix = true
	libConfig.NoAutoGroup = false // required
	libConfig.NoPgp = true

	runArgs.Config = libConfig // store any changes we made
	return runArgs, nil
}

// checkProxy validates the proxy dhcp args. They're for an existing network, so
// without a mac address, any host in the proxy range gets provisioned. This is
// why we never use the whole network unless it's explicitly forced.
func (obj *provisioner) checkProxy(existing bool) error {
	args := obj.localArgs
	if !args.Proxy {
		if args.ProxyRange != "" || args.ProxyForce {
			return fmt.Errorf("--proxy-range and --proxy-force need --proxy")
		}
		return nil
	}

	if !existing {
		return fmt.Errorf("--network must be empty with --proxy")
	}
	if args.ProxyRange != "" && args.ProxyForce {
		return fmt.Errorf("--proxy-range and --proxy-force can't be used together")
	}

	if args.Mac != nil { // a single host
		if args.ProxyRange != "" || args.ProxyForce {
			return fmt.Errorf("--proxy-range and --proxy-force can't be used with --mac")
		}
		// A bios host never asks us for its nbp once it has its lease,
		// so we could never check its ip, and it would never boot.
		if args.Bios && *args.IP != "" {
			return fmt.Errorf("--ip can't be used for a --bios host with --proxy")
		}
		obj.init.Logf("proxy: host %s", args.Mac.String())
		return nil
	}

	// Any host in the range gets provisioned, which erases its disk!
	if args.ProxyRange == "" && !args.ProxyForce {
		return fmt.Errorf("--proxy without --mac needs --proxy-range, or --proxy-force for the whole network")
	}
	if args.Bios {
		return fmt.Errorf("--bios hosts can't be provisioned with --proxy without --mac")
	}
	if args.ProxyRange == "" {
		obj.init.Logf("proxy: any uefi host on the network of %s (forced)", *args.Interface)
		return nil
	}
	if _, _, err := net.ParseCIDR(args.ProxyRange); err != nil {
		return errwrap.Wrapf(err, "invalid --proxy-range")
	}
	obj.init.Logf("proxy: any uefi host with an ip in %s", args.ProxyRange)

	return nil
}

// Register generates some functions that expose the output of our local CLI.
func (obj *provisioner) Register(moduleName string) error {

	// Build all the functions...
	if err := simple.StructRegister(moduleName, obj.localArgs); err != nil {
		return err
	}

	// Build a few separately...
	simple.ModuleRegister(moduleName, "cli_password", &simple.Scaffold{
		I: &simple.Info{
			Pure: false,
			Memo: false,
			Fast: false,
			Spec: false,
		},
		T: types.NewType("func() str"),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			if obj.localArgs == nil {
				// programming error
				return nil, fmt.Errorf("could not convert/access our struct")
			}

			// TODO: plumb through the password lookup here instead?

			//localArgs := *obj.localArgs // optional
			return &types.StrValue{
				V: obj.password,
			}, nil
		},
	})

	return nil
}

func init() {
	fullModuleName := embedded.FullModuleName(ModuleName)
	//fs := embedded.MergeFS(metadata, main, files) // To merge filesystems!
	embedded.ModuleRegister(fullModuleName, fs)

	var a interface{} = &localArgs{} // must use the pointer here

	custom := &provisioner{
		localArgs: a.(*localArgs), // force the correct type
	}

	entry.Register(ModuleName, &entry.Data{
		Program: ModuleName,
		Version: Version, // TODO: get from git?

		Debug: false,
		Logf: func(format string, v ...interface{}) {
			log.Printf(format, v...)
		},

		Args:   a,
		Custom: custom,

		Frontend: Frontend,
		Top:      top,
	})

	if err := custom.Register(fullModuleName); err != nil { // functions from cli
		panic(err)
	}
}
