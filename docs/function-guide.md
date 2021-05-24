# Function guide

## Overview

The `mgmt` tool has built-in functions which add useful, reactive functionality
to the language. This guide describes the different function API's that are
available. It is meant to instruct developers on how to write new functions.
Since `mgmt` and the core functions are written in golang, some prior golang
knowledge is assumed.

## Theory

Functions in `mgmt` are similar to functions in other languages, however they
also have a [reactive](https://en.wikipedia.org/wiki/Functional_reactive_programming)
component. Our functions can produce events over time, and there are different
ways to write functions. For some background on this design, please read the
[original article](https://purpleidea.com/blog/2018/02/05/mgmt-configuration-language/)
on the subject.

## Native Functions

Native functions are functions which are implemented in the mgmt language
itself. These are currently not available yet, but are coming soon. Stay tuned!

## Simple Function API

Most functions should be implemented using the simple function API. This API
allows you to implement simple, static, [pure](https://en.wikipedia.org/wiki/Pure_function)
functions that don't require you to write much boilerplate code. They will be
automatically re-evaluated as needed when their input values change. These will
all be automatically made available as helper functions within mgmt templates,
and are also available for use anywhere inside mgmt programs.

You'll need some basic knowledge of using the [`types`](https://github.com/purpleidea/mgmt/tree/master/lang/types)
library which is included with mgmt. This library lets you interact with the
available types and values in the mgmt language. It is very easy to use, and
should be fairly intuitive. Most of what you'll need to know can be inferred
from looking at example code.

To implement a function, you'll need to create a file that imports the
[`lang/funcs/simple/`](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/simple/)
module. It should probably get created in the correct directory inside of:
[`lang/funcs/core/`](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/core/).
The function should be implemented as a `FuncValue` in our type system. It is
then registered with the engine during `init()`. An example explains it best:

### Example

```golang
package simple

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

// you must register your functions in init when the program starts up
func init() {
	// Example function that squares an int and prints out answer as an str.
	simple.ModuleRegister(ModuleName, "talkingsquare", &types.FuncValue{
		T: types.NewType("func(int) str"), // declare the signature
		V: func(input []types.Value) (types.Value, error) {
			i := input[0].Int() // get first arg as an int64
			// must return the above specified value
			return &types.StrValue{
				V: fmt.Sprintf("%d^2 is %d", i, i * i),
			}, nil // no serious errors occurred
		},
	})
}
```

This simple function accepts one `int` as input, and returns one `str`.
Functions can have zero or more inputs, and must have exactly one output. You
must be sure to use the `types` library correctly, since if you try and access
an input which should not exist (eg: `input[2]`, when there are only two
that are expected), then you will cause a panic. If you have declared that a
particular argument is an `int` but you try to read it with `.Bool()` you will
also cause a panic. Lastly, make sure that you return a value in the correct
type or you will also cause a panic!

If anything goes wrong, you can return an error, however this will cause the
mgmt engine to shutdown. It should be seen as the equivalent to calling a
`panic()`, however it is safer because it brings the engine down cleanly.
Ideally, your functions should never need to error. You should never cause a
real `panic()`, since this could have negative consequences to the system.

## Simple Polymorphic Function API

Most functions should be implemented using the simple function API. If they need
to have multiple polymorphic forms under the same name, then you can use this
API. This is useful for situations when it would be unhelpful to name the
functions differently, or when the number of possible signatures for the
function would be infinite.

The canonical example of this is the `len` function which returns the number of
elements in either a `list` or a `map`. Since lists and maps are two different
types, you can see that polymorphism is more convenient than requiring a
`listlen` and `maplen` function. Nevertheless, it is also required because a
`list of int` is a different type than a `list of str`, which is a different
type than a `list of list of str` and so on. As you can see the number of
possible input types for such a `len` function is infinite.

Another downside to implementing your functions with this API is that they will
*not* be made available for use inside templates. This is a limitation of the
`golang` template library. In the future if this limitation proves to be
significantly annoying, we might consider writing our own template library.

As with the simple, non-polymorphic API, you can only implement [pure](https://en.wikipedia.org/wiki/Pure_function)
functions, without writing too much boilerplate code. They will be automatically
re-evaluated as needed when their input values change.

To implement a function, you'll need to create a file that imports the
[`lang/funcs/simplepoly/`](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/simplepoly/)
module. It should probably get created in the correct directory inside of:
[`lang/funcs/core/`](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/core/).
The function should be implemented as a list of `FuncValue`'s in our type
system. It is then registered with the engine during `init()`. You may also use
the `variant` type in your type definitions. This special type will never be
seen inside a running program, and will get converted to a concrete type if a
suitable match to this signature can be found. Be warned that signatures which
contain too many variants, or which are very general, might be hard for the
compiler to match, and ambiguous type graphs make for user compiler errors. The
top-level type must still be a function type, it may only contain variants as
part of its signature. It is probably more difficult to unify a function if its
return type is a variant, as opposed to if one of its args was.

An example explains it best:

### Example

```golang
import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	// You may use the simplepoly.ModuleRegister method to register your
	// function if it's in a module, as seen in the simple function example.
	simplepoly.Register("len", []*types.FuncValue{
		{
			T: types.NewType("func([]variant) int"),
			V: Len,
		},
		{
			T: types.NewType("func({variant: variant}) int"),
			V: Len,
		},
	})
}

// Len returns the number of elements in a list or the number of key pairs in a
// map. It can operate on either of these types.
func Len(input []types.Value) (types.Value, error) {
	var length int
	switch k := input[0].Type().Kind; k {
	case types.KindList:
		length = len(input[0].List())
	case types.KindMap:
		length = len(input[0].Map())

	default:
		return nil, fmt.Errorf("unsupported kind: %+v", k)
	}

	return &types.IntValue{
		V: int64(length),
	}, nil
}
```

This simple polymorphic function can accept an infinite number of signatures, of
which there are two basic forms. Both forms return an `int` as is seen above.
The first form takes a `[]variant` which means a `list` of `variant`'s, which
means that it can be a list of any type, since `variant` itself is not a
concrete type. The second form accepts a `{variant: variant}`, which means that
it accepts any form of `map` as input.

The implementation for both of these forms is the same: it is handled by the
same `Len` function which is clever enough to be able to deal with any of the
type signatures possible from those two patterns.

At compile time, if your `mcl` code type checks correctly, a concrete type will
be known for each and every usage of the `len` function, and specific values
will be passed in for this code to compute the length of. As usual, make sure to
only write safe code that will not panic! A panic is a bug. If you really cannot
continue, then you must return an error.

## Function API

To implement a reactive function in `mgmt` it must satisfy the
[`Func`](https://github.com/purpleidea/mgmt/blob/master/lang/interfaces/func.go)
interface. Using the [Simple Function API](#simple-function-api) is preferable
if it meets your needs. Most functions will be able to use that API. If you
really need something more powerful, then you can use the regular function API.
What follows are each of the method signatures and a description of each.

### Info

```golang
Info() *interfaces.Info
```

This returns some information about the function. It is necessary so that the
compiler can type check the code correctly, and know what optimizations can be
performed. This is usually the first method which is called by the engine.

#### Example

```golang
func (obj *FooFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Sig:  types.NewType("func(a int) str"),
	}
}
```

### Init

```golang
Init(init *interfaces.Init) error
```

This is called to initialize the function. If something goes wrong, it should
return an error. It is passed a struct that contains all the important
information and pointers that it might need to work with throughout its
lifetime. As a result, it will need to save a copy to that pointer for future
use in the other methods.

#### Example

```golang
// Init runs some startup code for this function.
func (obj *FooFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{}) // shutdown signal
	return nil
}
```

### Close

```golang
Close() error
```

This is called to cleanup the function. It usually causes the stream to
shutdown. Even if `Stream()` decided to shutdown early, it might still get
called. It is usually called by the engine to tell the function to shutdown.

#### Example

```golang
// Close runs some shutdown code for this function and turns off the stream.
func (obj *FooFunc) Close() error {
	close(obj.closeChan) // send a signal to tell the stream to close
	return nil
}
```

### Stream

```golang
Stream() error
```

`Stream` is where the real _work_ is done. This method is started by the
language function engine. It will run this function while simultaneously sending
it values on the `input` channel. It will only send a complete set of input
values. You should send a value to the output channel when you have decided that
one should be produced. Make sure to only use input values of the expected type
as declared in the `Info` struct, and send values of the similarly declared
appropriate return type. Failure to do so will may result in a panic and
sadness.

#### Example

```golang
// Stream returns the single value that was generated and then closes.
func (obj *FooFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	var result string
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}

			ix := input.Struct()["a"].Int()
			if ix < 0 {
				return fmt.Errorf("we can't deal with negatives")
			}

			result = fmt.Sprintf("the input is: %d", ix)

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: result,
		}:

		case <-obj.closeChan:
			return nil
		}
	}
}
```

As you can see, we read our inputs from the `input` channel, and write to the
`output` channel. Our code is careful to never block or deadlock, and can always
exit if a close signal is requested. It also cleans up after itself by closing
the `output` channel when it is done using it. This is done easily with `defer`.
If it notices that the `input` channel closes, then it knows that no more input
values are coming and it can consider shutting down early.

## Further considerations

There is some additional information that any function author will need to know.
Each issue is listed separately below!

### Function struct

Each function will implement methods as pointer receivers on a function struct.
The naming convention for resources is that they end with a `Func` suffix.

#### Example

```golang
type FooFunc struct {
	init *interfaces.Init

	// this space can be used if needed

	closeChan chan struct{} // shutdown signal
}
```

### Function registration

All functions must be registered with the engine so that they can be found. This
also ensures they can be encoded and decoded. Make sure to include the following
code snippet for this to work.

```golang
import "github.com/purpleidea/mgmt/lang/funcs"

func init() { // special golang method that runs once
	funcs.Register("foo", func() interfaces.Func { return &FooFunc{} })
}
```

Functions inside of built-in modules will need to use the `ModuleRegister`
method instead.

```golang
// moduleName is already set to "math" by the math package. Do this in `init`.
funcs.ModuleRegister(moduleName, "cos", func() interfaces.Func { return &CosFunc{} })
```

### Composite functions

Composite functions are functions which import one or more existing functions.
This is useful to prevent code duplication in higher level function scenarios.
Unfortunately no further documentation about this subject has been written. To
expand this section, please send a patch! Please contact us if you'd like to
work on a function that uses this feature, or to add it to an existing one!
We don't expect this functionality to be particularly useful or common, as it's
probably easier and preferable to simply import common golang library code into
multiple different functions instead.

## Polymorphic Function API

The polymorphic function API is an API that lets you implement functions which
do not necessarily have a single static function signature. After compile time,
all functions must have a static function signature. We also know that there
might be different ways you would want to call `printf`, such as:
`printf("the %s is %d", "answer", 42)` or `printf("3 * 2 = %d", 3 * 2)`. Since
you couldn't implement the infinite number of possible signatures, this API lets
you write code which can be coerced into different forms. This makes
implementing what would appear to be generic or polymorphic, instead of
something that is actually static and that still has the static type safety
properties that were guaranteed by the mgmt language.

Since this is an advanced topic, it is not described in full at this time. For
more information please have a look at the source code comments, some of the
existing implementations, and ask around in the community.

## Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### Can I use global variables?

Probably not. You must assume that multiple copies of your function may be used
at the same time. If they require a global variable, it's likely this won't
work. Instead it's probably better to use a struct local variable if you need to
store some state.

There might be some rare instances where a global would be acceptable, but if
you need one of these, you're probably already an internals expert. If you think
they need to lock or synchronize so as to not overwhelm an external resource,
then you have to be especially careful not to cause deadlocking the mgmt engine.

### Can I write functions in a different language?

Currently `golang` is the only supported language for built-in functions. We
might consider allowing external functions to be imported in the future. This
will likely require a language that can expose a C-like API, such as `python` or
`ruby`. Custom `golang` functions are already possible when using mgmt as a lib.

### What new functions need writing?

There are still many ideas for new functions that haven't been written yet. If
you'd like to contribute one, please contact us and tell us about your idea!

### Can I generate many different `FuncValue` implementations from one function?

Yes, you can use a function generator in `golang` to build multiple different
implementations from the same function generator. You just need to implement a
function which *returns* a `golang` type of `func([]types.Value) (types.Value, error)`
which is what `FuncValue` expects. The generator function can use any input it
wants to build the individual functions, thus helping with code re-use.

### How do I determine the signature of my simple, polymorphic function?

The determination of the input portion of the function signature can be
determined by inspecting the length of the input, and the specific type each
value has. Length is done in the standard `golang` way, and the type of each
element can be ascertained with the `Type()` method available on every value.

Knowing the output type is trickier. If it can not be inferred in some manner,
then the only way is to keep track of this yourself. You can use a function
generator to build your `FuncValue` implementations, and pass in the unique
signature to each one as you are building them. Using a generator is a common
technique which was mentioned previously.

One obvious situation where this might occur is if your function doesn't take
any inputs! An example `math.fortytwo()` function was implemented that
demonstrates the use of function generators to pass the type signatures into the
implementations.

### Where can I find more information about mgmt?

Additional blog posts, videos and other material [is available!](https://github.com/purpleidea/mgmt/blob/master/docs/on-the-web.md).

## Suggestions

If you have any ideas for API changes or other improvements to function writing,
please let us know! We're still pre 1.0 and pre 0.1 and happy to break API in
order to get it right!
