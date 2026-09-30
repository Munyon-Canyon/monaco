package domain

import (
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type State string

const (
	StatePreMarket  State = "pre_market"
	StateOpen       State = "open"
	StateAfterHours State = "after_hours"
	StateClosed     State = "closed"
)

type SessionInfo struct {
	State          State
	Continuous     bool
	Holiday        string
	EarlyClose     bool
	NextState      State
	NextTransition time.Time
	LastClose      time.Time
	TradingDay     Date
}

func Session(kind Kind, at time.Time) (SessionInfo, error) {
	switch kind {
	case KindPreIPO:
		return SessionInfo{State: StateOpen, Continuous: true, TradingDay: dateOf(at.UTC())}, nil
	case KindEquity:
		return calendar{years: nyseHolidays()}.session(at)
	}
	return SessionInfo{}, errs.New(errs.CodeInvalidInput, "market.Session", slog.String("kind", string(kind)))
}

type window struct {
	from  time.Duration
	state State
}

func regularHours() []window {
	return []window{
		{from: 4 * time.Hour, state: StatePreMarket},
		{from: 9*time.Hour + 30*time.Minute, state: StateOpen},
		{from: 16 * time.Hour, state: StateAfterHours},
		{from: 20 * time.Hour, state: StateClosed},
	}
}

func earlyCloseHours() []window {
	return []window{
		{from: 4 * time.Hour, state: StatePreMarket},
		{from: 9*time.Hour + 30*time.Minute, state: StateOpen},
		{from: 13 * time.Hour, state: StateAfterHours},
		{from: 17 * time.Hour, state: StateClosed},
	}
}

type schedule struct {
	windows []window
	holiday string
	early   bool
}

func (s schedule) stateAt(offset time.Duration) State {
	state := StateClosed
	for _, w := range s.windows {
		if w.from > offset {
			break
		}
		state = w.state
	}
	return state
}

func (s schedule) upcoming(offset time.Duration) (window, bool) {
	i := slices.IndexFunc(s.windows, func(w window) bool { return w.from > offset })
	if i < 0 {
		return window{}, false
	}
	return s.windows[i], true
}

func (s schedule) closeAt() (time.Duration, bool) {
	i := slices.IndexFunc(s.windows, func(w window) bool { return w.state == StateAfterHours })
	if i < 0 {
		return 0, false
	}
	return s.windows[i].from, true
}

type transition struct {
	state State
	at    time.Time
}

type calendar struct {
	years map[int][]exception
}

func (c calendar) on(date Date) (schedule, error) {
	exceptions, covered := c.years[date.Year]
	if !covered {
		return schedule{}, errs.New(errs.CodeCalendarExpired, "market.calendar.on", slog.String("date", date.String()))
	}
	if i := slices.IndexFunc(exceptions, func(e exception) bool { return e.matches(date) }); i >= 0 {
		return exceptions[i].schedule(), nil
	}
	if weekday := date.weekday(); weekday == time.Saturday || weekday == time.Sunday {
		return schedule{}, nil
	}
	return schedule{windows: regularHours()}, nil
}

func (c calendar) session(at time.Time) (SessionInfo, error) {
	wall := wallClock(at)
	date := dateOf(wall)
	today, err := c.on(date)
	if err != nil {
		return SessionInfo{}, err
	}
	offset := wall.Sub(date.midnight())
	next, err := c.nextTransition(date, today, offset)
	if err != nil {
		return SessionInfo{}, err
	}
	lastClose, err := c.lastClose(date, today, offset)
	if err != nil {
		return SessionInfo{}, err
	}
	return SessionInfo{
		State:          today.stateAt(offset),
		Holiday:        today.holiday,
		EarlyClose:     today.early,
		NextState:      next.state,
		NextTransition: next.at,
		LastClose:      lastClose,
		TradingDay:     date,
	}, nil
}

func (c calendar) nextTransition(date Date, today schedule, offset time.Duration) (transition, error) {
	if w, ok := today.upcoming(offset); ok {
		return transition{state: w.state, at: date.atExchange(w.from)}, nil
	}
	for next := date.addDays(1); ; next = next.addDays(1) {
		day, err := c.on(next)
		if err != nil {
			return transition{}, err
		}
		if len(day.windows) > 0 {
			return transition{state: day.windows[0].state, at: next.atExchange(day.windows[0].from)}, nil
		}
	}
}

func (c calendar) lastClose(date Date, today schedule, offset time.Duration) (time.Time, error) {
	if from, ok := today.closeAt(); ok && offset >= from {
		return date.atExchange(from), nil
	}
	for previous := date.addDays(-1); ; previous = previous.addDays(-1) {
		day, err := c.on(previous)
		if err != nil {
			return time.Time{}, err
		}
		if from, ok := day.closeAt(); ok {
			return previous.atExchange(from), nil
		}
	}
}
