# Alpha releases

#### 0.0.3 2016-03-28

* Update docs to refer to #mgmtlove patches
* Make debugging easier when running on Jenkins
* Switch to CentOS-7.1 for OMV tests.
* Add test case to match headers
* Update web articles
* Add a link to my article about debugging golang
* Add DevConf.cz video recording
* Add link to slides from Walter Heck
* Rename type to resource (res) and service to svc
* Add great article by felix frank
* Change API from StateOK/Apply to CheckApply()
* Make sure to error on test
* Fix golang 1.6 vet issue
* Add some fixes for building in tip or with golang 1.6
* Fix Makefile warnings on Travis
* Add package (pkg) resource
* Rename GetRes() which is not descriptive, to Kind()
* Clean up the examples/ directory
* Fix file resource regression
* Update Makefile plumbing and associated misc things
* Bump go versions in travis
* Add initial "autoedge" plumbing
* Make resource "kind" determination more obvious
* Add some of the pkg and svc autoedge logic
* Update make deps script to make it better for debian folks
* Update README and docs with information about new blog post
* Refactor out noop resource into a separate file
* Split the Res interface into a Base sub piece
* Update context since etcd upstream moved to vendor/ dir
* Add golint test improvements so we detect branches more cleverly
* Avoid errors on golint test with travis
* Support "noarch" style packages in arch check
* Graph cleanups to make way for the autogroup feature!
* Add resource auto grouping
* Force process events to be synchronous
* Add grouping algorithm
* Allow failures in 1.4.x because of etcd problem

This release contains contributions by Xavi S.B, Rob Wilson, and Daniele Sluijters.

#### 0.0.2 2016-02-12

* Add better reporting of errors in yaml formatting test
* Small grep flag fix so command is idempotent
* Add omv support
* Add missing watch event for files
* Add more shields!
* Add tag script
* Update gofmt test to allow version 1.5
* Reorganize testing for developer efficiency
* Add graphviz generation and visualization
* Update README
* Support N distributed agents
* Fix up go vet errors and integrate with ci
* Add missing stringer dependency
* Add information on which libraries are being used
* Fix effective off-by-one error in dependency processing
* Clean up the distributed example for clarity
* Catch a different form of etcd disconnect
* Fix dependency issue
* Fixup state related items
* Add state caching for most types
* Add state caching and invalidation to service type
* Avoid panic's when referencing non-existing objects
* Exit if program was not compiled correctly
* Limit the number of initial start poke's required
* Many examples now exist
* Improve wording in README.md for clarification
* Add information on providing good logs
* Make sure to unpause all elements when resuming
* Fix failure of go 1.4.3 due to missing `go vet`
* Don't generate file watch events if disabled
* Update faq to add etcd vs. consul answer
* Add shell based test harness
* Fix string issues in the build
* Enable shell tests
* Add CentOS jenkins ci hooks
* Add gopath Makefile target
* Add centos-ci script to mgmt for independence and for make gopath
* some debug output
* Add the ability to run individual shell tests manually
* Allow unbound variables like "$1"
* Add a fan in, fan out example and test
* Make it easier to use converged-timeout
* Add a simple bashfmt test
* Add a TODO list to serve as a near term roadmap
* Path fixes to avoid overwriting each other
* Fix up path issues with vtest+
* Pave the way for Debian
* Make an initial RPM package for COPR

This release contains contributions by Felix Frank.

#### 0.0.1 2015-09-25

 * initial release
