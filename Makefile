# Mgmt
# Copyright (C) 2013-2017+ James Shubin and the project contributors
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
.PHONY: all art cleanart version program path deps run race generate build clean test gofmt yamlfmt format docs rpmbuild mkdirs rpm srpm spec tar upload upload-sources upload-srpms upload-rpms copr
.SILENT: clean

GO_FILES := $(shell find . -name '*.go')
SVERSION := $(or $(SVERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --dirty --always))
VERSION := $(or $(VERSION),$(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --abbrev=0))
PROGRAM := $(shell echo $(notdir $(CURDIR)) | cut -f1 -d"-")
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

default: build

#
#	art
#
art: art/mgmt_logo_default_symbol.png art/mgmt_logo_default_tall.png art/mgmt_logo_default_wide.png art/mgmt_logo_reversed_symbol.png art/mgmt_logo_reversed_tall.png art/mgmt_logo_reversed_wide.png art/mgmt_logo_white_symbol.png art/mgmt_logo_white_tall.png art/mgmt_logo_white_wide.png

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
version:
	@echo $(VERSION)

program:
	@echo $(PROGRAM)

path:
	./misc/make-path.sh

deps:
	./misc/make-deps.sh

run:
	find . -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION)"

# include race flag
race:
	find . -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -race -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION)"

generate:
	go generate

build: $(PROGRAM)

$(PROGRAM): $(GO_FILES)
	@echo "Building: $(PROGRAM), version: $(SVERSION)..."
	time go build -i -ldflags "-X main.program=$(PROGRAM) -X main.version=$(SVERSION)" -o $(PROGRAM) $(BUILD_FLAGS);

$(PROGRAM).static: $(GO_FILES)
	@echo "Building: $(PROGRAM).static, version: $(SVERSION)..."
	go generate
	go build -a -installsuffix cgo -tags netgo -ldflags '-extldflags "-static" -X main.program=$(PROGRAM) -X main.version=$(SVERSION) -s -w' -o $(PROGRAM).static $(BUILD_FLAGS);

clean:
	[ ! -e $(PROGRAM) ] || rm $(PROGRAM)
	rm -f *_stringer.go	# generated by `go generate`
	rm -f *_mock.go		# generated by `go generate`

test:
	./test.sh

gofmt:
	# TODO: remove gofmt once goimports has a -s option
	find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -exec gofmt -s -w {} \;
	find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -exec goimports -w {} \;

yamlfmt:
	find . -maxdepth 3 -type f -name '*.yaml' -not -path './old/*' -not -path './tmp/*' -not -path './omv.yaml' -exec ruby -e "require 'yaml'; x=YAML.load_file('{}').to_yaml.each_line.map(&:rstrip).join(10.chr)+10.chr; File.open('{}', 'w').write x" \;

format: gofmt yamlfmt

docs: $(PROGRAM)-documentation.pdf

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

upload: upload-sources upload-srpms upload-rpms
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

#
#	copr build
#
copr: upload-srpms
	./misc/copr-build.py https://$(SERVER)/$(REMOTE_PATH)/SRPMS/$(SRPM_BASE)

# vim: ts=8
