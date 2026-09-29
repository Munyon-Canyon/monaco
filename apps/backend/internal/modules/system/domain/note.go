package domain

import (
	"log/slog"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxNoteRunes = 140

type Note struct{ text string }

func ParseNote(raw string) (Note, error) {
	if n := utf8.RuneCountInString(raw); n > MaxNoteRunes {
		return Note{}, errs.New(errs.CodeInvalidInput, "system.ParseNote",
			slog.Int("runes", n), slog.Int("max", MaxNoteRunes))
	}
	return Note{text: raw}, nil
}

func (n Note) String() string { return n.text }
