/// Field value types, matching Go's validate.FieldValueType ordinals. Sent as
/// the integer ordinal in [FieldConfig.toJson] because the Go decoder reads the
/// numeric enum, not the name.
enum FieldValueType {
  text(0),
  integer(1),
  decimal(2),
  boolean(3),
  date(4),
  datetime(5),
  duration(6);

  const FieldValueType(this.ordinal);
  final int ordinal;
}

/// A field declaration the engine validates queries against. JSON keys are
/// PascalCase to match the Go struct field names (which have no json tags), so a
/// misspelled key would silently zero-value the Go struct.
class FieldConfig {
  const FieldConfig({
    required this.name,
    required this.type,
    required this.allowedOps,
    this.searchable = false,
    this.nested = false,
  });

  final String name;
  final FieldValueType type;
  final List<String> allowedOps;
  final bool searchable;
  final bool nested;

  Map<String, Object?> toJson() => {
        'Name': name,
        'Type': type.ordinal,
        'AllowedOps': allowedOps,
        'Searchable': searchable,
        'Nested': nested,
      };
}

/// A record of field values to evaluate a query against. Named QueryRecord so it
/// does not shadow any Dart core type.
typedef QueryRecord = Map<String, Object?>;

/// Result of [TrazoQuery.parse] / [TrazoQuery.parseAndValidate]: the AST as a
/// decoded JSON map, or an error message.
class ParseResult {
  const ParseResult({this.ast, this.error});
  final Map<String, Object?>? ast;
  final String? error;
  bool get ok => error == null;
}

/// Result of [TrazoQuery.validate].
class ValidateResult {
  const ValidateResult({required this.valid, this.errors = const []});
  final bool valid;
  final List<String> errors;
}

/// Result of [TrazoQuery.stringify].
class StringifyResult {
  const StringifyResult({this.query, this.error});
  final String? query;
  final String? error;
  bool get ok => error == null;
}

/// Result of [TrazoQuery.match]: a boolean predicate outcome, or an error.
class MatchResult {
  const MatchResult({this.matched, this.error});
  final bool? matched;
  final String? error;
  bool get ok => error == null;
}

/// Result of [TrazoQuery.eval]: the computed value, or an error (including the
/// "did not resolve" case for a missing field or division by zero).
class EvalResult {
  const EvalResult({this.value, this.error});
  final Object? value;
  final String? error;
  bool get ok => error == null;
}
