import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

import 'backend.dart';

typedef _QueryNative = Pointer<Utf8> Function(Pointer<Utf8>);
typedef _QueryDart = Pointer<Utf8> Function(Pointer<Utf8>);
typedef _FreeNative = Void Function(Pointer<Utf8>);
typedef _FreeDart = void Function(Pointer<Utf8>);

/// Loads the native FFI backend backed by the `libquery` shared library built by
/// `make -C clients/ffi build`. [libraryPath] overrides the default lookup;
/// [wasmUrl] is ignored on native.
Future<Backend> openBackend({String? libraryPath, String? wasmUrl}) async {
  final lib = DynamicLibrary.open(libraryPath ?? _defaultLibraryPath());
  return _FfiBackend(lib);
}

String _defaultLibraryPath() {
  final base = 'native/libquery';
  if (Platform.isMacOS) return '$base.dylib';
  if (Platform.isWindows) return '$base.dll';
  return '$base.so';
}

/// Calls the Go engine through cgo c-shared exports. Each call passes a JSON
/// request string and returns the JSON response string; the engine malloc's the
/// response, so it is freed with the library's own QueryFree, never Dart's.
class _FfiBackend implements Backend {
  _FfiBackend(DynamicLibrary lib)
      : _parse = lib.lookupFunction<_QueryNative, _QueryDart>('QueryParse'),
        _validate = lib.lookupFunction<_QueryNative, _QueryDart>('QueryValidate'),
        _stringify = lib.lookupFunction<_QueryNative, _QueryDart>('QueryStringify'),
        _parseAndValidate =
            lib.lookupFunction<_QueryNative, _QueryDart>('QueryParseAndValidate'),
        _match = lib.lookupFunction<_QueryNative, _QueryDart>('QueryMatch'),
        _eval = lib.lookupFunction<_QueryNative, _QueryDart>('QueryEval'),
        _free = lib.lookupFunction<_FreeNative, _FreeDart>('QueryFree');

  final _QueryDart _parse;
  final _QueryDart _validate;
  final _QueryDart _stringify;
  final _QueryDart _parseAndValidate;
  final _QueryDart _match;
  final _QueryDart _eval;
  final _FreeDart _free;

  String _call(_QueryDart fn, String requestJson) {
    final reqPtr = requestJson.toNativeUtf8();
    try {
      final respPtr = fn(reqPtr);
      try {
        return respPtr.toDartString();
      } finally {
        _free(respPtr);
      }
    } finally {
      malloc.free(reqPtr);
    }
  }

  @override
  Future<String> parse(String requestJson) async => _call(_parse, requestJson);

  @override
  Future<String> validate(String requestJson) async => _call(_validate, requestJson);

  @override
  Future<String> stringify(String requestJson) async =>
      _call(_stringify, requestJson);

  @override
  Future<String> parseAndValidate(String requestJson) async =>
      _call(_parseAndValidate, requestJson);

  @override
  Future<String> match(String requestJson) async => _call(_match, requestJson);

  @override
  Future<String> eval(String requestJson) async => _call(_eval, requestJson);
}
