# Mgmt
# Copyright (C) 2013-2019+ James Shubin and the project contributors
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
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

SHELL = /usr/bin/env bash
.PHONY: all art cleanart version program lang path deps run race bindata generate build build-debug crossbuild clean test gofmt yamlfmt format docs
.PHONY: rpmbuild mkdirs rpm srpm spec tar upload upload-sources upload-srpms upload-rpms upload-releases copr tag
.PHONY: mkosi mkosi_fedora-30 mkosi_fedora-29 mkosi_centos-7 mkosi_debian-10 mkosi_ubuntu-bionic mkosi_archlinux
.PHONY: release releases_path release_fedora-30 release_fedora-29 release_centos-7 release_debian-10 release_ubuntu-bionic release_archlinux
.PHONY: funcgen
.SILENT: clean bindata

# a large amount of output from this `find`, can cause `make` to be much slower!
GO_FILES := $(shell find * -name '*.go' -not -path 'old/*' -not -path 'tmp/*')
MCL_FILES := $(shell find lang/funcs/ -name '*.mcl' -not -path 'old/*' -not -path 'tmp/*')

SVERSION := $(or $(SVERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --dirty --always))
VERSION := $(or $(VERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --abbrev=0))
PROGRAM := $(shell echo $(notdir $(CURDIR)) | cut -f1 -d"-")
PKGNAME := $(shell go list .)
ifeq ($(VERSION),$(SVERSION))
	RELEASE = 1
else
	RELEASE = untagged
endif
ARCH = $(uname -m)
SPEC = rpmbuild/SPECS/$(PROGRAM).spec
SOURCE = rpmbuild/SOURCES/$(PROGRAM)-$(VERSION).tar.bz2
SRPM = rpmbuild/SRPMS/$(PROGRAM)-$(VERSION)-$(RELEASE).src.rpm
SRPM_BASE = $(PROGRAM)-$(VERSION)-$(RELEASE).src.rpm
RPM = rpmbuild/RPMS/$(PROGRAM)-$(VERSION)-$(RELEASE).$(ARCH).rpm
USERNAME := $(shell cat ~/.config/copr 2>/dev/null | grep username | awk -F '=' '{print $$2}' | tr -d ' ')
SERVER = 'dl.fedoraproject.org'
REMOTE_PATH = 'pub/alt/$(USERNAME)/$(PROGRAM)'
ifneq ($(GOTAGS),)
	BUILD_FLAGS = -tags '$(GOTAGS)'
endif
GOOSARCHES ?= linux/amd64 linux/ppc64 linux/ppc64le linux/arm64 darwin/amd64

GOHOSTOS = $(shell go env GOHOSTOS)
GOHOSTARCH = $(shell go env GOHOSTARCH)

TOKEN_FEDORA-30 = fedora-30
TOKEN_FEDORA-29 = fedora-29
TOKEN_CENTOS-7 = centos-7
TOKEN_DEBIAN-10 = debian-10
TOKEN_UBUNTU-BIONIC = ubuntu-bionic
TOKEN_ARCHLINUX = archlinux

FILE_FEDORA-30 = mgmt-$(TOKEN_FEDORA-30)-$(VERSION)-1.x86_64.rpm
FILE_FEDORA-29 = mgmt-$(TOKEN_FEDORA-29)-$(VERSION)-1.x86_64.rpm
FILE_CENTOS-7 = mgmt-$(TOKEN_CENTOS-7)-$(VERSION)-1.x86_64.rpm
FILE_DEBIAN-10 = mgmt_$(TOKEN_DEBIAN-10)_$(VERSION)_amd64.deb
FILE_UBUNTU-BIONIC = mgmt_$(TOKEN_UBUNTU-BIONIC)_$(VERSION)_amd64.deb
FILE_ARCHLINUX = mgmt-$(TOKEN_ARCHLINUX)-$(VERSION)-1-x86_64.pkg.tar.xz

PKG_FEDORA-30 = releases/$(VERSION)/$(TOKEN_FEDORA-30)/$(FILE_FEDORA-30)
PKG_FEDORA-29 = releases/$(VERSION)/$(TOKEN_FEDORA-29)/$(FILE_FEDORA-29)
PKG_CENTOS-7 = releases/$(VERSION)/$(TOKEN_CENTOS-7)/$(FILE_CENTOS-7)
PKG_DEBIAN-10 = releases/$(VERSION)/$(TOKEN_DEBIAN-10)/$(FILE_DEBIAN-10)
PKG_UBUNTU-BIONIC = releases/$(VERSION)/$(TOKEN_UBUNTU-BIONIC)/$(FILE_UBUNTU-BIONIC)
PKG_ARCHLINUX = releases/$(VERSION)/$(TOKEN_ARCHLINUX)/$(FILE_ARCHLINUX)

SHA256SUMS = releases/$(VERSION)/SHA256SUMS
SHA256SUMS_ASC = $(SHA256SUMS).asc

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

run: ## run mgmt
	find . -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION)"

# include race flag
race:
	find . -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -race -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION)"

# generate go files from non-go source
bindata: ## generate go files from non-go sources
	$(MAKE) --quiet -C bindata
	$(MAKE) --quiet -C lang/funcs

generate:
	go generate

lang: ## generates the lexer/parser for the language frontend
	@# recursively run make in child dir named lang
	@$(MAKE) --quiet -C lang

# build a `mgmt` binary for current host os/arch
$(PROGRAM): build/mgmt-${GOHOSTOS}-${GOHOSTARCH} ## build an mgmt binary for current host os/arch
	cp -a $< $@

$(PROGRAM).static: $(GO_FILES) $(MCL_FILES)
	@echo "Building: $(PROGRAM).static, version: $(SVERSION)..."
	go generate
	go build -a -installsuffix cgo -tags netgo -ldflags '-extldflags "-static" -X main.program=$(PROGRAM) -X main.version=$(SVERSION) -s -w' -o $(PROGRAM).static $(BUILD_FLAGS);

build: LDFLAGS=-s -w ## build a fresh mgmt binary
build: $(PROGRAM)

build-debug: LDFLAGS=
build-debug: $(PROGRAM)

# pattern rule target for (cross)building, mgmt-OS-ARCH will be expanded to the correct build
# extract os and arch from target pattern
GOOS=$(firstword $(subst -, ,$*))
GOARCH=$(lastword $(subst -, ,$*))
build/mgmt-%: $(GO_FILES) $(MCL_FILES) | bindata lang funcgen
	@echo "Building: $(PROGRAM), os/arch: $*, version: $(SVERSION)..."
	@# reassigning GOOS and GOARCH to make build command copy/pastable
	@# go 1.10+ requires specifying the package for ldflags
	@if go version | grep -qE 'go1.9'; then \
		time env GOOS=${GOOS} GOARCH=${GOARCH} go build -i -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION) ${LDFLAGS}" -o $@ $(BUILD_FLAGS); \
	else \
		time env GOOS=${GOOS} GOARCH=${GOARCH} go build -i -ldflags=$(PKGNAME)="-X main.program=$(PROGRAM) -X main.version=$(SVERSION) ${LDFLAGS}" -o $@ $(BUILD_FLAGS); \
	fi

# create a list of binary file names to use as make targets
crossbuild_targets = $(addprefix build/mgmt-,$(subst /,-,${GOOSARCHES}))
crossbuild: ${crossbuild_targets}

clean: ## clean things up
	$(MAKE) --quiet -C bindata clean
	$(MAKE) --quiet -C lang/funcs clean
	$(MAKE) --quiet -C lang clean
	$(MAKE) --quiet -C misc/mkosi clean
	rm -f lang/funcs/core/generated_funcs.go || true
	rm -f lang/funcs/core/generated_funcs_test.go || true
	[ ! -e $(PROGRAM) ] || rm $(PROGRAM)
	rm -f *_stringer.go	# generated by `go generate`
	rm -f *_mock.go		# generated by `go generate`
	# crossbuild artifacts
	rm -f build/mgmt-*

test: build ## run tests
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
mkosi: mkosi_fedora-30 mkosi_fedora-29 mkosi_centos-7 mkosi_debian-10 mkosi_ubuntu-bionic mkosi_archlinux ## builds distro packages via mkosi

mkosi_fedora-30: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_fedora-29: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_centos-7: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_debian-10: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_ubuntu-bionic: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

mkosi_archlinux: releases/$(VERSION)/.mkdir
	@title='$@' ; echo "Generating: $${title#'mkosi_'} via mkosi..."
	@title='$@' ; distro=$${title#'mkosi_'} ; ./misc/mkosi/make.sh $${distro} `realpath "releases/$(VERSION)/"`

#
#	release
#
release: releases/$(VERSION)/mgmt-release.url ## generates and uploads a release

releases_path:
	@#Don't put any other output or dependencies in here or they'll show!
	@echo "releases/$(VERSION)/"

release_fedora-30: $(PKG_FEDORA-30)
release_fedora-29: $(PKG_FEDORA-29)
release_centos-7: $(PKG_CENTOS-7)
release_debian-10: $(PKG_DEBIAN-10)
release_ubuntu-bionic: $(PKG_UBUNTU-BIONIC)
release_archlinux: $(PKG_ARCHLINUX)

releases/$(VERSION)/mgmt-release.url: $(PKG_FEDORA-30) $(PKG_FEDORA-29) $(PKG_CENTOS-7) $(PKG_DEBIAN-10) $(PKG_UBUNTU-BIONIC) $(PKG_ARCHLINUX) $(SHA256SUMS_ASC)
	@echo "Pushing git tag $(VERSION) to origin..."
	git push origin $(VERSION)
	@echo "Creating github release..."
	hub release create \
		-F <( echo -e "$(VERSION)\n";echo "Verify the signatures of all packages before you use them. The signing key can be downloaded from https://purpleidea.com/contact/#pgp-key to verify the release." ) \
		-a $(PKG_FEDORA-30) \
		-a $(PKG_FEDORA-29) \
		-a $(PKG_CENTOS-7) \
		-a $(PKG_DEBIAN-10) \
		-a $(PKG_UBUNTU-BIONIC) \
		-a $(PKG_ARCHLINUX) \
		-a $(SHA256SUMS_ASC) \
		$(VERSION) \
		> releases/$(VERSION)/mgmt-release.url \
		&& cat releases/$(VERSION)/mgmt-release.url \
		|| rm -f releases/$(VERSION)/mgmt-release.url

releases/$(VERSION)/.mkdir:
	mkdir -p releases/$(VERSION)/{$(TOKEN_FEDORA-30),$(TOKEN_FEDORA-29),$(TOKEN_CENTOS-7),$(TOKEN_DEBIAN-10),$(TOKEN_UBUNTU-BIONIC),$(TOKEN_ARCHLINUX)}/ && touch releases/$(VERSION)/.mkdir

releases/$(VERSION)/$(TOKEN_FEDORA-30)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_FEDORA-30): releases/$(VERSION)/$(TOKEN_FEDORA-30)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_FEDORA-30)" libvirt-devel augeas-devel

releases/$(VERSION)/$(TOKEN_FEDORA-29)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_FEDORA-29): releases/$(VERSION)/$(TOKEN_FEDORA-29)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_FEDORA-29)" libvirt-devel augeas-devel

releases/$(VERSION)/$(TOKEN_CENTOS-7)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-rpm-changelog.sh "$${distro}" $(VERSION)

$(PKG_CENTOS-7): releases/$(VERSION)/$(TOKEN_CENTOS-7)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_CENTOS-7)" libvirt-devel augeas-devel

releases/$(VERSION)/$(TOKEN_DEBIAN-10)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-deb-changelog.sh "$${distro}" $(VERSION)

$(PKG_DEBIAN-10): releases/$(VERSION)/$(TOKEN_DEBIAN-10)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_DEBIAN-10)" libvirt-dev libaugeas-dev

releases/$(VERSION)/$(TOKEN_UBUNTU-BIONIC)/changelog: $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Generating: $${distro} changelog..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/make-deb-changelog.sh "$${distro}" $(VERSION)

$(PKG_UBUNTU-BIONIC): releases/$(VERSION)/$(TOKEN_UBUNTU-BIONIC)/changelog
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_UBUNTU-BIONIC)" libvirt-dev libaugeas-dev

$(PKG_ARCHLINUX): $(PROGRAM) releases/$(VERSION)/.mkdir
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; echo "Building: $${distro} package..."
	@title='$(@D)' ; distro=$${title#'releases/$(VERSION)/'} ; ./misc/fpm-pack.sh $${distro} $(VERSION) "$(FILE_ARCHLINUX)" libvirt augeas

$(SHA256SUMS): $(PKG_FEDORA-30) $(PKG_FEDORA-29) $(PKG_CENTOS-7) $(PKG_DEBIAN-10) $(PKG_UBUNTU-BIONIC) $(PKG_ARCHLINUX)
	@# remove the directory separator in the SHA256SUMS file
	@echo "Generating: sha256 sum..."
	sha256sum $(PKG_FEDORA-30) $(PKG_FEDORA-29) $(PKG_CENTOS-7) $(PKG_DEBIAN-10) $(PKG_UBUNTU-BIONIC) $(PKG_ARCHLINUX) | awk -F '/| ' '{print $$1"  "$$6}' > $(SHA256SUMS)

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

funcgen: lang/funcs/core/generated_funcs_test.go lang/funcs/core/generated_funcs.go

lang/funcs/core/generated_funcs_test.go: lang/funcs/funcgen/*.go lang/funcs/core/funcgen.yaml lang/funcs/funcgen/templates/generated_funcs_test.go.tpl
	@echo "Generating: funcs test..."
	@go run lang/funcs/funcgen/*.go -templates lang/funcs/funcgen/templates/generated_funcs_test.go.tpl 2>/dev/null

lang/funcs/core/generated_funcs.go: lang/funcs/funcgen/*.go lang/funcs/core/funcgen.yaml lang/funcs/funcgen/templates/generated_funcs.go.tpl
	@echo "Generating: funcs..."
	@go run lang/funcs/funcgen/*.go -templates lang/funcs/funcgen/templates/generated_funcs.go.tpl 2>/dev/null

# vim: ts=8
