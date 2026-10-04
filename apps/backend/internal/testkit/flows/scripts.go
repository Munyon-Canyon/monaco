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
	// These terminal Flow 05 outcomes share setup with the generated happy-path
	// script, so they are intentionally defined in f05.go rather than generated
	// from a separate route.
	maps.Copy(scripts, map[string]Script{
		"F05CreditDepositNotADeposit": F05CreditDepositNotADeposit,
		"F05CreditDepositMonacoSigned": F05CreditDepositMonacoSigned,
		"F05CreditDepositUnresolved":  F05CreditDepositUnresolved,
	})
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
