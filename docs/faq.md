## Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### Why did you start this project?

I wanted a next generation config management solution that didn't have all of
the design flaws or limitations that the current generation of tools do, and no
tool existed!

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
./mgmt run lang --lang examples/lang/hello0.mcl --converged-timeout=5
```

### What does the error message about an inconsistent dataDir mean?

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

### Why do resources have both a `Cmp` method and an `IFF` (on the UID) method?

The `Cmp()` methods are for determining if two resources are effectively the
same, which is used to make graph change delta's efficient. This is when we want
to change from the current running graph to a new graph, but preserve the common
vertices. Since we want to make this process efficient, we only update the parts
that are different, and leave everything else alone. This `Cmp()` method can
tell us if two resources are the same. In case it is not obvious, `cmp` is an
abbrev. for compare.

The `IFF()` method is part of the whole UID system, which is for discerning if a
resource meets the requirements another expects for an automatic edge. This is
because the automatic edge system assumes a unified UID pattern to test for
equality. In the future it might be helpful or sane to merge the two similar
comparison functions although for now they are separate because they are
actually answer different questions.

### Does this support Windows? OSX? GNU Hurd?

Mgmt probably works best on Linux, because that's what most developers use for
serious automation workloads. Support for non-Linux operating systems isn't a
high priority of mine, but we're happy to accept patches for missing features
or resources that you think would make sense on your favourite platform.

### Why aren't you using `glide` or `godep` for dependency management?

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
to see if someone can help you. For further information and news, subscribe to the [mailing list](https://www.redhat.com/mailman/listinfo/mgmtconfig-list). If you don't get any response from the above, you can
contact me through my [technical blog](https://purpleidea.com/contact/)
and I'll do my best to help. If you have a good question, please add it as a
patch to this documentation. I'll merge your question, and add a patch with the
answer!
