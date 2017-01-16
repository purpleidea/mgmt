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

##mgmt Puppet guide
####Available from:
####[https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/)

####This documentation is available in: [Markdown](https://github.com/purpleidea/mgmt/blob/master/docs/puppet-guide.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/docs/puppet-guide.md) format.

####Table of Contents

1. [Prerequisites](#prerequisites)
	* [Testing the Puppet side](#testing-the-puppet-side)
2. [Writing a suitable manifest](#writing-a-suitable-manifest)
	* [Unsupported attributes](#unsupported-attributes)
	* [Unsupported resources](#unsupported-resources)
	* [Avoiding common warnings](#avoiding-common-warnings)
3. [Configuring Puppet](#configuring-puppet)
4. [Caveats](#caveats)

`mgmt` can use Puppet as its source for the configuration graph.
This document goes into detail on how this works, and lists
some pitfalls and limitations.

For basic instructions on how to use the Puppet support, see
the [main documentation](documentation.md#puppet-support).

##Prerequisites

You need Puppet installed in your system. It is not important how you
get it. On the most common Linux distributions, you can use packages
from the OS maintainer, or upstream Puppet repositories. An alternative
that will also work on OSX is the `puppet` Ruby gem. It also has the
advantage that you can install any desired version in your home directory
or any other location.

Any release of Puppet's 3.x and 4.x series should be suitable for use with
`mgmt`. Most importantly, make sure to install the `ffrank-mgmtgraph` Puppet
module (referred to below as "the translator module").

```
puppet module install ffrank-mgmtgraph
```

Please note that the module is not required on your Puppet master (if you
use a master/agent setup). It's needed on the machine that runs `mgmt`.
You can install the module on the master anyway, so that it gets distributed
to your agents through Puppet's `pluginsync` mechanism.

###Testing the Puppet side

The following command should run successfully and print a YAML hash on your
terminal:

```puppet
puppet mgmtgraph print --code 'file { "/tmp/mgmt-test": ensure => present }'
```

You can use this CLI to test any manifests before handing them straight
to `mgmt`.

##Writing a suitable manifest

###Unsupported attributes

`mgmt` inherited its resource module from Puppet, so by and large, it's quite
possible to express `mgmt` graphs in terms of Puppet manifests. However,
there isn't (and likely never will be) full feature parity between the
respective resource types. In consequence, a manifest can have semantics that
cannot be transferred to `mgmt`.

For example, at the time of writing this, the `file` type in `mgmt` had no
notion of permissions (the file `mode`) yet. This lead to the following
warning (among others that will be discussed below):

```
$ puppet mgmtgraph print --code 'file { "/tmp/foo": mode => "0600" }'
Warning: cannot translate: File[/tmp/foo] { mode => "600" } (attribute is ignored)
```

This is a heads-up for the user, because the resulting `mgmt` graph will
in fact not pass this information to the `/tmp/foo` file resource, and
`mgmt` will ignore this file's permissions. Including such attributes in
manifests that are written expressly for `mgmt` is not sensible and should
be avoided.

###Unsupported resources

Puppet has a fairly large number of
[built-in types](https://docs.puppet.com/puppet/latest/reference/type.html),
and countless more are available through
[modules](https://forge.puppet.com/). It's unlikely that all of them will
eventually receive native counterparts in `mgmt`.

When encountering an unknown resource, the translator module will replace
it with an `exec` resource in its output. This resource will run the equivalent
of a `puppet resource` command to make Puppet apply the original resource
itself. This has quite abysmal performance, because processing such a
resource requires the forking of at least one Puppet process (two if it
is found to be out of sync). This comes with considerable overhead.
On most systems, starting up any Puppet command takes several seconds.
Compared to the split second that the actual work usually takes,
this overhead can amount to several orders of magnitude.

Avoid Puppet types that `mgmt` does not implement (yet).

###Avoiding common warnings

Many resource parameters in Puppet take default values. For the most part,
the translator module just ignores them. However, there are cases in which
Puppet will default to convenient behavior that `mgmt` cannot quite replicate.
For example, translating a plain `file` resource will lead to a warning message:

```
$ puppet mgmtgraph print --code 'file { "/tmp/mgmt-test": }'
Warning: File[/tmp/mgmt-test] uses the 'puppet' file bucket, which mgmt cannot do. There will be no backup copies!
```

The reason is that per default, Puppet assumes the following parameter value
(among others)

```puppet
file { "/tmp/mgmt-test":
	backup => 'puppet',
}
```

To avoid this, specify the parameter explicitly:

```
$ puppet mgmtgraph print --code 'file { "/tmp/mgmt-test": backup => false }'
```

This is tedious in a more complex manifest. A good simplification is the
following [resource default](https://docs.puppet.com/puppet/latest/reference/lang_defaults.html)
anywhere on the top scope of your manifest:

```puppet
File { backup => false }
```

If you encounter similar warnings from other types and/or parameters,
use the same approach to silence them if possible.

##Configuring Puppet

Since `mgmt` uses an actual Puppet CLI behind the scenes, you might
need to tweak some of Puppet's runtime options in order to make it
do what you want. Reasons for this could be among the following:

* You use the `--puppet agent` variant and need to configure
`servername`, `certname` and other master/agent-related options.
* You don't want runtime information to end up in the `vardir`
that is used by your regular `puppet agent`.
* You install specific Puppet modules for `mgmt` in a non-standard
location.

`mgmt` exposes only one Puppet option in order to allow you to
control all of them, through its `--puppet-conf` option. It allows
you to specify which `puppet.conf` file should be used during
translation.

```
mgmt run --puppet /opt/my-manifest.pp --puppet-conf /etc/mgmt/puppet.conf
```

Within this file, you can just specify any needed options in the
`[main]` section:

```
[main]
server=mgmt-master.example.net
vardir=/var/lib/mgmt/puppet
```

##Caveats

Please see the [README](https://github.com/ffrank/puppet-mgmtgraph/blob/master/README.md)
of the translator module for the current state of supported and unsupported
language features.

You should probably make sure to always use the latest release of
both `ffrank-mgmtgraph` and `ffrank-yamlresource` (the latter is
getting pulled in as a dependency of the former).
