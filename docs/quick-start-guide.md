#mgmt

<!--
Mgmt
Copyright (C) 2013-2016+ James Shubin and the project contributors
Written by James Shubin <james@shubin.ca> and the project contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

##mgmt quick start guide
####Available from:
####[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/)

####This documentation is available in: [Markdown](https://github.com/purpleidea/mgmt/blob/master/docs/quick-start-guide.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/docs/quick-start-guide.md) format.

####Table of Contents

0. [Introduction](#introduction)
1. [Dependencies](#dependencies)
2. [Quick start](#quick-start)
3. [Examples](#examples)
4. [Installation](#installation)
5. [Authors - Authors and contact information](#authors)

## Introduction:
This guide is intended for developers. Once `mgmt` is minimally viable, we'll
publish a quick start guide for users too. In the meantime, please contribute!
If you're brand new to `mgmt`, it's probably a good idea to start by reading the
[introductory article](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
or to watch an [introductory video](https://github.com/purpleidea/mgmt/#on-the-web).
Once you're familiar with the general idea, please start hacking...

## Dependencies:
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

###Runtime
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

###Testing
* golint `github.com/golang/lint/golint`

## Quick start:
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
go get -u github.com/purpleidea/mgmt
cd $GOPATH/src/github.com/purpleidea/mgmt
```
* Run `make deps` to install system and golang dependencies. Take a look at `misc/make-deps.sh` for details.
* Run `make build` to get a freshly built `mgmt` binary.
* Run `time ./mgmt run --yaml examples/graph0.yaml --converged-timeout=5 --tmp-prefix` to try out a very simple example!
* To run continuously in the default mode of operation, omit the `--converged-timeout` option.
* Have fun hacking on our future technology!

## Examples:
Please look in the [examples/](../examples/) folder for some examples!

## Installation:
Installation of `mgmt` from distribution packages currently needs improvement.
At the moment we have:
* [COPR](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/)
* [Arch](https://aur.archlinux.org/packages/mgmt/)

Please contribute more! We'd especially like to see a Debian package!

##Authors
Copyright (C) 2013-2016+ James Shubin and the project contributors

Please see the
[AUTHORS](https://github.com/purpleidea/mgmt/tree/master/AUTHORS) file
for more information.

* [github](https://github.com/purpleidea/)
* [&#64;purpleidea](https://twitter.com/#!/purpleidea)
* [https://ttboj.wordpress.com/](https://ttboj.wordpress.com/)
