@TestOn('vm')
library;

// Runs the shared cross-language corpus (/conformance/corpus.json) against the
// native FFI build. The browser test (conformance_web_test.dart) runs the same
// cases against the WASM build, and the Go and JS clients run the same file — so
// grammar drift between any implementation fails the build.

import 'dart:convert';
import 'dart:io';

import 'package:test/test.dart';
import 'package:trazo_query/trazo_query.dart';

import 'conformance_shared.dart';

String _libPath() {
  if (Platform.isMacOS) return 'native/libquery.dylib';
  if (Platform.isWindows) return 'native/libquery.dll';
  return 'native/libquery.so';
}

void main() {
  final corpus = jsonDecode(
    File('../../conformance/corpus.json').readAsStringSync(),
  ) as Map<String, Object?>;

  defineConformanceTests(
    expectedCases: 25,
    corpus: corpus,
    load: () async {
      final lib = _libPath();
      if (!File(lib).existsSync()) {
        throw StateError('missing $lib — run `make -C clients/ffi build` first');
      }
      return TrazoQuery.load(libraryPath: lib);
    },
  );
}
