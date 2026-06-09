import 'dart:async';
import 'dart:convert';
import 'dart:js_interop';

import 'package:web/web.dart' as web;

import 'backend.dart';

// The Go WASM runtime registers these globals on globalThis after go.run(). They
// mirror the native FFI exports; only the calling convention differs (positional
// JS args vs. a single JSON request object), so this backend translates the
// uniform request JSON into the positional form.
@JS('queryParse')
external JSAny? _queryParse(JSString query, JSNumber maxLength);

@JS('queryValidate')
external JSAny? _queryValidate(JSString astJson, JSString fieldsJson);

@JS('queryStringify')
external JSAny? _queryStringify(JSString astJson);

@JS('queryParseAndValidate')
external JSAny? _queryParseAndValidate(JSString query, JSString fieldsJson);

@JS('queryMatch')
external JSAny? _queryMatch(JSString query, JSString fieldsJson, JSString recordJson);

@JS('queryEval')
external JSAny? _queryEval(JSString query, JSString fieldsJson, JSString recordJson);

@JS('JSON.stringify')
external JSString _jsonStringify(JSAny? value);

@JS('Go')
extension type _Go._(JSObject _) implements JSObject {
  external _Go();
  external JSObject get importObject;
  external void run(JSObject instance);
}

@JS('WebAssembly.instantiate')
external JSPromise<_WasmResult> _wasmInstantiate(
  JSArrayBuffer bytes,
  JSObject importObject,
);

extension type _WasmResult._(JSObject _) implements JSObject {
  external JSObject get instance;
}

bool _started = false;

/// Loads the web backend backed by the query WASM build. [wasmUrl] overrides
/// where `query.wasm` is fetched from (the `wasm_exec.js` glue is expected
/// alongside the host page). [libraryPath] is ignored on the web.
Future<Backend> openBackend({String? libraryPath, String? wasmUrl}) async {
  await _ensureStarted(wasmUrl ?? 'query.wasm');
  return const _WebBackend();
}

Future<void> _ensureStarted(String wasmUrl) async {
  if (_started) return;
  await _injectScript('wasm_exec.js');
  final go = _Go();
  final resp = await web.window.fetch(wasmUrl.toJS).toDart;
  final bytes = await resp.arrayBuffer().toDart;
  final result = await _wasmInstantiate(bytes, go.importObject).toDart;
  // go.run resolves only when the Go program exits, so it must NOT be awaited —
  // awaiting would block until teardown and the globals would never be usable.
  go.run(result.instance);
  _started = true;
}

Future<void> _injectScript(String src) {
  final completer = Completer<void>();
  final script = web.HTMLScriptElement()..src = src;
  script.addEventListener('load', ((web.Event _) => completer.complete()).toJS);
  script.addEventListener(
    'error',
    ((web.Event _) =>
            completer.completeError(StateError('failed to load $src')))
        .toJS,
  );
  web.document.head!.appendChild(script);
  return completer.future;
}

/// Routes the uniform JSON request contract to the positional WASM globals.
class _WebBackend implements Backend {
  const _WebBackend();

  String _stringify(JSAny? value) {
    if (value == null) return '{"error":"null response from engine"}';
    return _jsonStringify(value).toDart;
  }

  Map<String, Object?> _req(String requestJson) =>
      jsonDecode(requestJson) as Map<String, Object?>;

  @override
  Future<String> parse(String requestJson) async {
    final req = _req(requestJson);
    final query = req['query'] as String? ?? '';
    final maxLength = (req['maxLength'] as num? ?? 256).toInt();
    return _stringify(_queryParse(query.toJS, maxLength.toJS));
  }

  @override
  Future<String> validate(String requestJson) async {
    final req = _req(requestJson);
    return _stringify(
      _queryValidate(jsonEncode(req['ast']).toJS, jsonEncode(req['fields']).toJS),
    );
  }

  @override
  Future<String> stringify(String requestJson) async {
    final req = _req(requestJson);
    return _stringify(_queryStringify(jsonEncode(req['ast']).toJS));
  }

  @override
  Future<String> parseAndValidate(String requestJson) async {
    final req = _req(requestJson);
    final query = req['query'] as String? ?? '';
    return _stringify(
      _queryParseAndValidate(query.toJS, jsonEncode(req['fields']).toJS),
    );
  }

  @override
  Future<String> match(String requestJson) async {
    final req = _req(requestJson);
    final query = req['query'] as String? ?? '';
    return _stringify(_queryMatch(
      query.toJS,
      jsonEncode(req['fields']).toJS,
      jsonEncode(req['record'] ?? <String, Object?>{}).toJS,
    ));
  }

  @override
  Future<String> eval(String requestJson) async {
    final req = _req(requestJson);
    final query = req['query'] as String? ?? '';
    return _stringify(_queryEval(
      query.toJS,
      jsonEncode(req['fields']).toJS,
      jsonEncode(req['record'] ?? <String, Object?>{}).toJS,
    ));
  }
}
