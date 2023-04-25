package fancyfunc

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// FuncValue represents a function value, for example a built-in or a lambda.
//
// In most languages, we can simply call a function with a list of arguments and
// expect to receive a single value. In this language, however, a function might
// be something like datetime.now() or fn(n) {shell(Sprintf("seq %d", n))},
// which might not produce a value immediately, and might then produce multiple
// values over time. Thus, in this language, a FuncValue does not receive
// Values, instead it receives input Func nodes. The FuncValue then adds more
// Func nodes and edges in order to arrange for output values to be sent to a
// particular output node, which the function returns so that the caller may
// connect that output node to more nodes down the line.
//
// The function can also return an error which could represent that something
// went horribly wrong. (Think, an internal panic.)
type FuncValue struct {
	V func(interfaces.Txn, []interfaces.Func) (interfaces.Func, error)
	T *types.Type // contains ordered field types, arg names are a bonus part
}

// NewFunc creates a new function with the specified type.
func NewFunc(t *types.Type) *FuncValue {
	if t.Kind != types.KindFunc {
		return nil // sanity check
	}
	v := func(interfaces.Txn, []interfaces.Func) (interfaces.Func, error) {
		return nil, fmt.Errorf("nil function") // TODO: is this correct?
	}
	return &FuncValue{
		V: v,
		T: t,
	}
}

// String returns a visual representation of this value.
func (obj *FuncValue) String() string {
	return fmt.Sprintf("func(%+v)", obj.T) // TODO: can't print obj.V w/o vet warning
}

// Type returns the type data structure that represents this type.
func (obj *FuncValue) Type() *types.Type { return obj.T }

// Less compares to value and returns true if we're smaller. This panics if the
// two types aren't the same.
func (obj *FuncValue) Less(v types.Value) bool {
	panic("functions are not comparable")
}

// Cmp returns an error if this value isn't the same as the arg passed in.
func (obj *FuncValue) Cmp(val types.Value) error {
	if obj == nil || val == nil {
		return fmt.Errorf("cannot cmp to nil")
	}
	if err := obj.Type().Cmp(val.Type()); err != nil {
		return errwrap.Wrapf(err, "cannot cmp types")
	}

	return fmt.Errorf("cannot cmp funcs") // TODO: can we ?
}

// Copy returns a copy of this value.
func (obj *FuncValue) Copy() types.Value {
	panic("cannot implement Copy() for FuncValue, because FuncValue is a FancyValue, not a Value")
}

// Value returns the raw value of this type.
func (obj *FuncValue) Value() interface{} {
	panic("TODO [SimpleFn] [Reflect]: what's all this reflection stuff for?")
	//typ := obj.T.Reflect()

	//// wrap our function with the translation that is necessary
	//fn := func(args []reflect.Value) (results []reflect.Value) { // build
	//	innerArgs := []Value{}
	//	for _, x := range args {
	//		v, err := ValueOf(x) // reflect.Value -> Value
	//		if err != nil {
	//			panic(fmt.Sprintf("can't determine value of %+v", x))
	//		}
	//		innerArgs = append(innerArgs, v)
	//	}
	//	result, err := obj.V(innerArgs) // call it
	//	if err != nil {
	//		// when calling our function with the Call method, then
	//		// we get the error output and have a chance to decide
	//		// what to do with it, but when calling it from within
	//		// a normal golang function call, the error represents
	//		// that something went horribly wrong, aka a panic...
	//		panic(fmt.Sprintf("function panic: %+v", err))
	//	}
	//	return []reflect.Value{reflect.ValueOf(result.Value())} // only one result
	//}
	//val := reflect.MakeFunc(typ, fn)
	//return val.Interface()
}

// Bool represents the value of this type as a bool if it is one. If this is not
// a bool, then this panics.
func (obj *FuncValue) Bool() bool {
	panic("not a bool")
}

// Str represents the value of this type as a string if it is one. If this is
// not a string, then this panics.
func (obj *FuncValue) Str() string {
	panic("not an str") // yes, i think this is the correct grammar
}

// Int represents the value of this type as an integer if it is one. If this is
// not an integer, then this panics.
func (obj *FuncValue) Int() int64 {
	panic("not an int")
}

// Float represents the value of this type as a float if it is one. If this is
// not a float, then this panics.
func (obj *FuncValue) Float() float64 {
	panic("not a float")
}

// List represents the value of this type as a list if it is one. If this is not
// a list, then this panics.
func (obj *FuncValue) List() []types.Value {
	panic("not a list")
}

// Map represents the value of this type as a dictionary if it is one. If this
// is not a map, then this panics.
func (obj *FuncValue) Map() map[types.Value]types.Value {
	panic("not a list")
}

// Struct represents the value of this type as a struct if it is one. If this is
// not a struct, then this panics.
func (obj *FuncValue) Struct() map[string]types.Value {
	panic("not a struct")
}

// Func represents the value of this type as a function if it is one. If this is
// not a function, then this panics.
func (obj *FuncValue) Func() func([]pgraph.Vertex) (pgraph.Vertex, error) {
	panic("cannot implement Func() for FuncValue, because FuncValue manipulates the graph, not just returns a value")
}

// Set sets the function value to be a new function.
func (obj *FuncValue) Set(fn func(interfaces.Txn, []interfaces.Func) (interfaces.Func, error)) error { // TODO: change method name?
	obj.V = fn
	return nil // TODO: can we do any sort of checking here?
}

func (obj *FuncValue) Call(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
	return obj.V(txn, args)
}
