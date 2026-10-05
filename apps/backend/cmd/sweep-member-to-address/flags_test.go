package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestParseSweepFlags_requiresDestination(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"--all"}} {
		var usage bytes.Buffer
		if _, err := parseSweepFlags(args, &usage); !errors.Is(err, errNoDestination) {
			t.Fatalf("%v: err = %v, want %v", args, err, errNoDestination)
		}
		if !strings.Contains(usage.String(), "usage: sweep-member-to-address --destination") {
			t.Fatalf("%v: usage = %q", args, usage.String())
		}
	}
}

func TestParseSweepFlags_manySources(t *testing.T) {
	t.Parallel()
	flags, err := parseSweepFlags([]string{
		"--destination", string(dest),
		"--source", string(member.Address),
		"--source", string(cabal.Address) + ", " + string(stray.Address) + ",",
		"--dry-run",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []chain.SolanaAddress{member.Address, cabal.Address, stray.Address}
	if flags.destination != dest || !slices.Equal(flags.sources, want) || !flags.dryRun || flags.all {
		t.Fatalf("flags = %+v", flags)
	}
}

func TestParseSweepFlags_allAndDryRun(t *testing.T) {
	t.Parallel()
	flags, err := parseSweepFlags([]string{"--destination", string(dest), "--all", "--dry-run"}, &bytes.Buffer{})
	if err != nil || !flags.all || !flags.dryRun || len(flags.sources) != 0 {
		t.Fatalf("flags = %+v, %v", flags, err)
	}
}

func TestParseSweepFlags_rejects(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		args []string
		want error
	}{
		"all with source": {
			[]string{"--destination", string(dest), "--all", "--source", string(member.Address)}, errAllWithSources,
		},
		"bad destination": {[]string{"--destination", "Dest111"}, errNotAnAddress},
		"bad source":      {[]string{"--destination", string(dest), "--source", "Src111"}, errUsage},
		"positional":      {[]string{"--destination", string(dest), "extra"}, errUsage},
	} {
		_, err := parseSweepFlags(tc.args, &bytes.Buffer{})
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}
