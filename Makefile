include make/licence.mk
# Package related
BINARY_NAME=ipoib
PACKAGE=ipoib-cni
ORG_PATH=github.com/Mellanox
REPO_PATH=$(ORG_PATH)/$(PACKAGE)
BINDIR =$(CURDIR)/bin
BUILDDIR=$(CURDIR)/build
BASE=$(CURDIR)
GOFILES=$(shell find . -name *.go | grep -vE "(_test.go)")
PKGS=$(or $(PKG),$(shell $(GO) list ./...))
TESTPKGS = $(shell $(GO) list -f '{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' $(PKGS))

# Version
VERSION?=master
DATE=`date -Iseconds`
COMMIT?=`git rev-parse --verify HEAD`
LDFLAGS="-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Docker
IMAGE_BUILDER?=@docker
IMAGEDIR=$(BASE)/images
DOCKERFILE?=$(BASE)/Dockerfile
TAG?=mellanox/ipoib-cni
IMAGE_BUILD_OPTS?=
# Accept proxy settings for docker
# To pass proxy for Docker invoke it as 'make image HTTP_POXY=http://192.168.0.1:8080'
DOCKERARGS=
ifdef HTTP_PROXY
	DOCKERARGS += --build-arg http_proxy=$(HTTP_PROXY)
endif
ifdef HTTPS_PROXY
	DOCKERARGS += --build-arg https_proxy=$(HTTPS_PROXY)
endif
IMAGE_BUILD_OPTS += $(DOCKERARGS)
GOPROXY ?= $(shell go env GOPROXY)

# Go tools
GO      = go
GOLANGCI_LINT = $(BINDIR)/golangci-lint-$(GOLANGCI_LINT_VER)
GOLANGCI_LINT_VER ?= v2.11.4
TIMEOUT = 15
Q = $(if $(filter 1,$V),,@)

.PHONY: all
all: lint build

$(BINDIR):
	@mkdir -p $@

$(BUILDDIR): ; $(info Creating build directory...)
	@mkdir -p $@

build: $(BUILDDIR)/$(BINARY_NAME) build-entrypoint ; $(info Building $(BINARY_NAME)...) ## Build executable file
	$(info Done!)

$(BUILDDIR)/$(BINARY_NAME): $(GOFILES) | $(BUILDDIR)
	@cd $(BASE)/cmd/$(BINARY_NAME) && CGO_ENABLED=0 $(GO) build -o $(BUILDDIR)/$(BINARY_NAME) -tags no_openssl -ldflags $(LDFLAGS)  -v

.PHONY: build-entrypoint
build-entrypoint: | $(BUILDDIR) ; $(info Building entrypoint...) ## Build entrypoint binary
	$Q CGO_ENABLED=0 $(GO) build -o $(BUILDDIR)/entrypoint -tags no_openssl ./cmd/entrypoint/

# Tools

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): | $(BINDIR) ; $(info  building golangci-lint...)
	$Q GOBIN=$(BINDIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VER)
	mv $(BINDIR)/golangci-lint $(GOLANGCI_LINT)

GOVERALLS = $(BINDIR)/goveralls
$(BINDIR)/goveralls: | $(BINDIR) ; $(info  building goveralls...)
	$Q GOBIN=$(BINDIR) go install github.com/mattn/goveralls@latest

HADOLINT_TOOL = $(BINDIR)/hadolint
$(HADOLINT_TOOL): | $(BINDIR) ; $(info  installing hadolint...)
	$(call wget-install-tool,$(HADOLINT_TOOL),"https://github.com/hadolint/hadolint/releases/download/v2.12.1-beta/hadolint-Linux-x86_64")

# Tests

.PHONY: lint
lint: golangci-lint ; $(info  running golangci-lint...) @ ## Run golangci-lint
	$Q $(GOLANGCI_LINT) run --timeout 10m

TEST_TARGETS := test-default test-bench test-short test-verbose test-race
.PHONY: $(TEST_TARGETS) test tests
test-bench:   ARGS=-run=__absolutelynothing__ -bench=. ## Run benchmarks
test-short:   ARGS=-short        ## Run only short tests
test-verbose: ARGS=-v            ## Run tests in verbose mode with coverage reporting
test-race:    ARGS=-race         ## Run tests with race detector
$(TEST_TARGETS): NAME=$(MAKECMDGOALS:test-%=%)
$(TEST_TARGETS): test
test tests: lint ; $(info  running $(NAME:%=% )tests...) @ ## Run tests
	$Q $(GO) test -timeout $(TIMEOUT)s $(ARGS) $(TESTPKGS)

COVERAGE_MODE = count
.PHONY: test-coverage test-coverage-tools
test-coverage-tools: | $(GOVERALLS)
test-coverage: COVERAGE_DIR := $(BASE)/test
test-coverage: test-coverage-tools | $(BASE) ; $(info  running coverage tests...) @ ## Run coverage tests
	$Q $(GO) test -covermode=$(COVERAGE_MODE) -coverprofile=ipoib-cni.cover ./...

# Container image
.PHONY: image
image: | $(BASE) ; $(info Building Docker image...)  ## Build conatiner image
	$(IMAGE_BUILDER) build -t $(TAG) -f $(DOCKERFILE) $(BASE) --build-arg GOPROXY="$(GOPROXY)" $(IMAGE_BUILD_OPTS)

.PHONY: hadolint
hadolint: $(BASE) $(HADOLINT_TOOL); $(info  running hadolint...) @ ## Run hadolint
	$Q $(HADOLINT_TOOL) Dockerfile

tests: lint hadolint test ## Run lint, hadolint, unit test

# Misc

.PHONY: clean
clean: ; $(info  Cleaning...)	 ## Cleanup everything
	@rm -rf $(BUILDDIR)
	@rm -rf $(BINDIR)
	@rm -rf  test

.PHONY: help
help: ## Show this message
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

define wget-install-tool
@[ -f $(1) ] || { \
echo "Downloading $(2)" ;\
mkdir -p $(BINDIR);\
wget -O $(1) $(2);\
chmod +x $(1) ;\
}
endef

