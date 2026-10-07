import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { renderReferralPage, handleReferral, lookupReferrer, normalizeCode, otherHost } from "../lib/referral.js";
import { onRequest as cloudflare } from "../functions/r/[code].js";

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
