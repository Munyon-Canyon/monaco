// Cloudflare Pages Functions adapter. See functions/api/waitlist.js.
import { handleHarbor } from "../../lib/waitlist.js";

export async function onRequest({ request, env }) {
  const { status, headers, body } = await handleHarbor({
    method: request.method,
    headers: Object.fromEntries(request.headers),
    env,
    fetch: globalThis.fetch,
  });
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
}
