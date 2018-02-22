# Development

This document contains some additional information and help regarding
developing `mgmt`. Useful tools, conventions, etc.

Be sure to read [quick start guide](docs/quick-start-guide.md) first.

## Testing

This project has both unit tests in the form of golang tests and integration
tests using shell scripting.

Native golang tests are preferred over tests written in our shell testing
framework. Please see [https://golang.org/pkg/testing/](https://golang.org/pkg/testing/)
for more information.

To run all tests:

```
make test
```

There is a library of quick and small integration tests for the language and
YAML related things, check out [`test/shell/`](/test/shell). Adding a test is as
easy as copying one of the files in [`test/shell/`](/test/shell) and adapting
it.

This test suite won't run by default (unless when on CI server) and needs to be
called explictly using:

```
make test-shell
```

Or run an individual shell test using:

```
make test-shell-load0
```

Tip: you can use TAB completion with `make` to quickly get a list of possible
individual tests to run.
