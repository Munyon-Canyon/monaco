import { handleHarbor } from "../lib/waitlist.js";

export default async function handler(req, res) {
  const { status, headers, body } = await handleHarbor({
    method: req.method,
    headers: req.headers,
    env: process.env,
    fetch: globalThis.fetch,
  });
  for (const [name, value] of Object.entries(headers)) res.setHeader(name, value);
  res.status(status).json(body);
}
