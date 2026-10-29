package domain

import "time"

const ReminderLead = time.Hour

func ReminderDue(expiresAt, now time.Time) time.Time {
	if due := expiresAt.Add(-ReminderLead); due.After(now) {
		return due
	}
	return now
}
