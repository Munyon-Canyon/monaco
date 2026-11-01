import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const here = (p) => new URL(p, import.meta.url);
const vercel = JSON.parse(readFileSync(here("../vercel.json"), "utf8"));
const cloudflare = readFileSync(here("../public/_headers"), "utf8");

function vercelValues(path, key) {
  return vercel.headers
    .filter((rule) => new RegExp(`^${rule.source}$`).test(path))
    .flatMap((rule) => rule.headers.filter((h) => h.key === key).map((h) => h.value));
}

const fundPaths = ["/fund", "/fund/", "/fund/index.html", "/fund/x"];
const otherPaths = ["/", "/r/abc", "/funding", "/fundraiser/x", "/assets/mark-ink.svg", "/api/health"];

test("Vercel sends one Permissions-Policy on /fund and it lets Stripe's frames use the camera", () => {
  for (const path of fundPaths) {
    const values = vercelValues(path, "Permissions-Policy");
    assert.equal(values.length, 1, path);
    assert.match(values[0], /camera=\(self "https:\/\/\*\.stripe\.com"/, path);
    assert.match(values[0], /microphone=\(\)/, path);
  }
});

test("Vercel keeps the camera off everywhere else, once", () => {
  for (const path of otherPaths) {
    assert.deepEqual(vercelValues(path, "Permissions-Policy"), ["camera=(), microphone=(), geolocation=()"], path);
  }
});

test("the other security headers still apply on /fund", () => {
  for (const key of ["X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options", "Cache-Control", "Content-Security-Policy"]) {
    assert.equal(vercelValues("/fund", key).length, 1, key);
  }
});

test("Cloudflare detaches the global policy on /fund and sends the same one as Vercel", () => {
  const expected = vercelValues("/fund", "Permissions-Policy")[0];
  for (const block of ["/fund", "/fund/*"]) {
    const body = cloudflare.split(/\n\n/).find((b) => b.split("\n").some((line) => line === block));
    assert.ok(body, block);
    const lines = body.split("\n").map((l) => l.trim());
    assert.ok(lines.indexOf("! Permissions-Policy") >= 0, block);
    assert.ok(lines.indexOf("! Permissions-Policy") < lines.indexOf(`Permissions-Policy: ${expected}`), block);
  }
  assert.match(cloudflare, /^\/\*\n(?:  .*\n)*  Permissions-Policy: camera=\(\), microphone=\(\), geolocation=\(\)$/m);
});
