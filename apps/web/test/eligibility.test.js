// The eligibility line: WAITLIST_ELIGIBILITY_LINE becomes one paragraph under the form, or nothing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { build, createServer } from "vite";
import { withEligibility } from "../eligibility.js";

const configFile = new URL("../vite.config.ts", import.meta.url).pathname;
const page = readFileSync(new URL("../index.html", import.meta.url), "utf8");
const MARKER = "<!--eligibility-->";
const LINE = "Open to US residents aged 18 and over.";

// vite reads the process environment as well as .env files, so setting the variable here, even to "", beats a .env.local.
const withLine = (value) => { process.env.WAITLIST_ELIGIBILITY_LINE = value; };

test("the page carries the marker once, under the status line at the foot of the form", () => {
  assert.equal(page.split(MARKER).length, 2);
  assert.match(page, /<p class="st mono"[^>]*><\/p>\s*<!--eligibility-->\s*<\/form>/);
});

test("the line steps aside while the cabal question is open", () => {
  assert.match(page, /\.bar\[data-step="2"\] ~ \.elig \{ display: none; \}/);
});

test("a line replaces the marker with one paragraph, trimmed", () => {
  const html = withEligibility(page, `  ${LINE}\n`);
  assert.ok(!html.includes(MARKER));
  assert.equal(html.split('<p class="elig mono">').length, 2);
  assert.ok(html.includes(`</p>\n      <p class="elig mono">${LINE}</p>\n    </form>`));
});

test("empty, blank or unset leaves neither a paragraph nor the marker, and the rest of the page alone", () => {
  for (const line of ["", "   ", "\n\t", undefined, null]) {
    const html = withEligibility(page, line);
    assert.ok(!html.includes(MARKER), JSON.stringify(line));
    assert.ok(!html.includes('class="elig mono"'), JSON.stringify(line));
    assert.equal(html, page.replace(MARKER, ""));
  }
});

test("the line is escaped, and a $ pattern in it is plain text", () => {
  const html = withEligibility(page, `<b onclick="x()">Fund's</b> & "more" $& $' $$ $1`);
  assert.ok(html.includes("<p class=\"elig mono\">&lt;b onclick=&quot;x()&quot;&gt;Fund&#39;s&lt;/b&gt; &amp; &quot;more&quot; $&amp; $&#39; $$ $1</p>"));
  assert.ok(!html.includes("<b onclick"));
});

test("the dev server writes the environment's line into the page, and nothing when it is empty", async () => {
  for (const [value, expected] of [[LINE, `<p class="elig mono">${LINE}</p>`], ["", null]]) {
    withLine(value);
    const vite = await createServer({ configFile, logLevel: "silent", server: { port: 0, host: "127.0.0.1" } });
    try {
      await vite.listen();
      const html = await (await fetch(`http://127.0.0.1:${vite.httpServer.address().port}/`)).text();
      assert.ok(!html.includes(MARKER));
      if (expected) assert.ok(html.includes(expected));
      else assert.ok(!html.includes('class="elig mono"'));
    } finally {
      await vite.close();
    }
  }
});

test("the production build does the same, and leaves the fund page alone", async () => {
  for (const [value, expected] of [[LINE, `<p class="elig mono">${LINE}</p>`], ["", null]]) {
    withLine(value);
    const out = await build({ configFile, logLevel: "silent", build: { write: false } });
    const files = Object.fromEntries((Array.isArray(out) ? out : [out]).flatMap((o) => o.output).map((f) => [f.fileName, f.source]));
    assert.ok(!files["index.html"].includes(MARKER));
    if (expected) assert.ok(files["index.html"].includes(expected));
    else assert.ok(!files["index.html"].includes('class="elig mono"'));
    assert.ok(!files["fund/index.html"].includes('class="elig mono"'));
  }
});
