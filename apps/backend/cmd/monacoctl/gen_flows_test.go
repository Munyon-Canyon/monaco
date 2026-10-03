package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestRenderFlowSwift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		flow flows.Flow
		want string
	}{
		{
			"codes and two crash points collapse into one interrupted case",
			flows.Flow{ID: "01", Command: "OpenSession", Outcomes: []flows.Outcome{
				"ok", "crash:before-commit", "Unauthorized", "InvalidInput", "crash:after-publish",
			}},
			flowsSwiftHeader + `
public enum Flow01Outcome: Sendable, Hashable, CaseIterable {
    case ok, unauthorized, invalidInput, interrupted

    public static let flowID = "01"
    public static let command = "OpenSession"

    public var code: String? {
        switch self {
        case .ok, .interrupted: nil
        case .unauthorized: "unauthorized"
        case .invalidInput: "invalid_input"
        }
    }

    public init?(code: String) {
        switch code {
        case "unauthorized": self = .unauthorized
        case "invalid_input": self = .invalidInput
        default: return nil
        }
    }
}
`,
		},
		{
			"only ok with a letter suffix id",
			flows.Flow{ID: "01a", Command: "Ping", Outcomes: []flows.Outcome{"ok"}},
			flowsSwiftHeader + `
public enum Flow01aOutcome: Sendable, Hashable, CaseIterable {
    case ok

    public static let flowID = "01a"
    public static let command = "Ping"

    public var code: String? {
        switch self {
        case .ok: nil
        }
    }

    public init?(code: String) {
        switch code {
        default: return nil
        }
    }
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := renderFlowSwift(tc.flow)
			if err != nil || got != tc.want {
				t.Fatalf("err=%v got:\n%s\nwant:\n%s", err, got, tc.want)
			}
		})
	}
}

func TestRenderFlowSwift_unknownCodeFails(t *testing.T) {
	t.Parallel()
	if _, err := renderFlowSwift(flows.Flow{ID: "07", Outcomes: []flows.Outcome{"ok", "NoSuchCode"}}); err == nil {
		t.Fatal("want an error for an outcome that is not an errs code name")
	}
}

func TestWriteFlowsSwift_writesOneFilePerRowAndPrunesTheRest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, flowsSwiftDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Flow99.gen.swift", "MonacoFlows.swift"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stale\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	parsed := []flows.Flow{
		{ID: "00", Outcomes: []flows.Outcome{"ok"}},
		{ID: "01a", Outcomes: []flows.Outcome{"ok"}},
	}
	if err := writeFlowsSwift(root, parsed); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"Flow00.gen.swift", "Flow01a.gen.swift", "MonacoFlows.swift"}; !slices.Equal(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}
}

func TestCommittedFlowsSwiftMatchesFlowsTSV(t *testing.T) {
	t.Parallel()
	parsed, err := readFlowsTSV(os.DirFS("../../../.."))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range parsed {
		want, err := renderFlowSwift(f)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("../../../..", flowsSwiftDir, "Flow"+f.ID+".gen.swift"))
		if err != nil || string(got) != want {
			t.Fatalf("Flow%s.gen.swift is stale or missing (%v); run go generate ./cmd/monacoctl", f.ID, err)
		}
	}
}

func TestFlowCaseName_escapesSwiftKeywords(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"InvalidInput": "invalidInput", "Default": "`default`", "Self": "`self`"} {
		if got := flowCaseName(in); got != want {
			t.Errorf("flowCaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderFlowSwift_wrapsALongCaseList(t *testing.T) {
	t.Parallel()
	f := flows.Flow{ID: "09", Outcomes: []flows.Outcome{"ok"}}
	for _, c := range errs.All()[:20] {
		f.Outcomes = append(f.Outcomes, flows.Outcome(errs.Name(c)))
	}
	got, err := renderFlowSwift(f)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(got, "\n")
	for _, line := range rows {
		if len(line) > swiftLineWidth {
			t.Fatalf("line over %d columns: %q", swiftLineWidth, line)
		}
	}
	if !strings.HasPrefix(rows[3], "    case ok, ") || !strings.HasSuffix(rows[3], ",") ||
		!strings.HasPrefix(rows[4], "        ") || strings.Contains(rows[4], ":") {
		t.Fatalf("case list did not wrap onto a continuation line:\n%s", got)
	}
}

func TestWriteFlowsSwift_reportsWriteAndPruneFailures(t *testing.T) {
	t.Parallel()
	parsed := []flows.Flow{{ID: "00", Outcomes: []flows.Outcome{"ok"}}}
	for name, blocker := range map[string]string{
		"write": "Flow00.gen.swift/x",
		"prune": "Flow99.gen.swift/x",
	} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, flowsSwiftDir, blocker), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeFlowsSwift(root, parsed); err == nil {
			t.Errorf("%s: want an error when %s is a non-empty directory", name, filepath.Dir(blocker))
		}
	}
}

func TestRunGenFlows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, tsv string
		code      int
		out       string
	}{
		{"writes the enums", flows.Header + "\n" + pingRow + "\n", 0, "wrote 1 flow outcome enums to " + flowsSwiftDir + "\n"},
		{"refuses a malformed flows.tsv", "id\n", 1, "monacoctl: monacoctl.readFlowsTSV: flows.tsv:1: header must be"},
		{
			"refuses an unknown outcome", flows.Header + "\n" + strings.Replace(pingRow, "ok;Internal", "ok;NoSuchCode", 1) + "\n",
			1, "monacoctl: monacoctl.flowOutcomes: flow 01 outcome NoSuchCode is not an errs code name",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "apps/backend"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "apps/backend/flows.tsv"), []byte(tc.tsv), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr strings.Builder
			code := runGenFlows(root, &stdout, &stderr)
			if code != tc.code || !strings.HasPrefix(stdout.String()+stderr.String(), tc.out) {
				t.Fatalf(
					"code=%d stdout=%q stderr=%q, want %d %q",
					code,
					stdout.String(),
					stderr.String(),
					tc.code,
					tc.out,
				)
			}
		})
	}
}

func TestWriteFlowsSwift_failsWithoutAWritableRoot(t *testing.T) {
	t.Parallel()
	parsed := []flows.Flow{{ID: "00", Outcomes: []flows.Outcome{"ok"}}}
	if err := writeFlowsSwift(filepath.Join(t.TempDir(), "missing"), parsed); err == nil {
		t.Error("missing root: want an error")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "packages"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFlowsSwift(root, parsed); err == nil {
		t.Error("packages is a file: want an error")
	}
}

func TestGenFlows_readsFlowsTSVTwoDirectoriesUp(t *testing.T) {
	t.Parallel()
	var stdout, stderr strings.Builder
	if code := gen([]string{"flows"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "apps/backend/flows.tsv") {
		t.Fatalf(
			"gen flows from cmd/monacoctl = %d %q, want 1 naming the repo-relative flows.tsv",
			code,
			stderr.String(),
		)
	}
}
