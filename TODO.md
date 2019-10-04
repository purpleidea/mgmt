# TODO

Here is a TODO list of longstanding items that are either lower-priority, or
more involved in terms of time, skill-level, and/or motivation.

Please have a look, and let us know if you're working on one of the items. It's
best to open an issue to track your progress and to discuss any implementation
questions you might have.

Lastly, if you'd like something different to work on, please ping @purpleidea
and I'll create an issue tailored especially for your approximate golang skill
level and available time commitment in terms of hours you'd need to spend on the
patch.

Happy Hacking!

## Package resource

- [ ] getfiles support on debian [bug](https://github.com/hughsie/PackageKit/issues/118)
- [ ] directory info on fedora [bug](https://github.com/hughsie/PackageKit/issues/117)
- [ ] dnf blocker [bug](https://github.com/hughsie/PackageKit/issues/110)

## File resource [bug](https://github.com/purpleidea/mgmt/issues/64) [:heart:](https://github.com/purpleidea/mgmt/labels/mgmtlove)

- [ ] recurse limit support [:heart:](https://github.com/purpleidea/mgmt/labels/mgmtlove)
- [ ] fanotify support [bug](https://github.com/go-fsnotify/fsnotify/issues/114)

## Svc resource

- [ ] refreshonly support [:heart:](https://github.com/purpleidea/mgmt/issues/464)

## Exec resource

- [ ] base resource improvements

## Timer resource

- [ ] increment algorithm (linear, exponential, etc...) [:heart:](https://github.com/purpleidea/mgmt/labels/mgmtlove)

## User/Group resource

- [ ] automatic edges to file resource [:heart:](https://github.com/purpleidea/mgmt/labels/mgmtlove)

## Http resource

- [ ] base resource [:heart:](https://github.com/purpleidea/mgmt/labels/mgmtlove)

## Etcd improvements

- [ ] fix etcd race bug that only happens during CI testing (intermittently
failing test case issue)

## Torrent/dht file transfer

- [ ] base plumbing

## GPG/Auth improvements

- [ ] base plumbing

## Resource improvements

- [ ] more reversible resources implemented
- [ ] more "cloud" resources

## Language improvements

- [ ] more core functions
- [ ] automatic language formatter, ala `gofmt`
- [ ] gedit/gnome-builder/gtksourceview syntax highlighting
- [ ] vim syntax highlighting
- [ ] emacs syntax highlighting: see `misc/emacs/` (needs updating)
- [ ] exposed $error variable for feedback in the language
- [ ] improve the printf function to add %[]s, %[]f ([]str, []float) and map,
struct, nested etc... %v would be nice too!
- [ ] add line/col/file annotations to AST so we can get locations of errors
that the parser finds
- [ ] add more error messages with the `%error` pattern in parser.y
- [ ] we should have helper functions or language sugar to pull a field out of a
struct, or a value out of a map, or an index out of a list, etc...

## Engine improvements

- [ ] add a "waiting for func" message in the func engine to notify the user
about slow functions...

## Other

- [ ] reproducible builds
- [ ] add your suggestions!
