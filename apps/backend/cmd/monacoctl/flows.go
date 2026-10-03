package main

import (
	"cmp"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	backendDir = "apps/backend"
	flowsUsage = "usage: monacoctl flows check [--from go-test.json | --structure-only] [--integration-xunit swift-xunit.xml]\n" +
		"                             [--affected --base <ref>]\n" +
		"       monacoctl flows --affected --base <ref>\n" +
		"       monacoctl flows seed <id> <outcome>"
)

func flowsCmd(environ, args []string, run execFunc, repo fs.FS, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 3 && args[0] == "seed":
		return flowsSeed(context.Background(), repo, environ, args[1], args[2], stdout, stderr)
	case len(args) > 0 && args[0] == "--affected":
		flags, ok := parseCheckFlags(args)
		if !ok || flags != (checkFlags{affected: true, base: flags.base}) {
			_, _ = fmt.Fprintln(stderr, flowsUsage)
			return 2
		}
		ids, err := affectedFlows(repo, run, flags.base)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows: %v\n", err)
			return 1
		}
		printIDs(stdout, ids)
		return 0
	case len(args) > 0 && args[0] == "check":
		return flowsCheckCmd(args[1:], os.Stdin, run, repo, stdout, stderr)
	default:
		_, _ = fmt.Fprintln(stderr, flowsUsage)
		return 2
	}
}

type checkFlags struct {
	from, xunit, base       string
	structureOnly, affected bool
}

func parseCheckFlags(args []string) (checkFlags, bool) {
	var f checkFlags
	set := flag.NewFlagSet("flows check", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&f.from, "from", "", "go test -json output")
	set.BoolVar(&f.structureOnly, "structure-only", false, "skip the go test result check")
	set.StringVar(&f.xunit, "integration-xunit", "", "swift test --xunit-output file")
	set.BoolVar(&f.affected, "affected", false, "check only the flows the diff against --base touches")
	set.StringVar(&f.base, "base", "", "the ref --affected diffs against")
	err := set.Parse(args)
	return f, err == nil && set.NArg() == 0 && (f.from == "" || !f.structureOnly) && f.affected == (f.base != "")
}

func flowsCheckCmd(args []string, stdin *os.File, run execFunc, repo fs.FS, stdout, stderr io.Writer) int {
	flags, ok := parseCheckFlags(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, flowsUsage)
		return 2
	}
	var ids []string
	if flags.affected {
		var code int
		if ids, code = checkedIDs(repo, run, flags.base, stdout, stderr); len(ids) == 0 {
			return code
		}
	}
	var tests, integration io.Reader
	if flags.from == "" && !flags.structureOnly {
		if info, err := stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
			tests = stdin
		}
	}
	for name, into := range map[string]*io.Reader{flags.from: &tests, flags.xunit: &integration} {
		if name == "" {
			continue
		}
		file, err := os.Open(filepath.Clean(name))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
		defer func() { _ = file.Close() }()
		*into = file
	}
	env := liveEnv(repo, ".", registered.Build(declaringDeps()))
	return flowsCheck(env, ids, tests, flags.structureOnly, integration, stderr)
}

func checkedIDs(repo fs.FS, run execFunc, base string, stdout, stderr io.Writer) ([]string, int) {
	ids, err := affectedFlows(repo, run, base)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
		return nil, 1
	}
	printIDs(stdout, ids)
	if len(ids) == 0 {
		_, _ = fmt.Fprintln(stderr, "monacoctl flows check: no affected flows")
	}
	return ids, 0
}

func printIDs(stdout io.Writer, ids []string) {
	for _, id := range ids {
		_, _ = fmt.Fprintln(stdout, id)
	}
}

func affectedFlows(repo fs.FS, run execFunc, base string) ([]string, error) {
	git := func(args ...string) ([]byte, error) { return run(context.Background(), ".", nil, "git", args...) }
	names, err := git("diff", "--name-only", "--diff-filter=d", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	changed := strings.Fields(string(names))
	if registry := path.Join(backendDir, flows.File); slices.Contains(changed, registry) {
		rows, err := git("diff", "-U0", base+"...HEAD", "--", flows.File)
		if err != nil {
			return nil, err
		}
		changed = slices.DeleteFunc(changed, func(f string) bool { return f == registry })
		changed = append(changed, flows.ChangedRows(string(rows))...)
	}
	parsed, _, err := readFlows(repo)
	if err != nil {
		return nil, err
	}
	rows, _ := flows.ReadApp(repo)
	app := map[string]flows.AppRow{}
	for _, row := range rows {
		app[row.ID] = row
	}
	return flows.Affected(changed, parsed, app), nil
}

func declaringDeps() module.Deps {
	cfg, _ := config.Load([]string{"MONACO_ENV=test", "DATABASE_URL=unused", "NATS_URL=unused"})
	return module.Deps{Config: cfg, HTTPClient: httpclient.New}
}

func flowsCheck(
	env flows.Env,
	ids []string,
	tests io.Reader,
	structureOnly bool,
	integration io.Reader,
	stderr io.Writer,
) int {
	parsed, problems, err := readFlows(env.Repo)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
		return 1
	}
	results := flows.TestResults{}
	if tests != nil {
		if results, err = flows.ReadTestResults(tests); err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
	}
	problems = append(problems, flows.CheckColumns(parsed, env, ids)...)
	if !structureOnly {
		problems = append(problems, flows.CheckTests(parsed, results, ids)...)
	}
	problems = append(problems, flows.CheckScripts(parsed, env, ids)...)
	app, appProblems := flows.ReadApp(env.Repo)
	problems = append(problems, appProblems...)
	problems = append(problems, flows.CheckApp(app, parsed, env)...)
	problems = append(problems, flows.CheckAppModels(app, parsed, env)...)
	problems = append(problems, flows.CheckNoAggregate(env.Repo, parsed)...)
	if integration == nil {
		if verified := appVerifiedIDs(app); len(verified) > 0 {
			_, _ = fmt.Fprintf(
				stderr,
				"monacoctl flows check: skipped the integration tests of app verified flows %s; "+
					"pass --integration-xunit to check them\n",
				strings.Join(verified, ", "),
			)
		}
	} else {
		results, err := flows.ReadIntegrationXUnit(integration)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
		problems = append(problems, flows.CheckIntegration(app, parsed, results)...)
	}
	slices.SortStableFunc(problems, func(a, b flows.Problem) int {
		return cmp.Or(strings.Compare(a.File, b.File), a.Line-b.Line)
	})
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func appVerifiedIDs(app []flows.AppRow) []string {
	var verified []string
	for _, row := range app {
		if row.Status == flows.AppVerified {
			verified = append(verified, row.ID)
		}
	}
	return verified
}

func readFlows(repo fs.FS) ([]flows.Flow, []flows.Problem, error) {
	file, err := repo.Open(path.Join(backendDir, flows.File))
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.readFlows")
	}
	defer func() { _ = file.Close() }()
	parsed, problems := flows.Parse(file)
	return parsed, problems, nil
}

func liveEnv(repo fs.FS, backend string, mods module.Set) flows.Env {
	catalog := events.Catalog()
	eventTypes := make([]string, 0, len(catalog))
	for _, e := range catalog {
		eventTypes = append(eventTypes, string(e.Type))
	}
	codes := errs.All()
	codeNames := make([]string, 0, len(codes))
	for _, c := range codes {
		codeNames = append(codeNames, errs.Name(c))
	}
	consumers := mods.Consumers()
	durables := make([]string, 0, len(consumers))
	for _, c := range consumers {
		durables = append(durables, c.Durable)
		for _, h := range c.Handlers {
			durables = append(durables, h.Name)
		}
	}
	pollers := mods.Pollers()
	pollerNames := make([]string, 0, len(pollers))
	for _, p := range pollers {
		pollerNames = append(pollerNames, p.Name())
	}
	return flows.Env{
		Repo:        repo,
		BackendDir:  backendDir,
		Events:      flows.Members(eventTypes),
		Codes:       flows.Members(codeNames),
		Triggers:    flows.Triggers(openapi.Spec, eventTypes, pollerNames),
		Commands:    flows.Commands(backend),
		Consumers:   flows.Members(durables),
		Faultpoints: func(_ flows.Flow, v string) bool { return faultpoint.Known(v) },
		Scripts: func(_ flows.Flow, name string) bool {
			_, ok := testflows.Scripts()[name]
			return ok
		},
	}
}

func toolFlows(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		return flowsCmd(env.environ, args, runCommand, os.DirFS("../.."), stdout, stderr)
	}
}

func flowsSeed(ctx context.Context, repo fs.FS, environ []string, id, outcome string, stdout, stderr io.Writer) int {
	parsed, _, err := readFlows(repo)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows seed: %v\n", err)
		return 1
	}
	flow, valid := flows.Flow{}, []string{}
	for _, f := range parsed {
		if f.ID == id {
			flow = f
			for _, o := range f.Outcomes {
				valid = append(valid, string(o))
			}
		}
	}
	if !slices.Contains(valid, outcome) {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows seed: no outcome %s on flow %s; valid outcomes: %s\n",
			outcome, id, cmp.Or(strings.Join(valid, ", "), "none, the flow is not in "+flows.File))
		return 2
	}
	var names []string
	for _, command := range flow.Commands {
		names = append(names, flows.ScriptName(flow, command, flows.Outcome(outcome)))
	}
	i := slices.IndexFunc(names, func(n string) bool { return testflows.Seeds()[n] != nil })
	if i < 0 {
		_, _ = fmt.Fprintf(
			stderr,
			"monacoctl flows seed: flow %s outcome %s has no seeder %s in internal/testkit/flows/seed.go\n",
			id,
			outcome,
			strings.Join(names, " or "),
		)
		return 1
	}
	name, seed := names[i], testflows.Seeds()[names[i]]
	vars := map[string]string{}
	for _, kv := range environ {
		if k, v, found := strings.Cut(kv, "="); found {
			vars[k] = v
		}
	}
	result, err := seed(ctx, testflows.SeedEnv{
		DatabaseURL: vars["DATABASE_URL"], NatsURL: vars["NATS_URL"],
		FakesURL: vars["MONACO_FAKES_URL"], TokenKey: vars["MONACO_DEV_TOKEN_KEY"],
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows seed: %s: %v\n", name, err)
		return 1
	}
	body, _ := json.Marshal(result)
	_, _ = fmt.Fprintf(stdout, "%s\n", body)
	return 0
}
