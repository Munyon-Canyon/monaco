#!/usr/bin/env python3
"""Tests for parity.py: the F frame tables, the frames and lines commands, pairing, missing.txt and exit codes.

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

LAUNCH = ["02-home-dark", "02-home-light", "01-tokens", "12-toast"]
SYSTEM = "14-notification"
LATER = "14-widgets"
UNLISTED = "99-designer-added"
LIGHT_SHOT = ("light", "f__02-home-light")
DARK_SHOTS = [("dark", "f__02-home-dark"), ("dark", "f__01-tokens"), ("dark", "f__12-toast")]
ALL_SHOTS = [LIGHT_SHOT] + DARK_SHOTS


class Folders:
    def __init__(self, shots, refs=None):
        self.dir = tempfile.TemporaryDirectory()
        base = Path(self.dir.name)
        self.refs = base / "refs"
        self.shots = base / "shots"
        self.out = base / "out"
        self.refs.mkdir()
        for frame in LAUNCH + [SYSTEM, LATER] if refs is None else refs:
            (self.refs / f"{frame}.png").write_bytes(b"png")
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


class FolderCase(unittest.TestCase):
    def folders(self, shots, refs=None):
        f = Folders(shots, refs)
        self.addCleanup(f.dir.cleanup)
        return f


class TableTests(unittest.TestCase):
    def test_frame_names_are_unique(self):
        names = [frame for frame, _, _ in parity.FRAMES]
        self.assertEqual(len(names), len(set(names)))
        self.assertEqual(len(parity.LATER), 16)
        self.assertFalse(set(names) & parity.LATER)

    def test_104_launch_frames_by_kind(self):
        kinds = [kind for _, kind, _ in parity.FRAMES]
        self.assertEqual(len(kinds), 104)
        self.assertEqual((kinds.count("screen"), kinds.count("board"), kinds.count("system")), (95, 4, 5))

    def test_every_frame_has_an_owner(self):
        for frame, _, owner in parity.FRAMES:
            self.assertRegex(owner, r"^#\d+(, #\d+)*$", frame)

    def test_five_light_twins_are_launch_frames(self):
        names = {frame for frame, _, _ in parity.FRAMES}
        self.assertEqual(len(parity.LIGHT), 5)
        self.assertTrue(parity.LIGHT <= names)

    def test_real_references_match_the_tables(self):
        listed = {frame for frame, _, _ in parity.FRAMES} | parity.LATER
        on_disk = {p.stem for p in parity.DEFAULT_REFS.glob("*.png")}
        self.assertEqual(on_disk - listed, set(), "PNG in neither FRAMES nor LATER")
        self.assertEqual(listed - on_disk, set(), "name without a PNG")

    def test_every_group_has_a_title(self):
        for frame, _, _ in parity.FRAMES:
            self.assertIn(parity.group_of(frame), parity.GROUP_TITLES, frame)


class FramesTests(FolderCase):
    def test_frames_prints_every_launch_frame_with_the_prefix(self):
        f = self.folders([])
        code, out, _ = f.run("frames")
        self.assertEqual(code, 0)
        self.assertEqual(out.splitlines(), ["f__" + frame for frame, _, _ in parity.FRAMES])

    def test_light_prints_only_the_twins(self):
        f = self.folders([])
        _, out, _ = f.run("frames", "--appearance", "light")
        self.assertEqual(
            out.splitlines(),
            [
                "f__02-home-light",
                "f__03-cabal-treasury-light",
                "f__04-proposal-light",
                "f__05-feed-light",
                "f__07-stock-light",
            ],
        )

    def test_dark_is_everything_else(self):
        f = self.folders([])
        _, out, _ = f.run("frames", "--appearance", "dark")
        self.assertEqual(len(out.splitlines()), 99)

    def test_kind_filter(self):
        f = self.folders([])
        self.assertEqual(len(f.run("frames", "--kind", "screen")[1].splitlines()), 95)
        self.assertEqual(len(f.run("frames", "--kind", "board")[1].splitlines()), 4)
        self.assertEqual(len(f.run("frames", "--kind", "system")[1].splitlines()), 5)

    def test_regex_matches_exactly_the_listed_names(self):
        f = self.folders([])
        _, out, _ = f.run("frames", "--kind", "board", "--regex")
        pattern = re.compile(out.strip())
        boards = ["f__01-tokens", "f__01-tokens-type", "f__01-tokens-controls", "f__01-chart-variants"]
        for name in boards:
            self.assertTrue(pattern.search(name), name)
        for name in ("f__01-tokens-later", "xf__01-tokens", "f__01-tokens-x", "01-tokens", "f__02-home-dark"):
            self.assertFalse(pattern.search(name), name)

    def test_later_frames_are_never_listed(self):
        f = self.folders([])
        _, out, _ = f.run("frames")
        for frame in parity.LATER:
            self.assertNotIn("f__" + frame, out.splitlines())


class SheetTests(FolderCase):
    def test_complete_run_exits_0_and_lists_the_system_frame_and_later_frame_only_as_missing(self):
        f = self.folders(ALL_SHOTS)
        code, _, _ = f.sheet("--allow-missing", *self.others())
        self.assertEqual(code, 0)
        self.assertIn(f"no shot\tdark\t{SYSTEM}", f.missing())
        self.assertNotIn(LATER, "".join(f.missing()))

    def others(self):
        return [frame for frame, kind, _ in parity.FRAMES if kind != "system" and frame not in LAUNCH]

    def test_pages_are_written_for_every_group(self):
        f = self.folders(ALL_SHOTS)
        f.sheet()
        for group in parity.GROUPS:
            self.assertTrue((f.out / f"{group}.html").is_file(), group)
        self.assertTrue((f.out / "index.html").is_file())

    def test_a_row_pairs_reference_and_shot_by_appearance_and_relative_path(self):
        f = self.folders(ALL_SHOTS)
        f.sheet()
        page = (f.out / "02.html").read_text()
        self.assertIn('src="../refs/02-home-dark.png"', page)
        self.assertIn('src="../shots/dark/f__02-home-dark.png"', page)
        self.assertIn('src="../refs/02-home-light.png"', page)
        self.assertIn('src="../shots/light/f__02-home-light.png"', page)
        self.assertNotIn("shots/light/f__02-home-dark", page)
        self.assertNotIn("shots/dark/f__02-home-light", page)
        self.assertIn("02-home-dark / dark / #6480, #6496", page)
        self.assertIn("02-home-light / light / #6480, #6496", page)
        self.assertIn('<ul class="differences"></ul>', page)

    def test_boards_sit_on_the_01_page(self):
        f = self.folders(ALL_SHOTS)
        f.sheet()
        self.assertIn("01-tokens / dark / #6432, #6440", (f.out / "01.html").read_text())

    def test_a_light_shot_in_the_dark_folder_does_not_pair(self):
        f = self.folders([("dark", "f__02-home-light")] + DARK_SHOTS)
        code, _, err = f.sheet()
        self.assertEqual(code, 1)
        self.assertIn("02-home-light has no light shot", err)
        self.assertIn("no reference\tdark\tf__02-home-light", f.missing())

    def test_a_missing_screen_shot_fails_and_is_listed(self):
        f = self.folders(ALL_SHOTS[:-1])
        code, _, err = f.sheet("--allow-missing", *self.others())
        self.assertEqual(code, 1)
        self.assertIn("no shot\tdark\t12-toast", f.missing())
        self.assertIn("screen frame 12-toast has no dark shot", err)

    def test_a_missing_board_shot_fails(self):
        f = self.folders([s for s in ALL_SHOTS if s[1] != "f__01-tokens"])
        code, _, err = f.sheet("--allow-missing", *self.others())
        self.assertEqual(code, 1)
        self.assertIn("board frame 01-tokens has no dark shot", err)

    def test_a_missing_screen_shot_passes_when_allowed(self):
        f = self.folders(ALL_SHOTS[:-1])
        code, _, _ = f.sheet("--allow-missing", "12-toast", *self.others())
        self.assertEqual(code, 0)
        self.assertIn("allowed missing\tdark\t12-toast", f.missing())
        self.assertNotIn("no shot\tdark\t12-toast", f.missing())

    def test_a_missing_system_shot_is_listed_but_passes(self):
        f = self.folders(ALL_SHOTS)
        code, _, _ = f.sheet("--allow-missing", *self.others())
        self.assertEqual(code, 0)
        self.assertIn(f"no shot\tdark\t{SYSTEM}", f.missing())

    def test_an_unlisted_reference_fails_and_is_listed(self):
        f = self.folders(ALL_SHOTS, LAUNCH + [UNLISTED])
        code, _, err = f.sheet("--allow-missing", *self.others())
        self.assertEqual(code, 1)
        self.assertIn(f"unlisted\t-\t{UNLISTED}", f.missing())
        self.assertIn(UNLISTED, err)

    def test_a_later_reference_is_skipped(self):
        f = self.folders(ALL_SHOTS)
        f.sheet()
        self.assertNotIn(LATER, "".join(f.missing()))
        self.assertNotIn(LATER, (f.out / "14.html").read_text())

    def test_an_unknown_allow_missing_name_exits_2(self):
        f = self.folders(ALL_SHOTS)
        code, _, err = f.sheet("--allow-missing", "99-nope")
        self.assertEqual(code, 2)
        self.assertIn("99-nope", err)

    def test_a_later_frame_is_not_a_valid_allow_missing_name(self):
        f = self.folders(ALL_SHOTS)
        code, _, _ = f.sheet("--allow-missing", LATER)
        self.assertEqual(code, 2)

    def test_a_shot_without_a_frame_is_listed(self):
        f = self.folders(ALL_SHOTS + [("dark", "f__99-nope")])
        f.sheet()
        self.assertIn("no reference\tdark\tf__99-nope", f.missing())

    def test_empty_shots_fail_every_screen_and_board_frame(self):
        f = self.folders([])
        code, _, err = f.sheet()
        self.assertEqual(code, 1)
        self.assertEqual(err.count("has no"), 99)

    def test_png_without_pillow_says_so(self):
        f = self.folders(ALL_SHOTS)
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


class LinesTests(unittest.TestCase):
    def manifest(self, text):
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        path = Path(d.name) / "manifest.txt"
        path.write_text(text)
        return str(path)

    def run_lines(self, text, *extra):
        out, err = StringIO(), StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = parity.main(["lines", "--manifest", self.manifest(text), *extra])
        return code, out.getvalue().splitlines(), err.getvalue()

    def test_reports_screen_and_board_frames_without_a_line_and_their_owner(self):
        code, out, _ = self.run_lines("f__01-tokens  -MonacoDesignGallery\nhome-ready -MonacoHomeSample ready\n")
        self.assertEqual(code, 0)
        self.assertEqual(len(out), 98)
        self.assertNotIn("f__01-tokens\t#6432, #6440", out)
        self.assertIn("f__01-tokens-type\t#6440", out)
        self.assertIn("f__02-home-dark\t#6480, #6496", out)
        self.assertFalse([line for line in out if line.startswith("f__14-notification")])

    def test_a_line_for_an_unknown_frame_fails(self):
        code, _, err = self.run_lines("f__99-nope -MonacoHomeSample ready\n")
        self.assertEqual(code, 1)
        self.assertIn("f__99-nope is not a launch frame", err)

    def test_a_line_for_a_later_frame_fails(self):
        code, _, _ = self.run_lines(f"f__{LATER} -MonacoHomeSample ready\n")
        self.assertEqual(code, 1)

    def test_a_line_for_a_system_frame_fails(self):
        code, _, err = self.run_lines(f"f__{SYSTEM} -MonacoHomeSample ready\n")
        self.assertEqual(code, 1)
        self.assertIn("system frame", err)

    def test_strict_fails_while_any_frame_is_uncovered(self):
        code, out, _ = self.run_lines("f__01-tokens -MonacoDesignGallery\n", "--strict")
        self.assertEqual(code, 1)
        self.assertTrue(out)

    def test_strict_passes_when_every_screen_and_board_frame_has_a_line(self):
        text = "".join(
            f"f__{frame} -MonacoHomeSample ready\n" for frame, kind, _ in parity.FRAMES if kind != "system"
        )
        code, out, _ = self.run_lines("# F parity\n" + text, "--strict")
        self.assertEqual((code, out), (0, []))

    def test_comments_and_other_names_are_ignored(self):
        code, out, _ = self.run_lines("# f__99-nope\n\nhome-ready -MonacoHomeSample ready\n")
        self.assertEqual(code, 0)
        self.assertEqual(len(out), 99)

    def test_the_real_manifest_has_the_four_board_lines(self):
        out, err = StringIO(), StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = parity.main(["lines"])
        lines = out.getvalue().splitlines()
        self.assertEqual(code, 0, err.getvalue())
        self.assertFalse([line for line in lines if line.split("\t")[0] in
                          {"f__01-tokens", "f__01-tokens-type", "f__01-tokens-controls", "f__01-chart-variants"}])


if __name__ == "__main__":
    unittest.main()
