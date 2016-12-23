# *mgmt*: next generation config management!

[![mgmt!](art/mgmt.png)](art/)

[![Go Report Card](https://goreportcard.com/badge/github.com/purpleidea/mgmt)](https://goreportcard.com/report/github.com/purpleidea/mgmt)
[![Build Status](https://secure.travis-ci.org/purpleidea/mgmt.png?branch=master)](http://travis-ci.org/purpleidea/mgmt)
[![Documentation](https://img.shields.io/docs/markdown.png)](docs/documentation.md)
[![GoDoc](https://godoc.org/github.com/purpleidea/mgmt?status.svg)](https://godoc.org/github.com/purpleidea/mgmt)
[![IRC](https://img.shields.io/irc/%23mgmtconfig.png)](https://webchat.freenode.net/?channels=#mgmtconfig)
[![Jenkins](https://img.shields.io/jenkins/status.png)](https://ci.centos.org/job/purpleidea-mgmt/)
[![COPR](https://img.shields.io/copr/builds.png)](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/)
[![arch](https://img.shields.io/arch/aur.png)](https://aur.archlinux.org/packages/mgmt/)

## Community:
Come join us on IRC in [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) on Freenode!
You may like the [#mgmtconfig](https://twitter.com/hashtag/mgmtconfig) hashtag if you're on [Twitter](https://twitter.com/#!/purpleidea).

## Status:
Mgmt is a fairly new project.
We're working towards being minimally useful for production environments.
We aren't feature complete for what we'd consider a 1.x release yet.
With your help you'll be able to influence our design and get us there sooner!

## Questions:
Please join the [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) IRC community!
If you have a well phrased question that might benefit others, consider asking it by sending a patch to the documentation [FAQ](https://github.com/purpleidea/mgmt/blob/master/docs/documentation.md#usage-and-frequently-asked-questions) section. I'll merge your question, and a patch with the answer!

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
Please look in the [examples/](examples/) folder for more examples!

## Documentation:
Please see: the manually created [documentation.md](docs/documentation.md) (also available as [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/docs/documentation.md)) and the automatically generated [GoDoc documentation](https://godoc.org/github.com/purpleidea/mgmt).

## Roadmap:
Please see: [TODO.md](TODO.md) for a list of upcoming work and TODO items.
Please get involved by working on one of these items or by suggesting something else!
Feel free to grab one of the straightforward [#mgmtlove](https://github.com/purpleidea/mgmt/labels/mgmtlove) issues if you're a first time contributor to the project or if you're unsure about what to hack on!

## Bugs:
Please set the `DEBUG` constant in [main.go](https://github.com/purpleidea/mgmt/blob/master/main.go) to `true`, and post the logs when you report the [issue](https://github.com/purpleidea/mgmt/issues).
Bonus points if you provide a [shell](https://github.com/purpleidea/mgmt/tree/master/test/shell) or [OMV](https://github.com/purpleidea/mgmt/tree/master/test/omv) reproducible test case.
Feel free to read my article on [debugging golang programs](https://ttboj.wordpress.com/2016/02/15/debugging-golang-programs/).

## Dependencies:
* golang 1.6 or higher (required, available in most distros)
* golang libraries (required, available with `go get`)
```
go get github.com/coreos/etcd/client
go get gopkg.in/yaml.v2
go get gopkg.in/fsnotify.v1
go get github.com/urfave/cli
go get github.com/coreos/go-systemd/dbus
go get github.com/coreos/go-systemd/util
go get github.com/coreos/pkg/capnslog
go get github.com/rgbkrk/libvirt-go
```
* stringer (optional for building), available as a package on some platforms, otherwise via `go get`
```
go get golang.org/x/tools/cmd/stringer
```
* pandoc (optional, for building a pdf of the documentation)
* graphviz (optional, for building a visual representation of the graph)

## Patches:
We'd love to have your patches! Please send them by email, or as a pull request.

## On the web:
* James Shubin; blog: [Next generation configuration mgmt](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
* James Shubin; video: [Introductory recording from DevConf.cz 2016](https://www.youtube.com/watch?v=GVhpPF0j-iE&html5=1)
* James Shubin; video: [Introductory recording from CfgMgmtCamp.eu 2016](https://www.youtube.com/watch?v=fNeooSiIRnA&html5=1)
* Julian Dunn; video: [On mgmt at CfgMgmtCamp.eu 2016](https://www.youtube.com/watch?v=kfF9IATUask&t=1949&html5=1)
* Walter Heck; slides: [On mgmt at CfgMgmtCamp.eu 2016](http://www.slideshare.net/olindata/configuration-management-time-for-a-4th-generation/3)
* Marco Marongiu; blog: [On mgmt](http://syslog.me/2016/02/15/leap-or-die/)
* Felix Frank; blog: [From Catalog To Mgmt (on puppet to mgmt "transpiling")](https://ffrank.github.io/features/2016/02/18/from-catalog-to-mgmt/)
* James Shubin; blog: [Automatic edges in mgmt (...and the pkg resource)](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/)
* James Shubin; blog: [Automatic grouping in mgmt](https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/)
* John Arundel; tweet: [“Puppet’s days are numbered.”](https://twitter.com/bitfield/status/732157519142002688)
* Felix Frank; blog: [Puppet, Meet Mgmt (on puppet to mgmt internals)](https://ffrank.github.io/features/2016/06/12/puppet,-meet-mgmt/)
* Felix Frank; blog: [Puppet Powered Mgmt (puppet to mgmt tl;dr)](https://ffrank.github.io/features/2016/06/19/puppet-powered-mgmt/)
* James Shubin; blog: [Automatic clustering in mgmt](https://ttboj.wordpress.com/2016/06/20/automatic-clustering-in-mgmt/)
* James Shubin; video: [Recording from CoreOSFest 2016](https://www.youtube.com/watch?v=KVmDCUA42wc&html5=1)
* James Shubin; video: [Recording from DebConf16](http://meetings-archive.debian.net/pub/debian-meetings/2016/debconf16/Next_Generation_Config_Mgmt.webm) ([Slides](https://annex.debconf.org//debconf-share/debconf16/slides/15-next-generation-config-mgmt.pdf))
* Felix Frank; blog: [Edging It All In (puppet and mgmt edges)](https://ffrank.github.io/features/2016/07/12/edging-it-all-in/)
* Felix Frank; blog: [Translating All The Things (puppet to mgmt translation warnings)](https://ffrank.github.io/features/2016/08/19/translating-all-the-things/)
* James Shubin; video: [Recording from systemd.conf 2016](https://www.youtube.com/watch?v=jB992Zb3nH0&html5=1)
* James Shubin; blog: [Remote execution in mgmt](https://ttboj.wordpress.com/2016/10/07/remote-execution-in-mgmt/)
* James Shubin; video: [Recording from High Load Strategy 2016](https://vimeo.com/191493409)
* James Shubin; video: [Recording from NLUUG 2016](https://www.youtube.com/watch?v=MmpwOQAb_SE&html5=1)
* James Shubin; blog: [Send/Recv in mgmt](https://ttboj.wordpress.com/2016/12/07/sendrecv-in-mgmt/)

##

Happy hacking!
