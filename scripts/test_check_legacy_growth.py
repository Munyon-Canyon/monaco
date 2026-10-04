import contextlib
import importlib.util
import io
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "check_legacy_growth", pathlib.Path(__file__).with_name("check-legacy-growth.py")
)
check = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = check
spec.loader.exec_module(check)

SWIFT_TEST = pathlib.Path(__file__).parent.parent / "packages/mobile-core/Tests/MonacoCoreTests/LegacyFreezeTests.swift"
CLIENT = "apps/mobile/Monaco/API/MonacoAPIClient.swift"
VIEW = "apps/mobile/Monaco/Features/Home/HomeView.swift"
UNLISTED = "apps/mobile/Monaco/Features/Home/Other.swift"
CLIENT_TEXT = "let a = URLRequest(url: u)\nlet b = 1\n"
VIEW_TEXT = "pollWhileVisible(every: 5)\n"


class LegacyGrowthTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        cwd = os.getcwd()
        self.addCleanup(os.chdir, cwd)
        self.root = pathlib.Path(tmp.name)
        os.chdir(self.root)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.email", "t@example.com")
        self.git("config", "user.name", "t")
        self.git("config", "maintenance.auto", "false")
        self.base = self.commit({
            check.LIST: f"{CLIENT}\n{VIEW}\n",
            CLIENT: CLIENT_TEXT,
            VIEW: VIEW_TEXT,
            UNLISTED: "",
        })

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True, text=True).stdout

    def commit(self, files):
        for path, text in files.items():
            p = self.root / path
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(text)
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", "change")
        return self.git("rev-parse", "HEAD").strip()

    def run_check(self, files, labels="[]"):
        head = self.commit(files)
        out = io.StringIO()
        env = {"BASE_SHA": self.base, "HEAD_SHA": head, "PR_LABELS": labels}
        with mock.patch.dict(os.environ, env), contextlib.redirect_stdout(out):
            code = check.main()
        return code, out.getvalue().splitlines()

    def test_raised_pattern_fails(self):
        code, out = self.run_check({VIEW: VIEW_TEXT + "PollLoop.run(x)\n"})
        self.assertEqual((code, out), (1, [f"legacy code grew: poll {VIEW} 1 -> 2"]))

    def test_raised_line_count_fails(self):
        code, out = self.run_check({CLIENT: CLIENT_TEXT + "let c = 2\n"})
        self.assertEqual((code, out), (1, [f"legacy code grew: lines {CLIENT} 2 -> 3"]))

    def test_lowered_or_equal_counts_pass(self):
        code, out = self.run_check({CLIENT: "let b = 1\n", VIEW: VIEW_TEXT.replace("5", "10")})
        self.assertEqual((code, out), (0, []))

    def test_unlisted_file_is_ignored(self):
        code, out = self.run_check({UNLISTED: "let r = URLRequest(url: u)\n"})
        self.assertEqual((code, out), (0, []))

    def test_newly_listed_file_counts_from_zero(self):
        added = "apps/mobile/Monaco/API/New.swift"
        code, out = self.run_check({check.LIST: f"{CLIENT}\n{VIEW}\n{added}\n", added: "Timer.publish(every: 1)\n"})
        self.assertEqual((code, out), (1, [
            f"legacy code grew: timer {added} 0 -> 1",
            f"legacy code grew: lines {added} 0 -> 1",
        ]))

    def test_label_waives_growth(self):
        code, out = self.run_check({CLIENT: CLIENT_TEXT + "let c = URLRequest(url: u)\n"},
                                   labels='["gate-change-approved"]')
        self.assertEqual(code, 0)
        self.assertEqual(out, [
            f"legacy code grew: urlrequest {CLIENT} 1 -> 2",
            f"legacy code grew: lines {CLIENT} 2 -> 3",
            "legacy growth approved by gate-change-approved",
        ])

    def test_patterns_match_the_swift_test(self):
        text = SWIFT_TEST.read_text()
        block = text[text.index("var patterns: [String] {"):text.index("struct MalformedRow")]
        swift = {}
        for case, literals in re.findall(r"case \.(\w+): return \[(.*)\]", block):
            raw = re.search(rf'case {case} = "(\w+)"', text)
            metric = raw.group(1) if raw else case
            swift[metric] = [s.replace('\\"', '"') for s in re.findall(r'"((?:[^"\\]|\\.)*)"', literals)]
        self.assertEqual(swift, check.PATTERNS)


if __name__ == "__main__":
    unittest.main()
