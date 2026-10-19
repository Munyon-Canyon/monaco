package marketing

import (
	"fmt"
	"time"
)

const maxText = 600

type alertLine struct {
	PostID string    `json:"post_id"`
	Score  float64   `json:"score"`
	Angle  string    `json:"angle"`
	Rules  []string  `json:"rules"`
	At     time.Time `json:"at"`
}

func Message(s Scored) string {
	p := s.Post
	return fmt.Sprintf("@%s · %d followers · score %.2f\n\n%s\n\nAngle: %s\n\n%s",
		p.Handle, p.Followers, s.Score, clip(p.Text, maxText), s.Angle, p.URL())
}

func clip(text string, limit int) string {
	r := []rune(text)
	if len(r) <= limit {
		return text
	}
	return string(r[:limit]) + "…"
}
