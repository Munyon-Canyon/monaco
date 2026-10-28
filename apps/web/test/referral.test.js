import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { renderReferralPage, handleReferral, lookupReferrer, lookupCabal, normalizeCode, normalizeId, otherHost } from "../lib/referral.js";
import { onRequest as cloudflare } from "../functions/r/[code].js";
import vercel from "../api/r/[code].js";
import { readFileSync } from "node:fs";

const STORE = "https://testflight.apple.com/join/example";
const kai = { user_id: "01890a5d-ac96-774b-bcce-b302099a8058", display_name: "Kai", photo_url: "https://img.example/kai.png", handle: "kaicenat" };
const env = { MONACO_API_URL: "http://api.test", APP_STORE_URL: STORE };

function fakeFetch({ referrer = kai, status = 200, throws = false, hang = false } = {}) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, init });
    if (throws) throw new TypeError("fetch failed");
    if (hang) {
      return new Promise((_, reject) => init.signal.addEventListener("abort", () => reject(init.signal.reason)));
    }
    return new Response(JSON.stringify({ referrer }), { status, headers: { "content-type": "application/json" } });
  };
  fn.calls = calls;
  return fn;
}

const render = (over = {}) => renderReferralPage({ code: "k7m4qx2p", referrer: kai, appStoreUrl: STORE, openHost: "www.monacolabs.xyz", ...over });

test("a found referrer gets its name, photo and canonical url in the preview tags", () => {
  const { status, headers, body } = render();
  assert.equal(status, 200);
  assert.equal(headers["Cache-Control"], "public, max-age=300");
  assert.match(headers["Content-Type"], /^text\/html/);
  assert.match(body, /<title>Kai invited you to Monaco<\/title>/);
  assert.match(body, /<meta property="og:title" content="Kai invited you to Monaco">/);
  assert.match(body, /<meta property="og:description" content="Invest together with friends\.">/);
  assert.match(body, /<meta property="og:image" content="https:\/\/img\.example\/kai\.png">/);
  assert.match(body, /<meta property="og:url" content="https:\/\/monacolabs\.xyz\/r\/k7m4qx2p">/);
  assert.match(body, /<meta name="twitter:card" content="summary">/);
  assert.match(body, /id="get-monaco" href="https:\/\/testflight\.apple\.com\/join\/example"/);
  assert.match(body, /href="https:\/\/www\.monacolabs\.xyz\/r\/k7m4qx2p">Open in Monaco/);
});

test("a referrer without a photo gets the default image, an absolute url", () => {
  const { body } = render({ referrer: { ...kai, photo_url: null } });
  assert.match(body, /<meta property="og:image" content="https:\/\/monacolabs\.xyz\/assets\/og-default\.png">/);
});

test("a photo that is not an http url is not used", () => {
  const { body } = render({ referrer: { ...kai, photo_url: "javascript:alert(1)" } });
  assert.doesNotMatch(body, /javascript:/);
  assert.match(body, /og-default\.png/);
});

test("an empty display name gets a fallback line, not a blank one", () => {
  const { body } = render({ referrer: { ...kai, display_name: "  " } });
  assert.match(body, /<meta property="og:title" content="You&#39;re invited to Monaco">/);
  assert.match(body, /<h1>You&#39;re invited to Monaco<\/h1>/);
  assert.doesNotMatch(body, /invited you to Monaco/);
});

test("an unknown code renders Get Monaco with the default image, no script and no copy", () => {
  const { status, body } = render({ referrer: null });
  assert.equal(status, 200);
  assert.match(body, />Get Monaco</);
  assert.match(body, /<meta property="og:title" content="Get Monaco">/);
  assert.match(body, /og-default\.png/);
  assert.match(body, /<a class="btn primary" href="https:\/\/testflight\.apple\.com\/join\/example">Get Monaco/);
  assert.doesNotMatch(body, /<script|clipboard|referrals\/clicks|og:url/);
});

test("the found page copies, posts the click, then goes to the App Store", () => {
  const { body } = render({ apiUrl: "http://api.test", posthogKey: "" });
  assert.match(body, /navigator\.clipboard\.writeText\(LINK\)/);
  assert.match(body, /"https:\/\/monacolabs\.xyz\/r\/k7m4qx2p"/);
  assert.match(body, /"Idempotency-Key": crypto\.randomUUID\(\)/);
  assert.match(body, /API \+ "\/v1\/referrals\/clicks"/);
  assert.match(body, /keepalive: true/);
  assert.match(body, /"http:\/\/api\.test"/);
  assert.ok(body.indexOf("clipboard") < body.indexOf("location.href = STORE"));
});

test("PostHog events are sent only when a key is set", () => {
  const off = render({ posthogKey: "" }).body;
  assert.match(off, /var PH_KEY = "";/);
  const on = render({ posthogKey: "phc_test" }).body;
  assert.match(on, /var PH_KEY = "phc_test";/);
  assert.match(on, /referral_page_viewed/);
  assert.match(on, /referral_get_tapped/);
});

test("a display name with markup is escaped everywhere it lands", () => {
  const evil = `<script>alert("x")</script>&'`;
  const { body } = render({ referrer: { ...kai, display_name: evil, photo_url: 'https://img.example/a.png"><script>1</script>' } });
  assert.doesNotMatch(body, /<script>alert/);
  assert.match(body, /&lt;script&gt;alert\(&quot;x&quot;\)&lt;\/script&gt;&amp;&#39;/);
  assert.equal((body.match(/<script>/g) || []).length, 1, "only the page's own script");
});

test("the app store url and api url cannot break out of the inline script", () => {
  const { body } = render({ appStoreUrl: "https://x.test/</script><script>alert(1)", apiUrl: "</script>" });
  assert.equal((body.match(/<script>/g) || []).length, 1);
  assert.equal((body.match(/<\/script>/g) || []).length, 1);
});

test("a missing app store url sends the button to the site, not nowhere", () => {
  assert.match(render({ appStoreUrl: undefined }).body, /id="get-monaco" href="\/"/);
});

test("codes are lowercased and validated", () => {
  assert.equal(normalizeCode("K7M4QX2P"), "k7m4qx2p");
  assert.equal(normalizeCode("ab"), null);
  assert.equal(normalizeCode("a".repeat(21)), null);
  assert.equal(normalizeCode("a-b"), null);
  assert.equal(normalizeCode('"><script>'), null);
  assert.equal(normalizeCode(undefined), null);
  const { body } = render({ code: '"><x' });
  assert.match(body, />Get Monaco</);
  assert.doesNotMatch(body, /og:url/);
});

test("Open in Monaco crosses hosts", () => {
  assert.equal(otherHost("monacolabs.xyz"), "www.monacolabs.xyz");
  assert.equal(otherHost("www.monacolabs.xyz"), "monacolabs.xyz");
  assert.equal(otherHost("localhost"), "www.monacolabs.xyz");
});

test("handleReferral looks the code up in lowercase and renders the found page", async () => {
  const fetch = fakeFetch();
  const res = await handleReferral({ code: "K7M4QX2P", host: "monacolabs.xyz", env, fetch });
  assert.equal(fetch.calls.length, 1);
  assert.equal(fetch.calls[0].url, "http://api.test/v1/referrals/k7m4qx2p");
  assert.match(res.body, /Kai invited you to Monaco/);
  assert.match(res.body, /https:\/\/www\.monacolabs\.xyz\/r\/k7m4qx2p/);
  assert.equal(res.headers["Cache-Control"], "public, max-age=300");
  const www = await handleReferral({ code: "k7m4qx2p", host: "www.monacolabs.xyz", env, fetch });
  assert.match(www.body, /href="https:\/\/monacolabs\.xyz\/r\/k7m4qx2p">Open in Monaco/);
});

test("an invalid code renders the unknown page without calling the API", async () => {
  const fetch = fakeFetch();
  const res = await handleReferral({ code: "no!", host: "monacolabs.xyz", env, fetch });
  assert.equal(fetch.calls.length, 0);
  assert.match(res.body, /<meta property="og:title" content="Get Monaco">/);
});

test("a null referrer from the API renders the unknown page", async () => {
  const res = await handleReferral({ code: "zzzzzzzz", host: "monacolabs.xyz", env, fetch: fakeFetch({ referrer: null }) });
  assert.match(res.body, /<meta property="og:title" content="Get Monaco">/);
  assert.equal(res.headers["Cache-Control"], "public, max-age=300");
});

test("an API error, a network error and a bad body fall back to the unknown page, uncached", async () => {
  for (const f of [fakeFetch({ status: 500 }), fakeFetch({ throws: true })]) {
    const res = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch: f });
    assert.match(res.body, /<meta property="og:title" content="Get Monaco">/);
    assert.equal(res.headers["Cache-Control"], "no-store");
  }
  const junk = async () => new Response("<html>", { status: 200 });
  const res = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch: junk });
  assert.match(res.body, /<meta property="og:title" content="Get Monaco">/);
});

test("an API that does not answer within the timeout falls back to the unknown page", async () => {
  const fetch = fakeFetch({ hang: true });
  await assert.rejects(lookupReferrer({ code: "k7m4qx2p", apiUrl: "http://api.test", fetch, timeoutMs: 20 }));
  const t0 = Date.now();
  const res = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch });
  assert.ok(Date.now() - t0 < 4000);
  assert.match(res.body, /<meta property="og:title" content="Get Monaco">/);
  assert.ok(fetch.calls.at(-1).init.signal instanceof AbortSignal);
});

// The Cloudflare adapter, called the way Pages calls it. globalThis.fetch is faked.
const realFetch = globalThis.fetch;
let stub;
beforeEach(() => { stub = fakeFetch(); globalThis.fetch = stub; });
afterEach(() => { globalThis.fetch = realFetch; });

test("the Cloudflare adapter serves the page with its headers and asks the edge cache to keep the lookup", async () => {
  const res = await cloudflare({ request: new Request("https://monacolabs.xyz/r/K7M4QX2P"), params: { code: "K7M4QX2P" }, env });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("cache-control"), "public, max-age=300");
  assert.match(res.headers.get("content-type"), /^text\/html/);
  assert.match(await res.text(), /Kai invited you to Monaco/);
  assert.equal(stub.calls[0].init.cf.cacheEverything, true);
  assert.equal(stub.calls[0].init.cf.cacheTtlByStatus["200-299"], 300);
});

test("the Cloudflare adapter takes the first value of a catch-all code", async () => {
  const res = await cloudflare({ request: new Request("https://www.monacolabs.xyz/r/k7m4qx2p"), params: { code: ["k7m4qx2p"] }, env });
  assert.match(await res.text(), /href="https:\/\/monacolabs\.xyz\/r\/k7m4qx2p">Open in Monaco/);
});

// Both hosts are live, so the Vercel adapter must give the same page as the Cloudflare one.
function vercelReq(code, host) {
  const headers = {};
  const res = {
    code: 0, body: "",
    setHeader(k, v) { headers[k.toLowerCase()] = v; },
    status(c) { this.code = c; return this; },
    send(b) { this.body = b; return this; },
  };
  return { req: { query: { code }, headers: { host } }, res, headers };
}

test("both adapters return identical bodies and status for the same input", async () => {
  for (const [code, host] of [["K7M4QX2P", "monacolabs.xyz"], ["k7m4qx2p", "www.monacolabs.xyz"], ["no!", "monacolabs.xyz"]]) {
    process.env.MONACO_API_URL = env.MONACO_API_URL;
    process.env.APP_STORE_URL = env.APP_STORE_URL;
    const cf = await cloudflare({ request: new Request(`https://${host}/r/${code}`), params: { code }, env });
    const v = vercelReq(code, host);
    await vercel(v.req, v.res);
    assert.equal(v.res.code, cf.status);
    assert.equal(v.res.body, await cf.text());
    assert.equal(v.headers["cache-control"], cf.headers.get("cache-control"));
    assert.equal(v.headers["content-type"], cf.headers.get("content-type"));
  }
});

test("the Vercel adapter reads the host a proxy forwarded and caches only a good answer", async () => {
  const ok = vercelReq("k7m4qx2p", "ignored.test");
  ok.req.headers["x-forwarded-host"] = "www.monacolabs.xyz";
  await vercel(ok.req, ok.res);
  assert.match(ok.res.body, /href="https:\/\/monacolabs\.xyz\/r\/k7m4qx2p">Open in Monaco/);
  assert.equal(ok.headers["cdn-cache-control"], "max-age=300");

  globalThis.fetch = fakeFetch({ throws: true });
  const bad = vercelReq("k7m4qx2p", "monacolabs.xyz");
  await vercel(bad.req, bad.res);
  assert.equal(bad.headers["cache-control"], "no-store");
  assert.equal(bad.headers["cdn-cache-control"], undefined);
});

// The cabal landing: a link that carries ?c=<cabal_id>.
const CABAL_ID = "01890a5d-ac96-774b-bcce-b302099a8058";
const PROPOSAL_ID = "01890a5d-ac96-774b-bcce-b302099a8057";
const preview = {
  inviter: { display_name: "Andres", photo_url: null },
  cabal: {
    id: CABAL_ID, name: "AC Capital", picture_url: "https://img.example/cover.png", member_count: 8, since: "2026-08-14",
    join_mode: "open", first_members: [{ display_name: "Christelle", photo_url: null }], pot_value_micros: "20000000000",
  },
  return_1m_bps: -410,
  turnout_month_bps: 9400,
  open_proposal: null,
};

// Answers the cabal route with `cabalBody` (or throws / fails) and every other route like fakeFetch.
function routedFetch({ cabalBody = preview, cabalStatus = 200, cabalThrows = false, referrer = kai } = {}) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, init });
    if (String(url).includes("/cabals/")) {
      if (cabalThrows) throw new TypeError("fetch failed");
      return new Response(JSON.stringify(cabalBody), { status: cabalStatus, headers: { "content-type": "application/json" } });
    }
    return new Response(JSON.stringify({ referrer }), { status: 200, headers: { "content-type": "application/json" } });
  };
  fn.calls = calls;
  return fn;
}

const cabalLink = (over = {}) => ({ code: "k7m4qx2p", cabalId: CABAL_ID, host: "monacolabs.xyz", env, ...over });

test("ids are accepted only as UUIDs", () => {
  assert.equal(normalizeId(CABAL_ID.toUpperCase()), CABAL_ID);
  for (const bad of [undefined, null, "", "abc", `${CABAL_ID}x`, "01890a5d-ac96-774b-bcce-b302099a805", '"><script>', ["x"]]) {
    assert.equal(normalizeId(bad), null);
  }
});

test("a valid c with a cabal renders the cabal landing and looks the cabal up", async () => {
  const fetch = routedFetch();
  const res = await handleReferral({ ...cabalLink(), cabalId: CABAL_ID.toUpperCase(), fetch });
  assert.equal(fetch.calls.length, 1);
  assert.equal(fetch.calls[0].url, `http://api.test/v1/referrals/k7m4qx2p/cabals/${CABAL_ID}`);
  assert.match(res.body, /<h1>AC Capital<\/h1>/);
  assert.match(res.body, /<meta property="og:title" content="Andres invited you to AC Capital">/);
  assert.match(res.body, /<dt>Treasury<\/dt><dd>\$20,000<\/dd>/);
  assert.match(res.body, new RegExp(`href="https://www\\.monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}">Open in Monaco`));
  assert.match(res.body, new RegExp(`var LINK = "https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}";`));
});

test("a valid p rides along in the clipboard link and the Open in Monaco link, on both hosts", async () => {
  const fetch = routedFetch();
  const apex = await handleReferral({ ...cabalLink(), proposalId: PROPOSAL_ID, fetch });
  assert.match(apex.body, new RegExp(`var LINK = "https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}\\\\u0026p=${PROPOSAL_ID}";`));
  assert.match(apex.body, new RegExp(`href="https://www\\.monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}&amp;p=${PROPOSAL_ID}">Open in Monaco`));
  const www = await handleReferral({ ...cabalLink({ host: "www.monacolabs.xyz" }), proposalId: PROPOSAL_ID, fetch });
  assert.match(www.body, new RegExp(`href="https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}&amp;p=${PROPOSAL_ID}">Open in Monaco`));
  const junk = await handleReferral({ ...cabalLink({ host: "www.monacolabs.xyz" }), proposalId: "not-a-uuid", fetch });
  assert.doesNotMatch(junk.body, /&amp;p=|u0026p=/);
});

test("the cabal page lives 60 s and the referrer page keeps 300; a failed lookup is no-store with no CDN age", async () => {
  const cabal = await handleReferral({ ...cabalLink(), fetch: routedFetch() });
  assert.equal(cabal.headers["Cache-Control"], "public, max-age=60");
  assert.equal(cabal.cdnMaxAge, 60);
  const referrerPage = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch: fakeFetch() });
  assert.equal(referrerPage.headers["Cache-Control"], "public, max-age=300");
  assert.equal(referrerPage.cdnMaxAge, 300);
  const failed = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch: fakeFetch({ throws: true }) });
  assert.equal(failed.headers["Cache-Control"], "no-store");
  assert.equal(failed.cdnMaxAge, undefined);
});

test("c absent or invalid renders the referrer page byte for byte, without asking the cabal route", async () => {
  const plain = await handleReferral({ code: "k7m4qx2p", host: "monacolabs.xyz", env, fetch: fakeFetch() });
  const expected = renderReferralPage({
    code: "k7m4qx2p", referrer: kai, appStoreUrl: STORE, openHost: "www.monacolabs.xyz", apiUrl: env.MONACO_API_URL, posthogKey: "",
  });
  assert.equal(plain.body, expected.body);
  assert.deepEqual(plain.headers, expected.headers);
  for (const cabalId of [undefined, null, "", "nope", `${CABAL_ID}0`]) {
    const fetch = routedFetch();
    const res = await handleReferral({ ...cabalLink({ cabalId }), fetch });
    assert.equal(res.body, expected.body);
    assert.equal(res.cdnMaxAge, 300);
    assert.equal(fetch.calls.length, 1);
    assert.doesNotMatch(fetch.calls[0].url, /cabals/);
  }
});

test("a null cabal falls back to today's referrer page, cached as before", async () => {
  const fetch = routedFetch({ cabalBody: { inviter: kai, cabal: null, return_1m_bps: null, turnout_month_bps: null, open_proposal: null } });
  const res = await handleReferral({ ...cabalLink(), fetch });
  assert.equal(res.body, renderReferralPage({
    code: "k7m4qx2p", referrer: kai, appStoreUrl: STORE, openHost: "www.monacolabs.xyz", apiUrl: env.MONACO_API_URL, posthogKey: "",
  }).body);
  assert.equal(res.headers["Cache-Control"], "public, max-age=300");
  assert.equal(res.cdnMaxAge, 300);
  assert.equal(fetch.calls.length, 2);
});

test("a failed cabal lookup falls back to the referrer page and is never cached", async () => {
  const cases = [
    routedFetch({ cabalStatus: 429 }),
    routedFetch({ cabalStatus: 500 }),
    routedFetch({ cabalThrows: true }),
    routedFetch({ cabalBody: { oops: true } }),
    routedFetch({ cabalBody: { ...preview, cabal: { ...preview.cabal, name: "" } } }),
    routedFetch({ cabalBody: { ...preview, cabal: { ...preview.cabal, pot_value_micros: "-1" } } }),
  ];
  for (const fetch of cases) {
    const res = await handleReferral({ ...cabalLink(), fetch });
    assert.match(res.body, /Kai invited you to Monaco/);
    assert.doesNotMatch(res.body, /AC Capital/);
    assert.equal(res.headers["Cache-Control"], "no-store");
    assert.equal(res.cdnMaxAge, undefined);
    assert.equal(fetch.calls.filter((c) => String(c.url).includes("/cabals/")).length, 1, "no retry loop");
  }
});

test("lookupCabal times out like lookupReferrer and merges the month facts into the cabal", async () => {
  const hang = fakeFetch({ hang: true });
  await assert.rejects(lookupCabal({ code: "k7m4qx2p", cabalId: CABAL_ID, apiUrl: "http://api.test", fetch: hang, timeoutMs: 20 }));
  const found = await lookupCabal({ code: "k7m4qx2p", cabalId: CABAL_ID, apiUrl: "http://api.test/", fetch: routedFetch() });
  assert.equal(found.referrer.display_name, "Andres");
  assert.equal(found.cabal.return_1m_bps, -410);
  assert.equal(found.cabal.turnout_month_bps, 9400);
  assert.equal(found.cabal.open_proposal, null);
  assert.equal(found.cabal.name, "AC Capital");
});

test("the Cloudflare adapter passes c and p, and asks the edge cache for 60 s on the cabal lookup and 300 s on the referrer's", async () => {
  globalThis.fetch = routedFetch();
  const res = await cloudflare({ request: new Request(`https://monacolabs.xyz/r/k7m4qx2p?c=${CABAL_ID}&p=${PROPOSAL_ID}`), params: { code: "k7m4qx2p" }, env });
  assert.equal(res.headers.get("cache-control"), "public, max-age=60");
  const text = await res.text();
  assert.match(text, /<h1>AC Capital<\/h1>/);
  assert.match(text, new RegExp(`c=${CABAL_ID}&amp;p=${PROPOSAL_ID}`));
  const [cabalCall] = globalThis.fetch.calls;
  assert.match(cabalCall.url, /\/cabals\//);
  assert.equal(cabalCall.init.cf.cacheEverything, true);
  assert.deepEqual(cabalCall.init.cf.cacheTtlByStatus, { "200-299": 60, "300-599": 0 });
  await cloudflare({ request: new Request("https://monacolabs.xyz/r/k7m4qx2p"), params: { code: "k7m4qx2p" }, env });
  assert.deepEqual(globalThis.fetch.calls.at(-1).init.cf.cacheTtlByStatus, { "200-299": 300, "300-599": 0 });
});

test("the Vercel adapter passes c and p, and sets CDN-Cache-Control 60 for a cabal page, 300 for a referrer page, none for a failure", async () => {
  process.env.MONACO_API_URL = env.MONACO_API_URL;
  process.env.APP_STORE_URL = env.APP_STORE_URL;
  globalThis.fetch = routedFetch();
  const cabal = vercelReq("k7m4qx2p", "monacolabs.xyz");
  cabal.req.query.c = CABAL_ID;
  cabal.req.query.p = [PROPOSAL_ID];
  await vercel(cabal.req, cabal.res);
  assert.match(cabal.res.body, /<h1>AC Capital<\/h1>/);
  assert.match(cabal.res.body, new RegExp(`c=${CABAL_ID}&amp;p=${PROPOSAL_ID}`));
  assert.equal(cabal.headers["cache-control"], "public, max-age=60");
  assert.equal(cabal.headers["cdn-cache-control"], "max-age=60");

  const referrerPage = vercelReq("k7m4qx2p", "monacolabs.xyz");
  await vercel(referrerPage.req, referrerPage.res);
  assert.equal(referrerPage.headers["cdn-cache-control"], "max-age=300");

  globalThis.fetch = routedFetch({ cabalThrows: true });
  const failed = vercelReq("k7m4qx2p", "monacolabs.xyz");
  failed.req.query.c = CABAL_ID;
  await vercel(failed.req, failed.res);
  assert.equal(failed.headers["cache-control"], "no-store");
  assert.equal(failed.headers["cdn-cache-control"], undefined);
});

test("both adapters return identical cabal pages", async () => {
  process.env.MONACO_API_URL = env.MONACO_API_URL;
  process.env.APP_STORE_URL = env.APP_STORE_URL;
  globalThis.fetch = routedFetch();
  const cf = await cloudflare({ request: new Request(`https://monacolabs.xyz/r/k7m4qx2p?c=${CABAL_ID}`), params: { code: "k7m4qx2p" }, env });
  const v = vercelReq("k7m4qx2p", "monacolabs.xyz");
  v.req.query.c = CABAL_ID;
  await vercel(v.req, v.res);
  assert.equal(v.res.body, await cf.text());
  assert.equal(v.headers["cache-control"], cf.headers.get("cache-control"));
});

// The AASA and its headers, read from the files the hosts serve.
const here = (p) => new URL(p, import.meta.url);

test("the AASA parses and lists /r/* for the app", () => {
  const aasa = JSON.parse(readFileSync(here("../public/.well-known/apple-app-site-association"), "utf8"));
  const [detail] = aasa.applinks.details;
  assert.deepEqual(detail.appIDs, ["8Q2V9WHNNT.xyz.monacolabs.app"]);
  assert.deepEqual(detail.components, [{ "/": "/r/*" }]);
});

test("both hosts serve the AASA as application/json and Vercel rewrites /r/:code", () => {
  const vj = JSON.parse(readFileSync(here("../vercel.json"), "utf8"));
  const rule = vj.headers.find((h) => h.source === "/.well-known/apple-app-site-association");
  assert.deepEqual(rule.headers, [{ key: "Content-Type", value: "application/json" }]);
  assert.deepEqual(vj.rewrites, [{ source: "/r/:code", destination: "/api/r/:code" }]);
  assert.match(readFileSync(here("../public/_headers"), "utf8"),
    /^\/\.well-known\/apple-app-site-association\n  Content-Type: application\/json$/m);
});
