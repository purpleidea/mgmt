// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package interfaces

import (
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
)

// Info is a static representation of some information about the function. It is
// used for static analysis and type checking. If you break this contract, you
// might cause a panic.
type Info struct {
	Pure bool        // is the function pure? (can it be memoized?)
	Memo bool        // should the function be memoized? (false if too much output)
	Slow bool        // is the function slow? (avoid speculative execution)
	Sig  *types.Type // the signature of the function, must be KindFunc
	Err  error       // is this a valid function, or was it created improperly?
}

// Init is the structure of values and references which is passed into all
// functions on initialization.
type Init struct {
	Hostname string // uuid for the host
	//Noop bool
	Input  chan types.Value // Engine will close `input` chan
	Output chan types.Value // Stream must close `output` chan
	// TODO: should we pass in a *Scope here for functions like template() ?
	World engine.World
	Debug bool
	Logf  func(format string, v ...interface{})
}

// Func is the interface that any valid func must fulfill. It is very simple,
// but still event driven. Funcs should attempt to only send values when they
// have changed.
// TODO: should we support a static version of this interface for funcs that
// never change to avoid the overhead of the goroutine and channel listener?
type Func interface {
	Validate() error // FIXME: this is only needed for PolyFunc. Get it moved and used!

	// Info returns some information about the function in question, which
	// includes the function signature. For a polymorphic function, this
	// might not be known until after Build was called. As a result, the
	// sig should be allowed to return a partial or variant type if it is
	// not known yet. This is because the Info method might be called
	// speculatively to aid in type unification.
	Info() *Info
	Init(*Init) error
	Stream() error
	Close() error
}

// PolyFunc is an interface for functions which are statically polymorphic. In
// other words, they are functions which before compile time are polymorphic,
// but after a successful compilation have a fixed static signature. This makes
// implementing what would appear to be generic or polymorphic instead something
// that is actually static and that still has the language safety properties.
type PolyFunc interface {
	Func // implement everything in Func but add the additional requirements

	// Polymorphisms returns a list of possible function type signatures. It
	// takes as input a list of partial "hints" as to limit the number of
	// possible results it returns. These partial hints take the form of a
	// function type signature (with as many types in it specified and the
	// rest set to nil) and any known static values for the input args. If
	// the partial type is not nil, then the Ord parameter must be of the
	// correct arg length. If any types are specified, then the array must
	// be of that length as well, with the known ones filled in. Some
	// static polymorphic functions require a minimal amount of hinting or
	// they will be unable to return any possible result that is not
	// infinite in length. If you expect to need to return an infinite (or
	// very large) amount of results, then you should return an error
	// instead. The arg names in your returned func type signatures should
	// be in the standardized "a..b..c" format. Use util.NumToAlpha if you
	// want to convert easily.
	Polymorphisms(*types.Type, []types.Value) ([]*types.Type, error)

	// Build takes the known type signature for this function and finalizes
	// this structure so that it is now determined, and ready to function as
	// a normal function would. (The normal methods in the Func interface
	// are all that should be needed or used after this point.)
	Build(*types.Type) error // then, you can get argNames from Info()
}

// NamedArgsFunc is a function that uses non-standard function arg names. If you
// don't implement this, then the argnames (if specified) must correspond to the
// a, b, c...z, aa, ab...az, ba...bz, and so on sequence.
type NamedArgsFunc interface {
	Func // implement everything in Func but add the additional requirements

	// ArgGen implements the arg name generator function. By default, we use
	// the util.NumToAlpha function when this interface isn't implemented...
	ArgGen(int) (string, error)
}
