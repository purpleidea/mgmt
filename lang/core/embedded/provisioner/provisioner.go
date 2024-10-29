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
	DNS []string `arg:"--dns" help:"upstream dns servers to use" func:"cli_dns"` // eg: ["8.8.8.8", "1.1.1.1"]

	// Prefix is a directory to store some provisioner specific state such
	// as cached distro packages. It can be safely deleted. If you don't
	// specify this value, one will be chosen automatically.
	Prefix *string `arg:"--prefix" help:"local XDG_CACHE_HOME path" func:"cli_prefix"` // eg: ~/.cache/mgmt/provisioner/

	// Firewalld will automatically open the required ports for being a
	// provisioner. By default this is enabled, but it can be disabled if
	// you use a different firewall system.
	Firewalld bool `arg:"--firewalld" default:"true" help:"should we open firewalld on our provisioner" func:"cli_firewalld"`

	// repo

	// Distro specifies the distribution to use. Currently only `fedora` is
	// supported.
	Distro string `arg:"--distro" default:"fedora" help:"distribution to use" func:"cli_distro"`

	// Version is the distribution version. This is a string, not an int.
	Version string `arg:"--dversion" help:"distribution version" func:"cli_version"` // eg: "38"

	// Arch is the distro architecture to use. Only x86_64 and aarch64 are
	// currently supported. Patches welcome.
	Arch string `arg:"--arch" default:"x86_64" help:"architecture to use" func:"cli_arch"`

	// Flavour describes a flavour of distribution to provision. The value
	// and what it does is highly dependent on the distro you specified. The
	// default is set automatically depending on your distro variable.
	Flavour *string `arg:"--flavour" help:"flavour of distribution" func:"cli_flavour"` // eg: "Workstation" or "Server"

	// Mirror is the mirror to provision from. Pick one that supports both
	// rsync AND https if you want the most capable provisioner features. A
	// list for fedora is at: https://admin.fedoraproject.org/mirrormanager/
	// eg: https://mirror.csclub.uwaterloo.ca/fedora/ for example. This
	// points to: https://download.fedoraproject.org/pub/fedora/linux/ by
	// default if unspecified, because it will automatically translate to a
	// local mirror near you.
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
	// and be contained in the above network that was specified.
	IP *string `arg:"--ip" help:"ip address with cidr of the host to provision" func:"cli_ip"` // eg: "192.168.42.114/24"

	// Bios should be set true if you want to provision legacy machines.
	Bios bool `arg:"--bios" help:"should we use bios or uefi" func:"cli_bios"`

	// Password is an `openssl passwd -6` salted password. If you don't
	// specify this, you will be prompted to enter the actual unhashed
	// password, and it will be salted and hashed for you.
	Password *string `arg:"--password" help:"the 'openssl passwd -6' salted password" func:"-"` // skip auto func gen

	// Part is the magic partitioning scheme to use. At the moment you can
	// either specify `plain` or `btrfs`. The default empty string will
	// use the `plain` scheme.
	Part string `arg:"--part" help:"partitioning scheme, read manual for details" func:"cli_part"` // eg: empty string for plain

	// Packages are a list of additional distro packages to install. It's up
	// to the user to make sure they exist and don't conflict with each
	// other or the base installation packages.
	Packages []string `arg:"--packages" help:"list of additional distro packages to install (comma separated)" func:"cli_packages"`

	// HandoffCode specifies that we want to handoff to this machine with a
	// static code deploy bolus. This is useful for isolated, one-time runs.
	HandoffCode string `arg:"--handoff-code" help:"code dir to handoff to host" func:"cli_handoff_code"` // eg: /etc/mgmt/

	// OnlyUnify tells the compiler to stop after type unification. This is
	// used for testing.
	OnlyUnify bool `arg:"--only-unify" help:"stop after type unification"`
}

// provisioner is our cli parser translator and general frontend object.
type provisioner struct {
	init *entry.Init

	// localArgs is a stored reference to the localArgs config struct that
	// is used in the API of the command line parsing library. After it
	// adds our flags and executes it, the resultant parsed values will be
	// made available here where we've stored a copy.
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

	if s := obj.localArgs.Interface; s == nil {
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
		obj.localArgs.Network = &x
	}
	_, netIPnet, err := net.ParseCIDR(*obj.localArgs.Network)
	if err != nil {
		return nil, err
	}
	if s := obj.localArgs.Router; s == nil {
		x := "192.168.42.1/24"
		obj.localArgs.Router = &x
	}
	routerIP, _, err := net.ParseCIDR(*obj.localArgs.Router)
	if err != nil {
		return nil, err
	}
	if !netIPnet.Contains(routerIP) {
		return nil, fmt.Errorf("network %s does not contain %s", *obj.localArgs.Network, *obj.localArgs.Router)
	}

	if s := obj.localArgs.IP; s == nil {
		x := "192.168.42.13/24"
		obj.localArgs.IP = &x
	}
	hostIP, _, err := net.ParseCIDR(*obj.localArgs.Router)
	if err != nil {
		return nil, err
	}
	if !netIPnet.Contains(hostIP) {
		return nil, fmt.Errorf("network %s does not contain %s", *obj.localArgs.Network, *obj.localArgs.IP)
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
	if obj.localArgs.Prefix == nil { // pick a default
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
	if obj.localArgs.Distro != "fedora" { // TODO: add other distros!
		return nil, fmt.Errorf("only fedora is currently supported")
	}

	if obj.localArgs.Distro == "fedora" && obj.localArgs.Version == "" {
		version, err := util.LatestFedoraVersion(ctx, obj.localArgs.Arch) // get a default for fedora
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

	if obj.localArgs.Distro == "fedora" && obj.localArgs.Flavour == nil {
		flavour := "Workstation" // set a default for fedora
		obj.localArgs.Flavour = &flavour
	}
	flavour := *obj.localArgs.Flavour

	if obj.localArgs.Distro == "fedora" && flavour != strings.Title(flavour) {
		return nil, fmt.Errorf("distro flavour should be in Title case")
	}

	if obj.localArgs.Distro == "fedora" && obj.localArgs.Mirror == "" {
		obj.localArgs.Mirror = "https://download.fedoraproject.org/pub/fedora/linux/" // default
		// This will auto-resolve once we get going.
		m, err := util.GetFedoraDownloadURL(ctx)
		if err == nil {
			obj.localArgs.Mirror = m
		}
	}

	obj.init.Logf("distro uid: %s%s-%s", obj.localArgs.Distro, obj.localArgs.Version, obj.localArgs.Arch)
	obj.init.Logf("flavour: %+v", flavour)
	obj.init.Logf("mirror: %+v", obj.localArgs.Mirror)
	if len(obj.localArgs.Packages) > 0 {
		obj.init.Logf("packages: %+v", strings.Join(obj.localArgs.Packages, ","))
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
		cmdArgs := []string{"run", "--tmp-prefix", "lang", "--only-unify", obj.localArgs.HandoffCode}
		if err := util.SimpleCmd(ctx, binary, cmdArgs, cmdOpts); err != nil {
			return nil, fmt.Errorf("handoff code didn't type check: %s", out)
		}

		obj.init.Logf("handoff: %s", obj.localArgs.HandoffCode)
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

	// Make any changes here that we want to...
	runArgs.RunLang.SkipUnify = true // speed things up for known good code
	if obj.localArgs.OnlyUnify {
		obj.init.Logf("stopping after type unification...")
		runArgs.RunLang.OnlyUnify = true
		runArgs.RunLang.SkipUnify = false // can't skip if we only unify
	}

	name := fastsolver.Name
	// TODO: Remove these optimizations when the solver is faster overall.
	runArgs.RunLang.UnifySolver = &name
	//runArgs.RunLang.UnifyOptimizations = []string{
	//	fastsolver.TODO,
	//}
	libConfig.TmpPrefix = true
	libConfig.NoPgp = true

	runArgs.Config = libConfig // store any changes we made
	return runArgs, nil
}

// Register generates some functions that expose the output of our local CLI.
func (obj *provisioner) Register(moduleName string) error {

	// Build all the functions...
	if err := simple.StructRegister(moduleName, obj.localArgs); err != nil {
		return err
	}

	// Build a few separately...
	simple.ModuleRegister(moduleName, "cli_password", &simple.Scaffold{
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
