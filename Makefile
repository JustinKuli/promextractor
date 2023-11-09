# Could alternatively use podman or something
DOCKER ?= docker

ifeq ($(REVISION),)
	REVISION = $(shell git describe --dirty)
endif

.PHONY: build
build:
	go build -ldflags="-X 'main.Version=$(REVISION)'" -o promextractor .

.PHONY: build-image
build-image:
	$(DOCKER) build -t promextractor:development -f ./build/Containerfile \
	--label org.opencontainers.image.version=$(REVISION) \
	--label org.opencontainers.image.revision=$(REVISION) \
	.
