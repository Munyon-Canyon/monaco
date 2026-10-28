package domain

import (
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Choice string

const (
	ChoiceYes Choice = "yes"
	ChoiceNo  Choice = "no"
)

func Choices() []Choice { return []Choice{ChoiceYes, ChoiceNo} }

func ParseChoice(raw string) (Choice, error) {
	if !slices.Contains(Choices(), Choice(raw)) {
		return "", errs.New(errs.CodeInvalidInput, "governance.ParseChoice", slog.String("field", "choice"))
	}
	return Choice(raw), nil
}

const UndoWindow = 15 * time.Second

func Undoable(firstCastAt time.Time, changed bool, now time.Time) bool {
	return !changed && now.Before(firstCastAt.Add(UndoWindow))
}
