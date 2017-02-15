# Quick start guide

## Introduction
This guide is intended for developers. Once `mgmt` is minimally viable, we'll
publish a quick start guide for users too. In the meantime, please contribute!
If you're brand new to `mgmt`, it's probably a good idea to start by reading the
[introductory article](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
or to watch an [introductory video](https://github.com/purpleidea/mgmt/#on-the-web).
Once you're familiar with the general idea, please start hacking...

## Vagrant
If you would like to avoid doing the following steps manually, we have prepared
a [Vagrant](https://www.vagrantup.com/) environment for your convenience. From
the project directory, run a `vagrant up`, and then a `vagrant status`. From
there, you can `vagrant ssh` into the `mgmt` machine. The MOTD will explain the
rest.

## Dependencies
Software projects have a few different kinds of dependencies. There are _build_
dependencies, _runtime_ dependencies, and additionally, a few extra dependencies
required for running the _test_ suite.

### Build
* `golang` 1.6 or higher (required, available in most distros)
* golang libraries (required, available with `go get ./...`) a partial list includes:
```
github.com/coreos/etcd/client
gopkg.in/yaml.v2
gopkg.in/fsnotify.v1
github.com/urfave/cli
github.com/coreos/go-systemd/dbus
github.com/coreos/go-systemd/util
github.com/libvirt/libvirt-go
```
* `stringer` (optional), available as a package on some platforms, otherwise via `go get`
```
golang.org/x/tools/cmd/stringer
```
* `pandoc` (optional), for building a pdf of the documentation

### Runtime
A relatively modern GNU/Linux system should be able to run `mgmt` without any
problems. Since `mgmt` runs as a single statically compiled binary, all of the
library dependencies are included. It is expected, that certain advanced
resources require host specific facilities to work. These requirements are
listed below:

| Resource | Dependency        | Version |
|----------|-------------------|---------|
| file     | inotify           | ?       |
| hostname | systemd-hostnamed | ?       |
| nspawn   | systemd-nspawn    | ?       |
| pkg      | packagekitd       | ?       |
| svc      | systemd           | ?       |
| virt     | libvirtd          | ?       |

For building a visual representation of the graph, `graphviz` is required.

### Testing
* golint `github.com/golang/lint/golint`

## Quick start
* Make sure you have golang version 1.6 or greater installed.
* If you do not have a GOPATH yet, create one and export it:
```
mkdir $HOME/gopath
export GOPATH=$HOME/gopath
```
* You might also want to add the GOPATH to your `~/.bashrc` or `~/.profile`.
* For more information you can read the [GOPATH documentation](https://golang.org/cmd/go/#hdr-GOPATH_environment_variable).
* Next download the mgmt code base, and switch to that directory:
```
mkdir -p $GOPATH/src/github.com/purpleidea/
cd $GOPATH/src/github.com/purpleidea/
git clone --recursive https://github.com/purpleidea/mgmt/
cd $GOPATH/src/github.com/purpleidea/mgmt
```
* Run `make deps` to install system and golang dependencies. Take a look at `misc/make-deps.sh` for details.
* Run `make build` to get a freshly built `mgmt` binary.
* Run `time ./mgmt run --yaml examples/graph0.yaml --converged-timeout=5 --tmp-prefix` to try out a very simple example!
* To run continuously in the default mode of operation, omit the `--converged-timeout` option.
* Have fun hacking on our future technology!

## Examples
Please look in the [examples/](../examples/) folder for some examples!

## Installation
Installation of `mgmt` from distribution packages currently needs improvement.
At the moment we have:
* [COPR](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/)
* [Arch](https://aur.archlinux.org/packages/mgmt/)

Please contribute more! We'd especially like to see a Debian package!
