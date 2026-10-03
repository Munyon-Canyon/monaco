package verify

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	upstreams "github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	PrivyAppID   = "verify-app"
	pollEvery    = 50 * time.Millisecond
	logPollEvery = 10 * time.Millisecond
	procFakes    = "fakes"
	procAPI      = "api"
	procWorker   = "worker"
)

type Binaries struct {
	API, Worker, Fakes string
}

type PostgresFunc func(ctx context.Context, runID string) (url string, remove func(context.Context) error, err error)

type Options struct {
	Dir        string
	Atlas      string
	Docker     Docker
	Environ    []string
	Bins       Binaries
	Budget     Budget
	CoverDir   string
	Faultpoint string
	WorkerEnv  []string
	Postgres   PostgresFunc
	Clock      clock.Clock
}

type Stack struct {
	RunID    string
	API      string
	Worker   string
	Fakes    string
	DBURL    string
	TokenKey string
	Pool     *pgxpool.Pool
	NATS     *testkit.EmbeddedNATS
	Bus      *bus.Conn
	Logs     *Logs
	Crashes  int

	opts   Options
	env    []string
	procs  map[string]*process
	remove func(context.Context) error
	armed  bool
}

func Up(ctx context.Context, o Options) (*Stack, error) {
	s := &Stack{RunID: newRunID(), Logs: &Logs{}, opts: o, procs: map[string]*process{}}
	s.TokenKey = "verify-" + s.RunID
	if s.opts.Postgres == nil {
		s.opts.Postgres = s.dockerPostgres
	}
	ctx, cancel := withDeadline(ctx, cmp.Or[clock.Clock](o.Clock, clock.Real{}), o.Budget.Stack,
		&OverBudgetError{Phase: PhaseStack, Budget: o.Budget.Stack})
	defer cancel()
	for _, step := range []func(context.Context) error{
		s.postgres, withoutContext(s.nats), s.schema, s.processes, s.healthy,
	} {
		if err := step(ctx); err != nil {
			if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
				err = fmt.Errorf("%w: %w", cause, err)
			}
			return s, err
		}
	}
	return s, nil
}

func (s *Stack) postgres(ctx context.Context) (err error) {
	s.DBURL, s.remove, err = s.opts.Postgres(ctx, s.RunID)
	return err
}

func (s *Stack) dockerPostgres(ctx context.Context, runID string) (string, func(context.Context) error, error) {
	pg, err := startPostgres(ctx, s.opts.Docker, runID)
	return pg.url, pg.remove, err
}

func (s *Stack) nats() (err error) {
	if s.NATS, err = testkit.StartEmbeddedNATS(); err == nil {
		s.Bus, err = bus.Connect(context.Background(), config.NATS{URL: s.NATS.URL}, bus.ProcessMonacoctl)
	}
	return err
}

func withoutContext(step func() error) func(context.Context) error {
	return func(context.Context) error { return step() }
}

func (s *Stack) schema(ctx context.Context) error {
	if err := waitPostgres(ctx, s.DBURL); err != nil {
		return err
	}
	if err := migrate(ctx, s.opts.Atlas, filepath.Join(s.opts.Dir, "migrations"), s.DBURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, config.DB{URL: s.DBURL, MaxConns: 8})
	s.Pool = pool
	return err
}

func (s *Stack) processes(ctx context.Context) error {
	fakes, err := s.start(ctx, procFakes, s.opts.Bins.Fakes, "FAKES_ADDR=127.0.0.1:0")
	if err != nil {
		return err
	}
	s.env = append(childEnv(s.opts.Environ),
		"MONACO_ENV=test",
		"DATABASE_URL="+s.DBURL,
		"NATS_URL="+s.NATS.URL,
		"MONACO_DEV_TOKEN_KEY="+s.TokenKey,
		"MONACO_JUPITER_SWAP_BASE_URL=http://"+fakes.addr+"/jupiter/swap/v2",
		"MONACO_JUPITER_PRICE_BASE_URL=http://"+fakes.addr+"/jupiter/price/v3",
		"XSTOCKS_BASE_URL=http://"+fakes.addr+"/xstocks",
		"TESSERA_API_BASE_URL=http://"+fakes.addr+"/tessera",
		"PRESTOCKS_API_BASE_URL=http://"+fakes.addr+"/prestocks",
		"SOLANA_RPC_URL=http://"+fakes.addr+"/rpc/",
		"SUPABASE_URL=http://"+fakes.addr,
		"SUPABASE_SERVICE_ROLE_KEY=verify-service-role",
		"MONACO_BUS_ACK_WAIT=100ms",
		"PRIVY_APP_ID="+PrivyAppID,
		"PRIVY_APP_SECRET=verify-app-secret",
		"PRIVY_VERIFICATION_KEY="+upstreams.PrivyVerificationKey(),
		"PRIVY_BASE_URL=http://"+fakes.addr+"/privy",
		"PRIVY_AUTHORIZATION_KEY_ID="+upstreams.PrivyAuthorizationKeyID,
		"PRIVY_AUTHORIZATION_PRIVATE_KEY="+upstreams.PrivyAuthorizationKeyConfig(),
	)
	s.Fakes = "http://" + fakes.addr
	apiEnv, workerEnv := []string{"MONACO_HTTP_ADDR=127.0.0.1:0", "TRUST_PROXY_HEADERS=true"}, []string(nil)
	if s.opts.Faultpoint != "" {
		apiEnv = append(apiEnv, "MONACO_BUS_API_RELAY=off")
		workerEnv = append(workerEnv, "MONACO_FAULTPOINT="+s.opts.Faultpoint)
		s.armed = true
	}
	api, err := s.start(ctx, procAPI, s.opts.Bins.API, apiEnv...)
	if err != nil {
		return err
	}
	s.API = "http://" + api.addr
	return s.startWorker(ctx, workerEnv...)
}

func (s *Stack) startWorker(ctx context.Context, extra ...string) error {
	worker, err := s.start(ctx, procWorker, s.opts.Bins.Worker,
		slices.Concat([]string{"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0"}, s.opts.WorkerEnv, extra)...)
	s.Worker = "http://" + worker.addr
	return err
}

func (s *Stack) start(ctx context.Context, name, bin string, extra ...string) (*process, error) {
	env := slices.Concat(s.env, extra)
	if name == procFakes {
		env = append(childEnv(s.opts.Environ), extra...)
	}
	if s.opts.CoverDir != "" {
		env = append(env, "GOCOVERDIR="+s.opts.CoverDir)
	}
	p, err := startProcess(ctx, name, bin, env, s.Logs)
	s.procs[name] = p
	return p, err
}

func (s *Stack) healthy(ctx context.Context) error {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	last := ""
	for {
		sick, err := s.probe(ctx)
		if err != nil {
			return err
		}
		if sick == "" {
			return nil
		}
		if ctx.Err() == nil {
			last = sick
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("stack never became healthy (%s): %w", last, context.Cause(ctx))
		case <-tick.C:
		}
	}
}

func (s *Stack) probe(ctx context.Context) (string, error) {
	sick := ""
	for name, target := range map[string]string{procAPI: s.API, procWorker: s.Worker} {
		select {
		case <-s.procs[name].exited:
			if name == procWorker && s.armed {
				continue
			}
			return "", fmt.Errorf("%w: %s exited before it was healthy: %w\n%s",
				errFailed, name, s.procs[name].err, s.Logs.tail(name, 20))
		default:
		}
		if status := healthz(ctx, target); status != http.StatusOK {
			sick = fmt.Sprintf("%s/healthz answered %d", target, status)
		}
	}
	return sick, nil
}

func healthz(ctx context.Context, base string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func (s *Stack) Down(ctx context.Context) error {
	budget := s.opts.Budget.Teardown
	ctx, cancel := context.WithTimeoutCause(context.WithoutCancel(ctx), budget,
		&OverBudgetError{Phase: PhaseTeardown, Budget: budget})
	defer cancel()
	var err error
	for _, name := range []string{procWorker, procAPI, procFakes} {
		if p := s.procs[name]; p != nil {
			err = errors.Join(err, p.stop(ctx))
		}
	}
	if s.Pool != nil {
		s.Pool.Close()
	}
	if s.Bus != nil {
		s.Bus.Close(ctx)
	}
	s.NATS.Stop()
	if s.remove != nil {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 2*budget)
		defer done()
		err = errors.Join(err, s.remove(cleanup))
	}
	return err
}

func childEnv(environ []string) []string {
	var out []string
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "MONACO_") || strings.HasPrefix(key, "OTEL_") || slices.Contains(
			[]string{"DATABASE_URL", "NATS_URL", "JUPITER_API_KEY", "FAKES_ADDR", "TEST_DATABASE_URL"}, key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func newRunID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func waitPostgres(ctx context.Context, url string) error {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		pool, err := db.Open(ctx, config.DB{URL: url, MaxConns: 1})
		if err == nil {
			pool.Close()
			return nil
		}
		if code := errs.CodeOf(err); code != errs.CodeDBUnavailable {
			if code == errs.CodeDBSchemaBehind {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres never accepted connections: %w", errors.Join(context.Cause(ctx), err))
		case <-tick.C:
		}
	}
}

func migrate(ctx context.Context, atlas, dir, url string) error {
	cmd := exec.CommandContext(ctx, atlas, "migrate", "apply", "--dir", "file://"+dir, "--url", url)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s migrate apply: %w\n%s", atlas, err, out)
	}
	return nil
}
