@TestOn('vm')
library;

import 'dart:io';

import 'package:test/test.dart';
import 'package:trazo_query/trazo_query.dart';

const _fields = [
  FieldConfig(name: 'state', type: FieldValueType.text, allowedOps: ['=', '!=', '*', '?']),
  FieldConfig(
    name: 'total',
    type: FieldValueType.decimal,
    allowedOps: ['=', '!=', '>', '>=', '<', '<=', '..'],
  ),
  FieldConfig(
    name: 'base',
    type: FieldValueType.decimal,
    allowedOps: ['=', '!=', '>', '>=', '<', '<=', '..'],
  ),
];

String _libPath() {
  if (Platform.isMacOS) return 'native/libquery.dylib';
  if (Platform.isWindows) return 'native/libquery.dll';
  return 'native/libquery.so';
}

void main() {
  late TrazoQuery q;

  setUpAll(() async {
    final lib = _libPath();
    if (!File(lib).existsSync()) {
      throw StateError('missing $lib — run `make -C clients/ffi build` first');
    }
    q = await TrazoQuery.load(libraryPath: lib);
  });

  test('parse returns the AST node type', () async {
    final r = await q.parse('state=draft AND total>50000');
    expect(r.ok, isTrue, reason: r.error);
    expect(r.ast?['type'], 'binary');
  });

  test('parse surfaces a syntax error', () async {
    final r = await q.parse('=invalid');
    expect(r.ok, isFalse);
    expect(r.error, isNotEmpty);
  });

  test('parseAndValidate rejects an unknown field', () async {
    final r = await q.parseAndValidate('nope=x', _fields);
    expect(r.ok, isFalse);
  });

  test('stringify round-trips a compound query', () async {
    final parsed = await q.parse('state=draft AND total>50000');
    final r = await q.stringify(parsed.ast!);
    expect(r.query, 'state=draft AND total>50000');
  });

  test('validate accepts a valid AST', () async {
    final parsed = await q.parse('state=draft');
    final r = await q.validate(parsed.ast!, _fields);
    expect(r.valid, isTrue, reason: r.errors.join('; '));
  });

  test('match evaluates a boolean predicate', () async {
    final r = await q.match(
      'state=draft AND total>50000',
      _fields,
      {'state': 'draft', 'total': 60000},
    );
    expect(r.ok, isTrue, reason: r.error);
    expect(r.matched, isTrue);
  });

  test('match supports cross-field comparison', () async {
    final r = await q.match('total>[base]', _fields, {'total': 100, 'base': 50});
    expect(r.matched, isTrue);
  });

  test('eval computes a value expression', () async {
    final r = await q.eval('[base]*2', _fields, {'base': 21});
    expect(r.ok, isTrue, reason: r.error);
    expect((r.value as num).toDouble(), closeTo(42, 1e-9));
  });

  test('eval reports an unresolvable expression', () async {
    final r = await q.eval('5/0', _fields, {});
    expect(r.ok, isFalse);
    expect(r.error, isNotEmpty);
  });
}
