// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package util

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ModuleMagicPrefix is the prefix which, if found as a prefix to the
	// last token in an import path, will be removed silently if there are
	// remaining characters following the name. If this is the empty string
	// then it will be ignored.
	ModuleMagicPrefix = "mgmt-"
)

// HasDuplicateTypes returns an error if the list of types is not unique.
func HasDuplicateTypes(typs []*types.Type) error {
	// FIXME: do this comparison in < O(n^2) ?
	for i, ti := range typs {
		for j, tj := range typs {
			if i == j {
				continue // don't compare to self
			}
			if ti.Cmp(tj) == nil {
				return fmt.Errorf("duplicate type of %+v found", ti)
			}
		}
	}
	return nil
}

// FnMatch is run to turn a polymorphic, undetermined list of functions, into a
// specific statically typed version. It is usually run after Unify completes.
// It returns the index of the matched function.
func FnMatch(typ *types.Type, fns []*types.FuncValue) (int, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ == nil {
		return 0, fmt.Errorf("type of function must be specified")
	}
	if typ.Kind != types.KindFunc {
		return 0, fmt.Errorf("type must be of kind Func")
	}
	if typ.Out == nil {
		return 0, fmt.Errorf("return type of function must be specified")
	}

	// find typ in fns
	for ix, f := range fns {
		if f.T.HasVariant() {
			continue // match these if no direct matches exist
		}
		// FIXME: can we replace this by the complex matcher down below?
		if f.T.Cmp(typ) == nil {
			return ix, nil // found match at this index
		}
	}

	// match concrete type against our list that might contain a variant
	var found bool
	var index int
	for ix, f := range fns {
		_, err := typ.ComplexCmp(f.T)
		if err != nil {
			continue
		}
		if found { // already found one...
			// TODO: we *could* check that the previous duplicate is
			// equivalent, but in this case, it is really a bug that
			// the function author had by allowing ambiguity in this
			return 0, fmt.Errorf("duplicate match found for build type: %+v", typ)
		}
		found = true
		index = ix // found match at this index
	}
	// ensure there's only one match...
	if found {
		return index, nil // w00t!
	}

	return 0, fmt.Errorf("unable to find a compatible function for type: %+v", typ)
}

// ValidateVarName returns an error if the string pattern does not match the
// format for a valid variable name. The leading dollar sign must not be passed
// in.
func ValidateVarName(name string) error {
	if name == "" {
		return fmt.Errorf("got empty var name")
	}

	// A variable always starts with an lowercase alphabetical char and
	// contains lowercase alphanumeric characters or numbers, underscores,
	// and non-consecutive dots. The last char must not be an underscore or
	// a dot.
	// TODO: put the variable matching pattern in a const somewhere?
	pattern := `^[a-z]([a-z0-9_]|(\.|_)[a-z0-9_])*$`

	matched, err := regexp.MatchString(pattern, name)
	if err != nil {
		return errwrap.Wrapf(err, "error matching regex")
	}
	if !matched {
		return fmt.Errorf("invalid var name: `%s`", name)
	}

	// Check that we don't get consecutive underscores or dots!
	// TODO: build this into the above regexp and into the parse.rl file!
	if strings.Contains(name, "..") {
		return fmt.Errorf("var name contains multiple periods: `%s`", name)
	}
	if strings.Contains(name, "__") {
		return fmt.Errorf("var name contains multiple underscores: `%s`", name)
	}

	return nil
}

// ParseImportName parses an import name and returns the default namespace name
// that should be used with it. For example, if the import name was:
// "git://example.com/purpleidea/Module-Name", this might return an alias of
// "module_name". It also returns a bunch of other data about the parsed import.
// TODO: check for invalid or unwanted special characters
func ParseImportName(name string) (*interfaces.ImportData, error) {
	magicPrefix := ModuleMagicPrefix
	if name == "" {
		return nil, fmt.Errorf("empty name")
	}
	if strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("absolute paths are not allowed")
	}

	u, err := url.Parse(name)
	if err != nil {
		return nil, errwrap.Wrapf(err, "name is not a valid url")
	}
	if u.Path == "" {
		return nil, fmt.Errorf("empty path")
	}
	p := u.Path
	// catch bad paths like: git:////home/james/ (note the quad slash!)
	// don't penalize if we have a dir with a trailing slash at the end
	if s := path.Clean(u.Path); u.Path != s && u.Path != s+"/" {
		// TODO: are there any cases where this is not what we want?
		return nil, fmt.Errorf("dirty path, cleaned it's: `%s`", s)
	}

	for strings.HasSuffix(p, "/") { // remove trailing slashes
		p = p[:len(p)-len("/")]
	}

	split := strings.Split(p, "/") // take last chunk if slash separated
	s := split[0]
	if len(split) > 1 {
		s = split[len(split)-1] // pick last chunk
	}

	// TODO: should we treat a special name: "purpleidea/mgmt-foo" as "foo"?
	if magicPrefix != "" && strings.HasPrefix(s, magicPrefix) && len(s) > len(magicPrefix) {
		s = s[len(magicPrefix):]
	}

	s = strings.Replace(s, "-", "_", -1) // XXX: allow underscores in IDENTIFIER
	if strings.HasPrefix(s, "_") || strings.HasSuffix(s, "_") {
		return nil, fmt.Errorf("name can't begin or end with dash or underscore")
	}
	alias := strings.ToLower(s)

	// if this is a local import, it's a straight directory path
	// if it's an fqdn import, it should contain a metadata file

	// if there's no protocol prefix, then this must be a local path
	isLocal := u.Scheme == ""
	// if it has a trailing slash or .mcl extension it's not a system import
	isSystem := isLocal && !strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(u.Path, interfaces.DotFileNameExtension)
	// is it a local file?
	isFile := !isSystem && isLocal && strings.HasSuffix(u.Path, interfaces.DotFileNameExtension)
	xpath := u.Path // magic path
	if isSystem {
		xpath = ""
	}
	if !isLocal {
		host := u.Host // host or host:port
		split := strings.Split(host, ":")
		if l := len(split); l == 1 || l == 2 {
			host = split[0]
		} else {
			return nil, fmt.Errorf("incorrect number of colons (%d) in hostname", l)
		}
		xpath = path.Join(host, xpath)
	}
	if !isLocal && !strings.HasSuffix(xpath, "/") {
		xpath = xpath + "/"
	}
	// we're a git repo with a local path instead of an fqdn over http!
	// this still counts as isLocal == false, since it's still a remote
	if u.Host == "" && strings.HasPrefix(u.Path, "/") {
		xpath = strings.TrimPrefix(xpath, "/") // make it a relative dir
	}
	if strings.HasPrefix(xpath, "/") { // safety check (programming error?)
		return nil, fmt.Errorf("can't parse strange import")
	}

	// build a url to clone from if we're not local...
	// TODO: consider adding some logic that is similar to the logic in:
	// https://github.com/golang/go/blob/054640b54df68789d9df0e50575d21d9dbffe99f/src/cmd/go/internal/get/vcs.go#L972
	// so that we can more correctly figure out the correct url to clone...
	xurl := ""
	if !isLocal {
		u.Fragment = ""
		// TODO: maybe look for ?sha1=... or ?tag=... to pick a real ref
		u.RawQuery = ""
		u.ForceQuery = false
		xurl = u.String()
	}

	// if u.Path is local file like: foo/server.mcl alias should be "server"
	// we should trim the alias to remove the .mcl (the dir is already gone)
	if isFile && strings.HasSuffix(alias, interfaces.DotFileNameExtension) {
		alias = strings.TrimSuffix(alias, interfaces.DotFileNameExtension)
	}

	return &interfaces.ImportData{
		Name:     name, // save the original value here
		Alias:    alias,
		IsSystem: isSystem,
		IsLocal:  isLocal,
		IsFile:   isFile,
		Path:     xpath,
		URL:      xurl,
	}, nil
}
