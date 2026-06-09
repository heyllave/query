#
# Podspec for the trazo_query_flutter FFI plugin. It vendors the prebuilt
# libquery.xcframework (built by `make -C clients/ffi ios`) — there is no native
# source compiled here.
#
Pod::Spec.new do |s|
  s.name             = 'trazo_query_flutter'
  s.version          = '0.1.0'
  s.summary          = 'Flutter packaging for the Trazo query engine (native libquery).'
  s.description      = 'Bundles the prebuilt libquery xcframework for iOS and re-exports the trazo_query API.'
  s.homepage         = 'https://github.com/heyllave/query'
  s.license          = { :type => 'Apache-2.0' }
  s.author           = { 'Trazo' => 'dev@heyllave.local' }
  s.source           = { :path => '.' }
  s.dependency 'Flutter'
  s.platform = :ios, '13.0'

  # Flutter plugins link statically, and the engine is a static archive inside
  # the xcframework.
  s.static_framework    = true
  s.vendored_frameworks = 'Frameworks/libquery.xcframework'

  # The cgo exports are resolved at runtime via dart:ffi DynamicLibrary.process(),
  # so nothing references them at link time and the linker would dead-strip the
  # archive. -force_load keeps the whole slice. The slice path differs between
  # device and simulator builds, so the flag is set per SDK.
  s.pod_target_xcconfig = {
    'DEFINES_MODULE' => 'YES',
    'OTHER_LDFLAGS[sdk=iphoneos*]' =>
      '-force_load "$(PODS_TARGET_SRCROOT)/Frameworks/libquery.xcframework/ios-arm64/libquery.a"',
    'OTHER_LDFLAGS[sdk=iphonesimulator*]' =>
      '-force_load "$(PODS_TARGET_SRCROOT)/Frameworks/libquery.xcframework/ios-arm64_x86_64-simulator/libquery.a"',
  }
end
