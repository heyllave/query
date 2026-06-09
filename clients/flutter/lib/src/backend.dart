// openBackend is resolved at compile time: the stub by default (so the package
// analyzes on any target), the FFI backend on native (dart:io), the web backend
// under dart:js_interop.
export 'backend_stub.dart'
    if (dart.library.io) 'backend_ffi.dart'
    if (dart.library.js_interop) 'backend_web.dart';

/// The raw bridge backend: each method takes a JSON request string and returns
/// the JSON response string produced by the Go engine. Both the FFI (native) and
/// WASM (web) backends implement this identically, so the public API layer is
/// platform-agnostic.
abstract class Backend {
  Future<String> parse(String requestJson);
  Future<String> validate(String requestJson);
  Future<String> stringify(String requestJson);
  Future<String> parseAndValidate(String requestJson);
  Future<String> match(String requestJson);
  Future<String> eval(String requestJson);
}
