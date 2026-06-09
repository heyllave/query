#!/bin/sh
# CC wrapper for the iOS cgo slices, invoked by `go build` as the cgo C
# compiler. The `ios` Makefile target sets three variables per slice:
#
#   SDK     iphoneos | iphonesimulator   (which Apple SDK to compile against)
#   CARCH   arm64 | x86_64               (-arch passed to clang)
#   TARGET  the clang -target triple      e.g. arm64-apple-ios13.0 (device) or
#           arm64-apple-ios13.0-simulator (simulator). The explicit -simulator
#           suffix is what keeps the simulator slice from being tagged as a
#           device slice, so `xcodebuild -create-xcframework` accepts both
#           (golang/go#57442).
#
# Requires macOS with Xcode so xcrun can resolve the SDKs and clang.
set -e
SDK_PATH=$(xcrun --sdk "$SDK" --show-sdk-path)
CLANG=$(xcrun --sdk "$SDK" --find clang)
exec "$CLANG" -arch "$CARCH" -isysroot "$SDK_PATH" -target "$TARGET" "$@"
