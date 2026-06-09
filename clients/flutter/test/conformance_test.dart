@TestOn('vm')
library;

// Runs the shared cross-language corpus (/conformance/corpus.json) against the
// native FFI build. The Go and JS clients run the same file, so grammar drift
// between implementations becomes a failing build.

import 'dart:convert';
import 'dart:io';

import 'package:test/test.dart';
import 'package:trazo_query/trazo_query.dart';

const _expectedCases = 25;

const _typeByOrdinal = {
  0: FieldValueType.text,
  1: FieldValueType.integer,
  2: FieldValueType.decimal,
  3: FieldValueType.boolean,
  4: FieldValueType.date,
  5: FieldValueType.datetime,
  6: FieldValueType.duration,
};

String _libPath() {
  if (Platform.isMacOS) return 'native/libquery.dylib';
  if (Platform.isWindows) return 'native/libquery.dll';
  return 'native/libquery.so';
}

List<FieldConfig> _fields(List<Object?> raw) => raw.map((e) {
      final m = e as Map<String, Object?>;
      final ops = (m['AllowedOps'] as List<Object?>).map((o) => o as String).toList();
      return FieldConfig(
        name: m['Name'] as String,
        type: _typeByOrdinal[(m['Type'] as num).toInt()]!,
        allowedOps: ops,
      );
    }).toList();

void main() {
  final corpus = jsonDecode(
    File('../../conformance/corpus.json').readAsStringSync(),
  ) as Map<String, Object?>;
  final fieldSets = corpus['fieldSets'] as Map<String, Object?>;
  final cases = corpus['cases'] as List<Object?>;

  late TrazoQuery q;

  setUpAll(() async {
    final lib = _libPath();
    if (!File(lib).existsSync()) {
      throw StateError('missing $lib — run `make -C clients/ffi build` first');
    }
    q = await TrazoQuery.load(libraryPath: lib);
  });

  test('corpus has at least $_expectedCases cases', () {
    expect(cases.length, greaterThanOrEqualTo(_expectedCases),
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
