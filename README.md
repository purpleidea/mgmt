# *mgmt*: This is: mgmt!

[![Build Status](https://secure.travis-ci.org/purpleidea/mgmt.png?branch=master)](http://travis-ci.org/purpleidea/mgmt)
[![Documentation](https://img.shields.io/docs/markdown.png)](DOCUMENTATION.md)
[![IRC](https://img.shields.io/irc/%23mgmtconfig.png)](https://webchat.freenode.net/?channels=#mgmtconfig)
[![Jenkins](https://img.shields.io/jenkins/status.png)](https://ci.centos.org/job/purpleidea-mgmt/)
[![COPR](https://img.shields.io/copr/builds.png)](https://copr.fedoraproject.org/coprs/purpleidea/mgmt/)

## Community:
Come join us on IRC in [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) on Freenode!
You may like the [#mgmtconfig](https://twitter.com/hashtag/mgmtconfig) hashtag if you're on [Twitter](https://twitter.com/#!/purpleidea).

## Questions:
Please join the [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) IRC community!
If you have a well phrased question that might benefit others, consider asking it by sending a patch to the documentation [FAQ](https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md#usage-and-frequently-asked-questions) section. I'll merge your question, and a patch with the answer!

## Quick start:
* Either get the golang dependencies on your own, or run `make deps` if you're comfortable with how we install them.
* Run `make build` to get a freshly built `mgmt` binary.
* Run `cd $(mktemp --tmpdir -d tmp.XXX) && etcd` to get etcd running. The `mgmt` software will do this automatically for you in the future.
* Run `time ./mgmt run --file examples/graph0.yaml --converged-timeout=1` to try out a very simple example!
* To run continuously in the default mode of operation, omit the `--converged-timeout` option.
* Have fun hacking on our future technology!

## Examples:
Please look in the [examples/](examples/) folder for more examples!

## Documentation:
Please see: [DOCUMENTATION.md](DOCUMENTATION.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md).

## Roadmap:
Please see: [TODO.md](TODO.md) for a list of upcoming work and TODO items.
Please get involved by working on one of these items or by suggesting something else!
Feel free to grab one of the straightforward [#mgmtlove](https://github.com/purpleidea/mgmt/labels/mgmtlove) issues if you're a first time contributor to the project or if you're unsure about what to hack on!

## Bugs:
Please set the `DEBUG` constant in [main.go](https://github.com/purpleidea/mgmt/blob/master/main.go) to `true`, and post the logs when you report the [issue](https://github.com/purpleidea/mgmt/issues).
Bonus points if you provide a [shell](https://github.com/purpleidea/mgmt/tree/master/test/shell) or [OMV](https://github.com/purpleidea/mgmt/tree/master/test/omv) reproducible test case.
Feel free to read my article on [debugging golang programs](https://ttboj.wordpress.com/2016/02/15/debugging-golang-programs/).

## Dependencies:
* golang 1.4 or higher (required, available in most distros)
* golang libraries (required, available with `go get`)

        go get github.com/coreos/etcd/client
        go get gopkg.in/yaml.v2
        go get gopkg.in/fsnotify.v1
        go get github.com/codegangsta/cli
        go get github.com/coreos/go-systemd/dbus
        go get github.com/coreos/go-systemd/util

* stringer (required for building), available as a package on some platforms, otherwise via `go get`

        go get golang.org/x/tools/cmd/stringer

* pandoc (optional, for building a pdf of the documentation)
* graphviz (optional, for building a visual representation of the graph)

## Patches:
We'd love to have your patches! Please send them by email, or as a pull request.

## On the web:
* Introductory blog post: [https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
* Introductory recording from DevConf.cz 2016: [https://www.youtube.com/watch?v=GVhpPF0j-iE&html5=1](https://www.youtube.com/watch?v=GVhpPF0j-iE&html5=1)
* Introductory recording from CfgMgmtCamp.eu 2016: [https://www.youtube.com/watch?v=fNeooSiIRnA&html5=1](https://www.youtube.com/watch?v=fNeooSiIRnA&html5=1)
* Julian Dunn at CfgMgmtCamp.eu 2016: [https://www.youtube.com/watch?v=kfF9IATUask&t=1949&html5=1](https://www.youtube.com/watch?v=kfF9IATUask&t=1949&html5=1)
* Walter Heck at CfgMgmtCamp.eu 2016: [http://www.slideshare.net/olindata/configuration-management-time-for-a-4th-generation/3](http://www.slideshare.net/olindata/configuration-management-time-for-a-4th-generation/3)
* Marco Marongiu on mgmt: [http://syslog.me/2016/02/15/leap-or-die/](http://syslog.me/2016/02/15/leap-or-die/)
* Felix Frank on puppet to mgmt "transpiling" [https://ffrank.github.io/features/2016/02/18/from-catalog-to-mgmt/](https://ffrank.github.io/features/2016/02/18/from-catalog-to-mgmt/)
* Blog post on automatic edges and the pkg resource: [https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/)
* Blog post on automatic grouping: [https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/](https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/)

##

Happy hacking!
