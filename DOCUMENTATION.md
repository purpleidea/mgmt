#mgmt

<!--
Mgmt
Copyright (C) 2013-2015+ James Shubin and the project contributors
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
4. [Usage/FAQ - Notes on usage and frequently asked questions](#usage-and-frequently-asked-questions)
5. [Reference - Detailed reference](#reference)
	* [graph.yaml](#graph.yaml)
	* [Command line](#command-line)
6. [Examples - Example configurations](#examples)
7. [Development - Background on module development and reporting bugs](#development)
8. [Authors - Authors and contact information](#authors)

##Overview

The `mgmt` tool is a research prototype to demonstrate next generation config
management techniques. Hopefully it will evolve into a useful, robust tool.

##Project Description

The mgmt tool is a distributed, event driven, config management tool, that
supports parallel execution, and librarification to be used as the management
foundation in and for, new and existing software.

##Setup

During this prototype phase, the tool can be run out of the source directory.
You'll probably want to use ```./run.sh run --file examples/graph1.yaml``` to
get started. Beware that this _can_ cause data loss. Understand what you're
doing first, or perform these actions in a virtual environment such as the one
provided by [Oh-My-Vagrant](https://github.com/purpleidea/oh-my-vagrant).

##Usage and frequently asked questions
(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

###Why did you start this project?

I wanted a next generation config management solution that didn't have all of
the design flaws or limitations that the current generation of tools do, and no
tool existed!

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
* [graph.yaml](#graph.yaml): Main graph definition file.
* [Command line](#command-line): Command line parameters.

###graph.yaml
This is the compiled graph definition file. The format is currently
undocumented, but by looking through the [examples/](https://github.com/purpleidea/mgmt/tree/master/examples)
you can probably figure out most of it, as it's fairly intuitive.

###Command line
The main interface to the `mgmt` tool is the command line. For the most recent
documentation, please run `mgmt --help`.

####`--file <graph.yaml>`
Point to a graph file to run.

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

Copyright (C) 2013-2015+ James Shubin and the project contributors

Please see the
[AUTHORS](https://github.com/purpleidea/mgmt/tree/master/AUTHORS) file
for more information.

* [github](https://github.com/purpleidea/)
* [&#64;purpleidea](https://twitter.com/#!/purpleidea)
* [https://ttboj.wordpress.com/](https://ttboj.wordpress.com/)
