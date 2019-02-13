# Resource guide

## Overview

The `mgmt` tool has built-in resource primitives which make up the building
blocks of any configuration. Each instance of a resource is mapped to a single
vertex in the resource [graph](https://en.wikipedia.org/wiki/Directed_acyclic_graph).
This guide is meant to instruct developers on how to write a brand new resource.
Since `mgmt` and the core resources are written in golang, some prior golang
knowledge is assumed.

## Theory

Resources in `mgmt` are similar to resources in other systems in that they are
[idempotent](https://en.wikipedia.org/wiki/Idempotence). Our resources are
uniquely different in that they can detect when their state has changed, and as
a result can run to revert or repair this change instantly. For some background
on this design, please read the
[original article](https://purpleidea.com/blog/2016/01/18/next-generation-configuration-mgmt/)
on the subject.

## Resource Prerequisites

### Imports

You'll need to import a few packages to make writing your resource easier. Here
is the list:

```
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
```

The `engine` package contains most of the interfaces and helper functions that
you'll need to use. The `traits` package contains some base functionality which
you can use to easily add functionality to your resource without needing to
implement it from scratch.

### Resource struct

Each resource will implement methods as pointer receivers on a resource struct.
The naming convention for resources is that they end with a `Res` suffix.

The resource struct should include an anonymous reference to the `Base` trait.
Other `traits` can be added to the resource to add additional functionality.
They are discussed below.

You'll most likely want to store a reference to the `*Init` struct type as
defined by the engine. This is data that the engine will provide to your
resource on Init.

Lastly you should define the public fields that make up your resource API, as
well as any private fields that you might want to use throughout your resource.
Do _not_ depend on global variables, since multiple copies of your resource
could get instantiated.

You'll want to add struct tags based on the different frontends that you want
your resources to be able to use. Some frontends can infer this information if
it is not specified, but others cannot, and some might poorly infer if the
struct name is ambiguous.

If you'd like your resource to be accessible by the `YAML` graph API (GAPI),
then you'll need to include the appropriate YAML fields as shown below. This is
used by the `Puppet` compiler as well, so make sure you include these struct
tags if you want existing `Puppet` code to be able to run using the `mgmt`
engine.

#### Example

```golang
type FooRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Groupable
	traits.Refreshable

	init *engine.Init

	Whatever string `lang:"whatever" yaml:"whatever"` // you pick!
	Baz      bool   `lang:"baz" yaml:"baz"`           // something else

	something string // some private field
}
```

## Resource API

To implement a resource in `mgmt` it must satisfy the
[`Res`](https://github.com/purpleidea/mgmt/blob/master/engine/resources.go)
interface. What follows are each of the method signatures and a description of
each.

### Default

```golang
Default() engine.Res
```

This returns a populated resource struct as a `Res`. It shouldn't populate any
values which already have the correct default as the golang zero value. In
general it is preferable if the zero values make for the correct defaults.

#### Example

```golang
// Default returns some sensible defaults for this resource.
func (obj *FooRes) Default() Res {
	return &FooRes{
		Answer: 42, // sometimes, defaults shouldn't be the zero value
	}
}
```

### Validate

```golang
Validate() error
```

This method is used to validate if the populated resource struct is a valid
representation of the resource kind. If it does not conform to the resource
specifications, it should return an error. If you notice that this method is
quite large, it might be an indication that you should reconsider the parameter
list and interface to this resource. This method is called by the engine
_before_ `Init`. It can also be called occasionally after a Send/Recv operation
to verify that the newly populated parameters are valid. Remember not to expect
access to the outside world when using this.

#### Example

```golang
// Validate reports any problems with the struct definition.
func (obj *FooRes) Validate() error {
	if obj.Answer != 42 { // validate whatever you want
		return fmt.Errorf("expected an answer of 42")
	}
	return nil
}
```

### Init

```golang
Init() error
```

This is called to initialize the resource. If something goes wrong, it should
return an error. It should do any resource specific work such as initializing
channels, sync primitives, or anything else that is relevant to your resource.
If it is not need throughout, it might be preferable to do some initialization
and tear down locally in either the Watch method or CheckApply method. The
choice depends on your particular resource and making the best decision requires
some experience with mgmt. If you are unsure, feel free to ask an existing
`mgmt` contributor. During `Init`, the engine will pass your resource a struct
containing some useful data and pointers. You should save a copy of this pointer
since you will need to use it in other parts of your resource.

#### Example

```golang
// Init initializes the Foo resource.
func (obj *FooRes) Init(init *engine.Init) error
	obj.init = init // save for later

	// run the resource specific initialization, and error if anything fails
	if some_error {
		return err // something went wrong!
	}
	return nil
}
```

This method is always called after `Validate` has run successfully, with the
exception that we can't prevent a malicious or buggy `libmgmt` user to not run
this. In other words, you should expect `Validate` to have run first, but you
shouldn't allow `Init` to dangerously `rm -rf /$the_world` if your code only
checks `$the_world` in `Validate`. Remember to always program safely!

### Close

```golang
Close() error
```

This is called to cleanup after the resource. It is usually not necessary, but
can be useful if you'd like to properly close a persistent connection that you
opened in the `Init` method and were using throughout the resource. It is *not*
the shutdown signal that tells the resource to exit. That happens in the Watch
loop.

#### Example

```golang
// Close runs some cleanup code for this resource.
func (obj *FooRes) Close() error {
	err := obj.conn.Close() // close some internal connection
	obj.someMap = nil       // free up some large data structure from memory
	return err
}
```

You should probably check the return errors of your internal methods, and pass
on an error if something went wrong.

### CheckApply

```golang
CheckApply(apply bool) (checkOK bool, err error)
```

`CheckApply` is where the real _work_ is done. Under normal circumstances, this
function should check if the state of this resource is correct, and if so, it
should return: `(true, nil)`. If the `apply` variable is set to `true`, then
this means that we should then proceed to run the changes required to bring the
resource into the correct state. If the `apply` variable is set to `false`, then
the resource is operating in _noop_ mode and _no operational changes_ should be
made!

After having executed the necessary operations to bring the resource back into
the desired state, or after having detected that the state was incorrect, but
that changes can't be made because `apply` is `false`, you should then return
`(false, nil)`.

You must cause the resource to converge during a single execution of this
function. If you cannot, then you must return an error! The exception to this
rule is that if an external force changes the state of the resource while it is
being remedied, it is possible to return from this function even though the
resource isn't now converged. This is not a bug, as the resources `Watch`
facility will detect the new change, ultimately resulting in a subsequent call
to `CheckApply`.

#### Example

```golang
// CheckApply does the idempotent work of checking and applying resource state.
func (obj *FooRes) CheckApply(apply bool) (bool, error) {
	// check the state
	if state_is_okay { return true, nil } // done early! :)

	// state was bad

	if !apply { return false, nil } // don't apply, we're in noop mode

	if any_error { return false, err } // anytime there's an err!

	// do the apply!
	return false, nil // after success applying
}
```

The `CheckApply` function is called by the `mgmt` engine when it believes a call
is necessary. Under certain conditions when a `Watch` call does not invalidate
the state of the resource, and no refresh call was sent, its execution might be
skipped. This is an engine optimization, and not a bug. It is mentioned here in
the documentation in case you are confused as to why a debug message you've
added to the code isn't always printed.

#### Paired execution

For many resources it is not uncommon to see `CheckApply` run twice in rapid
succession. This is usually not a pathological occurrence, but rather a healthy
pattern which is a consequence of the event system. When the state of the
resource is incorrect, `CheckApply` will run to remedy the state. In response to
having just changed the state, it is usually the case that this repair will
trigger the `Watch` code! In response, a second `CheckApply` is triggered, which
will likely find the state to now be correct.

#### Summary

* Anytime an error occurs during `CheckApply`, you should return `(false, err)`.
* If the state is correct and no changes are needed, return `(true, nil)`.
* You should only make changes to the system if `apply` is set to `true`.
* After checking the state and possibly applying the fix, return `(false, nil)`.
* Returning `(true, err)` is a programming error and can have a negative effect.

### Watch

```golang
Watch() error
```

`Watch` is a main loop that runs and sends messages when it detects that the
state of the resource might have changed. To send a message you should write to
the input event channel using the `Event` helper method. The Watch function
should run continuously until a shutdown message is received. If at any time
something goes wrong, you should return an error, and the `mgmt` engine will
handle possibly restarting the main loop based on the `retry` meta parameter.

It is better to send an event notification which turns out to be spurious, than
to miss a possible event. Resources which can miss events are incorrect and need
to be re-engineered so that this isn't the case. If you have an idea for a
resource which would fit this criteria, but you can't find a solution, please
contact the `mgmt` maintainers so that this problem can be investigated and a
possible system level engineering fix can be found.

You may have trouble deciding how much resource state checking should happen in
the `Watch` loop versus deferring it all to the `CheckApply` method. You may
want to put some simple fast path checking in `Watch` to avoid generating
obviously spurious events, but in general it's best to keep the `Watch` method
as simple as possible. Contact the `mgmt` maintainers if you're not sure.

If the resource is activated in `polling` mode, the `Watch` method will not get
executed. As a result, the resource must still work even if the main loop is not
running.

#### Select

The lifetime of most resources `Watch` method should be spent in an infinite
loop that is bounded by a `select` call. The `select` call is the point where
our method hands back control to the engine (and the kernel) so that we can
sleep until something of interest wakes us up. In this loop we must wait until
we get a shutdown event from the engine via the `<-obj.init.Done` channel, which
closes when we'd like to shut everything down. At this point you should cleanup,
and let `Watch` close.

#### Events

If the  `<-obj.init.Done` channel closes, we should shutdown our resource. When
When we want to send an event, we use the `Event` helper function. This
automatically marks the resource state as `dirty`. If you're unsure, it's not
harmful to send the event. This will ultimately cause `CheckApply` to run. This
method can block if the resource is being paused.

#### Startup

Once the `Watch` function has finished starting up successfully, it is important
to generate one event to notify the `mgmt` engine that we're now listening
successfully, so that it can run an initial `CheckApply` to ensure we're safely
tracking a healthy state and that we didn't miss anything when `Watch` was down
or from before `mgmt` was running. You must do this by calling the
`obj.init.Running` method.

#### Converged

The engine might be asked to shutdown when the entire state of the system has
not seen any changes for some duration of time. The engine can determine this
automatically, but each resource can block this if it is absolutely necessary.
If you need this functionality, please contact one of the maintainers and ask
about adding this feature and improving these docs right here.

This particular facility is most likely not required for most resources. It may
prove to be useful if a resource wants to start off a long operation, but avoid
sending out erroneous `Event` messages to keep things alive until it finishes.

#### Example

```golang
// Watch is the listener and main loop for this resource.
func (obj *FooRes) Watch() error {
	// setup the Foo resource
	var err error
	if err, obj.foo = OpenFoo(); err != nil {
		return err // we couldn't startup
	}
	defer obj.whatever.CloseFoo() // shutdown our Foo

	// notify engine that we're running
	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		select {
		// the actual events!
		case event := <-obj.foo.Events:
			if is_an_event {
				send = true
			}

		// event errors
		case err := <-obj.foo.Errors:
			return err // will cause a retry or permanent failure

		case <-obj.init.Done: // signal for shutdown request
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event()
		}
	}
}
```

#### Summary

* Remember to call `Running` when the `Watch` is running successfully.
* Remember to process internal events and shutdown promptly if asked to.
* Ensure the design of your resource is well thought out.
* Have a look at the existing resources for a rough idea of how this all works.

### Cmp

```golang
Cmp(engine.Res) error
```

Each resource must have a `Cmp` method. It is an abbreviation for `Compare`. It
takes as input another resource and must return whether they are identical or
not. This is used for identifying if an existing resource can be used in place
of a new one with a similar set of parameters. In particular, when switching
from one graph to a new (possibly identical) graph, this avoids recomputing the
state for resources which don't change or that are sufficiently similar that
they don't need to be swapped out.

In general if all the resource properties are identical, then they usually don't
need to be changed. On occasion, not all of them need to be compared, in
particular if they store some generated state, or if they aren't significant in
some way.

If the resource is identical, then you should return `nil`. If it is not, then
you should return a short error message which gives the reason it differs.

#### Example

```golang
// Cmp compares two resources and returns if they are equivalent.
func (obj *FooRes) Cmp(r engine.Res) error {
	// we can only compare FooRes to others of the same resource kind
	res, ok := r.(*FooRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Whatever != res.Whatever {
		return fmt.Errorf("the Whatever param differs")
	}
	if obj.Flag != res.Flag {
		return fmt.Errorf("the Flag param differs")
	}

	return nil // they must match!
}
```

## Traits

Resources can have different `traits`, which means they can be extended to have
additional functionality or special properties. Those special properties are
usually added by extending your resource so that it is compatible with
additional interface that contain the `Res` interface. Each of these interfaces
represents the additional functionality. Since in most cases this requires some
common boilerplate, you can usually get some or most of the functionality by
embedding the correct trait struct anonymously in your struct. This is shown in
the struct example above. You'll always want to include the `Base` trait in all
resources. This provides some basics which you'll always need.

What follows are a list of available traits.

### Refreshable

Some resources may choose to support receiving refresh notifications. In general
these should be avoided if possible, but nevertheless, they do make sense in
certain situations. Resources that support these need to verify if one was sent
during the CheckApply phase of execution. This is accomplished by calling the
`obj.init.Refresh() bool` method, and inspecting the return value. This is only
necessary if you plan to perform a refresh action. Refresh actions should still
respect the `apply` variable, and no system changes should be made if it is
`false`. Refresh notifications are generated by any resource when an action is
applied by that resource and are transmitted through graph edges which have
enabled their propagation. Resources that currently perform some refresh action
include `svc`, `timer`, and `password`.

It is very important that you include the `traits.Refreshable` struct in your
resource. If you do not include this, then calling `obj.init.Refresh` may
trigger a panic. This is programmer error.

### Edgeable

Edgeable is a trait that allows your resource to automatically connect itself to
other resources that use this trait to add edge dependencies between the two. An
older blog post on this topic is
[available](https://purpleidea.com/blog/2016/03/14/automatic-edges-in-mgmt/).

After you've included this trait, you'll need to implement two methods on your
resource.

#### UIDs

```golang
UIDs() []engine.ResUID
```

The `UIDs` method returns a list of `ResUID` interfaces that represent the
particular resource uniquely. This is used with the AutoEdges API to determine
if another resource can match a dependency to this one.

#### AutoEdges

```golang
AutoEdges() (engine.AutoEdge, error)
```

This returns a struct that implements the `AutoEdge` interface. This struct
is used to match other resources that might be relevant dependencies for this
resource.

### Groupable

Groupable is a trait that can allow your resource automatically group itself to
other resources. Doing so can reduce the resource or runtime burden on the
engine, and improve performance in some scenarios. An older blog post on this
topic is
[available](https://purpleidea.com/blog/2016/03/30/automatic-grouping-in-mgmt/).

### Sendable

Sendable is a trait that allows your resource to send values through the graph
edges to another resource. These values are produced during `CheckApply`. They
can be sent to any resource that has an appropriate parameter and that has the
`Recvable` trait. You can read more about this in the Send/Recv section below.

### Recvable

Recvable is a trait that allows your resource to receive values through the
graph edges from another resource. These values are consumed during the
`CheckApply` phase, and can be detected there as well. They can be received from
any resource that has an appropriate value and that has the `Sendable` trait.
You can read more about this in the Send/Recv section below.

### Collectable

This is currently a stub and will be updated once the DSL is further along.

## Resource Initialization

During the resource initialization in `Init`, the engine will pass in a struct
containing a bunch of data and methods. What follows is a description of each
one and how it is used.

### Program

Program is a string containing the name of the program. Very few resources need
this.

### Hostname

Hostname is the uuid for the host. It will be occasionally useful in some
resources. It is preferable if you can avoid depending on this. It is possible
that in the future this will be a channel which changes if the local hostname
changes.

### Running

Running must be called after your watches are all started and ready. It is only
called from within `Watch`. It is used to notify the engine that you're now
ready to detect changes.

### Event

Event sends an event notifying the engine of a possible state change. It is
only called from within `Watch`.

### Done

Done is a channel that closes when the engine wants us to shutdown. It is only
called from within `Watch`.

### Refresh

Refresh returns whether the resource received a notification. This flag can be
used to tell a `svc` to reload, or to perform some state change that wouldn't
otherwise be noticed by inspection alone. You must implement the `Refreshable`
trait for this to work. It is only called from within `CheckApply`.

### Send

Send exposes some variables you wish to send via the `Send/Recv` mechanism. You
must implement the `Sendable` trait for this to work. It is only called from
within `CheckApply`.

### Recv

Recv provides a map of variables which were sent to this resource via the
`Send/Recv` mechanism. You must implement the `Recvable` trait for this to work.
It is only called from within `CheckApply`.

### World

World provides a connection to the outside world. This is most often used for
communicating with the distributed database. It can be used in `Init`,
`CheckApply` and `Watch`. Use with discretion and understanding of the internals
if needed in `Close`.

### VarDir

VarDir is a facility for local storage. It is used to return a path to a
directory which may be used for temporary storage. It should be cleaned up on
resource `Close` if the resource would like to delete the contents. The resource
should not assume that the initial directory is empty, and it should be cleaned
on `Init` if that is a requirement.

### Debug

Debug signals whether we are running in debugging mode. In this case, we might
want to log additional messages.

### Logf

Logf is a logging facility which will correctly namespace any messages which you
wish to pass on. You should use this instead of the log package directly for
production quality resources.

## Further considerations

There is some additional information that any resource writer will need to know.
Each issue is listed separately below!

### Resource registration

All resources must be registered with the engine so that they can be found. This
also ensures they can be encoded and decoded. Make sure to include the following
code snippet for this to work.

```golang
func init() { // special golang method that runs once
	// set your resource kind and struct here (the kind must be lower case)
	engine.RegisterResource("foo", func() engine.Res { return &FooRes{} })
}
```

### YAML Unmarshalling

To support YAML unmarshalling for your resource, you must implement an
additional method. It is recommended if you want to use your resource with the
`Puppet` compiler.

```golang
UnmarshalYAML(unmarshal func(interface{}) error) error // optional
```

This is optional, but recommended for any resource that will have a YAML
accessible struct. It is not required because to do so would mean that
third-party or custom resources (such as those someone writes to use with
`libmgmt`) would have to implement this needlessly.

The signature intentionally matches what is required to satisfy the `go-yaml`
[Unmarshaler](https://godoc.org/gopkg.in/yaml.v2#Unmarshaler) interface.

#### Example

```golang
// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *FooRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FooRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*FooRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FooRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FooRes(raw) // restore from indirection with type conversion!
	return nil
}
```

## Send/Recv

In `mgmt` there is a novel concept called _Send/Recv_. For some background,
please read the [introductory article](https://purpleidea.com/blog/2016/12/07/sendrecv-in-mgmt/).
When using this feature, the engine will automatically send the user specified
value to the intended destination without requiring much resource specific code.
Any time that one of the destination values is changed, the engine automatically
marks the resource state as `dirty`. To detect if a particular value was
received, and if it changed (during this invocation of `CheckApply`) from the
previous value, you can query the `obj.init.Recv()` method. It will contain a
`map` of all the keys which can be received on, and the value has a `Changed`
property which will indicate whether the value was updated on this particular
`CheckApply` invocation. The type of the sending key must match that of the
receiving one. This can _only_ be done inside of the `CheckApply` function!

```golang
// inside CheckApply, probably near the top
if val, exists := obj.init.Recv()["SomeKey"]; exists {
	obj.init.Logf("the SomeKey param was sent to us from: %s.%s", val.Res, val.Key)
	if val.Changed {
		obj.init.Logf("the SomeKey param was just updated!")
		// you may want to invalidate some local cache
	}
}
```

The specifics of resource sending are not currently documented. Please send a
patch here!

## Composite resources

Composite resources are resources which embed one or more existing resources.
This is useful to prevent code duplication in higher level resource scenarios.
The best example of this technique can be seen in the `nspawn` resource which
can be seen to partially embed a `svc` resource, but without its `Watch`.
Unfortunately no further documentation about this subject has been written. To
expand this section, please send a patch! Please contact us if you'd like to
work on a resource that uses this feature, or to add it to an existing one!

## Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### Can I write resources in a different language?

Currently `golang` is the only supported language for built-in resources. We
might consider allowing external resources to be imported in the future. This
will likely require a language that can expose a C-like API, such as `python` or
`ruby`. Custom `golang` resources are already possible when using mgmt as a lib.
Higher level resource collections will be possible once the `mgmt` DSL is ready.

### Why does the resource API have `CheckApply` instead of two separate methods?

In an early version we actually had both "parts" as separate methods, namely:
`StateOK` (Check) and `Apply`, but the [decision](58f41eddd9c06b183f889f15d7c97af81b0331cc)
was made to merge the two into a single method. There are two reasons for this:

1. Many situations would involve the engine running both `Check` and `Apply`. If
the resource needed to share some state (for efficiency purposes) between the
two calls, this is much more difficult. A common example is that a resource
might want to open a connection to `dbus` or `http` to do resource state testing
and applying. If the methods are combined, there's no need to open and close
them twice. A counter argument might be that you could open the connection in
`Init`, and close it in `Close`, however you might not want that open for the
full lifetime of the resource if you only change state occasionally.
2. Suppose you came up with a really good reason why you wanted the two methods
to be separate. It turns out that the current `CheckApply` can wrap this easily.
It would look approximately like this:

```golang
func (obj *FooRes) CheckApply(apply bool) (bool, error) {
	// my private split implementation of check and apply
	if c, err := obj.check(); err != nil {
		return false, err // we errored
	} else if c {
		return true, nil // state was good!
	}

	if !apply {
		return false, nil // state needs fixing, but apply is false
	}

	err := obj.apply() // errors if failure or unable to apply

	return false, err // always return false, with an optional error
}
```

Feel free to use this pattern if you're convinced it's necessary. Alternatively,
if you think I got the `Res` API wrong and you have an improvement, please let
us know!

### What new resource primitives need writing?

There are still many ideas for new resources that haven't been written yet. If
you'd like to contribute one, please contact us and tell us about your idea!

### Is the resource API stable? Does it ever change?

Since we are pre 1.0, the resource API is not guaranteed to be stable, however
it is not expected to change significantly. The last major change kept the
core functionality nearly identical, simplified the implementation of all the
resources, and took about five to ten minutes to port each resource to the new
API. The fundamental logic and behaviour behind the resource API has not changed
since it was initially introduced.

### Where can I find more information about mgmt?

Additional blog posts, videos and other material [is available!](https://github.com/purpleidea/mgmt/blob/master/docs/on-the-web.md).

## Suggestions

If you have any ideas for API changes or other improvements to resource writing,
please let us know! We're still pre 1.0 and pre 0.1 and happy to break API in
order to get it right!
