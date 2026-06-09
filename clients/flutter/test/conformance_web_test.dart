@TestOn('browser')
library;

// Runs the shared cross-language corpus (/conformance/corpus.json) against the
// WASM build through the web (dart:js_interop) backend, the same way
// conformance_test.dart runs it against the native FFI build. The query.wasm,
// wasm_exec.js, and corpus.json assets are staged into test/ by
// `make -C clients/flutter web-assets` so the browser test server serves them.

import 'dart:convert';
import 'dart:js_interop';

import 'package:test/test.dart';
import 'package:trazo_query/trazo_query.dart';
import 'package:web/web.dart' as web;

import 'conformance_shared.dart';

Future<Map<String, Object?>> _fetchCorpus(String url) async {
  final resp = await web.window.fetch(url.toJS).toDart;
  final body = await resp.text().toDart;
  return jsonDecode(body.toDart) as Map<String, Object?>;
}

void main() async {
  final corpus = await _fetchCorpus('corpus.json');

  defineConformanceTests(
    expectedCases: 25,
    corpus: corpus,
    load: TrazoQuery.load,
  );
}
