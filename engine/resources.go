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

package engine

import (
	"context"
	"encoding/gob"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"

	"gopkg.in/yaml.v2"
)

const (
	// ResourcesRelDir is the path where the resources are kept, relative to
	// the main source code root.
	ResourcesRelDir = "engine/resources/"
)

// TODO: should each resource be a sub-package?
var registeredResources = map[string]func() Res{}

// RegisterResource registers a new resource by providing a constructor function
// that returns a resource object ready to be unmarshalled from YAML.
func RegisterResource(kind string, fn func() Res) {
	f := fn()
	if kind == "" {
		panic("can't register a resource with an empty kind")
	}
	if _, ok := registeredResources[kind]; ok {
		panic(fmt.Sprintf("a resource kind of %s is already registered", kind))
	}
	gob.Register(f)
	registeredResources[kind] = fn

	// Additional metadata for documentation generation!
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic(fmt.Sprintf("could not locate resource filename for %s", kind))
	}
	sp := strings.Split(reflect.TypeOf(f).String(), ".")
	if len(sp) != 2 {
		panic(fmt.Sprintf("could not parse resource struct name for %s", kind))
	}

	if err := docsUtil.RegisterResource(kind, &docsUtil.Metadata{
		Filename: filepath.Base(filename),
		Typename: sp[1],
	}); err != nil {
		panic(fmt.Sprintf("could not register resource metadata for %s", kind))
	}
}

// RegisteredResourcesNames returns the kind of the registered resources.
func RegisteredResourcesNames() []string {
	kinds := []string{}
	for k := range registeredResources {
		kinds = append(kinds, k)
	}
	return kinds
}

// IsKind returns true if this is a valid resource kind.
func IsKind(kind string) bool {
	_, ok := registeredResources[kind]
	return ok
}

// NewResource returns an empty resource object from a registered kind. It
// errors if the resource kind doesn't exist.
func NewResource(kind string) (Res, error) {
	fn, ok := registeredResources[kind]
	if !ok {
		return nil, fmt.Errorf("no resource kind `%s` available", kind)
	}
	res := fn().Default()
	res.SetKind(kind)
	return res, nil
}

// NewNamedResource returns an empty resource object from a registered kind. It
// also sets the name. It is a wrapper around NewResource. It also errors if the
// name is empty.
func NewNamedResource(kind, name string) (Res, error) {
	if name == "" {
		return nil, fmt.Errorf("resource name is empty")
	}
	res, err := NewResource(kind)
	if err != nil {
		return nil, err
	}
	res.SetName(name)
	return res, nil
}

// Init is the structure of values and references which is passed into all
// resources on initialization. None of these are available in Validate, or
// before Init runs.
type Init struct {
	// Program is the name of the program.
	Program string

	// Version is the version of the program.
	Version string

	// Hostname is the uuid for the host.
	Hostname string

	// Called from within Watch:

	// Running must be called after your watches are all started and ready.
	Running func()

	// Event sends an event notifying the engine of a possible state change.
	Event func()

	// Called from within CheckApply:

	// Refresh returns whether the resource received a notification. This
	// flag can be used to tell a svc to reload, or to perform some state
	// change that wouldn't otherwise be noticed by inspection alone. You
	// must implement the Refreshable trait for this to work.
	Refresh func() bool

	// Send exposes some variables you wish to send via the Send/Recv
	// mechanism. You must implement the Sendable trait for this to work.
	Send func(interface{}) error

	// Recv provides a map of variables which were sent to this resource via
	// the Send/Recv mechanism. You must implement the Recvable trait for
	// this to work.
	Recv func() map[string]*Send

	// Other functionality:

	// Graph is a function that returns the current graph. The returned
	// value won't be valid after a graphsync so make sure to call this when
	// you are about to use it, and discard it right after.
	// FIXME: it might be better to offer a safer, more limited, GraphQuery?
	//Graph func() *pgraph.Graph // TODO: not implemented, use FilteredGraph

	// FilteredGraph is a function that returns a filtered variant of the
	// current graph. Only resource that have allowed themselves to be added
	// into this graph will appear. If they did not consent, then those
	// vertices and any associated edges, will not be present.
	FilteredGraph func() (*pgraph.Graph, error)

	// TODO: GraphQuery offers an interface to query the resource graph.

	// Local has a bunch of methods and properties which are useful for
	// operations on the local machine and for communication between
	// functions and resources.
	Local *local.API

	// World provides a connection to the outside world. This is most often
	// used for communicating with the distributed database.
	World World

	// VarDir is a facility for local storage. It is used to return a path
	// to a directory which may be used for temporary storage. It should be
	// cleaned up on resource Close if the resource would like to delete the
	// contents. The resource should not assume that the initial directory
	// is empty, and it should be cleaned on Init if that is a requirement.
	VarDir func(string) (string, error)

	// Debug signals whether we are running in debugging mode. In this case,
	// we might want to log additional messages.
	Debug bool

	// Logf is a logging facility which will correctly namespace any
	// messages which you wish to pass on. You should use this instead of
	// the log package directly for production quality resources.
	Logf func(format string, v ...interface{})
}

// KindedRes is an interface that is required for a resource to have a kind.
type KindedRes interface {
	// Kind returns a string representing the kind of resource this is.
	Kind() string

	// SetKind sets the resource kind and should only be called by the
	// engine.
	SetKind(string)
}

// NamedRes is an interface that is used so a resource can have a unique name.
type NamedRes interface {
	Name() string
	SetName(string)
}

// Res is the minimum interface you need to implement to define a new resource.
type Res interface {
	fmt.Stringer // String() string

	KindedRes
	NamedRes // TODO: consider making this optional in the future
	MetaRes  // All resources must have meta params.

	// Default returns a struct with sane defaults for this resource.
	Default() Res

	// Validate determines if the struct has been defined in a valid state.
	Validate() error

	// Init initializes the resource and passes in some external information
	// and data from the engine.
	Init(*Init) error

	// Cleanup is run by the engine to clean up after the resource is done.
	Cleanup() error

	// Watch is run by the engine to monitor for state changes. If it
	// detects any, it notifies the engine which will usually run CheckApply
	// in response. If the input context cancels, we must shutdown.
	Watch(context.Context) error

	// CheckApply determines if the state of the resource is correct and if
	// asked to with the `apply` variable, applies the requested state. If
	// the input context cancels, we must return as quickly as possible. We
	// should never exit immediately if this would cause permanent
	// corruption of some sort. However it doesn't mean that a resource was
	// taken to the desired state.
	CheckApply(ctx context.Context, apply bool) (checkOK bool, err error)

	// Cmp compares itself to another resource and returns an error if they
	// are not equivalent. This is more strict than the Adapts method of the
	// CompatibleRes interface which allows for equivalent differences if
	// the have a compatible result in CheckApply.
	Cmp(Res) error
}

// Repr returns a representation of a resource from its kind and name. This is
// used as the definitive format so that it can be changed in one place.
func Repr(kind, name string) string {
	return fmt.Sprintf("%s[%s]", kind, name)
}

// Stringer returns a consistent and unique string representation of a resource.
func Stringer(res Res) string {
	return Repr(res.Kind(), res.Name())
}

// ResPtrUID is a unique identifier that is consistent for the kind and name of
// the resource only. This was formerly a string, but a struct is more precise.
// The result is suitable as a unique map key.
type ResPtrUID struct {
	Kind string
	Name string
}

// PtrUID generates a ResPtrUID from a resource. The result is suitable as a
// unique map key.
func PtrUID(res Res) ResPtrUID {
	// the use of "repr" is kind of arbitrary as long as it's unique
	//return ResPtrUID(Repr(res.Kind(), res.Name()))
	return ResPtrUID{Kind: res.Kind(), Name: res.Name()}
}

// Validate validates a resource by checking multiple aspects. This is the main
// entry point for running all the validation steps on a resource.
func Validate(res Res) error {
	if res.Kind() == "" { // shouldn't happen IIRC
		return fmt.Errorf("the Res has an empty Kind")
	}
	if res.Name() == "" {
		return fmt.Errorf("the Res has an empty Name")
	}

	if err := res.MetaParams().Validate(); err != nil {
		return errwrap.Wrapf(err, "the Res has an invalid meta param")
	}

	// TODO: pull dollar prefix from a constant
	// This catches typos where the user meant to use ${var} interpolation.
	if !res.MetaParams().Dollar && strings.HasPrefix(res.Name(), "$") {
		return fmt.Errorf("the Res name starts with a $")
	}

	// Don't need to validate normally if hidden.
	// XXX: Check if it's also Exported too? len(res.MetaParams.Export) > 0
	if res.MetaParams().Hidden {
		return nil
	}

	return res.Validate()
}

// InterruptableRes is an interface that adds interrupt functionality to
// resources. If the resource implements this interface, the engine will call
// the Interrupt method to shutdown the resource quickly. Running this method
// may leave the resource in a partial state, however this may be desired if you
// want a faster exit or if you'd prefer a partial state over letting the
// resource complete in a situation where you made an error and you wish to exit
// quickly to avoid data loss. It is usually triggered after multiple ^C
// signals.
type InterruptableRes interface {
	Res

	// Ask the resource to shutdown quickly. This can be called at any point
	// in the resource lifecycle after Init. Close will still be called. It
	// will only get called after an exit or pause request has been made. It
	// is designed to unblock any long running operation that is occurring
	// in the CheckApply portion of the life cycle. If the resource has
	// already exited, running this method should not block. (That is to say
	// that you should not expect CheckApply or Watch to be alive and be
	// able to read from a channel to satisfy your request.) It is best to
	// probably have this close a channel to multicast that signal around to
	// anyone who can detect it in a select. If you are in a situation which
	// cannot interrupt, then you can return an error.
	// FIXME: implement, and check the above description is what we expect!
	Interrupt() error
}

// CopyableRes is an interface that a resource can implement if we want to be
// able to copy the resource to build another one.
type CopyableRes interface {
	Res

	// Copy returns a new resource which has a copy of the public data.
	// Don't call this directly, use engine.ResCopy instead.
	// TODO: should we copy any private state or not?
	Copy() CopyableRes
}

// CompatibleRes is an interface that a resource can implement to express if a
// similar variant of itself is functionally equivalent. For example, two `pkg`
// resources that install `cowsay` could be equivalent if one requests a state
// of `installed` and the other requests `newest`, since they'll finish with a
// compatible result. This doesn't need to be behind a metaparam flag or trait,
// because it is never beneficial to turn it off, unless there is a bug to fix.
type CompatibleRes interface {
	//Res // causes "duplicate method" error
	CopyableRes // we'll need to use the Copy method in the Merge function!

	// Adapts compares itself to another resource and returns an error if
	// they are not compatibly equivalent. This is less strict than the
	// default `Cmp` method which should be used for most cases. Don't call
	// this directly, use engine.AdaptCmp instead.
	Adapts(CompatibleRes) error

	// Merge returns the combined resource to use when two are equivalent.
	// This might get called multiple times for N different resources that
	// need to get merged, and so it should produce a consistent result no
	// matter which order it is called in. Don't call this directly, use
	// engine.ResMerge instead.
	Merge(CompatibleRes) (CompatibleRes, error)
}

// ExportableRes allows the resource to have its own implementation of resource
// encoding, so that it can send data over the wire differently. It's unlikely
// that you will want to implement this interface for most scenarios. It may be
// useful to limit private data exposure, large data sizes, and to add more info
// to what would normally be shared.
type ExportableRes interface {
	Res

	// ToB64 lets the resource provide an alternative implementation of the
	// usual ResToB64 method. This lets the resource omit, add, or modify
	// the parameter data before it goes out over the wire.
	ToB64() (string, error)

	// TODO: Do we want to add a FromB64 method for decoding the Resource?
}

// YAMLRes is a resource that supports creation by unmarshalling.
type YAMLRes interface {
	Res

	yaml.Unmarshaler // UnmarshalYAML(unmarshal func(interface{}) error) error
}
