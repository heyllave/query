---
title: query and jq
nav_order: 2
parent: Guides
permalink: /jq/
---

# query and jq
{: .no_toc }

`jq` is a programming language for JSON. `query` is a filter language for records. On a simple filter they are the same length. They part ways when the data is untidy, when the filter reaches into lists, and when the filter comes from someone else.

Every result on this page was run with `jq` 1.8.1 against [five sample records]({{ '/assets/jq-events.jsonl' | relative_url }}).

1. TOC
{:toc}

## The records

```json
{"id":1,"name":"Joanna Smith","state":"Draft","total":60000,"created_at":"2026-03-10T09:00:00Z","labels":{"team":"payments"},"cpu":0.9,"cpu_limit":0.5,"orders":[{"status":"shipped","items":[{"sku":"A1","qty":3}]},{"status":"pending","items":[]}]}
{"id":2,"name":"josé Álvarez","state":"issued","total":"75000","created_at":"2026-03-12T08:00:00+05:00","labels":{"team":"Payments"},"cpu":0.2,"cpu_limit":0.5,"orders":[{"status":"cancelled","items":[{"sku":"A1","qty":1}]}]}
{"id":3,"name":null,"state":"draft","total":100,"created_at":"2026-02-01T12:00:00Z","labels":{},"cpu":0.4,"orders":[]}
{"id":4,"state":"paid","total":52000.5,"created_at":"2026-03-14T23:30:00-05:00","labels":{"team":"growth"},"cpu":0.7,"cpu_limit":0.7,"closed_at":"2026-03-20T10:00:00Z","orders":[{"status":"shipped","items":[{"sku":"B2","qty":9}]}]}
{"id":5,"name":"JOHN doe","state":"draft","total":null,"created_at":"2026-03-15T01:00:00Z","cpu":0.1,"cpu_limit":0.5,"orders":[{"status":"shipped","items":[{"sku":"A1","qty":5}]},{"status":"shipped","items":[{"sku":"C3","qty":1}]}]}
```

They are untidy the way real exports are: a name that is `null`, a name that is missing, a total stored as text, mixed capitals, and timestamps in three time zones.

The commands below are `jq -c '<filter> | .id' events.jsonl` and `query filter '<filter>' < events.jsonl`, and the tables give the ids that come back.

## Untidy data

`jq` compares any two values, whatever their types, and stops with an error when a function meets a type it does not take. `query` types each field once and leaves out a record whose value does not fit.

| Question | jq | Returns | query | Returns |
|---|---|---|---|---|
| Total over 50,000 | `select(.total > 50000)` | 1, 2, 4 | `total>50000` | 1, 4 |
| Over its CPU limit | `select(.cpu > .cpu_limit)` | 1, 3 | `cpu>[cpu_limit]` | 1 |
| Within 10% of its limit | `select(.cpu >= .cpu_limit * 0.9)` | error at record 3 | `cpu>=[cpu_limit]*0.9` | 1, 4 |
| Name starts with "jo" | `select(.name \| ascii_downcase \| startswith("jo"))` | error at records 3 and 4 | `name=jo*` | 1, 2, 5 |

- **Row 1.** Record 2 holds the text `"75000"`. In `jq` any text sorts above any number, so it passes, and so would `"abc"`.
- **Row 2.** Record 3 has no limit. In `jq` a missing value is `null`, and every number is greater than `null`.
- **Rows 3 and 4.** `jq` prints the records it reached and exits with an error. The fix is to guard each field: `select((.name // "") | ascii_downcase | startswith("jo"))`.

In `query` a missing or unreadable value makes a comparison false. [Missing and unreadable values]({{ '/syntax/' | relative_url }}#missing-and-unreadable-values) has the rules, including the two exceptions.

## Capitals

Text in `query` is compared without regard to case. In `jq` you lower both sides yourself, and `ascii_downcase` only knows ASCII.

| Question | jq | Returns | query | Returns |
|---|---|---|---|---|
| Team is payments | `select(.labels.team == "payments")` | 1 | `labels.team=payments` | 1, 2 |
| Draft or issued | `select(.state \| IN("draft", "issued"))` | 2, 3, 5 | `state IN (draft, issued)` | 1, 2, 3, 5 |
| Name starts with "JOSÉ" | `select((.name // "") \| ascii_downcase \| startswith("JOSÉ" \| ascii_downcase))` | none | `name=JOSÉ*` | 2 |

In the last row `ascii_downcase` turns `JOSÉ` into `josÉ`, which does not match `josé`.

If case matters to you, this is a point for `jq`: `query` has no case-sensitive comparison.

## Dates and time zones

`jq` compares timestamps as text. That is correct only while every timestamp is written in the same zone. `query` reads each one as a moment in time, and a date without a time means the whole UTC day.

| Question | jq | Returns | query | Returns |
|---|---|---|---|---|
| Created on 15 March | `select(.created_at \| startswith("2026-03-15"))` | 5 | `created_at=2026-03-15` | 4, 5 |
| Created 12 to 14 March | `select(.created_at >= "2026-03-12" and .created_at < "2026-03-15")` | 2, 4 | `created_at:2026-03-12..2026-03-14` | 2 |
| Closed more than 5 days after it was created | `select(.closed_at != null and (.closed_at \| fromdate) > (.created_at \| fromdate) + 5*86400)` | error at record 4 | `closed_at>addDays(created_at, 5)` | 4 |
| Created on a weekend | `select(.created_at \| fromdate \| strftime("%u") \| tonumber >= 6)` | error at records 2 and 4 | `NOT isBusinessDay(created_at)` | 3, 4, 5 |
| Created in the last 7 days | `select(.created_at >= (now - 7*86400 \| todate))` | text comparison, as above | `created_at>=now()-7d` | by moment in time |

Record 4 was created at `2026-03-14T23:30:00-05:00`, which is 04:30 on 15 March in UTC. Read as text it belongs to the 14th; read as a time it belongs to the 15th.

`fromdate` accepts only the form that ends in `Z`. `strptime` with `%z` parses an offset, but `mktime` then ignores it, so a correct `jq` version has to add the offset by hand.

## Lists inside lists

A selector applies a condition to the elements of a list, and selectors nest.

| Question | jq | query |
|---|---|---|
| A shipped order that has at least 3 of item A1 | `select(any(.orders[]?; .status == "shipped" and any(.items[]?; .sku == "A1" and .qty > 2)))` | `orders@(status=shipped AND items@(sku=A1 AND qty>2))` |
| Has orders, and every one is shipped | `select((.orders \| length) > 0 and all(.orders[]; .status == "shipped"))` | `orders@first AND orders@all(status=shipped)` |
| No cancelled order | `select(all(.orders[]?; .status != "cancelled"))` | `orders@none(status=cancelled)` |

Both tools return the same records here (1 and 5; 4 and 5; 1, 3, 4 and 5). The difference is what you have to remember: the `?` after `[]`, so a record without the list is not an error, and that `all` of an empty list is true in both.

## Mistakes

A mistake in a `jq` filter is often still a valid program.

```console
$ jq -c 'select(.state = "draft") | .id' events.jsonl
1
2
3
4
5
```

A single `=` is assignment in `jq`. The filter sets every state to `"draft"` and then selects everything. In `query` a single `=` is the comparison, and the doubled one is the error:

```console
$ query filter 'state==draft' < events.jsonl
error [unexpectedChar]: unexpected "=" where a value belongs
  state==draft
        ^
  hint: equality is a single =: state=draft
```

A misspelt field is `null` in `jq`, so the filter returns nothing and exits with success. Given a field list, `query` refuses it:

```console
$ jq -c 'select(.sate == "draft") | .id' events.jsonl
$ echo $?
0

$ query filter --schema fields.json 'sate=draft' < events.jsonl
error: validate: position 0: unknown field "sate" (did you mean "state"?)
```

The same field list stops a comparison that cannot work (`total>high`) and an operator you did not allow on a field (`state=dr*` when `state` allows only `=` and `!=`). [Validate against a field list]({{ '/cli/' | relative_url }}#validate-against-a-field-list) shows the file.

## A filter someone else wrote

This is the difference that decides whether a filter can go in a URL, a saved view or a config file.

A `jq` filter is a program. It can read the environment (`$ENV`, `env`), read other files (`input`, `--slurpfile`), and run without end:

```console
$ timeout 2 jq -n '[range(1e9)] | length'; echo $?
124
```

A `query` filter can only compare the fields of the record in front of it. It has no loops, no assignment and no access to anything outside the record. Its length and its nesting are bounded, and a query that is too long is refused before it is parsed:

```console
$ query filter "$(printf 'a=1 AND %.0s' {1..200})b=2" < events.jsonl
error: parse: position 0: query length 1603 exceeds maximum of 256 characters
```

The same query, with the same meaning, also runs in Go, in a browser and through the C library, and can be translated to SQL. [Stability]({{ '/stability/' | relative_url }}) lists the limits.

## Values and reshaping

`query eval` computes one value for each record, and `query map` builds one object for each record.

```console
$ query map 'id, who=upper(coalesce(name, "unknown")), orders=count(orders)' < events.jsonl
{"id":1,"who":"JOANNA SMITH","orders":2}
{"id":2,"who":"JOSÉ ÁLVAREZ","orders":1}
{"id":3,"who":"UNKNOWN","orders":0}
{"id":4,"who":"UNKNOWN","orders":1}
{"id":5,"who":"JOHN DOE","orders":2}
```

The `jq` form is `{id, who: ((.name // "unknown") | ascii_upcase), orders: (.orders | length)}`, and it writes `JOSé ÁLVAREZ` for record 2.

## Where jq is the tool to use

`query` works on one record at a time and has no way to build structure beyond `map`. Use `jq` for everything else:

| Task | jq |
|---|---|
| Group and count | `group_by(.state) \| map({state: .[0].state, n: length})` |
| Sum across records | `[.[].total] \| add` |
| Sort, or take the first ten | `sort_by(.total) \| .[:10]` |
| One output for each list element | `.orders[]` |
| Change a document | `.replicas = 3` |
| Case-sensitive text | `select(.state == "Draft")` |
| A regular expression | `select(.name \| test("^Jo(hn\|anna)"))` |

They compose, so the usual pipe is `query` for the filter and `jq` for the shape:

```bash
query filter 'state=draft AND orders@(status=shipped)' < events.jsonl | jq -s 'group_by(.labels.team) | map(length)'
```
