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

##mgmt by [James](https://ttboj.wordpress.com/)
####Available from:
####[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/)

####This documentation is available in: [Markdown](https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md) format.

####Table of Contents

1. [Overview](#overview)
2. [Project description - What the project does](#project-description)
3. [Setup - Getting started with mgmt](#setup)
4. [Features - All things mgmt can do](#features)
	* [Autoedges - Automatic resource relationships](#autoedges)
	* [Autogrouping - Automatic resource grouping](#autogrouping)
	* [Automatic clustering - Automatic cluster management](#automatic-clustering)
	* [Remote mode - Remote "agent-less" execution](#remote-agent-less-mode)
	* [Puppet support - write manifest code for mgmt](#puppet-support)
5. [Resources - All built-in primitives](#resources)
6. [Usage/FAQ - Notes on usage and frequently asked questions](#usage-and-frequently-asked-questions)
7. [Reference - Detailed reference](#reference)
	* [Meta parameters](#meta-parameters)
	* [Graph definition file](#graph-definition-file)
	* [Command line](#command-line)
8. [Examples - Example configurations](#examples)
9. [Development - Background on module development and reporting bugs](#development)
10. [Authors - Authors and contact information](#authors)

##Overview

The `mgmt` tool is a next generation config management prototype. It's not yet
ready for production, but we hope to get there soon. Get involved today!

##Project Description

The mgmt tool is a distributed, event driven, config management tool, that
supports parallel execution, and librarification to be used as the management
foundation in and for, new and existing software.

For more information, you may like to read some blog posts from the author:

* [Next generation config mgmt](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
* [Automatic edges in mgmt](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/)
* [Automatic grouping in mgmt](https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/)
* [Automatic clustering in mgmt](https://ttboj.wordpress.com/2016/06/20/automatic-clustering-in-mgmt/)

There is also an [introductory video](http://meetings-archive.debian.net/pub/debian-meetings/2016/debconf16/Next_Generation_Config_Mgmt.webm) available.
Older videos and other material [is available](https://github.com/purpleidea/mgmt/#on-the-web).

##Setup

During this prototype phase, the tool can be run out of the source directory.
You'll probably want to use ```./run.sh run --yaml examples/graph1.yaml``` to
get started. Beware that this _can_ cause data loss. Understand what you're
doing first, or perform these actions in a virtual environment such as the one
provided by [Oh-My-Vagrant](https://github.com/purpleidea/oh-my-vagrant).

##Features

This section details the numerous features of mgmt and some caveats you might
need to be aware of.

###Autoedges

Automatic edges, or AutoEdges, is the mechanism in mgmt by which it will
automatically create dependencies for you between resources. For example,
since mgmt can discover which files are installed by a package it will
automatically ensure that any file resource you declare that matches a
file installed by your package resource will only be processed after the
package is installed.

####Controlling autoedges

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

####Blog post

You can read the introductory blog post about this topic here:
[https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/)

###Autogrouping

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

####Blog post

You can read the introductory blog post about this topic here:
[https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/](https://ttboj.wordpress.com/2016/03/30/automatic-grouping-in-mgmt/)

###Automatic clustering

Automatic clustering is a feature by which mgmt automatically builds, scales,
and manages the embedded etcd cluster which is compiled into mgmt itself. It is
quite helpful for rapidly bootstrapping clusters and avoiding the extra work to
setup etcd.

If you prefer to avoid this feature. you can always opt to use an existing etcd
cluster that is managed separately from mgmt by pointing your mgmt agents at it
with the `--seeds` variable.

####Blog post

You can read the introductory blog post about this topic here:
[https://ttboj.wordpress.com/2016/06/20/automatic-clustering-in-mgmt/](https://ttboj.wordpress.com/2016/06/20/automatic-clustering-in-mgmt/)

###Remote ("agent-less") mode

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

####Blog post

You can read the introductory blog post about this topic here:
[https://ttboj.wordpress.com/2016/10/07/remote-execution-in-mgmt/](https://ttboj.wordpress.com/2016/10/07/remote-execution-in-mgmt/)

###Puppet support

You can supply a Puppet manifest instead of creating the (YAML) graph manually.
Puppet must be installed and in `mgmt`'s search path. You also need the
[ffrank-mgmtgraph Puppet module](https://forge.puppet.com/ffrank/mgmtgraph).

Invoke `mgmt` with the `--puppet` switch, which supports 3 variants:

1. Request the configuration from the Puppet Master (like `puppet agent` does)

        mgmt run --puppet agent

2. Compile a local manifest file (like `puppet apply`)

        mgmt run --puppet /path/to/my/manifest.pp

3. Compile an ad hoc manifest from the commandline (like `puppet apply -e`)

        mgmt run --puppet 'file { "/etc/ntp.conf": ensure => file }'

For more details and caveats see [Puppet.md](Puppet.md).

####Blog post

An introductory post on the Puppet support is on
[Felix's blog](http://ffrank.github.io/features/2016/06/19/puppet-powered-mgmt/).

##Resources

This section lists all the built-in resources and their properties. The
resource primitives in `mgmt` are typically more powerful than resources in
other configuration management systems because they can be event based which
lets them respond in real-time to converge to the desired state. This property
allows you to build more complex resources that you probably hadn't considered
in the past.

In addition to the resource specific properties, there are resource properties
(otherwise known as parameters) which can apply to every resource. These are
called [meta parameters](#meta-parameters) and are listed separately. Certain
meta parameters aren't very useful when combined with certain resources, but
in general, it should be fairly obvious, such as when combining the `noop` meta
parameter with the [Noop](#Noop) resource.

* [Exec](#Exec): Execute shell commands on the system.
* [File](#File): Manage files and directories.
* [Hostname](#Hostname): Manages the hostname on the system.
* [Msg](#Msg): Send log messages.
* [Noop](#Noop): A simple resource that does nothing.
* [Pkg](#Pkg):  Manage system packages with PackageKit.
* [Svc](#Svc): Manage system systemd services.
* [Timer](#Timer): Manage system systemd services.
* [Virt](#Virt): Manage virtual machines with libvirt.

###Exec

The exec resource can execute commands on your system.

###File

The file resource manages files and directories. In `mgmt`, directories are
identified by a trailing slash in their path name. File have no such slash.

####Path

The path property specifies the file or directory that we are managing.

####Content

The content property is a string that specifies the desired file contents.

####Source

The source property points to a source file or directory path that we wish to
copy over and use as the desired contents for our resource.

####State

The state property describes the action we'd like to apply for the resource. The
possible values are: `exists` and `absent`.

####Recurse

The recurse property limits whether file resource operations should recurse into
and monitor directory contents with a depth greater than one.

####Force

The force property is required if we want the file resource to be able to change
a file into a directory or vice-versa. If such a change is needed, but the force
property is not set to `true`, then this file resource will error.

###Hostname

The hostname resource manages static, transient/dynamic and pretty hostnames
on the system and watches them for changes.

#### static_hostname
The static hostname is the one configured in /etc/hostname or a similar
file.
It is chosen by the local user. It is not always in sync with the current
host name as returned by the gethostname() system call.

#### transient_hostname
The transient / dynamic hostname is the one configured via the kernel's
sethostbyname().
It can be different from the static hostname in case DHCP or mDNS have been
configured to change the name based on network information.

#### pretty_hostname
The pretty hostname is a free-form UTF8 host name for presentation to the user.

#### hostname
Hostname is the fallback value for all 3 fields above, if only `hostname` is
specified, it will set all 3 fields to this value.

###Msg

The msg resource sends messages to the main log, or an external service such
as systemd's journal.

###Noop

The noop resource does absolutely nothing. It does have some utility in testing
`mgmt` and also as a placeholder in the resource graph.

###Pkg

The pkg resource is used to manage system packages. This resource works on many
different distributions because it uses the underlying packagekit facility which
supports different backends for different environments. This ensures that we
have great Debian (deb/dpkg) and Fedora (rpm/dnf) support simultaneously.

###Svc

The service resource is still very WIP. Please help us my improving it!

###Timer

This resource needs better documentation. Please help us my improving it!

###Virt

The virt resource can manage virtual machines via libvirt.

##Usage and frequently asked questions
(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

###Why did you start this project?

I wanted a next generation config management solution that didn't have all of
the design flaws or limitations that the current generation of tools do, and no
tool existed!

###Why did you use etcd? What about consul?

Etcd and consul are both written in golang, which made them the top two
contenders for my prototype. Ultimately a choice had to be made, and etcd was
chosen, but it was also somewhat arbitrary. If there is available interest,
good reasoning, *and* patches, then we would consider either switching or
supporting both, but this is not a high priority at this time.

###Can I use an existing etcd cluster instead of the automatic embedded servers?

Yes, it's possible to use an existing etcd cluster instead of the automatic,
elastic embedded etcd servers. To do so, simply point to the cluster with the
`--seeds` variable, the same way you would if you were seeding a new member to
an existing mgmt cluster.

The downside to this approach is that you won't benefit from the automatic
elastic nature of the embedded etcd servers, and that you're responsible if you
accidentally break your etcd cluster, or if you use an unsupported version.

###What does the error message about an inconsistent dataDir mean?

If you get an error message similar to:

```
Etcd: Connect: CtxError...
Etcd: CtxError: Reason: CtxDelayErr(5s): No endpoints available yet!
Etcd: Connect: Endpoints: []
Etcd: The dataDir (/var/lib/mgmt/etcd) might be inconsistent or corrupt.
```

This happens when there are a series of fatal connect errors in a row. This can
happen when you start `mgmt` using a dataDir that doesn't correspond to the
current cluster view. As a result, the embedded etcd server never finishes
starting up, and as a result, a default endpoint never gets added. The solution
is to either reconcile the mistake, and if there is no important data saved, you
can remove the etcd dataDir. This is typically `/var/lib/mgmt/etcd/member/`.

###Why do resources have both a `Compare` method and an `IFF` (on the UID) method?

The `Compare()` methods are for determining if two resources are effectively the
same, which is used to make graph change delta's efficient. This is when we want
to change from the current running graph to a new graph, but preserve the common
vertices. Since we want to make this process efficient, we only update the parts
that are different, and leave everything else alone. This `Compare()` method can
tell us if two resources are the same.

The `IFF()` method is part of the whole UID system, which is for discerning if a
resource meets the requirements another expects for an automatic edge. This is
because the automatic edge system assumes a unified UID pattern to test for
equality. In the future it might be helpful or sane to merge the two similar
comparison functions although for now they are separate because they are
actually answer different questions.

###Did you know that there is a band named `MGMT`?

I didn't realize this when naming the project, and it is accidental. After much
anguishing, I chose the name because it was short and I thought it was
appropriately descriptive. If you need a less ambiguous search term or phrase,
you can try using `mgmtconfig` or `mgmt config`.

###You didn't answer my question, or I have a question!

It's best to ask on [IRC](https://webchat.freenode.net/?channels=#mgmtconfig)
to see if someone can help you. Once we get a big enough community going, we'll
add a mailing list. If you don't get any response from the above, you can
contact me through my [technical blog](https://ttboj.wordpress.com/contact/)
and I'll do my best to help. If you have a good question, please add it as a
patch to this documentation. I'll merge your question, and add a patch with the
answer!

##Reference
Please note that there are a number of undocumented options. For more
information on these options, please view the source at:
[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/).
If you feel that a well used option needs documenting here, please patch it!

###Overview of reference
* [Meta parameters](#meta-parameters): List of available resource meta parameters.
* [Graph definition file](#graph-definition-file): Main graph definition file.
* [Command line](#command-line): Command line parameters.

###Meta parameters
These meta parameters are special parameters (or properties) which can apply to
any resource. The usefulness of doing so will depend on the particular meta
parameter and resource combination.

####AutoEdge
Boolean. Should we generate auto edges for this resource?

####AutoGroup
Boolean. Should we attempt to automatically group this resource with others?

####Noop
Boolean. Should the Apply portion of the CheckApply method of the resource
make any changes? Noop is a concatenation of no-operation.

####Retry
Integer. The number of times to retry running the resource on error. Use -1 for
infinite. This currently applies for both the Watch operation (which can fail)
and for the CheckApply operation. While they could have separate values, I've
decided to use the same ones for both until there's a proper reason to want to
do something differently for the Watch errors.

####Delay
Integer. Number of milliseconds to wait between retries. The same value is
shared between the Watch and CheckApply retries. This currently applies for both
the Watch operation (which can fail) and for the CheckApply operation. While
they could have separate values, I've decided to use the same ones for both
until there's a proper reason to want to do something differently for the Watch
errors.

###Graph definition file
graph.yaml is the compiled graph definition file. The format is currently
undocumented, but by looking through the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples)
you can probably figure out most of it, as it's fairly intuitive.

###Command line
The main interface to the `mgmt` tool is the command line. For the most recent
documentation, please run `mgmt --help`.

####`--yaml <graph.yaml>`
Point to a graph file to run.

####`--converged-timeout <seconds>`
Exit if the machine has converged for approximately this many seconds.

####`--max-runtime <seconds>`
Exit when the agent has run for approximately this many seconds. This is not
generally recommended, but may be useful for users who know what they're doing.

####`--noop`
Globally force all resources into no-op mode. This also disables the export to
etcd functionality, but does not disable resource collection, however all
resources that are collected will have their individual noop settings set.

####`--remote <graph.yaml>`
Point to a graph file to run on the remote host specified within. This parameter
can be used multiple times if you'd like to remotely run on multiple hosts in
parallel.

####`--allow-interactive`
Allow interactive prompting for SSH passwords if there is no authentication
method that works.

####`--ssh-priv-id-rsa`
Specify the path for finding SSH keys. This defaults to `~/.ssh/id_rsa`. To
never use this method of authentication, set this to the empty string.

####`--cconns`
The maximum number of concurrent remote ssh connections to run. This defaults
to `0`, which means unlimited.

####`--no-caching`
Don't allow remote caching of the remote execution binary. This will require
the binary to be copied over for every remote execution, but it limits the
likelihood that there is leftover information from the configuration process.

####`--prefix <path>`
Specify a path to a custom working directory prefix. This directory will get
created if it does not exist. This usually defaults to `/var/lib/mgmt/`. This
can't be combined with the `--tmp-prefix` option. It can be combined with the
`--allow-tmp-prefix` option.

####`--tmp-prefix`
If this option is specified, a temporary prefix will be used instead of the
default prefix. This can't be combined with the `--prefix` option.

####`--allow-tmp-prefix`
If this option is specified, we will attempt to fall back to a temporary prefix
if the primary prefix couldn't be created. This is useful for avoiding failures
in environments where the primary prefix may or may not be available, but you'd
like to try. The canonical example is when running `mgmt` with `--remote` there
might be a cached copy of the binary in the primary prefix, but in case there's
no binary available continue working in a temporary directory to avoid failure.

##Examples
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

##Development

This is a project that I started in my free time in 2013. Development is driven
by all of our collective patches! Dive right in, and start hacking!
Please contact me if you'd like to invite me to speak about this at your event.

You can follow along [on my technical blog](https://ttboj.wordpress.com/).

To report any bugs, please file a ticket at: [https://github.com/purpleidea/mgmt/issues](https://github.com/purpleidea/mgmt/issues).

##Authors

Copyright (C) 2013-2016+ James Shubin and the project contributors

Please see the
[AUTHORS](https://github.com/purpleidea/mgmt/tree/master/AUTHORS) file
for more information.

* [github](https://github.com/purpleidea/)
* [&#64;purpleidea](https://twitter.com/#!/purpleidea)
* [https://ttboj.wordpress.com/](https://ttboj.wordpress.com/)
