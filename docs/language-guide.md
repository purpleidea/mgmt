# Language guide

## Overview

The `mgmt` tool has various frontends, each of which may produce a stream of
between zero or more graphs that are passed to the engine for desired state
application. In almost all scenarios, you're going to want to use the language
frontend. This guide describes some of the internals of the language.

## Theory

The mgmt language is a declarative (immutable) functional, reactive programming
language. It is implemented in `golang`. A longer introduction to the language
is [available as a blog post here](https://purpleidea.com/blog/2018/02/05/mgmt-configuration-language/)!

### Types

All expressions must have a type. A composite type such as a list of strings
(`[]str`) is different from a list of integers (`[]int`).

There _is_ a _variant_ type in the language's type system, but it is only used
internally and only appears briefly when needed for type unification hints
during static polymorphic function generation. This is an advanced topic which
is not required for normal usage of the software.

The implementation of the internal types can be found in
[lang/types/](https://github.com/purpleidea/mgmt/tree/master/lang/types/).

#### bool

A `true` or `false` value.

#### str

Any `"string!"` enclosed in quotes.

#### int

A number like `42` or `-13`. Integers are represented internally as golang's
`int64`.

#### float

A floating point number like: `3.1415926`. Float's are represented internally as
golang's `float64`.

#### list

An ordered collection of values of the same type, eg: `[6, 7, 8, 9,]`. It is
worth mentioning that empty lists have a type, although without type hints it
can be impossible to infer the item's type.

#### map

An unordered set of unique keys of the same type and corresponding value pairs
of another type, eg:
`{"boiling" => 100, "freezing" => 0, "room" => "25", "house" => 22, "canada" => -30,}`.
That is to say, all of the keys must have the same type, and all of the values
must have the same type. You can use any type for either, although it is
probably advisable to avoid using very complex types as map keys.

#### struct

An ordered set of field names and corresponding values, each of their own type,
eg: `struct{answer => "42", james => "awesome", is_mgmt_awesome => true,}`.
These are useful for combining more than one type into the same value. Note the
syntactical difference between these and map's: the key's in map's have types,
and as a result, string keys are enclosed in quotes, whereas struct _fields_ are
not string values, and as such are bare and specified without quotes.

#### func

An ordered set of optionally named, differently typed input arguments, and a
return type, eg: `func(s str) int` or:
`func(bool, []str, {str: float}) struct{foo str; bar int}`.

### Expressions

Expressions, and the `Expr` interface need to be better documented. For now
please consume
[lang/interfaces/ast.go](https://github.com/purpleidea/mgmt/tree/master/lang/interfaces/ast.go).
These docs will be expanded on when things are more certain to be stable.

### Statements

There are a very small number of statements in our language. They include:

- **bind**: bind's an expression to a variable within that scope
	- eg: `$x = 42`
- **if**: produces up to one branch of statements based on a conditional
expression

	```
	if <conditional> {
		<statements>
	} else {
		# the else branch is optional for if statements
		<statements>
	}
	```

- **resource**: produces a resource

	```
	file "/tmp/hello" {
		content => "world",
		mode => "o=rwx",
	}
	```

- **edge**: produces an edge

	```
	File["/tmp/hello"] -> Print["alert4"]
	```

All statements produce _output_. Output consists of between zero and more
`edges` and `resources`. A resource statement can produce a resource, whereas an
`if` statement produces whatever the chosen branch produces. Ultimately the goal
of executing our programs is to produce a list of `resources`, which along with
the produced `edges`, is built into a resource graph. This graph is then passed
to the engine for desired state application.

#### Bind

This section needs better documentation.

#### If

This section needs better documentation.

#### Resource

This section needs better documentation.

#### Edge

Edges express dependencies in the graph of resources which are output. They can
be chained as a pair, or in any greater number. For example, you may write:

```
Pkg["drbd"] -> File["/etc/drbd.conf"] -> Svc["drbd"]
```

to express a relationship between three resources. The first character in the
resource kind must be capitalized so that the parser can't ascertain
unambiguously that we are referring to a dependency relationship.

### Stages

The mgmt compiler runs in a number of stages. In order of execution they are:
* [Lexing](#lexing)
* [Parsing](#parsing)
* [Interpolation](#interpolation)
* [Scope propagation](#scope-propagation)
* [Type unification](#type-unification)
* [Function graph generation](#function-graph-generation)
* [Function engine creation and validation](#function-engine-creation-and-validation)

All of the above needs to be done every time the source code changes. After this
point, the [function engine runs](#function-engine-running-and-interpret) and
produces events. On every event, we "[interpret](#function-engine-running-and-interpret)"
which produces a resource graph. This series of resource graphs are passed
to the engine as they are produced.

What follows are some notes about each step.

#### Lexing

Lexing is done using [nex](https://github.com/blynn/nex). It is a pure-golang
implementation which is similar to _Lex_ or _Flex_, but which produces golang
code instead of C. It integrates reasonably well with golang's _yacc_ which is
used for parsing. The token definitions are in:
[lang/lexer.nex](https://github.com/purpleidea/mgmt/tree/master/lang/lexer.nex).
Lexing and parsing run together by calling the `LexParse` method.

#### Parsing

The parser used is golang's implementation of
[yacc](https://godoc.org/golang.org/x/tools/cmd/goyacc). The documentation is
quite abysmal, so it's helpful to rely on the documentation from standard yacc
and trial and error. One small advantage yacc has over standard yacc is that it
can produce error messages from examples. The best documentation is to examine
the source. There is a short write up available [here](https://research.swtch.com/yyerror).
The yacc file exists at:
[lang/parser.y](https://github.com/purpleidea/mgmt/tree/master/lang/parser.y).
Lexing and parsing run together by calling the `LexParse` method.

#### Interpolation

Interpolation is used to transform the AST (which was produced from lexing and
parsing) into one which is either identical or different. It expands strings
which might contain expressions to be interpolated (eg: `"the answer is: ${foo}"`)
and can be used for other scenarios in which one statement or expression would
be better represented by a larger AST. Most nodes in the AST simply return their
own node address, and do not modify the AST.

#### Scope propagation

Scope propagation passes the parent scope (starting with the top-level, built-in
scope) down through the AST. This is necessary so that children nodes can access
variables in the scope if needed. Most AST node's simply pass on the scope
without making any changes. The `ExprVar` node naturally consumes scope's and
the `StmtProg` node cleverly passes the scope through in the order expected for
the out-of-order bind logic to work.

#### Type unification

Each expression must have a known type. The unpleasant option is to force the
programmer to specify by annotation every type throughout their whole program
so that each `Expr` node in the AST knows what to expect. Type annotation is
allowed in situations when you want to explicitly specify a type, or when the
compiler cannot deduce it, however, most of it can usually be inferred.

For type inferrence to work, each node in the AST implements a `Unify` method
which is able to return a list of invariants that must hold true. This starts at
the top most AST node, and gets called through to it's children to assemble a
giant list of invariants. The invariants can take different forms. They can
specify that a particular expression must have a particular type, or they can
specify that two expressions must have the same types. More complex invariants
allow you to specify relationships between different types and expressions.
Furthermore, invariants can allow you to specify that only one invariant out of
a set must hold true.

Once the list of invariants has been collected, they are run through an
invariant solver. The solver can return either return successfully or with an
error. If the solver returns successfully, it means that it has found a trivial
mapping between every expression and it's corresponding type. At this point it
is a simple task to run `SetType` on every expression so that the types are
known. If the solver returns in error, it is usually due to one of two
possibilities:

1. Ambiguity

	The solver does not have enough information to make a definitive or
	unique determination about the expression to type mappings. The set of
	invariants is ambiguous, and we cannot continue. An error will be
	returned to the programmer. In this scenario the user will probably need
	to add a type annotation, possibly because of a design bug in the user's
	program.

2. Conflict

	The solver has conflicting information that cannot be reconciled. In
	this situation an explicit conflict has been found. If two invariants
	are found which both expect a particular expression to have different
	types, then it is not possible to find a valid solution. This almost
	always happens if the user has made a type error in their program.

Only one solver currently exists, but it is possible to easily plug in an
alternate implementation if someone more skilled in the art of solver design
would like to propose a more logical or performant variant.

#### Function graph generation

At this point we have a fully type AST. The AST must now be transformed into a
directed, acyclic graph (DAG) data structure that represents the flow of data as
necessary for everything to be reactive. Note that this graph is *different*
from the resource graph which is produced and sent to the engine. It is just a
coincidence that both happen to be DAG's. (You don't freak out when you see a
list data structure show up in more than one place, do you?)

To produce this graph, each node has a `Graph` method which it can call. This
starts at the top most node, and is called down through the AST. The edges in
the graphs must represent the individual expression values which are passed
from node to node. The names of the edges must match the function type argument
names which are used in the definition of the corresponding function. These
corresponding functions must exist for each expression node and are produced by
calling that expression's `Func` method. These are usually called by the
function engine during function creation and validation.

#### Function engine creation and validation

Finally we have a graph of the data flows. The function engine must first
initialize which creates references to each of the necessary function
implementations, and gets information about each one. It then needs to be type
checked to ensure that the data flows all correctly match what is expected. If
you were to pass an `int` to a function expecting a `bool`, this would be a
problem. If all goes well, the program should get run shortly.

#### Function engine running and interpret

At this point the function engine runs. It produces a stream of events which
cause the `Output()` method of the top-level program to run, which produces the
list of resources and edges. These are then transformed into the resource graph
which is passed to the engine.

### Function API

If you'd like to create a built-in, core function, you'll need to implement the
function API interface named `Func`. It can be found in
[lang/interfaces/func.go](https://github.com/purpleidea/mgmt/tree/master/lang/interfaces/func.go).
Your function must have a specific type. For example, a simple math function
might have a signature of `func(x int, x int) int`. As you can see, all the
types are known _before_ compile time.

A separate discussion on this matter can be found in the [function guide](function-guide.md).

What follows are each of the method signatures and a description of each.
Failure to implement the API correctly can cause the function graph engine to
block, or the program to panic.

### Info

```golang
Info() *Info
```

The Info method must return a struct containing some information about your
function. The struct has the following type:

```golang
type Info struct {
	Sig  *types.Type // the signature of the function, must be KindFunc
}
```

You must implement this correctly. Other fields in the `Info` struct may be
added in the future. This method is usually called before any other, and should
not depend on any other method being called first. Other methods must not depend
on this method being called first.

#### Example

```golang
func (obj *FooFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Sig: types.NewType("func(a str, b int) float"),
	}
}
```

### Init

```golang
Init(*Init) error
```

Init is called by the function graph engine to create an implementation of this
function. It is passed in a struct of the following form:

```golang
type Init struct {
	Hostname string // uuid for the host
	Input  chan types.Value // Engine will close `input` chan
	Output chan types.Value // Stream must close `output` chan
	World  resources.World
	Debug  bool
	Logf   func(format string, v ...interface{})
}
```

These values and references may be used (wisely) inside your function. `Input`
will contain a channel of input structs matching the expected input signature
for your function. `Output` will be the channel which you must send values to
whenever a new value should be produced. This must be done in the `Stream()`
function. You may carefully use `World` to access functionality provided by the
engine. You may use `Logf` to log informational messages, however there is no
guarantee that they will be displayed to the user. `Debug` specifies whether the
function is running in a user-requested debug mode. This might cause you to want
to print more log messages for example. You will need to save references to any
or all of these info fields that you wish to use in the struct implementing this
`Func` interface. At a minimum you will need to save `Output` as a minimum of
one value must be produced.

#### Example

```golang
Please see the example functions in
[lang/funcs/core/](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/core/).
```

### Stream

```golang
Stream() error
```

Stream is called by the function engine when it is ready for your function to
start accepting input and producing output. You must always produce at least one
value. Failure to produce at least one value will probably cause the function
engine to hang waiting for your output. This function must close the `Output`
channel when it has no more values to send. The engine will close the `Input`
channel when it has no more values to send. This may or may not influence
whether or not you close the `Output` channel.

#### Example

```golang
Please see the example functions in
[lang/funcs/core/](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/core/).
```

### Close

```golang
Close() error
```

Close asks the particular function to shutdown its `Stream()` function and
return.

#### Example

```golang
Please see the example functions in
[lang/funcs/core/](https://github.com/purpleidea/mgmt/tree/master/lang/funcs/core/).
```

### Polymorphic Function API

For some functions, it might be helpful to be able to implement a function once,
but to have multiple polymorphic variants that can be chosen at compile time.
For this more advanced topic, you will need to use the
[Polymorphic Function API](#polymorphic-function-api). This will help with code
reuse when you have a small, finite number of possible type signatures, and also
for more complicated cases where you might have an infinite number of possible
type signatures. (eg: `[]str`, or `[][]str`, or `[][][]str`, etc...)

Suppose you want to implement a function which can assume different type
signatures. The mgmt language does not support polymorphic types-- you must use
static types throughout the language, however, it is legal to implement a
function which can take different specific type signatures based on how it is
used. For example, you might wish to add a math function which could take the
form of `func(x int, x int) int` or `func(x float, x float) float` depending on
the input values. You might also want to implement a function which takes an
arbitrary number of input arguments (the number must be statically fixed at the
compile time of your program though) and which returns a string.

The `PolyFunc` interface adds additional methods which you must implement to
satisfy such a function implementation. If you'd like to implement such a
function, then please notify the project authors, and they will expand this
section with a longer description of the process.

#### Examples

What follows are a few examples that might help you understand some of the
language details.

##### Example Foo

TODO: please add an example here!

##### Example Bar

TODO: please add an example here!

## Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### What is the difference between `ExprIf` and `StmtIf`?

The language contains both an `if` expression, and and `if` statement. An `if`
expression takes a boolean conditional *and* it must contain exactly _two_
branches (a `then` and an `else` branch) which each contain one expression. The
`if` expression _will_ return the value of one of the two branches based on the
conditional.

#### Example:

```mcl
# this is an if expression, and both branches must exist
$b = true
$x = if $b {
	42
} else {
	-13
}
```

The `if` statement also takes a boolean conditional, but it may have either one
or two branches. Branches must only directly contain statements. The `if`
statement does not return any value, but it does produce output when it is
evaluated. The output consists primarily of resources (vertices) and edges.

#### Example:

```mcl
# this is an if statement, and in this scenario the else branch was omitted
$b = true
if $b {
	file "/tmp/hello" {
		content => "world",
	}
}
```

### What is the difference `types.Value.Str()` and `types.Value.String()`?

In the `lang/types` library, there is a `types.Value` interface. Every value in
our type system must implement this interface. One of the methods in this
interface is the `String() string` method. This lets you print a representation
of the value. You will probably never need to use this method.

In addition, the `types.Value` interface implements a number of helper functions
which return the value as an equivalent golang type. If you know that the value
is a `bool`, you can call `x.Bool()` on it. If it's a `string` you can call
`x.Str()`. Make sure not to call one of those type methods unless you know the
value is of that type, or you will trigger a panic!

### I created a `&ListValue{}` but it's not working!

If you create a base type like `bool`, `str`, `int`, or `float`, all you need to
do is build the `&BoolValue` and set the `V` field. Eg:

```golang
someBool := &types.BoolValue{V: true}
```

If you are building a container type like `list`, `map`, `struct`, or `func`,
then you *also* need to specify the type of the contained values. This is
because a list has a type of `[]str`, or `[]int`, or even `[][]foo`. Eg:

```golang
someListOfStrings := &types.ListValue{
	T: types.NewType("[]str"),	# must match the contents!
	V: []types.Value{
		&types.StrValue{V: "a"},
		&types.StrValue{V: "bb"},
		&types.StrValue{V: "ccc"},
	},
}
```

If you don't build these properly, then you will cause a panic! Even empty lists
have a type.

### I don't like the mgmt language, is there an alternative?

Yes, the language is just one of the available "frontends" that passes a stream
of graphs to the engine "backend". While it _is_ the recommended way of using
mgmt, you're welcome to either use an alternate frontend, or write your own. To
write your own frontend, you must implement the
[GAPI](https://github.com/purpleidea/mgmt/blob/master/gapi/gapi.go) interface.

### I'm an expert in FRP, and you got it all wrong; even the names of things!

I am certainly no expert in FRP, and I've certainly got lots more to learn. One
thing FRP experts might notice is that some of the concepts from FRP are either
named differently, or are notably absent.

In mgmt, we don't talk about behaviours, events, or signals in the strict FRP
definitons of the words. Firstly, because we only support discretized, streams
of values with no plan to add continuous semantics. Secondly, because we prefer
to use terms which are more natural and relatable to what our target audience is
expecting. Our users are more likely to have a background in Physiology, or
systems administration than a background in FRP.

Having said that, we hope that the FRP community will engage with us and help
improve the parts that we got wrong. Even if that means adding continuous
behaviours!

### This is brilliant, may I give you a high-five?

Thank you, and yes, probably. "Props" may also be accepted, although patches are
preferred. If you can't do either, [donations](https://purpleidea.com/misc/donate/)
to support the project are welcome too!

### Where can I find more information about mgmt?

Additional blog posts, videos and other material
[is available!](https://github.com/purpleidea/mgmt/blob/master/docs/on-the-web.md).

## Suggestions

If you have any ideas for changes or other improvements to the language, please
let us know! We're still pre 1.0 and pre 0.1 and happy to change it in order to
get it right!
