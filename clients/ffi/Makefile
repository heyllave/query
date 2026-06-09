.PHONY: build clean android ios

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

# --- Android cross-compilation (cgo via the NDK) ---
# Needs the Android NDK (r27c/r28+; r28+ is 16KB page-aligned by default).
# Point ANDROID_NDK_HOME at it. Produces one libquery.so per ABI under the
# Flutter plugin's jniLibs, which Gradle bundles into the app.
ANDROID_NDK_HOME ?= $(HOME)/Android/Sdk/ndk/28.1.13356709
ANDROID_API      ?= 21
ANDROID_HOST_TAG ?= linux-x86_64
ANDROID_ABIS     ?= arm64-v8a armeabi-v7a x86_64
NDK_BIN          := $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/$(ANDROID_HOST_TAG)/bin
JNILIBS          := ../flutter_plugin/android/src/main/jniLibs

# --- iOS cross-compilation (cgo c-archive -> xcframework) ---
# Needs macOS + Xcode (xcrun/lipo/xcodebuild). Builds a device slice and a fat
# simulator slice and assembles the xcframework the plugin's podspec vendors.
IOS_MIN     ?= 13.0
XCFRAMEWORK := ../flutter_plugin/ios/Frameworks/libquery.xcframework

# Build the desktop c-shared library + header and copy them where the Dart
# package's FFI backend loads them.
build:
	CGO_ENABLED=1 CC=$(CC) go build -buildmode=c-shared -tags cshared \
		-o libquery.$(EXT) .
	mkdir -p ../flutter/native
	cp libquery.$(EXT) libquery.h ../flutter/native/

# Cross-compile one libquery.so per Android ABI. Host CGO_* flags are cleared so
# a desktop '-arch' does not leak into the NDK clang (golang/go#74410); the 16KB
# page-size linker flags satisfy Google Play (harmless on NDK r28+, which already
# aligns to 16KB). Only the .so is kept — the generated header is dropped.
android:
	@for abi in $(ANDROID_ABIS); do \
		case "$$abi" in \
			arm64-v8a)   goarch=arm64; goarm=;  triple=aarch64-linux-android ;; \
			armeabi-v7a) goarch=arm;   goarm=7; triple=armv7a-linux-androideabi ;; \
			x86_64)      goarch=amd64; goarm=;  triple=x86_64-linux-android ;; \
			x86)         goarch=386;   goarm=;  triple=i686-linux-android ;; \
			*) echo "unknown ABI: $$abi" >&2; exit 1 ;; \
		esac; \
		cc=$(NDK_BIN)/$${triple}$(ANDROID_API)-clang; \
		if [ ! -x "$$cc" ]; then \
			echo "missing NDK clang: $$cc (set ANDROID_NDK_HOME)" >&2; exit 1; \
		fi; \
		out=$(JNILIBS)/$${abi}; mkdir -p "$$out"; \
		tmp=$$(mktemp -d); \
		echo "building android $$abi ($$goarch)"; \
		env -u CGO_CFLAGS -u CGO_LDFLAGS -u CGO_CPPFLAGS -u CGO_CXXFLAGS \
			CGO_ENABLED=1 GOOS=android GOARCH=$$goarch $${goarm:+GOARM=$$goarm} \
			CC=$$cc \
			CGO_LDFLAGS='-Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384' \
			go build -buildmode=c-shared -trimpath -tags cshared \
				-o "$$tmp/libquery.so" . || { rm -rf "$$tmp"; exit 1; }; \
		cp "$$tmp/libquery.so" "$$out/libquery.so"; \
		rm -rf "$$tmp"; \
	done

# Build the iOS device + simulator c-archives and assemble them into an
# xcframework. clangwrap.sh selects the SDK and target triple per slice; the two
# simulator archs are lipo'd into one slice (device and simulator must stay
# separate xcframework slices).
ios:
	@command -v xcrun >/dev/null 2>&1 || \
		{ echo "ios build needs macOS + Xcode (no xcrun found)" >&2; exit 1; }
	@tmp=$$(mktemp -d); \
	wrap=$(CURDIR)/clangwrap.sh; \
	mkdir -p "$$tmp/device" "$$tmp/sim-arm64" "$$tmp/sim-x86_64" "$$tmp/sim"; \
	echo "building ios device arm64"; \
	SDK=iphoneos CARCH=arm64 TARGET=arm64-apple-ios$(IOS_MIN) \
		CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$$wrap" \
		go build -buildmode=c-archive -trimpath -tags cshared \
			-o "$$tmp/device/libquery.a" . || { rm -rf "$$tmp"; exit 1; }; \
	echo "building ios simulator arm64"; \
	SDK=iphonesimulator CARCH=arm64 TARGET=arm64-apple-ios$(IOS_MIN)-simulator \
		CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$$wrap" \
		CGO_CFLAGS=-mios-simulator-version-min=$(IOS_MIN) \
		CGO_LDFLAGS=-mios-simulator-version-min=$(IOS_MIN) \
		go build -buildmode=c-archive -trimpath -tags cshared \
			-o "$$tmp/sim-arm64/libquery.a" . || { rm -rf "$$tmp"; exit 1; }; \
	echo "building ios simulator x86_64"; \
	SDK=iphonesimulator CARCH=x86_64 TARGET=x86_64-apple-ios$(IOS_MIN)-simulator \
		CGO_ENABLED=1 GOOS=ios GOARCH=amd64 CC="$$wrap" \
		CGO_CFLAGS=-mios-simulator-version-min=$(IOS_MIN) \
		CGO_LDFLAGS=-mios-simulator-version-min=$(IOS_MIN) \
		go build -buildmode=c-archive -trimpath -tags cshared \
			-o "$$tmp/sim-x86_64/libquery.a" . || { rm -rf "$$tmp"; exit 1; }; \
	lipo -create "$$tmp/sim-arm64/libquery.a" "$$tmp/sim-x86_64/libquery.a" \
		-output "$$tmp/sim/libquery.a"; \
	cp "$$tmp/sim-arm64/libquery.h" "$$tmp/sim/libquery.h"; \
	rm -rf $(XCFRAMEWORK); mkdir -p $$(dirname $(XCFRAMEWORK)); \
	xcodebuild -create-xcframework \
		-library "$$tmp/device/libquery.a" -headers "$$tmp/device" \
		-library "$$tmp/sim/libquery.a"    -headers "$$tmp/sim" \
		-output  $(XCFRAMEWORK); \
	rm -rf "$$tmp"

clean:
	rm -f libquery.so libquery.dylib libquery.dll libquery.h
	rm -f ../flutter/native/libquery.*
	rm -rf $(JNILIBS) $(XCFRAMEWORK)
