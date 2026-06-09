import 'dart:convert';

import 'backend.dart';
import 'json_codec.dart';
import 'models.dart';

/// The Trazo query engine for Dart and Flutter.
///
/// One [TrazoQuery] wraps a loaded backend — the Go engine compiled to a native
/// shared library (FFI) on desktop/mobile, or to WASM on the web. Both backends
/// run the identical engine, so results match the Go and JS clients byte for
/// byte (the shared conformance corpus enforces this).
///
/// Load once and reuse:
///
/// ```dart
/// final q = await TrazoQuery.load();
/// final r = await q.parse('state=draft AND total>50000');
/// if (r.ok) print(r.ast);
/// ```
class TrazoQuery {
  TrazoQuery._(this._backend);

  final Backend _backend;

  /// Loads the engine for the current platform.
  ///
  /// On native targets [libraryPath] overrides the shared-library path (defaults
  /// to the platform's `libquery` next to the executable). On the web [wasmUrl]
  /// overrides where `query.wasm` is fetched from. Both are ignored by the other
  /// platform's backend.
  static Future<TrazoQuery> load({String? libraryPath, String? wasmUrl}) async {
    final backend = await openBackend(libraryPath: libraryPath, wasmUrl: wasmUrl);
    return TrazoQuery._(backend);
  }

  /// Parses [query] into an AST. [maxLength] bounds the input size the lexer
  /// will accept.
  Future<ParseResult> parse(String query, {int maxLength = 256}) async {
    final req = jsonEncode({'query': query, 'maxLength': maxLength});
    return decodeParse(await _backend.parse(req));
  }

  /// Validates an already-parsed [ast] against [fields].
  Future<ValidateResult> validate(
    Map<String, Object?> ast,
    List<FieldConfig> fields,
  ) async {
    final req = jsonEncode({'ast': ast, 'fields': encodeFields(fields)});
    return decodeValidate(await _backend.validate(req));
  }

  /// Renders an [ast] back to its canonical query string.
  Future<StringifyResult> stringify(Map<String, Object?> ast) async {
    final req = jsonEncode({'ast': ast});
    return decodeStringify(await _backend.stringify(req));
  }

  /// Parses and validates [query] against [fields] in one call.
  Future<ParseResult> parseAndValidate(
    String query,
    List<FieldConfig> fields,
  ) async {
    final req = jsonEncode({'query': query, 'fields': encodeFields(fields)});
    return decodeParse(await _backend.parseAndValidate(req));
  }

  /// Compiles [query] as a boolean predicate and evaluates it against [record].
  Future<MatchResult> match(
    String query,
    List<FieldConfig> fields,
    QueryRecord record,
  ) async {
    final req = jsonEncode({
      'query': query,
      'fields': encodeFields(fields),
      'record': record,
    });
    return decodeMatch(await _backend.match(req));
  }

  /// Compiles [query] as a value expression and evaluates it against [record],
  /// returning the computed value. Unresolvable expressions (a missing field, a
  /// division by zero) surface as [EvalResult.error].
  Future<EvalResult> eval(
    String query,
    List<FieldConfig> fields,
    QueryRecord record,
  ) async {
    final req = jsonEncode({
      'query': query,
      'fields': encodeFields(fields),
      'record': record,
    });
    return decodeEval(await _backend.eval(req));
  }
}
