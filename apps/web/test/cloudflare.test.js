// The Cloudflare adapters, exercised the way Pages calls them: onRequest({ request, env }).
// No Cloudflare runtime and no network — globalThis.fetch is swapped for a fake, which is
// also what the adapters hand to the shared library.
import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { onRequest as waitlist } from "../functions/api/waitlist.js";
import { onRequest as health } from "../functions/api/health.js";
import { onRequest as harbor } from "../functions/api/harbor.js";
import { clientIp, hashIp } from "../lib/waitlist.js";

const env = { SUPABASE_URL: "https://db.test", SUPABASE_ANON_KEY: "anon", IP_HASH_SALT: "salt" };

const realFetch = globalThis.fetch;
let calls = [];

function stubFetch({ result = "ok", status = 200 } = {}) {
  calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return new Response(JSON.stringify(result), { status, headers: { "content-type": "application/json" } });
  };
}

function post(body, headers = {}) {
  return new Request("https://monacolabs.xyz/api/waitlist", {
    method: "POST",
    headers: { "content-type": "application/json", origin: "https://monacolabs.xyz", ...headers },
    body: JSON.stringify(body),
  });
}

beforeEach(() => stubFetch());
afterEach(() => { globalThis.fetch = realFetch; });

test("a POST reaches handleSignup with the body, method and headers intact", async () => {
  const res = await waitlist({ request: post({ email: " You@Email.com ", twitter: "@ada", cabal: "Cavemen", source: "x" }), env });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "application/json");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.deepEqual(await res.json(), { ok: true });

  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "https://db.test/rest/v1/rpc/join_waitlist");
  const args = JSON.parse(calls[0].init.body);
  assert.equal(args.p_email, "you@email.com");
  assert.equal(args.p_twitter, "ada");
  assert.equal(args.p_cabal, "cavemen");
  assert.equal(args.p_source, "x");
});

test("an unknown cabal is a 400 with the library's message, and the database is never called", async () => {
  const res = await waitlist({ request: post({ email: "you@email.com", cabal: "hodlers" }), env });
  assert.equal(res.status, 400);
  assert.match((await res.json()).error, /cabal/);
  assert.equal(calls.length, 0);
});

test("the request method is passed through, so a GET is still a 405", async () => {
  const res = await waitlist({ request: new Request("https://monacolabs.xyz/api/waitlist"), env });
  assert.equal(res.status, 405);
  assert.equal(calls.length, 0);
});

test("the Origin header is passed through, so a foreign origin is still a 403", async () => {
  const res = await waitlist({ request: post({ email: "you@email.com" }, { origin: "https://evil.example" }), env });
  assert.equal(res.status, 403);
  assert.equal(calls.length, 0);
});

test("cf-connecting-ip is what gets hashed, and it beats a spoofed x-forwarded-for", async () => {
  const res = await waitlist({
    request: post({ email: "you@email.com" }, { "cf-connecting-ip": "203.0.113.7", "x-forwarded-for": "6.6.6.6" }),
    env,
  });
  assert.equal(res.status, 200);
  const args = JSON.parse(calls[0].init.body);
  assert.equal(args.p_ip_hash, hashIp("203.0.113.7", "salt"));
  assert.match(args.p_ip_hash, /^[0-9a-f]{64}$/);
  assert.ok(!calls[0].init.body.includes("203.0.113.7"));
});

test("the user agent survives the Headers object", async () => {
  await waitlist({ request: post({ email: "you@email.com" }, { "user-agent": "Mozilla/5.0 (test)" }), env });
  assert.equal(JSON.parse(calls[0].init.body).p_user_agent, "Mozilla/5.0 (test)");
});

test("errors from the shared library keep their status and JSON", async () => {
  stubFetch({ result: "rate_limited" });
  const res = await waitlist({ request: post({ email: "you@email.com" }), env });
  assert.equal(res.status, 429);
  assert.match((await res.json()).error, /Too many signups/);
});

test("health returns its JSON with a no-store header", async () => {
  stubFetch({ result: true });
  const res = await health({ request: new Request("https://monacolabs.xyz/api/health"), env });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "application/json");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.deepEqual(await res.json(), { status: "ok", dependencies: { supabase: "ok", ipHashSalt: "ok" } });

  stubFetch({ result: false });
  const bad = await health({ request: new Request("https://monacolabs.xyz/api/health"), env: {} });
  assert.equal(bad.status, 503);
  assert.equal((await bad.json()).dependencies.ipHashSalt, "missing");
});

test("clientIp prefers cf-connecting-ip and leaves the other hosts' headers working", () => {
  assert.equal(clientIp({ "cf-connecting-ip": " 203.0.113.7 ", "x-forwarded-for": "6.6.6.6" }), "203.0.113.7");
  assert.equal(clientIp({ "cf-connecting-ip": "", "x-forwarded-for": "9.9.9.9, 1.1.1.1" }), "9.9.9.9");
  assert.equal(clientIp({ "x-forwarded-for": "9.9.9.9" }), "9.9.9.9");
  assert.equal(clientIp({ "x-real-ip": "8.8.8.8" }), "8.8.8.8");
  assert.equal(clientIp({}), "unknown");
});

// handleHarbor keeps its count in memory for a minute, and the adapter uses the library's own memo, so only the
// first of these reads the database. The library's tests cover the cache window and the database failures.
test("harbor returns the counts as JSON, with the library's cache and CORS headers", async () => {
  const counts = { cavemen: 12, pilates: 7, cadena: 5, degens: 9, undecided: 3, total: 36 };
  stubFetch({ result: counts });
  const res = await harbor({ request: new Request("https://monacolabs.xyz/api/harbor", { headers: { origin: "https://www.monacolabs.xyz" } }), env });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "application/json");
  assert.equal(res.headers.get("cache-control"), "public, max-age=60");
  assert.equal(res.headers.get("vary"), "Origin");
  assert.equal(res.headers.get("access-control-allow-origin"), "https://www.monacolabs.xyz");
  assert.deepEqual(await res.json(), counts);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "https://db.test/rest/v1/rpc/waitlist_harbor");
});

test("harbor passes the method and the Origin header through, so a POST is a 405 and a foreign origin a 403", async () => {
  const post405 = await harbor({ request: new Request("https://monacolabs.xyz/api/harbor", { method: "POST" }), env });
  assert.equal(post405.status, 405);
  const foreign = await harbor({ request: new Request("https://monacolabs.xyz/api/harbor", { headers: { origin: "https://evil.example" } }), env });
  assert.equal(foreign.status, 403);
  assert.equal(foreign.headers.get("access-control-allow-origin"), null);
  assert.equal(foreign.headers.get("cache-control"), "no-store");
});
