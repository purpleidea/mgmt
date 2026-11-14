# Provisioner

This is the `embedded/provisioner` module, which can network boot and install
Fedora and Debian hosts. The `mgmt provisioner` command line tool is built from
it, with `top.mcl` as its code. If you need more than that tool can do, import
this module in your own code instead, and glue it together the way you like.

## Remote handoff

Once a host has been installed, the provisioner can connect to it over ssh and
run some of your code there. Nothing needs to be installed on the host ahead of
time, since the provisioner pushes its own mgmt binary, and runs it there as a
transient child of itself, which reaches the etcd of the provisioner through
that ssh connection. That child runs the same code as the provisioner, which
includes your code. When your code has done its work, it tells the provisioner
with the `remote:done` resource.

### Example

Make a folder for your site with a `handoff/` module in it. Git ignores the
`sites/` dir of this repo, so that's a good place to keep your own site code,
which usually has private details in it, such as mac addresses and keys:

```
sites/example/
├── handoff/
│   └── main.mcl
└── provision.sh
```

The module must have a `host` class. The provisioner includes it on the host
with the name that it has for that host, and your code must use that same name
for its `remote:done` resource once its work is done. This one installs a
package and writes a file:

```mcl
import "res/remote"

class host($name) {
	$parent = remote.parent()

	pkg "htop" {
		state => "installed",
	}

	file "/etc/motd" {
		state => $const.res.file.state.exists,
		content => "${name} was provisioned by ${parent}\n",
	}

	remote:done $name {}

	Pkg["htop"] -> Remote:Done[$name]
	File["/etc/motd"] -> Remote:Done[$name]
}
```

Then from inside that folder, as root, run:

```bash
cd sites/example/
mgmt provisioner --mac 00:11:22:33:44:55 --proxy --handoff-remote handoff/
```

With `--proxy`, the provisioner uses proxy dhcp, so it works on the network that
you already have. Your own dhcp server still gives the host its ip, and the
provisioner only sends it what it needs to boot from the network. Leave it out
to have the provisioner run its own dhcp server instead, on a private network.

So that you don't need to remember that, keep it in a `provision.sh` script in
the same folder, with the settings for your site, and run `sudo ./provision.sh`
instead. It can pass any other args that you give it on to the provisioner:

```bash
#!/usr/bin/env bash
mac="00:11:22:33:44:55" # the host to provision

cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1 # handoff/ is beside us
MGMT="${MGMT:-../../mgmt}" # the one that's built in this repo, by default
exec "$MGMT" provisioner --mac "$mac" --proxy --handoff-remote handoff/ "$@"
```

**Warning:** this erases the disk of the host with that mac address, as soon as
it boots from the network!

Here's what happens next:

1. The host with that mac address boots from the network, and the installer
erases its disk and installs it.
2. At the end of the install, the host tells the provisioner that it's done,
and sends it its ip address and its ssh host key, which the provisioner stores
in etcd.
3. Once the host has rebooted, the provisioner connects to it as root at that
ip, checks that it has that same host key, and runs mgmt on it. Until sshd
answers, it keeps on trying every ten seconds.
4. That mgmt sees that it's a child, so it includes `handoff.host("mgmt0")`,
instead of running the provisioner. Your code installs `htop` and writes
`/etc/motd` on the host.
5. Your `remote:done "mgmt0"` resource tells the provisioner that it's done, so
it records that, stops the child, and then it prints a message which says:
`mgmt0 has finished its remote handoff`. It won't run your code there again.

### Without a mac address

If you leave out `--mac`, then the provisioner installs any host which boots
from its network, and each one of them gets the remote handoff. They have no
names of their own, so each is named `mgmt` with its mac address added, such as
`mgmt-001122334455`, and that's the name your `host` class gets. With `--proxy`,
that's your existing network, so you must say which hosts that is:

```bash
mgmt provisioner --proxy --proxy-force --handoff-remote handoff/
```

**Warning:** this erases the disk of *every* host that boots from the network!
Use `--proxy-range` instead of `--proxy-force` to limit it to the hosts which
get an ip in that range from the other dhcp server.

### Flags

* `--handoff-remote DIR` is the module dir. Since `top.mcl` imports it, the tool
builds its deploy from a copy of `top.mcl` and of this dir, in the `deploy/` dir
of its cache prefix. That happens each time it starts, so any edits take effect
the next time.
* `--ssh-key PATH` is a public key file. That key is authorized for root on the
host, and the provisioner logs in with the matching private key, which must be
in the `~/.ssh/` dir of the user that runs it. With `sudo` that's root. It
defaults to `~/.ssh/id_ed25519.pub` if that exists, so you usually don't need
it. Use `--ssh-key ''` to not authorize any key.
* `--mac` picks the one host to install. Without it, every host that boots from
the network is installed. (See above.) You don't need `--ip`, since the host
tells us its ip once it's installed.
* `--handoff-module-path DIR` is the module path that your module imports from.
* `--handoff-hostname NAME` sets the hostname of the host. The child always uses
its name, such as `mgmt0`, as its hostname in the cluster, since that must be
unique, and without `--mac`, every host gets this same hostname.

### Writing the module

* A module can only contain `import`, variable, `func` and `class` statements,
so all of your resources go in the `host` class.
* Use `remote:done $name {}` when your work is done, with edges to it from
everything which must happen first. Its `signal`, `error` and `return` params
say how the provisioner should stop the child, if it failed, and if anything
changed.
* The `provisioner.cli_*()` functions don't have any values on the host, so you
can't use them there.
* What happens after `remote:done` is up to you. By default the provisioner
stops the child once it's done, but your code could first set up something which
keeps on running, such as an mgmt service that joins another cluster.
* Once the host is done, the provisioner removes what the host sent it from its
store, since the host is now on its own. It remembers that the host is done,
until that's cleared with the `clear` param of the `remote` resource. So if you
reinstall that host while the tool is still running, your code won't run on it
again.
* The name that your `host` class gets is the name of the `remote` resource that
started the child, which `remote.name()` also returns.

## Using the module yourself

Most real setups will import this module in their own code, instead of using the
`mgmt provisioner` tool. The remote handoff has two halves. Set
`handoff => "remote"` in the config of the `base:host` class so that it installs
the host for it, and then include the `base:handoff` class, which runs mgmt on
it once it's installed. Use `remote.is_child()` to choose which part of your
code runs on the host, since the child gets your whole deploy. This uses proxy
dhcp, which needs `proxy => true` in both the `base` and the `base:host` class:

```mcl
import "embedded/provisioner"
import "os"
import "res/remote"
import "handoff/"

$mac = "00:11:22:33:44:55"

if remote.is_child() {
	include handoff.host(remote.name())
} else {
	include provisioner.base(struct{
		proxy => true,
		prefix => "/var/cache/mgmt/provisioner/",
	}) as base

	include base.repo(struct{
		distro => "fedora",
		version => "44",
		arch => "x86_64",
		mirror => "https://download.fedoraproject.org/pub/fedora/linux/",
	})

	include base.host("server1", struct{
		repo => "fedora44-x86_64",
		mac => $mac,
		proxy => true,
		sshkey => os.readfile("/root/.ssh/id_ed25519.pub"),
		handoff => "remote",
	})

	include base.handoff(struct{
		hosts => {$mac => "server1"},
	})

	if remote.is_done("server1") {
		print "next" {
			msg => "server1 has finished its first stage",
		}
	}
}
```

The `hosts` of the `base:handoff` class are the hosts that it hands off to, from
their mac address to the name that the child gets. With `any => true`, it also
hands off to every host that a `base:range` installs, and with `sshid`, you can
pick the private key that it logs in with. The hosts report to the `base` class,
which stores what they send in the `provisioner_hosts` key in etcd, unless you
pick another one with its `store_key`. The `$provisioned`, `$host_ip` and
`$host_hostkey` values of the `base:host` class tell you when the host is
installed, and what it sent. There's a complete version of this in
`examples/lang/provisioner-remote/`.

## Provisioning on request

The `base:runner` class makes a long running provisioner, which installs each
host that someone asks for, while it's running. Include it, along with the
`base:handoff` class, and an `sshkey` for it to log in with:

```mcl
include base.runner(struct{
	sshkey => os.readfile("/root/.ssh/id_ed25519.pub"),
})
include base.handoff(struct{})
```

To ask for a host, add an entry with its name to the `provisioner` namespace in
the etcd of the provisioner, with its config as json:

```bash
etcdctl put /_mgmt/strings/provisioner/web1 '{"mac": "00:11:22:33:44:55", "uid": "fedora44-x86_64", "flavour": "Server", "luks": true}'
```

Here's what happens next:

1. The runner sees the entry, and installs that host with the `base:host` class,
named `web1`. On the default network, the entry needs an `ip` too.
2. Once it's installed, the host reports to the provisioner, which stores that
as its record. The runner puts its name into that record, and then removes the
entry, so it won't install it again if it boots from the network again.
3. The `base:handoff` class then runs mgmt on it, as `web1`, which includes your
`handoff.host("web1")`, and once that's done, it removes its record too.

An entry that can't be used, such as with an invalid mac, is skipped, and the
provisioner prints why. If you ask for `web1` again later, it's installed again,
and it gets the handoff again.

With `"handoff": "svc"` in the entry, the host instead runs mgmt as a service on
its own, which joins the cluster of the provisioner over ssh. That needs the ssh
address and host key of its etcd server to be in the `provisioner_etcd_server`
key. The provisioner keeps the record of that host, since that's where it gets
the ssh key of the host from, which it exports, so that the server lets it in.

**Note:** if you ask for a host while it still has a record from the last time,
such as an `svc` host, or one whose handoff is still running, then the runner
uses that record, instead of installing that host again.
