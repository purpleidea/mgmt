# Service API design guide

This document is intended as a short instructional design guide in building a
service management API. It is certainly intended for someone who wishes to use
`mgmt` resources and functions to interact with their facilities, however it may
be of more general use as well. Hopefully this will help you make smarter design
considerations early on, and prevent some amount of unnecessary technical debt.

## Main aspects

What follows are some of the most common considerations which you may wish to
take into account when building your service. This list is non-exhaustive. Of
particular note, as of the writing of this document, many of these designs are
not taken into account or not well-handled or implemented by the major API
("cloud") providers.

### Authentication

#### The status-quo

Many services naturally require you to authenticate yourself. Usually the
initial user who sets up the account and provides credit card details will need
to download secret credentials in order to access the service. The onus is on
the user to keep those credentials private, and to prevent leaking them. It is
convenient (and insecure) to store them in `git` repositories containing scripts
and configuration management code. Since it's likely you will use multiple
different services, it also means you will have a ton of different credentials
to guard.

#### An alternative

Instead, build your service to accept a public key that you store in the users
account. Only consumers that can correctly sign messages matching this public
key should be authorized. This mechanism is well-understood by anyone who has
ever uploaded their public SSH key to a server. You can use SSH keys, GPG keys,
or even get into Kerberos if that's appropriate. Best of all, if you and other
services use a standardized mechanism like GPG, a user might only need to keep
track of their single key-pair, even when they're using multiple services!

### Events

#### The problem

People have been building "[CRUD](https://en.wikipedia.org/wiki/Create,_read,_update_and_delete)"
and "[REST](https://en.wikipedia.org/wiki/REST)"ful API's for years. The biggest
missing part that most of them don't provide is events. If users want to know
when a resource changes, they have to repeatedly poll the server, which is both
network intensive, and introduces latency. When services were simpler, this
wasn't as much of a consideration, but these days it matters. An embarrassingly
small number of major software vendors implement these correctly, if at all.

#### Why events?

The `mgmt` tool is different from most other static tools in that it allows
reading streams of incoming data, and stream of change events from resources we
are managing. If an event API is not available, we can still poll, but this is
not as desirable. An event-capable API doesn't prevent polling if that's
preferred, you can always repeat a read request periodically.

#### Variants

The two common mechanisms for receiving events are "callbacks" and
"long-polling". In the former, the service contacts the consumer when something
happens. In the latter, the consumer opens a connection, and the service either
closes the connection or sends the reply, when it's ready. Long-polling is often
preferred since it doesn't require an open firewall on the consumers side.
Callbacks are preferred because it's often cheaper for the service to implement
that. It's also less reliable since it's hard to know if the callback message
wasn't received because it was dropped, or if there just wasn't an event. And it
requires static timeouts when retrying a callback message, and so on. It's best
to implement long-polling or something equivalent at a minimum.

#### "Since" requests

When making an event request, some API's will let you tack on a "since" style
parameter that tells the endpoint that we're interested in all of the events
_since_ a particular timestamp, or _since_ a particular sequence ID. This can be
very useful if missing an intermediate event is a concern. Implement this if you
can, but it's better for all concerned if purely declarative facilities are all
that is required. It also forces the endpoint to maintain some state, which may
be undesirable for them.

#### Out of band

Some providers have the event system tacked on to a separate facility. If it's
not part of the core API, then it's not useful. You shouldn't have to configure
a separate system in order to start getting events.

### Batching

With so many resources, you might expect to have 1000's of long-polling
connections all sitting open and idle. That can't be efficient! It's not, which
is why good API's need a batching facility. This lets the consumer group
together many watches (all waiting on a long-poll) inside of a single call. That
way, a single connection might only be needed for a large amount of information.

### Don't auto-generate junk

Please build an elegant API. Many services auto-generate a "phone book" SDK of
junk. It might seem inevitable, so if you absolutely need to do this, then put
some extra effort into making it idiomatic. If I'm using an SDK generated for
`golang` and I see an internal `foo.String` wrapper, then chances are you have
designed your API and code to be easier to maintain for you, instead of
prioritizing your customers. Surely the total volume of all customer code is
more than your own, so why optimize for that instead of the putting the customer
first?

### Resources and functions

`Mgmt` has a concept of "resources" and "functions". Resources are used in an
idempotent model to express desired state and perform that work, and "functions"
are used to receive and pull data into the system. That separation has shown to
be an elegant one. Consider it when designing your API's. For example, if some
vital information can only be obtained after performing a modifying operation,
then it might signal that you're missing some sort of a lookup or event-log
system. Design your API's to be idempotent, this solves many distributed-system
problems involving receiving duplicate messages, and so on.

## Using mgmt as a library

Instead of building a new service from scratch, and re-inventing the typical
management and CLI layer, consider using `mgmt` as a library, and directly
benefiting from that work. This has not been done for a large production
service, but the author believes it would be quite efficient, particularly if
your application is written in golang. It's equivalently easy to do it for other
languages as well, you just end up with two binaries instead of one. (Or you can
embed the other binary into the new golang management tool.)

## Cloud API considerations

Many "cloud" companies have a lot of technical debt and a lot of customers. As a
result, it might be very hard for them to improve their API's, particularly
without breaking compatibility promises for their existing customers. As a
result, they should either add a versioned API, which lets newer consumers get
the benefit, or add new parallel services which offer the modern features. If
they don't, the only solution is for new competitors to build-in these better
efficiencies, eventually offering better value to cost ratios, which will then
make legacy products less lucrative and therefore unmaintainable as compared to
their competitors.

## Suggestions

If you have any ideas for suggestions or other improvements to this guide,
please let us know! I hope this was helpful. Please reach out if you are
building an API that you might like to have `mgmt` consume!
