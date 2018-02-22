# General documentation

## Overview

The `mgmt` tool is a next generation config management prototype. It's not yet
ready for production, but we hope to get there soon. Get involved today!

## Project Description

The mgmt tool is a distributed, event driven, config management tool, that
supports parallel execution, and librarification to be used as the management
foundation in and for, new and existing software.

For more information, you may like to read some blog posts from the author:

* [Next generation config mgmt](https://purpleidea.com/blog/2016/01/18/next-generation-configuration-mgmt/)
* [Automatic edges in mgmt](https://purpleidea.com/blog/2016/03/14/automatic-edges-in-mgmt/)
* [Automatic grouping in mgmt](https://purpleidea.com/blog/2016/03/30/automatic-grouping-in-mgmt/)
* [Automatic clustering in mgmt](https://purpleidea.com/blog/2016/06/20/automatic-clustering-in-mgmt/)
* [Remote execution in mgmt](https://purpleidea.com/blog/2016/10/07/remote-execution-in-mgmt/)
* [Send/Recv in mgmt](https://purpleidea.com/blog/2016/12/07/sendrecv-in-mgmt/)
* [Metaparameters in mgmt](https://purpleidea.com/blog/2017/03/01/metaparameters-in-mgmt/)

There is also an [introductory video](https://www.youtube.com/watch?v=LkEtBVLfygE&html5=1) available.
Older videos and other material [is available](on-the-web.md).

## Setup

You'll probably want to read the [quick start guide](quick-start-guide.md) to get going.

## Features

This section details the numerous features of mgmt and some caveats you might
need to be aware of.

### Autoedges

Automatic edges, or AutoEdges, is the mechanism in mgmt by which it will
automatically create dependencies for you between resources. For example,
since mgmt can discover which files are installed by a package it will
automatically ensure that any file resource you declare that matches a
file installed by your package resource will only be processed after the
package is installed.

#### Controlling autoedges

Though autoedges is likely to be very helpful and avoid you having to declare
all dependencies explicitly, there are cases where this behaviour is
undesirable.

Some distributions allow package installations to automatically start the
service they ship. This can be problematic in the case of packages like MySQL
as there are configuration options that need to be set before MySQL is ever
started for the first time (or you'll need to wipe the data directory). In
order to handle this situation you can disable autoedges per resource and
explicitly declare that you want `my.cnf` to be written to disk before the
installation of the `mysql-server` package.

You can disable autoedges for a resource by setting the `autoedge` key on
the meta attributes of that resource to `false`.

#### Blog post

You can read the introductory blog post about this topic here:
[https://purpleidea.com/blog/2016/03/14/automatic-edges-in-mgmt/](https://purpleidea.com/blog/2016/03/14/automatic-edges-in-mgmt/)

### Autogrouping

Automatic grouping or AutoGroup is the mechanism in mgmt by which it will
automatically group multiple resource vertices into a single one. This is
particularly useful for grouping multiple package resources into a single
resource, since the multiple installations can happen together in a single
transaction, which saves a lot of time because package resources typically have
a large fixed cost to running (downloading and verifying the package repo) and
if they are grouped they share this fixed cost. This grouping feature can be
used for other use cases too.

You can disable autogrouping for a resource by setting the `autogroup` key on
the meta attributes of that resource to `false`.

#### Blog post

You can read the introductory blog post about this topic here:
[https://purpleidea.com/blog/2016/03/30/automatic-grouping-in-mgmt/](https://purpleidea.com/blog/2016/03/30/automatic-grouping-in-mgmt/)

### Automatic clustering

Automatic clustering is a feature by which mgmt automatically builds, scales,
and manages the embedded etcd cluster which is compiled into mgmt itself. It is
quite helpful for rapidly bootstrapping clusters and avoiding the extra work to
setup etcd.

If you prefer to avoid this feature. you can always opt to use an existing etcd
cluster that is managed separately from mgmt by pointing your mgmt agents at it
with the `--seeds` variable.

#### Blog post

You can read the introductory blog post about this topic here:
[https://purpleidea.com/blog/2016/06/20/automatic-clustering-in-mgmt/](https://purpleidea.com/blog/2016/06/20/automatic-clustering-in-mgmt/)

### Remote ("agent-less") mode

Remote mode is a special mode that lets you kick off mgmt runs on one or more
remote machines which are only accessible via SSH. In this mode the initiating
host connects over SSH, copies over the `mgmt` binary, opens an SSH tunnel, and
runs the remote program while simultaneously passing the etcd traffic back
through the tunnel so that the initiators etcd cluster can be used to exchange
resource data.

The interesting benefit of this architecture is that multiple hosts which can't
connect directly use the initiator to pass the important traffic through to each
other. Once the cluster has converged all the remote programs can shutdown
leaving no residual agent.

This mode can also be useful for bootstrapping a new host where you'd like to
have the service run continuously and as part of an mgmt cluster normally.

In particular, when combined with the `--converged-timeout` parameter, the
entire set of running mgmt agents will need to all simultaneously converge for
the group to exit. This is particularly useful for bootstrapping new clusters
which need to exchange information that is only available at run time.

#### Blog post

You can read the introductory blog post about this topic here:
[https://purpleidea.com/blog/2016/10/07/remote-execution-in-mgmt/](https://purpleidea.com/blog/2016/10/07/remote-execution-in-mgmt/)

### Puppet support

You can supply a Puppet manifest instead of creating the (YAML) graph manually.
Puppet must be installed and in `mgmt`'s search path. You also need the
[ffrank-mgmtgraph Puppet module](https://forge.puppet.com/ffrank/mgmtgraph).

Invoke `mgmt` with the `--puppet` switch, which supports 3 variants:

1. Request the configuration from the Puppet Master (like `puppet agent` does)

	`mgmt run --puppet agent`

2. Compile a local manifest file (like `puppet apply`)

	`mgmt run --puppet /path/to/my/manifest.pp`

3. Compile an ad hoc manifest from the commandline (like `puppet apply -e`)

	`mgmt run --puppet 'file { "/etc/ntp.conf": ensure => file }'`

For more details and caveats see [Puppet.md](Puppet.md).

#### Blog post

An introductory post on the Puppet support is on
[Felix's blog](http://ffrank.github.io/features/2016/06/19/puppet-powered-mgmt/).

## Reference

Please note that there are a number of undocumented options. For more
information on these options, please view the source at:
[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/).
If you feel that a well used option needs documenting here, please patch it!

### Overview of reference

* [Meta parameters](#meta-parameters): List of available resource meta parameters.
* [Graph definition file](#graph-definition-file): Main graph definition file.
* [Command line](#command-line): Command line parameters.
* [Compilation options](#compilation-options): Compilation options.

### Meta parameters

These meta parameters are special parameters (or properties) which can apply to
any resource. The usefulness of doing so will depend on the particular meta
parameter and resource combination.

#### AutoEdge

Boolean. Should we generate auto edges for this resource?

#### AutoGroup

Boolean. Should we attempt to automatically group this resource with others?

#### Noop

Boolean. Should the Apply portion of the CheckApply method of the resource
make any changes? Noop is a concatenation of no-operation.

#### Retry

Integer. The number of times to retry running the resource on error. Use -1 for
infinite. This currently applies for both the Watch operation (which can fail)
and for the CheckApply operation. While they could have separate values, I've
decided to use the same ones for both until there's a proper reason to want to
do something differently for the Watch errors.

#### Delay

Integer. Number of milliseconds to wait between retries. The same value is
shared between the Watch and CheckApply retries. This currently applies for both
the Watch operation (which can fail) and for the CheckApply operation. While
they could have separate values, I've decided to use the same ones for both
until there's a proper reason to want to do something differently for the Watch
errors.

#### Poll

Integer. Number of seconds to wait between `CheckApply` checks. If this is
greater than zero, then the standard event based `Watch` mechanism for this
resource is replaced with a simple polling mechanism. In general, this is not
recommended, unless you have a very good reason for doing so.

Please keep in mind that if you have a resource which changes every `I` seconds,
and you poll it every `J` seconds, and you've asked for a converged timeout of
`K` seconds, and `I <= J <= K`, then your graph will likely never converge.

When polling, the system detects that a resource is not converged if its
`CheckApply` method returns false. This allows a resource which changes every
`I` seconds, and which is polled every `J` seconds, and with a converged timeout
of `K` seconds to still converge when `J <= K`, as long as `I > J || I > K`,
which is another way of saying that if the resource finally settles down to give
the graph enough time, it can probably converge.

#### Limit

Float. Maximum rate of `CheckApply` runs started per second. Useful to limit
an especially _eventful_ process from causing excessive checks to run. This
defaults to `+Infinity` which adds no limiting. If you change this value, you
will also need to change the `Burst` value to a non-zero value. Please see the
[rate](https://godoc.org/golang.org/x/time/rate) package for more information.

#### Burst

Integer. Burst is the maximum number of runs which can happen without invoking
the rate limiter as designated by the `Limit` value. If the `Limit` is not set
to `+Infinity`, this must be a non-zero value. Please see the
[rate](https://godoc.org/golang.org/x/time/rate) package for more information.

#### Sema

List of string ids. Sema is a P/V style counting semaphore which can be used to
limit parallelism during the CheckApply phase of resource execution. Each
resource can have `N` different semaphores which share a graph global namespace.
Each semaphore has a maximum count associated with it. The default value of the
size is 1 (one) if size is unspecified. Each string id is the unique id of the
semaphore. If the id contains a trailing colon (:) followed by a positive
integer, then that value is the max size for that semaphore. Valid semaphore
id's include: `some_id`, `hello:42`, `not:smart:4` and `:13`. It is expected
that the last bare example be only used by the engine to add a global semaphore.

### Graph definition file

graph.yaml is the compiled graph definition file. The format is currently
undocumented, but by looking through the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples)
you can probably figure out most of it, as it's fairly intuitive.

### Command line

The main interface to the `mgmt` tool is the command line. For the most recent
documentation, please run `mgmt --help`.

#### `--yaml <graph.yaml>`

Point to a graph file to run.

#### `--converged-timeout <seconds>`

Exit if the machine has converged for approximately this many seconds.

#### `--max-runtime <seconds>`

Exit when the agent has run for approximately this many seconds. This is not
generally recommended, but may be useful for users who know what they're doing.

#### `--noop`

Globally force all resources into no-op mode. This also disables the export to
etcd functionality, but does not disable resource collection, however all
resources that are collected will have their individual noop settings set.

#### `--sema <size>`

Globally add a counting semaphore of this size to each resource in the graph.
The semaphore will get given an id of `:size`. In other words if you specify a
size of 42, you can expect a semaphore if named: `:42`. It is expected that
consumers of the semaphore metaparameter always include a prefix to avoid a
collision with this globally defined semaphore. The size value must be greater
than zero at this time. The traditional non-parallel execution found in config
management tools such as `Puppet` can be obtained with `--sema 1`.

#### `--remote <graph.yaml>`

Point to a graph file to run on the remote host specified within. This parameter
can be used multiple times if you'd like to remotely run on multiple hosts in
parallel.

#### `--allow-interactive`

Allow interactive prompting for SSH passwords if there is no authentication
method that works.

#### `--ssh-priv-id-rsa`

Specify the path for finding SSH keys. This defaults to `~/.ssh/id_rsa`. To
never use this method of authentication, set this to the empty string.

#### `--cconns`

The maximum number of concurrent remote ssh connections to run. This defaults
to `0`, which means unlimited.

#### `--no-caching`

Don't allow remote caching of the remote execution binary. This will require
the binary to be copied over for every remote execution, but it limits the
likelihood that there is leftover information from the configuration process.

#### `--prefix <path>`

Specify a path to a custom working directory prefix. This directory will get
created if it does not exist. This usually defaults to `/var/lib/mgmt/`. This
can't be combined with the `--tmp-prefix` option. It can be combined with the
`--allow-tmp-prefix` option.

#### `--tmp-prefix`

If this option is specified, a temporary prefix will be used instead of the
default prefix. This can't be combined with the `--prefix` option.

#### `--allow-tmp-prefix`

If this option is specified, we will attempt to fall back to a temporary prefix
if the primary prefix couldn't be created. This is useful for avoiding failures
in environments where the primary prefix may or may not be available, but you'd
like to try. The canonical example is when running `mgmt` with `--remote` there
might be a cached copy of the binary in the primary prefix, but in case there's
no binary available continue working in a temporary directory to avoid failure.

### Compilation options

You can control some compilation variables by using environment variables.

#### Disable libvirt support

If you wish to compile mgmt without libvirt, you can use the following command:

```
GOTAGS=novirt make build
```

#### Disable augeas support

If you wish to compile mgmt without augeas support, you can use the following command:

```
GOTAGS=noaugeas make build
```

#### Combining compile-time flags

You can combine multiple tags by using a space-separated list:

```
GOTAGS="noaugeas novirt" make build
```

## Examples

For example configurations, please consult the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples) directory in the git
source repository. It is available from:

[https://github.com/purpleidea/mgmt/tree/master/examples](https://github.com/purpleidea/mgmt/tree/master/examples)

### Systemd:

See [`misc/mgmt.service`](misc/mgmt.service) for a sample systemd unit file.
This unit file is part of the RPM.

To specify your custom options for `mgmt` on a systemd distro:

```bash
sudo mkdir -p /etc/systemd/system/mgmt.service.d/

cat > /etc/systemd/system/mgmt.service.d/env.conf <<EOF
# Environment variables:
MGMT_SEEDS=http://127.0.0.1:2379
MGMT_CONVERGED_TIMEOUT=-1
MGMT_MAX_RUNTIME=0

# Other CLI options if necessary.
#OPTS="--max-runtime=0"
EOF

sudo systemctl daemon-reload
```

## Development

This is a project that I started in my free time in 2013. Development is driven
by all of our collective patches! Dive right in, and start hacking!
Please contact me if you'd like to invite me to speak about this at your event.

You can follow along [on my technical blog](https://purpleidea.com/blog/).

To report any bugs, please file a ticket at: [https://github.com/purpleidea/mgmt/issues](https://github.com/purpleidea/mgmt/issues).

## Authors

Copyright (C) 2013-2018+ James Shubin and the project contributors

Please see the
[AUTHORS](https://github.com/purpleidea/mgmt/tree/master/AUTHORS) file
for more information.

* [github](https://github.com/purpleidea/)
* [&#64;purpleidea](https://twitter.com/#!/purpleidea)
* [https://purpleidea.com/](https://purpleidea.com/)
