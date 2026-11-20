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
MIN_CASES = 155

CALLS = ["QueryParse", "QueryValidate", "QueryStringify",
         "QueryParseAndValidate", "QueryMatch", "QueryEval",
         "QueryComplete", "QueryOffer"]


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

    if op == "complete":
        return run_complete(lib, fields, tc)
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


def run_complete(lib, fields, tc):
    """A completion case, in bytes and in UTF-16 code units, and through QueryOffer."""
    strict = tc.get("strict", False)
    for utf16, cursor, replace in ((False, tc["cursor"], tc["expectReplace"]),
                                   (True, tc["cursorUtf16"], tc["expectReplaceUtf16"])):
        env = lib.call("QueryComplete", query=tc["query"], fields=fields, cursor=cursor, utf16=utf16,
                       explicitAnd=strict, strictValues=strict)
        assert env.get("error") is None, f"unexpected error: {env.get('error')}"
        c = env["result"]
        assert c["slot"] == tc["expectSlot"], f"slot {c['slot']}"
        assert c["prefix"] == tc["expectPrefix"], f"prefix {c['prefix']!r}"
        if "expectField" in tc:
            assert c.get("field", "") == tc["expectField"], f"field {c.get('field')!r}"
        assert [c["replace"]["offset"], c["replace"]["length"]] == replace, f"replace {c['replace']} (utf16={utf16})"
        assert [it["text"] for it in c["items"]] == tc["expectTexts"], f"items {[it['text'] for it in c['items']]}"
        if "candidates" in tc:
            offered = lib.call("QueryOffer", completion=c, candidates=tc["candidates"])
            assert offered.get("error") is None, f"offer: {offered.get('error')}"
            assert [it["text"] for it in offered["result"]] == tc["expectOffered"], f"offered {offered['result']}"


def check_completion_input(lib):
    """A cursor that is not a position in the query is the caller's mistake, and is said so."""
    fields = [{"name": "name", "type": "text"}]
    # Without a cursor the position is the end of the query, in either unit.
    for utf16 in (False, True):
        env = lib.call("QueryComplete", query="name=\u00e9", fields=fields, utf16=utf16)
        assert env["result"]["slot"] == "value" and env["result"]["prefix"] == "\u00e9", env
    assert lib.call("QueryComplete", query="name=", fields=fields, cursor=None)["result"]["slot"] == "value"
    for cursor in (-1, 99):
        env = lib.call("QueryComplete", query="name=x", fields=fields, cursor=cursor)
        assert "is not a position in the query" in env.get("error", ""), env
    # Inside the two bytes of a character.
    env = lib.call("QueryComplete", query="caf\u00e9=x", fields=fields, cursor=4)
    assert "cannot split a character" in env.get("error", ""), env
    for bad in ("3", 1.5, [], {}):
        env = lib.call("QueryComplete", query="name=x", fields=fields, cursor=bad)
        assert env.get("error") == "cursor must be an integer", env
    env = lib.call("QueryComplete", query="name=x", fields=fields, utf16="yes")
    assert env.get("error") == "utf16 must be a boolean", env
    env = lib.call("QueryComplete", query="name=x", fields=[{"type": "text"}])
    assert env.get("error", "").startswith("invalid fields config"), env
    env = lib.call("QueryComplete", query="name=x", fields=fields, record={})
    assert env.get("error", "").startswith('unknown key "record"'), env
    # QueryOffer refuses what it cannot read.
    assert lib.call("QueryOffer", completion={"slot": "bogus"}, candidates=[]).get("error", "").startswith("invalid completion")
    assert lib.call("QueryOffer", completion={"slot": "value"}, candidates="x").get("error") == "candidates must be an array of strings"
    assert lib.call("QueryOffer", completion={"slot": "value"}).get("error") == "candidates must be an array of strings"
    assert lib.call("QueryOffer", completion={"slot": "key"}, candidates=["a"]).get("error", "").startswith("invalid completion")
    assert lib.call("QueryOffer", completion={"slot": "value", "type": "integer"}, candidates=["3", "x y"])["result"] == [
        {"label": "3", "text": "3", "kind": "value"}, {"label": "x y", "text": '"x y"', "kind": "value"}]


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


def check_strict_options(lib):
    """The strict switches refuse the lenient forms, and are off unless asked for."""
    assert "result" in lib.call("QueryParse", query="a=1 b=2")
    assert first_code(lib.call("QueryParse", query="a=1 b=2", explicitAnd=True)) == "missingAnd"
    assert "result" in lib.call("QueryParse", query="a=1 AND b=2", explicitAnd=True, strictValues=True)
    assert "result" in lib.call("QueryParse", query="a=1+b=2")
    assert first_code(lib.call("QueryParse", query="a=1+b=2", strictValues=True)) == "unexpectedChar"
    fields = [{"name": "a", "type": "integer"}, {"name": "b", "type": "integer"}]
    env = lib.call("QueryParseAndValidate", query="a=1 b=2", fields=fields, explicitAnd=True)
    assert first_code(env) == "missingAnd", env
    env = lib.call("QueryMatch", query="a=1 b=2", fields=fields, record={"a": 1, "b": 2}, explicitAnd=True)
    assert first_code(env) == "missingAnd", env
    env = lib.call("QueryMatch", query="a=1 AND b=2", fields=fields, record={"a": 1, "b": 2}, explicitAnd=True)
    assert env.get("result") is True, env
    env = lib.call("QueryParse", query="a=1", explicitAnd="yes")
    assert "must be a boolean" in env.get("error", ""), env
    for bad in ("yes", 1, None):
        env = lib.call("QueryParse", query="a=1 b=2", strictValues=bad)
        assert env.get("error") == "strictValues must be a boolean", env
    # Two bad members always name the same one.
    for _ in range(20):
        env = lib.call("QueryParse", query="a=1", strictValues=1, explicitAnd=1)
        assert env.get("error") == "explicitAnd must be a boolean", env


def check_unknown_keys(lib):
    """A key a call does not take is an error, so a typo cannot pass for an option."""
    env = lib.call("QueryParse", query="a=1 b=2", strict=True)
    assert env.get("error", "").startswith('unknown key "strict"'), env
    assert "result" not in env, env
    # Two unknown keys always name the same one.
    for _ in range(20):
        env = lib.call("QueryParse", query="a=1", zeta=1, alpha=2)
        assert env.get("error", "").startswith('unknown key "alpha"'), env
    # The empty key is a key like any other, and does not hide the ones after it.
    assert lib.call("QueryParse", **{"query": "a=1", "": 0}).get("error", "").startswith('unknown key ""')
    env = lib.call("QueryParse", **{"query": "a=1 b=2", "strict": True, "": 0})
    assert env.get("error", "").startswith('unknown key ""') and "result" not in env, env
    # A key another call takes is still unknown to this one.
    assert lib.call("QueryParse", query="a=1", record={}).get("error", "").startswith('unknown key "record"')
    assert lib.call("QueryEval", query="1+1", fields=[], record={}, explicitAnd=True).get("error", "").startswith('unknown key "explicitAnd"')
    # An input of another call sent as null asks for nothing, so a binding
    # that fills one struct for every call keeps working. A switch is never
    # excused this way, and neither is an input that carries a value.
    assert "result" in lib.call("QueryParse", query="a=1", fields=None, record=None, ast=None)
    assert lib.call("QueryEval", query="1+1", fields=[], record={}, explicitAnd=None).get("error", "").startswith('unknown key "explicitAnd"')
    assert lib.call("QueryParse", query="a=1", strict=None).get("error", "").startswith('unknown key "strict"')
    assert lib.call("QueryParse", query="a=1", fields=[]).get("error", "").startswith('unknown key "fields"')
    # Every listed key is accepted.
    fields = [{"name": "a", "type": "integer"}]
    env = lib.call("QueryMatch", query="a=1", fields=fields, record={"a": 1}, maxLength=64, explicitAnd=True, strictValues=True)
    assert env.get("result") is True, env


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
    for name, check in (("hostile input", check_hostile_input), ("strict options", check_strict_options), ("unknown keys", check_unknown_keys),
                        ("completion input", check_completion_input), ("threads", check_threads)):
        try:
            check(lib)
        except AssertionError as err:
            failed += 1
            print(f"FAIL {name}: {err}")

    print(f"{len(cases) + 5 - failed} passed, {failed} failed")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
