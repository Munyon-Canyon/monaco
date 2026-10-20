// The cabal list lives in three places: CABALS in lib/waitlist.js, the database, and the page's options.
// Nothing generates one from another, so these tests fail when they drift.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { CABALS } from "../lib/waitlist.js";

const migration = readFileSync(new URL("../migrations/000030_waitlist_cabal.sql", import.meta.url), "utf8");

test("the migration's constraint and function accept exactly the cabals the API does", () => {
  const lists = [...migration.matchAll(/\(('cavemen'[^)]*)\)/g)].map((m) => [...m[1].matchAll(/'([a-z]+)'/g)].map((x) => x[1]));
  assert.equal(lists.length, 2, "the CHECK constraint and join_waitlist() each list the cabals");
  for (const list of lists) assert.deepEqual(list, CABALS);
});
