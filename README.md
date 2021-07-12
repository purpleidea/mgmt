# *mgmt*: next generation config management!

[![mgmt!](art/mgmt.png)](art/)

[![Go Report Card](https://goreportcard.com/badge/github.com/purpleidea/mgmt?style=flat-square)](https://goreportcard.com/report/github.com/purpleidea/mgmt)
[![Build Status](https://img.shields.io/travis/purpleidea/mgmt/master.svg?style=flat-square)](http://travis-ci.org/purpleidea/mgmt)
[![Build Status](https://github.com/purpleidea/mgmt/workflows/.github/workflows/test.yaml/badge.svg)](https://github.com/purpleidea/mgmt/actions/)
[![GoDoc](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square)](https://godoc.org/github.com/purpleidea/mgmt)
[![IRC](https://img.shields.io/badge/irc-%23mgmtconfig-orange.svg?style=flat-square)](https://web.libera.chat/?channels=#mgmtconfig)
[![Patreon](https://img.shields.io/badge/patreon-donate-yellow.svg?style=flat-square)](https://www.patreon.com/purpleidea)
[![Liberapay](https://img.shields.io/badge/liberapay-donate-yellow.svg?style=flat-square)](https://liberapay.com/purpleidea/donate)

## About:

`Mgmt` is a real-time automation tool. It is familiar to existing configuration
management software, but is drastically more powerful as it can allow you to
build real-time, closed-loop feedback systems, in a very safe way, and with a
surprisingly small amout of our `mcl` code. For example, the following code will
ensure that your file server is set to read-only when it's friday.

```mcl
import "datetime"
$is_friday = datetime.weekday(datetime.now()) == "friday"
file "/srv/files/" {
	state => $const.res.file.state.exists,
	mode => if $is_friday { # this updates the mode, the instant it changes!
		"0550"
	} else {
		"0770"
	},
}
```

It can run continuously, intermittently, or on-demand, and in the first case, it
will guarantee that your system is always in the desired state for that instant!
In this mode it can run as a decentralized cluster of agents across your
network, each exchanging information with the others in real-time, to respond to
your changing needs. For example, if you want to ensure that some resource runs
on a maximum of two hosts in your cluster, you can specify that as well:

```mcl
import "sys"
import "world"

# we'll set a few scheduling options:
$opts = struct{strategy => "rr", max => 2, ttl => 10,}

# schedule in a particular namespace with options:
$set = world.schedule("xsched", $opts)

if sys.hostname() in $set {
	# use your imagination to put something more complex right here...
	print "i got scheduled" {} # this will run on the chosen machines
}
```

As you add and remove hosts from the cluster, the real-time `schedule` function
will dynamically pick up to two hosts from the available pool. These specific
functions aren't intrinsic to the core design, and new ones can be easily added.

Please read on if you'd like to learn more...

## Community:

Come join us in the `mgmt` community!

| Medium | Link |
|---|---|
| IRC | [#mgmtconfig](https://web.libera.chat/?channels=#mgmtconfig) on Libera.Chat |
| Twitter | [@mgmtconfig](https://twitter.com/mgmtconfig) & [#mgmtconfig](https://twitter.com/hashtag/mgmtconfig) |
| Mailing list | [mgmtconfig-list@redhat.com](https://www.redhat.com/mailman/listinfo/mgmtconfig-list) |
| Patreon | [purpleidea](https://www.patreon.com/purpleidea) on Patreon |
| Liberapay | [purpleidea](https://liberapay.com/purpleidea/donate) on Liberapay |

## Status:

Mgmt is a next generation automation tool. It has similarities to other tools in
the configuration management space, but has a fast, modern, distributed systems
approach. The project contains an engine and a language.
[Please have a look at an introductory video or blog post.](docs/on-the-web.md)

Mgmt is a fairly new project. It is usable today, but not yet feature complete.
With your help you'll be able to influence our design and get us to 1.0 sooner!
Interested users should read the [quick start guide](docs/quick-start-guide.md).

## Documentation:

Please read, enjoy and help improve our documentation!

| Documentation | Additional Notes |
|---|---|
| [quick start guide](docs/quick-start-guide.md) | for everyone |
| [frequently asked questions](docs/faq.md) | for everyone |
| [general documentation](docs/documentation.md) | for everyone |
| [language guide](docs/language-guide.md) | for everyone |
| [function guide](docs/function-guide.md) | for mgmt developers |
| [resource guide](docs/resource-guide.md) | for mgmt developers |
| [style guide](docs/style-guide.md) | for mgmt developers |
| [godoc API reference](https://godoc.org/github.com/purpleidea/mgmt) | for mgmt developers |
| [prometheus guide](docs/prometheus.md) | for everyone |
| [puppet guide](docs/puppet-guide.md) | for puppet sysadmins |
| [development](docs/development.md) | for mgmt developers |

## Questions:

Please ask in the [community](#community)!
If you have a well phrased question that might benefit others, consider asking
it by sending a patch to the [FAQ](docs/faq.md) section. I'll merge your
question, and a patch with the answer!

## Get involved:

Feel free to grab one of the straightforward [#mgmtlove](https://github.com/purpleidea/mgmt/labels/mgmtlove)
issues if you're a first time contributor to the project or if you're unsure
about what to hack on! Please get involved by working on one of these items or
by suggesting something else! There are some lower priority issues and harder
issues available in our [TODO](TODO.md) file. Please have a look.

## Bugs:

Please set the `DEBUG` constant in [main.go](https://github.com/purpleidea/mgmt/blob/master/main.go)
to `true`, and post the logs when you report the [issue](https://github.com/purpleidea/mgmt/issues).
Feel free to read my article on [debugging golang programs](https://purpleidea.com/blog/2016/02/15/debugging-golang-programs/).

## Patches:

We'd love to have your patches! Please send them by email, or as a pull request.

## On the web:

[Read what people are saying and publishing about mgmt!](docs/on-the-web.md)

Happy hacking!
