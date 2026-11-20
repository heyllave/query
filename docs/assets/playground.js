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
    '{"name": "status", "type": "text", "allowedOps": ["=", "!="], "values": ["ready", "pending", "failed"]}',
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
  var suggestBox = el("pg-suggest");
  var ready = false;
  // The completion on show: what the engine answered, the items listed and
  // which of them is picked.
  var shown = null, shownItems = [], picked = -1;

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
      if (item.hint) {
        var hint = document.createElement("div");
        hint.className = "pg-hint";
        hint.textContent = "Hint: " + item.hint;
        li.appendChild(hint);
      }
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

  function hideSuggestions() {
    shown = null;
    shownItems = [];
    picked = -1;
    suggestBox.hidden = true;
    suggestBox.textContent = "";
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
  }

  // What a host would fetch from its own data, taken here from the record:
  // the keys of a nested field, and the value the record holds for a field.
  function ownCandidates(c) {
    var record;
    try { record = JSON.parse(recordBox.value); } catch (err) { return []; }
    if (!record || typeof record !== "object" || !c.field || c.lists) return [];
    var dot = c.field.indexOf(".");
    var root = dot < 0 ? c.field : c.field.slice(0, dot);
    var held = record[root];
    var found = [];
    if (c.slot === "key" && held && typeof held === "object" && !Array.isArray(held)) {
      found = Object.keys(held);
    } else if (c.slot === "value" && c.items.length === 0) {
      if (dot >= 0) held = held && typeof held === "object" ? held[c.field.slice(dot + 1)] : undefined;
      if (typeof held === "string" || typeof held === "number") found = [String(held)];
    }
    var typed = c.prefix.toLowerCase();
    return found.filter(function (name) { return name.toLowerCase().indexOf(typed) === 0; });
  }

  function pick(index) {
    picked = index;
    Array.prototype.forEach.call(suggestBox.children, function (li, i) {
      li.setAttribute("aria-selected", i === index ? "true" : "false");
      if (i === index) {
        input.setAttribute("aria-activedescendant", li.id);
        if (li.scrollIntoView) li.scrollIntoView({ block: "nearest" });
      }
    });
  }

  function accept(index) {
    var item = shownItems[index];
    if (!shown || !item) return;
    var from = shown.replace.offset, to = from + shown.replace.length;
    input.value = input.value.slice(0, from) + item.text + input.value.slice(to);
    var cursor = from + item.text.length;
    input.setSelectionRange(cursor, cursor);
    run();
    suggest();
  }

  function suggest() {
    if (!ready || document.activeElement !== input || input.selectionStart !== input.selectionEnd) {
      hideSuggestions();
      return;
    }
    var fields;
    try {
      fields = parseJSON(fieldsBox, "Fields");
    } catch (err) {
      hideSuggestions();
      return;
    }
    // The cursor indexes the string, as selectionStart gives it.
    var answer = window.queryComplete(input.value, fields, { cursor: input.selectionStart });
    if (answer.error || answer.result.slot === "none") {
      hideSuggestions();
      return;
    }
    var c = answer.result, items = c.items;
    var own = ownCandidates(c);
    if (own.length > 0) {
      var offered = window.queryOffer(JSON.stringify(c), JSON.stringify(own));
      if (!offered.error) items = items.concat(offered.result);
    }
    // Nothing to add when the only suggestion is what is already written.
    var written = input.value.slice(c.replace.offset, c.replace.offset + c.replace.length);
    if (items.length === 0 || (items.length === 1 && items[0].text === written)) {
      hideSuggestions();
      return;
    }

    shown = c;
    shownItems = items;
    suggestBox.textContent = "";
    items.forEach(function (item, i) {
      var li = document.createElement("li");
      li.id = "pg-suggest-" + i;
      li.setAttribute("role", "option");
      var label = document.createElement("span");
      label.textContent = item.label;
      li.appendChild(label);
      var kind = document.createElement("span");
      kind.className = "pg-kind";
      kind.textContent = item.detail || item.kind;
      li.appendChild(kind);
      // mousedown, not click: the input must keep the focus.
      li.addEventListener("mousedown", function (event) {
        event.preventDefault();
        accept(i);
      });
      suggestBox.appendChild(li);
    });
    suggestBox.hidden = false;
    input.setAttribute("aria-expanded", "true");
    pick(0);
  }

  input.addEventListener("keydown", function (event) {
    if (!shown) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      var step = event.key === "ArrowDown" ? 1 : shownItems.length - 1;
      pick((picked + step) % shownItems.length);
    } else if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      accept(picked);
    } else if (event.key === "Escape") {
      hideSuggestions();
    }
  });
  // The cursor moves without the text changing: arrows, Home, End, a click.
  input.addEventListener("keyup", function (event) {
    if (["ArrowLeft", "ArrowRight", "Home", "End"].indexOf(event.key) >= 0) suggest();
  });
  input.addEventListener("click", suggest);
  input.addEventListener("focus", suggest);
  input.addEventListener("blur", hideSuggestions);
  input.addEventListener("input", suggest);

  EXAMPLES.forEach(function (example) {
    var button = document.createElement("button");
    button.type = "button";
    button.textContent = example;
    button.addEventListener("click", function () {
      input.value = example;
      run();
      input.focus();
      suggest();
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
