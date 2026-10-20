import { test } from "node:test";
import assert from "node:assert/strict";
import { handleHarbor } from "../lib/waitlist.js";

const env = { SUPABASE_URL: "https://db.test", SUPABASE_ANON_KEY: "anon", IP_HASH_SALT: "salt" };
const quiet = { info() {}, warn() {}, error() {} };
const counts = { cavemen: 12, pilates: 7, cadena: 5, degens: 9, undecided: 3, total: 36 };

// Fake PostgREST: records calls and answers every RPC with one fixed result.
function fakeFetch({ result = counts, status = 200, throws = false } = {}) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, init });
    if (throws) throw new TypeError("fetch failed");
    return new Response(JSON.stringify(result), { status, headers: { "content-type": "application/json" } });
  };
  fn.calls = calls;
  return fn;
}

// A clock the test moves by hand.
function clock(start = 1_000_000) {
  let t = start;
  const now = () => t;
  now.advance = (ms) => { t += ms; };
  return now;
}

// Each test gets its own memo, as a fresh instance would.
const run = (over = {}) => handleHarbor({ method: "GET", headers: {}, env, fetch: fakeFetch(), memo: {}, now: clock(), log: quiet, ...over });

test("answers with the five cabals and the total, read through the anon key", async () => {
  const fetch = fakeFetch();
  const r = await run({ fetch });
  assert.equal(r.status, 200);
  assert.deepEqual(r.body, counts);
  assert.equal(fetch.calls.length, 1);
  assert.equal(fetch.calls[0].url, "https://db.test/rest/v1/rpc/waitlist_harbor");
  assert.equal(fetch.calls[0].init.headers.apikey, "anon");
});

test("publishes only the six counts, whatever else the database sends", async () => {
  const r = await run({ fetch: fakeFetch({ result: { ...counts, email: "someone@email.com", extra: 1 } }) });
  assert.deepEqual(Object.keys(r.body), ["cavemen", "pilates", "cadena", "degens", "undecided", "total"]);
  assert.deepEqual(r.body, counts);
});

test("a malformed answer is a 502 and is never kept", async () => {
  const bad = [null, [], "oops", {}, { ...counts, total: "36" }, { ...counts, cavemen: -1 }, { ...counts, degens: 1.5 }, { ...counts, undecided: null }];
  for (const result of bad) {
    const memo = {};
    const r = await run({ memo, fetch: fakeFetch({ result }) });
    assert.equal(r.status, 502, JSON.stringify(result));
    assert.equal(r.headers["Cache-Control"], "no-store");
    assert.equal(memo.counts, undefined);
  }
});

test("the database is read once a minute, and a fresh read follows the minute", async () => {
  const fetch = fakeFetch();
  const now = clock();
  const memo = {};
  await run({ fetch, now, memo });
  now.advance(59_999);
  await run({ fetch, now, memo });
  assert.equal(fetch.calls.length, 1, "inside the minute the memo answers");

  now.advance(1);
  const later = fakeFetch({ result: { ...counts, cavemen: 13, total: 37 } });
  const r = await run({ fetch: later, now, memo });
  assert.equal(later.calls.length, 1, "at the minute it reads again");
  assert.equal(r.body.cavemen, 13);
});

test("the answer is public for a minute and varies by origin", async () => {
  const r = await run();
  assert.equal(r.headers["Cache-Control"], "public, max-age=60");
  assert.equal(r.headers.Vary, "Origin");
});

test("only GET is allowed", async () => {
  for (const method of ["POST", "PUT", "DELETE", "OPTIONS"]) {
    const fetch = fakeFetch();
    const r = await run({ method, fetch });
    assert.equal(r.status, 405, method);
    assert.equal(fetch.calls.length, 0);
  }
});

test("an allowed origin is echoed back, and no origin gets no CORS header", async () => {
  for (const origin of ["https://monacolabs.xyz", "https://www.monacolabs.xyz"]) {
    const r = await run({ headers: { origin } });
    assert.equal(r.status, 200, origin);
    assert.equal(r.headers["Access-Control-Allow-Origin"], origin);
  }
  const r = await run();
  assert.equal(r.status, 200);
  assert.equal(r.headers["Access-Control-Allow-Origin"], undefined);
});

test("a foreign origin gets a 403 and no database call, and the lapsed domain counts as foreign", async () => {
  for (const origin of ["https://evil.example", "https://trymonaco.xyz", "https://monacolabs.xyz.evil.example", "null"]) {
    const fetch = fakeFetch();
    const r = await run({ headers: { origin }, fetch });
    assert.equal(r.status, 403, origin);
    assert.equal(r.headers["Access-Control-Allow-Origin"], undefined);
    assert.equal(r.headers["Cache-Control"], "no-store");
    assert.equal(fetch.calls.length, 0);
  }
});

test("ALLOWED_ORIGINS replaces the defaults", async () => {
  const local = { ...env, ALLOWED_ORIGINS: "http://localhost:5173, https://preview.example" };
  const ok = await run({ env: local, headers: { origin: "https://preview.example" } });
  assert.equal(ok.status, 200);
  assert.equal(ok.headers["Access-Control-Allow-Origin"], "https://preview.example");
  assert.equal((await run({ env: local, headers: { origin: "https://monacolabs.xyz" } })).status, 403);
});

test("database errors and network failures are a 502, not a crash, and the next request tries again", async () => {
  for (const down of [fakeFetch({ status: 500, result: { message: "boom" } }), fakeFetch({ status: 404, result: {} }), fakeFetch({ throws: true })]) {
    const memo = {};
    const r = await run({ memo, fetch: down });
    assert.equal(r.status, 502);
    assert.equal(r.headers["Cache-Control"], "no-store");
    assert.equal((await run({ memo })).status, 200);
  }
});

test("missing configuration is a 500 and never calls the database", async () => {
  for (const key of ["SUPABASE_URL", "SUPABASE_ANON_KEY"]) {
    const fetch = fakeFetch();
    const r = await run({ fetch, env: { ...env, [key]: "" } });
    assert.equal(r.status, 500, key);
    assert.equal(fetch.calls.length, 0);
  }
});

test("a failed read is logged without the response, and a good one logs nothing", async () => {
  const lines = [];
  const log = { ...quiet, error: (l) => lines.push(l) };
  await run({ fetch: fakeFetch({ throws: true }), log });
  assert.equal(lines.length, 1);
  assert.ok(lines[0].includes("harbor_read_failed"));
  await run({ log });
  assert.equal(lines.length, 1);
});
