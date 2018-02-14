# Frequently asked questions

(Send your questions as a patch to this FAQ! I'll review it, merge it, and
respond by commit with the answer.)

### Why did you start this project?

I wanted a next generation config management solution that didn't have all of
the design flaws or limitations that the current generation of tools do, and no
tool existed!

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

### Why do resources have both a `Compare` method and an `IFF` (on the UID) method?

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
to see if someone can help you. Once we get a big enough community going, we'll
add a mailing list. If you don't get any response from the above, you can
contact me through my [technical blog](https://purpleidea.com/contact/)
and I'll do my best to help. If you have a good question, please add it as a
patch to this documentation. I'll merge your question, and add a patch with the
answer!
