# trazo_query (Dart & Flutter)

Dart client for the Trazo query language. It runs the **Go engine** itself — not
a Dart re-implementation — so parse, validate, match, and eval results match the
Go and JavaScript clients exactly. A shared
[conformance corpus](../../conformance/corpus.json) is run by all three clients
in CI, so any grammar drift fails the build.

Two backends are selected automatically at compile time:

| Target | Backend | Mechanism |
|--------|---------|-----------|
| Desktop / mobile (`dart:io`) | native shared library | cgo c-shared via `dart:ffi` |
| Web (`dart:js_interop`) | WASM | the WASM build loaded in the browser |

## Usage

```dart
import 'package:trazo_query/trazo_query.dart';

final q = await TrazoQuery.load();

const fields = [
  FieldConfig(name: 'state', type: FieldValueType.text,    allowedOps: ['=', '!=']),
  FieldConfig(name: 'total', type: FieldValueType.decimal, allowedOps: ['>', '>=', '<', '<=']),
  FieldConfig(name: 'base',  type: FieldValueType.decimal, allowedOps: ['>', '>=', '<', '<=']),
];

// Parse to an AST.
final parsed = await q.parse('state=draft AND total>50000');
print(parsed.ast?['type']); // binary

// Boolean predicate against a record (cross-field comparison supported).
final m = await q.match('total>[base]', fields, {'total': 100, 'base': 50});
print(m.matched); // true

// Value expression — returns the computed value.
final v = await q.eval('[base]*2', fields, {'base': 21});
print(v.value); // 42

// Unresolvable expressions surface as an error, not an exception.
final bad = await q.eval('5/0', fields, {});
print(bad.ok);    // false
print(bad.error); // division by zero ...
```

Every method returns a typed result with an `ok`/`error` (or `valid`/`errors`)
field rather than throwing on engine-level failures.

## Native setup

The FFI backend loads `libquery` (`.so` / `.dylib` / `.dll`). Build it from the
repo root:

```bash
make -C clients/ffi build
```

This compiles the cgo c-shared library and copies it to `clients/flutter/native/`.
`TrazoQuery.load()` looks for `native/libquery.<ext>` by default; pass
`libraryPath:` to point elsewhere (e.g. when bundling with an app):

```dart
final q = await TrazoQuery.load(libraryPath: '/opt/app/libquery.so');
```

The library must be built for each platform you ship to. Producing the
per-platform binary matrix (Linux/macOS/Windows, Android `.so` ABIs, iOS
`.framework`) and bundling it through Flutter's plugin tooling is a follow-up.

## Web setup

The web backend fetches `query.wasm` and loads Go's `wasm_exec.js`. Build them:

```bash
make -C wasm build   # writes query.wasm and src/wasm_exec.js into clients/npm
```

Serve `query.wasm` and `wasm_exec.js` next to your page (or pass
`wasmUrl:` to `TrazoQuery.load()` to override the `.wasm` location).

## Tests

```bash
make -C clients/ffi build         # the native lib the tests load
cd clients/flutter
dart pub get
dart analyze --fatal-infos
dart test                         # client + conformance, on the Dart VM (FFI)
```

`dart test` exercises the native FFI backend. Running the web/WASM backend needs
a browser (`dart test -p chrome`) with `query.wasm` + `wasm_exec.js` served; that
is a follow-up.
