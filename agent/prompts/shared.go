package prompts

import (
	"fmt"
	"time"
)

// TimeParams contains parameters for formatting time
type TimeParams struct {
	Date   time.Time
	Locale string // e.g., "en-US", "zh-CN"
}

// Time formats the date and time according to the locale
func Time(params TimeParams) string {
	dateStr := params.Date.Format("2006-01-02")
	timeStr := params.Date.Format("15:04:05")

	return fmt.Sprintf("date: %s\ntime: %s", dateStr, timeStr)
}
