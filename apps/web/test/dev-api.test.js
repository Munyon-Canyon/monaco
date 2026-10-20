// `npx vite` takes a real sign-up: the dev server mounts api/*.js, which talk to a fake Supabase here.
// Same shape as fund-dev.test.js: a real Vite dev server on a free port, no browser.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { createServer as createHttpServer } from "node:http";
import { createServer } from "vite";

// A fake PostgREST: records every RPC and answers the way the real functions do.
const rpcs = [];
const supabase = createHttpServer((req, res) => {
  let raw = "";
  req.on("data", (chunk) => (raw += chunk));
  req.on("end", () => {
    const name = req.url.replace("/rest/v1/rpc/", "");
    rpcs.push({ name, apikey: req.headers.apikey, args: raw ? JSON.parse(raw) : null });
    res.setHeader("content-type", "application/json");
    res.end(name === "waitlist_ready" ? "true" : '"ok"');
  });
});
await new Promise((resolve) => supabase.listen(0, "127.0.0.1", resolve));
Object.assign(process.env, {
  SUPABASE_URL: `http://127.0.0.1:${supabase.address().port}`,
  SUPABASE_ANON_KEY: "anon",
  IP_HASH_SALT: "salt",
});

const vite = await createServer({ configFile: new URL("../vite.config.ts", import.meta.url).pathname, logLevel: "silent", server: { port: 0, host: "127.0.0.1" } });
await vite.listen();
const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
process.env.ALLOWED_ORIGINS = base;
after(async () => { await vite.close(); supabase.close(); });

const signup = (body, headers = {}) =>
  fetch(`${base}/api/waitlist`, { method: "POST", headers: { "content-type": "application/json", origin: base, ...headers }, body: JSON.stringify(body) });

test("a sign-up posted to the dev server reaches the database with its email, handle, cabal and source", async () => {
  rpcs.length = 0;
  const res = await signup({ email: " You@Email.com ", twitter: "@ada", cabal: "Cavemen", source: "test" });
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { ok: true });
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(rpcs.length, 1);
  assert.equal(rpcs[0].name, "join_waitlist");
  assert.equal(rpcs[0].apikey, "anon");
  assert.equal(rpcs[0].args.p_email, "you@email.com");
  assert.equal(rpcs[0].args.p_twitter, "ada");
  assert.equal(rpcs[0].args.p_cabal, "cavemen");
  assert.equal(rpcs[0].args.p_source, "test");
});

test("the handlers' own checks run in dev: method, origin and body", async () => {
  rpcs.length = 0;
  assert.equal((await fetch(`${base}/api/waitlist`)).status, 405);
  assert.equal((await signup({ email: "you@email.com" }, { origin: "https://evil.example" })).status, 403);
  assert.equal((await signup({ email: "nope" })).status, 400);
  assert.equal((await signup({ email: "you@email.com", cabal: "hodlers" })).status, 400);
  assert.equal((await fetch(`${base}/api/waitlist`, { method: "POST", body: "{not json", headers: { origin: base } })).status, 400);
  assert.equal(rpcs.length, 0);
});

test("health asks the database and reports it", async () => {
  rpcs.length = 0;
  const res = await fetch(`${base}/api/health`);
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { status: "ok", dependencies: { supabase: "ok", ipHashSalt: "ok" } });
  assert.deepEqual(rpcs.map((r) => r.name), ["waitlist_ready"]);
});

test("everything outside /api is still the page", async () => {
  const html = await (await fetch(`${base}/`)).text();
  assert.match(html, /<title>Monaco/);
});
