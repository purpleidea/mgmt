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
5. [Usage/FAQ - Notes on usage and frequently asked questions](#usage-and-frequently-asked-questions)
6. [Reference - Detailed reference](#reference)
	* [Graph definition file](#graph-definition-file)
	* [Command line](#command-line)
7. [Examples - Example configurations](#examples)
8. [Development - Background on module development and reporting bugs](#development)
9. [Authors - Authors and contact information](#authors)

##Overview

The `mgmt` tool is a research prototype to demonstrate next generation config
management techniques. Hopefully it will evolve into a useful, robust tool.

##Project Description

The mgmt tool is a distributed, event driven, config management tool, that
supports parallel execution, and librarification to be used as the management
foundation in and for, new and existing software.

For more information, you may like to read some blog posts from the author:

* [Next generation config mgmt](https://ttboj.wordpress.com/2016/01/18/next-generation-configuration-mgmt/)
* [Automatic edges in mgmt](https://ttboj.wordpress.com/2016/03/14/automatic-edges-in-mgmt/)

There is also an [introductory video](https://www.youtube.com/watch?v=GVhpPF0j-iE&html5=1) available.

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

####Controlling autodeges

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

##Examples
For example configurations, please consult the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples) directory in the git
source repository. It is available from:

[https://github.com/purpleidea/mgmt/tree/master/examples](https://github.com/purpleidea/mgmt/tree/master/examples)

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
