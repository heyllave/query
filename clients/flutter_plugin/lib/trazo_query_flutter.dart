/// Flutter packaging for the Trazo query engine.
///
/// Depend on this package from a Flutter app to bundle the prebuilt native
/// `libquery` (Android `.so` per ABI, iOS xcframework) so the FFI backend has a
/// library to load on a device. The query API itself comes from the pure-Dart
/// [trazo_query](../flutter) package and is re-exported here, so an app needs a
/// single import and a single dependency:
///
/// ```dart
/// import 'package:trazo_query_flutter/trazo_query_flutter.dart';
///
/// final q = await TrazoQuery.load();
/// ```
///
/// This package contains no query logic — only the native binaries and this
/// re-export.
library;

export 'package:trazo_query/trazo_query.dart';
