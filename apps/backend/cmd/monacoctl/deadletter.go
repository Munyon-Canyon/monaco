package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const deadletterUsage = "usage: monacoctl deadletter list | retry <seq> | retry --all --consumer <durable>"

type lettersFunc func(ctx context.Context, conn *bus.Conn, letters []bus.DeadLetter) int

func toolDeadletter(env toolEnv) tool { return deadletterTool(env.environ, clock.Real{}) }

func deadletterTool(environ []string, clk clock.Clock) tool {
	list := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			_, _ = fmt.Fprintln(stderr, deadletterUsage)
			return 2
		}
		return withLetters(cfg, stderr, func(_ context.Context, _ *bus.Conn, letters []bus.DeadLetter) int {
			for _, l := range letters {
				_, _ = fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\t%s\t%s\n",
					l.Seq, l.Consumer, l.Handler, reason(l), l.MsgID, l.Error)
			}
			return 0
		})
	}
	retry := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		match, ok := retrySelector(args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, deadletterUsage)
			return 2
		}
		return withLetters(cfg, stderr, func(ctx context.Context, conn *bus.Conn, letters []bus.DeadLetter) int {
			return redeliver(ctx, conn, letters, match, clk, stdout, stderr)
		})
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"list": list, "retry": retry}, nil, environ, args, stdout, stderr)
	}
}

func withLetters(cfg config.Config, stderr io.Writer, fn lettersFunc) int {
	ctx := context.Background()
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
	if err != nil {
		return fail(stderr, err)
	}
	defer conn.Close(ctx)
	letters, err := conn.DeadLetters(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	return fn(ctx, conn, letters)
}

func redeliver(
	ctx context.Context, conn *bus.Conn, letters []bus.DeadLetter, match func(bus.DeadLetter) bool,
	clk clock.Clock, stdout, stderr io.Writer,
) int {
	retried := 0
	for _, l := range letters {
		if !match(l) {
			continue
		}
		suffix := fmt.Sprintf("retry-%d-%d", l.Seq, clk.Now().UnixNano())
		if err := conn.Redeliver(ctx, l, suffix); err != nil {
			return fail(stderr, err)
		}
		retried++
		_, _ = fmt.Fprintf(stdout, "retried %d %s\n", l.Seq, l.MsgID)
	}
	if retried == 0 {
		_, _ = fmt.Fprintln(stderr, "monacoctl: no dead letter matches")
		return 1
	}
	return 0
}

func reason(l bus.DeadLetter) string {
	if l.Advisory != nil {
		return "max_deliveries"
	}
	return l.Code
}

func retrySelector(args []string) (func(bus.DeadLetter) bool, bool) {
	if len(args) == 1 {
		seq, err := strconv.ParseUint(args[0], 10, 64)
		return func(l bus.DeadLetter) bool { return l.Seq == seq }, err == nil
	}
	fs := flag.NewFlagSet("deadletter retry", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "")
	consumer := fs.String("consumer", "", "")
	ok := fs.Parse(args) == nil && fs.NArg() == 0 && *all && *consumer != ""
	return func(l bus.DeadLetter) bool { return l.Consumer == *consumer }, ok
}
