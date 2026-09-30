package flows

import (
	"go/types"
	"path"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

type TriggerKind string

const (
	TriggerRoute    TriggerKind = "route"
	TriggerPoller   TriggerKind = "poller"
	TriggerConsumer TriggerKind = "consumer"
)

func (f Flow) TriggerKind() (kind TriggerKind, name string) { return triggerKind(f.Trigger) }

func triggerKind(trigger string) (TriggerKind, string) {
	for _, kind := range []TriggerKind{TriggerPoller, TriggerConsumer} {
		if name, ok := strings.CutPrefix(trigger, string(kind)+":"); ok {
			return kind, name
		}
	}
	return TriggerRoute, trigger
}

func Members(values []string) Lookup {
	return func(_ Flow, v string) bool { return slices.Contains(values, v) }
}

func Triggers(spec []byte, subjects, pollers []string) Lookup {
	known := map[TriggerKind][]string{TriggerRoute: Routes(spec), TriggerPoller: pollers, TriggerConsumer: subjects}
	return func(_ Flow, trigger string) bool {
		kind, name := triggerKind(trigger)
		return slices.Contains(known[kind], name)
	}
}

func Routes(spec []byte) []string {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil
	}
	var routes []string
	for p, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			routes = append(routes, method+" "+p)
		}
	}
	slices.Sort(routes)
	return routes
}

func Commands(backendDir string) Lookup {
	scopes := map[string]*types.Scope{}
	return func(f Flow, name string) bool {
		scope, loaded := scopes[f.Module]
		if !loaded {
			scope = appScope(backendDir, f.Module)
			scopes[f.Module] = scope
		}
		if scope == nil {
			return false
		}
		_, isType := scope.Lookup(name).(*types.TypeName)
		return isType
	}
}

func appScope(backendDir, module string) *types.Scope {
	pattern := "./" + path.Join("internal/modules", module, "app")
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes, Dir: backendDir}, pattern)
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		return nil
	}
	return pkgs[0].Types.Scope()
}
