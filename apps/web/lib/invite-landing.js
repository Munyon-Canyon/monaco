// The cabal version of the /r/<code> page, shown when the link carries ?c=<cabal_id> and the
// preview resolves. It follows frame 18-invite-landing as a web page. Pure rendering: the lookup
// and the choice between this page and the referrer page live in lib/referral.js.
// referral.js and this module import each other, so the imports below are read only inside
// functions, never while the module loads.

import { DEFAULT_IMAGE, SITE, WWW, escapeHtml, isHttpUrl, script } from "./referral.js";

const CACHE_CONTROL = "public, max-age=60";
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const HUES = ["#5E5CE6", "#FF9F0A", "#40C8E0", "#FF375F", "#BF5AF2", "#AC8E68", "#0A84FF", "#64D2FF"];
const MINUS = "−";

// F's tokens (docs/design/f/kit.md, 01-tokens), defined on this page's root only.
const STYLE = `
  :root { color-scheme: dark; --canvas: #000000; --cell: #1C1C1E; --fill: #2C2C2E; --label: #FFFFFF;
    --secondary: #A1A1A6; --brand: #1E5C3A; --gain: #30F57A; --loss: #FF453A;
    --font: -apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", system-ui, "Helvetica Neue", Arial, sans-serif; }
  * { box-sizing: border-box; }
  body { margin: 0; background: var(--canvas); color: var(--label); font: 17px/1.4 var(--font);
    -webkit-text-size-adjust: 100%; font-variant-numeric: tabular-nums; }
  main { max-width: 480px; margin: 0 auto; padding: 16px 16px 40px; }
  header { display: flex; justify-content: center; padding: 8px 0 16px; }
  header img { height: 22px; width: auto; display: block; }
  img { display: block; }
  .cover { width: 100%; aspect-ratio: 9 / 4; border-radius: 16px; object-fit: cover; background: var(--cell); }
  .id { display: flex; align-items: center; gap: 16px; margin: 20px 0 16px; }
  .crest { flex: none; width: 56px; height: 56px; border-radius: 6px; object-fit: cover; display: flex; align-items: center;
    justify-content: center; font-weight: 700; font-size: 20px; color: #FFFFFF; background: var(--fill); }
  .face { flex: none; border-radius: 50%; object-fit: cover; display: flex; align-items: center; justify-content: center;
    font-weight: 600; color: #FFFFFF; background: var(--fill); }
  h1 { margin: 0; font-size: 28px; line-height: 1.2; font-weight: 700; overflow-wrap: anywhere; }
  .sub { margin: 0; color: var(--secondary); font-size: 15px; }
  .by { display: flex; align-items: center; gap: 12px; margin: 0 0 16px; font-size: 17px; font-weight: 500; }
  .by .face { width: 32px; height: 32px; font-size: 15px; }
  .by span { color: var(--label); font-weight: 400; }
  .by b { font-weight: 600; }
  .members { display: flex; align-items: center; gap: 12px; margin: 0 0 20px; }
  .stack { display: flex; flex: none; }
  .stack .face { width: 32px; height: 32px; font-size: 15px; border: 2px solid var(--canvas); margin-left: -8px; }
  .stack .face:first-child { margin-left: 0; }
  .members .sub { overflow-wrap: anywhere; }
  .card { background: var(--cell); border-radius: 16px; padding: 16px; margin: 0 0 24px; }
  .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .stats dt { color: var(--secondary); font-size: 15px; margin: 0 0 4px; }
  .stats dd { margin: 0; font-size: 20px; font-weight: 600; }
  .gain { color: var(--gain); }
  .loss { color: var(--loss); }
  h2 { margin: 0 0 12px; font-size: 20px; line-height: 1.25; font-weight: 700; }
  .prop { display: flex; align-items: center; gap: 12px; }
  .logo { flex: none; width: 44px; height: 44px; border-radius: 8px; object-fit: cover; background: var(--fill); }
  .prop b { display: block; font-size: 17px; font-weight: 600; overflow-wrap: anywhere; }
  .prop .sub { overflow-wrap: anywhere; }
  .bar { display: flex; gap: 8px; height: 6px; margin: 16px 0; }
  .bar i { display: block; height: 100%; border-radius: 3px; background: #8E8E93; }
  .bar i.n { background: #636366; }
  .bar.empty { background: var(--fill); border-radius: 3px; }
  .ends { display: flex; align-items: center; justify-content: space-between; margin: 0 0 16px; font-size: 20px; font-weight: 600; }
  .ends .mid { color: var(--secondary); font-size: 17px; font-weight: 400; }
  .btn { display: flex; align-items: center; justify-content: center; min-height: 52px; margin: 0 0 12px; padding: 0 16px;
    border-radius: 14px; font: 600 17px/1.2 var(--font); text-decoration: none; text-align: center; overflow-wrap: anywhere; }
  .primary { background: var(--brand); color: #FFFFFF; }
  .ghost { color: var(--secondary); min-height: 44px; font-weight: 500; }
  .note { margin: 0 0 8px; text-align: center; color: var(--secondary); font-size: 15px; }
`;

const text = (v) => (typeof v === "string" ? v.trim() : "");
const count = (v) => (Number.isInteger(v) && v >= 0 ? v : null);

function hueFor(seed) {
  let h = 0;
  for (const ch of String(seed)) h = (h * 31 + ch.codePointAt(0)) >>> 0;
  return HUES[h % HUES.length];
}

function initials(name) {
  const words = text(name).split(/\s+/).filter(Boolean);
  const first = (w) => Array.from(w)[0] || "";
  return (words.length > 1 ? first(words[0]) + first(words[1]) : first(words[0] || "")).toUpperCase();
}

// "Aug 2026" from "2026-08-14", read as the UTC day the API sends.
function monthYear(since) {
  const m = /^(\d{4})-(\d{2})-/.exec(since);
  const month = m ? MONTHS[Number(m[2]) - 1] : null;
  return month ? `${month} ${m[1]}` : "";
}

// USDC micros as dollars with thousands separators, "$20,000". The treasury rounds to whole
// dollars; a proposal amount keeps its cents when it has any.
function dollars(micros, { cents = false } = {}) {
  if (typeof micros !== "string" || !/^[0-9]+$/.test(micros)) return "\u2014";
  const total = BigInt(micros);
  const hundredths = cents ? (total + 5000n) / 10000n : (total + 500000n) / 1000000n * 100n;
  const grouped = (hundredths / 100n).toString().replace(/\B(?=(\d{3})+$)/g, ",");
  const rest = hundredths % 100n;
  return rest === 0n ? `$${grouped}` : `$${grouped}.${rest.toString().padStart(2, "0")}`;
}

function signedPercent(bps) {
  if (!Number.isInteger(bps)) return { label: "—", tone: "" };
  const tenths = Math.round(Math.abs(bps) / 10) / 10;
  const body = `${tenths.toFixed(1)}%`;
  if (bps > 0 && tenths > 0) return { label: `+${body}`, tone: "gain" };
  if (bps < 0 && tenths > 0) return { label: `${MINUS}${body}`, tone: "loss" };
  return { label: body, tone: "" };
}

// "4h", "35m", "2d": the time left at render, never negative.
function relative(expiresAt, now) {
  const end = Date.parse(expiresAt);
  if (!Number.isFinite(end)) return "";
  const ms = end - now;
  if (ms < 60000) return "soon";
  const minutes = Math.floor(ms / 60000);
  if (minutes < 60) return `in ${minutes}m`;
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? `in ${hours}h` : `in ${Math.floor(hours / 24)}d`;
}

// The cabal's asset symbol as people say it: NVDAx is shown as NVDA.
function shown(symbol) {
  return text(symbol).replace(/(?<=[A-Z0-9])x$/, "");
}

function face(person, size) {
  const name = text(person && person.display_name);
  const photo = person && isHttpUrl(person.photo_url) ? person.photo_url : null;
  if (photo) return `<img class="face" src="${escapeHtml(photo)}" alt="" width="${size}" height="${size}">`;
  return `<span class="face" style="background:${hueFor(name)}" aria-hidden="true">${escapeHtml(initials(name))}</span>`;
}

function names(first, memberCount) {
  const shownNames = first.slice(0, 3).map((p) => text(p.display_name)).filter(Boolean);
  if (!shownNames.length) return "";
  const more = memberCount - 1 - shownNames.length;
  const head = shownNames.length > 1 && more <= 0
    ? `${shownNames.slice(0, -1).join(", ")} and ${shownNames[shownNames.length - 1]}`
    : shownNames.join(", ");
  return more > 0 ? `${head} and ${more} more` : head;
}

function proposalCard(p, now) {
  if (!p || typeof p !== "object") return "";
  const verb = p.kind === "buy" ? "Buy" : p.kind === "sell" ? "Sell" : "";
  const yes = count(p.yes);
  const no = count(p.no);
  if (!verb || yes === null || no === null) return "";
  const amount = p.kind === "buy" ? dollars(p.usdc_micros, { cents: true }) : "";
  const heading = [`${verb} ${shown(p.symbol)}`.trim(), amount].filter(Boolean).join(" · ");
  const logo = isHttpUrl(p.asset_logo_url)
    ? `<img class="logo" src="${escapeHtml(p.asset_logo_url)}" alt="" width="44" height="44">`
    : `<span class="logo" aria-hidden="true"></span>`;
  const total = yes + no;
  const bar = total
    ? `<div class="bar" aria-hidden="true"><i class="y" style="flex:${yes}"></i><i class="n" style="flex:${no}"></i></div>`
    : `<div class="bar empty" aria-hidden="true"></div>`;
  const voted = count(p.voted);
  const eligible = count(p.eligible);
  const replies = count(p.replies);
  const left = relative(p.expires_at, now);
  const facts = [
    voted !== null && eligible !== null ? `${voted} of ${eligible} voted` : "",
    replies !== null ? `${replies} ${replies === 1 ? "reply" : "replies"}` : "",
    left ? (left === "soon" ? "ends soon" : `ends ${left}`) : "",
  ].filter(Boolean).join(" · ");
  const who = [text(p.proposer_name), text(p.title)].filter(Boolean).join(" · ");
  return `<h2>Open now</h2>
    <section class="card">
      <div class="prop">${logo}<div><b>${escapeHtml(heading)}</b><p class="sub">${escapeHtml(who)}</p></div></div>
      ${bar}
      <div class="ends"><span>✓ ${yes}</span><span class="mid">Join to vote</span><span>${no} ✕</span></div>
      ${facts ? `<p class="sub">${escapeHtml(facts)}</p>` : ""}
    </section>`;
}

// Returns {status, headers, body}. referrer is the preview's inviter ({display_name, photo_url}),
// cabal the preview's cabal with its month facts beside it (what lookupCabal resolves to).
// proposalId is a valid ?p= or null; it only rides along in the links. now is the clock the
// relative times read.
export function renderCabalLanding({ code, cabalId, proposalId = null, referrer, cabal, appStoreUrl, openHost, apiUrl = "", posthogKey = "", now = Date.now() }) {
  const store = isHttpUrl(appStoreUrl) ? appStoreUrl : "/";
  const query = `?c=${cabalId}${proposalId ? `&p=${proposalId}` : ""}`;
  const link = `${SITE}/r/${code}${query}`;
  const openLink = `https://${openHost || WWW}/r/${code}${query}`;
  const name = text(cabal.name);
  const inviter = text(referrer && referrer.display_name);
  const cover = isHttpUrl(cabal.picture_url) ? cabal.picture_url : null;
  const joinLabel = cabal.join_mode === "request" ? `Ask to join ${name}` : `Join ${name}`;
  const title = inviter ? `${inviter} invited you to ${name}` : `You're invited to ${name}`;
  const description = `${cabal.member_count} members on Monaco`;
  const image = cover || DEFAULT_IMAGE;
  const members = Array.isArray(cabal.first_members) ? cabal.first_members.filter((p) => p && typeof p === "object").slice(0, 4) : [];
  const memberLine = names(members, cabal.member_count);
  const oneM = signedPercent(cabal.return_1m_bps);
  const turnout = Number.isInteger(cabal.turnout_month_bps) ? `${Math.round(cabal.turnout_month_bps / 100)}%` : "—";
  const since = monthYear(cabal.since);

  const meta = [
    `<meta property="og:type" content="website">`,
    `<meta property="og:url" content="${escapeHtml(link)}">`,
    `<meta property="og:title" content="${escapeHtml(title)}">`,
    `<meta property="og:description" content="${escapeHtml(description)}">`,
    `<meta property="og:image" content="${escapeHtml(image)}">`,
    `<meta name="twitter:card" content="${cover ? "summary_large_image" : "summary"}">`,
    `<meta name="twitter:title" content="${escapeHtml(title)}">`,
    `<meta name="twitter:image" content="${escapeHtml(image)}">`,
  ];

  const crest = cover
    ? `<img class="crest" src="${escapeHtml(cover)}" alt="" width="56" height="56">`
    : `<span class="crest" style="background:${hueFor(cabalId)}" aria-hidden="true">${escapeHtml(initials(name))}</span>`;
  const coverBlock = cover
    ? `<img class="cover" src="${escapeHtml(cover)}" alt="">`
    : `<div class="cover" aria-hidden="true"></div>`;
  const memberWord = cabal.member_count === 1 ? "member" : "members";
  const identity = ["Cabal", `${cabal.member_count} ${memberWord}`, since ? `since ${since}` : ""].filter(Boolean).join(" · ");
  const by = inviter
    ? `<p class="by">${face(referrer, 32)}<span><b>${escapeHtml(inviter)}</b> invited you</span></p>`
    : "";
  const faces = members.length
    ? `<div class="members"><div class="stack">${members.map((p) => face(p, 32)).join("")}</div>${memberLine ? `<p class="sub">${escapeHtml(memberLine)}</p>` : ""}</div>`
    : "";

  const body = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="dark">
<title>${escapeHtml(title)}</title>
<meta name="description" content="${escapeHtml(description)}">
<link rel="icon" href="/assets/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/assets/apple-touch-icon.png">
${meta.join("\n")}
<style>${STYLE}</style>
</head>
<body>
<main>
    <header><img src="/assets/wordmark.svg" alt="Monaco" width="125" height="22"></header>
    ${coverBlock}
    <div class="id">${crest}<div><h1>${escapeHtml(name)}</h1><p class="sub">${escapeHtml(identity)}</p></div></div>
    ${by}
    ${faces}
    <dl class="card stats">
      <div><dt>Treasury</dt><dd>${escapeHtml(dollars(cabal.pot_value_micros))}</dd></div>
      <div><dt>1M</dt><dd class="${oneM.tone}">${escapeHtml(oneM.label)}</dd></div>
      <div><dt>Turnout</dt><dd>${escapeHtml(turnout)}</dd></div>
    </dl>
    ${proposalCard(cabal.open_proposal, now)}
    <a class="btn primary" id="get-monaco" href="${escapeHtml(store)}">${escapeHtml(joinLabel)}</a>
    <p class="note">Get Monaco, then open this link again or tap Paste invite.</p>
    <a class="btn ghost" href="${escapeHtml(openLink)}">Open in Monaco</a>
</main>
<script>${script({ code, apiUrl, appStoreUrl: store, posthogKey, link })}</script>
</body>
</html>
`;
  return {
    status: 200,
    headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": CACHE_CONTROL },
    body,
  };
}
