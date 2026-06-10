# trazo_query_flutter — Windows desktop.
#
# Bundles the prebuilt libquery.dll (built by `make -C clients/ffi desktop-plugin`
# on Windows and staged here); compiles no native code. Flutter copies the DLL
# next to the app executable, where dart:ffi opens it by name
# (DynamicLibrary.open('libquery.dll')).
cmake_minimum_required(VERSION 3.14)

set(PROJECT_NAME "trazo_query_flutter")
project(${PROJECT_NAME} LANGUAGES CXX)

set(_libquery "${CMAKE_CURRENT_SOURCE_DIR}/libquery.dll")
if(NOT EXISTS "${_libquery}")
  message(FATAL_ERROR
    "Missing ${_libquery}. Build it first: make -C clients/ffi desktop-plugin")
endif()

set(trazo_query_flutter_bundled_libraries
  "${_libquery}"
  PARENT_SCOPE
)
