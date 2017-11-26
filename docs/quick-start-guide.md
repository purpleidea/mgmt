# Quick start guide

## Introduction
This guide is intended for developers. Once `mgmt` is minimally viable, we'll
publish a quick start guide for users too. If you're brand new to `mgmt`, it's
probably a good idea to start by reading the
[introductory article](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
or to watch an [introductory video](https://www.youtube.com/watch?v=LkEtBVLfygE&html5=1).
Once you're familiar with the general idea, please start hacking...

## Quick start

### Installing golang
* You need golang version 1.8 or greater installed.
** To install on rpm style systems: `sudo dnf install golang`
** To install on apt style systems: `sudo apt install golang`
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
* Run `make deps` to install system and golang dependencies. Take a look at `misc/make-deps.sh` for details.
* Run `make build` to get a freshly built `mgmt` binary.

### Running mgmt
* Run `time ./mgmt run --yaml examples/graph0.yaml --converged-timeout=5 --tmp-prefix` to try out a very simple example!
* To run continuously in the default mode of operation, omit the `--converged-timeout` option.
* Look in that example file that you ran to see if you can figure out what it did!
* The yaml frontend is provided as a developer tool to test the engine until the language is ready.
* Have fun hacking on our future technology and get involved to shape the project!

## Examples
Please look in the [examples/](../examples/) folder for some more examples!

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
* `golang` 1.8 or higher (required, available in some distros and distributed
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
