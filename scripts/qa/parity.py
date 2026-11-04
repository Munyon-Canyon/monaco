#!/usr/bin/env python3
"""Pair the app's screenshots with F's launch frames and lay them side by side.

  parity.py frames [--design f|v5] [--appearance dark|light] [--kind screen|board|system] [--regex]
      The f__<frame> names of the launch frames, one per line, or one anchored alternation for
      MONACO_QA_SCREEN_ONLY.
  parity.py lines [--design f|v5] [--manifest FILE] [--strict]
      Read the f__ lines of scripts/qa/sample-screens.txt (the  block) and print each
      screen or board frame that has no line, with its owner. Exits 1 on an f__ line whose frame
      is not a launch frame, or on a line for a system frame; with --strict also when any screen
      or board frame has no line.
  parity.py sheet [--design f|v5] --shots DIR --out DIR [--refs DIR] [--png] [--allow-missing FRAME ...]
      Write index.html, one <group>.html per two-digit frame prefix and missing.txt. With --png
      also write <group>.png sheets (needs Pillow). Exits 1 when a screen or board frame has no
      shot (unless it is passed to --allow-missing) or when a reference PNG is in neither FRAMES
      nor LATER ("unlisted"). A system frame with no shot is listed but does not fail.
  parity.py sheet [--design v5] FRAME --before PNG --after PNG [--out FILE] [--refs DIR]
      Write one PNG of three columns, frame | before | after, at one common height
      (default FILE: <FRAME>-sheet.png). Needs Pillow. FRAME must be in the design's table.

--design picks the table: f (default, docs/design/f/screens, shots f__<frame>) or v5
(docs/design/v5/screens, every frame dark). Both share the f__<frame> harness lines of the manifest,
so lines accepts an f__ line for a frame of either table.

FRAMES are the 104 launch frames of docs/design/f/screens, in the order of #6583's Reference
table. A shot is named f__<frame>, the manifest line name, and lives at
<shots>/dark/f__<frame>.png or <shots>/light/f__<frame>.png by the frame's appearance. The
reference is <refs>/<frame>.png. There is no pixel diff on purpose: the 15pt text floor (founder
decision Q1) and the system chrome make every pixel differ.
"""

import argparse
import html
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_REFS = ROOT / "docs" / "design" / "f" / "screens"
DEFAULT_MANIFEST = ROOT / "scripts" / "qa" / "sample-screens.txt"
APPEARANCES = ("light", "dark")
KINDS = ("screen", "board", "system")
PREFIX = "f__"
V5_REFS = ROOT / "docs" / "design" / "v5" / "screens"

FRAMES = [
    ("01-tokens", "board", "#6432, #6440"),
    ("01-tokens-type", "board", "#6440"),
    ("01-tokens-controls", "board", "#6449, #6460, #6461, #6458, #6469, #6548"),
    ("01-chart-variants", "board", "#6450"),
    ("02-home-dark", "screen", "#6480, #6496"),
    ("02-home-light", "screen", "#6480, #6496"),
    ("02-home-scrolled", "screen", "#6480, #6496"),
    ("02-home-empty", "screen", "#6564"),
    ("04b-hold-1-pick", "screen", "#6508"),
    ("04b-hold-2-holding", "screen", "#6508"),
    ("04b-hold-3-committed", "screen", "#6508"),
    ("12-toast", "screen", "#6458, #6506"),
    ("03-cabals-tab", "screen", "#6522"),
    ("03-cabals-tab-menu", "screen", "#6560"),
    ("03-cabal-treasury", "screen", "#6481, #6497, #6511"),
    ("03-cabal-treasury-light", "screen", "#6481, #6497, #6511"),
    ("03-cabal-treasury-scrolled", "screen", "#6481, #6497, #6511"),
    ("03-cabal-rules", "screen", "#6523"),
    ("03-cabal-rules-scrolled", "screen", "#6523"),
    ("03-cabal-rules-sides", "screen", "#6523"),
    ("03-cabal-members", "screen", "#6534"),
    ("03-cabal-members-scrolled", "screen", "#6534"),
    ("03-cabal-members-all", "screen", "#6534"),
    ("03-cabal-empty", "screen", "#6524"),
    ("15-first-fund", "screen", "#6509"),
    ("04-proposal-dark", "screen", "#6491"),
    ("04-proposal-light", "screen", "#6491"),
    ("04-proposal-scrolled", "screen", "#6491"),
    ("04-vote-idle", "screen", "#6506"),
    ("04-vote-picked", "screen", "#6506"),
    ("04-vote-holding", "screen", "#6506"),
    ("04-vote-committed", "screen", "#6506"),
    ("19-permission-sheet", "screen", "#6554"),
    ("04-vote-closed", "screen", "#6532"),
    ("04-proposal-passed", "screen", "#6532, #6541"),
    ("15-passed", "screen", "#6545"),
    ("04-proposal-sides", "screen", "#6539"),
    ("04-proposal-failed", "screen", "#6532, #6541"),
    ("04-proposal-preipo", "screen", "#6550"),
    ("04-reply-composer", "screen", "#6548"),
    ("04-composer-menu", "screen", "#6563, #6568"),
    ("04-gif-sheet", "screen", "#6556, #6558"),
    ("04-gif-reply", "screen", "#6556, #6558"),
    ("14-hold-sheet", "screen", "#6557, #6559"),
    ("05-feed-A", "screen", "#6500, #6535, #6469, #6551"),
    ("05-feed-A-scrolled", "screen", "#6500, #6535"),
    ("05-feed-light", "screen", "#6500, #6535"),
    ("05-reactions", "screen", "#6469"),
    ("05-emoji-picker", "screen", "#6494"),
    ("05-post", "screen", "#6555, #6541"),
    ("05-post-scrolled", "screen", "#6555, #6541"),
    ("05-post-reply", "screen", "#6548, #6556, #6558"),
    ("05-comment-actions", "screen", "#6552"),
    ("05-post-public", "screen", "#6553, #6506"),
    ("05-feed-empty", "screen", "#6542"),
    ("06-markets-A", "screen", "#6487"),
    ("06-markets-A-scrolled", "screen", "#6487"),
    ("06-markets-search", "screen", "#6501"),
    ("06-markets-held", "screen", "#6516"),
    ("07-stock-dark", "screen", "#6528, #6536"),
    ("07-stock-light", "screen", "#6528, #6536"),
    ("07-stock-preipo", "screen", "#6544"),
    ("07-stock-preipo-scrolled", "screen", "#6544"),
    ("08-review-sheet", "screen", "#6567"),
    ("08-review-sheet-preipo", "screen", "#6567"),
    ("12-info-sheet", "screen", "#6562"),
    ("09-fund-cabal-empty", "screen", "#6475, #6490, #6459"),
    ("09-fund-cabal", "screen", "#6475, #6490, #6459"),
    ("09-fund-cabal-invalid", "screen", "#6475, #6490, #6459"),
    ("09-fund-cabal-settled", "screen", "#6475, #6490, #6459"),
    ("09-withdraw-empty", "screen", "#6476, #6504, #6459"),
    ("09-withdraw", "screen", "#6476, #6504, #6459"),
    ("09-withdraw-invalid", "screen", "#6476, #6504, #6459"),
    ("09-withdraw-settled", "screen", "#6476, #6504, #6459"),
    ("10-profile-own", "screen", "#6488, #6505"),
    ("10-profile-own-scrolled", "screen", "#6488, #6505"),
    ("10-profile-other", "screen", "#6530"),
    ("10-profile-other-cabals", "screen", "#6530"),
    ("11-invite-sheet", "screen", "#6489"),
    ("18-invite-landing", "screen", "#6502"),
    ("11-share-A", "screen", "#6503, #6519"),
    ("11-share-B", "screen", "#6531"),
    ("11-share-C", "screen", "#6537"),
    ("11-share-card-vote", "screen", "#6531"),
    ("11-share-sticker", "screen", "#6537"),
    ("11-share-sticker-on", "screen", "#6537"),
    ("13-leaderboard-C-people", "screen", "#6484"),
    ("13-leaderboard-C", "screen", "#6484, #6499"),
    ("13-leaderboard-C-scrolled", "screen", "#6484"),
    ("13-leaderboard-C-cabals", "screen", "#6514"),
    ("13-leaderboard-C-unranked", "screen", "#6514"),
    ("15-leaderboard-moment", "screen", "#6526"),
    ("14-lock-screen-launch", "system", "#6565, #6425"),
    ("19-notifications", "system", "#6565, #6425"),
    ("14-notification", "system", "#6565"),
    ("14-faceid-alt", "system", "#6565"),
    ("15-last-vote", "system", "#6565"),
    ("14-wrap-sheet", "screen", "#6570"),
    ("14-ledger-story", "screen", "#6581"),
    ("16-catch-up", "screen", "#6561"),
    ("16-catch-up-open", "screen", "#6571"),
    ("16-catch-up-open-scrolled", "screen", "#6571"),
    ("16-catch-up-empty", "screen", "#6561, #6566"),
    ("16-catch-up-empty-scrolled", "screen", "#6561, #6566"),
]

LIGHT = {
    "02-home-light",
    "03-cabal-treasury-light",
    "04-proposal-light",
    "05-feed-light",
    "07-stock-light",
}

V5_FRAMES = [
    ("00-kit", "board", "f__01-tokens"),
    ("02-home-dark", "screen", "f__02-home-dark"),
    ("02-home-empty", "screen", "f__02-home-empty"),
    ("03-cabals-tab", "screen", "f__03-cabals-tab"),
    ("03-cabals-tab-menu", "screen", "f__03-cabals-tab-menu"),
    ("03-cabals-tab-few", "screen", "f__03-cabals-tab-few"),
    ("03-cabal-chat", "screen", "chat-thread"),
    ("03-cabal-chat-few", "screen", "chat-empty"),
    ("03-cabal-treasury", "screen", "f__03-cabal-treasury"),
    ("03-cabal-members", "screen", "f__03-cabal-members"),
    ("03-cabal-rules", "screen", "f__03-cabal-rules"),
    ("04-proposal-dark", "screen", "f__04-proposal-dark"),
    ("04b-hold-2-holding", "screen", "f__04b-hold-2-holding"),
    ("04-reply-composer", "screen", "f__04-reply-composer"),
    ("04-gif-sheet", "screen", "f__04-gif-sheet"),
    ("05-feed-A", "screen", "f__05-feed-A"),
    ("05-post", "screen", "f__05-post"),
    ("05-emoji-picker", "screen", "f__05-emoji-picker"),
    ("06-markets-A", "screen", "f__06-markets-A"),
    ("06-markets-search", "screen", "f__06-markets-search"),
    ("07-stock-dark", "screen", "f__07-stock-dark"),
    ("08-review-sheet", "screen", "f__08-review-sheet"),
    ("09-fund-cabal", "screen", "f__09-fund-cabal"),
    ("09-withdraw", "screen", "f__09-withdraw"),
    ("10-profile-own", "screen", "f__10-profile-own"),
    ("10-profile-other", "screen", "f__10-profile-other"),
    ("11-share-A", "screen", "f__11-share-A"),
    ("11-invite-sheet", "screen", "f__11-invite-sheet"),
    ("12-info-sheet", "screen", "f__12-info-sheet"),
    ("12-toast", "screen", "f__12-toast"),
    ("19-permission-sheet", "screen", "f__19-permission-sheet"),
    ("13-leaderboard-C", "screen", "f__13-leaderboard-C"),
    ("15-first-fund", "screen", "f__15-first-fund"),
    ("16-catch-up", "screen", "f__16-catch-up"),
]

HARNESS_VARIANTS = {
    "12-info-sheet-content",
}

LATER = {
    "01-tokens-later",
    "04-proposal-follow",
    "10-season-sheet",
    "14-dynamic-island",
    "14-live-activity-compact",
    "14-live-activity-expanded",
    "14-live-activity-lock",
    "14-lock-screen",
    "14-spotlight",
    "14-standby",
    "14-widgets",
    "15-season-close",
    "15-season-close-scrolled",
    "15-streak",
    "17-season-story-1",
    "17-season-story-card",
}

GROUP_TITLES = {
    "01": "Tokens and boards",
    "02": "Home",
    "03": "Cabals",
    "04": "Proposal",
    "05": "Feed",
    "06": "Markets",
    "07": "Stock",
    "08": "Review",
    "09": "Money",
    "10": "Profile",
    "11": "Share and invite",
    "12": "Toast and info",
    "13": "Leaderboard",
    "14": "Notifications and Ledger",
    "15": "Moments",
    "16": "Catch up",
    "18": "Invite landing",
    "19": "Permission and pushes",
}
GROUPS = tuple(GROUP_TITLES)

TILE_WIDTH = 300
LABEL_STRIP = 22
SHEET_HEIGHT = 852
PILLOW_MESSAGE = "parity.py: --png needs Pillow; install it with `python3 -m pip install pillow`"


class Design:
    def __init__(self, name, label, frames, refs, prefix, light, titles):
        self.name = name
        self.label = label
        self.frames = frames
        self.refs = refs
        self.prefix = prefix
        self.light = light
        self.titles = titles

    @property
    def groups(self):
        return tuple(self.titles)


DESIGNS = {
    "f": Design("f", "F", FRAMES, DEFAULT_REFS, PREFIX, LIGHT, GROUP_TITLES),
    "v5": Design("v5", "v5", V5_FRAMES, V5_REFS, PREFIX, set(), {"00": "Kit", **GROUP_TITLES}),
}


def appearance_of(frame, design=DESIGNS["f"]):
    return "light" if frame in design.light else "dark"


def group_of(frame):
    return frame[:2]


class Pair:
    def __init__(self, frame, kind, owner, ref, shot, design=DESIGNS["f"]):
        self.frame = frame
        self.name = design.prefix + frame
        self.kind = kind
        self.owner = owner
        self.appearance = appearance_of(frame, design)
        self.group = group_of(frame)
        self.ref = ref
        self.shot = shot
        self.allowed = False

    @property
    def label(self):
        return f"{self.frame} / {self.appearance} / {self.owner}"


def referenced(refs):
    """{stem: path} of every PNG in refs, or {} when refs is not a folder."""
    folder = Path(refs)
    if not folder.is_dir():
        return {}
    return {p.stem: p for p in sorted(folder.glob("*.png"))}


def shot_path(shots, frame, design=DESIGNS["f"]):
    return Path(shots) / appearance_of(frame, design) / f"{design.prefix}{frame}.png"


def resolve(refs, shots, allowed=(), design=DESIGNS["f"]):
    found = referenced(refs)
    rows = []
    expected = {a: set() for a in APPEARANCES}
    for frame, kind, owner in design.frames:
        shot = shot_path(shots, frame, design)
        pair = Pair(frame, kind, owner, found.get(frame), shot if shot.is_file() else None, design)
        pair.allowed = frame in allowed
        rows.append(pair)
        expected[pair.appearance].add(f"{design.prefix}{frame}")
    unlisted = sorted(stem for stem in found if stem not in {f for f, _, _ in design.frames} and stem not in LATER)
    stray = []
    for appearance in APPEARANCES:
        folder = Path(shots) / appearance
        if not folder.is_dir():
            continue
        for path in sorted(folder.glob(f"{design.prefix}*.png")):
            if path.stem not in expected[appearance]:
                stray.append((appearance, path.stem))
    return rows, unlisted, stray


def missing_lines(rows, unlisted, stray):
    lines = []
    for pair in rows:
        if pair.shot is None:
            kind = "allowed missing" if pair.allowed else "no shot"
            lines.append(f"{kind}\t{pair.appearance}\t{pair.frame}")
        if pair.ref is None:
            lines.append(f"no reference\t{pair.appearance}\t{pair.frame}")
    for appearance, name in stray:
        lines.append(f"no reference\t{appearance}\t{name}")
    for stem in unlisted:
        lines.append(f"unlisted\t-\t{stem}")
    return lines


def failing(rows):
    return [p for p in rows if p.kind != "system" and p.shot is None and not p.allowed]


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


def nav(design=DESIGNS["f"]):
    links = ['<a href="index.html">index</a>'] + [
        f'<a href="{g}.html">{g} {html.escape(design.titles[g])}</a>' for g in design.groups
    ]
    return "<nav>" + "".join(links) + "</nav>\n"


def figure(path, alt, caption, out):
    if path is None:
        return f'<div class="none">no {caption}</div>'
    return (
        f'<figure><img src="{html.escape(relative(path, out))}" alt="{html.escape(alt)}">'
        f"<figcaption>{caption}</figcaption></figure>"
    )


def pair_html(pair, out):
    return (
        f'<section class="pair" id="{html.escape(pair.frame)}">\n'
        f"<h3>{html.escape(pair.label)}</h3>\n"
        '<div class="shots">'
        f"{figure(pair.ref, 'reference ' + pair.label, 'reference', out)}"
        f"{figure(pair.shot, 'app ' + pair.label, 'app', out)}</div>\n"
        '<ul class="differences"></ul>\n</section>\n'
    )


def write_html(rows, out, design=DESIGNS["f"]):
    out = Path(out)
    lines = []
    for group in design.groups:
        members = [p for p in rows if p.group == group]
        title = f"{group} {design.titles[group]}"
        body = nav(design) + f"<h1>{html.escape(title)}</h1>\n" + "".join(pair_html(p, out) for p in members)
        (out / f"{group}.html").write_text(page(f"{design.label} parity {title}", body))
        shot = sum(1 for p in members if p.shot)
        lines.append(f'<li><a href="{group}.html">{html.escape(title)}</a> {shot} of {len(members)} shot</li>')
    body = nav(design) + f"<h1>{design.label} parity</h1>\n<ul>\n" + "\n".join(lines) + "\n</ul>\n"
    (out / "index.html").write_text(page(f"{design.label} parity", body))


def write_png(rows, out, design=DESIGNS["f"]):
    try:
        from PIL import Image, ImageDraw
    except ImportError:
        raise SystemExit(PILLOW_MESSAGE)
    out = Path(out)

    def tile(label, path):
        image = Image.open(path).convert("RGB")
        return label, image.resize((TILE_WIDTH, max(1, int(image.height * TILE_WIDTH / image.width))))

    for group in design.groups:
        members = [p for p in rows if p.group == group and p.shot and p.ref]
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
        sheet.save(out / f"{group}.png")


def command_frames(args):
    design = DESIGNS[args.design]
    names = [
        design.prefix + frame
        for frame, kind, _ in design.frames
        if (args.appearance is None or appearance_of(frame, design) == args.appearance)
        and (args.kind is None or kind == args.kind)
    ]
    if args.regex:
        print("^(" + "|".join(names) + ")$")
    else:
        print("\n".join(names))
    return 0


def manifest_names(path):
    names = []
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            names.append(line.split()[0])
    return names


def command_lines(args):
    design = DESIGNS[args.design]
    prefix = design.prefix
    kinds = {frame: kind for frame, kind, _ in design.frames}
    other_frames = {frame for d in DESIGNS.values() if d is not design for frame, _, _ in d.frames}
    covered = set()
    bad = 0
    for name in manifest_names(args.manifest):
        if not name.startswith(prefix):
            continue
        frame = name[len(prefix):]
        if frame in HARNESS_VARIANTS or (frame not in kinds and frame in other_frames):
            continue
        if frame not in kinds:
            print(f"parity: {name} is not a launch frame", file=sys.stderr)
            bad += 1
        elif kinds[frame] == "system":
            print(f"parity: {name} is a system frame, a push on the simulator, not a harness line", file=sys.stderr)
            bad += 1
        else:
            covered.add(frame)
    uncovered = [(frame, owner) for frame, kind, owner in design.frames if kind != "system" and frame not in covered]
    for frame, owner in uncovered:
        print(f"{prefix}{frame}\t{owner}")
    if bad:
        return 1
    return 1 if args.strict and uncovered else 0


def command_frame_sheet(args, design):
    frames = {frame for frame, _, _ in design.frames}
    if args.frame not in frames:
        print(f"parity: {args.frame} is not a {design.name} frame", file=sys.stderr)
        return 2
    try:
        from PIL import Image, ImageDraw
    except ImportError:
        raise SystemExit(PILLOW_MESSAGE)
    refs = Path(args.refs) if args.refs else design.refs
    columns = []
    for label, path in (("frame", refs / f"{args.frame}.png"), ("before", args.before), ("after", args.after)):
        image = Image.open(path).convert("RGB")
        columns.append((label, image.resize((max(1, round(image.width * SHEET_HEIGHT / image.height)), SHEET_HEIGHT))))
    sheet = Image.new("RGB", (sum(i.width for _, i in columns), SHEET_HEIGHT + LABEL_STRIP), "white")
    draw = ImageDraw.Draw(sheet)
    x = 0
    for label, image in columns:
        draw.text((x + 4, 4), label, fill="black")
        sheet.paste(image, (x, LABEL_STRIP))
        x += image.width
    out = Path(args.out or f"{args.frame}-sheet.png")
    sheet.save(out)
    print(out)
    return 0


def command_sheet(args):
    design = DESIGNS[args.design]
    if args.frame:
        if not (args.before and args.after):
            args.parser.error("sheet FRAME needs --before and --after")
        return command_frame_sheet(args, design)
    if not (args.shots and args.out):
        args.parser.error("sheet needs --shots and --out (or a FRAME with --before and --after)")
    launch = {frame for frame, _, _ in design.frames}
    unknown = [name for name in args.allow_missing if name not in launch]
    if unknown:
        print(f"parity: --allow-missing takes launch frame names; unknown: {', '.join(unknown)}", file=sys.stderr)
        return 2
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rows, unlisted, stray = resolve(args.refs or design.refs, args.shots, set(args.allow_missing), design)
    write_html(rows, out, design)
    lines = missing_lines(rows, unlisted, stray)
    (out / "missing.txt").write_text("".join(line + "\n" for line in lines))
    if args.png:
        write_png(rows, out, design)
    bad = failing(rows)
    for p in bad:
        print(f"parity: {p.kind} frame {p.frame} has no {p.appearance} shot", file=sys.stderr)
    for stem in unlisted:
        print(f"parity: reference {stem} is in neither FRAMES nor LATER", file=sys.stderr)
    print(f"parity: {sum(1 for p in rows if p.shot)} of {len(rows)} frames paired, {len(lines)} lines in {out / 'missing.txt'}")
    return 1 if bad or unlisted else 0


def add_design(parser):
    parser.add_argument("--design", choices=tuple(DESIGNS), default="f", help="frame table: f (default) or v5")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    f = sub.add_parser("frames", help="print the launch frame names")
    add_design(f)
    f.add_argument("--appearance", choices=APPEARANCES)
    f.add_argument("--kind", choices=KINDS)
    f.add_argument("--regex", action="store_true", help="print the anchored alternation")
    f.set_defaults(run=command_frames)
    li = sub.add_parser("lines", help="list the launch frames that have no manifest line")
    add_design(li)
    li.add_argument("--manifest", default=str(DEFAULT_MANIFEST))
    li.add_argument("--strict", action="store_true", help="exit 1 while any screen or board frame has no line")
    li.set_defaults(run=command_lines)
    s = sub.add_parser("sheet", help="build the side-by-side pages, or one frame | before | after PNG")
    add_design(s)
    s.add_argument("frame", nargs="?", help="one frame: write a frame | before | after PNG (needs --before and --after)")
    s.add_argument("--before", help="the app's shot before the change (with FRAME)")
    s.add_argument("--after", help="the app's shot after the change (with FRAME)")
    s.add_argument("--shots", help="folder of shots (without FRAME)")
    s.add_argument("--refs", help="reference folder (default: the design's screens)")
    s.add_argument("--out", help="output folder, or with FRAME the PNG (default <FRAME>-sheet.png)")
    s.add_argument("--png", action="store_true", help="also write PNG sheets (needs Pillow)")
    s.add_argument("--allow-missing", nargs="*", default=[], metavar="FRAME", help="launch frames that may have no shot")
    s.set_defaults(run=command_sheet, parser=s)
    args = parser.parse_args(argv)
    return args.run(args)


if __name__ == "__main__":
    sys.exit(main())
