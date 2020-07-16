## Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### Why did you start this project?

I wanted a next generation config management solution that didn't have all of
the design flaws or limitations that the current generation of tools do, and no
tool existed!

### Why did you choose `golang` for the project?

When I started working on the project, I needed to choose a language that
already had an implementation of a distributed consensus algorithm available.
That meant [Paxos](https://en.wikipedia.org/wiki/Paxos_(computer_science)) or
[Raft](https://en.wikipedia.org/wiki/Raft_(computer_science)). Golang was one
language that actually had two different Raft implementations, `etcd`, and
`consul`. Other design requirements included something that was reasonably fast,
typed and memory-safe, and suited for systems engineering. After a reasonably
extensive search, I chose `golang`. I think it was the right decision. There are
a number of other features of the language which helped influence the decision.

### How do I contribute to the project if I don't know `golang`?

There are many different ways you can contribute to the project. They can be
broadly divided into two main categories:

1. With contributions written in `golang`
2. With contributions _not_ written in `golang`

If you do not know `golang`, and have no desire to learn, you can still
contribute to mgmt by using it, testing it, writing docs, or even just by
telling your friends about it. If you don't mind some coding, learning about the
[mgmt language](https://purpleidea.com/blog/2018/02/05/mgmt-configuration-language/)
might be an enjoyable experience for you. It is a small [DSL](https://en.wikipedia.org/wiki/Domain-specific_language)
and not a general purpose programming language, and you might find it more fun
than what you're typically used to. One of the reasons the mgmt author got into
writing automation modules, was because he found it much more fun to build with
a higher level DSL, than in a general purpose programming language.

If you do not know `golang`, and would like to learn, are a beginner and want to
improve your skills, or want to gain some great interdisciplinary systems
engineering knowledge around a cool automation project, we're happy to mentor
you. Here are some pre-requisites steps which we recommend:

1. Make sure you have a somewhat recent GNU/Linux environment to hack on. A
recent [Fedora](https://getfedora.org/) or [Debian](https://www.debian.org/)
environment is recommended. Developing, testing, and contributing on `macOS` or
`Windows` will be either more difficult or impossible.
2. Ensure that you're mildly comfortable with the basics of using `git`. You can
find a number of tutorials online.
3. Spend between four to six hours with the [golang tour](https://tour.golang.org/).
Skip over the longer problems, but try and get a solid overview of everything.
If you forget something, you can always go back and repeat those parts.
4. Connect to our [#mgmtconfig](https://webchat.freenode.net/?channels=#mgmtconfig)
IRC channel on the [Freenode](https://freenode.net/) network. You can use any
IRC client that you'd like, but the [hosted web portal](https://webchat.freenode.net/?channels=#mgmtconfig)
will suffice if you don't know what else to use.
5. Now it's time to try and starting writing a patch! We have tagged a bunch of
[open issues as #mgmtlove](https://github.com/purpleidea/mgmt/issues?q=is%3Aissue+is%3Aopen+label%3Amgmtlove)
for new users to have somewhere to get involved. Look through them to see if
something interests you. If you find one, let us know you're working on it by
leaving a comment in the ticket. We'll be around to answer questions in the IRC
channel, and to create new issues if there wasn't something that fit your
interests. When you submit a patch, we'll review it and give you some feedback.
Over time, we hope you'll learn a lot while supporting the project! Now get
hacking!

### Is this project ready for production?

It's getting pretty close. I'm able to write modules for it now!

Compared to some existing automation tools out there, mgmt is a relatively new
project. It is probably not as feature complete as some other software, but it
also offers a number of features which are not currently available elsewhere.

Because we have not released a `1.0` release yet, we are not guaranteeing
stability of the internal or external API's. We only change them if it's really
necessary, and we don't expect anything particularly drastic to occur. We would
expect it to be relatively easy to adapt your code if such changes happened.

As with all software, bugs can occur, and while we make no guarantees of being
bug-free, there are a number of things we've done to reduce the chances of one
causing you trouble:

1. Our software is written in golang, which is a memory-safe language, and which
is known to reduce or eliminate entire classes of bugs.
2. We have a test suite which we run on every commit, and every 24 hours. If you
have a particular case that you'd like to test, you are welcome to add it in!
3. The mgmt language itself offers a number of safety features. You can
[read about them in the introductory blog post](https://purpleidea.com/blog/2018/02/05/mgmt-configuration-language/).

Having said all this, as with all software, there are still missing features
which some users might want in their production environments. We're working hard
to get all of those implemented, but we hope that you'll get involved and help
us finish off the ones that are most important to you. We are happy to mentor
new contributors, and have even [tagged](https://github.com/purpleidea/mgmt/issues?q=is%3Aissue+is%3Aopen+label%3Amgmtlove)
a number of issues if you need help getting started.

Some of the current limitations include:

* Auth hasn't been implemented yet, so you should only use it in trusted
environments (not on publicly accessible networks) for now.
* The number of built-in core functions is still small. You may encounter
scenarios where you're missing a function. The good news is that it's relatively
easy to add this missing functionality yourself. In time, with your help, the
list will grow!
* Large file distribution is not yet implemented. You might want a scenario
where mgmt is used to distribute large files (such as `.iso` images) throughout
your cluster. While this isn't a common use-case, it won't be possible until
someone wants to write the patch. (Mentoring available!) You can workaround this
easily by storing those files on a separate fileserver for the interim.
* There isn't an ecosystem of community `modules` yet. We've got this on our
roadmap, so please stay tuned!

We hope you'll participate as an early adopter. Every additional pair of helping
hands gets us all there faster! It's quite possible to use this to build useful
automation today, and we hope you'll start getting familiar with the software.

### Why did you use etcd? What about consul?

Etcd and consul are both written in golang, which made them the top two
contenders for my prototype. Ultimately a choice had to be made, and etcd was
chosen, but it was also somewhat arbitrary. If there is available interest,
good reasoning, *and* patches, then we would consider either switching or
supporting both, but this is not a high priority at this time.

### Can I use an existing etcd cluster instead of the automatic embedded servers?

Yes, it's possible to use an existing etcd cluster instead of the automatic,
elastic embedded etcd servers. To do so, simply point to the cluster with the
`--seeds` variable, the same way you would if you were seeding a new member to
an existing mgmt cluster.

The downside to this approach is that you won't benefit from the automatic
elastic nature of the embedded etcd servers, and that you're responsible if you
accidentally break your etcd cluster, or if you use an unsupported version.

### In `mgmt` you talk about events. What is this referring to?

Mgmt has two main concepts that involve "events":
1. Events in the [resource primitive](resource-guide.md).
2. Events in the [reactive language](language-guide.md).

Each resource primitive in mgmt can test (check) and set (apply) the desired
state that was requested of it. This is familiar to what is common with existing
tools such as `Puppet`, `Ansible`, `Chef`, `Terraform`, etc... In addition,
`mgmt` can also **watch** the state and detect changes. As a result, it never
has to waste time and cpu resources by polling to test and set state, leading to
a design which is algorithmically much faster than the existing generation of
tools.

To describe the set of resources to apply, mgmt describes this collection with a
language. In order to model the time component of infrastructure, we use a
special kind of language called an [FRP](https://en.wikipedia.org/wiki/Functional_reactive_programming).
This language has a built-in concept that we call "events", and which means that
we re-evaluate the relevant portions of the code whenever a value or function
has an event that tells us that it changed. The `R` in `FRP` stands for
reactive. This is similar to how a spreadsheet updates dependent cells when a
pre-requisite value is modified. [This article](https://en.wikipedia.org/wiki/Reactive_programming)
provides a bit more background.

Whenever any of the streams of values in the language change, the program is
partially re-evaluated. The output of any mgmt program is a [DAG](https://en.wikipedia.org/wiki/Directed_acyclic_graph)
of resources, or more precisely, a stream of resource graphs. Since we have
events per-resource, we can efficiently switch from one desired-state resource
graph to the next without re-checking their individual states, since we've been
monitoring them all along.

One side-effect of all this, is that if a rogue systems administrator manually
changes the state of any managed resource, mgmt will detect this and attempt to
revert the change. This makes for excellent live demos, but is not the primary
design goal. It is a consequence of tracking state so that graph changes are
efficient. We implement the event detection via an intentional per-resource
[main loop](https://en.wikipedia.org/wiki/Event_loop) which can enable other
interesting functionality too!

Make sure to get rid of your rogue sysadmin! ;)

### Do I need to run `mgmt` as `root`?

No and yes. It depends. Nothing in mgmt explicitly requires root in the design,
however mgmt will require root only if the changes to your system that you want
it to make require root.

For example, if you use it to manage files that require root access to modify,
then you'll need root. If you only use it to manage files and resources
elsewhere, then it shouldn't need root. Many resources are perfectly usable
without root, and virtually all of my live demos are done without root.

### How can I run `mgmt` on-demand, or in `cron`, instead of continuously?

By default, `mgmt` will run continuously in an attempt to keep your machine in a
converged state, even as external forces change the current state, or as your
time-varying desired state changes over time. (You can write code in the mgmt
language which will let you describe a desired state which might change over
time.)

Some users might prefer to only run `mgmt` on-demand manually, or at a set
interval via a tool like `cron`. In order to do so, `mgmt` must have a way to
shut itself down after a single "run". This feature is possible with the
`--converged-timeout` flag. You may specify this flag, along with a number of
seconds as the argument, and when there has been no activity for that many
seconds, the program will shutdown.

Alternatively, while it is not recommended, if you'd like to ensure the program
never runs for longer that a specific number of seconds, you can ask it to
shutdown after that time interval using the `--max-runtime` flag. This also
requires a number of seconds as an argument.

#### Example:

```
./mgmt run lang examples/lang/hello0.mcl --converged-timeout=5
```

### When I try to build `mgmt` I see: `no Go files in $GOPATH/src/github.com/purpleidea/mgmt/bindata`.

Due to the arcane way that `golang` designed its `$GOPATH`, the main project
directory must be inside your `$GOPATH`, and at the appropriate FQDN. This is:
`$GOPATH/src/github.com/purpleidea/mgmt/`. If you have your project root outside
of that directory, then you may get this error when you try to build it. In this
case there is likely a `go get` version of the project at this location. Remove
it and replace it with your git cloned directory. In my case, I like to work on
things in `~/code/mgmt/`, so that path is a symlink that points to the long
project directory.

### Why does my file resource error with `no such file or directory`?

If you create a file resource and only specify the content like this:

```
file "/tmp/foo" {
	content => "hello world\n",
}
```

Then this will attempt to set the contents of that file to the desired string,
but *only* if that file already exists. If you'd like to ensure that it also
gets created in case it is not present, then you must also specify the state:

```
file "/tmp/foo" {
	state => $const.res.file.state.exists,
	content => "hello world\n",
}
```

Similar logic applies for situations when you only specify the `mode` parameter.

This all turns out to be more safe and "correct", in that it would error and
prevent masking an error for a situation when you expected a file to already be
at that location. It also turns out to simplify the internals significantly, and
remove an ambiguous scenario with the reversable file resource.

### Why do function names inside of templates include underscores?

The golang template library which we use to implement the template() function
doesn't support the dot notation, so we import all our normal functions, and
just replace dots with underscores. As an example, the standard `datetime.print`
function is shown within mcl scripts as datetime_print after being imported.

### On startup `mgmt` hangs after: `etcd: server: starting...`.

If you get an error message similar to:

```
etcd: server: starting...
etcd: server: start timeout of 1m0s reached
etcd: server: close timeout of 15s reached
```

But nothing happens afterwards, this can be due to a corrupt etcd storage
directory. Each etcd server embedded in mgmt must have a special directory where
it stores local state. It must not be shared by more than one individual member.
This dir is typically `/var/lib/mgmt/etcd/member/`. If you accidentally use it
(for example during testing) with a different cluster view, then you can corrupt
it. This can happen if you use it with more than one different hostname.

The solution is to avoid making this mistake, and if there is no important data
saved, you can remove the etcd member dir and start over.

### On running `make` to build a new version, it errors with: `Text file busy`.

If you get an error like:

```
cp: cannot create regular file 'mgmt': Text file busy
```

This can happen if you ran `make build` (or just `make`) when there was already
an instance of mgmt running, or if a related file locking issue occurred. To
solve this, shutdown and running mgmt process, run `rm mgmt` to remove the file,
and then get a new one by running `make` again.

### The docs speaks of `--remote` but the CLI errors out?

The `--remote` flag existed in an earlier version of mgmt. It was removed and
will be replaced with a more powerful version, which is a "remote" resource. The
code is mostly ready but it's not finished. If you'd like to help finish it or
sponsor the work, please let me know.

### Does this support Windows? OSX? GNU Hurd?

Mgmt probably works best on Linux, because that's what most developers use for
serious automation workloads. Support for non-Linux operating systems isn't a
high priority of mine, but we're happy to accept patches for missing features
or resources that you think would make sense on your favourite platform.

### Why aren't you using `glide`, `godep` or `go mod` for dependency management?

Vendoring dependencies means that as the git master branch of each dependency
marches on, you're left behind using an old version. As a result, bug fixes and
improvements are not automatically brought into the project. Instead, we run our
complete test suite against the entire project (with the latest dependencies)
[every 24 hours](https://docs.travis-ci.com/user/cron-jobs/) to ensure that it
all still works.

Occasionally a dependency breaks API and causes a failure. In those situations,
we're notified almost immediately, it's easy to see exactly which commit caused
the breakage, and we can either quickly notify the author (if it was a mistake)
or update our code if it was a sensible change. This also puts less burden on
authors to support old, legacy versions of their software unnecessarily.

Historically, we've had approximately one such breakage per year, which were all
detected and fixed within a few hours. The cost of these small, rare,
interruptions is much less expensive than having to periodically move every
dependency in the project to the latest versions. Some examples of this include:

* We caught the `go-bindata` swap before it was publicly known, and fixed it in:
[adbe9c7be178898de3645b0ed17ed2ca06646017](https://github.com/purpleidea/mgmt/commit/adbe9c7be178898de3645b0ed17ed2ca06646017).

* We caught the `codegangsta/cli` API change improvement, and fixed it in:
[ab73261fd4e98cf7ecb08066ad228a8f559ba16a](https://github.com/purpleidea/mgmt/commit/ab73261fd4e98cf7ecb08066ad228a8f559ba16a).

* We caught an un-announced libvirt API change, and promptly fixed it in:
[95cb94a03958a9d2ebf01df0821a8c13a4f3a28c](https://github.com/purpleidea/mgmt/commit/95cb94a03958a9d2ebf01df0821a8c13a4f3a28c).

If we choose responsible dependencies, then it usually means that those authors
are also responsible with their changes to API and to git master. If we ever
find that it's not the case, then we will either switch that dependency to a
more responsible version, or fork it if necessary.

Occasionally, we want to pin a dependency to a particular version. This can
happen if the project treats `git master` as an unstable branch, or because a
dependency needs a newer version of golang than the minimum that we require for
our project. In those cases it's sensible to assume the technical debt, and
vendor the dependency. The common tools such as `glide` and `godep` work by
requiring you install their software, and by either storing a yaml file with the
version of that dependency in your repository, and/or copying all of that code
into git and explicitly storing it. This project thinks that all of these
solutions are wasteful and unnecessary, particularly when an existing elegant
solution already exists: `[git submodules](https://git-scm.com/book/en/v2/Git-Tools-Submodules)`.

The advantages of using `git submodules` are three-fold:
1. You already have the required tools installed.
2. You only store a pointer to the dependency, not additional files or code.
3. The git submodule tools let you easily switch dependency versions, see diff
output, and responsibly plan and test your versions bumps with ease.

Don't blindly use the tools that others tell you to. Learn what they do, think
for yourself, and become a power user today! That process led us to using
`git submodules`. Hopefully you'll come to the same conclusions that we did.

### Did you know that there is a band named `MGMT`?

I didn't realize this when naming the project, and it is accidental. After much
anguishing, I chose the name because it was short and I thought it was
appropriately descriptive. If you need a less ambiguous search term or phrase,
you can try using `mgmtconfig` or `mgmt config`.

It also doesn't stand for
[Methyl Guanine Methyl Transferase](https://en.wikipedia.org/wiki/O-6-methylguanine-DNA_methyltransferase)
which definitely existed before the band did.

### You didn't answer my question, or I have a question!

It's best to ask on [IRC](https://webchat.freenode.net/?channels=#mgmtconfig)
to see if someone can help you. If you don't get a response from IRC, you can
contact me through my [technical blog](https://purpleidea.com/contact/) and I'll
do my best to help. If you have a good question, please add it as a patch to
this documentation. I'll merge your question, and add a patch with the answer!
For news and updates, subscribe to the [mailing list](https://www.redhat.com/mailman/listinfo/mgmtconfig-list).
