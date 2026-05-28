MAJOR_VERSION = $(shell grep 'MAJOR' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
MINOR_VERSION = $(shell grep 'MINOR' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
PATCH_VERSION = $(shell grep 'PATCH' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
BUILD_VERSION ?= $(shell git describe --always 2>/dev/null | sed "s/^v//g" || echo "unknown")

PACKAGE = github.com/naver/p2pcp

TARGET = p2pcp
CONTAINER_ENGINE = docker
BUILD_TYPE = release

ifeq ($(BUILD_TYPE),debug)
  BUILD_FLAG = -gcflags all="-N -l"
  LDFLAGS =
else
  BUILD_FLAG =
  LDFLAGS = -w -s
endif

GO ?= go
SRC = $(shell find src -name '*.go')

LDFLAGS += -X $(PACKAGE)/version.MajorVersion=$(MAJOR_VERSION) \
           -X $(PACKAGE)/version.MinorVersion=$(MINOR_VERSION) \
           -X $(PACKAGE)/version.PatchVersion=$(PATCH_VERSION) \
           -X $(PACKAGE)/version.BuildVersion=$(BUILD_VERSION)

GO_BUILD = cd src && CGO_ENABLED=1 $(GO) build -trimpath $(BUILD_FLAG)

BUILD_TAGS ?= netgo,osusergo

$(TARGET): $(SRC) Makefile
	$(GO_BUILD) -tags $(BUILD_TAGS) -o ../$(TARGET) -ldflags '-extldflags "-static" $(LDFLAGS)'

dynamic: $(SRC) Makefile
	$(GO_BUILD) -o ../$(TARGET) -ldflags '$(LDFLAGS)'

image:
	$(CONTAINER_ENGINE) build --build-arg BUILD_VERSION=$(BUILD_VERSION) -t $(TARGET):latest .

GO_LICENSES_VERSION = v1.6.0

_notice:
	@cd src && $(GO) run github.com/google/go-licenses@$(GO_LICENSES_VERSION) report ./... \
		--ignore github.com/naver/p2pcp \
		--template ../scripts/notice.tpl \
		> ../$(OUTPUT) 2>/dev/null

notice:
	@$(MAKE) _notice OUTPUT=NOTICE
	@echo "NOTICE regenerated."

notice-check:
	@$(MAKE) _notice OUTPUT=NOTICE.tmp
	@diff -u NOTICE NOTICE.tmp >/dev/null 2>&1 || \
		(echo "NOTICE is out of date. Run 'make notice' and commit the changes."; \
		 diff -u NOTICE NOTICE.tmp; \
		 rm -f NOTICE.tmp; exit 1)
	@rm -f NOTICE.tmp

fmt:
	cd src && $(GO) fmt ./...

fmt-check:
	@cd src && test -z "$$(gofmt -l .)" || (echo "format violation:"; gofmt -l .; exit 1)

test: $(SRC)
	@cd src && \
		rm -f coverage.md test-output.json && \
		$(GO) test -cover -race -json ./... > test-output.json; \
		TEST_RESULT=$$?; \
		if [ $$TEST_RESULT -ne 0 ]; then \
			echo "=== TEST FAILURES ==="; \
			jq -r 'select(.Action=="fail" and .Test) | .Test' test-output.json | sort -u | while read test; do \
				jq -j --arg t "$$test" 'select(.Test==$$t and .Action=="output") | .Output' test-output.json; \
			done; \
			exit $$TEST_RESULT; \
		fi && \
		echo "## Test coverage" > coverage.md && \
		echo "" >> coverage.md && \
		echo "| Package | Coverage |" >> coverage.md && \
		echo "|---------|----------|" >> coverage.md && \
		jq -r 'select(.Output and (.Output | test("^coverage:"))) | "\(.Package)\t\(.Output | capture("coverage: (?<cov>[0-9.]+%)") | .cov)"' test-output.json | \
			grep -v "/testutil" | \
			sed 's|$(PACKAGE)/||; s|$(PACKAGE)|(root)|' | \
			LC_ALL=C sort | \
			awk -F'\t' '{printf "| %s | %s |\n", $$1, $$2}' >> coverage.md && \
		cat coverage.md

e2e: e2e-local e2e-peer e2e-command e2e-k8s

e2e-local: $(TARGET)
	test/create_random_data.sh test_src
	test/local_test.sh test_src/ test_dst/
	rm -rf test_src test_dst

e2e-peer: $(TARGET)
	test/create_random_data.sh test_src
	test/peer_test.sh test_src/ test_dst/
	rm -rf test_src test_dst

e2e-command: $(TARGET)
	test/create_random_data.sh test_src
	test/command_test.sh test_src/ test_dst/
	rm -rf test_src test_dst

e2e-k8s:
	test/k8s_test.sh

clean:
	rm $(TARGET) || true
	rm -rf test_src test_dst || true
	rm -f src/coverage.md || true

.PHONY: dynamic image fmt fmt-check test e2e e2e-local e2e-peer e2e-command e2e-k8s clean
