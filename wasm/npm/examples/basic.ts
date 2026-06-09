/**
 * Basic TypeScript example: parse, validate, and stringify queries.
 *
 * Run:
 *   cd wasm/npm
 *   npm run build:wasm
 *   npx tsx examples/basic.ts
 */

import { loadQuery, visit, walk, fields, fieldToString } from "../src/index.js";
import type { Visitor, Expression, QualifierExpr, PresenceExpr, BinaryExpr, UnaryExpr, GroupExpr } from "../src/index.js";

async function main() {
  const q = await loadQuery("../query.wasm");

  // -----------------------------------------------------------------------
  // 1. Parse a query
  // -----------------------------------------------------------------------
  console.log("=== Parse ===");
  const { result: ast, error } = q.parse("state=draft AND total>50000");
  if (error) {
    console.error("Parse error:", error);
    return;
  }
  console.log("AST:", JSON.stringify(ast, null, 2));

  // -----------------------------------------------------------------------
  // 2. Validate against field config
  // -----------------------------------------------------------------------
  console.log("\n=== Validate ===");
  const fieldConfigs = [
    { Name: "state", Type: 0, AllowedOps: ["=", "!=", "*", "?"] },
    { Name: "total", Type: 2, AllowedOps: ["=", "!=", ">", ">=", "<", "<=", ".."] },
  ];
  const validation = q.validate(ast!, fieldConfigs);
  console.log("Valid:", validation.valid);

  // -----------------------------------------------------------------------
  // 3. Stringify back to query
  // -----------------------------------------------------------------------
  console.log("\n=== Stringify ===");
  const { result: queryStr } = q.stringify(ast!);
  console.log("Query:", queryStr);

  // -----------------------------------------------------------------------
  // 4. Extract fields
  // -----------------------------------------------------------------------
  console.log("\n=== Fields ===");
  const fieldPaths = fields(ast!);
  for (const fp of fieldPaths) {
    console.log(" ", fieldToString(fp));
  }

  // -----------------------------------------------------------------------
  // 5. Walk the AST
  // -----------------------------------------------------------------------
  console.log("\n=== Walk ===");
  walk(ast!, (node) => {
    console.log("  node:", node.type);
    return true;
  });

  // -----------------------------------------------------------------------
  // 6. SQL generation via Visitor
  // -----------------------------------------------------------------------
  console.log("\n=== SQL Visitor ===");
  const params: unknown[] = [];

  const sqlVisitor: Visitor<string> = {
    visitBinary(e: BinaryExpr): string {
      const left = visit(sqlVisitor, e.left);
      const right = visit(sqlVisitor, e.right);
      return `${left} ${e.op} ${right}`;
    },
    visitUnary(e: UnaryExpr): string {
      return `NOT (${visit(sqlVisitor, e.expr)})`;
    },
    visitQualifier(e: QualifierExpr): string {
      const field = e.field.join(".");
      const op = e.value.wildcard ? "LIKE" : e.op;
      const val = e.value.wildcard
        ? String(e.value.raw).replace(/\*/g, "%")
        : e.value.value;
      params.push(val);
      return `${field} ${op} $${params.length}`;
    },
    visitPresence(e: PresenceExpr): string {
      return `${e.field.join(".")} IS NOT NULL`;
    },
    visitGroup(e: GroupExpr): string {
      return `(${visit(sqlVisitor, e.expr)})`;
    },
  };

  const where = visit(sqlVisitor, ast!);
  console.log("  WHERE", where);
  console.log("  params:", params);

  // -----------------------------------------------------------------------
  // 6b. Evaluate a predicate / value against a record
  // -----------------------------------------------------------------------
  console.log("\n=== Match / Eval ===");
  const record = { state: "draft", total: 60000, base: 50000 };
  const matched = q.match("state=draft AND total>50000", fieldConfigs, record);
  console.log("  match state=draft AND total>50000 →", matched.result);

  const crossField = q.match("total>[base]", fieldConfigs, record);
  console.log("  match total>[base] (cross-field)  →", crossField.result);

  const computed = q.eval("[base]*1.1", fieldConfigs, record);
  console.log("  eval  [base]*1.1                  →", computed.result);

  // -----------------------------------------------------------------------
  // 7. Parse error handling
  // -----------------------------------------------------------------------
  console.log("\n=== Error handling ===");
  const bad = q.parse("=invalid");
  console.log("  Error:", bad.error);

  // -----------------------------------------------------------------------
  // 8. Complex queries
  // -----------------------------------------------------------------------
  console.log("\n=== Complex queries ===");
  const queries = [
    "state=draft",
    "name=John*",
    "(state=draft OR state=issued) AND total>50000",
    "NOT state=cancelled",
    "created_at:2026-01-01..2026-03-31",
    "labels.dev=jane",
    "tire_size",
  ];
  for (const query of queries) {
    const { result: r, error: e } = q.parse(query);
    const status = e ? `ERROR: ${e}` : "OK";
    const roundTrip = r ? q.stringify(r).result : "";
    console.log(`  ${query.padEnd(50)} ${status.padEnd(6)} → ${roundTrip}`);
  }
}

main().catch(console.error);
