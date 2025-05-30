# Style guide

This document aims to be a reference for the desired style for patches to mgmt,
and the associated `mcl` language. In particular it describes conventions which
are not officially enforced by tools and in test cases, or that aren't clearly
defined elsewhere. We try to turn as many of these into automated tests as we
can. If something here is not defined in a test, or you think it should be,
please write one! Even better, you can write a tool to automatically fix it,
since this is more useful and can easily be turned into a test!

## Overview for golang code

Most style issues are enforced by the `gofmt` tool. Other style aspects are
often common sense to seasoned programmers, and we hope this will be a useful
reference for new programmers.

There are a lot of useful code review comments described
[here](https://github.com/golang/go/wiki/CodeReviewComments). We don't
necessarily follow everything strictly, but it is in general a very good guide.

### Basics

* All of our golang code is formatted with `gofmt`.

### Comments

All of our code is commented with the minimums required for `godoc` to function,
and so that our comments pass `golint`. Code comments should either be full
sentences (which end with a period, use proper punctuation, and capitalize the
first word when it is not a lower cased identifier), or are short one-line
comments in the source which are not full sentences and don't end with a period.

They should explain algorithms, describe non-obvious behaviour, or situations
which would otherwise need explanation or additional research during a code
review. Notes about use of unfamiliar API's is a good idea for a code comment.

#### Example

Here you can see a function with the correct `godoc` string. The first word must
match the name of the function. It is _not_ capitalized because the function is
private.

```golang
// square multiplies the input integer by itself and returns this product.
func square(x int) int {
	return x * x // we don't care about overflow errors
}
```

### Line length

In general we try to stick to 80 character lines when it is appropriate. It is
almost *always* appropriate for function `godoc` comments and most longer
paragraphs. Exceptions are always allowed based on the will of the maintainer.

It is usually better to exceed 80 characters than to break code unnecessarily.
If your code often exceeds 80 characters, it might be an indication that it
needs refactoring.

Occasionally inline, two line source code comments are used within a function.
These should usually be balanced so that you don't have one line with 78
characters and the second with only four. Split the comment between the two.

### Default values

Whenever a constant or function parameter is defined, try and have the safer or
default value be the `zero` value. For example, instead of `const NoDanger`, use
`const AllowDanger` so that the `false` value is the safe scenario.

### Method receiver pointers

You almost always want any method receivers to be declared on the pointer to the
struct. There are only a few rare situations where this is not the case. This
makes it easier to merge future changes that mutate the state without wondering
why you now have two different copies of a struct. When you do need to copy a
a struct, you can add a `Copy()` method to it. It's true that in many situations
adding the pointer adds a small performance penalty, but we haven't found them
to be significant in practice. If you do have a performance sensitive patch
which benefits from skipping the pointer, please demonstrate this need with
data first.

#### Example

```golang
type Foo struct {
	Whatever string
	// ...
}

// Bar is implemented correctly as a pointer on Foo.
func (obj *Foo) Bar(baz string) int {
	// ...
}

// Bar is implemented *incorrectly* without a pointer to Foo.
func (obj Foo) Bar(baz string) int {
	// ...
}
```

### Method receiver naming

[Contrary](https://github.com/golang/go/wiki/CodeReviewComments#receiver-names)
to the specialized naming of the method receiver variable, we usually name all
of these `obj` for ease of code copying throughout the project, and for faster
identification when reviewing code. Some anecdotal studies have shown that it
makes the code easier to read since you don't need to remember the name of the
method receiver variable in each different method. This is very similar to what
is done in `python`.

#### Example

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

### Variable naming

We prefer shorter, scoped variables rather than `unnecessarilyLongIdentifiers`.
Remember the scoping rules and feel free to use new variables where appropriate.
For example, in a short string snippet you can use `s` instead of `myString`, as
well as other common choices. `i` is a common `int` counter, `f` for files, `fn`
for functions, `x` for something else and so on.

### Variable reuse

Feel free to create and use new variables instead of attempting to reuse the
same string. For example, if a function input arg is named `s`, you can use a
new variable to receive the first computation result on `s` instead of storing
it back into the original `s`. This avoids confusion if a different part of the
code wants to read the original input, and it avoids any chance of edit by
reference of the original callers copy of the variable.

#### Example

```golang
MyNotIdealFunc(s string, b bool) string {
	if !b {
		return s + "hey"
	}
	s = strings.Replace(s, "blah", "", -1) // not ideal (reuse of `s` var)
	return s
}

MyOkayFunc(s string, b bool) string {
	if !b {
		return s + "hey"
	}
	s2 := strings.Replace(s, "blah", "", -1) // doesn't reuse `s` variable
	return s2
}

MyGreatFunc(s string, b bool) string {
	if !b {
		return s + "hey"
	}
	return strings.Replace(s, "blah", "", -1) // even cleaner
}
```

### Constants in code

If a function takes a specifier (often a bool) it's sometimes better to name
that variable (often with a `const`) rather than leaving a naked `bool` in the
code. For example, `x := MyFoo("blah", false)` is less clear than
`const useMagic = false; x := MyFoo("blah", useMagic)`.

### Empty slice declarations

When declaring a new empty slice, there are three different mechanisms:

1. `a := []string{}`

2. `var a []string`

3. `a := make([]string, 0)`

In general, we prefer the first method because we find that it is succinct, and
very readable. The third method is the least recommended because you're adding
extra data that a smart compiler could probably figure out on its own. There are
performance implications between these three methods, so unless your code is in
a fast path or memory constrained environment where this matters (and that you
ideally have proof of this) please use the methods as ordered as much as
possible.

### Consistent ordering

In general we try to preserve a logical ordering in source files which usually
matches the common order of execution that a _lazy evaluator_ would follow.

This is also the order which is recommended when creating interface types. When
implementing an interface, arrange your methods in the same order that they are
declared in the interface.

When implementing code for the various types in the language, please follow this
order: `bool`, `str`, `int`, `float`, `list`, `map`, `struct`, `func`.

For other aspects where you have a set of items, try to be internally consistent
as well. For example, if you have two switch statements with `A`, `B`, and `C`,
please use the same ordering for these elements elsewhere that they appear in
the code and in the commentary if it is not illogical to do so.

### Product identifiers

Try to avoid references in the code to `mgmt` or a specific program name string
if possible. This makes it easier to rename code if we ever pick a better name
or support `libmgmt` better if we embed it. You can use the `Program` variable
which is available in numerous places if you want a string to put in the logs.

It is also recommended to avoid the `go` (programming language name) string if
possible. Try to use `golang` if required, since the word `go` is already
overloaded, and in particular it was even already used by the
[`go!`](https://en.wikipedia.org/wiki/Go!_(programming_language)).

## Overview for mcl code

The `mcl` language is quite new, so this guide will probably change over time as
we find what's best, and hopefully we'll be able to add an `mclfmt` tool in the
future so that less of this needs to be documented. (Patches welcome!)

### Indentation

Code indentation is done with tabs. The tab-width is a private preference, which
is the beauty of using tabs: you can have your own personal preference. The
inventor of `mgmt` uses and recommends a width of eight, and that is what should
be used if your tool requires a modeline to be publicly committed.

### Line length

We recommend you stick to 80 char line width. If you find yourself with deeper
nesting, it might be a hint that your code could be refactored in a more
pleasant way.

### Capitalization

At the moment, variables, function names, and classes are all lowercase and do
not contain underscores. We will probably figure out what style to recommend
when the language is a bit further along. For example, we haven't decided if we
should have a notion of public and private variables, and if we'd like to
reserve capitalization for this situation.

### Module naming

We recommend you name your modules with an `mgmt-` prefix. For example, a module
about bananas might be named `mgmt-banana`. This is helpful for the useful magic
built-in to the module import code, which will by default take a remote import
like: `import "https://github.com/purpleidea/mgmt-banana/"` and namespace it as
`banana`. Of course you can always pick the namespace yourself on import with:
`import "https://github.com/purpleidea/mgmt-banana/" as tomato` or something
similar.

### Licensing

We believe that sharing code helps reduce unnecessary re-invention, so that we
can [stand on the shoulders of giants](https://en.wikipedia.org/wiki/Standing_on_the_shoulders_of_giants)
and hopefully make faster progress in science, medicine, exploration, etc... As
a result, we recommend releasing your modules under the [LGPLv3+](https://www.gnu.org/licenses/lgpl-3.0.en.html)
license for the maximum balance of freedom and re-usability. We strongly oppose
any [CLA](https://en.wikipedia.org/wiki/Contributor_License_Agreement)
requirements and believe that the ["inbound==outbound"](https://ref.fedorapeople.org/fontana-linuxcon.html#slide2)
rule applies. Lastly, we do not support software patents and we hope you don't
either!

## Suggestions

If you have any ideas for suggestions or other improvements to this guide,
please let us know!
