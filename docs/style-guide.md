# Style guide

## Overview

This document aims to be a reference for the desired style for patches to mgmt.
In particular it describes conventions which we use which are not officially
enforced by the `gofmt` tool, and which might not be clearly defined elsewhere.
Most of these are common sense to seasoned programmers, and we hope this will be
a useful reference for new programmers.

There are a lot of useful code review comments described
[here](https://github.com/golang/go/wiki/CodeReviewComments). We don't
necessarily follow everything strictly, but it is in general a very good guide.

## Basics

* All of our golang code is formatted with `gofmt`.

## Comments

All of our code is commented with the minimums required for `godoc` to function,
and so that our comments pass `golint`. Code comments should either be full
sentences (which end with a period, use proper punctuation, and capitalize the
first word when it is not a lower cased identifier), or are short one-line
comments in the source which are not full sentences and don't end with a period.

They should explain algorithms, describe non-obvious behaviour, or situations
which would otherwise need explanation or additional research during a code
review. Notes about use of unfamiliar API's is a good idea for a code comment.

### Example

Here you can see a function with the correct `godoc` string. The first word must
match the name of the function. It is _not_ capitalized because the function is
private.

```golang
// square multiplies the input integer by itself and returns this product.
func square(x int) int {
	return x * x // we don't care about overflow errors
}
```

## Line length

In general we try to stick to 80 character lines when it is appropriate. It is
almost *always* appropriate for function `godoc` comments and most longer
paragraphs. Exceptions are always allowed based on the will of the maintainer.

It is usually better to exceed 80 characters than to break code unnecessarily.
If your code often exceeds 80 characters, it might be an indication that it
needs refactoring.

Occasionally inline, two line source code comments are used within a function.
These should usually be balanced so that you don't have one line with 78
characters and the second with only four. Split the comment between the two.

## Method receiver naming

[Contrary](https://github.com/golang/go/wiki/CodeReviewComments#receiver-names)
to the specialized naming of the method receiver variable, we usually name all
of these `obj` for ease of code copying throughout the project, and for faster
identification when reviewing code. Some anecdotal studies have shown that it
makes the code easier to read since you don't need to remember the name of the
method receiver variable in each different method. This is very similar to what
is done in `python`.

### Example

```golang
// Bar does a thing, and returns the number of baz results found in our
database.
func (obj *Foo) Bar(baz string) int {
	if len(obj.s) > 0 {
		return strings.Count(obj.s, baz)
	}
	return -1
}
```

## Consistent ordering

In general we try to preserve a logical ordering in source files which usually
matches the common order of execution that a _lazy evaluator_ would follow.

This is also the order which is recommended when creating interface types. When
implementing an interface, arrange your methods in the same order that they are
declared in the interface.

When implementing code for the various types in the language, please follow this
order: `bool`, `str`, `int`, `float`, `list`, `map`, `struct`, `func`.

## Suggestions

If you have any ideas for suggestions or other improvements to this guide,
please let us know!
