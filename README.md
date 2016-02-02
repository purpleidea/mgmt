# *mgmt*: This is: mgmt!

[![Build Status](https://secure.travis-ci.org/purpleidea/mgmt.png?branch=master)](http://travis-ci.org/purpleidea/mgmt)
[![Documentation](https://img.shields.io/docs/markdown.png)](DOCUMENTATION.md)
[![IRC](https://img.shields.io/irc/%23mgmtconfig.png)](https://webchat.freenode.net/?channels=#mgmtconfig)
[![Jenkins](https://img.shields.io/jenkins/status.png)](https://ci.centos.org/job/purpleidea-mgmt/)

## Community:
Come join us on IRC in [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) on Freenode!
You may like the [#mgmtconfig](https://twitter.com/hashtag/mgmtconfig) hashtag if you're on [Twitter](https://twitter.com/#!/purpleidea).

## Questions:
Please join the [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) IRC community!
If you have a well phrased question that might benefit others, consider asking it by sending a patch to the documentation [FAQ](https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md#usage-and-frequently-asked-questions) section. I'll merge your question, and a patch with the answer!

## Quick start:
* Either get the golang dependencies on your own, or run `make deps` if you're comfortable with how we install them.
* Run `make build` to get a fresh built `mgmt` binary.
* Run `cd $(mktemp --tmpdir -d tmp.XXX) && etcd` to get etcd running. The `mgmt` software will do this automatically for you in the future.
* Run `time ./mgmt run --file examples/graph0.yaml --converged-timeout=1` to try out a very simple example!
* To run continuously in the default mode of operation, omit the `--converged-timeout` option.
* Have fun hacking on our future technology!

## Examples:
Please look in the [examples/](examples/) folder for more examples!

## Documentation:
Please see: [DOCUMENTATION.md](DOCUMENTATION.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md).

## Bugs:
Please set the `DEBUG` constant in [main.go](https://github.com/purpleidea/mgmt/blob/master/main.go) to `true`, and post the logs when you report the [issue](https://github.com/purpleidea/mgmt/issues).
Bonus points if you provide a [shell](https://github.com/purpleidea/mgmt/tree/master/test/shell) or [OMV](https://github.com/purpleidea/mgmt/tree/master/test/omv) reproducible test case.
There are currently a few known bugs which I hope to squash soon.

## Notes:
* This is currently a research project into next generation config management technologies!
* This is my first complex project in golang, please notify me of any issues.
* I have some well thought out designs for the future of this project, which I'll try and write up clearly and publish as soon as possible.
* The interfaces and code are not yet stable. Please run in development environments only!
* This design is the result of ideas I've had from hacking on advanced config management projects.
* I first started hacking on this in ~2013, even though I had very little time for it.
* I've published a number of articles about this tool:
  * [https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
* There are some screencasts available:
  * TODO

## Dependencies:
* golang (required, available in most distros)
* golang libraries (required, available with `go get`)

        go get github.com/coreos/etcd/client
        go get gopkg.in/yaml.v2
        go get gopkg.in/fsnotify.v1
        go get github.com/codegangsta/cli
        go get github.com/coreos/go-systemd/dbus
        go get github.com/coreos/go-systemd/util

* pandoc (optional, for building a pdf of the documentation)
* graphviz (optional, for building a visual representation of the graph)

## Patches:
We'd love to have your patch! Please send it by email, or as a pull request.

##

Happy hacking!
