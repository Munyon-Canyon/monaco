#!/usr/bin/env python3
"""Tests for journey-cross.sh, the merge-time cross-check of a PR head against the current base.

Run: python3 scripts/qa/test_journey_cross.py
"""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
CROSS = REPO_ROOT / "scripts" / "qa" / "journey-cross.sh"
SIGN_IN = "docs/journeys/auth/sign-in.md"


def git(*args, env=None):
    out = subprocess.run(["git", *args], cwd=REPO_ROOT, env=env, capture_output=True, text=True, check=True)
    return out.stdout.strip()


def has_commit(sha):
    return subprocess.run(["git", "cat-file", "-e", sha + "^{commit}"], cwd=REPO_ROOT, capture_output=True).returncode == 0


class JourneyCross(unittest.TestCase):
    """scripts/qa/journey-cross.sh: fail when merging a head onto a base adds a journey check error."""

    @classmethod
    def setUpClass(cls):
        cls.index = tempfile.TemporaryDirectory()
        cls.head = git("rev-parse", "HEAD")

    @classmethod
    def tearDownClass(cls):
        cls.index.cleanup()

    def commit(self, edit):
        """A commit on HEAD whose sign-in journey doc is edit(text). Only objects are written."""
        env = dict(os.environ, GIT_INDEX_FILE=self.index.name + "/index")
        git("read-tree", self.head, env=env)
        text = git("show", self.head + ":" + SIGN_IN)
        blob = subprocess.run(["git", "hash-object", "-w", "--stdin"], cwd=REPO_ROOT, input=edit(text + "\n"),
                              capture_output=True, text=True, check=True).stdout.strip()
        git("update-index", "--cacheinfo", "100644," + blob + "," + SIGN_IN, env=env)
        return git("commit-tree", git("write-tree", env=env), "-p", self.head, "-m", "synthetic")

    def cross(self, base, head):
        env = dict(os.environ, BASE=base,
                   GIT_CONFIG_COUNT="1", GIT_CONFIG_KEY_0="remote.origin.url", GIT_CONFIG_VALUE_0="/nonexistent")
        return subprocess.run([str(CROSS), head], cwd=REPO_ROOT, env=env, capture_output=True, text=True)

    def test_a_merge_that_adds_an_error_exits_1_and_prints_it(self):
        broken = self.commit(lambda text: text.replace("\nversion: ", "\nversion: x", 1))
        result = self.cross("HEAD", broken)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("journey-cross: FAIL", result.stdout)
        self.assertIn(SIGN_IN + ": version must be a whole number from 1, got 'x", result.stdout)
        paths = [line.split()[0] for line in git("worktree", "list").splitlines()]
        self.assertEqual([p for p in paths if p.endswith(("/base", "/merged"))], [], "worktrees left behind")

    def test_a_prose_only_change_exits_0(self):
        prose = self.commit(lambda text: text + "A line of prose.\n")
        result = self.cross("HEAD", prose)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("journey-cross: ok", result.stdout)

    def test_a_head_that_conflicts_with_the_base_exits_2(self):
        ours = self.commit(lambda text: text.replace("title: Sign in", "title: Sign in A", 1))
        theirs = self.commit(lambda text: text.replace("title: Sign in", "title: Sign in B", 1))
        result = self.cross(ours, theirs)
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn("does not merge cleanly", result.stdout)

    PR_8156_BASE = "46e9ec191"
    PR_8156_HEAD = "adc4a47ace7d5e0ecd7cac7b5f0e11c1b6edacce"

    @unittest.skipUnless(has_commit(PR_8156_BASE) and has_commit(PR_8156_HEAD), "PR 8156 commits are not in this clone")
    def test_replay_8156_over_46e9ec191_reports_the_best_price_error(self):
        result = self.cross(self.PR_8156_BASE, self.PR_8156_HEAD)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertRegex(result.stdout, r"S2\.2")
        self.assertRegex(result.stdout, r"(?i)best.price")


if __name__ == "__main__":
    unittest.main()
