// The cabal list lives in three places: CABALS in lib/waitlist.js, the database, and the page's options.
// Nothing generates one from another, so these tests fail when they drift.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { CABALS } from "../lib/waitlist.js";

const migration = readFileSync(new URL("../migrations/000030_waitlist_cabal.sql", import.meta.url), "utf8");
const harbor = readFileSync(new URL("../migrations/000031_waitlist_harbor.sql", import.meta.url), "utf8");
const html = readFileSync(new URL("../index.html", import.meta.url), "utf8");

test("the migration's constraint and function accept exactly the cabals the API does", () => {
  const lists = [...migration.matchAll(/\(('cavemen'[^)]*)\)/g)].map((m) => [...m[1].matchAll(/'([a-z]+)'/g)].map((x) => x[1]));
  assert.equal(lists.length, 2, "the CHECK constraint and join_waitlist() each list the cabals");
  for (const list of lists) assert.deepEqual(list, CABALS);
});

test("the page offers exactly the cabals the API accepts, in the same order", () => {
  const values = [...html.matchAll(/<input type="radio" name="cabal" value="([a-z]+)">/g)].map((m) => m[1]);
  assert.deepEqual(values, CABALS);
});

test("each row reads as the copy the founders gave, and the question and its error use their words", () => {
  const rows = [...html.matchAll(/<label><input type="radio" name="cabal"[^>]*><span>(.*?)<\/span><\/label>/g)].map((m) => m[1].replace(/<[^>]+>/g, ""));
  assert.deepEqual(rows, [
    "Cavemen, never sell",
    "Pilates, buy what you use",
    "La Cadena, deposit on the 1st",
    "Degens in recovery, one stock a month",
    "Undecided",
  ]);
  assert.match(html, />Which cabal are you\?</);
  assert.match(html, /"Pick a cabal, or Undecided\."/);
});

test("the sign-up request carries the chosen cabal next to the source", () => {
  assert.match(html, /JSON\.stringify\(\{[^}]*cabal: form\.cabal\.value[^}]*source: q\.get\("ref"\)/);
});

test("the counter's SQL reports every cabal and the total, in the order the API publishes them", () => {
  const keys = [...harbor.matchAll(/^\s+'([a-z]+)',/gm)].map((m) => m[1]);
  assert.deepEqual(keys, [...CABALS, "total"]);
});
