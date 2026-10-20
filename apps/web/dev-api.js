// Dev only. `npx vite` serves static files, so /api/* would 404 on a laptop. This mounts the same
// Vercel handlers that api/ ships, so a sign-up in the dev server runs the code production runs.

import harbor from "./api/harbor.js";
import health from "./api/health.js";
import waitlist from "./api/waitlist.js";

const routes = { "/api/harbor": harbor, "/api/health": health, "/api/waitlist": waitlist };

// Vercel hands a handler a parsed body and a res with .status() and .json(); Vite's connect gives plain Node objects.
async function run(handler, req, res) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  req.body = Buffer.concat(chunks).toString();
  res.status = (code) => { res.statusCode = code; return res; };
  res.json = (body) => { res.setHeader("content-type", "application/json"); res.end(JSON.stringify(body)); };
  await handler(req, res);
}

// env is Vite's loadEnv(mode, dir, ""): the .env files plus the process environment.
export function waitlistDevApi(env) {
  return {
    name: "waitlist-dev-api",
    apply: "serve",
    configureServer(server) {
      // The handlers read process.env, as they do on Vercel. A variable already set wins over .env.local.
      for (const [key, value] of Object.entries(env)) process.env[key] ??= value;
      for (const [path, handler] of Object.entries(routes)) {
        server.middlewares.use(path, (req, res, next) => run(handler, req, res).catch(next));
      }
    },
  };
}
