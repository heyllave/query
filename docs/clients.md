---
title: Clients
nav_order: 3
permalink: /clients/
---

# Clients

One engine, written in Go, reached four ways. All of them run the same test corpus, so they give the same answer for the same query.

| You have | Use | It gives you |
|---|---|---|
| A Go program | [Go]({{ '/go/' | relative_url }}) | The whole API: parse, validate, compile, your own functions, a visitor over the tree. |
| A browser or Node | [TypeScript]({{ '/typescript/' | relative_url }}) | The engine as WebAssembly, with typed results. |
| Any other language | [C library]({{ '/c-library/' | relative_url }}) | Six functions that take and return JSON. |
| A shell | [Command line]({{ '/cli/' | relative_url }}) | `query explain` to see how a query is read, and `filter`, `eval` and `map` to run one over JSON records. |

A query is parsed leniently unless you ask otherwise. Every client takes the same two [strict options]({{ '/grammar/' | relative_url }}#strict-options), and every client refuses an option it does not know.
