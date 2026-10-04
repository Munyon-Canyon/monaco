package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/eventlog"
)

const eventsUsage = "usage: monacoctl events export [--aggregate <type>:<id>] [--anonymize] > file.jsonl\n--anonymize needs MONACO_EXPORT_KEY: 32 random bytes as hex (openssl rand -hex 32)"

const anonymizeKeyError = "--anonymize needs MONACO_EXPORT_KEY: 32 random bytes as hex (openssl rand -hex 32)"

func toolEvents(env toolEnv) tool { return eventsTool(env.environ) }

func eventsTool(environ []string) tool {
	export := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("events export", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		aggregate := fs.String("aggregate", "", "")
		anonymize := fs.Bool("anonymize", false, "")
		o, ok := exportOptions(fs, args, aggregate)
		if !ok {
			_, _ = fmt.Fprintln(stderr, eventsUsage)
			return 2
		}
		o.Anonymize = *anonymize
		if o.Anonymize {
			key, ok := exportKey(environ)
			if !ok {
				_, _ = fmt.Fprintln(stderr, anonymizeKeyError)
				return 2
			}
			o.Key = key
		}
		ctx := context.Background()
		pools, err := openPools(ctx, cfg.DB)
		if err == nil {
			defer closePools(pools)
			err = eventlog.Export(ctx, pools[0], stdout, o)
		}
		if err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"export": export}, nil, withoutExportKey(environ), args, stdout, stderr)
	}
}

func withoutExportKey(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if key != "MONACO_EXPORT_KEY" {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func exportKey(environ []string) ([]byte, bool) {
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if key != "MONACO_EXPORT_KEY" || !ok {
			continue
		}
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) < 32 {
			return nil, false
		}
		return decoded, true
	}
	return nil, false
}

func exportOptions(fs *flag.FlagSet, args []string, aggregate *string) (eventlog.Options, bool) {
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return eventlog.Options{}, false
	}
	if *aggregate == "" {
		return eventlog.Options{}, true
	}
	typ, raw, _ := strings.Cut(*aggregate, ":")
	id, err := uuid.Parse(raw)
	return eventlog.Options{AggregateType: typ, AggregateID: id}, err == nil && typ != ""
}
