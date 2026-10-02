package domain

import (
	"log/slog"
	"regexp"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Environment string

const (
	EnvironmentSandbox    Environment = "sandbox"
	EnvironmentProduction Environment = "production"
)

func ParseEnvironment(raw string) (Environment, error) {
	switch env := Environment(raw); env {
	case EnvironmentSandbox, EnvironmentProduction:
		return env, nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "notify.ParseEnvironment", slog.String("environment", raw))
	}
}

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{64,200}$`)

type DeviceToken struct {
	value string
}

func ParseDeviceToken(raw string) (DeviceToken, error) {
	if !tokenPattern.MatchString(raw) {
		return DeviceToken{}, errs.New(errs.CodeInvalidInput, "notify.ParseDeviceToken", slog.Int("len", len(raw)))
	}
	return DeviceToken{value: raw}, nil
}

func (t DeviceToken) String() string { return t.value }

func (DeviceToken) LogValue() slog.Value { return slog.StringValue("[redacted]") }
