// Cloudflare Pages Functions adapter. See lib/referral.js.
import { handleReferral } from "../../lib/referral.js";

// The lookup is rate limited per IP, so go through Cloudflare's cache: the API sends
// max-age=300 and every visitor would otherwise share this function's bucket. Errors are not
// cached.
function edgeCachedFetch(url, init) {
  return globalThis.fetch(url, {
    ...init,
    cf: { cacheEverything: true, cacheTtlByStatus: { "200-299": 300, "300-599": 0 } },
  });
}

export async function onRequest({ request, params, env }) {
  const { status, headers, body } = await handleReferral({
    code: Array.isArray(params.code) ? params.code[0] : params.code,
    host: new URL(request.url).hostname,
    env,
    fetch: edgeCachedFetch,
  });
  return new Response(body, { status, headers });
}
