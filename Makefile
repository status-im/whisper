SHELL = /bin/bash

GO111MODULE = on

test:
	go test ./whisperv6/...
	go test ./shhclient/...
.PHONY: test

vendor:
	go mod tidy
	go mod vendor
	modvendor -copy="**/*.c **/*.h" -v
.PHONY: vendor

install-dev:
	# a tool to vendor non-go files
	go get github.com/goware/modvendor@latest
.PHONY: install-dev

