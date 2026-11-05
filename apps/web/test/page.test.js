// Static checks on index.html that need no browser.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const html = readFileSync(new URL("../index.html", import.meta.url), "utf8");
const meta = (key) => html.match(new RegExp(`<meta (?:property|name)="${key}" content="([^"]*)"`))?.[1];

test("the share links name the live domain, and the image they name is in the build", () => {
  assert.equal(meta("og:url"), "https://monacolabs.xyz/");
  assert.equal(meta("og:image"), "https://monacolabs.xyz/assets/og.png");
  assert.equal(meta("twitter:image"), "https://monacolabs.xyz/assets/og.png");
  assert.ok(existsSync(new URL("../public/assets/og.png", import.meta.url)));
});

test("no meta tag still names the lapsed domain", () => {
  assert.doesNotMatch(html, /<meta[^>]*trymonaco\.xyz/);
});

test("the waitlist asks only for an email and an optional Twitter handle", () => {
  const form = html.match(/<form data-waitlist[\s\S]*?<\/form>/)[0];
  const fields = [...form.matchAll(/<input name="([a-z]+)"/g)].map((m) => m[1]);
  assert.deepEqual(fields, ["email", "twitter", "company"]); // company is the honeypot
  assert.match(html, /JSON\.stringify\(\{ email: email\.value\.trim\(\), twitter: handle\(tw\.value\), company: trap\.value, source:/);
});
