/// Flutter packaging for the Trazo query engine.
///
/// Depend on this package from a Flutter app to bundle the prebuilt engine on
/// every target — Android `.so` per ABI, iOS xcframework, the desktop shared
/// library, and the WASM module on web — so the FFI/WASM backend always has
/// something to load. The query API itself comes from the pure-Dart
/// [trazo_query] package and is re-exported here, so an app needs a single
/// import and a single dependency:
///
/// ```dart
/// import 'package:trazo_query_flutter/trazo_query_flutter.dart';
///
/// final q = await loadTrazoQuery();
/// ```
///
/// This package contains no query logic — only the bundled binaries, the
/// platform plumbing, and this re-export.
library;

import 'package:trazo_query/trazo_query.dart';

export 'package:trazo_query/trazo_query.dart';

/// Loads the engine for the current platform: the bundled shared library on
/// desktop/mobile, the WASM module on web.
///
/// The single entry point a Flutter app should use. On the web it expects
/// `query.wasm` and `wasm_exec.js` to be served from the page root — copy them
/// into your app's `web/` directory with
/// `make -C clients/ffi web-assets WEB=<app>/web`.
Future<TrazoQuery> loadTrazoQuery() => TrazoQuery.load();
