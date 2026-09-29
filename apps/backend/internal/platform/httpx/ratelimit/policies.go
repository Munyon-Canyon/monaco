package ratelimit

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const Extension = "x-rate-limit"

type scope string

const (
	scopeActor scope = "actor"
	scopeIP    scope = "ip"
)

type limit struct {
	scope  scope
	policy Policy
}

type route struct {
	operation string
	limits    []limit
}

type Policies struct {
	routes map[string]route
}

type wirePolicy struct {
	Rate  int64  `json:"rate"`
	Per   string `json:"per"`
	Burst int64  `json:"burst"`
}

type wireScopes struct {
	Actor *wirePolicy `json:"actor"`
	IP    *wirePolicy `json:"ip"`
}

func Load(spec []byte) (Policies, error) {
	const op = "ratelimit.Load"
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return Policies{}, errs.Wrap(err, errs.CodeInvalidConfig, op)
	}
	routes := map[string]route{}
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			attr := slog.String("operation", operation.OperationID)
			raw, ok := operation.Extensions[Extension]
			if !ok {
				if public(doc, operation) && method != http.MethodGet {
					return Policies{}, errs.New(errs.CodeInvalidConfig, op, attr,
						slog.String("reason", "public "+method+" has no "+Extension))
				}
				continue
			}
			limits, err := parse(raw)
			if err != nil {
				return Policies{}, errs.Wrap(err, errs.CodeInvalidConfig, op, attr)
			}
			routes[method+" "+path] = route{operation: operation.OperationID, limits: limits}
		}
	}
	return Policies{routes: routes}, nil
}

func public(doc *openapi3.T, operation *openapi3.Operation) bool {
	security := doc.Security
	if operation.Security != nil {
		security = *operation.Security
	}
	return len(security) == 0
}

func parse(raw any) ([]limit, error) {
	const op = "ratelimit.parse"
	encoded, _ := json.Marshal(raw)
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.DisallowUnknownFields()
	var scopes wireScopes
	if err := dec.Decode(&scopes); err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidConfig, op)
	}
	var limits []limit
	for _, s := range []struct {
		scope scope
		wire  *wirePolicy
	}{{scopeActor, scopes.Actor}, {scopeIP, scopes.IP}} {
		if s.wire == nil {
			continue
		}
		p, err := s.wire.policy()
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidConfig, op, slog.String("scope", string(s.scope)))
		}
		limits = append(limits, limit{scope: s.scope, policy: p})
	}
	if len(limits) == 0 {
		return nil, errs.New(errs.CodeInvalidConfig, op, slog.String("reason", "no actor or ip scope"))
	}
	return limits, nil
}

func (w wirePolicy) policy() (Policy, error) {
	per, err := time.ParseDuration(w.Per)
	p := Policy{Rate: w.Rate, Per: per, Burst: w.Burst}
	if err != nil || !p.valid() {
		return Policy{}, errs.New(errs.CodeInvalidConfig, "ratelimit.policy",
			slog.Int64("rate", w.Rate), slog.String("per", w.Per), slog.Int64("burst", w.Burst))
	}
	return p, nil
}
