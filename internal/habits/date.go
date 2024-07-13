package habits

import "time"

func DayOfYearToDate(year int, dayOfYear int) string {
	startOfYear := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	date := startOfYear.AddDate(0, 0, dayOfYear-1)

	return date.Format("25.01")
}
