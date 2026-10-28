import { handleReferral } from "../../lib/referral.js";

// Vercel's CDN keeps a referrer page for five minutes and a cabal page for one, so the
// rate-limited lookups run about once per code (and cabal) per window rather than once per
// visitor. Errors are not cached: handleReferral marks them no-store without a cdnMaxAge, and the
// CDN header is only set on a cacheable answer.
const first = (value) => (Array.isArray(value) ? value[0] : value);

export default async function handler(req, res) {
  const host = String(req.headers["x-forwarded-host"] || req.headers.host || "").split(":")[0];
  const { status, headers, body, cdnMaxAge } = await handleReferral({
    code: first(req.query.code),
    cabalId: first(req.query.c),
    proposalId: first(req.query.p),
    host,
    env: process.env,
    fetch: globalThis.fetch,
  });
  for (const [name, value] of Object.entries(headers)) res.setHeader(name, value);
  if (cdnMaxAge) res.setHeader("CDN-Cache-Control", `max-age=${cdnMaxAge}`);
  res.status(status).send(body);
}
