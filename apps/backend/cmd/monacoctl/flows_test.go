package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const pingRow = "01\tPing\tsystem\tpoller:platform.retention\tPing\tsystem.pinged\t\tok;Internal\tbuilt\tdocs/flows.md#ping"

type echoModule struct{}

func (echoModule) Name() string { return "system" }

func (echoModule) Routes(*httpx.Routes) {}

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
		"internal/modules/system/app/app.go": "package app\n\ntype Ping struct{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(backend, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backend, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo := fstest.MapFS{
		"apps/backend/flows.tsv":                          {Data: []byte(tsv)},
		"apps/backend/internal/modules/system/app/app.go": {Data: []byte("package app\n")},
		"docs/flows.md":                                   {Data: []byte("## Ping\n")},
	}
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
		{"header only", flows.Header + "\n", "", false, false, 0, ""},
		{"built row with every test passing", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK", "TestFlow01_Ping_Internal"), false, true, 0, ""},
		{
			"built row missing a test", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK"), false, true, 1,
			"flows.tsv:2: outcome Internal has no test TestFlow01_Ping_Internal in the go test -json input\n",
		},
		{"structure only skips the test check", flows.Header + "\n" + pingRow + "\n", "", true, true, 0, ""},
		{
			"built row needs a script for each non-crash outcome",
			flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK", "TestFlow01_Ping_Internal"), false, false, 1,
			"flows.tsv:2: built flow outcome ok has no script F01PingOK " +
				"in internal/testkit/flows; monacoctl verify all fails without it\n" +
				"flows.tsv:2: built flow outcome Internal has no script F01PingInternal " +
				"in internal/testkit/flows; monacoctl verify all fails without it\n",
		},
		{
			"verified row needs a script per outcome",
			flows.Header + "\n" + strings.Replace(pingRow, "\tbuilt\t", "\tverified\t", 1) + "\n", "", true, false, 1,
			"flows.tsv:2: verified flow outcome ok has no script F01PingOK in internal/testkit/flows for monacoctl verify all\n" +
				"flows.tsv:2: verified flow outcome Internal has no script F01PingInternal in internal/testkit/flows for " +
				"monacoctl verify all\n",
		},
		{
			"structure only still checks the columns", flows.Header + "\n" +
				strings.Replace(pingRow, "system.pinged", "system.exploded", 1) + "\n",
			"", true, true, 1,
			"flows.tsv:2: event system.exploded is not in the events registry\n",
		},
		{
			"live registry and errs table", flows.Header + "\n" +
				strings.Replace(strings.Replace(pingRow, "system.pinged", "system.pinged;system.exploded", 1), "ok;Internal\tbuilt", "ok;Internal;NoSuchCode;crash:before-commit;crash:after-lunch\tplanned", 1) + "\n",
			"", false, false, 1,
			"flows.tsv:2: event system.exploded is not in the events registry\n" +
				"flows.tsv:2: outcome NoSuchCode is not an errs code name\n" +
				"flows.tsv:2: outcome crash:after-lunch is not a registered faultpoint\n",
		},
		{
			"live routes, commands and consumers", flows.Header + "\n" +
				"02\tPong\tsystem\tPOST /v1/pong\tPong\t\tghost.durable\tok\tplanned\tdocs/flows.md#ping\n" +
				"03\tHealth\tsystem\tGET /healthz\tPing\t\t\tok\tplanned\tdocs/flows.md#ping\n" +
				"04\tPinged\tsystem\tconsumer:system.pinged\tPing\t\tsystem.echo;system_echo\tok\tplanned\tdocs/flows.md#ping\n",
			"", false, false, 1,
			"flows.tsv:2: trigger POST /v1/pong is not a route, subject or poller\n" +
				"flows.tsv:2: command Pong is not a type in internal/modules/system/app\n" +
				"flows.tsv:2: consumer ghost.durable is not a registered durable\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			env := envWith(t, tc.tsv)
			if tc.acceptScripts {
				env.Scripts = func(flows.Flow, string) bool { return true }
			}
			code := flowsCheck(env, strings.NewReader(tc.tests), tc.structureOnly, &stderr)
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
	if code := flowsCheck(env, nil, false, &stderr); code != 1 || !strings.Contains(stderr.String(), "flows.tsv") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsRejectsUnknownArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"flows"}, {"flows", "lint"}, {"flows", "check", "--from"}, {"flows", "check", "-x", "f"}, {"flows", "check", "--structure-only", "x"}} {
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
	code := flowsCheck(envWith(t, flows.Header+"\n"), iotest.ErrReader(io.ErrUnexpectedEOF), false, &stderr)
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
		{"id not in flows.tsv", map[string]string{
			"packages/flows/app/99.tsv": app("99\tX\tplanned\tdocs/flows.md"),
		}, 1, "packages/flows/app/99.tsv:2: id 99 is not in apps/backend/flows.tsv; add the backend row first or delete this file\n"},
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
			code := flowsCheck(env, nil, true, &stderr)
			if code != tc.code || stderr.String() != tc.stderr {
				t.Fatalf("code=%d stderr=\n%s\nwant code=%d stderr=\n%s", code, stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}
