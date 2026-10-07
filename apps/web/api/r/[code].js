import { handleReferral } from "../../lib/referral.js";

// Vercel's CDN keeps the page for five minutes, so the rate-limited lookup runs about once per
// code per window rather than once per visitor. Errors are not cached: handleReferral marks
// them no-store, and the CDN header is only set on a cacheable answer.
export default async function handler(req, res) {
  const code = Array.isArray(req.query.code) ? req.query.code[0] : req.query.code;
  const host = String(req.headers["x-forwarded-host"] || req.headers.host || "").split(":")[0];
  const { status, headers, body } = await handleReferral({ code, host, env: process.env, fetch: globalThis.fetch });
  for (const [name, value] of Object.entries(headers)) res.setHeader(name, value);
  if (headers["Cache-Control"] !== "no-store") res.setHeader("CDN-Cache-Control", "max-age=300");
  res.status(status).send(body);
}
