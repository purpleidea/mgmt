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

package util

import (
	"reflect"
	"strings"
)

// LookupSubcommand returns the name of the subcommand in the obj, of a struct.
// This is useful for determining the name of the subcommand that was activated.
// It returns an empty string if a specific name was not found.
func LookupSubcommand(obj interface{}, st interface{}) string {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr { // max one de-referencing
		val = val.Elem()
	}

	v := reflect.ValueOf(st) // value of the struct
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := val.Field(i) // value of the field
		if f.Interface() != v.Interface() {
			continue
		}

		field := typ.Field(i)
		alias, ok := field.Tag.Lookup("arg")
		if !ok {
			continue
		}

		// XXX: `arg` needs a split by comma first or fancier parsing
		prefix := "subcommand"
		split := strings.Split(alias, ":")
		if len(split) != 2 || split[0] != prefix {
			continue
		}

		return split[1] // found
	}
	return "" // not found
}

// EmptyArgs is the empty CLI parsing structure and type of the parsed result.
type EmptyArgs struct {
	Wait bool `arg:"--wait" help:"don't use any existing (stale) deploys"`
}

// LangArgs is the lang CLI parsing structure and type of the parsed result.
type LangArgs struct {
	// Input is the input mcl code or file path or any input specification.
	Input string `arg:"positional,required"`

	// TODO: removed (temporarily?)
	//Stdin bool `arg:"--stdin" help:"use passthrough stdin"`

	Download     bool `arg:"--download" help:"download any missing imports"`
	OnlyDownload bool `arg:"--only-download" help:"stop after downloading any missing imports"`
	Update       bool `arg:"--update" help:"update all dependencies to the latest versions"`

	OnlyUnify          bool     `arg:"--only-unify" help:"stop after type unification"`
	SkipUnify          bool     `arg:"--skip-unify" help:"skip type unification"`
	UnifySolver        *string  `arg:"--unify-name" help:"pick a specific unification solver"`
	UnifyOptimizations []string `arg:"--unify-optimizations,separate" help:"list of unification optimizations to request (experts only)"`

	Depth int `arg:"--depth" default:"-1" help:"max recursion depth limit (-1 is unlimited)"`

	// The default of 0 means any error is a failure by default.
	Retry int `arg:"--depth" help:"max number of retries (-1 is unlimited)"`

	ModulePath string `arg:"--module-path,env:MGMT_MODULE_PATH" help:"choose the modules path (absolute)"`
}

// YamlArgs is the yaml CLI parsing structure and type of the parsed result.
type YamlArgs struct {
	// Input is the input yaml code or file path or any input specification.
	Input string `arg:"positional,required"`
}

// PuppetArgs is the puppet CLI parsing structure and type of the parsed result.
type PuppetArgs struct {
	// Input is the input puppet code or file path or just "agent".
	Input string `arg:"positional,required"`

	// PuppetConf is the optional path to a puppet.conf config file.
	PuppetConf string `arg:"--puppet-conf" help:"full path to the puppet.conf file to use"`
}

// LangPuppetArgs is the langpuppet CLI parsing structure and type of the parsed
// result.
type LangPuppetArgs struct {
	// LangInput is the input mcl code or file path or any input specification.
	LangInput string `arg:"--lang,required" help:"the input parameter for the lang module"`

	// PuppetInput is the input puppet code or file path or just "agent".
	PuppetInput string `arg:"--puppet,required" help:"the input parameter for the puppet module"`

	// copy-pasted from PuppetArgs

	// PuppetConf is the optional path to a puppet.conf config file.
	PuppetConf string `arg:"--puppet-conf" help:"full path to the puppet.conf file to use"`

	// end PuppetArgs

	// copy-pasted from LangArgs

	// TODO: removed (temporarily?)
	//Stdin bool `arg:"--stdin" help:"use passthrough stdin"`

	Download     bool `arg:"--download" help:"download any missing imports"`
	OnlyDownload bool `arg:"--only-download" help:"stop after downloading any missing imports"`
	Update       bool `arg:"--update" help:"update all dependencies to the latest versions"`

	OnlyUnify bool `arg:"--only-unify" help:"stop after type unification"`
	SkipUnify bool `arg:"--skip-unify" help:"skip type unification"`

	Depth int `arg:"--depth" default:"-1" help:"max recursion depth limit (-1 is unlimited)"`

	// The default of 0 means any error is a failure by default.
	Retry int `arg:"--depth" help:"max number of retries (-1 is unlimited)"`

	ModulePath string `arg:"--module-path,env:MGMT_MODULE_PATH" help:"choose the modules path (absolute)"`

	// end LangArgs
}

// SetupPkgArgs is the setup service CLI parsing structure and type of the
// parsed result.
type SetupPkgArgs struct {
	Distro string `arg:"--distro" help:"build for this distro"`
	Sudo   bool   `arg:"--sudo" help:"include sudo in the command"`
	Exec   bool   `arg:"--exec" help:"actually run these commands"`
}

// SetupSvcArgs is the setup service CLI parsing structure and type of the
// parsed result.
type SetupSvcArgs struct {
	BinaryPath string   `arg:"--binary-path" help:"path to the binary"`
	SSHURL     string   `arg:"--ssh-url" help:"transport the etcd client connection over SSH to this server"`
	Seeds      []string `arg:"--seeds,separate,env:MGMT_SEEDS" help:"default etcd client endpoints"`
	NoServer   bool     `arg:"--no-server" help:"do not start embedded etcd server (do not promote from client to peer)"`

	Install bool `arg:"--install" help:"install the systemd mgmt service"`
	Start   bool `arg:"--start" help:"start the mgmt service"`
	Enable  bool `arg:"--enable" help:"enable the mgmt service"`
}

// SetupFirstbootArgs is the setup service CLI parsing structure and type of the
// parsed result.
type SetupFirstbootArgs struct {
	BinaryPath string `arg:"--binary-path" help:"path to the binary"`
	Mkdir      bool   `arg:"--mkdir" help:"make the necessary firstboot dirs"`
	Install    bool   `arg:"--install" help:"install the systemd firstboot service"`
	Start      bool   `arg:"--start" help:"start the firstboot service (typically not used)"`
	Enable     bool   `arg:"--enable" help:"enable the firstboot service"`

	FirstbootStartArgs // Include these options if we want to specify them.
}

// FirstbootStartArgs is the firstboot service CLI parsing structure and type of
// the parsed result.
type FirstbootStartArgs struct {
	LockFilePath string `arg:"--lock-file-path" help:"path to the lock file"`
	ScriptsDir   string `arg:"--scripts-dir" help:"path to the scripts dir"`
	DoneDir      string `arg:"--done-dir" help:"dir to move done scripts to"`
	LoggingDir   string `arg:"--logging-dir" help:"directory to store logs in"`
}

// DocsGenerateArgs is the docgen utility CLI parsing structure and type of the
// parsed result.
type DocsGenerateArgs struct {
	Output      string `arg:"--output" help:"output path to write to"`
	RootDir     string `arg:"--root-dir" help:"path to mgmt source dir"`
	NoResources bool   `arg:"--no-resources" help:"skip resource doc generation"`
	NoFunctions bool   `arg:"--no-functions" help:"skip function doc generation"`
}
