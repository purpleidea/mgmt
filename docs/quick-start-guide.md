# Quick start guide

## Introduction

This guide is intended for users and developers. If you're brand new to `mgmt`,
it's probably a good idea to start by reading an
[introductory article about the engine](https://purpleidea.com/blog/2016/01/18/next-generation-configuration-mgmt/)
and an [introductory article about the language](https://purpleidea.com/blog/2018/02/05/mgmt-configuration-language/).
[There are other articles and videos available](on-the-web.md) if you'd like to
learn more or prefer different formats. Once you're familiar with the general
idea, or if you prefer a hands-on approach, please start hacking...

## Getting mgmt

You can either build `mgmt` from source, or you can download a pre-built
release. There are also some distro repositories available, but they may not be
up to date. A pre-built release is the fastest option if there's one that's
available for your platform. If you are developing or testing a new patch to
`mgmt`, or there is not a release available for your platform, then you'll have
to build your own.

### Downloading a pre-built release:

The latest releases can be found [here](https://github.com/purpleidea/mgmt/releases/).
An alternate mirror is available [here](https://dl.fedoraproject.org/pub/alt/purpleidea/mgmt/releases/).

Make sure to verify the signatures of all packages before you use them. The
signing key can be downloaded from [https://purpleidea.com/contact/#pgp-key](https://purpleidea.com/contact/#pgp-key)
to verify the release.

If you've decided to install a pre-build release, you can skip to the
[Running mgmt](#running-mgmt) section below!

### Building a release:

You'll need some dependencies, including `golang`, and some associated tools.

#### Installing golang

* You need golang version 1.11 or greater installed.
	* To install on rpm style systems: `sudo dnf install golang`
	* To install on apt style systems: `sudo apt install golang`
	* To install on macOS systems install [Homebrew](https://brew.sh)
	and run: `brew install go`
* You can run `go version` to check the golang version.
* If your distro is too old, you may need to [download](https://golang.org/dl/)
a newer golang version.

#### Setting up golang

* You can skip this step, as your installation will default to using `~/go/`,
but if you do not have a `GOPATH` yet and want one in a custom location, create
one and export it:

```shell
mkdir $HOME/gopath
export GOPATH=$HOME/gopath
```

* You might also want to add the GOPATH to your `~/.bashrc` or `~/.profile`.
* For more information you can read the
[GOPATH documentation](https://golang.org/cmd/go/#hdr-GOPATH_environment_variable).

#### Getting the mgmt code and associated dependencies

* Download the `mgmt` code into the `GOPATH`, and switch to that directory:

```shell
[ -z "$GOPATH" ] && mkdir ~/go/ || mkdir -p $GOPATH/src/github.com/purpleidea/
cd $GOPATH/src/github.com/purpleidea/ || cd ~/go/
git clone --recursive https://github.com/purpleidea/mgmt/
cd $GOPATH/src/github.com/purpleidea/mgmt/ || cd ~/go/src/github.com/purpleidea/mgmt/
```

* Add `$GOPATH/bin` to `$PATH`

```shell
export PATH=$PATH:$GOPATH/bin
```

* Run `make deps` to install system and golang dependencies. Take a look at
`misc/make-deps.sh` if you want to see the details of what it does.

#### Building mgmt

* Now run `make` to get a freshly built `mgmt` binary. If this succeeds, you can
proceed to the [Running mgmt](#running-mgmt) section below!

### Installing a distro release

Installation of `mgmt` from distribution packages currently needs improvement.
They are not always up-to-date with git master and as such are not recommended.
At the moment we have:
* [COPR](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/) (currently dead)
* [Arch](https://aur.archlinux.org/packages/mgmt/) (currently stale)

Please contribute more and help improve these! We'd especially like to see a
Debian package!

## Running mgmt

* Run `mgmt run --tmp-prefix lang examples/lang/hello0.mcl` to try out a very
simple example! If you built it from source, you'll need to use `./mgmt` from
the project directory.
* Look in that example file that you ran to see if you can figure out what it
did! You can press `^C` to exit `mgmt`.
* Have fun hacking on our future technology and get involved to shape the
project!

## Examples

Please look in the [examples/lang/](../examples/lang/) folder for some more
examples!
