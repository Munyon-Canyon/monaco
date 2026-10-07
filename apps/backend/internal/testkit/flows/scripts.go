package flows

import (
	"maps"
	"reflect"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
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

type Letter struct {
	Subject events.Type
	Code    errs.Code
}

func Alone(s Script) bool {
	alone := false
	each("AloneF", func(_ string, out any) {
		for _, a := range out.([]Script) {
			alone = alone || sameScript(a, s)
		}
	})
	return alone
}

func LettersOf(s Script) []Letter {
	var letters []Letter
	scripts := Scripts()
	each("Letters", func(_ string, out any) {
		for name, declared := range out.(map[string][]Letter) {
			if sameScript(scripts[name], s) {
				letters = append(letters, declared...)
			}
		}
	})
	return letters
}

func sameScript(a, b Script) bool {
	return a != nil && b != nil && reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func each(prefix string, yield func(suffix string, out any)) {
	v := reflect.ValueOf(defined{})
	for i := range v.NumMethod() {
		if name := v.Type().Method(i).Name; strings.HasPrefix(name, prefix) {
			yield(strings.TrimPrefix(name, prefix), v.Method(i).Call(nil)[0].Interface())
		}
	}
}
