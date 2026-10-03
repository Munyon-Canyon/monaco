package flows

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type IntegrationResult string

const (
	IntegrationPassed  IntegrationResult = "passed"
	IntegrationFailed  IntegrationResult = "failed"
	IntegrationSkipped IntegrationResult = "skipped"
)

type IntegrationResults map[string]IntegrationResult

type xunitCase struct {
	Class    string     `xml:"classname,attr"`
	Name     string     `xml:"name,attr"`
	Failures []struct{} `xml:"failure"`
	Errors   []struct{} `xml:"error"`
	Skipped  *struct{}  `xml:"skipped"`
}

func ReadIntegrationXUnit(r io.Reader) (IntegrationResults, error) {
	results := IntegrationResults{}
	decoder := xml.NewDecoder(r)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return results, nil
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, "flows.ReadIntegrationXUnit")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "testcase" {
			continue
		}
		var c xunitCase
		if err := decoder.DecodeElement(&c, &start); err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, "flows.ReadIntegrationXUnit")
		}
		class := c.Class[strings.LastIndex(c.Class, ".")+1:]
		results[class+"."+strings.TrimSuffix(c.Name, "()")] = c.result()
	}
}

func (c xunitCase) result() IntegrationResult {
	switch {
	case len(c.Failures)+len(c.Errors) > 0:
		return IntegrationFailed
	case c.Skipped != nil:
		return IntegrationSkipped
	default:
		return IntegrationPassed
	}
}

func IntegrationClass(f Flow) string { return "F" + f.ID + "IntegrationTests" }

func integrationSuffixes(f Flow) []string {
	var suffixes []string
	for _, o := range f.Outcomes {
		suffix := string(o)
		if _, crash := o.CrashPoint(); crash {
			suffix = "interrupted"
		}
		if !slices.Contains(suffixes, suffix) {
			suffixes = append(suffixes, suffix)
		}
	}
	return suffixes
}

func CheckIntegration(app []AppRow, backend []Flow, results IntegrationResults) []Problem {
	var problems []Problem
	for _, row := range app {
		i := slices.IndexFunc(backend, func(f Flow) bool { return f.ID == row.ID })
		if row.Status != AppVerified || i < 0 {
			continue
		}
		for _, suffix := range integrationSuffixes(backend[i]) {
			if gap, ok := integrationGap(backend[i], suffix, results); ok {
				problems = append(problems, Problem{
					File: row.File, Line: row.Line,
					Msg: fmt.Sprintf("flow %s: app verified but %s %s", row.ID, IntegrationClass(backend[i]), gap),
				})
			}
		}
	}
	return problems
}

func integrationGap(f Flow, suffix string, results IntegrationResults) (string, bool) {
	var names, failed, skipped []string
	for _, command := range f.Commands {
		name := "test_F" + f.ID + "_" + command + "_" + suffix
		names = append(names, name)
		switch results[IntegrationClass(f)+"."+name] {
		case IntegrationPassed:
			return "", false
		case IntegrationFailed:
			failed = append(failed, name)
		case IntegrationSkipped:
			skipped = append(skipped, name)
		}
	}
	switch {
	case len(failed) > 0:
		return "has a failing " + strings.Join(failed, " and "), true
	case len(skipped) > 0:
		return "skipped " + strings.Join(skipped, " and "), true
	default:
		return "lacks " + strings.Join(names, " or "), true
	}
}
