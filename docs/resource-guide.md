#mgmt

<!--
Mgmt
Copyright (C) 2013-2016+ James Shubin and the project contributors
Written by James Shubin <james@shubin.ca> and the project contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

##mgmt resource guide by [James](https://ttboj.wordpress.com/)
####Available from:
####[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/)

####This documentation is available in: [Markdown](https://github.com/purpleidea/mgmt/blob/master/docs/resource-guide.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/docs/resource-guide.md) format.

####Table of Contents

1. [Overview](#overview)
2. [Theory - Resource theory in mgmt](#theory)
3. [Resource API - Getting started with mgmt](#resource-api)
	* [Init - Initialize the resource](#init)
	* [CheckApply - Check and apply resource state](#checkapply)
	* [Watch - Detect resource changes](#watch)
	* [Compare - Compare resource with another](#compare)
4. [Further considerations - More information about resource writing](#further-considerations)
5. [Automatic edges - Adding automatic resources dependencies](#automatic-edges)
6. [Automatic grouping - Grouping multiple resources into one](#automatic-grouping)
7. [Send/Recv - Communication between resources](#send-recv)
8. [Composite resources - Importing code from one resource into another](#composite-resources)
9. [FAQ - Frequently asked questions](#frequently-asked-questions)
10. [Suggestions - API change suggestions](#suggestions)
11. [Authors - Authors and contact information](#authors)

##Overview

The `mgmt` tool has built-in resource primitives which make up the building
blocks of any configuration. Each instance of a resource is mapped to a single
vertex in the resource [graph](https://en.wikipedia.org/wiki/Directed_acyclic_graph).
This guide is meant to instruct developers on how to write a brand new resource.
Since `mgmt` and the core resources are written in golang, some prior golang
knowledge is assumed.

##Theory

Resources in `mgmt` are similar to resources in other systems in that they are
[idempotent](https://en.wikipedia.org/wiki/Idempotence). Our resources are
uniquely different in that they can detect when their state has changed, and as
a result can run to revert or repair this change instantly. For some background
on this design, please read the
[original article](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
on the subject.

##Resource API

To implement a resource in `mgmt` it must satisfy the
[`Res`](https://github.com/purpleidea/mgmt/blob/master/resources/resources.go)
interface. What follows are each of the method signatures and a description of
each.

###Init
```golang
Init() error
```

This is called to initialize the resource. If something goes wrong, it should
return an error. It should set the resource `kind`, do any resource specific
work, and finish by calling the `Init` method of the base resource.

####Example
```golang
// Init initializes the Foo resource.
func (obj *FooRes) Init() error {
	obj.BaseRes.kind = "Foo" // must set capitalized resource kind
	// run the resource specific initialization, and error if anything fails
	if some_error {
		return err // something went wrong!
	}
	return obj.BaseRes.Init() // call the base resource init
}
```

###CheckApply
```golang
CheckApply(apply bool) (checkOK bool, err error)
```

`CheckApply` is where the real _work_ is done. Under normal circumstances, this
function should check if the state of this resource is correct, and if so, it
should return: `(true, nil)`. If the `apply` variable is set to `true`, then
this means that we should then proceed to run the changes required to bring the
resource into the correct state. If the `apply` variable is set to `false`, then
the resource is operating in _noop_ mode and _no operations_ should be executed!

After having executed the necessary operations to bring the resource back into
the desired state, or after having detected that the state was incorrect, but
that changes can't be made because `apply` is `false`, you should then return
`(false, nil)`.

You must cause the resource to converge during a single execution of this
function. If you cannot, then you must return an error! The exception to this
rule is that if an external force changes the state of the resource while it is
being remedied, it is possible to return from this function even though the
resource isn't now converged. This is not a bug, as the resources `Watch`
facility will detect the change, ultimately resulting in a subsequent call to
`CheckApply`.

####Example
```golang
// CheckApply does the idempotent work of checking and applying resource state.
func (obj *FooRes) CheckApply(apply bool) (bool, error) {
	// check the state
	if state_is_okay { return true, nil } // done early! :)
	// state was bad
	if !apply { return false, nil } // don't apply; !stateok, nil
	// do the apply!
	return false, nil // after success applying
	if any_error { return false, err } // anytime there's an err!
}
```

The `CheckApply` function is called by the `mgmt` engine when it believes a call
is necessary. Under certain conditions when a `Watch` call does not invalidate
the state of the resource, and no refresh call was sent, its execution might be
skipped. This is an engine optimization, and not a bug. It is mentioned here in
the documentation in case you are confused as to why a debug message you've
added to the code isn't always printed.

####Refresh notifications
Some resources may choose to support receiving refresh notifications. In general
these should be avoided if possible, but nevertheless, they do make sense in
certain situations. Resources that support these need to verify if one was sent
during the CheckApply phase of execution. This is accomplished by calling the
`Refresh() bool` method of the resource, and inspecting the return value. This
is only necessary if you plan to perform a refresh action. Refresh actions
should still respect the `apply` variable, and no system changes should be made
if it is `false`. Refresh notifications are generated by any resource when an
action is applied by that resource and are transmitted through graph edges which
have enabled their propagation. Resources that currently perform some refresh
action include `svc`, `timer`, and `password`.

####Paired execution
For many resources it is not uncommon to see `CheckApply` run twice in rapid
succession. This is usually not a pathological occurrence, but rather a healthy
pattern which is a consequence of the event system. When the state of the
resource is incorrect, `CheckApply` will run to remedy the state. In response to
having just changed the state, it is usually the case that this repair will
trigger the `Watch` code! In response, a second `CheckApply` is triggered, which
will likely find the state to now be correct.

####Summary
* Anytime an error occurs during `CheckApply`, you should return `(false, err)`.
* If the state is correct and no changes are needed, return `(true, nil)`.
* You should only make changes to the system if `apply` is set to `true`.
* After checking the state and possibly applying the fix, return `(false, nil)`.
* Returning `(true, err)` is a programming error and will cause a `Fatal`.

###Watch
```golang
Watch(chan Event) error
```

`Watch` is a main loop that runs and sends messages when it detects that the
state of the resource might have changed. To send a message you should write to
the input `Event` channel using the `DoSend` helper method. The Watch function
should run continuously until a shutdown message is received. If at any time
something goes wrong, you should return an error, and the `mgmt` engine will
handle possibly restarting the main loop based on the `retry` meta parameters.

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

####Select
The lifetime of most resources `Watch` method should be spent in an infinite
loop that is bounded by a `select` call. The `select` call is the point where
our method hands back control to the engine (and the kernel) so that we can
sleep until something of interest wakes us up. In this loop we must process
events from the engine via the `<-obj.Events()` call, wait for the converged
timeout with `<-cuid.ConvergedTimer()`, and receive events for our resource
itself!

####Events
If we receive an internal event from the `<-obj.Events()` method, we can read it
with the ReadEvent helper function. This function tells us if we should shutdown
our resource, and if we should generate an event. When we want to send an event,
we use the `DoSend` helper function. It is also important to mark the resource
state as `dirty` if we believe it might have changed. We do this with the
`StateOK(false)` function.

####Startup
Once the `Watch` function has finished starting up successfully, it is important
to generate one event to notify the `mgmt` engine that we're now listening
successfully, so that it can run an initial `CheckApply` to ensure we're safely
tracking a healthy state and that we didn't miss anything when `Watch` was down
or from before `mgmt` was running. It does this by calling the `Running` method.

####Converged
The engine might be asked to shutdown when the entire state of the system has
not seen any changes for some duration of time. In order for the engine to be
able to make this determination, each resource must report its converged state.
To do this, the `Watch` method should get the `ConvergedUID` handle that has
been prepared for it by the engine. This is done by calling the `ConvergerUID`
method on the resource object. The result can be used to set the converged
status with `SetConverged`, and to notify when the particular timeout has been
reached by waiting on `ConvergedTimer`.

Instead of interacting with the `ConvergedUID` with these two methods, we can
instead use the `StartTimer` and `ResetTimer` methods which accomplish the same
thing, but provide a `select`-free interface for different coding situations.

####Example
```golang
// Watch is the listener and main loop for this resource.
func (obj *FooRes) Watch(processChan chan event.Event) error {
	cuid := obj.ConvergerUID() // get the converger uid used to report status

	// setup the Foo resource
	var err error
	if err, obj.foo = OpenFoo(); err != nil {
		return err // we couldn't startup
	}
	defer obj.whatever.CloseFoo() // shutdown our

	// notify engine that we're running
	if err := obj.Running(processChan); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit = false
	for {
		obj.SetState(ResStateWatching) // reset
		select {
		case event := <-obj.Events():
			cuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}

		// the actual events!
		case event := <-obj.foo.Events:
			if is_an_event {
				send = true // used below
				cuid.SetConverged(false)
				obj.StateOK(false) // dirty
			}

		// event errors
		case err := <-obj.foo.Errors:
			cuuid.SetConverged(false)
			return err // will cause a retry or permanent failure

		case <-cuid.ConvergedTimer():
			cuid.SetConverged(true) // converged!
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}
```

####Summary
* Remember to call the appropriate `converger` methods throughout the resource.
* Remember to call `Startup` when the `Watch` is running successfully.
* Remember to process internal events and shutdown promptly if asked to.
* Ensure the design of your resource is well thought out.
* Have a look at the existing resources for a rough idea of how this all works.

###Compare
```golang
Compare(Res) bool
```

Each resource must have a `Compare` method. This takes as input another resource
and must return whether they are identical or not. This is used for identifying
if an existing resource can be used in place of a new one with a similar set of
parameters. In particular, when switching from one graph to a new (possibly
identical) graph, this avoids recomputing the state for resources which don't
change or that are sufficiently similar that they don't need to be swapped out.

In general if all the resource properties are identical, then they usually don't
need to be changed. On occasion, not all of them need to be compared, in
particular if they store some generated state, or if they aren't significant in
some way.

####Example
```golang
// Compare two resources and return if they are equivalent.
func (obj *FooRes) Compare(res Res) bool {
	switch res.(type) {
	case *FooRes: // only compare to other resources of the Foo kind!
		res := res.(*FileRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}
		if obj.Name != res.Name {
			return false
		}
		if obj.whatever != res.whatever {
			return false
		}
		if obj.Flag != res.Flag {
			return false
		}
	default:
		return false // different kind of resource
	}
	return true // they must match!
}
```

###Validate
```golang
Validate() error
```

This method is used to validate if the populated resource struct is a valid
representation of the resource kind. If it does not conform to the resource
specifications, it should generate an error. If you notice that this method is
quite large, it might be an indication that you might want to reconsider the
parameter list and interface to this resource.

###GetUIDs
```golang
GetUIDs() []ResUID
```

The `GetUIDs` method returns a list of `ResUID` interfaces that represent the
particular resource uniquely. This is used with the AutoEdges API to determine
if another resource can match a dependency to this one.

###AutoEdges
```golang
AutoEdges() AutoEdge
```

This returns a struct that implements the `AutoEdge` interface. This struct
is used to match other resources that might be relevant dependencies for this
resource.

###CollectPattern
```golang
CollectPattern() string
```

This is currently a stub and will be updated once the DSL is further along.

##Further considerations
There is some additional information that any resource writer will need to know.
Each issue is listed separately below!

###Resource struct
Each resource will implement methods as pointer receivers on a resource struct.
The resource struct must include an anonymous reference to the `BaseRes` struct.
The naming convention for resources is that they end with a `Res` suffix. If
you'd like your resource to be accessible by the `YAML` graph API (GAPI), then
you'll need to include the appropriate YAML fields as shown below.

####Example
```golang
type FooRes struct {
	BaseRes `yaml:",inline"` // base properties

	Whatever string `yaml:"whatever"` // you pick!
	Bar int // no yaml, used as public output value for send/recv
	Baz bool `yaml:"baz"` // something else

	something string // some private field
}
```

###YAML
In addition to labelling your resource struct with YAML fields, you must also
add an entry to the internal `GraphConfig` struct. It is a fairly straight
forward one line patch.

```golang
type GraphConfig struct {
// [snip...]
	Resources struct {
		Noop []*resources.NoopRes `yaml:"noop"`
		File []*resources.FileRes `yaml:"file"`
		// [snip...]
		Foo []*resources.FooRes `yaml:"foo"` // tada :)
	}
}
```

###Gob registration
All resources must be registered with the `golang` _gob_ module so that they can
be encoded and decoded. Make sure to include the following code snippet for this
to work.

```golang
import "encoding/gob"
func init() { // special golang method that runs once
	gob.Register(&FooRes{}) // substitude your resource here
}
```

##Automatic edges
Automatic edges in `mgmt` are well described in [this article](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/).
The best example of this technique can be seen in the `svc` resource.
Unfortunately no further documentation about this subject has been written. To
expand this section, please send a patch! Please contact us if you'd like to
work on a resource that uses this feature, or to add it to an existing one!

##Automatic grouping
Automatic grouping in `mgmt` is well described in [this article](https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/).
The best example of this technique can be seen in the `pkg` resource.
Unfortunately no further documentation about this subject has been written. To
expand this section, please send a patch! Please contact us if you'd like to
work on a resource that uses this feature, or to add it to an existing one!


##Send/Recv
In `mgmt` there is a novel concept called _Send/Recv_. For some background,
please [read the introductory article](https://ttboj.wordpress.com/2016/12/07/sendrecv-in-mgmt/).
When using this feature, the engine will automatically send the user specified
value to the intended destination without requiring any resource specific code.
Any time that one of the destination values is changed, the engine automatically
marks the resource state as `dirty`. To detect if a particular value was
received, and if it changed (during this invocation of CheckApply) from the
previous value, you can query the Recv parameter. It will contain a `map` of all
the keys which can be received on, and the value has a `Changed` property which
will indicate whether the value was updated on this particular `CheckApply`
invocation. The type of the sending key must match that of the receiving one.
This can _only_ be done inside of the `CheckApply` function!

```golang
// inside CheckApply, probably near the top
if val, exists := obj.Recv["SomeKey"]; exists {
	log.Printf("SomeKey was sent to us from: %s[%s].%s", val.Res.Kind(), val.Res.GetName(), val.Key)
	if val.Changed {
		log.Printf("SomeKey was just updated!")
		// you may want to invalidate some local cache
	}
}
```

Astute readers will note that there isn't anything that prevents a user from
sending an identically typed value to some arbitrary (public) key that the
resource author hadn't considered! While this is true, resources should probably
work within this problem space anyways. The rule of thumb is that any public
parameter which is normally used in a resource can be used safely.

One subtle scenario is that if a resource creates a local cache or stores a
computation that depends on the value of a public parameter and will require
invalidation should that public parameter change, then you must detect that
scenario and invalidate the cache when it occurs. This *must* be processed
before there is a possibility of failure in CheckApply, because if we fail (and
possibly run again) the subsequent send->recv transfer might not have a new
value to copy, and therefore we won't see this notification of change.
Therefore, it is important to process these promptly, if they must not be lost,
such as for cache invalidation.

Remember, `Send/Recv` only changes your resource code if you cache state.

##Composite resources
Composite resources are resources which embed one or more existing resources.
This is useful to prevent code duplication in higher level resource scenarios.
The best example of this technique can be seen in the `nspawn` resource which
can be seen to partially embed a `svc` resource, but without its `Watch`.
Unfortunately no further documentation about this subject has been written. To
expand this section, please send a patch! Please contact us if you'd like to
work on a resource that uses this feature, or to add it to an existing one!

##Frequently asked questions
(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

###Can I write resources in a different language?
Currently `golang` is the only supported language for built-in resources. We
might consider allowing external resources to be imported in the future. This
will likely require a language that can expose a C-like API, such as `python` or
`ruby`. Custom `golang` resources are already possible when using mgmt as a lib.
Higher level resource collections will be possible once the `mgmt` DSL is ready.

###What new resource primitives need writing?
There are still many ideas for new resources that haven't been written yet. If
you'd like to contribute one, please contact us and tell us about your idea!

###Where can I find more information about mgmt?
Additional blog posts, videos and other material [is available!](https://github.com/purpleidea/mgmt/#on-the-web).

##Suggestions
If you have any ideas for API changes or other improvements to resource writing,
please let us know! We're still pre 1.0 and pre 0.1 and happy to break API in
order to get it right!

##Authors

Copyright (C) 2013-2016+ James Shubin and the project contributors

Please see the
[AUTHORS](https://github.com/purpleidea/mgmt/tree/master/AUTHORS) file
for more information.

* [github](https://github.com/purpleidea/)
* [&#64;purpleidea](https://twitter.com/#!/purpleidea)
* [https://ttboj.wordpress.com/](https://ttboj.wordpress.com/)
