# Package related
BINARY_NAME=infiniband
PACKAGE=infiniband-cni
ORG_PATH=github.com/Mellanox
REPO_PATH=$(ORG_PATH)/$(PACKAGE)
GOPATH=$(CURDIR)/.gopath
GOBIN =$(CURDIR)/bin
BUILDDIR=$(CURDIR)/build
BASE=$(GOPATH)/src/$(REPO_PATH)
GOFILES=$(shell find . -name *.go | grep -vE "(\/vendor\/)|(_test.go)")
PKGS=$(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) $(GO) list ./... | grep -v "^$(PACKAGE)/vendor/"))

export GOPATH
export GOBIN

# Go tools
GO      = go
GOFMT   = gofmt
GOLINT = $(GOBIN)/golint
V = 0
Q = $(if $(filter 1,$V),,@)

.PHONY: all
all: fmt lint build

$(BASE): ; $(info  setting GOPATH...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

$(GOBIN):
	@mkdir -p $@

$(BUILDDIR): | $(BASE) ; $(info Creating build directory...)
	@cd $(BASE) && mkdir -p $@

build: $(BUILDDIR)/$(BINARY_NAME) ; $(info Building $(BINARY_NAME)...) ## Build executable file
	$(info Done!)

$(BUILDDIR)/$(BINARY_NAME): $(GOFILES) | $(BUILDDIR)
	@cd $(BASE)/cmd/$(BINARY_NAME) && CGO_ENABLED=0 $(GO) build -o $(BUILDDIR)/$(BINARY_NAME) -tags no_openssl -v

# Tools

.PHONY: fmt
fmt: ; $(info  running gofmt...) @ ## Run gofmt on all source files
	@ret=0 && for d in $$($(GO) list -f '{{.Dir}}' ./... | grep -v /vendor/); do \
		$(GOFMT) -l -w $$d/*.go || ret=$$? ; \
	 done ; exit $$ret

$(GOLINT): | $(BASE) ; $(info  building golint...)
	$Q go get -u golang.org/x/lint/golint

.PHONY: lint
lint: | $(BASE) $(GOLINT) ; $(info  running golint...) @ ## Run golint
	$Q cd $(BASE) && ret=0 && for pkg in $(PKGS); do \
		test -z "$$($(GOLINT) $$pkg | tee /dev/stderr)" || ret=1 ; \
	 done ; exit $$ret

# Misc

.PHONY: clean
clean: ; $(info  Cleaning...)	 ## Cleanup everything
	@rm -rf $(GOPATH)
	@rm -rf $(BUILDDIR)

.PHONY: help
help: ## Show this message
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'