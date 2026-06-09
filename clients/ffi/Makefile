.PHONY: build clean

# Host shared-library extension. Override for cross builds.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	EXT := dylib
else
	EXT := so
endif

# C compiler for cgo. Defaults to clang (the CI runner has clang but no gcc,
# cgo's default). Override on the command line: make CC=gcc build.
CC := clang

# Build the c-shared library + header and copy them where the Dart package's
# FFI backend loads them.
build:
	CGO_ENABLED=1 CC=$(CC) go build -buildmode=c-shared -tags cshared \
		-o libquery.$(EXT) .
	mkdir -p ../flutter/native
	cp libquery.$(EXT) libquery.h ../flutter/native/

clean:
	rm -f libquery.so libquery.dylib libquery.dll libquery.h
	rm -f ../flutter/native/libquery.*
