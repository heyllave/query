// The playground: runs the query engine (query.wasm) in the page and shows
// what a query parses to and whether it matches the record.
(function () {
  "use strict";

  var WASM_URL = document.currentScript.getAttribute("data-wasm");
  var EXAMPLES = [
    "labels.team=payments AND replicas>=2",
    "name=check* AND status!=failed",
    "status IN (ready, pending)",
    "created_at>=now()-30d",
    "lower(name)=CHECKOUT*",
    "labels.example.com/owner=jane",
    "NOT (replicas<2 OR status=failed)",
    "status=rea*",
    "replicas==3",
    "unknown=1"
  ];

  var FIELDS = [
    '{"name": "name", "type": "text"}',
    '{"name": "status", "type": "text", "allowedOps": ["=", "!="]}',
    '{"name": "replicas", "type": "integer"}',
    '{"name": "created_at", "type": "datetime"}',
    '{"name": "labels", "type": "text", "nested": true}'
  ];
  var RECORD = {
    name: "checkout-preview",
    status: "ready",
    replicas: 3,
    created_at: "2026-03-14T09:30:00Z",
    labels: { team: "payments", "example.com/owner": "jane" }
  };

  var el = function (id) { return document.getElementById(id); };
  var input = el("pg-query"), fieldsBox = el("pg-fields"), recordBox = el("pg-record");
  var verdict = el("pg-verdict"), mark = el("pg-mark"), errorList = el("pg-errors");
  var printed = el("pg-printed"), tree = el("pg-tree");
  var ready = false;

  // Set here rather than in the page: the site's HTML is minified, which
  // would fold the line breaks out of a textarea's initial text.
  fieldsBox.value = "[\n  " + FIELDS.join(",\n  ") + "\n]";
  recordBox.value = JSON.stringify(RECORD, null, 2);

  function setVerdict(state, text) {
    verdict.dataset.state = state;
    verdict.textContent = text;
  }

  function showErrors(query, items) {
    errorList.textContent = "";
    mark.textContent = "";
    if (!items || items.length === 0) {
      errorList.hidden = true;
      mark.hidden = true;
      return;
    }
    // Offsets are byte offsets into the UTF-8 query, so slice bytes, not characters.
    var bytes = new TextEncoder().encode(query);
    var decode = function (from, to) { return new TextDecoder().decode(bytes.slice(from, to)); };
    var first = items[0];
    var start = Math.min(first.offset, bytes.length);
    var end = Math.min(start + Math.max(first.length, 1), bytes.length);
    mark.appendChild(document.createTextNode(decode(0, start)));
    var hit = document.createElement("mark");
    hit.textContent = start === end ? " " : decode(start, end);
    mark.appendChild(hit);
    mark.appendChild(document.createTextNode(decode(end, bytes.length)));
    mark.hidden = false;

    items.forEach(function (item) {
      var li = document.createElement("li");
      var code = document.createElement("code");
      code.textContent = item.code;
      li.appendChild(code);
      li.appendChild(document.createTextNode(" at offset " + item.offset + ": " + item.message));
      errorList.appendChild(li);
    });
    errorList.hidden = false;
  }

  function parseJSON(box, what) {
    try {
      JSON.parse(box.value);
      return box.value;
    } catch (err) {
      throw new Error(what + " is not valid JSON: " + err.message);
    }
  }

  function run() {
    if (!ready) return;
    var query = input.value;
    printed.textContent = "";
    tree.textContent = "";

    var fields, record;
    try {
      fields = parseJSON(fieldsBox, "Fields");
      record = parseJSON(recordBox, "Record");
    } catch (err) {
      showErrors(query, null);
      setVerdict("error", err.message);
      return;
    }

    if (query.trim() === "") {
      showErrors(query, null);
      setVerdict("no", "Type a query.");
      return;
    }

    var parsed = window.queryParseAndValidate(query, fields);
    if (parsed.error) {
      showErrors(query, parsed.errors || parsed.validationErrors);
      setVerdict("error", parsed.errors ? "Syntax error" :
        parsed.validationErrors ? "Not allowed by the field list" : parsed.error);
      return;
    }
    showErrors(query, null);

    tree.textContent = JSON.stringify(parsed.result, null, 2);
    var text = window.queryStringify(JSON.stringify(parsed.result));
    printed.textContent = text.error ? text.error : text.result;

    var matched = window.queryMatch(query, fields, record);
    if (matched.error) {
      setVerdict("error", matched.error);
    } else if (matched.result) {
      setVerdict("yes", "The record matches.");
    } else {
      setVerdict("no", "The record does not match.");
    }
  }

  EXAMPLES.forEach(function (example) {
    var button = document.createElement("button");
    button.type = "button";
    button.textContent = example;
    button.addEventListener("click", function () {
      input.value = example;
      run();
      input.focus();
    });
    el("pg-examples").appendChild(button);
  });

  [input, fieldsBox, recordBox].forEach(function (box) {
    box.addEventListener("input", run);
  });

  function instantiate(imports) {
    return fetch(WASM_URL).then(function (response) {
      if (!response.ok) throw new Error("query.wasm: HTTP " + response.status);
      return response.arrayBuffer();
    }).then(function (bytes) {
      return WebAssembly.instantiate(bytes, imports);
    });
  }

  if (typeof Go === "undefined" || typeof WebAssembly === "undefined") {
    setVerdict("error", "This browser cannot run WebAssembly.");
    return;
  }
  var go = new Go();
  instantiate(go.importObject).then(function (result) {
    go.run(result.instance); // never resolves: the engine stays alive for the page
    ready = true;
    run();
  }).catch(function (err) {
    setVerdict("error", "Could not load the engine: " + err.message);
  });
})();
