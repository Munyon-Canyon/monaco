// Cloudflare Pages Functions adapter. See lib/referral.js.
import { handleReferral } from "../../lib/referral.js";

// The lookups are rate limited per IP, so go through Cloudflare's cache: the referrer lookup
// sends max-age=300 and every visitor would otherwise share this function's bucket. A cabal
// lookup carries a live tally, so it is kept for 60 s only. Errors are not cached.
function edgeCachedFetch(url, init) {
  const ttl = new URL(url).pathname.includes("/cabals/") ? 60 : 300;
  return globalThis.fetch(url, {
    ...init,
    cf: { cacheEverything: true, cacheTtlByStatus: { "200-299": ttl, "300-599": 0 } },
  });
}

export async function onRequest({ request, params, env }) {
  const url = new URL(request.url);
  const { status, headers, body } = await handleReferral({
    code: Array.isArray(params.code) ? params.code[0] : params.code,
    cabalId: url.searchParams.get("c"),
    proposalId: url.searchParams.get("p"),
    host: url.hostname,
    env,
    fetch: edgeCachedFetch,
  });
  return new Response(body, { status, headers });
}
