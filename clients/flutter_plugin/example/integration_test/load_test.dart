// On-device proof that the bundled native engine loads and its FFI symbols
// resolve. This is the only validation that exercises the prebuilt Android .so /
// iOS xcframework — run it on a real device or emulator:
//
//   flutter test integration_test/load_test.dart
//
// (from clients/flutter_plugin/example, after `make -C clients/ffi android`
// and/or `make -C clients/ffi ios` have produced the binaries).

import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:trazo_query_flutter/trazo_query_flutter.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  const fields = [
    FieldConfig(name: 'state', type: FieldValueType.text, allowedOps: ['=']),
    FieldConfig(
      name: 'total',
      type: FieldValueType.decimal,
      allowedOps: ['>', '>=', '<', '<='],
    ),
    FieldConfig(
      name: 'base',
      type: FieldValueType.decimal,
      allowedOps: ['>', '>=', '<', '<='],
    ),
  ];

  late TrazoQuery q;

  setUpAll(() async => q = await loadTrazoQuery());

  test('loads the native engine and parses', () async {
    final r = await q.parse('state=draft AND total>50000');
    expect(r.ok, isTrue, reason: r.error);
    expect(r.ast?['type'], 'binary');
  });

  test('matches a boolean predicate', () async {
    final r = await q.match(
      'state=draft AND total>50000',
      fields,
      {'state': 'draft', 'total': 60000},
    );
    expect(r.ok, isTrue, reason: r.error);
    expect(r.matched, isTrue);
  });

  test('cross-field comparison resolves', () async {
    final r = await q.match('total>[base]', fields, {'total': 100, 'base': 50});
    expect(r.matched, isTrue);
  });

  test('evaluates a value expression', () async {
    final r = await q.eval('[base]*2', fields, {'base': 21});
    expect(r.ok, isTrue, reason: r.error);
    expect((r.value as num).toDouble(), closeTo(42, 1e-9));
  });
}
