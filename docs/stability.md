---
title: Stability
nav_order: 8
permalink: /stability/
---

# Stability

What you can rely on, and what holds the library to it.

| Promise | Held by |
|---|---|
| A query keeps its meaning. | A golden file records the printed form and the JSON tree, or the error codes, for several hundred queries. Any change shows up as a diff of that file and has to be made on purpose. |
| Error codes never change once shipped. | The same file, plus a test that requires an input for every code. English messages may be reworded. |
| Every client agrees. | One corpus runs against the Go API, the JavaScript client and the C library. |
| The evaluator agrees with SQL. | Every case in a second corpus runs through the evaluator and a real SQLite database. |
| No input panics, and printing a query gives a query that parses to the same tree. | Four fuzz targets, run on every push and for longer every week. |
| A compiled program is safe to share between goroutines. | Race-detector runs of concurrent matching and validation. |
| A record cannot crash the evaluator. | Tests match against channels, functions, nil pointers, NaN, self-referencing maps and other values a record should never hold. |
| Your own functions cannot crash it either. | A panic inside a registered function becomes an error from that call. |
| It behaves the same everywhere Go runs. | CI runs the tests on Linux, macOS and Windows, on the oldest supported Go and the newest, and on a 32-bit build. |

## Limits on every query

- 256 bytes, unless the host sets another limit.
- 128 levels of nesting.
- About 10,000 years in either direction for day arithmetic.

A query runs in time proportional to its length and the size of the record. The language has no loops, no recursion and no user-defined functions.

## Versions

Until the first tagged release the Go API may still change between commits, so pin a commit. From `v1.0.0` on, versions follow semantic versioning: the grammar, the error codes, the JSON tree and the exported Go API only change incompatibly in a major version.

Changes are listed in the [changelog](https://github.com/heyllave/query/blob/main/CHANGELOG.md).

## Security

The library treats every query as hostile. What counts as a vulnerability, and how to report one privately, is in [SECURITY.md](https://github.com/heyllave/query/blob/main/SECURITY.md).
