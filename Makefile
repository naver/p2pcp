MAJOR_VERSION = $(shell grep 'MAJOR' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
MINOR_VERSION = $(shell grep 'MINOR' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
PATCH_VERSION = $(shell grep 'PATCH' VERSION | grep -v -E '\#' | grep -E '[0-9]*' -o)
BUILD_VERSION = $(shell git describe --always | sed "s/^v//g")

TARGET = p2pcp

OUTPUT_DIR = output
BUILD_TYPE = release

ifeq ($(BUILD_TYPE),debug)
  BUIlD_FLAG = -gcflags all="-N -l"
else
  BUIlD_FLAG = 
endif

GO = go
SRC = $(wildcard *.go)

$(TARGET): $(SRC) Makefile
	$(GO) build -o $(TARGET) $(BUIlD_FLAG) \
	-ldflags "-X main.MajorVersion=$(MAJOR_VERSION) -X main.MinorVersion=$(MINOR_VERSION) -X main.PatchVersion=$(PATCH_VERSION) -X main.BuildVersion=$(BUILD_VERSION)" \
	$(SRC)

install:
	mkdir -p $(OUTPUT_DIR)/bin
	cp $(TARGET) $(OUTPUT_DIR)/bin/

test:
	$(GO) test -v $(SRC)

clean:
	rm -r $(OUTPUT_DIR) || true
	rm $(TARGET) || true

.PHONY: install test clean

