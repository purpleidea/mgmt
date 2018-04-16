# *mgmt*: next generation config management!

[![mgmt!](art/mgmt.png)](art/)

[![Go Report Card](https://goreportcard.com/badge/github.com/purpleidea/mgmt?style=flat-square)](https://goreportcard.com/report/github.com/purpleidea/mgmt)
[![Build Status](https://img.shields.io/travis/purpleidea/mgmt/master.svg?style=flat-square)](http://travis-ci.org/purpleidea/mgmt)
[![GoDoc](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square)](https://godoc.org/github.com/purpleidea/mgmt)
[![IRC](https://img.shields.io/badge/irc-%23mgmtconfig-orange.svg?style=flat-square)](https://webchat.freenode.net/?channels=#mgmtconfig)
[![Patreon](https://img.shields.io/badge/patreon-donate-yellow.svg?style=flat-square)](https://www.patreon.com/purpleidea)

## Community:

Come join us in the `mgmt` community!

| Medium | Link |
|---|---|
| IRC | [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig) on Freenode |
| Twitter | [@mgmtconfig](https://twitter.com/mgmtconfig) & [#mgmtconfig](https://twitter.com/hashtag/mgmtconfig) |
| Mailing list | [mgmtconfig-list@redhat.com](https://www.redhat.com/mailman/listinfo/mgmtconfig-list) |
| Patreon | [purpleidea](https://www.patreon.com/purpleidea) on Patreon |

## Status:

Mgmt is a next generation automation tool. It has similarities to other tools in
the configuration management space, but has a fast, modern, distributed systems
approach. The project contains an engine and a language.
[Please have a look at an introductory video or blog post.](docs/on-the-web.md)

Mgmt is a fairly new project. It is usable today, but not yet feature complete.
With your help you'll be able to influence our design and get us to 1.0 sooner!
Interested developers should read the [quick start guide](docs/quick-start-guide.md).

## Documentation:

Please read, enjoy and help improve our documentation!

| Documentation | Additional Notes |
|---|---|
| [quick start guide](docs/quick-start-guide.md) | for mgmt developers |
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

## Roadmap:

Feel free to grab one of the straightforward [#mgmtlove](https://github.com/purpleidea/mgmt/labels/mgmtlove)
issues if you're a first time contributor to the project or if you're unsure
about what to hack on!
Please see: [TODO.md](TODO.md) for a list of upcoming work and TODO items.
Please get involved by working on one of these items or by suggesting something
else!

## Bugs:

Please set the `DEBUG` constant in [main.go](https://github.com/purpleidea/mgmt/blob/master/main.go)
to `true`, and post the logs when you report the [issue](https://github.com/purpleidea/mgmt/issues).
Bonus points if you provide a [shell](https://github.com/purpleidea/mgmt/tree/master/test/shell)
or [OMV](https://github.com/purpleidea/mgmt/tree/master/test/omv) reproducible
test case.
Feel free to read my article on [debugging golang programs](https://purpleidea.com/blog/2016/02/15/debugging-golang-programs/).

## Patches:

We'd love to have your patches! Please send them by email, or as a pull request.

## On the web:

[Read what people are saying and publishing about mgmt!](docs/on-the-web.md)

Happy hacking!
