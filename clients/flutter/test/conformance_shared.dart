// Platform-agnostic driver for the shared cross-language corpus
// (/conformance/corpus.json). Both the native FFI test and the browser WASM test
// register the same cases through [defineConformanceTests], differing only in how
// they load the engine and the corpus — so a single corpus proves the FFI and
// WASM backends agree with the Go and JS clients. No dart:io here, so it compiles
// for the browser.

import 'package:test/test.dart';
import 'package:trazo_query/trazo_query.dart';

const _typeByOrdinal = {
  0: FieldValueType.text,
  1: FieldValueType.integer,
  2: FieldValueType.decimal,
  3: FieldValueType.boolean,
  4: FieldValueType.date,
  5: FieldValueType.datetime,
  6: FieldValueType.duration,
};

List<FieldConfig> _fields(List<Object?> raw) => raw.map((e) {
      final m = e as Map<String, Object?>;
      final ops =
          (m['AllowedOps'] as List<Object?>).map((o) => o as String).toList();
      return FieldConfig(
        name: m['Name'] as String,
        type: _typeByOrdinal[(m['Type'] as num).toInt()]!,
        allowedOps: ops,
      );
    }).toList();

/// Registers the corpus count check plus one test per corpus case against the
/// engine returned by [load]. [corpus] is the decoded corpus.json; [expectedCases]
/// guards against a silently shrunk corpus.
void defineConformanceTests({
  required int expectedCases,
  required Map<String, Object?> corpus,
  required Future<TrazoQuery> Function() load,
}) {
  final fieldSets = corpus['fieldSets'] as Map<String, Object?>;
  final cases = corpus['cases'] as List<Object?>;

  late TrazoQuery q;
  setUpAll(() async => q = await load());

  test('corpus has at least $expectedCases cases', () {
    expect(cases.length, greaterThanOrEqualTo(expectedCases),
        reason: 'corpus has ${cases.length} cases (stale?)');
  });

  List<FieldConfig> fieldsFor(Map<String, Object?> tc) {
    final set = tc['fieldSet'];
    if (set is String) return _fields(fieldSets[set] as List<Object?>);
    return _fields((tc['fields'] as List<Object?>?) ?? const []);
  }

  for (final raw in cases) {
    final tc = raw as Map<String, Object?>;
    final op = tc['op'] as String;
    final id = tc['id'] as String;
    final wantErr = tc['expectError'] == true;

    test('$op: $id', () async {
      final query = tc['query'] as String;
      final record = (tc['record'] as Map<String, Object?>?) ?? const {};

      switch (op) {
        case 'parse':
          final r = await q.parse(query);
          expect(r.ok, !wantErr, reason: r.error);
          if (!wantErr && tc['expectAst'] != null) {
            expect(r.ast?['type'], tc['expectAst']);
          }
        case 'parseAndValidate':
          final r = await q.parseAndValidate(query, fieldsFor(tc));
          expect(r.ok, !wantErr, reason: r.error);
          if (!wantErr && tc['expectAst'] != null) {
            expect(r.ast?['type'], tc['expectAst']);
          }
        case 'stringify':
          final parsed = await q.parse(query);
          final r = await q.stringify(parsed.ast!);
          if (tc['expectString'] != null) {
            expect(r.query, tc['expectString']);
          }
        case 'match':
          final r = await q.match(query, fieldsFor(tc), record);
          if (wantErr) {
            expect(r.ok, isFalse);
          } else {
            expect(r.ok, isTrue, reason: r.error);
            if (tc.containsKey('expectMatch')) {
              expect(r.matched, tc['expectMatch']);
            }
          }
        case 'eval':
          final r = await q.eval(query, fieldsFor(tc), record);
          if (wantErr) {
            expect(r.ok, isFalse);
          } else {
            expect(r.ok, isTrue, reason: r.error);
            if (tc.containsKey('expectValue')) {
              final tol = (tc['tolerance'] as num?)?.toDouble() ?? 1e-9;
              expect((r.value as num).toDouble(),
                  closeTo((tc['expectValue'] as num).toDouble(), tol));
            }
          }
        default:
          fail('unknown op $op');
      }
    });
  }
}
