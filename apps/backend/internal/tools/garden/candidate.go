package garden

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	enforcedConfig  = ".golangci.yml"
	candidateConfig = ".golangci.candidate.yml"
)

func CandidateConfig(moduleDir string) ([]byte, error) {
	const op = "garden.CandidateConfig"
	root, err := os.OpenRoot(moduleDir)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	defer func() { _ = root.Close() }()
	var merged any
	for _, name := range []string{enforcedConfig, candidateConfig} {
		data, err := root.ReadFile(name)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		var layer any
		if err := yaml.Unmarshal(data, &layer); err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
		}
		merged = overlay(merged, layer)
	}
	out, _ := yaml.Marshal(merged)
	return out, nil
}

func overlay(base, top any) any {
	switch t := top.(type) {
	case map[string]any:
		b, ok := base.(map[string]any)
		if !ok {
			return t
		}
		for k, v := range t {
			b[k] = overlay(b[k], v)
		}
		return b
	case []any:
		b, _ := base.([]any)
		return append(b, t...)
	default:
		return top
	}
}
