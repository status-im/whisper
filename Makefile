SHELL = /bin/bash

GO111MODULE = on

clean:
	rm -rf crashers corpus suppressions whisperv6-fuzz.zip
.PHONY: clean

test:
	go test -timeout 60s ./whisperv6/...
	go test -timeout 30s ./shhclient/...
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

# Couple things of note here are the use of the `ios` tag and disabling CGO
# altogether.
#
# On our fork of go-ethereum, we've tagged `metrics/cpu.go` as so not to have
# it built on the `ios` and `android` targets via a build tag. This diverges
# from the upstream implementation, which does not have these tags.
#
# Metrics are captured through the use of a library, `gosigar`, which is an
# abstraction for capturing this sort of info across different operating
# systems. On Linux, for instance, it reads from the proc filesystem, but on
# Darwin/OSX it'll use syscalls that need C.
#
# Disabling CGO here is also necessary, as the tool doesn't play well with CGO
# at all. The tag and the environment variable are the combination that makes
# this work.
fuzz:
	CGO_ENABLED=0 go-fuzz-build -tags ios github.com/status-im/whisper/whisperv6
	go-fuzz -bin=whisperv6-fuzz.zip
.PHONY: fuzz
