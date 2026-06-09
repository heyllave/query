# trazo_query_flutter

Flutter packaging for the Trazo query engine. This package bundles the **prebuilt
native `libquery`** (Android `.so` per ABI, iOS xcframework) so a Flutter app has
a library for the FFI backend to load on a device, and re-exports the
[`trazo_query`](../flutter) API.

It contains **no query logic** — that all lives in the pure-Dart `trazo_query`
package, which stays independently testable (`dart test`, `dart test -p chrome`).
This package is only packaging: a thin re-export plus the platform build glue.
There is no second implementation to keep in sync.

## Usage

```yaml
# your app's pubspec.yaml
dependencies:
  trazo_query_flutter:
    path: ../query/clients/flutter_plugin   # or a git dependency
```

```dart
import 'package:trazo_query_flutter/trazo_query_flutter.dart';

final q = await TrazoQuery.load();
final m = await q.match('state=draft AND total>50000', fields, record);
```

`TrazoQuery.load()` resolves the native library per platform: Android opens the
bundled `libquery.so` by soname; iOS looks the statically-linked symbols up in
the process; desktop opens `libquery.{so,dylib,dll}`.

## Building the native binaries

The binaries are **not committed** (they are platform-specific build artifacts,
like the desktop `libquery`). Produce them before building the app:

```bash
# Android: one libquery.so per ABI into android/src/main/jniLibs/<abi>/
ANDROID_NDK_HOME=~/Android/Sdk/ndk/28.1.13356709 make -C ../ffi android

# iOS: device + simulator slices into ios/Frameworks/libquery.xcframework
make -C ../ffi ios        # macOS + Xcode only
```

Android needs NDK r27c/r28+ (r28+ aligns to 16 KB pages by default, which Google
Play requires). iOS must be built on macOS with Xcode. The default ABIs are
`arm64-v8a armeabi-v7a x86_64`; override with `ANDROID_ABIS="…"` (add `x86` for
old emulators).

## Example

`example/` is a minimal app plus an on-device integration test. Generate the
platform shells once, then run it on a device or emulator:

```bash
cd example
flutter create --platforms=android,ios .   # generate android/ and ios/ shells
flutter pub get
flutter test integration_test/load_test.dart   # proves the binary loads + FFI resolves
```

The integration test is the **only** check that exercises the bundled binary —
a missing ABI (Android `UnsatisfiedLinkError`) or stripped symbols (iOS
`DynamicLibrary.process()` lookup failure) surface only at runtime on a target.

## iOS release builds

The cgo exports are resolved at runtime, so the linker would dead-strip them. The
podspec force-loads the archive to prevent that. Additionally, in the host app's
`Runner` target set **Build Settings → Strip Style → Non-Global Symbols** so the
FFI symbols survive release/IPA builds.

## Distribution

A binary-bearing plugin must not be published to pub.dev — keep `publish_to:
none` and consume it via a path or git dependency. Because the binaries are
gitignored, CI and contributors must run the `make -C ../ffi android|ios` step
before `flutter build`, or the `jniLibs` / xcframework will be missing.
