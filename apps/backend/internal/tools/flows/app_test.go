package flows_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func appBackend() []flows.Flow { return []flows.Flow{{ID: "00"}, {ID: "01"}, {ID: "01a"}} }

func appRepo(files map[string]string) fstest.MapFS {
	repo := fstest.MapFS{"docs/flows.md": {Data: []byte("# Flows\n\n## Fund\n")}}
	for name, body := range files {
		repo[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return repo
}

func appFile(rows ...string) string { return tsvWith(flows.AppHeader, rows...) }

func tsvWith(header string, rows ...string) string {
	return strings.Join(append([]string{header}, rows...), "\n") + "\n"
}

func checkApp(repo fstest.MapFS) []string {
	env := flows.Env{Repo: repo, BackendDir: "apps/backend"}
	rows, problems := flows.ReadApp(repo)
	problems = append(problems, flows.CheckApp(rows, appBackend(), env)...)
	return lines(append(problems, flows.CheckNoAggregate(repo, appBackend())...))
}

func TestCheckApp_validRegistriesPass(t *testing.T) {
	t.Parallel()
	got := checkApp(appRepo(map[string]string{
		"packages/flows/app/00.tsv":  appFile("00\t-\tnone\tdocs/flows.md#fund"),
		"packages/flows/app/01.tsv":  appFile("01\tSignIn\tplanned\tdocs/flows.md"),
		"packages/flows/app/01a.tsv": appFile("01a\tSignIn\tverified\tdocs/flows.md#fund"),
		"packages/flows/README.md":   "Flows 00, 01 and 01a each get one file.\n",
		"packages/flows/.build/x":    "00 01\n",
		"packages/flows/Package.swift": "// swift-tools-version: 6.2\n" +
			"platforms: [.macOS(.v15), .iOS(.v18)]\n",
	}))
	if len(got) != 0 {
		t.Fatalf("problems = %q", got)
	}
}

func TestCheckApp_eachViolationFailsWithOneLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, body, want string
	}{
		{
			"two data rows", "00.tsv", appFile("00\tPing\tplanned\tdocs/flows.md", "00\tSignIn\tplanned\tdocs/flows.md"),
			"packages/flows/app/00.tsv:3: extra data row \"00\\tSignIn\\tplanned\\tdocs/flows.md\"; keep exactly one row per file",
		},
		{
			"bad header", "00.tsv", tsvWith("id\tscreen\tstatus", "00\tPing\tplanned\tdocs/flows.md"),
			"packages/flows/app/00.tsv:1: header must be \"id\\tscreen\\tstatus\\tdoc\"",
		},
		{"no data row", "00.tsv", appFile(), "packages/flows/app/00.tsv:1: has no data row; add one row after the header"},
		{"short row", "00.tsv", appFile("00\tPing\tplanned"), "packages/flows/app/00.tsv:2: has 3 columns, want 4"},
		{
			"id differs from the file name", "00.tsv", appFile("01\tPing\tplanned\tdocs/flows.md"),
			"packages/flows/app/00.tsv:2: id 01 differs from the file name; rename the file to 01.tsv or set the id to 00",
		},
		{
			"id not in the backend", "99.tsv", appFile("99\tX\tplanned\tdocs/flows.md"),
			"packages/flows/app/99.tsv:2: id 99 has no valid row in packages/flows/backend/99.tsv; add the backend row first or delete this file",
		},
		{
			"unknown status", "00.tsv", appFile("00\tPing\tdone\tdocs/flows.md"),
			"packages/flows/app/00.tsv:2: status \"done\" is not planned, built, verified or none",
		},
		{
			"none with a screen", "00.tsv", appFile("00\tPing\tnone\tdocs/flows.md"),
			"packages/flows/app/00.tsv:2: screen \"Ping\" must be - on a none flow, which has no app surface",
		},
		{
			"planned without a screen", "00.tsv", appFile("00\t-\tplanned\tdocs/flows.md"),
			"packages/flows/app/00.tsv:2: screen is empty on a planned flow; name the screen or set status none",
		},
		{
			"missing doc file", "00.tsv", appFile("00\tPing\tplanned\tdocs/nope.md#fund"),
			"packages/flows/app/00.tsv:2: doc docs/nope.md does not exist",
		},
		{
			"missing anchor", "00.tsv", appFile("00\tPing\tplanned\tdocs/flows.md#vote"),
			"packages/flows/app/00.tsv:2: doc docs/flows.md has no heading with anchor #vote",
		},
		{"empty doc", "00.tsv", appFile("00\tPing\tplanned\t"), "packages/flows/app/00.tsv:2: doc is empty; point it at path#anchor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkApp(appRepo(map[string]string{"packages/flows/app/" + tc.file: tc.body}))
			if !slices.Equal(got, []string{tc.want}) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckNoAggregate_failsAFileNamingTwoFlows(t *testing.T) {
	t.Parallel()
	got := checkApp(appRepo(map[string]string{
		"packages/flows/all.tsv":   "id\n00\n01\n00\n",
		"packages/flows/one.swift": "let flowID = \"01a\" // Flow01Outcome\n",
	}))
	want := []string{"packages/flows/all.tsv: names flows 00 and 01; keep one flow per file"}
	if !slices.Equal(got, want) {
		t.Fatalf("problems = %q, want %q", got, want)
	}
}

func TestCheckAppModels_requiresABuiltRowsModelInItsModuleTarget(t *testing.T) {
	t.Parallel()
	backend := []flows.Flow{{ID: "01", Module: "identity"}, {ID: "02", Module: "system"}}
	row := func(id string, status flows.AppStatus) flows.AppRow {
		return flows.AppRow{File: "packages/flows/app/" + id + ".tsv", Line: 2, ID: id, Status: status}
	}
	for _, tc := range []struct {
		name   string
		row    flows.AppRow
		models []string
		want   []string
	}{
		{
			"built with its model", row("01", flows.AppBuilt),
			[]string{"packages/mobile-core/Sources/MonacoIdentity/Flow01SignInModel.swift"},
			nil,
		},
		{
			"verified with its model", row("02", flows.AppVerified),
			[]string{"packages/mobile-core/Sources/MonacoSystem/Flow02PingModel.swift"},
			nil,
		},
		{"planned without a model", row("01", flows.AppPlanned), nil, nil},
		{"none without a model", row("01", flows.AppNone), nil, nil},
		{"built without a model", row("01", flows.AppBuilt), nil, []string{
			"packages/flows/app/01.tsv:2: status built but no model Flow01*.swift in packages/mobile-core/Sources/MonacoIdentity",
		}},
		{
			"verified with the model in another module", row("01", flows.AppVerified),
			[]string{"packages/mobile-core/Sources/MonacoSystem/Flow01SignInModel.swift"},
			[]string{
				"packages/flows/app/01.tsv:2: status verified but no model Flow01*.swift in packages/mobile-core/Sources/MonacoIdentity",
			},
		},
		{
			"built with another flow's model", row("01", flows.AppBuilt),
			[]string{"packages/mobile-core/Sources/MonacoIdentity/Flow02PingModel.swift"},
			[]string{
				"packages/flows/app/01.tsv:2: status built but no model Flow01*.swift in packages/mobile-core/Sources/MonacoIdentity",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := fstest.MapFS{}
			for _, m := range tc.models {
				repo[m] = &fstest.MapFile{}
			}
			got := lines(flows.CheckAppModels([]flows.AppRow{tc.row}, backend, flows.Env{Repo: repo}))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAppStatus_atLeastBuilt(t *testing.T) {
	t.Parallel()
	for status, want := range map[flows.AppStatus]bool{
		flows.AppPlanned: false, flows.AppNone: false, flows.AppBuilt: true, flows.AppVerified: true,
	} {
		if got := status.AtLeastBuilt(); got != want {
			t.Errorf("%s.AtLeastBuilt() = %v, want %v", status, got, want)
		}
	}
}

type unreadable struct {
	fstest.MapFS
	name string
}

func (u unreadable) ReadFile(name string) ([]byte, error) {
	if name == u.name {
		return nil, fs.ErrPermission
	}
	return u.MapFS.ReadFile(name)
}

func TestReadApp_andCheckNoAggregate_reportAnUnreadableFile(t *testing.T) {
	t.Parallel()
	repo := unreadable{
		appRepo(map[string]string{"packages/flows/app/00.tsv": appFile("00\tPing\tplanned\tdocs/flows.md")}),
		"",
	}
	repo.name = "packages/flows/app/00.tsv"
	_, problems := flows.ReadApp(repo)
	want := []string{"packages/flows/app/00.tsv: read: permission denied"}
	if got := lines(problems); !slices.Equal(got, want) {
		t.Fatalf("ReadApp problems = %q, want %q", got, want)
	}
	if got := lines(flows.CheckNoAggregate(repo, appBackend())); !slices.Equal(got, want) {
		t.Fatalf("CheckNoAggregate problems = %q, want %q", got, want)
	}
}

func TestCheckNoAggregate_passesARepoWithNoFlowsPackage(t *testing.T) {
	t.Parallel()
	if got := flows.CheckNoAggregate(fstest.MapFS{"README.md": {Data: []byte("x\n")}}, nil); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}
