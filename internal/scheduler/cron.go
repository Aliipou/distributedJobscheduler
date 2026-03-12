package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// NextRun returns the next scheduled time after 'from' for the given cron expression.
func NextRun(expr string, from time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return schedule.Next(from), nil
}

// ValidateCron returns an error if the cron expression is invalid.
func ValidateCron(expr string) error {
	_, err := NextRun(expr, time.Now())
	return err
}
