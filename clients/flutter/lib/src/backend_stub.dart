import 'backend.dart';

/// Fallback used only when neither dart:io nor dart:js_interop is available
/// (e.g. during analysis on an unknown target). Never reached at runtime on a
/// real platform.
Future<Backend> openBackend({String? libraryPath, String? wasmUrl}) {
  throw UnsupportedError(
    'trazo_query: no backend for this platform (expected native FFI or web WASM)',
  );
}
