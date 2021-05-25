# Development

This document contains some additional information and help regarding
developing `mgmt`. Useful tools, conventions, etc.

Be sure to read [quick start guide](quick-start-guide.md) first.

## Vagrant

If you would like to avoid doing the above steps manually, we have prepared a
[Vagrant](https://www.vagrantup.com/) environment for your convenience. From the
project directory, run a `vagrant up`, and then a `vagrant status`. From there,
you can `vagrant ssh` into the `mgmt` machine. The `MOTD` will explain the rest.
This environment isn't commonly used by the `mgmt` developers, so it might not
be working properly.

## Using Docker

Alternatively, you can check out the [docker-guide](docker-guide.md) in order to
develop or deploy using docker. This method is not endorsed or supported, so use
at your own risk, as it might not be working properly.

## Information about dependencies

Software projects have a few different kinds of dependencies. There are _build_
dependencies, _runtime_ dependencies, and additionally, a few extra dependencies
required for running the _test_ suite.

### Build

* `golang` 1.16 or higher (required, available in some distros and distributed
as a binary officially by [golang.org](https://golang.org/dl/))

### Runtime

A relatively modern GNU/Linux system should be able to run `mgmt` without any
problems. Since `mgmt` runs as a single statically compiled binary, all of the
library dependencies are included. It is expected, that certain advanced
resources require host specific facilities to work. These requirements are
listed below:

| Resource | Dependency        | Version                     | Check version with                                        |
|----------|-------------------|-----------------------------|-----------------------------------------------------------|
| augeas   | augeas-devel      | `augeas 1.6` or greater     | `dnf info augeas-devel` or `apt-cache show libaugeas-dev` |
| file     | inotify           | `Linux 2.6.27` or greater   | `uname -a`                                                |
| hostname | systemd-hostnamed | `systemd 25` or greater     | `systemctl --version`                                     |
| nspawn   | systemd-nspawn    | `systemd ???` or greater    | `systemctl --version`                                     |
| pkg      | packagekitd       | `packagekit 1.x` or greater | `pkcon --version`                                         |
| svc      | systemd           | `systemd ???` or greater    | `systemctl --version`                                     |
| virt     | libvirt-devel     | `libvirt 1.2.0` or greater  | `dnf info libvirt-devel` or `apt-cache show libvirt-dev`  |
| virt     | libvirtd          | `libvirt 1.2.0` or greater  | `libvirtd --version`                                      |

For building a visual representation of the graph, `graphviz` is required.

To build `mgmt` without augeas support please run:
`GOTAGS='noaugeas' make build`

To build `mgmt` without libvirt support please run:
`GOTAGS='novirt' make build`

To build `mgmt` without docker support please run:
`GOTAGS='nodocker' make build`

To build `mgmt` without augeas, libvirt or docker support please run:
`GOTAGS='noaugeas novirt nodocker' make build`

## OSX/macOS/Darwin development

Developing and running `mgmt` on macOS is currently not supported (but not
discouraged either). Meaning it might work but in the case it doesn't you would
have to provide your own patches to fix problems (the project maintainer and
community are glad to assist where needed).

There are currently some issues that make `mgmt` less suitable to run for
provisioning macOS. But as a client to provision remote servers it should run
fine.

Since the primary supported systems are Linux and these are the environments
tested, it is wise to run these suites during macOS development as well. To ease
this, Docker can be leveraged ([Docker for Mac](https://docs.docker.com/docker-for-mac/)).

Before running any of the commands below create the development Docker image:

```
docker/scripts/build-development
```

This image requires updating every time dependencies (`make-deps.sh`) changes.

Then to run the test suite:

```
docker run --rm -ti \
	-v $PWD:/go/src/github.com/purpleidea/mgmt/ \
	-w /go/src/github.com/purpleidea/mgmt/ \
	purpleidea/mgmt:development \
	make test
```

For convenience this command is wrapped in `docker/scripts/exec-development`.

Basically any command can be executed this way. Because the repository source is
mounted into the Docker container invocation will be quick and allow rapid
testing, for example:

```
docker/scripts/exec-development test/test-shell.sh load0.sh
```

Other examples:

```
docker/scripts/exec-development make build
docker/scripts/exec-development ./mgmt run --tmp-prefix lang examples/lang/load0.mcl
```

Be advised that this method is not supported and it might not be working
properly.

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

## Tools, integrations, IDE's etc

### IDE/Editor support

* Emacs: see `misc/emacs/`
* [Textmate](https://github.com/aequitas/mgmt.tmbundle)
* [VSCode](https://github.com/aequitas/mgmt.vscode)
