package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const swiftTestTree = "packages/mobile-core/Tests/"

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFlakeTestsSwift_printsOneFilterPerModuleForTheDeclaredTypes(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		swiftTestTree + "MonacoCoreTests/FormatterTests.swift": "import XCTest\n\n" +
			"final class FormatterTests: XCTestCase {\n    func testA() {}\n}\n\n" +
			"@MainActor\nfinal class PagerTests: XCTestCase, @unchecked Sendable {\n    func testB() {}\n}\n\n" +
			"// class CommentedTests: XCTestCase {}\n" +
			"class BaseTests {}\n",
		swiftTestTree + "MonacoCoreTests/SuiteTests.swift": "import Testing\n\n" +
			"@Suite struct OneLine {\n    @Test func a() {}\n}\n\n" +
			"@Suite(\"Display name\", .serialized)\nstruct NextLine {}\n\n" +
			"@Suite(.serialized) @MainActor final class Attributed {}\n\n" +
			"@Suite\n@MainActor\nstruct TwoAttributes {}\n\n" +
			"struct NotASuite {}\n",
		swiftTestTree + "MonacoCoreTests/Helpers.swift":     "import Foundation\n\nstruct Fixture {}\n",
		swiftTestTree + "MonacoCoreTests/Free.swift":        "import Testing\n\n@Test func freeFunction() {}\n",
		swiftTestTree + "MonacoAPITests/ClientTests.swift":  "import XCTest\n\nfinal class ClientTests: XCTestCase {}\nfinal class ClientTests: XCTestCase {}\n",
		swiftTestTree + "MonacoCoreTests/Fixtures/a.json":   "{}\n",
		"packages/mobile-core/Sources/MonacoCore/Foo.swift": "final class FooTests: XCTestCase {}\n",
		"apps/mobile/MonacoTests/AppTests.swift":            "final class AppTests: XCTestCase {}\n",
	})
	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests-swift.sh")
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{
			"an XCTestCase subclass",
			[]string{swiftTestTree + "MonacoCoreTests/FormatterTests.swift"},
			`swift test --filter 'MonacoCoreTests\.(FormatterTests|PagerTests)'`,
		},
		{
			"a @Suite type",
			[]string{swiftTestTree + "MonacoCoreTests/SuiteTests.swift"},
			`swift test --filter 'MonacoCoreTests\.(OneLine|NextLine|Attributed|TwoAttributes)'`,
		},
		{
			"a deleted file",
			[]string{swiftTestTree + "MonacoCoreTests/DeletedTests.swift"},
			"",
		},
		{
			"a file outside the test tree",
			[]string{"packages/mobile-core/Sources/MonacoCore/Foo.swift", "apps/mobile/MonacoTests/AppTests.swift", "./" + swiftTestTree + "MonacoCoreTests/Fixtures/a.json"},
			"",
		},
		{
			"a helper and free test functions",
			[]string{swiftTestTree + "MonacoCoreTests/Helpers.swift", swiftTestTree + "MonacoCoreTests/Free.swift"},
			"",
		},
		{
			"two modules, a repeated class and an absolute path",
			[]string{filepath.Join(root, swiftTestTree+"MonacoCoreTests/FormatterTests.swift"), "./" + swiftTestTree + "MonacoAPITests/ClientTests.swift", swiftTestTree + "MonacoCoreTests/FormatterTests.swift"},
			"swift test --filter 'MonacoCoreTests\\.(FormatterTests|PagerTests)'\nswift test --filter 'MonacoAPITests\\.(ClientTests)'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", append([]string{script, "--print", "--root", root}, tc.files...)...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("flake-tests-swift.sh %v: %v\n%s", tc.files, err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Errorf("flake-tests-swift.sh %v printed\n%s\nwant\n%s", tc.files, got, tc.want)
			}
		})
	}
}

func TestFlakeTestsSwift_baseSelectsChangedAndAddedTestFilesOnly(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	class := func(name string) string { return "final class " + name + ": XCTestCase {}\n" }
	git("init", "-q")
	writeTree(t, root, map[string]string{
		swiftTestTree + "MonacoCoreTests/UntouchedTests.swift": class("UntouchedTests"),
		swiftTestTree + "MonacoCoreTests/ChangedTests.swift":   class("ChangedTests"),
		swiftTestTree + "MonacoCoreTests/GoneTests.swift":      class("GoneTests"),
	})
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	writeTree(t, root, map[string]string{
		swiftTestTree + "MonacoCoreTests/ChangedTests.swift": class("ChangedTests") + class("MoreChangedTests"),
		swiftTestTree + "MonacoCoreTests/AddedTests.swift":   class("AddedTests"),
		"packages/mobile-core/Sources/MonacoCore/Foo.swift":  class("FooTests"),
	})
	if err := os.Remove(filepath.Join(root, swiftTestTree, "MonacoCoreTests/GoneTests.swift")); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "head")

	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests-swift.sh")
	out, err := exec.Command("bash", script, "--print", "--root", root, "--base", "HEAD~1").CombinedOutput()
	if err != nil {
		t.Fatalf("flake-tests-swift.sh --base: %v\n%s", err, out)
	}
	want := `swift test --filter 'MonacoCoreTests\.(AddedTests|ChangedTests|MoreChangedTests)'`
	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("--base printed\n%s\nwant\n%s", got, want)
	}
}

func TestFlakeTestsSwift_runsTenTimesAndNamesTheFirstRedIteration(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		swiftTestTree + "MonacoCoreTests/FlakyTests.swift": "final class FlakyTests: XCTestCase {}\n",
	})
	bin := t.TempDir()
	counter := filepath.Join(bin, "count")
	shim := "#!/bin/sh\necho x >> \"$COUNT_FILE\"\n" +
		"n=$(($(wc -l < \"$COUNT_FILE\")))\n" +
		"[ -z \"$FAIL_ON\" ] || [ \"$n\" != \"$FAIL_ON\" ] || exit 1\n"
	writeExecutable(t, filepath.Join(bin, "swift"), shim)
	if err := os.MkdirAll(filepath.Join(root, "packages", "mobile-core"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests-swift.sh")
	file := swiftTestTree + "MonacoCoreTests/FlakyTests.swift"
	for _, tc := range []struct {
		name   string
		failOn string
		runs   int
		red    string
	}{
		{"green", "", 10, ""},
		{"red on the third run", "3", 3, "red on run 3 of 10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(counter, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script, "--root", root, file)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "COUNT_FILE="+counter, "FAIL_ON="+tc.failOn)
			out, err := cmd.CombinedOutput()
			if tc.red == "" && err != nil {
				t.Fatalf("green loop failed: %v\n%s", err, out)
			}
			if tc.red != "" && (err == nil || !strings.Contains(string(out), tc.red)) {
				t.Fatalf("want a failure naming %q, got err %v\n%s", tc.red, err, out)
			}
			data, _ := os.ReadFile(counter)
			if got := strings.Count(string(data), "x"); got != tc.runs {
				t.Errorf("swift ran %d times, want %d\n%s", got, tc.runs, out)
			}
		})
	}
}
