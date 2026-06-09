/// Trazo query language for Dart and Flutter.
///
/// A thin client over the Go query engine: parse, validate, render, and evaluate
/// queries through the same engine the Go and JS clients use. On native targets
/// it calls a cgo c-shared library over `dart:ffi`; on the web it calls the WASM
/// build. The backend is selected at compile time — callers only see
/// [TrazoQuery].
library;

export 'src/models.dart';
export 'src/query_api.dart' show TrazoQuery;
