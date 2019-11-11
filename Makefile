SHELL = /bin/bash

GO111MODULE = on

clean:
	rm -rf crashers corpus suppressions whisperv6-fuzz.zip
.PHONY: clean

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
	# a tool for fuzzing
	go get github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build
.PHONY: install-dev

fuzz:
	CGO_ENABLED=0 go-fuzz-build -tags ios github.com/status-im/whisper/whisperv6
	go-fuzz -bin=whisperv6-fuzz.zip
.PHONY: fuzz
