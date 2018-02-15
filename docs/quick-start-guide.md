# Quick start guide

## Introduction
This guide is intended for developers. Once `mgmt` is minimally viable, we'll
publish a quick start guide for users too. If you're brand new to `mgmt`, it's
probably a good idea to start by reading the
[introductory article](https://purpleidea.com/blog/2016/01/18/next-generation-configuration-mgmt/)
or to watch an [introductory video](https://www.youtube.com/watch?v=LkEtBVLfygE&html5=1).
Once you're familiar with the general idea, please start hacking...

## Quick start

### Installing golang
* You need golang version 1.9 or greater installed.
** To install on rpm style systems: `sudo dnf install golang`
** To install on apt style systems: `sudo apt install golang`
** To install on macOS systems install [Homebrew](https://brew.sh) and run: `brew install go`
* You can run `go version` to check the golang version.
* If your distro is tool old, you may need to [download](https://golang.org/dl/) a newer golang version.

### Setting up golang
* If you do not have a GOPATH yet, create one and export it:
```
mkdir $HOME/gopath
export GOPATH=$HOME/gopath
```
* You might also want to add the GOPATH to your `~/.bashrc` or `~/.profile`.
* For more information you can read the [GOPATH documentation](https://golang.org/cmd/go/#hdr-GOPATH_environment_variable).

### Getting the mgmt code and dependencies
* Download the `mgmt` code into the GOPATH, and switch to that directory:
```
mkdir -p $GOPATH/src/github.com/purpleidea/
cd $GOPATH/src/github.com/purpleidea/
git clone --recursive https://github.com/purpleidea/mgmt/
cd $GOPATH/src/github.com/purpleidea/mgmt
```

* Add $GOPATH/bin to $PATH
```
export PATH=$PATH:$GOPATH/bin
```

* Run `make deps` to install system and golang dependencies. Take a look at `misc/make-deps.sh` for details.
* Run `make build` to get a freshly built `mgmt` binary.

### Running mgmt
* Run `time ./mgmt run --lang examples/lang/hello0.mcl --tmp-prefix` to try out a very simple example!
* Look in that example file that you ran to see if you can figure out what it did!
* Have fun hacking on our future technology and get involved to shape the project!

## Examples
Please look in the [examples/lang/](../examples/lang/) folder for some more examples!

## Vagrant
If you would like to avoid doing the above steps manually, we have prepared a
[Vagrant](https://www.vagrantup.com/) environment for your convenience. From the
project directory, run a `vagrant up`, and then a `vagrant status`. From there,
you can `vagrant ssh` into the `mgmt` machine. The MOTD will explain the rest.

## Information about dependencies
Software projects have a few different kinds of dependencies. There are _build_
dependencies, _runtime_ dependencies, and additionally, a few extra dependencies
required for running the _test_ suite.

### Build
* `golang` 1.9 or higher (required, available in some distros and distributed
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

To build `mgmt` without augeas or libvirt support please run:
`GOTAGS='noaugeas novirt' make build`

## Binary Package Installation
Installation of `mgmt` from distribution packages currently needs improvement.
They are not always up-to-date with git master and as such are not recommended.
At the moment we have:
* [COPR](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/)
* [Arch](https://aur.archlinux.org/packages/mgmt/)

Please contribute more! We'd especially like to see a Debian package!

## OSX/macOS/Darwin development
Developing and running `mgmt` on macOS is currently not supported (but not discouraged either). Meaning it might work but in the case it doesn't you would have to provide your own patches to fix problems (the project maintainer and community are glad to assist where needed).

There are currently some issues that make `mgmt` less suitable to run for provisioning macOS (eg: https://github.com/purpleidea/mgmt/issues/33). But as a client to provision remote servers it should run fine.

Since the primary supported systems are Linux and these are the environments tested for it is wise to run these suites during macOS development as well. To ease this Docker can be levaraged ((Docker for Mac)[https://docs.docker.com/docker-for-mac/]).

Before running any of the commands below create the development Docker image:

```
docker/scripts/build-development
```

This image requires updating every time dependencies (`make-deps.sh`) change.

Then to run the test suite:

```
docker run --rm -ti \
  -v $PWD:/go/src/github.com/purpleidea/mgmt/ \
  -w /go/src/github.com/purpleidea/mgmt/ \
  purpleidea/mgmt:development \
  make test
```

For convenience this command is wrapped in `docker/scripts/exec-development`.

Basically any command can be executed this way. Because the repository source is mounted into the Docker container invocation will be quick and allow rapid testing, example:

```
docker/scripts/exec-development test/test-shell.sh load0.sh
```

Other examples:

```
docker/scripts/exec-development make build
docker/scripts/exec-development ./mgmt run --tmp-prefix --lang examples/lang/load0.mcl
```
