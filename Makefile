# Copyright 2017 Berkeley Electronic Press.
# All rights reserved.
#
.SILENT: ; # no need for @

PROJECT			=camo
PROJECT_DIR		=$(shell pwd)

GOFILES         :=$(shell find . -name '*.go' -not -path './vendor/*')
GOPACKAGES      :=$(shell go list ./... | grep -v /vendor/| grep -v /checkers)
OS              := $(shell go env GOOS)
ARCH            := $(shell go env GOARCH)

GITHASH         :=$(shell git rev-parse --short HEAD)
GITBRANCH       :=$(shell git rev-parse --abbrev-ref HEAD)
GITTAGORBRANCH 	:=$(shell sh -c 'git describe --always --dirty --tags 2>/dev/null')
BUILDDATE      	:=$(shell date -u +%Y%m%d%H%M)
GO_LDFLAGS		?= -s -w
GO_BUILD_FLAGS  :=-ldflags "${GOLDFLAGS} -X main.BuildVersion=${GITTAGORBRANCH} -X main.GitHash=${GITHASH} -X main.GitBranch=${GITBRANCH} -X main.BuildDate=${BUILDDATE}"


## What if there's no CIRCLE_BUILD_NUM
ifeq ($$CIRCLE_BUILD_NUM, "")
		BUILD_NUM:=""
else
		CB:=$$CIRCLE_BUILD_NUM
		BUILD_NUM:=$(CB)/
endif

ARTIFACT_BUCKET :="artifacts.production.bepress.com"
ARTIFACT_NAME   :=$(PROJECT)-$(GITTAGORBRANCH)-$(GITHASH).tar.gz
ARTIFACT_DIR    :=$(PROJECT_DIR)/_artifacts
WORKDIR         :=$(PROJECT_DIR)/_workdir

default: build-linux

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(WORKDIR)/$(PROJECT)_linux_amd64 $(GO_BUILD_FLAGS)

build:
	CGO_ENABLED=0 go build -o $(WORKDIR)/$(PROJECT)_$(OS)_$(ARCH) $(GO_BUILD_FLAGS)

clean:
	rm -f $(WORKDIR)/*
	rm -f $(ARTIFACT_DIR)/*
	rm -rf .cover
	go clean -r

coverage:
	./_misc/coverage.sh

coverage-html:
	./_misc/coverage.sh --html

dependencies:
	go get honnef.co/go/tools/cmd/megacheck
	go get github.com/alecthomas/gometalinter
	go get github.com/golang/dep/cmd/dep
	dep ensure
	gometalinter --install

develop: dependencies
	(cd .git/hooks && ln -sf ../../_misc/pre-push.bash pre-push )
	git flow init -d

lint:
# TODO(ro) 2017-09-18 See also
# https://github.com/360EntSecGroup-Skylar/goreporter
# metalinter runs the degault linters
# https://github.com/alecthomas/gometalinter#supported-linters and the enabled
# ones here.
# At the time of this writing megacheck runs gosimple, staticcheck, and
# unused. All production honnef tools.
	# gometalinter --enable=goimports --enable=unparam --enable=unused --disable=golint --disable=govet .
	echo "metalinter..."
	gometalinter --enable=goimports --enable=unparam --enable=unused --disable=golint --disable=govet $(GOPACKAGES)
	echo "megacheck..."
	megacheck $(GOPACKAGES)
	echo "golint..."
	golint -set_exit_status $(GOPACKAGES) 
	echo "go vet..."
	go vet --all $(GOPACKAGES)

master: build-linux
# NB: This target is intended for CircleCI.
# Copy the systemd unit into _workdir and make a tarball. Then copy to master
# prefix, copy latest to previous, next to latest and this build to next.
	cp _misc/$(PROJECT).service _misc/awslogs.conf _misc/go_expvar.yaml $(WORKDIR)
	mkdir -p $(PROJECT_DIR)/_artifacts
	(cd $(PROJECT_DIR)/_artifacts && tar -czvf $(ARTIFACT_NAME) -C $(WORKDIR) .)
	aws s3 cp $(ARTIFACT_DIR)/$(ARTIFACT_NAME) s3://$(ARTIFACT_BUCKET)/$(PROJECT)/master/$(GITHASH)/$(BUILD_NUM)$(PROJECT).tar.gz

release: build-linux
# NB: This target is intended for CircleCI.
# Copy the systemd unit into _workdir and make a tarball. Then copy to release
# prefix, copy latest to previous, next to latest and this release to next.
	cp _misc/$(PROJECT).service _misc/awslogs.conf _misc/go_expvar.yaml $(WORKDIR)
	mkdir -p $(PROJECT_DIR)/_artifacts
	(cd $(PROJECT_DIR)/_artifacts && tar -czvf $(ARTIFACT_NAME) -C $(WORKDIR) .)
	aws s3 cp $(ARTIFACT_DIR)/$(ARTIFACT_NAME) s3://$(ARTIFACT_BUCKET)/$(PROJECT)/release/$(GITTAGORBRANCH)-$(GITHASH)/$(BUILD_NUM)$(PROJECT).tar.gz
	aws s3 cp s3://$(ARTIFACT_BUCKET)/$(PROJECT)/latest/$(PROJECT).tar.gz s3://$(ARTIFACT_BUCKET)/$(PROJECT)/previous/$(PROJECT).tar.gz || true
	aws s3 cp s3://$(ARTIFACT_BUCKET)/$(PROJECT)/next/$(PROJECT).tar.gz s3://$(ARTIFACT_BUCKET)/$(PROJECT)/latest/$(PROJECT).tar.gz || true
	aws s3 cp $(ARTIFACT_DIR)/$(ARTIFACT_NAME) s3://$(ARTIFACT_BUCKET)/$(PROJECT)/next/$(PROJECT).tar.gz || true

test:
	CGO_ENABLED=0 go test $(GOPACKAGES)

test-race:
	CGO_ENABLED=1 go test -race $(GOPACKAGES)


