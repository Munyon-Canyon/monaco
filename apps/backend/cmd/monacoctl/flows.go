package main

import (
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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
	flowsUsage = "usage: monacoctl flows check [--from go-test.json | --structure-only]"
)

func flowsCmd(args []string, _, stderr io.Writer) int {
	var tests io.Reader
	structureOnly := false
	switch {
	case slices.Equal(args, []string{"check", "--structure-only"}):
		structureOnly = true
	case slices.Equal(args, []string{"check"}):
		if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
			tests = os.Stdin
		}
	case len(args) == 3 && args[0] == "check" && args[1] == "--from":
		file, err := os.Open(args[2])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
		defer func() { _ = file.Close() }()
		tests = file
	default:
		_, _ = fmt.Fprintln(stderr, flowsUsage)
		return 2
	}
	env := liveEnv(os.DirFS("../.."), ".", registered.Build(declaringDeps()))
	return flowsCheck(env, tests, structureOnly, stderr)
}

func declaringDeps() module.Deps {
	cfg, _ := config.Load([]string{"MONACO_ENV=test", "DATABASE_URL=unused", "NATS_URL=unused"})
	return module.Deps{Config: cfg, HTTPClient: httpclient.New}
}

func flowsCheck(env flows.Env, tests io.Reader, structureOnly bool, stderr io.Writer) int {
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
	problems = append(problems, flows.CheckColumns(parsed, env)...)
	if !structureOnly {
		problems = append(problems, flows.CheckTests(parsed, results)...)
	}
	problems = append(problems, flows.CheckScripts(parsed, env)...)
	app, appProblems := flows.ReadApp(env.Repo)
	problems = append(problems, appProblems...)
	problems = append(problems, flows.CheckApp(app, parsed, env)...)
	problems = append(problems, flows.CheckAppModels(app, parsed, env)...)
	problems = append(problems, flows.CheckNoAggregate(env.Repo, parsed)...)
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

func toolFlows(_ toolEnv) tool { return flowsCmd }
