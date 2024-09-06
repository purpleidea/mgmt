# Modules!

This is a collection of `mcl` modules that may be of general interest.

## Why?

In particular while the project is evolving very rapidly, it makes sense to keep
these alongside the compiler so that if anything needs refactoring, it happens
together in the same commit, and if there are improvements made and new features
added, you can see exactly what commit brought in this change!

## Acceptance criteria

It's sort of arbitrary at the moment, but I'm putting in what I find to be
generally helpful for my needs. Patches are welcome, but expect I might be a bit
discerning if it's not what I'm looking for right now.

## Long-term

I see `mgmt` and `mcl` as being able to replace a traditional software
management daemon, and front-end configuration interface for many projects. As a
result, it might end up being the de facto way to interact with certain
services. For example, the `dhcpd` project could decide to provide an mcl module
as either the primary or secondary interface for managing that service, and in
that case, it would make sense for that module to live in that source tree.

## Importing?

You can import any of these modules into your `mcl` project with the following
example for the `purpleidea` module:

```mcl
import "git://github.com/purpleidea/mgmt/modules/purpleidea/"
```

## Bugs, questions, thanks?

Reach out and let us know!
