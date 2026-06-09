import 'dart:convert';

import 'models.dart';

/// Encodes a list of field configs to the JSON the engine expects (PascalCase
/// keys, integer type ordinals).
List<Map<String, Object?>> encodeFields(List<FieldConfig> fields) =>
    fields.map((f) => f.toJson()).toList();

/// Decodes the `{result|error}` envelope from a parse-style call.
ParseResult decodeParse(String responseJson) {
  final m = _decode(responseJson);
  final err = m['error'];
  if (err is String) return ParseResult(error: err);
  final ast = m['result'];
  if (ast is Map<String, Object?>) return ParseResult(ast: ast);
  return const ParseResult(error: 'malformed response: missing result');
}

/// Decodes the `{valid, errors}` envelope from a validate call. An `error` key
/// (a bridge-level failure such as a bad AST) surfaces as a single error.
ValidateResult decodeValidate(String responseJson) {
  final m = _decode(responseJson);
  final err = m['error'];
  if (err is String) return ValidateResult(valid: false, errors: [err]);
  final valid = m['valid'] == true;
  final raw = m['errors'];
  final errors = raw is List ? raw.map((e) => e.toString()).toList() : <String>[];
  return ValidateResult(valid: valid, errors: errors);
}

/// Decodes the `{result|error}` envelope from a stringify call.
StringifyResult decodeStringify(String responseJson) {
  final m = _decode(responseJson);
  final err = m['error'];
  if (err is String) return StringifyResult(error: err);
  final s = m['result'];
  if (s is String) return StringifyResult(query: s);
  return const StringifyResult(error: 'malformed response: missing result');
}

/// Decodes the `{result|error}` envelope from a match call.
MatchResult decodeMatch(String responseJson) {
  final m = _decode(responseJson);
  final err = m['error'];
  if (err is String) return MatchResult(error: err);
  final r = m['result'];
  if (r is bool) return MatchResult(matched: r);
  return const MatchResult(error: 'malformed response: missing result');
}

/// Decodes the `{result|error}` envelope from an eval call. The result may be
/// any JSON value (number, string, list, …).
EvalResult decodeEval(String responseJson) {
  final m = _decode(responseJson);
  final err = m['error'];
  if (err is String) return EvalResult(error: err);
  if (!m.containsKey('result')) {
    return const EvalResult(error: 'malformed response: missing result');
  }
  return EvalResult(value: m['result']);
}

Map<String, Object?> _decode(String responseJson) {
  final decoded = jsonDecode(responseJson);
  if (decoded is Map<String, Object?>) return decoded;
  return {'error': 'malformed response: not a JSON object'};
}
