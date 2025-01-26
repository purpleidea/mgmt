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

package simple

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	unificationUtil "github.com/purpleidea/mgmt/lang/unification/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DirectInterface specifies whether we should use the direct function
	// API or not. If we don't use it, then these simple functions are
	// wrapped with the struct below.
	DirectInterface = false // XXX: fix any bugs and set to true!
)

// RegisteredFuncs maps a function name to the corresponding function scaffold.
var RegisteredFuncs = make(map[string]*Scaffold) // must initialize

// Scaffold holds the necessary data to build a (possibly polymorphic) function
// with this API.
type Scaffold struct {
	// T is the type of the function. It can include unification variables.
	// At a minimum, this must be a `func(?1) ?2` as a naked `?1` is not
	// allowed. (TODO: Because of ArgGen.)
	T *types.Type

	// C is a check function to run after type unification. It will get
	// passed the solved type of this function. It should error if this is
	// not an acceptable option. This function can be omitted.
	C func(typ *types.Type) error

	// F is the implementation of the function. The input type can be
	// determined by inspecting the values. Of note, this API does not tell
	// the implementation what the correct return type should be. If it
	// can't be determined from the input types, then a different function
	// API needs to be used. XXX: Should we extend this here?
	F interfaces.FuncSig

	// D is the documentation handle for this function. We look on that
	// struct or function for the doc string instead of the F field if this
	// is specified. (This is used for facts.)
	D interface{}
}

// Register registers a simple, static, pure, polymorphic function. It is easier
// to use than the raw function API. It allows you to build and check a function
// based on a type signature that contains unification variables. You may only
// specify a single type signature with the API, so some complex patterns are
// not possible with this API. Implementing a function like `printf` would not
// be possible. Implementing a function which counts the number of elements in a
// list would be.
func Register(name string, scaffold *Scaffold) {
	if _, exists := RegisteredFuncs[name]; exists {
		panic(fmt.Sprintf("a simple func named %s is already registered", name))
	}

	if scaffold == nil {
		panic("no scaffold specified for simple func")
	}
	if scaffold.T == nil {
		panic("no type specified for simple func")
	}
	if scaffold.T.Kind != types.KindFunc {
		panic("type must be a func")
	}
	if scaffold.T.HasVariant() {
		panic("func contains a variant type signature")
	}
	// It's okay if scaffold.C is nil.
	if scaffold.F == nil {
		panic("no implementation specified for simple func")
	}

	RegisteredFuncs[name] = scaffold // store a copy for ourselves

	// TODO: Do we need to special case either of these?
	//if strings.HasPrefix(name, "embedded/") {}
	//if strings.HasPrefix(name, "golang/") {}

	var f interface{} = scaffold.F
	if scaffold.D != nil { // override the doc lookup location if specified
		f = scaffold.D
	}
	metadata, err := funcs.GetFunctionMetadata(f)
	if err != nil {
		panic(fmt.Sprintf("could not locate function filename for %s", name))
	}

	// register a copy in the main function database
	funcs.Register(name, func() interfaces.Func {
		return &Func{
			Metadata: metadata,
			WrappedFunc: &wrapped.Func{
				Name: name,
				// NOTE: It might be more correct to Copy here,
				// but we do the copy inside of ExprFunc.Copy()
				// instead, so that the same type can be unified
				// in more than one way. Doing it here wouldn't
				// be harmful, but it's an extra copy we don't
				// need to do AFAICT.
				Type: scaffold.T, // .Copy(),
			},
			Check: scaffold.C,
			Func:  scaffold.F,
		}
	})
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
func ModuleRegister(module, name string, scaffold *Scaffold) {
	Register(module+funcs.ModuleSep+name, scaffold)
}

// WrappedFunc is a type alias so that we can embed `wrapped.Func` inside our
// struct, since the Func name collides with our Func field name.
type WrappedFunc = wrapped.Func

var _ interfaces.BuildableFunc = &Func{} // ensure it meets this expectation

// Func is a scaffolding function struct which fulfills the boiler-plate for the
// function API, but that can run a very simple, static, pure, polymorphic
// function.
type Func struct {
	*docsUtil.Metadata
	*WrappedFunc // *wrapped.Func as a type alias to pull in the base impl.

	// Check is a check function to run after type unification. It will get
	// passed the solved type of this function. It should error if this is
	// not an acceptable option. This function can be omitted.
	Check func(typ *types.Type) error

	// Func is the implementation of the function. The input type can be
	// determined by inspecting the values. Of note, this API does not tell
	// the implementation what the correct return type should be. If it
	// can't be determined from the input types, then a different function
	// API needs to be used. XXX: Should we extend this here?
	Func interfaces.FuncSig
}

// Build is run to turn the maybe polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after unification
// completes, and must be run before Info() and any of the other Func interface
// methods are used. For this function API, it just runs the Check function to
// make sure that the type found during unification is one of the valid ones.
func (obj *Func) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...

	if obj.Check != nil {
		if err := obj.Check(typ); err != nil {
			return nil, errwrap.Wrapf(err, "can't build %s with %s", obj.Name, typ)
		}
	}

	fn := &types.FuncValue{
		T: typ,
		V: obj.Func, // implementation
	}
	obj.Fn = fn
	return obj.Fn.T, nil
}

// TypeMatch accepts a list of possible type signatures that we want to check
// against after type unification. This helper function returns a function which
// is suitable for use in the scaffold check function field.
func TypeMatch(typeList []string) func(*types.Type) error {
	return func(typ *types.Type) error {
		for _, s := range typeList {
			t := types.NewType(s)
			if t == nil {
				// TODO: should we panic?
				continue // skip
			}
			if unificationUtil.UnifyCmp(typ, t) == nil {
				return nil
			}
		}
		return fmt.Errorf("did not match")

	}
}

// StructRegister takes an CLI args struct with optional struct tags, and
// generates simple functions from the contained fields in the specified
// namespace. If no struct field named `func` is included, then a default
// function name which is the lower case representation of the field name will
// be used, otherwise the struct tag contents are used. If the struct tag
// contains the `-` character, then the field will be skipped.
// TODO: An alternative version of this might choose to return all of the values
// as a single giant struct.
func StructRegister(moduleName string, args interface{}) error {
	if args == nil {
		// programming error
		return fmt.Errorf("could not convert/access our struct")
	}
	//fmt.Printf("A: %+v\n", args)

	val := reflect.ValueOf(args)
	if val.Kind() == reflect.Ptr { // max one de-referencing
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		v := val.Field(i) // value of the field
		t := typ.Field(i) // struct type, get real type with .Type

		name := strings.ToLower(t.Name) // default
		if alias, ok := t.Tag.Lookup("func"); ok {
			if alias == "-" { // skip
				continue
			}
			name = alias
		}
		//fmt.Printf("N: %+v\n", name) // debug
		if len(strings.Trim(name, "abcdefghijklmnopqrstuvwxyz_")) > 0 {
			return fmt.Errorf("struct field index(%d) has invalid char(s) in function name", i)
		}

		typed, err := types.TypeOf(t.Type) // reflect.Type -> (*types.Type, error)
		if err != nil {
			return err
		}
		//fmt.Printf("T: %+v\n", typed.String()) // debug

		ModuleRegister(moduleName, name, &Scaffold{
			T: types.NewType(fmt.Sprintf("func() %s", typed.String())),
			F: func(ctx context.Context, input []types.Value) (types.Value, error) {
				//if args == nil {
				//	// programming error
				//	return nil, fmt.Errorf("could not convert/access our struct")
				//}

				value, err := types.ValueOf(v) // reflect.Value -> (types.Value, error)
				if err != nil {
					return nil, errwrap.Wrapf(err, "func `%s.%s()` has nil value", moduleName, name)
				}
				//fmt.Printf("V: %+v\n", value) // debug
				return value, nil
			},
		})
	}

	return nil
}
