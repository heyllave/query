# trazo_query_flutter — Linux desktop.
#
# This plugin compiles no native code: it bundles the prebuilt libquery.so
# (built by `make -C clients/ffi build` and staged here by the plugin Makefile)
# so it ships in the app bundle's lib/ dir, where dart:ffi opens it by soname
# (DynamicLibrary.open('libquery.so')).
cmake_minimum_required(VERSION 3.10)

set(PROJECT_NAME "trazo_query_flutter")
project(${PROJECT_NAME} LANGUAGES CXX)

set(_libquery "${CMAKE_CURRENT_SOURCE_DIR}/libquery.so")
if(NOT EXISTS "${_libquery}")
  message(FATAL_ERROR
    "Missing ${_libquery}. Build it first: make -C clients/ffi desktop-plugin")
endif()

# Absolute paths to libraries Flutter copies into the app bundle.
set(trazo_query_flutter_bundled_libraries
  "${_libquery}"
  PARENT_SCOPE
)
