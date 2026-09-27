import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    "check_pr_size", pathlib.Path(__file__).with_name("check-pr-size.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class CountTest(unittest.TestCase):
    def test_counts_added_and_deleted_lines(self):
        total, _ = check.count("10\t5\tapps/backend/internal/app/fund.go\n3\t0\tREADME.md\n")
        self.assertEqual(total, 18)

    def test_skips_generated_and_lock_files(self):
        numstat = (
            "400\t0\tapps/backend/internal/db/queries.gen.go\n"
            "900\t12\tapps/backend/go.sum\n"
            "50\t0\tapps/backend/test/evidence/07.json\n"
            "7\t1\tapps/backend/internal/app/fund.go\n"
        )
        self.assertEqual(check.count(numstat)[0], 8)

    def test_skips_binary_files(self):
        self.assertEqual(check.count("-\t-\tdocs/legacy/qa/home.png\n")[0], 0)

    def test_pure_rename_counts_zero(self):
        self.assertEqual(check.count("0\t0\tdocs/{archive => legacy}/README.md\n")[0], 0)

    def test_rename_with_edits_counts_edits_under_new_path(self):
        total, counted = check.count("4\t2\tdocs/{archive => legacy}/README.md\n")
        self.assertEqual(total, 6)
        self.assertEqual(counted[0][1], "docs/legacy/README.md")

    def test_renamed_path_forms(self):
        self.assertEqual(check.renamed_path("a/{b => c}/d.go"), "a/c/d.go")
        self.assertEqual(check.renamed_path("old.go => new.go"), "new.go")
        self.assertEqual(check.renamed_path("plain.go"), "plain.go")

    def test_limit_is_1000(self):
        self.assertEqual(check.LIMIT, 1000)


if __name__ == "__main__":
    unittest.main()
