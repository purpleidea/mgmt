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
5. [Usage/FAQ - Notes on usage and frequently asked questions](#usage-and-frequently-asked-questions)
6. [Reference - Detailed reference](#reference)
	* [Graph definition file](#graph-definition-file)
	* [Command line](#command-line)
7. [Examples - Example configurations](#examples)
8. [Development - Background on module development and reporting bugs](#development)
9. [Authors - Authors and contact information](#authors)

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
You'll probably want to use ```./run.sh run --file examples/graph1.yaml``` to
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

####Blog post

An introductory blog post about this topic will follow soon.

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
* [Graph definition file](#graph-definition-file): Main graph definition file.
* [Command line](#command-line): Command line parameters.

###Graph definition file
graph.yaml is the compiled graph definition file. The format is currently
undocumented, but by looking through the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples)
you can probably figure out most of it, as it's fairly intuitive.

###Command line
The main interface to the `mgmt` tool is the command line. For the most recent
documentation, please run `mgmt --help`.

####`--file <graph.yaml>`
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
