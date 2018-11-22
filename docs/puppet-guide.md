# Puppet guide

`mgmt` can use Puppet as its source for the configuration graph.
This document goes into detail on how this works, and lists
some pitfalls and limitations.

For basic instructions on how to use the Puppet support, see
the [main documentation](documentation.md#puppet-support).

## Prerequisites

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

### Testing the Puppet side

The following command should run successfully and print a YAML hash on your
terminal:

```puppet
puppet mgmtgraph print --code 'file { "/tmp/mgmt-test": ensure => present }'
```

You can use this CLI to test any manifests before handing them straight
to `mgmt`.

## Writing a suitable manifest

### Unsupported attributes

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

### Unsupported resources

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

### Avoiding common warnings

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

```bash
puppet mgmtgraph print --code 'file { "/tmp/mgmt-test": backup => false }'
```

This is tedious in a more complex manifest. A good simplification is the
following [resource default](https://docs.puppet.com/puppet/latest/reference/lang_defaults.html)
anywhere on the top scope of your manifest:

```puppet
File { backup => false }
```

If you encounter similar warnings from other types and/or parameters,
use the same approach to silence them if possible.

## Configuring Puppet

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
mgmt run puppet --puppet /opt/my-manifest.pp --puppet-conf /etc/mgmt/puppet.conf
```

Within this file, you can just specify any needed options in the
`[main]` section:

```
[main]
server=mgmt-master.example.net
vardir=/var/lib/mgmt/puppet
```

## Caveats

Please see the [README](https://github.com/ffrank/puppet-mgmtgraph/blob/master/README.md)
of the translator module for the current state of supported and unsupported
language features.

You should probably make sure to always use the latest release of
both `ffrank-mgmtgraph` and `ffrank-yamlresource` (the latter is
getting pulled in as a dependency of the former).

## Using Puppet in conjunction with the mcl lang

The graph that Puppet generates for `mgmt` can be united with a graph
that is created from native `mgmt` code in its mcl language. This is
useful when you are in the process of replacing Puppet with mgmt. You
can translate your custom modules into mgmt's language one by one,
and let mgmt run the current mix.

Instead of the usual `--puppet`, `--puppet-conf`, and `--lang` for mcl,
you need to use alternative flags to make this work:

* `--lp-lang` to specify the mcl input
* `--lp-puppet` to specify the puppet input
* `--lp-puppet-conf` to point to the optional puppet.conf file

`mgmt` will derive a graph that contains all edges and vertices from
both inputs. You essentially get two unrelated subgraphs that run in
parallel. To form edges between these subgraphs, you have to define
special vertices that will be merged. This works through a hard-coded
naming scheme.

### Mixed graph example 1 - No merges

```mcl
# lang
file "/tmp/mgmt_dir/" { state => "present" }
file "/tmp/mgmt_dir/a" { state => "present" }
```

```puppet
# puppet
file { "/tmp/puppet_dir": ensure => "directory" }
file { "/tmp/puppet_dir/a": ensure => "file" }
```

These very simple inputs (including implicit edges from directory to
respective file) result in two subgraphs that do not relate.

```
File[/tmp/mgmt_dir/] -> File[/tmp/mgmt_dir/a]

File[/tmp/puppet_dir] -> File[/tmp/puppet_dir/a]
```

### Mixed graph example 2 - Merged vertex

In order to have merged vertices in the resulting graph, you will
need to include special resources and classes in the respective
input code.

* On the lang side, add `noop` resources with names starting in `puppet_`.
* On the Puppet side, add **empty** classes with names starting in `mgmt_`.

```mcl
# lang
noop "puppet_handover_to_mgmt" {}
file "/tmp/mgmt_dir/" { state => "present" }
file "/tmp/mgmt_dir/a" { state => "present" }

Noop["puppet_handover_to_mgmt"] -> File["/tmp/mgmt_dir/"]
```

```puppet
# puppet
class mgmt_handover_to_mgmt {}
include mgmt_handover_to_mgmt

file { "/tmp/puppet_dir": ensure => "directory" }
file { "/tmp/puppet_dir/a": ensure => "file" }

File["/tmp/puppet_dir/a"] -> Class["mgmt_handover_to_mgmt"]
```

The new `noop` resource is merged with the new class, resulting in
the following graph:

```
File[/tmp/puppet_dir] -> File[/tmp/puppet_dir/a]
				|
				V
		Noop[handover_to_mgmt]
			|
			V
	File[/tmp/mgmt_dir/] -> File[/tmp/mgmt_dir/a]
```

You put all your ducks in a row, and the resources from the Puppet input
run before those from the mcl input.

**Note:** The names of the `noop` and the class must be identical after the
respective prefix. The common part (here, `handover_to_mgmt`) becomes the name
of the merged resource.

## Mixed graph example 3 - Multiple merges

In most scenarios, it will not be possible to define a single handover
point like in the previous example. For example, if some Puppet resources
need to run in between two stages of native resources, you need at least
two merged vertices:

```mcl
# lang
noop "puppet_handover" {}
noop "puppet_handback" {}
file "/tmp/mgmt_dir/" { state => "present" }
file "/tmp/mgmt_dir/a" { state => "present" }
file "/tmp/mgmt_dir/puppet_subtree/state-file" { state => "present" }

File["/tmp/mgmt_dir/"] -> Noop["puppet_handover"]
Noop["puppet_handback"] -> File["/tmp/mgmt_dir/puppet_subtree/state-file"]
```

```puppet
# puppet
class mgmt_handover {}
class mgmt_handback {}

include mgmt_handover, mgmt_handback

class important_stuff {
	file { "/tmp/mgmt_dir/puppet_subtree":
		ensure => "directory"
	}
	# ...
}

Class["mgmt_handover"] -> Class["important_stuff"] -> Class["mgmt_handback"]
```

The resulting graph looks roughly like this:

```
File[/tmp/mgmt_dir/] -> File[/tmp/mgmt_dir/a]
	|
	V
Noop[handover] -> ( class important_stuff resources )
			|
			V
		Noop[handback]
			|
			V
File[/tmp/mgmt_dir/puppet_subtree/state-file]
```

You can add arbitrary numbers of merge pairs to your code bases,
with relationships as needed. From our limited experience, code
readability suffers quite a lot from these, however. We advise
to keep these structures simple.
