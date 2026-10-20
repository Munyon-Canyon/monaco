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
