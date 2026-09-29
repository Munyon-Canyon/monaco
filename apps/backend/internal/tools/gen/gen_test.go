package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const changelog = "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- Old line.\n\n## [0.1.0]\n\n### Added\n\n- Released.\n"

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestModule_writesTheSkeletonAndAnUnreleasedChangelogLine(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"go.mod": "module example.com/app\n\ngo 1.25\n", "CHANGELOG.md": changelog})
	touched, err := gen.Apply(root, "module", "wallets")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/modules/wallets/adapters/adapters.go",
		"internal/modules/wallets/app/app.go",
		"internal/modules/wallets/domain/domain.go",
		"internal/modules/wallets/main_test.go",
		"internal/modules/wallets/module.go",
		"queries/wallets/.gitkeep",
		"CHANGELOG.md",
	}
	if strings.Join(touched, "\n") != strings.Join(want, "\n") {
		t.Fatalf("touched\n%s\nwant\n%s", strings.Join(touched, "\n"), strings.Join(want, "\n"))
	}
	if got := read(
		t,
		root,
		"CHANGELOG.md",
	); got != strings.Replace(
		changelog,
		"- Old line.",
		"- The `wallets` module.\n- Old line.",
		1,
	) {
		t.Fatalf("CHANGELOG.md =\n%s", got)
	}
	mod := read(t, root, "internal/modules/wallets/module.go")
	for _, want := range []string{"package wallets\n", `"example.com/app/internal/platform/module"`, `return "wallets"`} {
		if !strings.Contains(mod, want) {
			t.Errorf("module.go lacks %q:\n%s", want, mod)
		}
	}
	if got := read(t, root, "internal/modules/wallets/main_test.go"); !strings.Contains(got, "testkit.Main(m)") {
		t.Errorf("main_test.go does not run testkit.Main:\n%s", got)
	}
}

func TestModule_refusesToOverwriteAndLeavesTheTreeUntouched(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{
		"go.mod":                             "module example.com/app\n",
		"CHANGELOG.md":                       changelog,
		"internal/modules/wallets/module.go": "package wallets\n",
	})
	if _, err := gen.Apply(root, "module", "wallets"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already exists", err)
	}
	if got := read(t, root, "CHANGELOG.md"); got != changelog {
		t.Fatalf("a refused generate edited CHANGELOG.md:\n%s", got)
	}
}

func TestModule_rejectsBadInput(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		files map[string]string
		args  []string
		want  string
	}{
		"upper case name": {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": changelog}, []string{"Wallets"}, "must match"},
		"underscore":      {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": changelog}, []string{"my_wallets"}, "must match"},
		"no module line":  {map[string]string{"go.mod": "go 1.25\n", "CHANGELOG.md": changelog}, []string{"wallets"}, "no module line"},
		"no unreleased":   {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": "# Changelog\n"}, []string{"wallets"}, "Unreleased"},
		"no added list": {
			map[string]string{"go.mod": "module m\n", "CHANGELOG.md": "## [Unreleased]\n\n## [0.1.0]\n\n### Added\n\n"},
			[]string{"wallets"},
			"no ### Added",
		},
		"wrong arity": {map[string]string{"go.mod": "module m\n"}, nil, "usage: gen module <name>"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := tree(t, tc.files)
			if _, err := gen.Apply(root, "module", tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(root, "internal")); !os.IsNotExist(err) {
				t.Fatalf("a rejected generate wrote files: %v", err)
			}
		})
	}
}

func withModule(t *testing.T) string {
	t.Helper()
	root := tree(t, map[string]string{"go.mod": "module example.com/app\n", "CHANGELOG.md": changelog})
	if _, err := gen.Apply(root, "module", "wallets"); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCommandAndQuery_writeIntoTheModuleUnderSnakeCaseNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind, name string
		want       []string
	}{
		{"command", "OpenHTTPWallet", []string{
			"internal/modules/wallets/app/open_http_wallet.go",
			"internal/modules/wallets/open_http_wallet_test.go",
		}},
		{"query", "GetWallet", []string{
			"internal/modules/wallets/app/get_wallet_query.go",
			"queries/wallets/get_wallet.sql",
		}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			root := withModule(t)
			touched, err := gen.Apply(root, tc.kind, "wallets", tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(touched, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("touched %v, want %v", touched, tc.want)
			}
		})
	}
}

func TestCommand_carriesAnIdempotencyKeyAndAppendsInAUnitOfWork(t *testing.T) {
	t.Parallel()
	root := withModule(t)
	if _, err := gen.Apply(root, "command", "wallets", "OpenWallet"); err != nil {
		t.Fatal(err)
	}
	src := read(t, root, "internal/modules/wallets/app/open_wallet.go")
	for _, want := range []string{
		"IdempotencyKey string",
		"func (h *OpenWalletHandler) Handle(ctx context.Context, cmd OpenWallet) (OpenWalletResult, error)",
		"h.uow.Do(ctx,",
		"tx.Events.Append(ctx,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("open_wallet.go lacks %q:\n%s", want, src)
		}
	}
	test := read(t, root, "internal/modules/wallets/open_wallet_test.go")
	if !strings.Contains(test, "testkit.DB(t)") || !strings.Contains(test, `t.Fatal("not implemented")`) {
		t.Errorf("open_wallet_test.go does not start from testkit.DB and fail as not implemented:\n%s", test)
	}
}

func TestQuery_namesTheSqlcQueryItCalls(t *testing.T) {
	t.Parallel()
	root := withModule(t)
	if _, err := gen.Apply(root, "query", "wallets", "GetWallet"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "queries/wallets/get_wallet.sql"); !strings.HasPrefix(got, "-- name: GetWallet :one\n") {
		t.Errorf("get_wallet.sql =\n%s", got)
	}
	if got := read(
		t,
		root,
		"internal/modules/wallets/app/get_wallet_query.go",
	); !strings.Contains(
		got,
		"sqlc.New(q).GetWallet(ctx)",
	) {
		t.Errorf("get_wallet_query.go does not call the sqlc query:\n%s", got)
	}
}

func TestModuleScopedGenerators_rejectBadNamesAndMissingModules(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"command", "query", "consumer"} {
		for name, tc := range map[string]struct{ module, name, want string }{
			"bad name":       {"wallets", "open-wallet", "must match"},
			"bad module":     {"Wallets", "OpenWallet", "must match"},
			"missing module": {"ledger", "OpenWallet", "run gen module ledger first"},
		} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				t.Parallel()
				name := tc.name
				if kind == "consumer" {
					name = strings.ToLower(name[:1]) + name[1:]
				}
				_, err := gen.Apply(withModule(t), kind, tc.module, name)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("err = %v, want %q", err, tc.want)
				}
			})
		}
	}
}

func TestConsumer_registersItsHandlerInModuleGoAndStartsNATSForTheSuite(t *testing.T) {
	t.Parallel()
	root := withModule(t)
	touched, err := gen.Apply(root, "consumer", "wallets", "onDeposit")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/modules/wallets/adapters/on_deposit.go",
		"internal/modules/wallets/on_deposit_test.go",
		"internal/modules/wallets/main_test.go",
		"internal/modules/wallets/module.go",
	}
	if strings.Join(touched, "\n") != strings.Join(want, "\n") {
		t.Fatalf("touched %v, want %v", touched, want)
	}
	mod := read(t, root, "internal/modules/wallets/module.go")
	for _, want := range []string{
		`"example.com/app/internal/modules/wallets/adapters"`,
		"func New(d module.Deps) *Module { return &Module{deps: d} }",
		`Durable: "wallets_on_deposit",`,
		`bus.Handle("wallets.on_deposit", adapters.OnDeposit{Bus: m.deps.Bus}.Handle),`,
	} {
		if !strings.Contains(mod, want) {
			t.Errorf("module.go lacks %q:\n%s", want, mod)
		}
	}
	if got := read(
		t,
		root,
		"internal/modules/wallets/main_test.go",
	); !strings.Contains(
		got,
		"testkit.Main(m, testkit.WithNATS())",
	) {
		t.Errorf("main_test.go does not start NATS:\n%s", got)
	}
	if got := read(t, root, "internal/modules/wallets/on_deposit_test.go"); !strings.Contains(
		got,
		"testkit.ConsumerSuite(t,",
	) || !strings.Contains(got, "Bus: h.Bus}") {
		t.Errorf("on_deposit_test.go does not run the consumer suite on the harness bus:\n%s", got)
	}

	if _, err := gen.Apply(root, "consumer", "wallets", "onWithdrawal"); err != nil {
		t.Fatal(err)
	}
	mod = read(t, root, "internal/modules/wallets/module.go")
	if strings.Count(mod, "adapters.On") != 2 || strings.Count(mod, "/adapters\"") != 1 {
		t.Errorf("a second consumer did not append once and reuse the import:\n%s", mod)
	}
	if got := read(t, root, "internal/modules/wallets/main_test.go"); strings.Count(got, "WithNATS") != 1 {
		t.Errorf("a second consumer changed main_test.go again:\n%s", got)
	}
}

func TestConsumer_refusesAHandEditedConsumersMethod(t *testing.T) {
	t.Parallel()
	root := withModule(t)
	path := filepath.Join(root, "internal/modules/wallets/module.go")
	edited := strings.Replace(read(t, root, "internal/modules/wallets/module.go"),
		"return []bus.Consumer{}", "out := []bus.Consumer{}\n\treturn out", 1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.Apply(root, "consumer", "wallets", "onDeposit"); err == nil ||
		!strings.Contains(err.Error(), "Consumers must be a single return of a slice literal") {
		t.Fatalf("err = %v, want the Consumers shape named", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal/modules/wallets/adapters/on_deposit.go")); !os.IsNotExist(err) {
		t.Fatalf("a refused consumer wrote its handler: %v", err)
	}
}

func TestProvider_writesAClientAPortFakeAndAFakesFixture(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"go.mod": "module example.com/app\n"})
	touched, err := gen.Apply(root, "provider", "quotes")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/providers/quotes/client.go",
		"internal/providers/quotes/client_test.go",
		"internal/providers/quotes/main_test.go",
		"internal/providers/quotes/quotesfake/fake.go",
		"internal/testkit/fakes/testdata/fakes/quotes/_health.json",
		"internal/testkit/fakes/testdata/fakes/quotes/things/t1.json",
	}
	if strings.Join(touched, "\n") != strings.Join(want, "\n") {
		t.Fatalf("touched %v, want %v", touched, want)
	}
	if got := read(
		t,
		root,
		"internal/providers/quotes/quotesfake/fake.go",
	); !strings.Contains(
		got,
		"\ttestkit.Faults\n",
	) {
		t.Errorf("the port fake does not embed testkit.Faults:\n%s", got)
	}
	if got := read(t, root, "internal/providers/quotes/client.go"); !strings.Contains(got, "type thingWire struct") {
		t.Errorf("the client has no unexported wire type:\n%s", got)
	}
	if _, err := gen.Apply(root, "provider", "Quotes"); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("err = %v, want must match", err)
	}
}

const flowsHeader = "id\tflow\tmodule\ttrigger\tcommand\tevents\tconsumers\toutcomes\tstatus\tdoc\n"

func withFlows(t *testing.T, rows string) string {
	t.Helper()
	root := withModule(t)
	if err := os.WriteFile(filepath.Join(root, "flows.tsv"), []byte(flowsHeader+rows), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestFlow_writesOneNotImplementedTestPerOutcome(t *testing.T) {
	t.Parallel()
	root := withFlows(
		t,
		"7\tOpen a wallet\twallets\tPOST /v1/wallets\tOpenWallet\t\t\tok;invalid_input;crash:before-commit\tplanned\tdocs/x.md\n",
	)
	touched, err := gen.Apply(root, "flow", "7")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{
		"internal/modules/wallets/flow7_test.go",
	}; strings.Join(
		touched,
		"\n",
	) != strings.Join(
		want,
		"\n",
	) {
		t.Fatalf("touched %v, want %v", touched, want)
	}
	src := read(t, root, "internal/modules/wallets/flow7_test.go")
	for _, name := range []string{"TestFlow7_OpenWallet_OK", "TestFlow7_OpenWallet_invalid_input", "TestFlow7_OpenWallet_CrashBeforeCommit"} {
		if !strings.Contains(src, "func "+name+"(t *testing.T) {\n\tt.Parallel()\n\tt.Fatal(\"not implemented\")\n}") {
			t.Errorf("flow7_test.go lacks a not implemented %s:\n%s", name, src)
		}
	}
	if got := strings.Count(src, "func Test"); got != 3 {
		t.Errorf("flow7_test.go has %d tests, want 3", got)
	}
}

func TestFlow_rejectsRowsItCannotNameTestsFor(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ rows, id, want string }{
		"unknown id":     {"", "7", `no valid row with id "7"`},
		"invalid row":    {"7\tOpen\twallets\t\tOpenWallet\t\t\t\tplanned\tdocs/x.md\n", "7", `no valid row with id "7" (1 problems)`},
		"no command":     {"7\tOpen\twallets\t\t\t\t\tok\tplanned\tdocs/x.md\n", "7", "has no command"},
		"missing module": {"7\tOpen\tledger\t\tPost\t\t\tok\tplanned\tdocs/x.md\n", "7", "run gen module ledger first"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := withFlows(t, tc.rows)
			if _, err := gen.Apply(root, "flow", tc.id); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFlow_needsFlowsTSV(t *testing.T) {
	t.Parallel()
	if _, err := gen.Apply(withModule(t), "flow", "1"); err == nil || !strings.Contains(err.Error(), "flows.tsv") {
		t.Fatalf("err = %v, want flows.tsv named", err)
	}
}

func TestCheck_panicsNamingTheTemplateBug(t *testing.T) {
	t.Parallel()
	gen.Check("fine", nil)
	defer func() {
		if got := recover(); got != "gen: template x: boom" {
			t.Fatalf("recover = %v", got)
		}
	}()
	gen.Check("template x", boomError("boom"))
}

type boomError string

func (p boomError) Error() string { return string(p) }
