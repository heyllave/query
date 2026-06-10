#
# Podspec for trazo_query_flutter on macOS desktop. It vendors the prebuilt
# libquery.dylib (built by `make -C clients/ffi desktop-plugin` on macOS and
# staged here) — there is no native source compiled.
#
Pod::Spec.new do |s|
  s.name             = 'trazo_query_flutter'
  s.version          = '0.1.0'
  s.summary          = 'Flutter packaging for the Trazo query engine (native libquery).'
  s.description      = 'Bundles the prebuilt libquery dylib for macOS and re-exports the trazo_query API.'
  s.homepage         = 'https://github.com/heyllave/query'
  s.license          = { :type => 'Apache-2.0' }
  s.author           = { 'Trazo' => 'dev@heyllave.local' }
  s.source           = { :path => '.' }
  s.dependency 'FlutterMacOS'
  s.platform = :osx, '10.14'

  # The dylib is copied into the app bundle's Frameworks; dart:ffi opens it by
  # name. @loader_path keeps the install name relative to the bundle.
  s.vendored_libraries = 'libquery.dylib'
  s.pod_target_xcconfig = {
    'DEFINES_MODULE' => 'YES',
    'LD_RUNPATH_SEARCH_PATHS' => '@loader_path @executable_path/../Frameworks',
  }
end
