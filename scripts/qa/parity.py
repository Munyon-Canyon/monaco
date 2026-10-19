#!/usr/bin/env python3
"""Pair the app's v3 screenshots with the v3 reference images and lay them side by side.

  parity.py defaults [--regex] [--refs DIR]
      The 43 default-state names, one per line, or one anchored alternation for
      MONACO_QA_SCREEN_ONLY. A reference with a __dark.jpg sibling is a default state.
  parity.py sheet --shots DIR --refs DIR --out DIR [--png]
      Write index.html, one <group>.html per group and missing.txt. With --png also write
      <group>-<appearance>.png sheets (needs Pillow). Exits 1 when a default state has no light
      or no dark shot.

Shots live at <shots>/light/<name>.png and <shots>/dark/<name>.png. A name is
<group>__<template>__<state>, the reference file name without __<appearance>.jpg. There is no
pixel diff on purpose: the 15pt text floor and the system chrome make every pixel differ.
"""

import argparse
import html
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_REFS = ROOT / "docs" / "design" / "v3" / "screens"
APPEARANCES = ("light", "dark")
GROUPS = ("access", "home", "money", "proposals", "cabals", "markets", "feed", "explore", "profile")

TICKETS = {
    "signin": "#5558", "verify": "#5558",
    "home": "#5559",
    "balances": "#5560", "notifications": "#5560",
    "add-money": "#5561", "card-funding": "#5561", "cash-out": "#5561", "deposit": "#5561",
    "fund-cabal": "#5561", "network": "#5561", "withdraw": "#5561",
    "activity": "#5562", "group-cash": "#5562", "transaction": "#5562",
    "propose": "#5563", "review-proposal": "#5563", "select-stock": "#5563", "trade-amount": "#5563",
    "proposal": "#5564", "proposals": "#5564", "votes": "#5564",
    "cabals": "#5565", "create-cabal": "#5565", "discover": "#5565",
    "cabal": "#5566", "cabal-info": "#5566", "invite": "#5566", "members": "#5566",
    "public-cabal": "#5566",
    "chat": "#5567", "holding": "#5567",
    "instrument": "#5568", "markets": "#5568", "watchlist": "#5568",
    "feed": "#5569", "post": "#5569",
    "leaderboard": "#5570",
    "edit-profile": "#5571", "people": "#5571", "person": "#5571", "profile": "#5571",
    "settings": "#5571",
}

TILE_WIDTH = 300
LABEL_STRIP = 22


class Pair:
    def __init__(self, name, appearance, ref):
        self.name = name
        self.group, self.template, self.state = name.split("__")
        self.appearance = appearance
        self.ref = ref
        self.default = False
        self.shot = None

    @property
    def label(self):
        return f"{self.template} / {self.state} / {self.appearance}"

    @property
    def ticket(self):
        return TICKETS.get(self.template, "unowned")


def split_ref(filename):
    """(name, appearance) of a reference file name, or None when it is not one."""
    for appearance in APPEARANCES:
        suffix = f"__{appearance}.jpg"
        if filename.endswith(suffix):
            name = filename[: -len(suffix)]
            if name.count("__") == 2:
                return name, appearance
    return None


def references(refs):
    """{appearance: {name: path}} for every reference image in refs."""
    found = {a: {} for a in APPEARANCES}
    for path in sorted(Path(refs).iterdir()):
        split = split_ref(path.name)
        if split:
            found[split[1]][split[0]] = path
    return found


def default_names(refs):
    found = references(refs)
    return sorted(found["dark"])


def shots_in(shots, appearance):
    folder = Path(shots) / appearance
    if not folder.is_dir():
        return {}
    return {p.stem: p for p in sorted(folder.glob("*.png"))}


def sort_key(pair):
    group = GROUPS.index(pair.group) if pair.group in GROUPS else len(GROUPS)
    return (group, pair.template, pair.appearance != "light", pair.state)


def resolve(refs, shots):
    found = references(refs)
    defaults = set(found["dark"])
    light, dark = shots_in(shots, "light"), shots_in(shots, "dark")
    taken = {"light": light, "dark": dark}
    rows = []
    for appearance in APPEARANCES:
        for name, ref in found[appearance].items():
            pair = Pair(name, appearance, ref)
            pair.default = name in defaults
            pair.shot = taken[appearance].get(name)
            rows.append(pair)
    rows.sort(key=sort_key)
    stray = []
    for appearance in APPEARANCES:
        for name in taken[appearance]:
            if name not in found[appearance]:
                stray.append((appearance, name))
    return rows, stray


def missing_lines(rows, stray):
    lines = []
    for pair in rows:
        if pair.shot is None:
            lines.append(f"no shot\t{pair.appearance}\t{pair.name}")
    for appearance, name in stray:
        lines.append(f"no reference\t{appearance}\t{name}")
    return lines


def failing(rows):
    return [p for p in rows if p.default and p.shot is None]


def relative(target, start):
    return os.path.relpath(target, start).replace(os.sep, "/")


PAGE_CSS = """
body { font: 14px/1.4 -apple-system, system-ui, sans-serif; margin: 24px; color: #1c1b18; background: #fff; }
nav a { margin-right: 12px; }
.pair { margin: 0 0 32px; }
.pair h3 { margin: 0 0 6px; font-size: 14px; }
.pair h3 span { color: #6b6a64; font-weight: 400; }
.shots { display: flex; gap: 12px; align-items: flex-start; }
figure { margin: 0; width: %dpx; }
figure img { width: %dpx; display: block; border: 1px solid #ddd; }
figcaption { color: #6b6a64; font-size: 12px; }
.none { width: %dpx; padding: 48px 0; text-align: center; border: 1px dashed #c00; color: #c00; }
.differences { margin: 6px 0 0; min-height: 1.4em; }
.differences::before { content: "Differences"; font-weight: 600; }
""" % (TILE_WIDTH, TILE_WIDTH, TILE_WIDTH)


def page(title, body):
    return (
        "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"
        f"<title>{html.escape(title)}</title>\n<style>{PAGE_CSS}</style>\n</head>\n<body>\n"
        f"{body}</body>\n</html>\n"
    )


def nav():
    links = ['<a href="index.html">index</a>'] + [
        f'<a href="{g}.html">{g}</a>' for g in GROUPS
    ]
    return "<nav>" + "".join(links) + "</nav>\n"


def pair_html(pair, out):
    ref = relative(pair.ref, out)
    if pair.shot is None:
        shot = '<div class="none">no shot</div>'
    else:
        shot = (
            f'<figure><img src="{html.escape(relative(pair.shot, out))}" alt="app {html.escape(pair.label)}">'
            "<figcaption>app</figcaption></figure>"
        )
    return (
        f'<section class="pair" id="{html.escape(pair.name)}-{pair.appearance}">\n'
        f"<h3>{html.escape(pair.label)} <span>{html.escape(pair.ticket)}</span></h3>\n"
        '<div class="shots">'
        f'<figure><img src="{html.escape(ref)}" alt="reference {html.escape(pair.label)}">'
        "<figcaption>reference</figcaption></figure>"
        f"{shot}</div>\n"
        '<ul class="differences"></ul>\n</section>\n'
    )


def write_html(rows, out):
    out = Path(out)
    for group in GROUPS:
        members = [p for p in rows if p.group == group]
        body = nav() + f"<h1>{html.escape(group)}</h1>\n" + "".join(pair_html(p, out) for p in members)
        (out / f"{group}.html").write_text(page(f"v3 parity {group}", body))
    lines = []
    for group in GROUPS:
        members = [p for p in rows if p.group == group]
        shot = sum(1 for p in members if p.shot)
        lines.append(f'<li><a href="{group}.html">{html.escape(group)}</a> {shot} of {len(members)} shot</li>')
    body = nav() + "<h1>v3 parity</h1>\n<ul>\n" + "\n".join(lines) + "\n</ul>\n"
    (out / "index.html").write_text(page("v3 parity", body))


def write_png(rows, out):
    try:
        from PIL import Image, ImageDraw
    except ImportError:
        raise SystemExit("parity.py: --png needs Pillow; install it with `python3 -m pip install pillow`")
    out = Path(out)

    def tile(label, path):
        image = Image.open(path).convert("RGB")
        return label, image.resize((TILE_WIDTH, max(1, int(image.height * TILE_WIDTH / image.width))))

    for group in GROUPS:
        for appearance in APPEARANCES:
            members = [p for p in rows if p.group == group and p.appearance == appearance and p.shot]
            if not members:
                continue
            height = 0
            lines = []
            for p in members:
                ref = tile(f"{p.label} ref", p.ref)
                app = tile(f"{p.label} app", p.shot)
                row = max(ref[1].height, app[1].height) + LABEL_STRIP
                lines.append((ref, app, row))
                height += row
            sheet = Image.new("RGB", (2 * TILE_WIDTH, height), "white")
            draw = ImageDraw.Draw(sheet)
            y = 0
            for ref, app, row in lines:
                for column, (label, image) in enumerate((ref, app)):
                    draw.text((column * TILE_WIDTH + 4, y + 4), label, fill="black")
                    sheet.paste(image, (column * TILE_WIDTH, y + LABEL_STRIP))
                y += row
            sheet.save(out / f"{group}-{appearance}.png")


def command_defaults(args):
    names = default_names(args.refs)
    if args.regex:
        print("^(" + "|".join(names) + ")$")
    else:
        print("\n".join(names))
    return 0


def command_sheet(args):
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rows, stray = resolve(args.refs, args.shots)
    write_html(rows, out)
    lines = missing_lines(rows, stray)
    (out / "missing.txt").write_text("".join(line + "\n" for line in lines))
    if args.png:
        write_png(rows, out)
    bad = failing(rows)
    for p in bad:
        print(f"parity: default state {p.name} has no {p.appearance} shot", file=sys.stderr)
    print(f"parity: {sum(1 for p in rows if p.shot)} of {len(rows)} references paired, {len(lines)} lines in {out / 'missing.txt'}")
    return 1 if bad else 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    d = sub.add_parser("defaults", help="print the default-state names")
    d.add_argument("--regex", action="store_true", help="print the anchored alternation")
    d.add_argument("--refs", default=str(DEFAULT_REFS))
    d.set_defaults(run=command_defaults)
    s = sub.add_parser("sheet", help="build the side-by-side pages")
    s.add_argument("--shots", required=True)
    s.add_argument("--refs", default=str(DEFAULT_REFS))
    s.add_argument("--out", required=True)
    s.add_argument("--png", action="store_true", help="also write PNG sheets (needs Pillow)")
    s.set_defaults(run=command_sheet)
    args = parser.parse_args(argv)
    return args.run(args)


if __name__ == "__main__":
    sys.exit(main())
