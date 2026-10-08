import { test, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ configFile: new URL("../vite.config.ts", import.meta.url).pathname, logLevel: "silent", server: { port: 0, host: "127.0.0.1" } });
await server.listen();
after(() => server.close());
const base = `http://127.0.0.1:${server.httpServer.address().port}`;

const title = async (path) => (await (await fetch(base + path)).text()).match(/<title>([^<]*)/)?.[1];

test("/fund and /fund?query serve the deposit page in dev", async () => {
  assert.equal(await title("/fund"), "Deposit · Monaco");
  assert.equal(await title("/fund?session=abc"), "Deposit · Monaco");
  assert.equal(await title("/fund/"), "Deposit · Monaco");
});

// Runs the plugin's middleware on its own, since the served page never shows the query string.
function rewrite(url) {
  const plugin = server.config.plugins.find((p) => p.name === "fund-dev-rewrite");
  let handler;
  plugin.configureServer({ middlewares: { use: (fn) => (handler = fn) } });
  const req = { url };
  handler(req, {}, () => {});
  return req.url;
}

test("the rewrite keeps the query string verbatim", () => {
  assert.equal(rewrite("/fund?session=abc&x=a%20b"), "/fund/?session=abc&x=a%20b");
  assert.equal(rewrite("/fund"), "/fund/");
});

test("the rewrite leaves other paths alone", () => {
  for (const url of ["/funding", "/fund/x", "/fund/", "/fund/?a=1", "/", "/fundx?a=1"]) assert.equal(rewrite(url), url);
});

test("other paths are left alone", async () => {
  assert.notEqual(await title("/funding"), "Deposit · Monaco");
  assert.notEqual(await title("/"), "Deposit · Monaco");
});
