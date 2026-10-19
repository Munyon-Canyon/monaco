#!/usr/bin/env python3
"""Tests for parity.py: default-state detection, the regex, pairing, missing.txt and exit codes.

Run: python3 -m unittest scripts/qa/test_parity.py
"""

import re
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity  # noqa: E402

REFS = [
    "home__home__loss__light.jpg",
    "home__home__loss__dark.jpg",
    "home__home__gain__light.jpg",
    "cabals__cabals__ready__light.jpg",
    "cabals__cabals__ready__dark.jpg",
    "cabals__cabals__empty__light.jpg",
    "cabals__cabal-info__member__light.jpg",
    "cabals__cabal-info__member__dark.jpg",
]
DEFAULTS = ["cabals__cabal-info__member", "cabals__cabals__ready", "home__home__loss"]


class Folders:
    def __init__(self, shots):
        self.dir = tempfile.TemporaryDirectory()
        base = Path(self.dir.name)
        self.refs = base / "refs"
        self.shots = base / "shots"
        self.out = base / "out"
        self.refs.mkdir()
        for name in REFS:
            (self.refs / name).write_bytes(b"jpg")
        for appearance, name in shots:
            folder = self.shots / appearance
            folder.mkdir(parents=True, exist_ok=True)
            (folder / f"{name}.png").write_bytes(b"png")

    def run(self, *args):
        out, err = StringIO(), StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = parity.main(list(args))
        return code, out.getvalue(), err.getvalue()

    def sheet(self, *extra):
        return self.run(
            "sheet", "--shots", str(self.shots), "--refs", str(self.refs), "--out", str(self.out), *extra
        )

    def missing(self):
        return (self.out / "missing.txt").read_text().splitlines()


ALL_LIGHT = [("light", n) for n in ("home__home__loss", "home__home__gain", "cabals__cabals__ready",
                                    "cabals__cabals__empty", "cabals__cabal-info__member")]
ALL_DARK = [("dark", n) for n in DEFAULTS]


class FolderCase(unittest.TestCase):
    def folders(self, shots):
        f = Folders(shots)
        self.addCleanup(f.dir.cleanup)
        return f


class DefaultsTests(FolderCase):
    def test_default_states_are_the_ones_with_a_dark_sibling(self):
        f = self.folders([])
        self.assertEqual(parity.default_names(f.refs), DEFAULTS)

    def test_defaults_prints_one_name_per_line(self):
        f = self.folders([])
        code, out, _ = f.run("defaults", "--refs", str(f.refs))
        self.assertEqual(code, 0)
        self.assertEqual(out.splitlines(), DEFAULTS)

    def test_regex_matches_exactly_the_defaults(self):
        f = self.folders([])
        _, out, _ = f.run("defaults", "--regex", "--refs", str(f.refs))
        pattern = re.compile(out.strip())
        for name in DEFAULTS:
            self.assertTrue(pattern.search(name), name)
        for name in ("home__home__gain", "home__home__loss-dark", "xhome__home__loss", "cabals__cabals__empty"):
            self.assertFalse(pattern.search(name), name)

    def test_split_ref_ignores_other_files(self):
        self.assertEqual(parity.split_ref("home__home__loss__dark.jpg"), ("home__home__loss", "dark"))
        self.assertIsNone(parity.split_ref("index.json"))
        self.assertIsNone(parity.split_ref("home__loss__light.jpg"))

    def test_real_references_have_43_defaults(self):
        self.assertEqual(len(parity.default_names(parity.DEFAULT_REFS)), 43)


class SheetTests(FolderCase):
    def test_complete_run_exits_0_and_missing_is_empty(self):
        f = self.folders(ALL_LIGHT + ALL_DARK)
        code, _, _ = f.sheet()
        self.assertEqual(code, 0)
        self.assertEqual(f.missing(), [])

    def test_pages_are_written_for_every_group(self):
        f = self.folders(ALL_LIGHT + ALL_DARK)
        f.sheet()
        for group in parity.GROUPS:
            self.assertTrue((f.out / f"{group}.html").is_file(), group)
        self.assertTrue((f.out / "index.html").is_file())

    def test_a_row_pairs_reference_and_shot_by_relative_path(self):
        f = self.folders(ALL_LIGHT + ALL_DARK)
        f.sheet()
        page = (f.out / "home.html").read_text()
        self.assertIn('src="../refs/home__home__loss__light.jpg"', page)
        self.assertIn('src="../shots/light/home__home__loss.png"', page)
        self.assertIn('src="../refs/home__home__loss__dark.jpg"', page)
        self.assertIn('src="../shots/dark/home__home__loss.png"', page)
        self.assertIn("home / loss / light", page)
        self.assertIn("#5559", page)
        self.assertIn('<ul class="differences"></ul>', page)

    def test_missing_default_dark_shot_fails_and_is_listed(self):
        f = self.folders(ALL_LIGHT + ALL_DARK[:-1])
        code, _, err = f.sheet()
        self.assertEqual(code, 1)
        self.assertIn("no shot\tdark\thome__home__loss", f.missing())
        self.assertIn("home__home__loss has no dark shot", err)

    def test_missing_default_light_shot_fails(self):
        f = self.folders([s for s in ALL_LIGHT if s[1] != "cabals__cabals__ready"] + ALL_DARK)
        code, _, _ = f.sheet()
        self.assertEqual(code, 1)
        self.assertIn("no shot\tlight\tcabals__cabals__ready", f.missing())

    def test_missing_non_default_state_is_listed_but_passes(self):
        f = self.folders([s for s in ALL_LIGHT if s[1] != "home__home__gain"] + ALL_DARK)
        code, _, _ = f.sheet()
        self.assertEqual(code, 0)
        self.assertEqual(f.missing(), ["no shot\tlight\thome__home__gain"])

    def test_shot_without_a_reference_is_listed(self):
        f = self.folders(ALL_LIGHT + ALL_DARK + [("light", "home__home__nope"), ("dark", "home__home__gain")])
        code, _, _ = f.sheet()
        self.assertEqual(code, 0)
        self.assertEqual(
            sorted(f.missing()),
            ["no reference\tdark\thome__home__gain", "no reference\tlight\thome__home__nope"],
        )

    def test_empty_shots_fail_every_default(self):
        f = self.folders([])
        code, _, err = f.sheet()
        self.assertEqual(code, 1)
        self.assertEqual(err.count("has no"), 6)
        self.assertEqual(len(f.missing()), len(REFS))

    def test_png_without_pillow_says_so(self):
        f = self.folders(ALL_LIGHT + ALL_DARK)
        real = sys.modules.get("PIL")
        sys.modules["PIL"] = None
        try:
            with self.assertRaises(SystemExit) as caught:
                f.sheet("--png")
        finally:
            if real is None:
                del sys.modules["PIL"]
            else:
                sys.modules["PIL"] = real
        self.assertIn("needs Pillow", str(caught.exception))

    def test_every_real_template_has_a_ticket(self):
        for name in parity.references(parity.DEFAULT_REFS)["light"]:
            template = name.split("__")[1]
            self.assertIn(template, parity.TICKETS, template)


if __name__ == "__main__":
    unittest.main()
