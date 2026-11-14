.PHONY: build test clean

# Host shared-library extension.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	EXT := dylib
else
	EXT := so
endif

# C compiler for cgo.
#
# Make always defines CC (its built-in default is `cc`), so a plain assignment
# here would silently outrank a CC exported by the caller. Only a value that
# came from the environment or the command line counts as a real choice;
# otherwise take the first compiler that exists.
ifeq ($(origin CC),default)
CC := $(shell command -v clang 2>/dev/null || command -v gcc 2>/dev/null || command -v cc 2>/dev/null)
endif
ifeq ($(strip $(CC)),)
$(error no C compiler found: cgo needs clang, gcc or cc on PATH)
endif

# Build the shared library and its header for the host platform.
build:
	CGO_ENABLED=1 CC=$(CC) go build -buildmode=c-shared -trimpath -tags cshared \
		-o libquery.$(EXT) .

# Run the shared conformance corpus through the library from another language
# (Python's ctypes, standard library only), the way a real binding would.
test: build
	python3 conformance_test.py ./libquery.$(EXT)

clean:
	rm -f libquery.so libquery.dylib libquery.dll libquery.h
