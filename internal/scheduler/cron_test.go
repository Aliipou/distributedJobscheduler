package scheduler_test

import (
	"testing"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/scheduler"
)

func TestNextRun(t *testing.T) {
	// Use a fixed reference time: 2024-01-15 12:00:00 UTC (Monday)
	ref := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		expr     string
		from     time.Time
		wantErr  bool
		validate func(t *testing.T, got time.Time)
	}{
		{
			name: "every_minute",
			expr: "* * * * *",
			from: ref,
			validate: func(t *testing.T, got time.Time) {
				expected := ref.Add(time.Minute)
				if !got.Equal(expected) {
					t.Errorf("got %v, want %v", got, expected)
				}
			},
		},
		{
			name: "every_6_hours",
			expr: "0 */6 * * *",
			from: ref,
			validate: func(t *testing.T, got time.Time) {
				// 12:00 → next 6h boundary is 18:00
				expected := time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("got %v, want %v", got, expected)
				}
			},
		},
		{
			name: "daily_midnight",
			expr: "0 0 * * *",
			from: ref,
			validate: func(t *testing.T, got time.Time) {
				expected := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("got %v, want %v", got, expected)
				}
			},
		},
		{
			name: "weekly_monday",
			expr: "0 9 * * 1",
			from: ref, // ref is Monday 12:00
			validate: func(t *testing.T, got time.Time) {
				// Next Monday 09:00 is next week
				expected := time.Date(2024, 1, 22, 9, 0, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("got %v, want %v", got, expected)
				}
			},
		},
		{
			name:    "invalid_expression",
			expr:    "not a cron",
			from:    ref,
			wantErr: true,
		},
		{
			name:    "empty_expression",
			expr:    "",
			from:    ref,
			wantErr: true,
		},
		{
			name:    "too_many_fields",
			expr:    "* * * * * * *",
			from:    ref,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scheduler.NextRun(tc.expr, tc.from)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.validate(t, got)
		})
	}
}

func TestValidateCron(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 */6 * * *",
		"0 0 * * *",
		"0 9 * * 1",
		"*/15 * * * *",
		"30 4 1,15 * 5",
	}
	for _, expr := range valid {
		t.Run(expr, func(t *testing.T) {
			if err := scheduler.ValidateCron(expr); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}

	invalid := []string{
		"",
		"not cron",
		"99 * * * *",
		"* 99 * * *",
	}
	for _, expr := range invalid {
		t.Run("invalid_"+expr, func(t *testing.T) {
			if err := scheduler.ValidateCron(expr); err == nil {
				t.Error("expected error for invalid expression")
			}
		})
	}
}

func TestNextRun_AlwaysInFuture(t *testing.T) {
	exprs := []string{"* * * * *", "0 */6 * * *", "0 0 * * *"}
	now := time.Now()
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			next, err := scheduler.NextRun(expr, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !next.After(now) {
				t.Errorf("next run %v is not after now %v", next, now)
			}
		})
	}
}
