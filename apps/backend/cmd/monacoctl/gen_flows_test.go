package main

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

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
			flows.Flow{ID: "01", Commands: []string{"OpenSession", "RefreshSession"}, Outcomes: []flows.Outcome{
				"ok", "crash:before-commit", "Unauthorized", "InvalidInput", "crash:after-publish",
			}},
			flowsSwiftHeader + `
public enum Flow01Outcome: Sendable, Hashable, CaseIterable {
    case ok, unauthorized, invalidInput, interrupted

    public static let flowID = "01"
    public static let commands: [String] = ["OpenSession", "RefreshSession"]

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
			"only ok with a letter suffix id and one command",
			flows.Flow{ID: "01a", Commands: []string{"Ping"}, Outcomes: []flows.Outcome{"ok"}},
			flowsSwiftHeader + `
public enum Flow01aOutcome: Sendable, Hashable, CaseIterable {
    case ok

    public static let flowID = "01a"
    public static let commands: [String] = ["Ping"]

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
			outcomes, err := flowOutcomes(tc.flow)
			if got := renderFlowSwift(tc.flow, outcomes); err != nil || got != tc.want {
				t.Fatalf("err=%v got:\n%s\nwant:\n%s", err, got, tc.want)
			}
		})
	}
}

func TestFlowOutcomes_unknownCodeFails(t *testing.T) {
	t.Parallel()
	if _, err := flowOutcomes(flows.Flow{ID: "07", Outcomes: []flows.Outcome{"ok", "NoSuchCode"}}); err == nil {
		t.Fatal("want an error for an outcome that is not an errs code name")
	}
}

func TestWriteFlowFiles_writesEveryFileAndPrunesStaleFlowFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, flowsSwiftDir)
	writeTree(t, root, map[string]string{
		path.Join(flowsSwiftDir, "Flow99.gen.swift"):          "stale\n",
		path.Join(flowsSwiftDir, "Flow99Scenarios.gen.swift"): "stale\n",
		path.Join(flowsSwiftDir, "MonacoFlows.swift"):         "kept\n",
	})
	files := map[string]string{
		path.Join(flowsSwiftDir, "Flow00.gen.swift"):          "a\n",
		path.Join(flowsSwiftDir, "Flow00Scenarios.gen.swift"): "b\n",
		path.Join(flowsSwiftDir, "Flow01a.gen.swift"):         "c\n",
		scenarioManifest: "d\n",
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts/qa"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeFlowFiles(root, files); err != nil {
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
	want := []string{"Flow00.gen.swift", "Flow00Scenarios.gen.swift", "Flow01a.gen.swift", "MonacoFlows.swift"}
	if !slices.Equal(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}
	if body, err := fs.ReadFile(os.DirFS(root), scenarioManifest); err != nil || string(body) != "d\n" {
		t.Fatalf("manifest = %q, %v", body, err)
	}
}

func TestCommittedFlowFilesMatchTheGenerator(t *testing.T) {
	t.Parallel()
	repo := os.DirFS("../../../..")
	files, err := renderFlows(repo)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		if got, err := fs.ReadFile(repo, name); err != nil || string(got) != want {
			t.Errorf("%s is stale or missing (%v); run go generate ./cmd/monacoctl", name, err)
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
	outcomes, err := flowOutcomes(f)
	if err != nil {
		t.Fatal(err)
	}
	got := renderFlowSwift(f, outcomes)
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

func TestWriteFlowFiles_reportsWriteAndPruneFailures(t *testing.T) {
	t.Parallel()
	files := map[string]string{path.Join(flowsSwiftDir, "Flow00.gen.swift"): "a\n"}
	for name, blocker := range map[string]string{
		"write": "Flow00.gen.swift/x",
		"prune": "Flow99.gen.swift/x",
	} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, flowsSwiftDir, blocker), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeFlowFiles(root, files); err == nil {
			t.Errorf("%s: want an error when %s is a non-empty directory", name, filepath.Dir(blocker))
		}
	}
}

func TestRunGenFlows(t *testing.T) {
	t.Parallel()
	tsv := flows.Header + "\n" + pingRow + "\n"
	manifest := "home  -MonacoHomeSample\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		code  int
		out   string
	}{
		{
			"writes the enums", nil,
			0, "wrote 1 flow files to " + flowsSwiftDir + " and the " + scenarioManifest + " scenario block\n",
		},
		{
			"refuses a malformed flows.tsv",
			map[string]string{"apps/backend/flows.tsv": "id\n"},
			1, "monacoctl: monacoctl.readFlowsTSV: flows.tsv:1: header must be",
		},
		{
			"refuses an unknown outcome",
			map[string]string{"apps/backend/flows.tsv": strings.Replace(tsv, "ok;Internal", "ok;NoSuchCode", 1)},
			1, "monacoctl: monacoctl.flowOutcomes: flow 01 outcome NoSuchCode is not an errs code name",
		},
		{
			"refuses a broken app registry",
			map[string]string{"packages/flows/app/01.tsv": "id\n"},
			1, "monacoctl: monacoctl.readBuiltFlows: packages/flows/app/01.tsv:1",
		},
		{
			"refuses a missing manifest",
			map[string]string{scenarioManifest: ""},
			1, "monacoctl: monacoctl.renderFlows",
		},
		{
			"refuses an unpaired marker",
			map[string]string{scenarioManifest: scenarioBlockBegin + "\n"},
			1, "monacoctl: monacoctl.spliceScenarioBlock",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			files := map[string]string{"apps/backend/flows.tsv": tsv, scenarioManifest: manifest}
			maps.Copy(files, tc.files)
			if files[scenarioManifest] == "" {
				delete(files, scenarioManifest)
			}
			writeTree(t, root, files)
			var stdout, stderr strings.Builder
			code := runGenFlows(root, &stdout, &stderr)
			if code != tc.code || !strings.HasPrefix(stdout.String()+stderr.String(), tc.out) {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %d %q",
					code, stdout.String(), stderr.String(), tc.code, tc.out)
			}
		})
	}
}

func TestWriteFlowFiles_failsWithoutAWritableRoot(t *testing.T) {
	t.Parallel()
	files := map[string]string{path.Join(flowsSwiftDir, "Flow00.gen.swift"): "a\n"}
	if err := writeFlowFiles(filepath.Join(t.TempDir(), "missing"), files); err == nil {
		t.Error("missing root: want an error")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "packages"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFlowFiles(root, files); err == nil {
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

func TestRenderFlowScenariosSwift(t *testing.T) {
	t.Parallel()
	scenarios := []swiftOutcome{{name: "invalidInput"}, {name: "unauthorized"}, {name: "interrupted"}}
	want := scenariosHeader + `
public enum Flow00Scenario: String, CaseIterable, Sendable {
    case invalidInput, unauthorized, interrupted

    /// The scenario that ` + "`-MonacoFlow 00 <case>`" + ` names in the launch arguments.
    public static func matching(_ arguments: [String]) -> Flow00Scenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoFlow"),
            arguments.indices.contains(flag + 2),
            arguments[flag + 1] == "00"
        else { return nil }
        return Flow00Scenario(rawValue: arguments[flag + 2])
    }
}
`
	if got := renderFlowScenariosSwift("00", scenarios); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderFlows_writesScenariosOnlyForBuiltFlowsInFlowsTSVOrder(t *testing.T) {
	t.Parallel()
	row := func(id, outcomes string) string {
		return strings.Replace(strings.Replace(pingRow, "01\t", id+"\t", 1), "ok;Internal", outcomes, 1)
	}
	app := func(id, status string) *fstest.MapFile {
		return &fstest.MapFile{
			Data: []byte(flows.AppHeader + "\n" + id + "\tScreen\t" + status + "\tdocs/flows.md#ping\n"),
		}
	}
	repo := fstest.MapFS{
		"apps/backend/flows.tsv": {Data: []byte(flows.Header + "\n" + strings.Join([]string{
			row("00", "ok;InvalidInput;crash:after-publish"),
			row("01", "ok;Unauthorized"),
			row("05", "ok"),
			row("23a", "ok;PhotoInvalid;StorageUnavailable"),
		}, "\n") + "\n")},
		"packages/flows/app/00.tsv":  app("00", "built"),
		"packages/flows/app/01.tsv":  app("01", "planned"),
		"packages/flows/app/05.tsv":  app("05", "verified"),
		"packages/flows/app/23a.tsv": app("23a", "verified"),
		scenarioManifest: {
			Data: []byte("a  -A\n\n" + scenarioBlockBegin + "\nold\n" + scenarioBlockEnd + "\n\nb  -B\n"),
		},
	}
	files, err := renderFlows(repo)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, path.Base(name))
	}
	slices.Sort(names)
	want := []string{
		"Flow00.gen.swift", "Flow00Scenarios.gen.swift", "Flow01.gen.swift", "Flow05.gen.swift",
		"Flow23a.gen.swift", "Flow23aScenarios.gen.swift", "sample-screens.txt",
	}
	if !slices.Equal(names, want) {
		t.Errorf("files = %q, want %q", names, want)
	}
	lines := strings.Split(files[scenarioManifest], "\n")
	wantLines := []string{
		"a  -A", "", scenarioBlockBegin, lines[3],
		"flow-00-invalid-input         -MonacoFlow 00 invalidInput",
		"flow-00-interrupted           -MonacoFlow 00 interrupted",
		"flow-23a-photo-invalid        -MonacoFlow 23a photoInvalid",
		"flow-23a-storage-unavailable  -MonacoFlow 23a storageUnavailable",
		scenarioBlockEnd, "", "b  -B", "",
	}
	if !slices.Equal(lines, wantLines) || !strings.HasPrefix(lines[3], "# ") {
		t.Errorf("manifest:\n%s\nwant:\n%s", files[scenarioManifest], strings.Join(wantLines, "\n"))
	}
}

func TestSpliceScenarioBlock(t *testing.T) {
	t.Parallel()
	block := scenarioBlockBegin + "\nflow-00-x  -MonacoFlow 00 x\n" + scenarioBlockEnd + "\n"
	for _, tc := range []struct{ name, manifest, want string }{
		{"empty manifest", "", block},
		{"appends after the hand lines", "home  -MonacoHomeSample\n", "home  -MonacoHomeSample\n\n" + block},
		{
			"replaces only the block",
			"a  -A\n\n" + scenarioBlockBegin + "\nhand edit\n" + scenarioBlockEnd + "\n\nb  -B\n",
			"a  -A\n\n" + block + "\nb  -B\n",
		},
	} {
		got, err := spliceScenarioBlock(tc.manifest, block)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v, want %q", tc.name, got, err, tc.want)
		}
		if again, _ := spliceScenarioBlock(got, block); again != got {
			t.Errorf("%s: a second splice changed the manifest to %q", tc.name, again)
		}
	}
	for _, broken := range []string{scenarioBlockBegin + "\n", scenarioBlockEnd + "\n" + scenarioBlockBegin + "\n"} {
		if _, err := spliceScenarioBlock(broken, block); err == nil {
			t.Errorf("manifest %q: want an error for an unpaired marker", broken)
		}
	}
}

func TestRunGenFlows_writesScenariosForBuiltFlowsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ping := "00\tPing\tsystem\tPOST /v1/system/pings\tRecordPing\tsystem.pinged\t\tok;InvalidInput;Unauthorized;crash:after-publish\tverified\tdocs/flows.md#ping"
	stale := filepath.Join(root, flowsSwiftDir, "Flow07Scenarios.gen.swift")
	writeTree(t, root, map[string]string{
		"apps/backend/flows.tsv":    flows.Header + "\n" + ping + "\n",
		"packages/flows/app/00.tsv": flows.AppHeader + "\n00\tSystemPing\tbuilt\tdocs/flows.md#ping\n",
		scenarioManifest:            "# screens\nhome  -MonacoHomeSample populated\n",
		filepath.Join(flowsSwiftDir, "Flow07Scenarios.gen.swift"): "stale\n",
	})
	read := func() string {
		var all strings.Builder
		for _, name := range []string{scenarioManifest, filepath.Join(flowsSwiftDir, "Flow00Scenarios.gen.swift")} {
			body, err := fs.ReadFile(os.DirFS(root), name)
			if err != nil {
				t.Fatal(err)
			}
			all.Write(body)
		}
		return all.String()
	}
	var stdout, stderr strings.Builder
	if code := runGenFlows(root, &stdout, &stderr); code != 0 {
		t.Fatalf("first run = %d %s", code, stderr.String())
	}
	first := read()
	for _, want := range []string{
		"# screens\nhome  -MonacoHomeSample populated\n\n" + scenarioBlockBegin,
		"flow-00-invalid-input         -MonacoFlow 00 invalidInput\n",
		"flow-00-unauthorized          -MonacoFlow 00 unauthorized\n",
		"flow-00-interrupted           -MonacoFlow 00 interrupted\n",
		"public enum Flow00Scenario: String, CaseIterable, Sendable {",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("output lacks %q:\n%s", want, first)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("Flow07Scenarios.gen.swift survived with no flow 07 (%v)", err)
	}
	if code := runGenFlows(root, &stdout, &stderr); code != 0 || read() != first {
		t.Fatalf("second run = %d changed the output:\n%s", code, read())
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
