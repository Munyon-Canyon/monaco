package marketing

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/providers/telegram"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

const usage = "usage: monacoctl marketing watch [--once] [--dry-run]"

type Deps struct {
	Environ      []string
	Dir          string
	Clock        clock.Clock
	XBase        string
	TelegramBase string
}

type keys struct {
	xToken, botToken, chatID string
}

type appender struct {
	store *Store
	path  string
}

func (a appender) Write(p []byte) (int, error) {
	if err := a.store.Append(a.path, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func Main(ctx context.Context, d Deps, args []string, stdout, stderr io.Writer) int {
	once, dry, ok := parse(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := run(ctx, d, once, dry, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl marketing: %s\n", report(err))
		return 1
	}
	return 0
}

func parse(args []string) (once, dry, ok bool) {
	if len(args) == 0 || args[0] != "watch" {
		return false, false, false
	}
	fs := flag.NewFlagSet("marketing watch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&once, "once", false, "")
	fs.BoolVar(&dry, "dry-run", false, "")
	return once, dry, fs.Parse(args[1:]) == nil && fs.NArg() == 0
}

func run(ctx context.Context, d Deps, once, dry bool, stdout, stderr io.Writer) error {
	root, err := findRoot(d.Dir)
	if err != nil {
		return err
	}
	cfg, err := Load(os.DirFS(root))
	if err != nil {
		return err
	}
	if dry {
		return DryRun(stdout, cfg, Fixture())
	}
	k, err := readKeys(d.Environ)
	if err != nil {
		return err
	}
	store, err := OpenStore(root)
	if err != nil {
		return err
	}
	defer store.Close()
	xhttp := x.HTTP(cmp.Or(d.XBase, x.DefaultBaseURL))
	defer xhttp.CloseIdleConnections()
	thttp := telegram.HTTP(cmp.Or(d.TelegramBase, telegram.DefaultBaseURL))
	defer thttp.CloseIdleConnections()
	w := &Watcher{
		Config: cfg,
		Source: Searcher{Client: x.New(xhttp, k.xToken), Config: cfg, Clock: d.Clock},
		Notify: telegram.New(thttp, k.botToken, k.chatID),
		Store:  store,
		Clock:  d.Clock,
		Log:    slog.New(slog.NewJSONHandler(io.MultiWriter(stderr, appender{store, logPath}), nil)),
	}
	if once {
		return w.Poll(ctx)
	}
	w.Run(ctx)
	return nil
}

func findRoot(dir string) (string, error) {
	for cur := dir; ; cur = filepath.Dir(cur) {
		if _, err := os.Stat(filepath.Join(cur, ConfigPath)); err == nil {
			return cur, nil
		}
		if filepath.Dir(cur) == cur {
			return "", invalid("no " + ConfigPath + " in " + dir + " or above")
		}
	}
}

func readKeys(environ []string) (keys, error) {
	get := func(name string) string {
		for _, kv := range environ {
			if v, ok := strings.CutPrefix(kv, name+"="); ok {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	k := keys{xToken: get("X_BEARER_TOKEN"), botToken: get("TELEGRAM_BOT_TOKEN"), chatID: get("TELEGRAM_CHAT_ID")}
	var missing []string
	for _, kv := range [...]struct{ name, value string }{
		{"X_BEARER_TOKEN", k.xToken}, {"TELEGRAM_BOT_TOKEN", k.botToken}, {"TELEGRAM_CHAT_ID", k.chatID},
	} {
		if kv.value == "" {
			missing = append(missing, kv.name)
		}
	}
	if len(missing) > 0 {
		return keys{}, invalid("set " + strings.Join(missing, ", ") + " (docs/how-to/reply-finder.md)")
	}
	return k, nil
}

func report(err error) string {
	for _, a := range errs.Detail(err) {
		if a.Key == "detail" {
			return a.Value.String()
		}
	}
	return err.Error()
}
