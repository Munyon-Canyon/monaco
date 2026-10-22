from __future__ import annotations

import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    "harvest_confessions", pathlib.Path(__file__).with_name("harvest_confessions.py")
)
harvest = importlib.util.module_from_spec(spec)
spec.loader.exec_module(harvest)


def body(came_up: str, focus: str = "") -> str:
    return f"## TLDR\n\nx\n\n## What came up\n\n{came_up}\n\n## Reviewer focus\n\n{focus}\n\n## Needs from Logan\n\nNothing.\n"


def categories(text: str) -> list[str]:
    return [harvest.classify(t) for _, t in harvest.parse_items(body(text))]


class ClassifyTest(unittest.TestCase):
    def test_fixtures_moved_off_a_touches_path(self):
        self.assertEqual(
            categories("- I moved the fixtures to `testdata/` because the old path is outside Touches."),
            ["edits outside Touches"],
        )

    def test_ratchet_count_growing(self):
        self.assertEqual(
            categories("- The ratchet count grew by one and failed stage 0 on the first push."),
            ["gate failed on first push"],
        )

    def test_price_floor(self):
        self.assertEqual(
            categories("- The price floor rejects a quote below 1 cent."),
            ["money and price correctness"],
        )

    def test_unmatched_text_falls_back_to_other(self):
        self.assertEqual(categories("- Chose the lighter green."), ["other"])

    def test_returns_exactly_one_known_category(self):
        known = {name for name, _ in harvest.CATEGORIES} | {harvest.FALLBACK}
        text = "price floor and ratchet and Touches and a race"
        self.assertIn(harvest.classify(text), known)
        self.assertIsInstance(harvest.classify(text), str)


class ParseTest(unittest.TestCase):
    def test_body_without_what_came_up_yields_no_came_up_items(self):
        items = harvest.parse_items("## TLDR\n\nx\n\n## Reviewer focus\n\n- look here\n")
        self.assertEqual(items, [("reviewer_focus", "look here")])
        self.assertEqual(harvest.parse_items("## TLDR\n\nx\n"), [])
        self.assertEqual(harvest.parse_items(""), [])

    def test_nested_bullets_fold_into_their_parent(self):
        items = harvest.parse_items(body("- parent\n  - child one\n  - child two\n- sibling"))
        self.assertEqual([t for _, t in items], ["parent - child one - child two", "sibling"])

    def test_continuation_lines_fold_and_prose_is_not_an_item(self):
        items = harvest.parse_items(body("Nothing came up.", "- first line\n  second line"))
        self.assertEqual(items, [("reviewer_focus", "first line second line")])

    def test_heading_inside_a_code_fence_does_not_end_the_section(self):
        items = harvest.parse_items(body("- a\n  ```\n  ## Not a heading\n  ```\n- b"))
        self.assertEqual(len(items), 2)

    def test_render_table_counts_items_and_prs(self):
        rows = [
            {"pr": 2, "kind": "came_up", "category": "other", "text": "a"},
            {"pr": 2, "kind": "came_up", "category": "other", "text": "b"},
            {"pr": 1, "kind": "came_up", "category": "process", "text": "c"},
        ]
        table = harvest.render_table(rows).splitlines()
        self.assertEqual(table[2], "| other | 2 | 1 |")
        self.assertEqual(table[-1], "| Total | 3 | 2 |")


if __name__ == "__main__":
    unittest.main()
