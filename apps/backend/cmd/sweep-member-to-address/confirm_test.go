package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func confirmOptsFor(dryRun bool) confirmOpts {
	return confirmOpts{
		dest:        dest,
		databaseURL: "postgres://qa-operator@localhost:54322/monaco",
		sourceNote:  "postgres user_wallets + treasury_wallets",
		dryRun:      dryRun,
		sources:     []sweepSource{{kind: kindMember, wallet: member}},
	}
}

func TestConfirmSweep_liveRunNeedsThePhraseAndTheDestinationAgain(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		input string
		want  error
	}{
		"wrong phrase":        {"nope\n" + string(dest) + "\n", errRejected},
		"no input":            {"", errAborted},
		"mismatched dest":     {ackPhrase + "\n" + string(member.Address) + "\n", errDestDiffers},
		"dest never re-typed": {ackPhrase + "\n", errDestAborted},
	} {
		err := confirmSweep(strings.NewReader(tc.input), &bytes.Buffer{}, confirmOptsFor(false))
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	broken := errors.New("stdin closed")
	if err := confirmSweep(iotest.ErrReader(broken), &bytes.Buffer{}, confirmOptsFor(false)); !errors.Is(err, broken) {
		t.Fatalf("unreadable stdin: err = %v", err)
	}
	var out bytes.Buffer
	confirmed := strings.NewReader(ackPhrase + "\n" + string(dest) + "\n")
	if err := confirmSweep(confirmed, &out, confirmOptsFor(false)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	for _, want := range []string{
		"DANGER", "DATABASE_URL=***@localhost:54322/monaco", "  - member " + string(member.Address),
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "qa-operator") {
		t.Fatalf("printed the database user: %s", out.String())
	}
}

func TestConfirmSweep_dryRunSkipsAck(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := confirmSweep(strings.NewReader(""), &out, confirmOptsFor(true)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !strings.Contains(out.String(), "DRY-RUN") || strings.Contains(out.String(), "Type exactly") {
		t.Fatalf("dry-run prompt: %s", out.String())
	}
}
