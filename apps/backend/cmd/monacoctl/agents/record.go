package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type checkRecord struct {
	Head        string   `json:"head,omitempty"`
	Base        string   `json:"base,omitempty"`
	CarriedFrom string   `json:"carried_from,omitempty"`
	Rows        []string `json:"rows"`
}

func (env *Env) writeCheckRecord(tree string, record checkRecord) (string, error) {
	body, _ := json.Marshal(record)
	return env.writeState("checks", tree, append(body, '\n'))
}

func (env *Env) readCheckRecord(tree string) (checkRecord, error) {
	body, err := os.ReadFile(env.statePath("checks", tree))
	if err != nil {
		return checkRecord{}, fmt.Errorf("read check record: %w", err)
	}
	var record checkRecord
	if json.Unmarshal(body, &record) != nil {
		record = checkRecord{}
	}
	return record, nil
}

func attestedRows() []string {
	return []string{"swift test", "xcode", "journeys"}
}

func (r checkRecord) description(tree string) string {
	var rows []string
	for _, row := range attestedRows() {
		if slices.Contains(r.Rows, row) {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return "tree " + tree
	}
	return "tree " + tree + " rows " + strings.Join(rows, ",")
}
