package flows

import (
	"maps"
	"reflect"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type Script func(*scenario.Scenario)

type defined struct{}

func Scripts() map[string]Script {
	scripts := map[string]Script{}
	each("Scripts", func(_ string, out any) { maps.Copy(scripts, out.(map[string]Script)) })
	return scripts
}

func Env() map[string][]string {
	env := map[string][]string{}
	each("WorkerEnvF", func(id string, out any) { env[id] = out.([]string) })
	return env
}

func each(prefix string, yield func(suffix string, out any)) {
	v := reflect.ValueOf(defined{})
	for i := range v.NumMethod() {
		if name := v.Type().Method(i).Name; strings.HasPrefix(name, prefix) {
			yield(strings.TrimPrefix(name, prefix), v.Method(i).Call(nil)[0].Interface())
		}
	}
}
