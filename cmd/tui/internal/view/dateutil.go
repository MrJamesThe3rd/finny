package view

import (
	"time"
)

type Timeframe int

const (
	TimeframeThisWeek  Timeframe = 0
	TimeframeLastWeek  Timeframe = 1
	TimeframeThisMonth Timeframe = 2
	TimeframeLastMonth Timeframe = 3
	TimeframeAll       Timeframe = 4
	TimeframeCustom    Timeframe = 5
)

func (t Timeframe) String() string {
	switch t {
	case TimeframeThisWeek:
		return "This Week"
	case TimeframeLastWeek:
		return "Last Week"
	case TimeframeThisMonth:
		return "This Month"
	case TimeframeLastMonth:
		return "Last Month"
	case TimeframeAll:
		return "All Time"
	case TimeframeCustom:
		return "Custom Range"
	}

	return "Unknown"
}

func TimeframeToDateRange(tf Timeframe) (time.Time, time.Time) {
	now := time.Now()

	var start, end time.Time

	switch tf {
	case TimeframeThisWeek:
		// Start of week (Monday)
		// ISO week starts Monday.
		offset := int(now.Weekday())
		if offset == 0 {
			offset = 7
		} // Sunday -> 7

		start = now.AddDate(0, 0, -offset+1)
		end = now
	case TimeframeLastWeek:
		offset := int(now.Weekday())
		if offset == 0 {
			offset = 7
		}

		end = now.AddDate(0, 0, -offset)
		start = end.AddDate(0, 0, -6)
	case TimeframeThisMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = now
	case TimeframeLastMonth:
		lastMonth := now.AddDate(0, -1, 0)
		start = time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, lastMonth.Location())
		end = start.AddDate(0, 1, -1)
	}

	return start, end
}

func NormalizeDateRange(start time.Time, end time.Time) (time.Time, time.Time) {
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
		time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, time.UTC)
}
