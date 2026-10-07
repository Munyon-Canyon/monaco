package events

import "github.com/google/uuid"

const TypeReportCreated Type = "report.created"

type ReportCreated struct {
	V          int       `json:"v"`
	ReportID   uuid.UUID `json:"report_id"`
	ReporterID uuid.UUID `json:"reporter_id" pii:"true"`
	Kind       string    `json:"kind"`
	TargetID   uuid.UUID `json:"target_id"   pii:"true"`
	Reason     string    `json:"reason"`
}

func (ReportCreated) Type() Type { return TypeReportCreated }

func (ReportCreated) AggregateType() string { return "report" }

func (e ReportCreated) AggregateID() uuid.UUID { return e.ReportID }
