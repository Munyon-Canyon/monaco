import { test } from "node:test";
import assert from "node:assert/strict";
import { renderCabalLanding } from "../lib/invite-landing.js";
import { renderReferralPage } from "../lib/referral.js";

const STORE = "https://testflight.apple.com/join/example";
const CABAL_ID = "01890a5d-ac96-774b-bcce-b302099a8058";
const PROPOSAL_ID = "01890a5d-ac96-774b-bcce-b302099a8057";
const NOW = Date.parse("2026-10-28T12:00:00Z");

const referrer = { display_name: "Andres", photo_url: "https://img.example/andres.png" };
const proposal = {
  id: PROPOSAL_ID, kind: "buy", symbol: "NVDAx", asset_name: "NVIDIA", asset_logo_url: "https://img.example/nvda.png",
  usdc_micros: "400000000", proposer_name: "Mia", title: "small add before earnings", yes: 4, no: 1, voted: 5, eligible: 8,
  replies: 6, expires_at: "2026-10-28T16:00:00Z",
};
const cabal = {
  id: CABAL_ID, name: "AC Capital", picture_url: "https://img.example/cover.png", member_count: 8, since: "2026-08-14",
  join_mode: "open", pot_value_micros: "20000000000",
  first_members: [
    { display_name: "Christelle", photo_url: "https://img.example/c.png" },
    { display_name: "Logan", photo_url: null },
    { display_name: "Mia", photo_url: null },
    { display_name: "Sam", photo_url: null },
  ],
  return_1m_bps: -410, turnout_month_bps: 9400, open_proposal: proposal,
};

const render = (over = {}) => renderCabalLanding({
  code: "k7m4qx2p", cabalId: CABAL_ID, referrer, cabal, appStoreUrl: STORE, openHost: "www.monacolabs.xyz", now: NOW, ...over,
});

test("a cabal with data renders every section of the invite landing", () => {
  const { status, body } = render();
  assert.equal(status, 200);
  assert.match(body, /<img src="\/assets\/wordmark\.svg" alt="Monaco"/);
  assert.match(body, /<img class="cover" src="https:\/\/img\.example\/cover\.png"/);
  assert.match(body, /<img class="crest" src="https:\/\/img\.example\/cover\.png"/);
  assert.match(body, /<h1>AC Capital<\/h1>/);
  assert.match(body, /Cabal · 8 members · since Aug 2026/);
  assert.match(body, /<b>Andres<\/b> invited you/);
  assert.match(body, /src="https:\/\/img\.example\/andres\.png"/);
  assert.equal((body.match(/<span class="face"/g) || []).length, 3, "three members have no photo and draw initials");
  assert.match(body, /Christelle, Logan, Mia and 4 more/);
  assert.match(body, /<dt>Treasury<\/dt><dd>\$20,000<\/dd>/);
  assert.match(body, /<dt>1M<\/dt><dd class="loss">−4\.1%<\/dd>/);
  assert.match(body, /<dt>Turnout<\/dt><dd>94%<\/dd>/);
  assert.match(body, /<h2>Open now<\/h2>/);
  assert.match(body, /<b>Buy NVDA · \$400<\/b>/);
  assert.match(body, /Mia · small add before earnings/);
  assert.match(body, /<span>✓ 4<\/span><span class="mid">Join to vote<\/span><span>1 ✕<\/span>/);
  assert.match(body, /5 of 8 voted · 6 replies · ends in 4h/);
  assert.match(body, /id="get-monaco" href="https:\/\/testflight\.apple\.com\/join\/example">Join AC Capital</);
  assert.match(body, /Get Monaco, then open this link again or tap Paste invite\./);
  assert.match(body, />Open in Monaco</);
});

test("without data the sections that need it are left out or dashed", () => {
  const { body } = render({
    cabal: { ...cabal, picture_url: null, first_members: [], return_1m_bps: null, turnout_month_bps: null, open_proposal: null },
  });
  assert.match(body, /<div class="cover" aria-hidden="true"><\/div>/);
  assert.match(body, /<span class="crest" style="background:#[0-9A-F]{6}"[^>]*>AC<\/span>/);
  assert.doesNotMatch(body, /class="members"|Open now/);
  assert.match(body, /<dt>1M<\/dt><dd class="">—<\/dd>/);
  assert.match(body, /<dt>Turnout<\/dt><dd>—<\/dd>/);
  assert.match(body, /og-default\.png/);
});

test("a no-proposal variant still carries the join and open actions", () => {
  const { body } = render({ cabal: { ...cabal, open_proposal: null } });
  assert.doesNotMatch(body, /Open now|Join to vote/);
  assert.match(body, /id="get-monaco"/);
  assert.match(body, />Open in Monaco</);
});

test("a request cabal asks to join", () => {
  assert.match(render({ cabal: { ...cabal, join_mode: "request" } }).body, />Ask to join AC Capital</);
});

test("the stats read the way the frame does", () => {
  const up = render({ cabal: { ...cabal, return_1m_bps: 1250, turnout_month_bps: 6666, pot_value_micros: "1234567890000" } }).body;
  assert.match(up, /<dd class="gain">\+12\.5%<\/dd>/);
  assert.match(up, /<dd>67%<\/dd>/);
  assert.match(up, /<dd>\$1,234,568<\/dd>/);
  assert.match(render({ cabal: { ...cabal, return_1m_bps: 0 } }).body, /<dd class="">0\.0%<\/dd>/);
});

test("the proposal reads buy and sell, amounts, reply counts and the time left", () => {
  const sell = render({ cabal: { ...cabal, open_proposal: { ...proposal, kind: "sell", usdc_micros: null, replies: null, expires_at: "2026-10-28T12:35:00Z" } } }).body;
  assert.match(sell, /<b>Sell NVDA<\/b>/);
  assert.match(sell, /5 of 8 voted · ends in 35m/);
  const cents = render({ cabal: { ...cabal, open_proposal: { ...proposal, usdc_micros: "1500000", replies: 1, expires_at: "2026-10-30T12:00:00Z" } } }).body;
  assert.match(cents, /Buy NVDA · \$1\.50/);
  assert.match(cents, /1 reply · ends in 2d/);
  const ended = render({ cabal: { ...cabal, open_proposal: { ...proposal, expires_at: "2026-10-28T11:00:00Z" } } }).body;
  assert.match(ended, /ends soon/);
  assert.doesNotMatch(render({ cabal: { ...cabal, open_proposal: { ...proposal, kind: "swap" } } }).body, /Open now/);
});

test("every interpolated value is escaped", () => {
  const evil = `<script>alert("x")</script>&'`;
  const { body } = render({
    referrer: { display_name: evil, photo_url: 'https://img.example/a.png"><script>1</script>' },
    cabal: {
      ...cabal, name: evil, picture_url: 'https://img.example/b.png"><script>2</script>',
      first_members: [{ display_name: evil, photo_url: null }],
      open_proposal: { ...proposal, symbol: evil, proposer_name: evil, title: evil, asset_logo_url: 'https://img.example/c.png"><script>3</script>' },
    },
  });
  assert.doesNotMatch(body, /<script>alert|<script>[123]</);
  assert.match(body, /&lt;script&gt;alert\(&quot;x&quot;\)&lt;\/script&gt;&amp;&#39;/);
  assert.equal((body.match(/<script>/g) || []).length, 1, "only the page's own script");
});

test("a cover or photo that is not an http url is not used", () => {
  const { body } = render({
    referrer: { display_name: "Andres", photo_url: "javascript:alert(1)" },
    cabal: { ...cabal, picture_url: "data:text/html,x", open_proposal: { ...proposal, asset_logo_url: "javascript:1" } },
  });
  assert.doesNotMatch(body, /javascript:|data:text/);
  assert.match(body, /og-default\.png/);
});

test("the copy action writes the link with ?c=, and &p= when the request carried one", () => {
  assert.match(render().body, /var LINK = "https:\/\/monacolabs\.xyz\/r\/k7m4qx2p\?c=01890a5d-ac96-774b-bcce-b302099a8058";/);
  const withP = render({ proposalId: PROPOSAL_ID }).body;
  assert.match(withP, new RegExp(`var LINK = "https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}\\\\u0026p=${PROPOSAL_ID}";`));
  assert.match(withP, /navigator\.clipboard\.writeText\(LINK\)/);
  assert.match(withP, /API \+ "\/v1\/referrals\/clicks"/);
  assert.ok(withP.indexOf("clipboard") < withP.indexOf("location.href = STORE"));
});

test("Open in Monaco keeps ?c= (and &p=) on both hosts", () => {
  assert.match(render().body, new RegExp(`href="https://www\\.monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}">Open in Monaco`));
  assert.match(render({ openHost: "monacolabs.xyz", proposalId: PROPOSAL_ID }).body,
    new RegExp(`href="https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}&amp;p=${PROPOSAL_ID}">Open in Monaco`));
});

test("the preview tags name the referrer and the cabal, with the cover and ?c= in the url", () => {
  const { body } = render();
  assert.match(body, /<title>Andres invited you to AC Capital<\/title>/);
  assert.match(body, /<meta property="og:title" content="Andres invited you to AC Capital">/);
  assert.match(body, /<meta property="og:description" content="8 members on Monaco">/);
  assert.match(body, /<meta property="og:image" content="https:\/\/img\.example\/cover\.png">/);
  assert.match(body, new RegExp(`<meta property="og:url" content="https://monacolabs\\.xyz/r/k7m4qx2p\\?c=${CABAL_ID}">`));
  assert.match(render({ cabal: { ...cabal, picture_url: null } }).body, /<meta property="og:image" content="https:\/\/monacolabs\.xyz\/assets\/og-default\.png">/);
  assert.match(render({ referrer: null }).body, /<meta property="og:title" content="You&#39;re invited to AC Capital">/);
});

test("the page is cached for 60 seconds", () => {
  const { headers } = render();
  assert.equal(headers["Cache-Control"], "public, max-age=60");
  assert.match(headers["Content-Type"], /^text\/html/);
});

test("the stylesheet is scoped to this page: dark tokens as custom properties, no gradients, no crypto words", () => {
  const { body } = render();
  assert.match(body, /color-scheme: dark/);
  assert.match(body, /--secondary: #A1A1A6/);
  assert.match(body, /-apple-system, BlinkMacSystemFont, "SF Pro Text"/);
  assert.doesNotMatch(body, /gradient|Outfit|F8F5EE/i);
  assert.doesNotMatch(body.replace(/<style>[^]*?<\/style>|<script>[^]*?<\/script>/g, ""), /crypto|token|wallet|blockchain/i);
  const referrerPage = renderReferralPage({ code: "k7m4qx2p", referrer: { display_name: "Kai", photo_url: null }, appStoreUrl: STORE, openHost: "www.monacolabs.xyz" }).body;
  assert.doesNotMatch(referrerPage, /--canvas|color-scheme: dark/);
  assert.match(referrerPage, /color-scheme: light/);
});

test("the app store url cannot break out of the inline script", () => {
  const { body } = render({ appStoreUrl: "https://x.test/</script><script>alert(1)", apiUrl: "</script>" });
  assert.equal((body.match(/<script>/g) || []).length, 1);
  assert.equal((body.match(/<\/script>/g) || []).length, 1);
});
