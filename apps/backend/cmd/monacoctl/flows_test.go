package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const pingRow = "01\tPing\tsystem\tpoller:platform.retention\tPing\tsystem.pinged\t\tok;Internal\tbuilt\tdocs/flows.md#ping"

type echoModule struct{}

func (echoModule) Name() string { return "system" }

func (echoModule) Mount(api.Mount) {}

func (echoModule) Consumers() []bus.Consumer {
	echo := bus.Handle("system.echo", func(context.Context, db.Tx, events.SystemPinged, time.Time) error { return nil })
	return []bus.Consumer{{Durable: "system_echo", Handlers: []bus.HandlerSpec{echo}}}
}

func (echoModule) Pollers() []poller.Poller { return []poller.Poller{poller.NewRetention(nil, nil)} }

func envWith(t *testing.T, tsv string) flows.Env {
	t.Helper()
	backend := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":                             "module example.com/backend\n\ngo 1.25\n",
		"internal/modules/system/app/app.go": "package app\n\ntype Ping struct{}\n\ntype RecordPing struct{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(backend, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backend, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo := flowFS(tsv)
	repo["apps/backend/internal/modules/system/app/app.go"] = &fstest.MapFile{Data: []byte("package app\n")}
	repo["docs/flows.md"] = &fstest.MapFile{Data: []byte("## Ping\n")}
	mods := module.NewSet(echoModule{})
	return liveEnv(repo, backend, mods)
}

func pass(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString(`{"Action":"pass","Package":"p","Test":"` + n + "\"}\n")
	}
	return b.String()
}

func TestFlowsCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		tsv           string
		tests         string
		structureOnly bool
		acceptScripts bool
		code          int
		stderr        string
	}{
		{"built row with every test passing", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK", "TestFlow01_Ping_Internal"), false, true, 0, ""},
		{
			"built row missing a test", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK"), false, true, 1,
			"packages/flows/backend/01.tsv:2: outcome Internal has no test TestFlow01_Ping_Internal in the go test -json input\n",
		},
		{"structure only skips the test check", flows.Header + "\n" + pingRow + "\n", "", true, true, 0, ""},
		{
			"built row needs a script for each non-crash outcome",
			flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK", "TestFlow01_Ping_Internal"), false, false, 1,
			"packages/flows/backend/01.tsv:2: built flow outcome ok has no script F01PingOK " +
				"in internal/testkit/flows; monacoctl verify all fails without it\n" +
				"packages/flows/backend/01.tsv:2: built flow outcome Internal has no script F01PingInternal " +
				"in internal/testkit/flows; monacoctl verify all fails without it\n",
		},
		{
			"verified row needs a script per outcome",
			flows.Header + "\n" + strings.Replace(pingRow, "\tbuilt\t", "\tverified\t", 1) + "\n", "", true, false, 1,
			"packages/flows/backend/01.tsv:2: verified flow outcome ok has no script F01PingOK in internal/testkit/flows for monacoctl verify all\n" +
				"packages/flows/backend/01.tsv:2: verified flow outcome Internal has no script F01PingInternal in internal/testkit/flows for " +
				"monacoctl verify all\n",
		},
		{
			"structure only still checks the columns", flows.Header + "\n" +
				strings.Replace(pingRow, "system.pinged", "system.exploded", 1) + "\n",
			"", true, true, 1,
			"packages/flows/backend/01.tsv:2: event system.exploded is not in the events registry\n",
		},
		{
			"live registry and errs table", flows.Header + "\n" +
				strings.Replace(strings.Replace(pingRow, "system.pinged", "system.pinged;system.exploded", 1), "ok;Internal\tbuilt", "ok;Internal;NoSuchCode;crash:before-commit;crash:after-lunch\tplanned", 1) + "\n",
			"", false, false, 1,
			"packages/flows/backend/01.tsv:2: event system.exploded is not in the events registry\n" +
				"packages/flows/backend/01.tsv:2: outcome NoSuchCode is not an errs code name\n" +
				"packages/flows/backend/01.tsv:2: outcome crash:after-lunch is not a registered faultpoint\n",
		},
		{
			"live routes, commands and consumers", flows.Header + "\n" +
				"02\tPong\tsystem\tPOST /v1/pong\tPong\t\tghost.durable\tok\tplanned\tdocs/flows.md#ping\n" +
				"03\tHealth\tsystem\tGET /healthz\tPing\t\t\tok\tplanned\tdocs/flows.md#ping\n" +
				"04\tPinged\tsystem\tconsumer:system.pinged\tPing\t\tsystem.echo;system_echo\tok\tplanned\tdocs/flows.md#ping\n",
			"", false, false, 1,
			"packages/flows/backend/02.tsv:2: trigger POST /v1/pong is not a route, subject or poller\n" +
				"packages/flows/backend/02.tsv:2: command Pong is not a type in internal/modules/system/app\n" +
				"packages/flows/backend/02.tsv:2: consumer ghost.durable is not a registered durable\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			env := envWith(t, tc.tsv)
			if tc.acceptScripts {
				env.Scripts = func(flows.Flow, string) bool { return true }
			}
			code := flowsCheck(env, nil, strings.NewReader(tc.tests), tc.structureOnly, nil, &stderr)
			if code != tc.code || stderr.String() != tc.stderr {
				t.Fatalf("code=%d stderr=\n%s\nwant code=%d stderr=\n%s", code, stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}

func TestFlowsCheck_missingFileFails(t *testing.T) {
	t.Parallel()
	env := envWith(t, "")
	env.Repo = fstest.MapFS{}
	var stderr bytes.Buffer
	if code := flowsCheck(
		env,
		nil,
		nil,
		false,
		nil,
		&stderr,
	); code != 1 ||
		!strings.Contains(stderr.String(), flows.Dir) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsRejectsUnknownArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"flows"}, {"flows", "lint"}, {"flows", "check", "--from"}, {"flows", "check", "-x", "f"}, {"flows", "check", "--structure-only", "x"}, {"flows", "seed", "00"}, {"flows", "check", "--from", "x", "--structure-only"}, {"flows", "--affected"}, {"flows", "--affected", "--base", "x", "--structure-only"}, {"flows", "check", "--affected"}, {"flows", "check", "--base", "staging"}, {"flows", "check", "--affected", "--base", "b", "--flows", "00"}} {
		var stdout, stderr bytes.Buffer
		code := run(commands(), tools(nil), nil, args, &stdout, &stderr)
		if code != 2 || stderr.String() != flowsUsage+"\n" {
			t.Fatalf("%q: code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestFlowsCheck_fromAMissingFileFails(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "go-test.json")
	if code := run(
		commands(),
		tools(nil),
		nil,
		[]string{"flows", "check", "--from", missing},
		&stdout,
		&stderr,
	); code != 1 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsCheck_failsWhenTheTestResultsCannotBeRead(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	code := flowsCheck(
		envWith(t, flows.Header+"\n"+pingRow+"\n"),
		nil,
		iotest.ErrReader(io.ErrUnexpectedEOF),
		false,
		nil,
		&stderr,
	)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl flows check: ") ||
		!strings.Contains(stderr.String(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsCheck_appRegistry(t *testing.T) {
	t.Parallel()
	backend := flows.Header + "\n" + pingRow + "\n" +
		"02\tPinged\tsystem\tconsumer:system.pinged\tPing\t\tsystem_echo\tok\tplanned\tdocs/flows.md#ping\n"
	app := func(row string) string { return flows.AppHeader + "\n" + row + "\n" }
	for _, tc := range []struct {
		name   string
		files  map[string]string
		code   int
		stderr string
	}{
		{"one file per flow passes", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tplanned\tdocs/flows.md#ping"),
			"packages/flows/app/02.tsv": app("02\t-\tnone\tdocs/flows.md"),
			"packages/flows/README.md":  "Flows 01 and 02.\n",
		}, 0, ""},
		{"two data rows", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tplanned\tdocs/flows.md\n01\tOther\tplanned\tdocs/flows.md"),
		}, 1, "packages/flows/app/01.tsv:3: extra data row \"01\\tOther\\tplanned\\tdocs/flows.md\"; keep exactly one row per file\n"},
		{"id with no backend row", map[string]string{
			"packages/flows/app/99.tsv": app("99\tX\tplanned\tdocs/flows.md"),
		}, 1, "packages/flows/app/99.tsv:2: id 99 has no valid row in packages/flows/backend/99.tsv; add the backend row first or delete this file\n"},
		{"unknown status", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tdone\tdocs/flows.md"),
		}, 1, "packages/flows/app/01.tsv:2: status \"done\" is not planned, built, verified or none\n"},
		{"missing anchor", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tplanned\tdocs/flows.md#pong"),
		}, 1, "packages/flows/app/01.tsv:2: doc docs/flows.md has no heading with anchor #pong\n"},
		{"aggregate file", map[string]string{
			"packages/flows/all.tsv": "id\n01\n02\n",
		}, 1, "packages/flows/all.tsv: names flows 01 and 02; keep one flow per file\n"},
		{"built without a model", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tbuilt\tdocs/flows.md#ping"),
		}, 1, "packages/flows/app/01.tsv:2: status built but no model Flow01*.swift in packages/mobile-core/Sources/MonacoSystem\n"},
		{"built with its model", map[string]string{
			"packages/flows/app/01.tsv": app("01\tSystemPing\tbuilt\tdocs/flows.md#ping"),
			"packages/mobile-core/Sources/MonacoSystem/Flow01SystemPingModel.swift": "",
		}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := envWith(t, backend)
			repo := env.Repo.(fstest.MapFS)
			for name, body := range tc.files {
				repo[name] = &fstest.MapFile{Data: []byte(body)}
			}
			env.Scripts = func(flows.Flow, string) bool { return true }
			var stderr bytes.Buffer
			code := flowsCheck(env, nil, nil, true, nil, &stderr)
			if code != tc.code || stderr.String() != tc.stderr {
				t.Fatalf("code=%d stderr=\n%s\nwant code=%d stderr=\n%s", code, stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}

func runFlowsSeed(t *testing.T, environ []string, id, outcome string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := flowsSeed(t.Context(), os.DirFS("../../../.."), environ, id, outcome, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestFlowsSeed_signedInOutcomesMintATokenForANewUser(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cfg := devNewUserConfig(t, pool, nil)
	environ := []string{
		"DATABASE_URL=" + cfg.DB.URL, "NATS_URL=" + cfg.NATS.URL, "MONACO_DEV_TOKEN_KEY=" + cfg.Auth.DevTokenKey,
		"MONACO_FAKES_URL=" + strings.TrimSuffix(cfg.Privy.BaseURL, "/privy"),
	}
	seen := map[string]bool{}
	for _, outcome := range []string{"ok", "InvalidInput", "crash:after-publish"} {
		code, stdout, stderr := runFlowsSeed(t, environ, "00", outcome)
		var got testflows.SeedResult
		if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || strings.Count(stdout, "\n") != 1 {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q err=%v", outcome, code, stdout, stderr, err)
		}
		if got.Token == "" || got.UserID == "" || got.IDs == nil || seen[got.UserID] {
			t.Fatalf("%s: seed = %+v, want a token for a fresh user and an empty ids map", outcome, got)
		}
		seen[got.UserID] = true
		assertDevTokenVerifies(t, cfg, got.Token, got.UserID)
		if status, _ := devMe(t, pool, cfg, got.Token); status != http.StatusOK {
			t.Fatalf("%s: GET /v1/me = %d, want the seeded user to exist", outcome, status)
		}
	}
}

func TestFlowsSeed_unauthorizedPrintsAnEmptyToken(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runFlowsSeed(t, nil, "00", "Unauthorized")
	if code != 0 || stdout != `{"token":"","user_id":"","ids":{}}`+"\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestFlowsSeed_refusesWhatItCannotSeed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, id, outcome string
		code              int
		stderr            string
	}{
		{
			"unknown outcome", "00", "Bogus", 2,
			"monacoctl flows seed: no outcome Bogus on flow 00; " +
				"valid outcomes: ok, InvalidInput, Unauthorized, crash:after-publish\n",
		},
		{
			"unknown flow", "99", "ok", 2,
			"monacoctl flows seed: no outcome ok on flow 99; valid outcomes: none, the flow has no packages/flows/backend/99.tsv\n",
		},
		{
			"outcome without a seeder", "01", "ok", 1,
			"monacoctl flows seed: flow 01 outcome ok has no seeder F01OpenSessionOK in internal/testkit/flows/seed.go\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runFlowsSeed(t, nil, tc.id, tc.outcome)
			if code != tc.code || stdout != "" || stderr != tc.stderr {
				t.Fatalf(
					"code=%d stdout=%q stderr=%q, want code=%d stderr=%q",
					code,
					stdout,
					stderr,
					tc.code,
					tc.stderr,
				)
			}
		})
	}
}

func xunit(cases ...string) string {
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<testsuites>\n<testsuite name=\"TestResults\">\n" +
		strings.Join(cases, "\n") + "\n</testsuite>\n</testsuites>\n"
}

func xcase(name, body string) string {
	return `<testcase classname="MonacoCoreTests.F00IntegrationTests" name="` + name + `" time="0.1">` + body + `</testcase>`
}

func TestFlowsCheck_integrationXUnit(t *testing.T) {
	t.Parallel()
	backend := flows.Header + "\n" + "00\tPing\tsystem\tpoller:platform.retention\tRecordPing\tsystem.pinged\t\t" +
		"ok;InvalidInput;Unauthorized;crash:after-publish\tbuilt\tdocs/flows.md#ping\n"
	app := func(status string) string {
		return flows.AppHeader + "\n00\tSystemPing\t" + status + "\tdocs/flows.md#ping\n"
	}
	ok := xcase("test_F00_RecordPing_ok", "")
	invalid := xcase("test_F00_RecordPing_InvalidInput", "")
	unauthorized := xcase("test_F00_RecordPing_Unauthorized", "")
	interrupted := xcase("test_F00_RecordPing_interrupted", "")
	const gap = "packages/flows/app/00.tsv:2: flow 00: app verified but F00IntegrationTests "
	for _, tc := range []struct {
		name, status, xunit string
		noXUnit             bool
		code                int
		stderr              string
	}{
		{"every outcome passed", "verified", xunit(ok, invalid, unauthorized, interrupted), false, 0, ""},
		{
			"a missing crash outcome test", "verified", xunit(ok, invalid, unauthorized), false, 1,
			gap + "lacks test_F00_RecordPing_interrupted\n",
		},
		{
			"a skipped test", "verified",
			xunit(ok, invalid, xcase("test_F00_RecordPing_Unauthorized", "<skipped/>"), interrupted), false, 1,
			gap + "skipped test_F00_RecordPing_Unauthorized\n",
		},
		{
			"a failed test", "verified",
			xunit(xcase("test_F00_RecordPing_ok", `<failure message="failed"></failure>`), invalid, unauthorized, interrupted),
			false, 1, gap + "has a failing test_F00_RecordPing_ok\n",
		},
		{
			"a test in another class", "verified",
			xunit(ok, invalid, unauthorized, strings.Replace(interrupted, "F00Integration", "SystemPingIntegration", 1)),
			false, 1, gap + "lacks test_F00_RecordPing_interrupted\n",
		},
		{"a built app flow needs no integration test", "built", xunit(), false, 0, ""},
		{
			"without the flag the check says it skipped", "verified", "", true, 0,
			"monacoctl flows check: skipped the integration tests of app verified flows 00; " +
				"pass --integration-xunit to check them\n",
		},
		{
			"without the flag a test missing from the source fails", "verified", "", true, 1,
			"monacoctl flows check: skipped the integration tests of app verified flows 00; " +
				"pass --integration-xunit to check them\n" + gap + "declares no test_F00_RecordPing_Unauthorized in packages/mobile-core/Tests\n",
		},
		{"a file that is not xml fails", "verified", "<testsuites><testcase", false, 1, "monacoctl flows check: "},
		{
			"a test case that is not xml fails", "verified", xunit(xcase("test_F00_RecordPing_ok", "<failure>")), false, 1,
			"monacoctl flows check: ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := envWith(t, backend)
			env.Repo.(fstest.MapFS)["packages/flows/app/00.tsv"] = &fstest.MapFile{Data: []byte(app(tc.status))}
			env.Repo.(fstest.MapFS)["packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift"] = &fstest.MapFile{}
			declared := "func test_F00_RecordPing_ok() {}\nfunc test_F00_RecordPing_InvalidInput() {}\n" +
				"func test_F00_RecordPing_interrupted() {}\n"
			if tc.code == 0 {
				declared += "func test_F00_RecordPing_Unauthorized() {}\n"
			}
			env.Repo.(fstest.MapFS)["packages/mobile-core/Tests/MonacoCoreTests/F00IntegrationTests.swift"] = &fstest.MapFile{
				Data: []byte(declared),
			}
			env.Scripts = func(flows.Flow, string) bool { return true }
			var integration io.Reader
			if !tc.noXUnit {
				integration = strings.NewReader(tc.xunit)
			}
			var stderr bytes.Buffer
			code := flowsCheck(env, nil, nil, true, integration, &stderr)
			if code != tc.code || !strings.HasPrefix(stderr.String(), tc.stderr) ||
				(tc.code == 0 && stderr.String() != tc.stderr) {
				t.Fatalf("code=%d stderr=\n%s\nwant code=%d stderr=\n%s", code, stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}

func TestFlowsCheck_integrationXUnitFromAMissingFileFails(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "integration.xml")
	args := []string{"flows", "check", "--structure-only", "--integration-xunit", missing}
	if code := run(commands(), tools(nil), nil, args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "integration.xml") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsCheck_readsEveryInputBeforeTheRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"go-test.json", "integration.xml"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	t.Cleanup(func() { _ = read.Close() })
	for _, args := range [][]string{
		{"--from", filepath.Join(dir, "go-test.json"), "--integration-xunit", filepath.Join(dir, "integration.xml")},
		{},
	} {
		var stderr bytes.Buffer
		if code := flowsCheckCmd(
			args,
			read,
			runCommand,
			os.DirFS("../.."),
			io.Discard,
			&stderr,
		); code != 1 ||
			!strings.Contains(stderr.String(), flows.Dir) {
			t.Fatalf("%q: code=%d stderr=%q, want the missing flow files of this test's working directory", args, code,
				stderr.String())
		}
	}
}

func TestFlowsSeed_runsFromTheBackendDirectory(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := run(commands(), tools(nil), nil, []string{"flows", "seed", "00", "ok"}, &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl flows seed: ") ||
		!strings.Contains(stderr.String(), flows.Dir) {
		t.Fatalf(
			"code=%d stderr=%q, want the missing flow files of this test's working directory",
			code,
			stderr.String(),
		)
	}
}

func TestFlowsSeed_reportsASeederThatFails(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runFlowsSeed(t, nil, "00", "ok")
	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "monacoctl flows seed: F00RecordPingOK: ") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestAffectedFlows(t *testing.T) {
	t.Parallel()
	repo := flowFS(flows.Header + "\n" +
		"00\tPing\tsystem\tGET /p\tPing\t\t\tok\tbuilt\tdocs/f.md\n" +
		"01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n" +
		"02\tCabal\tcabal\tGET /c\tCreate\t\t\tok\tbuilt\tdocs/f.md\n")
	repo["packages/flows/app/01.tsv"] = &fstest.MapFile{
		Data: []byte(flows.AppHeader + "\n01\tSignIn\tbuilt\tdocs/f.md\n"),
	}
	for _, tc := range []struct {
		name, names string
		want        []string
	}{
		{"an app row", "packages/flows/app/02.tsv\n", []string{"02"}},
		{"nothing a flow owns", "README.md\n", nil},
		{"a module", "apps/backend/internal/modules/identity/http.go\n", []string{"01"}},
		{"the spec reaches the flows whose routes changed", "apps/backend/api/openapi.yaml\n", []string{"01"}},
		{"a backend row", flows.Dir + "/00.tsv\nREADME.md\n", []string{"00"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls [][]string
			git := func(_ context.Context, _ string, _ []string, _ string, args ...string) ([]byte, error) {
				calls = append(calls, args)
				switch strings.Join(args, " ") {
				case "merge-base staging HEAD":
					return []byte("fork\n"), nil
				case "show fork:" + flows.SpecPath:
					return []byte("paths:\n  /p:\n    get: {}\n"), nil
				case "show HEAD:" + flows.SpecPath:
					return []byte("paths:\n  /p:\n    get: {}\n  /s:\n    get: {}\n"), nil
				}
				return []byte(tc.names), nil
			}
			got, err := affectedFlows(repo, git, "staging")
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("affectedFlows = %q, %v, want %q", got, err, tc.want)
			}
			if want := []string{
				"diff",
				"--name-only",
				"--diff-filter=d",
				"staging...HEAD",
			}; !slices.Equal(
				calls[0],
				want,
			) {
				t.Fatalf("git %q, want %q", calls[0], want)
			}
		})
	}
}

func TestAffectedFlows_reportsAFailingDiff(t *testing.T) {
	t.Parallel()
	git := func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, io.ErrUnexpectedEOF
	}
	if _, err := affectedFlows(fstest.MapFS{}, git, "staging"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v", err)
	}
}

func TestAffectedFlows_reportsAFailingMergeBase(t *testing.T) {
	t.Parallel()
	repo := flowFS(flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\t\tok\tbuilt\tdocs/f.md\n")
	git := func(_ context.Context, _ string, _ []string, _ string, args ...string) ([]byte, error) {
		if args[0] == "merge-base" {
			return nil, io.ErrUnexpectedEOF
		}
		return []byte(flows.SpecPath + "\n"), nil
	}
	if _, err := affectedFlows(repo, git, "staging"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v", err)
	}
}

func TestFlowsAffectedCommands(t *testing.T) {
	t.Parallel()
	repo := flowFS(flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\t\tok\tbuilt\tdocs/f.md\n")
	names := func(out string, err error) execFunc {
		return func(context.Context, string, []string, string, ...string) ([]byte, error) {
			return []byte(out), err
		}
	}
	for _, tc := range []struct {
		name   string
		args   []string
		run    execFunc
		repo   fs.FS
		code   int
		stdout string
		stderr string
	}{
		{"ids", []string{"--affected", "--base", "b"}, names("packages/flows/app/00.tsv\n", nil), repo, 0, "00\n", ""},
		{
			"a failing diff",
			[]string{"--affected", "--base", "b"},
			names("", io.ErrUnexpectedEOF), repo, 1, "",
			"monacoctl flows: " + io.ErrUnexpectedEOF.Error() + "\n",
		},
		{
			"no registry",
			[]string{"--affected", "--base", "b"},
			names("README.md\n", nil),
			fstest.MapFS{},
			1, "",
			"monacoctl flows: ",
		},
		{
			"check with no affected flow",
			[]string{"check", "--affected", "--base", "b"},
			names("README.md\n", nil), repo, 0,
			"", "monacoctl flows check: no affected flows\n",
		},
		{
			"check with a failing diff",
			[]string{"check", "--affected", "--base", "b"},
			names("", io.ErrUnexpectedEOF), repo,
			1, "", "monacoctl flows check: " + io.ErrUnexpectedEOF.Error() + "\n",
		},
		{
			"check only the listed flows",
			[]string{"check", "--flows", "00,01", "--structure-only"},
			names("", io.ErrUnexpectedEOF), repo, 1, "", "",
		},
		{
			"check prints the ids it checks",
			[]string{"check", "--affected", "--base", "b", "--structure-only"},
			names("packages/flows/app/00.tsv\n", nil), repo, 1, "00\n", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := flowsCmd(nil, tc.args, tc.run, tc.repo, &stdout, &stderr)
			if code != tc.code || stdout.String() != tc.stdout || !strings.HasPrefix(stderr.String(), tc.stderr) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestFlowsCrashPoints(t *testing.T) {
	t.Parallel()
	all := []flows.Flow{
		{
			ID:       "30",
			Status:   flows.StatusVerified,
			Commands: []string{"Mute"},
			Outcomes: []flows.Outcome{"ok", "crash:before-commit"},
		},
		{
			ID:       "31",
			Status:   flows.StatusBuilt,
			Commands: []string{"Pin", "Unpin"},
			Outcomes: []flows.Outcome{"crash:after-sign", "crash:after-publish"},
		},
		{
			ID:       "32",
			Status:   flows.StatusBuilt,
			Commands: []string{"Star"},
			Outcomes: []flows.Outcome{"crash:after-publish", "crash:after-create"},
		},
		{
			ID:       "33",
			Status:   flows.StatusPlanned,
			Commands: []string{"Plan"},
			Outcomes: []flows.Outcome{"crash:after-execute"},
		},
	}
	scripts := map[string]bool{}
	for _, f := range all {
		for _, command := range f.Commands {
			for _, o := range f.Outcomes {
				scripts[flows.ScriptName(f, command, o)] = true
			}
		}
	}
	delete(scripts, flows.ScriptName(all[2], "Star", "crash:after-create"))
	delete(scripts, flows.ScriptName(all[1], "Pin", "crash:after-sign"))
	got := crashPoints(all, func(name string) bool { return scripts[name] })
	if want := "after-publish,after-sign,before-commit"; strings.Join(got, ",") != want {
		t.Fatalf("crashPoints = %v, want %s", got, want)
	}
}

func TestFlowsCrashPoints_printsEachPointOfTheRepoOrFailsOnABadRepo(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := flowsCrashPoints(os.DirFS("../../../.."), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	points := strings.Fields(stdout.String())
	if len(points) == 0 || !slices.IsSorted(points) {
		t.Fatalf("flows crash-points = %q, want sorted points", stdout.String())
	}
	for _, p := range points {
		if !faultpoint.Known(p) {
			t.Errorf("flows crash-points printed %q, not a faultpoint", p)
		}
	}
	stderr.Reset()
	if code := run(commands(), tools(nil), nil, []string{"flows", "crash-points"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl flows crash-points") {
		t.Fatalf("crash-points without flow files: code=%d stderr=%q", code, stderr.String())
	}
}

func TestReadFlowsTSV_namesTheFirstProblem(t *testing.T) {
	t.Parallel()
	if _, err := readFlowsTSV(flowFS(flows.Header + "\n01\tPing\n")); err == nil ||
		!strings.Contains(err.Error(), "monacoctl.readFlowsTSV: "+flows.Dir+"/01.tsv:2: has 2 columns") {
		t.Fatalf("readFlowsTSV = %v, want the first problem", err)
	}
}
