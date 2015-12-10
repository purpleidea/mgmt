SHELL = /bin/bash
.PHONY: all version deps run race build clean test format docs
.SILENT: clean

VERSION := $(shell git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --dirty)
PROGRAM := $(notdir $(CURDIR))

all: docs

# show the current version
version:
	@echo $(VERSION)

deps:
	./misc/make-deps.sh

run:
	find -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -ldflags "-X main.version=$(VERSION) -X main.program=$(PROGRAM)"

# include race flag
race:
	find -maxdepth 1 -type f -name '*.go' -not -name '*_test.go' | xargs go run -race -ldflags "-X main.version=$(VERSION) -X main.program=$(PROGRAM)"

build: mgmt

mgmt: main.go
	go generate
	go build -ldflags "-X main.version=$(VERSION) -X main.program=$(PROGRAM)"

clean:
	[ ! -e mgmt ] || rm mgmt

test:
	./test.sh

format:
	find -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -exec gofmt -w {} \;
	find -type f -name '*.yaml' -not -path './old/*' -not -path './tmp/*' -not -path './omv.yaml' -exec ruby -e "require 'yaml'; x=YAML.load_file('{}').to_yaml.each_line.map(&:rstrip).join(10.chr)+10.chr; File.open('{}', 'w').write x" \;

docs: mgmt-documentation.pdf

mgmt-documentation.pdf: DOCUMENTATION.md
	pandoc DOCUMENTATION.md -o 'mgmt-documentation.pdf'
