#!/usr/bin/env python3
"""Generate apps/mobile/Monaco/Features/Reactions/emoji.json for the emoji picker.

Reads Unicode 15.1 emoji-test.txt and the CLDR 45 English annotations (both under the Unicode
License v3), downloaded at run time and never vendored. Keeps fully-qualified emoji without skin
tones and writes [{"c": emoji, "n": CLDR name, "k": [keywords], "g": group}] in picker order.

Run once and commit the output:  python3 scripts/gen-emoji.py
"""

from __future__ import annotations

import json
import sys
import urllib.request
from pathlib import Path

EMOJI_TEST = "https://unicode.org/Public/emoji/15.1/emoji-test.txt"
CLDR = "https://raw.githubusercontent.com/unicode-org/cldr-json/45.0.0/cldr-json"
ANNOTATIONS = [
    f"{CLDR}/cldr-annotations-full/annotations/en/annotations.json",
    f"{CLDR}/cldr-annotations-derived-full/annotationsDerived/en/annotations.json",
]
OUTPUT = Path(__file__).resolve().parent.parent / "apps/mobile/Monaco/Features/Reactions/emoji.json"
MAX_BYTES = 300 * 1024

GROUPS = {
    "Smileys & Emotion": "smileys",
    "People & Body": "people",
    "Animals & Nature": "nature",
    "Food & Drink": "food",
    "Activities": "activities",
    "Travel & Places": "travel",
    "Objects": "objects",
    "Symbols": "symbols",
    "Flags": "flags",
}
SKIN_TONES = range(0x1F3FB, 0x1F400)
VARIATION_SELECTOR = "️"


def fetch(url: str) -> str:
    with urllib.request.urlopen(url, timeout=60) as response:
        return response.read().decode("utf-8")


def annotations() -> dict[str, dict[str, list[str]]]:
    merged: dict[str, dict[str, list[str]]] = {}
    for url in ANNOTATIONS:
        document = json.loads(fetch(url))
        merged.update(next(iter(document.values()))["annotations"])
    return merged


def lookup(table: dict[str, dict[str, list[str]]], emoji: str) -> dict[str, list[str]] | None:
    for key in (emoji, emoji.replace(VARIATION_SELECTOR, ""), emoji + VARIATION_SELECTOR):
        if key in table:
            return table[key]
    return None


def parse(text: str) -> list[tuple[str, str, str]]:
    rows = []
    group = ""
    for line in text.splitlines():
        if line.startswith("# group:"):
            group = line.split(":", 1)[1].strip()
            continue
        if not line or line.startswith("#"):
            continue
        fields, _, comment = line.partition("#")
        codes, _, status = fields.partition(";")
        if status.strip() != "fully-qualified" or group not in GROUPS:
            continue
        scalars = [int(code, 16) for code in codes.split()]
        if any(scalar in SKIN_TONES for scalar in scalars):
            continue
        emoji = "".join(chr(scalar) for scalar in scalars)
        name = comment.split(" ", 3)[-1].strip() if comment else ""
        rows.append((emoji, name, GROUPS[group]))
    return rows


def build() -> list[dict]:
    table = annotations()
    entries = []
    for emoji, fallback, group in parse(fetch(EMOJI_TEST)):
        found = lookup(table, emoji)
        name = found["tts"][0] if found and found.get("tts") else fallback
        keywords = [word for word in (found or {}).get("default", []) if word != name]
        entries.append({"c": emoji, "n": name, "k": keywords, "g": group})
    return entries


def main() -> int:
    entries = build()
    data = json.dumps(entries, ensure_ascii=False, separators=(",", ":")) + "\n"
    if len(data.encode("utf-8")) >= MAX_BYTES:
        print(f"emoji.json is {len(data.encode('utf-8'))} bytes, over {MAX_BYTES}", file=sys.stderr)
        return 1
    OUTPUT.write_text(data, encoding="utf-8")
    print(f"wrote {len(entries)} emoji, {len(data.encode('utf-8'))} bytes to {OUTPUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
