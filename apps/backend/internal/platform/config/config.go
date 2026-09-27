package config

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Env string

const (
	EnvLocal      Env = "local"
	EnvTest       Env = "test"
	EnvStaging    Env = "staging"
	EnvProduction Env = "production"
)

type Config struct {
	Env      Env
	HTTP     HTTP
	Worker   Worker
	DB       DB
	NATS     NATS
	OTel     OTel
	Timeouts Timeouts
}

type HTTP struct {
	Addr         string
	MaxBodyBytes int32
}

type Worker struct {
	HealthAddr string
}

type DB struct {
	URL      string
	MaxConns int32
}

type NATS struct {
	URL  string
	Name string
}

type OTel struct {
	Endpoint    string
	Headers     string
	ServiceName string
}

type Timeouts struct {
	RPC             time.Duration
	Privy           time.Duration
	JupiterQuote    time.Duration
	JupiterExecute  time.Duration
	HTTPServerRead  time.Duration
	HTTPServerWrite time.Duration
	Shutdown        time.Duration
}

const redacted = "***"

func Load(environ []string) (Config, error) {
	vars := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		vars[k] = v
	}
	var cfg Config
	var bad keysError
	known := map[string]bool{}
	for _, f := range fields() {
		known[f.key] = true
		v := vars[f.key]
		if v == "" && f.mandatory {
			bad.missing = append(bad.missing, f.key)
			continue
		}
		if v == "" {
			v = f.fallback
		}
		if !f.set(&cfg, v) {
			bad.invalid = append(bad.invalid, f.key+" ("+f.want+")")
		}
	}
	for k := range vars {
		if strings.HasPrefix(k, "MONACO_") && !known[k] {
			bad.unknown = append(bad.unknown, k)
		}
	}
	if len(bad.missing)+len(bad.unknown)+len(bad.invalid) > 0 {
		slices.Sort(bad.unknown)
		return Config{}, errs.Wrap(bad, errs.CodeInvalidInput, "config.Load")
	}
	return cfg, nil
}

const DefaultTestDBURL = "postgres://monaco:monaco@localhost:54323/monaco?sslmode=disable"

func TestDBURL(environ []string) string {
	url := DefaultTestDBURL
	for _, kv := range environ {
		if k, v, _ := strings.Cut(kv, "="); k == "TEST_DATABASE_URL" && v != "" {
			url = v
		}
	}
	return url
}

func (c Config) Redacted() map[string]string {
	fs := fields()
	out := make(map[string]string, len(fs))
	for _, f := range fs {
		v := f.get(&c)
		if f.redact {
			v = redacted
		}
		out[f.key] = v
	}
	return out
}

type keysError struct {
	missing []string
	unknown []string
	invalid []string
}

func (e keysError) Error() string {
	var parts []string
	for _, group := range []struct {
		label string
		keys  []string
	}{{"missing", e.missing}, {"unknown", e.unknown}, {"invalid", e.invalid}} {
		if len(group.keys) > 0 {
			parts = append(parts, group.label+" "+strings.Join(group.keys, ", "))
		}
	}
	return strings.Join(parts, "; ")
}

type field struct {
	key       string
	fallback  string
	mandatory bool
	redact    bool
	want      string
	set       func(c *Config, v string) bool
	get       func(c *Config) string
}

func (f field) required() field {
	f.mandatory = true
	return f
}

func (f field) secret() field {
	f.redact = true
	return f
}

func fields() []field {
	return []field{
		environment("MONACO_ENV", func(c *Config) *Env { return &c.Env }).required(),
		text("MONACO_HTTP_ADDR", ":8080", func(c *Config) *string { return &c.HTTP.Addr }),
		count("MONACO_HTTP_MAX_BODY_BYTES", 1<<20, func(c *Config) *int32 { return &c.HTTP.MaxBodyBytes }),
		text("MONACO_WORKER_HEALTH_ADDR", ":8081", func(c *Config) *string { return &c.Worker.HealthAddr }),
		text("DATABASE_URL", "", func(c *Config) *string { return &c.DB.URL }).required().secret(),
		count("MONACO_DB_MAX_CONNS", 10, func(c *Config) *int32 { return &c.DB.MaxConns }),
		text("NATS_URL", "", func(c *Config) *string { return &c.NATS.URL }).required().secret(),
		text("MONACO_NATS_NAME", "monaco", func(c *Config) *string { return &c.NATS.Name }),
		text("OTEL_EXPORTER_OTLP_ENDPOINT", "", func(c *Config) *string { return &c.OTel.Endpoint }),
		text("OTEL_EXPORTER_OTLP_HEADERS", "", func(c *Config) *string { return &c.OTel.Headers }).secret(),
		text("OTEL_SERVICE_NAME", "monaco", func(c *Config) *string { return &c.OTel.ServiceName }),
		duration("MONACO_TIMEOUT_RPC", 5*time.Second, func(c *Config) *time.Duration { return &c.Timeouts.RPC }),
		duration("MONACO_TIMEOUT_PRIVY", 10*time.Second, func(c *Config) *time.Duration { return &c.Timeouts.Privy }),
		duration("MONACO_TIMEOUT_JUPITER_QUOTE", 5*time.Second,
			func(c *Config) *time.Duration { return &c.Timeouts.JupiterQuote }),
		duration("MONACO_TIMEOUT_JUPITER_EXECUTE", 2*time.Minute,
			func(c *Config) *time.Duration { return &c.Timeouts.JupiterExecute }),
		duration("MONACO_TIMEOUT_HTTP_SERVER_READ", 10*time.Second,
			func(c *Config) *time.Duration { return &c.Timeouts.HTTPServerRead }),
		duration("MONACO_TIMEOUT_HTTP_SERVER_WRITE", 30*time.Second,
			func(c *Config) *time.Duration { return &c.Timeouts.HTTPServerWrite }),
		duration("MONACO_TIMEOUT_SHUTDOWN", 10*time.Second,
			func(c *Config) *time.Duration { return &c.Timeouts.Shutdown }),
	}
}

func text(key, fallback string, at func(*Config) *string) field {
	return field{
		key:      key,
		fallback: fallback,
		set:      func(c *Config, v string) bool { *at(c) = v; return true },
		get:      func(c *Config) string { return *at(c) },
	}
}

func environment(key string, at func(*Config) *Env) field {
	envs := []Env{EnvLocal, EnvTest, EnvStaging, EnvProduction}
	return field{
		key:  key,
		want: "local, test, staging or production",
		set: func(c *Config, v string) bool {
			*at(c) = Env(v)
			return slices.Contains(envs, Env(v))
		},
		get: func(c *Config) string { return string(*at(c)) },
	}
}

func count(key string, fallback int32, at func(*Config) *int32) field {
	return field{
		key:      key,
		fallback: strconv.Itoa(int(fallback)),
		want:     "positive integer",
		set: func(c *Config, v string) bool {
			n, err := strconv.ParseInt(v, 10, 32)
			*at(c) = int32(n)
			return err == nil && n > 0
		},
		get: func(c *Config) string { return strconv.Itoa(int(*at(c))) },
	}
}

func duration(key string, fallback time.Duration, at func(*Config) *time.Duration) field {
	return field{
		key:      key,
		fallback: fallback.String(),
		want:     "positive duration like 5s",
		set: func(c *Config, v string) bool {
			d, err := time.ParseDuration(v)
			*at(c) = d
			return err == nil && d > 0
		},
		get: func(c *Config) string { return at(c).String() },
	}
}
