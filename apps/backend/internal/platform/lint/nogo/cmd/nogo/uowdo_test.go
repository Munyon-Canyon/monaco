package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const uowdoModule = "github.com/monaco/monaco/apps/backend/internal/"

func uowdoTree(caller string) map[string]string {
	imports := "import (\n\t\"context\"\n\t\"log/slog\"\n\n" +
		"\t\"" + uowdoModule + "platform/bus\"\n" +
		"\t\"" + uowdoModule + "platform/chain/relayer/sqlc\"\n" +
		"\t\"" + uowdoModule + "platform/chain/solana\"\n" +
		"\t\"" + uowdoModule + "platform/observability/boundary\"\n" +
		"\t\"net/http\"\n" +
		"\t\"" + uowdoModule + "platform/db\"\n" +
		"\t\"" + uowdoModule + "platform/httpclient\"\n" +
		"\t\"" + uowdoModule + "platform/observability\"\n)\n\n" +
		"var _ = slog.Info\nvar _ = bus.Conn{}\nvar _ = solana.Client{}\nvar _ = httpclient.Client{}\nvar _ = observability.Info\nvar _ = sqlc.New\nvar _ = boundary.Warn\nvar _ = http.Client{}\n\n"
	return map[string]string{
		"go.mod": "module github.com/monaco/monaco/apps/backend\n\ngo 1.25\n",
		"internal/platform/db/db.go": "package db\n\nimport \"context\"\n\ntype UnitOfWork struct{}\n\ntype Tx struct{}\n\n" +
			"func (Tx) AfterCommit(fn func(context.Context)) {}\n\n" +
			"func (*UnitOfWork) Do(ctx context.Context, fn func(context.Context, Tx) error) error { return fn(ctx, Tx{}) }\n",
		"internal/platform/chain/solana/solana.go": "package solana\n\nimport \"context\"\n\ntype Client struct{}\n\n" +
			"func (Client) Send(context.Context) error { return nil }\n\nfunc (Client) Name() string { return \"\" }\n\n" +
			"type Port interface{ Ping(context.Context) error }\n",
		"internal/platform/chain/relayer/sqlc/sqlc.go": "package sqlc\n\ntype DBTX interface{}\n\nfunc New() int { return 0 }\n",
		"internal/platform/chain/solana/intx.go": "package solana\n\nimport (\n\t\"context\"\n\n\t\"" + uowdoModule + "platform/chain/relayer/sqlc\"\n)\n\n" +
			"func InTx(context.Context, sqlc.DBTX) error { return nil }\n",
		"internal/platform/chain/solana/own.go": "package solana\n\nimport (\n\t\"context\"\n\n\t\"" + uowdoModule + "platform/db\"\n)\n\n" +
			"func own(ctx context.Context, uow *db.UnitOfWork) {\n\t_ = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return Client{}.Send(ctx) })\n}\n",
		"internal/platform/httpclient/httpclient.go": "package httpclient\n\nimport \"context\"\n\ntype Client struct{}\n\n" +
			"func (Client) Do(context.Context) error { return nil }\n",
		"internal/platform/bus/bus.go": "package bus\n\nimport \"context\"\n\ntype Conn struct{}\n\n" +
			"func (Conn) Publish(context.Context, string) error { return nil }\n",
		"internal/platform/observability/boundary/boundary.go": "package boundary\n\nimport \"context\"\n\nfunc Warn(context.Context, string) {}\n",
		"internal/platform/observability/obs.go": "package observability\n\nimport \"context\"\n\n" +
			"func Info(context.Context, string) {}\n\nfunc Debug(context.Context, string) {}\n",
		"internal/modules/referrals/app/attach_referral.go": "package app\n\nimport (\n\t\"context\"\n\n\t\"" + uowdoModule + "platform/db\"\n" +
			"\t\"" + uowdoModule + "platform/observability\"\n)\n\n" +
			"func Attach(ctx context.Context, uow *db.UnitOfWork) {\n\t_ = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {\n" +
			"\t\tobservability.Info(ctx, \"x\")\n\t\treturn nil\n\t})\n}\n",
		"internal/modules/m/m.go": "package m\n\n" + imports + caller,
	}
}

func TestUowdo_reportsOutboundCallsInfoLogsAndUnwrappedPublishesInsideUowDo(t *testing.T) {
	t.Parallel()
	tail := "\nfunc named(ctx context.Context, tx db.Tx) error { _ = solana.Client{}.Send(ctx); return nil }\n\n" +
		"func G(ctx context.Context, uow *db.UnitOfWork) { _ = uow.Do(ctx, named) }\n"
	const (
		chainMsg   = "outbound chain or relayer call; run it before or after the transaction"
		httpMsg    = "HTTP call; run it before or after the transaction"
		logMsg     = "log at Info or above; log at Debug, or log after the commit"
		publishMsg = "publish not wrapped in tx.AfterCommit; append the event with tx.Events or publish after commit"
	)
	cases := []struct {
		name, stmt, call, msg string
	}{
		{"chain call", "_ = solana.Client{}.Send(ctx)", "solana.Client{}.Send", chainMsg},
		{
			"chain call after commit",
			"tx.AfterCommit(func(ctx context.Context) { _ = solana.Client{}.Send(ctx) })",
			"",
			"",
		},
		{"chain call that takes the transaction handle", "_ = solana.InTx(ctx, nil)", "", ""},
		{"chain package query helper", "_ = sqlc.New()", "", ""},
		{"http client call", "_ = httpclient.Client{}.Do(ctx)", "httpclient.Client{}.Do", httpMsg},
		{"net/http client call", "_, _ = (&http.Client{}).Get(\"u\")", "(&http.Client{}).Get", httpMsg},
		{
			"http call after commit",
			"tx.AfterCommit(func(ctx context.Context) { _ = httpclient.Client{}.Do(ctx) })",
			"",
			"",
		},
		{"info log", "observability.Info(ctx, \"x\")", "observability.Info", logMsg},
		{"slog warn", "slog.Warn(\"x\")", "slog.Warn", logMsg},
		{"boundary warn", "boundary.Warn(ctx, \"x\")", "boundary.Warn", logMsg},
		{"debug log", "observability.Debug(ctx, \"x\")", "", ""},
		{"publish", "_ = bus.Conn{}.Publish(ctx, \"s\")", "bus.Conn{}.Publish", publishMsg},
		{
			"publish after commit",
			"tx.AfterCommit(func(ctx context.Context) { _ = bus.Conn{}.Publish(ctx, \"s\") })",
			"",
			"",
		},
		{"call of a function value", "f := func() {}\n\t\tf()\n\t\tvar e error\n\t\t_ = e.Error()", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "func F(ctx context.Context, uow *db.UnitOfWork) {\n\t_ = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {\n\t\t" +
				tc.stmt + "\n\t\treturn nil\n\t})\n}\n" + tail
			dir := writeTree(t, uowdoTree(src))
			code, stderr := nogoStderr(t, dir, "-uowdo")
			var want string
			wantCode := 0
			if tc.msg != "" {
				file := filepath.Join(dir, "internal/modules/m/m.go")
				whole, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				offset := strings.Index(string(whole), "uow.Do(ctx, func")
				offset += strings.Index(string(whole[offset:]), tc.call)
				line := 1 + strings.Count(string(whole[:offset]), "\n")
				col := offset - strings.LastIndex(string(whole[:offset]), "\n")
				want = fmt.Sprintf("%s:%d:%d: %s inside uow.Do: %s", file, line, col, tc.call, tc.msg)
				wantCode = 3
			}
			if got := strings.TrimSpace(stderr); code != wantCode || got != want {
				t.Fatalf("nogo = %d\n%s\nwant %d and\n%s", code, got, wantCode, want)
			}
		})
	}
}

func TestUowdo_followsSameFileHelpersAndNamesThem(t *testing.T) {
	t.Parallel()
	const chainMsg = "outbound chain or relayer call; run it before or after the transaction"
	cases := []struct {
		name, helpers, stmt, want string
	}{
		{
			"helper that calls the chain",
			"func fetch(ctx context.Context) error { return solana.Client{}.Send(ctx) }\n",
			"_ = fetch(ctx)",
			"solana.Client{}.Send inside uow.Do: " + chainMsg + " (via fetch)",
		},
		{
			"helper of a helper",
			"func outer(ctx context.Context) error { return inner(ctx) }\n\n" +
				"func inner(ctx context.Context) error { return solana.Client{}.Send(ctx) }\n",
			"_ = outer(ctx)",
			"solana.Client{}.Send inside uow.Do: " + chainMsg + " (via outer)",
		},
		{
			"helper method that logs at Info",
			"type svc struct{}\n\nfunc (svc) note(ctx context.Context) { observability.Info(ctx, \"x\") }\n",
			"svc{}.note(ctx)",
			"observability.Info inside uow.Do: log at Info or above; log at Debug, or log after the commit (via note)",
		},
		{
			"helper that calls an interface declared in another package",
			"func ask(ctx context.Context, p solana.Port) error { return p.Ping(ctx) }\n",
			"_ = ask(ctx, nil)",
			"",
		},
		{
			"mutually recursive helpers",
			"func ping(ctx context.Context) error { return pong(ctx) }\n\n" +
				"func pong(ctx context.Context) error { _ = solana.Client{}.Send(ctx); return ping(ctx) }\n",
			"_ = ping(ctx)",
			"solana.Client{}.Send inside uow.Do: " + chainMsg + " (via ping)",
		},
		{
			"helper that only debug-logs",
			"func quiet(ctx context.Context) { observability.Debug(ctx, \"x\") }\n",
			"quiet(ctx)",
			"",
		},
		{
			"helper that touches the transaction handle",
			"func store(ctx context.Context, tx sqlc.DBTX) error { return solana.InTx(ctx, tx) }\n",
			"_ = store(ctx, nil)",
			"",
		},
		{
			"helper that calls the chain after commit",
			"func later(ctx context.Context, tx db.Tx) {\n\ttx.AfterCommit(func(ctx context.Context) { _ = solana.Client{}.Send(ctx) })\n}\n",
			"later(ctx, tx)",
			"",
		},
		{
			"helper called after commit",
			"func fetch(ctx context.Context) error { return solana.Client{}.Send(ctx) }\n",
			"tx.AfterCommit(func(ctx context.Context) { _ = fetch(ctx) })",
			"",
		},
		{
			"helper that makes a pure chain call without a context",
			"func addr() string { return solana.Client{}.Name() }\n",
			"_ = addr()",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "func F(ctx context.Context, uow *db.UnitOfWork) {\n\t_ = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {\n\t\t" +
				tc.stmt + "\n\t\treturn nil\n\t})\n}\n\n" + tc.helpers
			dir := writeTree(t, uowdoTree(src))
			code, stderr := nogoStderr(t, dir, "-uowdo")
			want, wantCode := "", 0
			if tc.want != "" {
				want, wantCode = viaWant(t, dir, tc.stmt, tc.want), 3
			}
			if got := strings.TrimSpace(stderr); code != wantCode || got != want {
				t.Fatalf("nogo = %d\n%s\nwant %d and\n%s", code, got, wantCode, want)
			}
		})
	}
}

func viaWant(t *testing.T, dir, stmt, msg string) string {
	t.Helper()
	file := filepath.Join(dir, "internal/modules/m/m.go")
	whole, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(string(whole), "uow.Do(ctx, func")
	offset += strings.Index(string(whole[offset:]), stmt)
	if strings.HasPrefix(stmt, "_ = ") {
		offset += len("_ = ")
	}
	line := 1 + strings.Count(string(whole[:offset]), "\n")
	col := offset - strings.LastIndex(string(whole[:offset]), "\n")
	return fmt.Sprintf("%s:%d:%d: %s", file, line, col, msg)
}

func TestUowdo_countsAnInterfaceCallInsideTheChainPackageItsHelperIsIn(t *testing.T) {
	t.Parallel()
	tree := uowdoTree("var _ context.Context\nvar _ db.Tx\n")
	tree["internal/platform/chain/relayer/relayer.go"] = "package relayer\n\nimport (\n\t\"context\"\n\n\t\"" + uowdoModule + "platform/db\"\n)\n\n" +
		"type Port interface{ Ping(context.Context) error }\n\ntype T struct{ p Port }\n\n" +
		"func (t T) F(ctx context.Context, uow *db.UnitOfWork) {\n\t_ = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {\n\t\treturn t.ask(ctx)\n\t})\n}\n\n" +
		"func (t T) ask(ctx context.Context) error { return t.p.Ping(ctx) }\n"
	dir := writeTree(t, tree)

	code, stderr := nogoStderr(t, dir, "-uowdo")

	file := filepath.Join(dir, "internal/platform/chain/relayer/relayer.go")
	want := file + ":15:10: t.p.Ping inside uow.Do: outbound chain or relayer call; run it before or after the transaction (via ask)"
	if got := strings.TrimSpace(stderr); code != 3 || got != want {
		t.Fatalf("nogo = %d\n%s\nwant 3 and\n%s", code, got, want)
	}
}
