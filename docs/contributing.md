# Contributing

What follows is a short guide with information for participants who wish to
contribute to the project. It hopes to set both some expectations and boundaries
so that we both benefit.

## Small patches

If you have a small patch which you believe is straightforward, should be easy
to merge, and isn't overly onerous on your time to write, please feel free to
send it our way without asking first. Bug fixes are excellent examples of small
patches. Please make sure to familiarize yourself with the rough coding style of
the project first, and read through the [style guide](style-guide.md).

## Making an excellent small patch

As a special case: We'd like to avoid minimal effort, one-off, drive-by patches
by bots and contributors looking to increase their "activity" numbers. As an
example: a patch which fixes a small linting issue isn't rousing, but a patch
that adds a linter test _and_ fixes a small linting issue is, because it shows
you put in more effort.

## Medium patches

Medium sized patches are especially welcome. Good examples of these patches
can include writing a new `mgmt` resource or function. You'll generally need
some knowledge of golang interfaces and concurrency to write these patches.
Before writing one of these, please make sure you understand some basics about
the project and how the tool works. After this, it is recommended that you join
our discussion channel to suggest the idea, and ideally include the actual API
you'd like to propose before writing the code and sending a patch.

## Making an excellent medium patch proposal

The "API" of a resource is the type signature of the resource struct, and the
"API" of a function is the type signature or signatures that it supports. (Since
functions can be polymorphic, more than one signature can be possible!) A good
proposal would likely also comment on the mechanisms the resources or functions
would use to watch for events, to check state, and to apply changes. If these
mechanisms need new dependencies, a brief survey of which dependencies are
available and why you recommend a particular one is encouraged.

## Large patches or structural and core patches

Please do not send us large, core or structurally significant patches without
first getting our approval and without getting some medium patches in first.
These patches take a lot of effort to review, and we don't want to skimp on our
commitment to that if we can't muster it. Instead grow our relationship with you
on the medium-sized patches first. (A core patch might refer to something that
touches either the function engine, resource engine, compiler internals, or
something that is part of one of the internal API's.)

## Expectations and boundaries

When interacting with the project and soliciting feedback (either for design or
during a code review) please keep in mind that the project (unfortunately!) has
time constraints and so must prioritize how it handles workloads. If you are
someone who has successfully sent in small patches, we will be more willing to
spend time mentoring your medium sized patches and so on. Think of it this way:
as you show that you're contributing to the project, we'll contribute more to
you. Put another way: we can't afford to spend large amounts of time discussing
potential patches with you, just to end up nowhere. Build up your reputation
with us, and we hope to help grow our symbiosis with you all the while as you
grow too!

## Energy output

The same goes for users and issue creators. There are times when we simply don't
have the cycles to discuss or litigate an issue with you. We wish we did have
more time, but it is finite, and running a project is not free. Therefore,
please keep in mind that you don't automatically qualify for free support or
attention.

## LLM's and AI

If you use LLM's and/or other AI related tooling to help you with programming,
this does not absolve you of the responsibility for the code submitted. It is
not our job to review your machine generated code. If you send in something that
surreptitiously adds bugs, security holes, or otherwise is sloppy code, we will
hold you responsible, and ultimately this will probably lead to you being banned
from contributing. If you are not confident enough to thoroughly understand and
review the generated code you are submitting, then please do not submit it!

## Copyright

You are legally responsible for the code you submit, in so far as respecting the
terms that the license dictates. This means that you are legally allowed to
submit it under the license of the project, and that you haven't signed an
agreement (with for example, your employer) that forbids you from contributing
such code. This also applies to your use of LLM's and AI related tooling. Please
make sure that the output from the tooling you're using isn't incompatible with
the copyright of the project.

## Attention seeking behaviours

Some folks spend too much time starting discussions, commenting on issues,
"planning" and otherwise displaying attention seeking behaviours. Please avoid
doing this as much as possible, especially if you are not already a major
contributor to the project. While it may be well intentioned, if it is
indistinguishable to us from intentional interference, then it's not welcome
behaviour. Remember that Free Software is not free to write. If you require more
attention, then either contribute more to the project, or consider paying for a
[support contract](https://mgmtconfig.com/).

## Consulting

Having said all that, there are some folks who want to do some longer-term
planning to decide if our core design and architecture is right for them to
invest in. If that's the case, and you aren't already a well-known project
contributor, please [contact](https://mgmtconfig.com/) us for a consulting
quote. We have packages available for both individuals and businesses.

## Respect

Please be mindful and respectful of others when interacting with the project and
its contributors. If you cannot abide by that, you may no longer be welcome.
