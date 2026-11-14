#!/usr/bin/env python3
"""Runs the shared conformance corpus through libquery from Python.

This is what a binding in another language looks like: load the shared
library, send one JSON object per call, read one back, free it. It uses the
standard library only. The same corpus runs against the Go API and the
JavaScript/WASM client, so a difference between them fails a build.

    python3 conformance_test.py ./libquery.so
"""

import ctypes
import json
import os
import sys
import threading

HERE = os.path.dirname(os.path.abspath(__file__))
CORPUS = os.path.join(HERE, "..", "..", "conformance", "corpus.json")
MIN_CASES = 135

CALLS = ["QueryParse", "QueryValidate", "QueryStringify",
         "QueryParseAndValidate", "QueryMatch", "QueryEval"]


class Library:
    def __init__(self, path):
        self.lib = ctypes.CDLL(os.path.abspath(path))
        for name in CALLS:
            fn = getattr(self.lib, name)
            fn.argtypes = [ctypes.c_char_p]
            fn.restype = ctypes.c_void_p
        self.lib.QueryFree.argtypes = [ctypes.c_void_p]
        self.lib.QueryFree.restype = None

    def raw(self, name, payload):
        """Send bytes (or NULL), return the decoded envelope, free the reply."""
        ptr = getattr(self.lib, name)(payload)
        if not ptr:
            raise AssertionError(f"{name} returned NULL")
        try:
            return json.loads(ctypes.string_at(ptr).decode("utf-8"))
        finally:
            self.lib.QueryFree(ptr)

    def call(self, name, **request):
        return self.raw(name, json.dumps(request).encode("utf-8"))


def first_code(env):
    for key in ("errors", "validationErrors"):
        if env.get(key):
            return env[key][0].get("code")
    return None


def run_case(lib, corpus, tc):
    fields = corpus["fieldSets"][tc["fieldSet"]] if tc.get("fieldSet") else tc.get("fields", [])
    record = tc.get("record", {})
    query = tc.get("query", "")
    op = tc["op"]

    if op == "parse":
        env = lib.call("QueryParse", query=query)
    elif op == "parseAndValidate":
        env = lib.call("QueryParseAndValidate", query=query, fields=fields)
    elif op == "stringify":
        tree = lib.call("QueryParse", query=query)["result"]
        env = lib.call("QueryStringify", ast=tree)
    elif op == "match":
        env = lib.call("QueryMatch", query=query, fields=fields, record=record)
    elif op == "eval":
        env = lib.call("QueryEval", query=query, fields=fields, record=record)
    else:
        raise AssertionError(f"unknown op {op}")

    error = env.get("error")
    assert not (error or "").startswith("internal error"), f"library panicked: {error}"
    if tc.get("expectError"):
        assert error, f"expected an error, got {env}"
        if tc.get("expectCode"):
            assert first_code(env) == tc["expectCode"], f"code {first_code(env)}, want {tc['expectCode']}"
        return
    assert error is None, f"unexpected error: {error}"
    if tc.get("expectAst"):
        assert env["result"]["type"] == tc["expectAst"], f"ast type {env['result']['type']}"
    if tc.get("expectString"):
        assert env["result"] == tc["expectString"], f"string {env['result']!r}"
    if "expectMatch" in tc:
        assert env["result"] is tc["expectMatch"], f"match {env['result']}"
    if "expectValue" in tc:
        tolerance = tc.get("tolerance", 1e-9)
        assert abs(float(env["result"]) - tc["expectValue"]) <= tolerance, f"value {env['result']}"


def check_hostile_input(lib):
    """No request, however malformed, may crash the process or return NULL."""
    payloads = [None, b"", b"null", b"[]", b"{", b"\xff\xfe", b'{"query":1}',
                b'{"query":"a=1","fields":"x","record":[]}',
                b'{"ast":{"type":"binary"},"fields":[]}',
                b'{"query":"' + b"(" * 5000 + b'a","maxLength":0}',
                b'{"query":"a=1","maxLength":"x"}']
    for name in CALLS:
        for payload in payloads:
            env = lib.raw(name, payload)
            error = env.get("error", "")
            assert not error.startswith("internal error"), f"{name}({payload!r}) panicked: {error}"


def check_threads(lib):
    """Calls from many threads at once agree with a single-threaded call."""
    fields = [{"name": "total", "type": "decimal"}]
    failures = []

    def work():
        for i in range(300):
            env = lib.call("QueryMatch", query="total>50", fields=fields, record={"total": i})
            if env.get("result") is not (i > 50):
                failures.append((i, env))

    threads = [threading.Thread(target=work) for _ in range(8)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert not failures, f"wrong answers under concurrency: {failures[:3]}"


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: conformance_test.py <path to libquery>")
    lib = Library(sys.argv[1])
    with open(CORPUS, encoding="utf-8") as f:
        corpus = json.load(f)
    cases = corpus["cases"]
    assert len(cases) >= MIN_CASES, f"corpus has {len(cases)} cases, expected >= {MIN_CASES} (stale?)"

    failed = 0
    for tc in cases:
        try:
            run_case(lib, corpus, tc)
        except AssertionError as err:
            failed += 1
            print(f"FAIL {tc['op']}: {tc['id']}: {err}")
    for name, check in (("hostile input", check_hostile_input), ("threads", check_threads)):
        try:
            check(lib)
        except AssertionError as err:
            failed += 1
            print(f"FAIL {name}: {err}")

    print(f"{len(cases) + 2 - failed} passed, {failed} failed")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
