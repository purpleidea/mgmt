# Mgmt
# Copyright (C) James Shubin and the project contributors
# Written by James Shubin <james@shubin.ca> and the project contributors
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
#
# Additional permission under GNU GPL version 3 section 7
#
# If you modify this program, or any covered work, by linking or combining it
# with embedded mcl code and modules (and that the embedded mcl code and
# modules which link with this program, contain a copy of their source code in
# the authoritative form) containing parts covered by the terms of any other
# license, the licensors of this program grant you additional permission to
# convey the resulting work. Furthermore, the licensors of this program grant
# the original author, James Shubin, additional permission to update this
# additional permission if he deems it necessary to achieve the goals of this
# additional permission.

SHELL = bash
.PHONY: all art cleanart version program lang path deps run race generate build build-debug crossbuild clean test gofmt yamlfmt format docs
.PHONY: rpmbuild mkdirs rpm srpm spec tar upload upload-sources upload-srpms upload-rpms upload-releases copr tag
.PHONY: mkosi mkosi_fedora-latest mkosi_fedora-older mkosi_stream-latest mkosi_debian-stable mkosi_ubuntu-latest mkosi_archlinux
.PHONY: release release_test releases_path release_binary_amd64 release_binary_arm64 release_fedora-latest release_fedora-older release_stream-latest release_debian-stable release_ubuntu-latest release_archlinux
.PHONY: funcgen
.SILENT: clean

# a large amount of output from this `find`, can cause `make` to be much slower!
GO_FILES := $(shell find * -name '*.go' -not -path 'old/*' -not -path 'tmp/*')
MCL_FILES := $(shell find lang/ -name '*.mcl' -not -path 'old/*' -not -path 'tmp/*')
MISC_FILES := $(shell find engine/resources/http_server_ui/)

SVERSION := $(or $(SVERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --dirty --always))
VERSION := $(or $(VERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --abbrev=0))
PROGRAM := $(shell echo $(notdir $(CURDIR)) | cut -f1 -d"-")
PKGNAME := $(shell go list .)
ifeq ($(VERSION),$(SVERSION))
	RELEASE = 1
else
	RELEASE = untagged
endif
# debugging is harder if -trimpath is set, so disable it if env var is set
# this is force enabled automatically when using the release target
ifeq ($(MGMT_NOTRIMPATH),true)
	TRIMPATH =
else
	TRIMPATH = -trimpath
endif
ifeq ($(MGMT_NOGOLANGRACE),true)
	GOLANGRACE =
else
	GOLANGRACE = -race
endif
ARCH = $(uname -m)
SPEC = rpmbuild/SPECS/$(PROGRAM).spec
SOURCE = rpmbuild/SOURCES/$(PROGRAM)-$(VERSION).tar.bz2
SRPM = rpmbuild/SRPMS/$(PROGRAM)-$(VERSION)-$(RELEASE).src.rpm
SRPM_BASE = $(PROGRAM)-$(VERSION)-$(RELEASE).src.rpm
RPM = rpmbuild/RPMS/$(PROGRAM)-$(VERSION)-$(RELEASE).$(ARCH).rpm
USERNAME := $(shell cat ~/.config/copr 2>/dev/null | grep username | awk -F '=' '{print $$2}' | tr -d ' ')
SERVER = 'dl.fedoraproject.org'
REMOTE_PATH = '/srv/pub/alt/$(USERNAME)/$(PROGRAM)'
ifneq ($(GOTAGS),)
	BUILD_FLAGS = -tags '$(GOTAGS)'
endif
GOOSARCHES ?= linux/amd64 linux/ppc64 linux/ppc64le linux/arm64 darwin/amd64

GOHOSTOS = $(shell go env GOHOSTOS)
GOHOSTARCH = $(shell go env GOHOSTARCH)

# The underscores separate the prefix name ("TOKEN") the distro ("BINARY",
# "FEDORA-LATEST", etc...) and the arch ("AMD64"). The distro can have a dash.
TOKEN_BINARY_AMD64 = $(shell grep -v '#' releases/binary-amd64.release)
TOKEN_BINARY_ARM64 = $(shell grep -v '#' releases/binary-arm64.release)
TOKEN_FEDORA-LATEST = $(shell grep -v '#' releases/fedora-latest.release)
TOKEN_FEDORA-OLDER = $(shell grep -v '#' releases/fedora-older.release)
TOKEN_STREAM-LATEST = $(shell grep -v '#' releases/stream-latest.release)
TOKEN_DEBIAN-STABLE = $(shell grep -v '#' releases/debian-stable.release)
TOKEN_UBUNTU-LATEST = $(shell grep -v '#' releases/ubuntu-latest.release)
TOKEN_ARCHLINUX = $(shell grep -v '#' releases/archlinux.release)

FILE_BINARY_AMD64 = mgmt-linux-amd64-$(VERSION)
FILE_BINARY_ARM64 = mgmt-linux-arm64-$(VERSION)
# TODO: add ARCH onto these at the end, eg _AMD64
FILE_FEDORA-LATEST = mgmt-$(TOKEN_FEDORA-LATEST)-$(VERSION)-1.x86_64.rpm
FILE_FEDORA-OLDER = mgmt-$(TOKEN_FEDORA-OLDER)-$(VERSION)-1.x86_64.rpm
FILE_STREAM-LATEST = mgmt-$(TOKEN_STREAM-LATEST)-$(VERSION)-1.x86_64.rpm
FILE_DEBIAN-STABLE = mgmt_$(TOKEN_DEBIAN-STABLE)_$(VERSION)_amd64.deb
FILE_UBUNTU-LATEST = mgmt_$(TOKEN_UBUNTU-LATEST)_$(VERSION)_amd64.deb
FILE_ARCHLINUX = mgmt-$(TOKEN_ARCHLINUX)-$(VERSION)-1-x86_64.pkg.tar.xz

PKG_BINARY_AMD64_MASTER = releases/master/$(TOKEN_BINARY_AMD64)/$(FILE_BINARY_AMD64)
PKG_BINARY_ARM64_MASTER = releases/master/$(TOKEN_BINARY_ARM64)/$(FILE_BINARY_ARM64)
PKG_BINARY_AMD64 = releases/$(VERSION)/$(TOKEN_BINARY_AMD64)/$(FILE_BINARY_AMD64)
PKG_BINARY_ARM64 = releases/$(VERSION)/$(TOKEN_BINARY_ARM64)/$(FILE_BINARY_ARM64)
PKG_FEDORA-LATEST = releases/$(VERSION)/$(TOKEN_FEDORA-LATEST)/$(FILE_FEDORA-LATEST)
PKG_FEDORA-OLDER = releases/$(VERSION)/$(TOKEN_FEDORA-OLDER)/$(FILE_FEDORA-OLDER)
PKG_STREAM-LATEST = releases/$(VERSION)/$(TOKEN_STREAM-LATEST)/$(FILE_STREAM-LATEST)
PKG_DEBIAN-STABLE = releases/$(VERSION)/$(TOKEN_DEBIAN-STABLE)/$(FILE_DEBIAN-STABLE)
PKG_UBUNTU-LATEST = releases/$(VERSION)/$(TOKEN_UBUNTU-LATEST)/$(FILE_UBUNTU-LATEST)
PKG_ARCHLINUX = releases/$(VERSION)/$(TOKEN_ARCHLINUX)/$(FILE_ARCHLINUX)

DEP_BINARY_AMD64 =
ifneq ($(TOKEN_BINARY_AMD64),)
	DEP_BINARY_AMD64 = $(PKG_BINARY_AMD64)
endif
DEP_BINARY_ARM64 =
ifneq ($(TOKEN_BINARY_ARM64),)
	DEP_BINARY_ARM64 = $(PKG_BINARY_ARM64)
endif
DEP_FEDORA-LATEST =
ifneq ($(TOKEN_FEDORA-LATEST),)
	DEP_FEDORA-LATEST = $(PKG_FEDORA-LATEST)
endif
DEP_FEDORA-OLDER =
ifneq ($(TOKEN_FEDORA-OLDER),)
	DEP_FEDORA-OLDER = $(PKG_FEDORA-OLDER)
endif
DEP_STREAM-LATEST =
ifneq ($(TOKEN_STREAM-LATEST),)
	DEP_STREAM-LATEST = $(PKG_STREAM-LATEST)
endif
DEP_DEBIAN-STABLE =
ifneq ($(TOKEN_DEBIAN-STABLE),)
	DEP_DEBIAN-STABLE = $(PKG_DEBIAN-STABLE)
endif
DEP_UBUNTU-LATEST =
ifneq ($(TOKEN_UBUNTU-LATEST),)
	DEP_UBUNTU-LATEST = $(PKG_UBUNTU-LATEST)
endif
DEP_ARCHLINUX =
ifneq ($(TOKEN_ARCHLINUX),)
	DEP_ARCHLINUX = $(PKG_ARCHLINUX)
endif

SHA256SUMS = releases/$(VERSION)/SHA256SUMS
SHA256SUMS_ASC = $(SHA256SUMS).asc

SHA256SUMS_MASTER = releases/master/SHA256SUMS
SHA256SUMS_MASTER_ASC = $(SHA256SUMS_MASTER).asc

default: build

#
#	art
#
art: art/mgmt_logo_default_symbol.png art/mgmt_logo_default_tall.png art/mgmt_logo_default_wide.png art/mgmt_logo_reversed_symbol.png art/mgmt_logo_reversed_tall.png art/mgmt_logo_reversed_wide.png art/mgmt_logo_white_symbol.png art/mgmt_logo_white_tall.png art/mgmt_logo_white_wide.png ## generate artwork

cleanart:
	rm -f art/mgmt_logo_default_symbol.png art/mgmt_logo_default_tall.png art/mgmt_logo_default_wide.png art/mgmt_logo_reversed_symbol.png art/mgmt_logo_reversed_tall.png art/mgmt_logo_reversed_wide.png art/mgmt_logo_white_symbol.png art/mgmt_logo_white_tall.png art/mgmt_logo_white_wide.png

# NOTE: the widths are arbitrary
art/mgmt_logo_default_symbol.png: art/mgmt_logo_default_symbol.svg
	inkscape --export-background='#ffffff' --without-gui --export-png "$@" --export-width 300 $(@:png=svg)

art/mgmt_logo_default_tall.png: art/mgmt_logo_default_tall.svg
	inkscape --export-background='#ffffff' --without-gui --export-png "$@" --export-width 400 $(@:png=svg)

art/mgmt_logo_default_wide.png: art/mgmt_logo_default_wide.svg
	inkscape --export-background='#ffffff' --without-gui --export-png "$@" --export-width 800 $(@:png=svg)

art/mgmt_logo_reversed_symbol.png: art/mgmt_logo_reversed_symbol.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 300 $(@:png=svg)

art/mgmt_logo_reversed_tall.png: art/mgmt_logo_reversed_tall.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 400 $(@:png=svg)

art/mgmt_logo_reversed_wide.png: art/mgmt_logo_reversed_wide.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 800 $(@:png=svg)

art/mgmt_logo_white_symbol.png: art/mgmt_logo_white_symbol.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 300 $(@:png=svg)

art/mgmt_logo_white_tall.png: art/mgmt_logo_white_tall.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 400 $(@:png=svg)

art/mgmt_logo_white_wide.png: art/mgmt_logo_white_wide.svg
	inkscape --export-background='#231f20' --without-gui --export-png "$@" --export-width 800 $(@:png=svg)

all: docs $(PROGRAM).static

# show the current version
version: ## show the current version
	@echo $(VERSION)

program: ## show the program name
	@echo $(PROGRAM)

path: ## create working paths
	./misc/make-path.sh

deps: ## install system and golang dependencies
	./misc/make-deps.sh

generate:
	go generate

lang: ## generates the lexer/parser for the language frontend
	@# recursively run make in child dir named lang
	@$(MAKE) --quiet -C lang

resources: ## builds the resources dependencies required for the engine backend
	@# recursively run make in child dir named engine/resources
	@$(MAKE) --quiet -C engine/resources

# build a `mgmt` binary for current host os/arch
$(PROGRAM): build/mgmt-${GOHOSTOS}-${GOHOSTARCH} ## build an mgmt binary for current host os/arch
	cp -a $< $@

$(PROGRAM).static: $(GO_FILES) $(MCL_FILES) $(MISC_FILES) go.mod go.sum
	@echo "Building: $(PROGRAM).static, version: $(SVERSION)..."
	go generate
	go build $(TRIMPATH) -a -installsuffix cgo -tags netgo -ldflags '-extldflags "-static" -X main.program=$(PROGRAM) -X main.version=$(SVERSION) -s -w' -o $(PROGRAM).static $(BUILD_FLAGS);

build: LDFLAGS=-s -w ## build a fresh mgmt binary
build: $(PROGRAM)

build-debug: LDFLAGS=
build-debug: $(PROGRAM)

# if you're using the bad/dev branch, you might want this too!
baddev: BUILD_FLAGS = -tags 'noaugeas novirt'
baddev: $(PROGRAM)

# pattern rule target for (cross)building, mgmt-OS-ARCH will be expanded to the correct build
# extract os and arch from target pattern
GOOS=$(firstword $(subst -, ,$*))
GOARCH=$(lastword $(subst -, ,$*))
build/mgmt-%: $(GO_FILES) $(MCL_FILES) $(MISC_FILES) go.mod go.sum | lang resources funcgen
	@# If you need to run `go mod tidy` then this can trigger.
	@if [ "$(PKGNAME)" = "" ]; then echo "\$$(PKGNAME) is empty, test with: go list ."; exit 42; fi
	@echo "Building: $(PROGRAM), os/arch: $*, version: $(SVERSION)..."
	@# XXX: leave race detector on by default for now. For production
	@# builds, we can consider turning it off for performance improvements.
	@# XXX: ./mgmt run --tmp-prefix lang something_fast.mcl > /tmp/race 2>&1 # search for "WARNING: DATA RACE"
	time env GOOS=${GOOS} GOARCH=${GOARCH} go build $(TRIMPATH) $(GOLANGRACE) -ldflags=$(PKGNAME)="-X main.program=$(PROGRAM) -X main.version=$(SVERSION) ${LDFLAGS}" -o $@ $(BUILD_FLAGS)

# create a list of binary file names to use as make targets
# to use this you might want to run something like:
# GOOSARCHES='linux/arm64' GOTAGS='noaugeas novirt' make crossbuild
# and the output will end up in build/
crossbuild_targets = $(addprefix build/mgmt-,$(subst /,-,${GOOSARCHES}))
crossbuild: ${crossbuild_targets}

clean: ## clean things up
	$(MAKE) --quiet -C test clean
	$(MAKE) --quiet -C lang clean
	$(MAKE) --quiet -C engine/resources clean
	$(MAKE) --quiet -C misc/mkosi clean
	rm -f lang/core/generated_funcs.go || true
	rm -f lang/core/generated_funcs_test.go || true
	[ ! -e $(PROGRAM) ] || rm $(PROGRAM)
	rm -f *_stringer.go	# generated by `go generate`
	rm -f *_mock.go		# generated by `go generate`
	# crossbuild artifacts
	rm -f build/mgmt-*

test: build ## run tests
	@# recursively run make in child dir named test
	@$(MAKE) --quiet -C test
	./test.sh

# create all test targets for make tab completion (eg: make test-gofmt)
test_suites=$(shell find test/ -maxdepth 1 -name test-* -exec basename {} .sh \;)
# allow to run only one test suite at a time
${test_suites}: test-%: build
	./test.sh $*

# targets to run individual shell tests (eg: make test-shell-load0)
test_shell=$(shell find test/shell/ -maxdepth 1 -name "*.sh" -exec basename {} .sh \;)
$(addprefix test-shell-,${test_shell}): test-shell-%: build
	./test/test-shell.sh "$*.sh"

gofmt:
	# TODO: remove gofmt once goimports has a -s option
	find . -maxdepth 9 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -not -path './vendor/*' -exec gofmt -s -w {} \;
	find . -maxdepth 9 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -not -path './vendor/*' -exec goimports -w {} \;

yamlfmt:
	find . -maxdepth 3 -type f -name '*.yaml' -not -path './old/*' -not -path './tmp/*' -not -path './omv.yaml' -exec ruby -e "require 'yaml'; x=YAML.load_file('{}').to_yaml.each_line.map(&:rstrip).join(10.chr)+10.chr; File.open('{}', 'w').write x" \;

format: gofmt yamlfmt ## format yaml and golang code

docs: $(PROGRAM)-documentation.pdf ## generate docs

$(PROGRAM)-documentation.pdf: docs/documentation.md
	pandoc docs/documentation.md -o docs/'$(PROGRAM)-documentation.pdf'

#
#	build aliases
#
# TODO: does making an rpm depend on making a .srpm first ?
rpm: $(SRPM) $(RPM)
	# do nothing

srpm: $(SRPM)
	# do nothing

spec: $(SPEC)
	# do nothing

tar: $(SOURCE)
	# do nothing

rpmbuild/SOURCES/: tar
rpmbuild/SRPMS/: srpm
rpmbuild/RPMS/: rpm

upload: upload-sources upload-srpms upload-rpms ## upload sources
	# do nothing

#
#	rpmbuild
#
$(RPM): $(SPEC) $(SOURCE)
	@echo Running rpmbuild -bb...
	rpmbuild --define '_topdir $(shell pwd)/rpmbuild' -bb $(SPEC) && \
	mv rpmbuild/RPMS/$(ARCH)/$(PROGRAM)-$(VERSION)-$(RELEASE).*.rpm $(RPM)

$(SRPM): $(SPEC) $(SOURCE)
	@echo Running rpmbuild -bs...
	rpmbuild --define '_topdir $(shell pwd)/rpmbuild' -bs $(SPEC)
	# renaming is not needed because we aren't using the dist variable
	#mv rpmbuild/SRPMS/$(PROGRAM)-$(VERSION)-$(RELEASE).*.src.rpm $(SRPM)

#
#	spec
#
$(SPEC): rpmbuild/ spec.in
	@echo Running templater...
	cat spec.in > $(SPEC)
	sed -e s/__PROGRAM__/$(PROGRAM)/g -e s/__VERSION__/$(VERSION)/g -e s/__RELEASE__/$(RELEASE)/g < spec.in > $(SPEC)
	# append a changelog to the .spec file
	git log --format="* %cd %aN <%aE>%n- (%h) %s%d%n" --date=local | sed -r 's/[0-9]+:[0-9]+:[0-9]+ //' >> $(SPEC)

#
#	archive
#
$(SOURCE): rpmbuild/
	@echo Running git archive...
	# use HEAD if tag doesn't exist yet, so that development is easier...
	git archive --prefix=$(PROGRAM)-$(VERSION)/ -o $(SOURCE) $(VERSION) 2> /dev/null || (echo 'Warning: $(VERSION) does not exist. Using HEAD instead.' && git archive --prefix=$(PROGRAM)-$(VERSION)/ -o $(SOURCE) HEAD)
	# TODO: if git archive had a --submodules flag this would easier!
	@echo Running git archive submodules...
	# i thought i would need --ignore-zeros, but it doesn't seem necessary!
	p=`pwd` && (echo .; git submodule foreach) | while read entering path; do \
		temp="$${path%\'}"; \
		temp="$${temp#\'}"; \
		path=$$temp; \
		[ "$$path" = "" ] && continue; \
		(cd $$path && git archive --prefix=$(PROGRAM)-$(VERSION)/$$path/ HEAD > $$p/rpmbuild/tmp.tar && tar --concatenate --file=$$p/$(SOURCE) $$p/rpmbuild/tmp.tar && rm $$p/rpmbuild/tmp.tar); \
	done

# TODO: ensure that each sub directory exists
rpmbuild/:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

mkdirs:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

#
#	sha256sum
#
rpmbuild/SOURCES/SHA256SUMS: rpmbuild/SOURCES/ $(SOURCE)
	@echo Running SOURCES sha256sum...
	cd rpmbuild/SOURCES/ && sha256sum *.tar.bz2 > SHA256SUMS; cd -

rpmbuild/SRPMS/SHA256SUMS: rpmbuild/SRPMS/ $(SRPM)
	@echo Running SRPMS sha256sum...
	cd rpmbuild/SRPMS/ && sha256sum *src.rpm > SHA256SUMS; cd -

rpmbuild/RPMS/SHA256SUMS: rpmbuild/RPMS/ $(RPM)
	@echo Running RPMS sha256sum...
	cd rpmbuild/RPMS/ && sha256sum *.rpm > SHA256SUMS; cd -

#
#	gpg
#
rpmbuild/SOURCES/SHA256SUMS.asc: rpmbuild/SOURCES/SHA256SUMS
	@echo Running SOURCES gpg...
	# the --yes forces an overwrite of the SHA256SUMS.asc if necessary
	gpg2 --yes --clearsign rpmbuild/SOURCES/SHA256SUMS

rpmbuild/SRPMS/SHA256SUMS.asc: rpmbuild/SRPMS/SHA256SUMS
	@echo Running SRPMS gpg...
	gpg2 --yes --clearsign rpmbuild/SRPMS/SHA256SUMS

rpmbuild/RPMS/SHA256SUMS.asc: rpmbuild/RPMS/SHA256SUMS
	@echo Running RPMS gpg...
	gpg2 --yes --clearsign rpmbuild/RPMS/SHA256SUMS

#
#	upload
#
# upload to public server
upload-sources: rpmbuild/SOURCES/ rpmbuild/SOURCES/SHA256SUMS rpmbuild/SOURCES/SHA256SUMS.asc
	if [ "`cat rpmbuild/SOURCES/SHA256SUMS`" != "`ssh $(SERVER) 'cd $(REMOTE_PATH)/SOURCES/ && cat SHA256SUMS'`" ]; then \
		echo Running SOURCES upload...; \
		rsync -avz rpmbuild/SOURCES/ $(SERVER):$(REMOTE_PATH)/SOURCES/; \
	fi

upload-srpms: rpmbuild/SRPMS/ rpmbuild/SRPMS/SHA256SUMS rpmbuild/SRPMS/SHA256SUMS.asc
	if [ "`cat rpmbuild/SRPMS/SHA256SUMS`" != "`ssh $(SERVER) 'cd $(REMOTE_PATH)/SRPMS/ && cat SHA256SUMS'`" ]; then \
		echo Running SRPMS upload...; \
		rsync -avz rpmbuild/SRPMS/ $(SERVER):$(REMOTE_PATH)/SRPMS/; \
	fi

upload-rpms: rpmbuild/RPMS/ rpmbuild/RPMS/SHA256SUMS rpmbuild/RPMS/SHA256SUMS.asc
	if [ "`cat rpmbuild/RPMS/SHA256SUMS`" != "`ssh $(SERVER) 'cd $(REMOTE_PATH)/RPMS/ && cat SHA256SUMS'`" ]; then \
		echo Running RPMS upload...; \
		rsync -avz --prune-empty-dirs rpmbuild/RPMS/ $(SERVER):$(REMOTE_PATH)/RPMS/; \
	fi

upload-releases:
	echo Running releases/ upload...
	rsync -avz --exclude '.mkdir' --exclude 'mgmt-release.url' releases/ $(SERVER):$(REMOTE_PATH)/releases/

#
#	copr build
#
copr: upload-srpms ## build in copr
	./misc/copr-build.py https://$(SERVER)/$(REMOTE_PATH)/SRPMS/$(SRPM_BASE)

#
#	tag
#
tag: ## tags a new release
	./misc/tag.sh

#
#	mkosi
#
mkosi: mkosi_fedora-latest mkosi_fedora-older mkosi_stream-latest mkosi_debian-stable mkosi_ubuntu-latest mkosi_archlinux ## builds distro packages via mkosi

mkosi_fedora-latest: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_fedora-older: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_stream-latest: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_debian-stable: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_ubuntu-latest: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_archlinux: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

#
#	release master
#
.PHONY: release_master
release_master: TRIMPATH = -trimpath
release_master: GOLANGRACE =
release_master: VERSION = master
release_master: $(SHA256SUMS_MASTER_ASC) ## makes and uploads a master release
	@#echo SVERSION: $(SVERSION)
	@#echo VERSION: $(VERSION)
	./misc/fpm-repo.sh --master --version $(SVERSION)

$(SHA256SUMS_MASTER_ASC): $(SHA256SUMS_MASTER)
	@echo "Signing sha256 sum..."
	@gpg2 --yes --clearsign $(SHA256SUMS_MASTER)

#$(SHA256SUMS_MASTER): $(PKG_BINARY_AMD64_MASTER) $(PKG_BINARY_ARM64_MASTER)
$(SHA256SUMS_MASTER): $(PKG_BINARY_AMD64_MASTER)
	@# remove the directory separator in the SHA256SUMS file
	@echo "Generating: sha256 sum..."
	@sha256sum \
	` [ -e $(PKG_BINARY_AMD64_MASTER) ] && printf -- "$(PKG_BINARY_AMD64_MASTER)" ` \
	` [ -e $(PKG_BINARY_ARM64_MASTER) ] && printf -- "$(PKG_BINARY_ARM64_MASTER)" ` \
	| awk -F '/| ' '{print $$1"  "$$6}' > $(SHA256SUMS_MASTER)

$(PKG_BINARY_AMD64_MASTER): build/mgmt-linux-amd64
	@mkdir -p $$(dirname $(PKG_BINARY_AMD64_MASTER))
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; cp -a build/mgmt-linux-amd64 $(PKG_BINARY_AMD64_MASTER)

# XXX: requires magic to cross-compile go-libvirt for arm64, so figure this out later...
$(PKG_BINARY_ARM64_MASTER): build/mgmt-linux-arm64
	@mkdir -p $$(dirname $(PKG_BINARY_ARM64_MASTER))
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; cp -a build/mgmt-linux-arm64 $(PKG_BINARY_ARM64_MASTER)

#
#	release
#
release: TRIMPATH = -trimpath
release: releases/$(VERSION)/mgmt-release.url ## generates and uploads a release

releases_path:
	@#Don't put any other output or dependencies in here or they'll show!
	@echo "releases/$(VERSION)/"

release_test: $(DEP_BINARY_AMD64) $(DEP_BINARY_ARM64) $(DEP_FEDORA-LATEST) $(DEP_FEDORA-OLDER) $(DEP_STREAM-LATEST) $(DEP_DEBIAN-STABLE) $(DEP_UBUNTU-LATEST) $(DEP_ARCHLINUX) $(SHA256SUMS_ASC)
	@echo '$$< denotes ‘the first dependency of the current rule’.'
	@echo '> '"$<"
	@echo
	@echo '$$@ denotes ‘the target of the current rule’.'
	@echo '> '"$@"
	@echo
	@echo '$$^ denotes ‘the dependencies of the current rule’.'
	@echo '> '"$^"
	@echo
	@echo '$$* denotes ‘the stem with which the pattern of the current rule matched’.'
	@echo '> '"$*"
	@echo
	@echo "TOKEN_BINARY_AMD64: $(TOKEN_BINARY_AMD64)"
	@echo "DEP_BINARY_AMD64: $(DEP_BINARY_AMD64)"
	@echo
	@echo "TOKEN_BINARY_ARM64: $(TOKEN_BINARY_ARM64)"
	@echo "DEP_BINARY_ARM64: $(DEP_BINARY_ARM64)"
	@echo
	@echo "TOKEN_FEDORA-LATEST: $(TOKEN_FEDORA-LATEST)"
	@echo "DEP_FEDORA-LATEST: $(DEP_FEDORA-LATEST)"
	@echo
	@echo "TOKEN_FEDORA-OLDER: $(TOKEN_FEDORA-OLDER)"
	@echo "DEP_FEDORA-OLDER: $(DEP_FEDORA-OLDER)"
	@echo
	@echo "TOKEN_STREAM-LATEST: $(TOKEN_STREAM-LATEST)"
	@echo "DEP_STREAM-LATEST: $(DEP_STREAM-LATEST)"
	@echo
	@echo "TOKEN_DEBIAN-STABLE: $(TOKEN_DEBIAN-STABLE)"
	@echo "DEP_DEBIAN-STABLE: $(DEP_DEBIAN-STABLE)"
	@echo
	@echo "TOKEN_UBUNTU-LATEST: $(TOKEN_UBUNTU-LATEST)"
	@echo "DEP_UBUNTU-LATEST: $(DEP_UBUNTU-LATEST)"
	@echo
	@echo "TOKEN_ARCHLINUX: $(TOKEN_ARCHLINUX)"
	@echo "DEP_ARCHLINUX: $(DEP_ARCHLINUX)"

release_binary_amd64: $(PKG_BINARY_AMD64)
release_binary_arm64: $(PKG_BINARY_ARM64)
release_fedora-latest: $(PKG_FEDORA-LATEST)
release_fedora-older: $(PKG_FEDORA-OLDER)
release_stream-latest: $(PKG_STREAM-LATEST)
release_debian-stable: $(PKG_DEBIAN-STABLE)
release_ubuntu-latest: $(PKG_UBUNTU-LATEST)
release_archlinux: $(PKG_ARCHLINUX)

releases/$(VERSION)/mgmt-release.url: $(DEP_BINARY_AMD64) $(DEP_BINARY_ARM64) $(DEP_FEDORA-LATEST) $(DEP_FEDORA-OLDER) $(DEP_STREAM-LATEST) $(DEP_DEBIAN-STABLE) $(DEP_UBUNTU-LATEST) $(DEP_ARCHLINUX) $(SHA256SUMS_ASC)
	@echo "Pushing git tag $(VERSION) to origin..."
	git push origin $(VERSION)
	@echo "Creating github release..."
	hub release create \
		-F <( echo -e "$(VERSION)\n";echo "Verify the signatures of all packages before you use them. The signing key can be downloaded from https://purpleidea.com/contact/#pgp-key to verify the release." ) \
		` [ -e $(PKG_BINARY_AMD64) ] && printf -- "-a $(PKG_BINARY_AMD64)" ` \
		` [ -e $(PKG_BINARY_ARM64) ] && printf -- "-a $(PKG_BINARY_ARM64)" ` \
		` [ -e $(PKG_FEDORA-LATEST) ] && printf -- "-a $(PKG_FEDORA-LATEST)" ` \
		` [ -e $(PKG_FEDORA-OLDER) ] && printf -- "-a $(PKG_FEDORA-OLDER)" ` \
		` [ -e $(PKG_STREAM-LATEST) ] && printf -- "-a $(PKG_STREAM-LATEST)" ` \
		` [ -e $(PKG_DEBIAN-STABLE) ] && printf -- "-a $(PKG_DEBIAN-STABLE)" ` \
		` [ -e $(PKG_UBUNTU-LATEST) ] && printf -- "-a $(PKG_UBUNTU-LATEST)" ` \
		` [ -e $(PKG_ARCHLINUX) ] && printf -- "-a $(PKG_ARCHLINUX)" ` \
		-a $(SHA256SUMS_ASC) \
		$(VERSION) \
		> releases/$(VERSION)/mgmt-release.url \
		&& cat releases/$(VERSION)/mgmt-release.url \
		|| rm -f releases/$(VERSION)/mgmt-release.url

releases/$(VERSION)/.mkdir:
	mkdir -p \
	` [ "$(TOKEN_BINARY_AMD64)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_BINARY_AMD64)/" ` \
	` [ "$(TOKEN_BINARY_ARM64)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_BINARY_ARM64)/" ` \
	` [ "$(TOKEN_FEDORA-LATEST)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_FEDORA-LATEST)/" ` \
	` [ "$(TOKEN_FEDORA-OLDER)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_FEDORA-OLDER)/" ` \
	` [ "$(TOKEN_STREAM-LATEST)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_STREAM-LATEST)/" ` \
	` [ "$(TOKEN_DEBIAN-STABLE)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_DEBIAN-STABLE)/" ` \
	` [ "$(TOKEN_UBUNTU-LATEST)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_UBUNTU-LATEST)/" ` \
	` [ "$(TOKEN_ARCHLINUX)" != "" ] && printf -- "releases/$(VERSION)/$(TOKEN_ARCHLINUX)/" ` \
	&& touch releases/$(VERSION)/.mkdir

# These are defined conditionally, since if the token is empty, they warn!
ifneq ($(TOKEN_BINARY_AMD64),)
$(PKG_BINARY_AMD64): build/mgmt-linux-amd64 releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; cp -a build/mgmt-linux-amd64 $(PKG_BINARY_AMD64)
endif
ifneq ($(TOKEN_BINARY_ARM64),)
$(PKG_BINARY_ARM64): build/mgmt-linux-arm64 releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; cp -a build/mgmt-linux-arm64 $(PKG_BINARY_ARM64)
endif
ifneq ($(TOKEN_FEDORA-LATEST),)
releases/$(VERSION)/$(TOKEN_FEDORA-LATEST)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_FEDORA-LATEST): releases/$(VERSION)/$(TOKEN_FEDORA-LATEST)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_FEDORA-LATEST)" libvirt-devel augeas-devel
endif
ifneq ($(TOKEN_FEDORA-OLDER),)
releases/$(VERSION)/$(TOKEN_FEDORA-OLDER)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_FEDORA-OLDER): releases/$(VERSION)/$(TOKEN_FEDORA-OLDER)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_FEDORA-OLDER)" libvirt-devel augeas-devel
endif
ifneq ($(TOKEN_STREAM-LATEST),)
releases/$(VERSION)/$(TOKEN_STREAM-LATEST)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_STREAM-LATEST): releases/$(VERSION)/$(TOKEN_STREAM-LATEST)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_STREAM-LATEST)" libvirt-devel augeas-devel
endif
ifneq ($(TOKEN_DEBIAN-STABLE),)
releases/$(VERSION)/$(TOKEN_DEBIAN-STABLE)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-deb-changelog.sh "$${distro}" $(VERSION)

$(PKG_DEBIAN-STABLE): releases/$(VERSION)/$(TOKEN_DEBIAN-STABLE)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_DEBIAN-STABLE)" libvirt-dev libaugeas-dev
endif
ifneq ($(TOKEN_UBUNTU-LATEST),)
releases/$(VERSION)/$(TOKEN_UBUNTU-LATEST)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-deb-changelog.sh "$${distro}" $(VERSION)

$(PKG_UBUNTU-LATEST): releases/$(VERSION)/$(TOKEN_UBUNTU-LATEST)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_UBUNTU-LATEST)" libvirt-dev libaugeas-dev
endif
ifneq ($(TOKEN_ARCHLINUX),)
$(PKG_ARCHLINUX): $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_ARCHLINUX)" libvirt augeas
endif

$(SHA256SUMS): $(DEP_BINARY_AMD64) $(DEP_BINARY_ARM64) $(DEP_FEDORA-LATEST) $(DEP_FEDORA-OLDER) $(DEP_STREAM-LATEST) $(DEP_DEBIAN-STABLE) $(DEP_UBUNTU-LATEST) $(DEP_ARCHLINUX)
	@# remove the directory separator in the SHA256SUMS file
	@echo "Generating: sha256 sum..."
	sha256sum \
	` [ -e $(PKG_BINARY_AMD64) ] && printf -- "$(PKG_BINARY_AMD64)" ` \
	` [ -e $(PKG_BINARY_ARM64) ] && printf -- "$(PKG_BINARY_ARM64)" ` \
	` [ -e $(PKG_FEDORA-LATEST) ] && printf -- "$(PKG_FEDORA-LATEST)" ` \
	` [ -e $(PKG_FEDORA-OLDER) ] && printf -- "$(PKG_FEDORA-OLDER)" ` \
	` [ -e $(PKG_STREAM-LATEST) ] && printf -- "$(PKG_STREAM-LATEST)" ` \
	` [ -e $(PKG_DEBIAN-STABLE) ] && printf -- "$(PKG_DEBIAN-STABLE)" ` \
	` [ -e $(PKG_UBUNTU-LATEST) ] && printf -- "$(PKG_UBUNTU-LATEST)" ` \
	` [ -e $(PKG_ARCHLINUX) ] && printf -- "$(PKG_ARCHLINUX)" ` \
	| awk -F '/| ' '{print $$1"  "$$6}' > $(SHA256SUMS)

$(SHA256SUMS_ASC): $(SHA256SUMS)
	@echo "Signing sha256 sum..."
	gpg2 --yes --clearsign $(SHA256SUMS)

build_container: ## builds the container
	docker build -t purpleidea/mgmt-build -f docker/Dockerfile.build .
	docker run -td --name mgmt-build purpleidea/mgmt-build
	docker cp mgmt-build:/root/gopath/src/github.com/purpleidea/mgmt/mgmt .
	docker build -t purpleidea/mgmt -f docker/Dockerfile.static .
	docker rm mgmt-build || true

clean_container: ## removes the container
	docker rmi purpleidea/mgmt-build
	docker rmi purpleidea/mgmt

help: ## show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ''

funcgen: lang/core/generated_funcs.go

lang/core/generated_funcs.go: lang/funcs/funcgen/*.go lang/core/funcgen.yaml lang/funcs/funcgen/templates/generated_funcs.go.tpl
	@echo "Generating: funcs..."
	@go run `find lang/funcs/funcgen/ -maxdepth 1 -type f -name '*.go' -not -name '*_test.go'` -templates=lang/funcs/funcgen/templates/generated_funcs.go.tpl >/dev/null
	@gofmt -s -w $@

# vim: ts=8
