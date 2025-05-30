# Resources

Here we list all the built-in resources and their properties. The resource
primitives in `mgmt` are typically more powerful than resources in other
configuration management systems because they can be event based which lets them
respond in real-time to converge to the desired state. This property allows you
to build more complex resources that you probably hadn't considered in the past.

In addition to the resource specific properties, there are resource properties
(otherwise known as parameters) which can apply to every resource. These are
called [meta parameters](documentation.md#meta-parameters) and are listed
separately. Certain meta parameters aren't very useful when combined with
certain resources, but in general, it should be fairly obvious, such as when
combining the `noop` meta parameter with the [Noop](#Noop) resource.

You might want to look at the [generated documentation](https://godoc.org/github.com/purpleidea/mgmt/engine/resources)
for more up-to-date information about these resources.

* [Augeas](#Augeas): Manipulate files using augeas.
* [Consul:KV](#ConsulKV): Set keys in a Consul datastore.
* [Docker](#Docker):[Container](#Container) Manage docker containers.
* [Exec](#Exec): Execute shell commands on the system.
* [File](#File): Manage files and directories.
* [Group](#Group): Manage system groups.
* [Hostname](#Hostname): Manages the hostname on the system.
* [KV](#KV): Set a key value pair in our shared world database.
* [Msg](#Msg): Send log messages.
* [Net](#Net): Manage a local network interface.
* [Noop](#Noop): A simple resource that does nothing.
* [Nspawn](#Nspawn): Manage systemd-machined nspawn containers.
* [Password](#Password): Create random password strings.
* [Pkg](#Pkg):  Manage system packages with PackageKit.
* [Print](#Print): Print messages to the console.
* [Svc](#Svc): Manage system systemd services.
* [Test](#Test): A mostly harmless resource that is used for internal testing.
* [Tftp:File](#TftpFile): Add files to the small embedded embedded tftp server.
* [Tftp:Server](#TftpServer): Run a small embedded tftp server.
* [Timer](#Timer): Manage system systemd services.
* [User](#User): Manage system users.
* [Virt](#Virt): Manage virtual machines with libvirt.

## Augeas

The augeas resource uses [augeas](http://augeas.net/) commands to manipulate
files.

## Docker

### Container

The docker:container resource manages docker containers.

It has the following properties:

* `state`: either `running`, `stopped`, or `removed`
* `image`: docker `image` or `image:tag`
* `cmd`: a command or list of commands to run on the container
* `env`: a list of environment variables, e.g. `["VAR=val",],`
* `ports`: a map of portmappings, e.g. `{"tcp" => {8080 => 80, 8443 => 443,},},`
* `apiversion:` override the host's default docker version, e.g. `"v1.35"`
* `force`: destroy and rebuild the container instead of erroring on wrong image

## Exec

The exec resource can execute commands on your system.

## File

The file resource manages files and directories. In `mgmt`, directories are
identified by a trailing slash in their path name. File have no such slash.

It has the following properties:

* `path`: absolute file path (directories have a trailing slash here)
* `state`: either `exists`, `absent`, or undefined
* `content`: raw file content
* `mode`: octal unix file permissions or symbolic string
* `owner`: username or uid for the file owner
* `group`: group name or gid for the file group

### Path

The path property specifies the file or directory that we are managing.

### State

The state property describes the action we'd like to apply for the resource. The
possible values are: `exists` and `absent`. If you do not specify either of
these, it is undefined. Without specifying this value as `exists`, another param
cannot cause a file to get implicitly created. When specifying this value as
`absent`, you should not specify any other params that would normally change the
file. For example, if you specify `content` and this param is `absent`, then you
will get an engine validation error.

### Content

The content property is a string that specifies the desired file contents.

### Source

The source property points to a source file or directory path that we wish to
copy over and use as the desired contents for our resource.

### Fragments

The fragments property lets you specify a list of files to concatenate together
to make up the contents of this file. They will be combined in the order that
they are listed in. If one of the files specified is a directory, then the
files in that top-level directory will be themselves combined together and used.

### Recurse

The recurse property limits whether file resource operations should recurse into
and monitor directory contents with a depth greater than one.

### Force

The force property is required if we want the file resource to be able to change
a file into a directory or vice-versa. If such a change is needed, but the force
property is not set to `true`, then this file resource will error.

### Purge

The purge property is used when this file represents a directory, and we'd like
to remove any unmanaged files from within it. Please note that any unmanaged
files in a directory with this flag set will be irreversibly deleted.

## Group

The group resource manages the system groups from `/etc/group`.

## Hostname

The hostname resource manages static, transient/dynamic and pretty hostnames
on the system and watches them for changes.

### static_hostname

The static hostname is the one configured in /etc/hostname or a similar
file.
It is chosen by the local user. It is not always in sync with the current
host name as returned by the gethostname() system call.

### transient_hostname

The transient / dynamic hostname is the one configured via the kernel's
sethostbyname().
It can be different from the static hostname in case DHCP or mDNS have been
configured to change the name based on network information.

### pretty_hostname

The pretty hostname is a free-form UTF8 host name for presentation to the user.

### hostname

Hostname is the fallback value for all 3 fields above, if only `hostname` is
specified, it will set all 3 fields to this value.

## KV

The KV resource sets a key and value pair in the global world database. This is
quite useful for setting a flag after a number of resources have run. It will
ignore database updates to the value that are greater in compare order than the
requested key if the `SkipLessThan` parameter is set to true. If we receive a
refresh, then the stored value will be reset to the requested value even if the
stored value is greater.

### Key

The string key used to store the key.

### Value

The string value to set. This can also be set via Send/Recv.

### SkipLessThan

If this parameter is set to `true`, then it will ignore updating the value as
long as the database versions are greater than the requested value. The compare
operation used is based on the `SkipCmpStyle` parameter.

### SkipCmpStyle

By default this converts the string values to integers and compares them as you
would expect.

## Msg

The msg resource sends messages to the main log, or an external service such
as systemd's journal.

## Net

The net resource manages a local network interface using netlink.

## Noop

The noop resource does absolutely nothing. It does have some utility in testing
`mgmt` and also as a placeholder in the resource graph.

## Nspawn

The nspawn resource is used to manage systemd-machined style containers.

## Password

The password resource can generate a random string to be used as a password. It
will re-generate the password if it receives a refresh notification.

## Pkg

The pkg resource is used to manage system packages. This resource works on many
different distributions because it uses the underlying packagekit facility which
supports different backends for different environments. This ensures that we
have great Debian (deb/dpkg) and Fedora (rpm/dnf) support simultaneously.

## Print

The print resource prints messages to the console.

## Svc

The service resource is still very WIP. Please help us by improving it!

## Test

The test resource is mostly harmless and is used for internal tests.

## Tftp:File

This adds files to the running tftp server. It's useful because it allows you to
add individual files without needing to create them on disk.

## Tftp:Server

Run a small embedded tftp server. This doesn't apply any state, but instead runs
a pure golang tftp server in the Watch loop.

## Timer

This resource needs better documentation. Please help us by improving it!

## User

The user resource manages the system users from `/etc/passwd`.

## Virt

The virt resource can manage virtual machines via libvirt.
