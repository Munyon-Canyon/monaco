#!/usr/bin/env python3
"""Pair the app's screenshots with F's launch frames and lay them side by side.

  parity.py frames [--appearance dark|light] [--kind screen|board|system] [--regex]
      The f__<frame> names of the launch frames, one per line, or one anchored alternation for
      MONACO_QA_SCREEN_ONLY. It replaces v3's Command line interface to a user's defaults.
Syntax:

'defaults' [-verbose] [-currentHost | -host <hostname>] [-container <container identifier>] followed by one of the following:

  read                                 shows all defaults
  read <domain>                        shows defaults for given domain
  read <domain> <key>                  shows defaults for given domain, key

  read-type <domain> <key>             shows the type for the given domain, key

  write <domain> <domain_rep>          writes domain (overwrites existing)
  write <domain> <key> <value>         writes key for domain

  rename <domain> <old_key> <new_key>  renames old_key to new_key

  delete <domain>                      deletes domain
  delete <domain> <key>                deletes key in domain
  delete-all <domain>                  deletes the domain from all containers
  delete-all <domain> Key>             deletes key in domain from all containers

  import <domain> <path to plist>      writes the plist at path to domain
  import <domain> -                    writes a plist from stdin to domain
  export <domain> <path to plist>      saves domain as a binary plist to path
  export <domain> -                    writes domain as an xml plist to stdout
  domains                              lists all domains
  find <word>                          lists all entries containing word
  help                                 print this help

<domain> is ( <domain_name> | -app <application_name> | -globalDomain )
         or a path to a file omitting the '.plist' extension

<value> is one of:
  <value_rep>
  -string <string_value>
  -data <hex_digits>
  -int[eger] <integer_value>
  -float  <floating-point_value>
  -bool[ean] (true | false | yes | no)
  -date <date_rep>
  -array <value1> <value2> ...
  -array-add <value1> <value2> ...
  -dict <key1> <value1> <key2> <value2> ...
  -dict-add <key1> <value1> ....
  parity.py lines [--manifest FILE] [--strict]
      Read the f__ lines of scripts/qa/sample-screens.txt (the  block) and print each
      screen or board frame that has no line, with its owner. Exits 1 on an f__ line whose frame
      is not a launch frame, or on a line for a system frame; with --strict also when any screen
      or board frame has no line.
  parity.py sheet --shots DIR --out DIR [--refs DIR] [--png] [--allow-missing FRAME ...]
      Write index.html, one <group>.html per two-digit frame prefix and missing.txt. With --png
      also write <group>.png sheets (needs Pillow). Exits 1 when a screen or board frame has no
      shot (unless it is passed to --allow-missing) or when a reference PNG is in neither FRAMES
      nor LATER ("unlisted"). A system frame with no shot is listed but does not fail.

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


def appearance_of(frame):
    return "light" if frame in LIGHT else "dark"


def group_of(frame):
    return frame[:2]


class Pair:
    def __init__(self, frame, kind, owner, ref, shot):
        self.frame = frame
        self.name = PREFIX + frame
        self.kind = kind
        self.owner = owner
        self.appearance = appearance_of(frame)
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


def shot_path(shots, frame):
    return Path(shots) / appearance_of(frame) / f"{PREFIX}{frame}.png"


def resolve(refs, shots, allowed=()):
    found = referenced(refs)
    rows = []
    expected = {a: set() for a in APPEARANCES}
    for frame, kind, owner in FRAMES:
        shot = shot_path(shots, frame)
        pair = Pair(frame, kind, owner, found.get(frame), shot if shot.is_file() else None)
        pair.allowed = frame in allowed
        rows.append(pair)
        expected[pair.appearance].add(f"{PREFIX}{frame}")
    unlisted = sorted(stem for stem in found if stem not in {f for f, _, _ in FRAMES} and stem not in LATER)
    stray = []
    for appearance in APPEARANCES:
        folder = Path(shots) / appearance
        if not folder.is_dir():
            continue
        for path in sorted(folder.glob(f"{PREFIX}*.png")):
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


def nav():
    links = ['<a href="index.html">index</a>'] + [
        f'<a href="{g}.html">{g} {html.escape(GROUP_TITLES[g])}</a>' for g in GROUPS
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


def write_html(rows, out):
    out = Path(out)
    lines = []
    for group in GROUPS:
        members = [p for p in rows if p.group == group]
        title = f"{group} {GROUP_TITLES[group]}"
        body = nav() + f"<h1>{html.escape(title)}</h1>\n" + "".join(pair_html(p, out) for p in members)
        (out / f"{group}.html").write_text(page(f"F parity {title}", body))
        shot = sum(1 for p in members if p.shot)
        lines.append(f'<li><a href="{group}.html">{html.escape(title)}</a> {shot} of {len(members)} shot</li>')
    body = nav() + "<h1>F parity</h1>\n<ul>\n" + "\n".join(lines) + "\n</ul>\n"
    (out / "index.html").write_text(page("F parity", body))


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
    names = [
        PREFIX + frame
        for frame, kind, _ in FRAMES
        if (args.appearance is None or appearance_of(frame) == args.appearance)
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
    kinds = {frame: kind for frame, kind, _ in FRAMES}
    covered = set()
    bad = 0
    for name in manifest_names(args.manifest):
        if not name.startswith(PREFIX):
            continue
        frame = name[len(PREFIX):]
        if frame not in kinds:
            print(f"parity: {name} is not a launch frame", file=sys.stderr)
            bad += 1
        elif kinds[frame] == "system":
            print(f"parity: {name} is a system frame, a push on the simulator, not a harness line", file=sys.stderr)
            bad += 1
        else:
            covered.add(frame)
    uncovered = [(frame, owner) for frame, kind, owner in FRAMES if kind != "system" and frame not in covered]
    for frame, owner in uncovered:
        print(f"{PREFIX}{frame}\t{owner}")
    if bad:
        return 1
    return 1 if args.strict and uncovered else 0


def command_sheet(args):
    launch = {frame for frame, _, _ in FRAMES}
    unknown = [name for name in args.allow_missing if name not in launch]
    if unknown:
        print(f"parity: --allow-missing takes launch frame names; unknown: {', '.join(unknown)}", file=sys.stderr)
        return 2
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rows, unlisted, stray = resolve(args.refs, args.shots, set(args.allow_missing))
    write_html(rows, out)
    lines = missing_lines(rows, unlisted, stray)
    (out / "missing.txt").write_text("".join(line + "\n" for line in lines))
    if args.png:
        write_png(rows, out)
    bad = failing(rows)
    for p in bad:
        print(f"parity: {p.kind} frame {p.frame} has no {p.appearance} shot", file=sys.stderr)
    for stem in unlisted:
        print(f"parity: reference {stem} is in neither FRAMES nor LATER", file=sys.stderr)
    print(f"parity: {sum(1 for p in rows if p.shot)} of {len(rows)} frames paired, {len(lines)} lines in {out / 'missing.txt'}")
    return 1 if bad or unlisted else 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    f = sub.add_parser("frames", help="print the launch frame names")
    f.add_argument("--appearance", choices=APPEARANCES)
    f.add_argument("--kind", choices=KINDS)
    f.add_argument("--regex", action="store_true", help="print the anchored alternation")
    f.set_defaults(run=command_frames)
    li = sub.add_parser("lines", help="list the launch frames that have no manifest line")
    li.add_argument("--manifest", default=str(DEFAULT_MANIFEST))
    li.add_argument("--strict", action="store_true", help="exit 1 while any screen or board frame has no line")
    li.set_defaults(run=command_lines)
    s = sub.add_parser("sheet", help="build the side-by-side pages")
    s.add_argument("--shots", required=True)
    s.add_argument("--refs", default=str(DEFAULT_REFS))
    s.add_argument("--out", required=True)
    s.add_argument("--png", action="store_true", help="also write PNG sheets (needs Pillow)")
    s.add_argument("--allow-missing", nargs="*", default=[], metavar="FRAME", help="launch frames that may have no shot")
    s.set_defaults(run=command_sheet)
    args = parser.parse_args(argv)
    return args.run(args)


if __name__ == "__main__":
    sys.exit(main())
