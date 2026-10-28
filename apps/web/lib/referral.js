// The /r/<code> invite page: look the referrer up and render the HTML.
// Pure logic with injected fetch/env so every unhappy path is testable. The Cloudflare and
// Vercel adapters are thin wrappers over handleReferral().
// A link that carries a cabal (?c=<cabal_id>) renders lib/invite-landing.js instead. That module
// imports helpers back from here, so neither module reads the other's exports while loading.

import { renderCabalLanding } from "./invite-landing.js";

export const SITE = "https://monacolabs.xyz";
export const APEX = "monacolabs.xyz";
export const WWW = "www.monacolabs.xyz";
const CODE = /^[a-z0-9_]{3,20}$/;
const LOOKUP_TIMEOUT_MS = 2000;
const CACHE_CONTROL = "public, max-age=300";
const CDN_MAX_AGE = 300;
const CABAL_MAX_AGE = 60;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export const DEFAULT_IMAGE = `${SITE}/assets/og-default.png`;
export const DESCRIPTION = "Invest together with friends.";
const POSTHOG_CAPTURE = "https://us.i.posthog.com/capture/";

// Lowercased code if it is a well-formed invite code, else null.
export function normalizeCode(value) {
  if (typeof value !== "string") return null;
  const code = value.trim().toLowerCase();
  return CODE.test(code) ? code : null;
}

// Lowercased id if it is a UUID, else null. Used for the cabal (c) and proposal (p) of a link.
export function normalizeId(value) {
  if (typeof value !== "string") return null;
  const id = value.trim().toLowerCase();
  return UUID.test(id) ? id : null;
}

// "Open in Monaco" crosses hosts: iOS does not open a Universal Link for a navigation that
// stays on the same host, so the apex page links to www and the www page links to the apex.
export function otherHost(host) {
  return String(host || "").toLowerCase() === WWW ? APEX : WWW;
}

export function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (c) => (
    { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]
  ));
}

// JSON that is safe inside an inline <script>: no "</script>" and no HTML comment openers.
export function scriptJson(value) {
  return JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026");
}

export function isHttpUrl(value) {
  if (typeof value !== "string") return false;
  try {
    const u = new URL(value);
    return u.protocol === "https:" || u.protocol === "http:";
  } catch {
    return false;
  }
}

const STYLE = `
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px;
    background: #F8F5EE; color: #0F291C; font: 17px/1.55 "Outfit", "Helvetica Neue", Arial, sans-serif; }
  main { max-width: 420px; width: 100%; text-align: center; }
  img.avatar { width: 96px; height: 96px; border-radius: 50%; object-fit: cover; margin: 0 auto 20px; display: block; }
  h1 { font-size: 28px; line-height: 1.2; margin: 0 0 8px; }
  p { margin: 0 0 28px; color: rgba(15, 41, 28, 0.68); }
  .btn { display: flex; align-items: center; justify-content: center; min-height: 48px; margin: 0 0 12px;
    border-radius: 999px; border: 1px solid #0F291C; font-weight: 600; font-size: 16px; text-decoration: none; }
  .primary { background: #0F291C; color: #fff; }
  .ghost { background: transparent; color: #0F291C; }
`;

export function script({ code, apiUrl, appStoreUrl, posthogKey, link = `${SITE}/r/${code}` }) {
  return `
(function () {
  var CODE = ${scriptJson(code)};
  var LINK = ${scriptJson(link)};
  var API = ${scriptJson(apiUrl)};
  var STORE = ${scriptJson(appStoreUrl)};
  var PH_KEY = ${scriptJson(posthogKey)};
  var PH_URL = ${scriptJson(POSTHOG_CAPTURE)};
  var visitor = crypto.randomUUID();
  function track(event) {
    if (!PH_KEY) return;
    try {
      fetch(PH_URL, { method: "POST", keepalive: true, headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ api_key: PH_KEY, event: event, distinct_id: visitor, properties: { code: CODE } }) }).catch(function () {});
    } catch (e) {}
  }
  track("referral_page_viewed");
  document.getElementById("get-monaco").addEventListener("click", function (e) {
    e.preventDefault();
    track("referral_get_tapped");
    try { navigator.clipboard.writeText(LINK).catch(function () {}); } catch (err) {}
    try {
      fetch(API + "/v1/referrals/clicks", { method: "POST", keepalive: true,
        headers: { "Idempotency-Key": crypto.randomUUID(), "Content-Type": "application/json" },
        body: JSON.stringify({ code: CODE }) }).catch(function () {});
    } catch (err) {}
    location.href = STORE;
  });
})();
`;
}

// Returns {status, headers, body}. referrer is null for an unknown code; code is validated
// here so a malformed one gets the unknown page whatever the caller passed.
export function renderReferralPage({ code, referrer, appStoreUrl, openHost, apiUrl = "", posthogKey = "" }) {
  const clean = normalizeCode(code);
  const found = clean !== null && referrer !== null && typeof referrer === "object";
  const store = isHttpUrl(appStoreUrl) ? appStoreUrl : "/";
  const name = found && typeof referrer.display_name === "string" ? referrer.display_name.trim() : "";
  const title = !found ? "Get Monaco" : name ? `${name} invited you to Monaco` : "You're invited to Monaco";
  const photo = found && isHttpUrl(referrer.photo_url) ? referrer.photo_url : null;
  const image = photo || DEFAULT_IMAGE;
  const heading = !found ? "Get Monaco" : title;

  const meta = [
    `<meta property="og:type" content="website">`,
    `<meta property="og:title" content="${escapeHtml(title)}">`,
    `<meta property="og:description" content="${DESCRIPTION}">`,
    `<meta property="og:image" content="${escapeHtml(image)}">`,
    `<meta name="twitter:card" content="summary">`,
    `<meta name="twitter:title" content="${escapeHtml(title)}">`,
    `<meta name="twitter:image" content="${escapeHtml(image)}">`,
  ];
  if (found) meta.splice(1, 0, `<meta property="og:url" content="${SITE}/r/${clean}">`);

  const avatar = photo ? `<img class="avatar" src="${escapeHtml(photo)}" alt="" width="96" height="96">` : "";
  const buttons = found
    ? `<a class="btn primary" id="get-monaco" href="${escapeHtml(store)}">Get Monaco</a>
    <a class="btn ghost" href="https://${escapeHtml(openHost || WWW)}/r/${clean}">Open in Monaco</a>`
    : `<a class="btn primary" href="${escapeHtml(store)}">Get Monaco</a>`;

  const body = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>${escapeHtml(title)}</title>
<meta name="description" content="${DESCRIPTION}">
<link rel="icon" href="/assets/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/assets/apple-touch-icon.png">
${meta.join("\n")}
<style>${STYLE}</style>
</head>
<body>
<main>
    ${avatar}
    <h1>${escapeHtml(heading)}</h1>
    <p>${DESCRIPTION}</p>
    ${buttons}
</main>${found ? `\n<script>${script({ code: clean, apiUrl, appStoreUrl: store, posthogKey })}</script>` : ""}
</body>
</html>
`;
  return {
    status: 200,
    headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": CACHE_CONTROL },
    body,
  };
}

// Runs one JSON GET with the lookup timeout. Rejects on a timeout, a network error, a non-2xx
// answer or a body that is not JSON.
async function getJson(url, fetch, timeoutMs) {
  // An AbortController and a plain timer rather than AbortSignal.timeout(): that timer is
  // unref'd, so a lookup that never answers would let a bare Node process exit mid-await.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(new DOMException("referral lookup timed out", "TimeoutError")), timeoutMs);
  try {
    const res = await fetch(url, { headers: { Accept: "application/json" }, signal: controller.signal });
    if (!res.ok) throw new Error(`referral lookup answered ${res.status}`);
    return await res.json();
  } finally {
    clearTimeout(timer);
  }
}

const isCount = (v) => Number.isInteger(v) && v >= 0;
const isDate = (v) => typeof v === "string" && /^\d{4}-\d{2}-\d{2}/.test(v);

// GET {apiUrl}/v1/referrals/{code}/cabals/{cabalId}. Resolves to {referrer, cabal}, where cabal
// carries the month facts (return_1m_bps, turnout_month_bps, open_proposal) beside its own, or
// to null when the link does not resolve to a cabal. Rejects like lookupReferrer, and on a cabal
// that is missing a field the page draws.
export async function lookupCabal({ code, cabalId, apiUrl, fetch, timeoutMs = LOOKUP_TIMEOUT_MS }) {
  const url = `${String(apiUrl).replace(/\/+$/, "")}/v1/referrals/${encodeURIComponent(code)}/cabals/${encodeURIComponent(cabalId)}`;
  const data = await getJson(url, fetch, timeoutMs);
  if (!data || typeof data !== "object" || !("cabal" in data)) throw new Error("cabal lookup answered an unexpected body");
  const c = data.cabal;
  if (c === null) return null;
  if (typeof c !== "object" || typeof c.name !== "string" || !c.name.trim() || !isCount(c.member_count) || !isDate(c.since)
    || typeof c.pot_value_micros !== "string" || !/^[0-9]+$/.test(c.pot_value_micros)) {
    throw new Error("cabal lookup answered an unexpected body");
  }
  const referrer = data.inviter && typeof data.inviter === "object" ? data.inviter : null;
  return {
    referrer,
    cabal: {
      ...c,
      return_1m_bps: data.return_1m_bps ?? null,
      turnout_month_bps: data.turnout_month_bps ?? null,
      open_proposal: data.open_proposal && typeof data.open_proposal === "object" ? data.open_proposal : null,
    },
  };
}

// GET {apiUrl}/v1/referrals/{code}. Resolves to the referrer object, or null when the code
// does not resolve. Rejects on a timeout, a network error or a malformed answer.
export async function lookupReferrer({ code, apiUrl, fetch, timeoutMs = LOOKUP_TIMEOUT_MS }) {
  const data = await getJson(`${String(apiUrl).replace(/\/+$/, "")}/v1/referrals/${encodeURIComponent(code)}`, fetch, timeoutMs);
  if (!data || typeof data !== "object" || !("referrer" in data)) throw new Error("referral lookup answered an unexpected body");
  return data.referrer && typeof data.referrer === "object" ? data.referrer : null;
}

// What an adapter calls: validate the code, look it up, render. Any lookup error renders the
// unknown page, marked no-store so a transient failure is not cached for five minutes.
// fetch is the host's, with its edge cache turned on: the lookup is rate limited per IP, and
// without the cache every visitor would share the function's bucket.
// cabalId and proposalId are the raw ?c= and ?p= of the link. A valid cabalId whose preview
// resolves renders the cabal landing, cached for 60 s because it shows a live tally; anything
// else renders the referrer page. cdnMaxAge is how long a CDN may keep the answer, absent on a
// no-store one.
export async function handleReferral({ code, cabalId, proposalId, host, env, fetch }) {
  const clean = normalizeCode(code);
  const cabal = normalizeId(cabalId);
  const page = {
    code: clean,
    appStoreUrl: env.APP_STORE_URL,
    openHost: otherHost(host),
    apiUrl: env.MONACO_API_URL || "",
    posthogKey: env.POSTHOG_KEY || "",
  };
  const cacheable = (res, cdnMaxAge) => ({ ...res, cdnMaxAge });
  const uncached = (res) => {
    res.headers["Cache-Control"] = "no-store";
    return res;
  };
  if (!clean) return cacheable(renderReferralPage({ ...page, referrer: null }), CDN_MAX_AGE);
  let cabalFailed = false;
  if (cabal) {
    try {
      const found = await lookupCabal({ code: clean, cabalId: cabal, apiUrl: env.MONACO_API_URL, fetch });
      if (found) {
        const res = renderCabalLanding({ ...page, cabalId: cabal, proposalId: normalizeId(proposalId), ...found });
        return cacheable(res, CABAL_MAX_AGE);
      }
    } catch {
      cabalFailed = true;
    }
  }
  try {
    const referrer = await lookupReferrer({ code: clean, apiUrl: env.MONACO_API_URL, fetch });
    const res = renderReferralPage({ ...page, referrer });
    return cabalFailed ? uncached(res) : cacheable(res, CDN_MAX_AGE);
  } catch {
    return uncached(renderReferralPage({ ...page, referrer: null }));
  }
}
