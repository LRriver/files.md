package habits

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/uniseg"

	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/pkg/txt"
)

// [1 => false, <year day> => 0, 1...]
type Year map[int]int

const (
	habitSkipped            = "⚪️"
	habitCompleted          = "🟢"
	habitCompletedAtWeekend = "🟡"
)

var (
	errMalformedMonthLine = errors.New("malformed month line")
	now                   = time.Now
)

func Habits(botFS *fs.FS, year int) (map[string]Year, error) {
	filename := "%d Habits.md"
	habitsStr, err := botFS.Read(fs.DirInsights, fmt.Sprintf(filename, year))
	if err != nil {
		return nil, fmt.Errorf("read %s error: %w", filename, err)
	}

	habits := make(map[string]Year)
	month := time.January
	lines := strings.Split(txt.NormNewLines(habitsStr), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Parsing month line
		isMonthLine := strings.HasPrefix(line, "###")
		if isMonthLine {
			parts := strings.Split(line, " ")
			if len(parts) < 2 {
				return nil, fmt.Errorf("read habits: can't parse month line '%s': %w", line, errMalformedMonthLine)
			}

			date, err := time.Parse("January", parts[1])
			if err != nil {
				return nil, fmt.Errorf("read habits: can't parse month %s: %w", line, err)
			}
			month = date.Month()

			continue
		}

		// Tolerant reader, if we encounter gibberish,
		// we skip it. See ADRs in README.md for details for details
		isHabbitLine := strings.ContainsAny(line, fmt.Sprintf("%s%s", habitSkipped, habitCompleted))
		if !isHabbitLine {
			continue
		}

		// At this point we are on habits line, which is
		// [⚪️🟢... Habit name] i.e. completion status
		// for every day of the above found month

		daysAndHabit := strings.SplitN(line, " ", 2)
		if len(daysAndHabit) < 2 {
			return nil, nil
			// return "bad month line: %s"
		}
		habitName := strings.TrimSpace(daysAndHabit[1])
		if _, ok := habits[habitName]; !ok {
			habits[habitName] = make(Year)
		}

		firstDayOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		dayOfTheYear := firstDayOfMonth.YearDay()

		days := daysAndHabit[0]
		// See README.md ADRs
		gr := uniseg.NewGraphemes(days)
		dayOffset := 0
		for gr.Next() {
			habits[habitName][dayOfTheYear+dayOffset] = 0
			if gr.Str() != habitSkipped {
				habits[habitName][dayOfTheYear+dayOffset] = 1
			}
			dayOfTheYear++
		}
	}

	return habits, nil
}

func LastWeekHabits(botFS *fs.FS) (map[string]Year, error) {
	habitsForYear, err := Habits(botFS, now().Year())
	if err != nil {
		return nil, fmt.Errorf("can't get habits for last week: %w", err)
	}

	var currentDay = now()
	for currentDay.Weekday() != time.Monday {
		currentDay = currentDay.Add(-24 * time.Hour)
	}

	habits := make(map[string]Year)
	for habit, statuses := range habitsForYear {
		habits[habit] = make(Year)
		for offset := range 7 {
			yearDay := currentDay.Add(time.Duration(offset) * 24 * time.Hour).YearDay()
			habits[habit][yearDay] = 0
			if status, ok := statuses[yearDay]; ok {
				habits[habit][yearDay] = status
			}
		}
	}

	return habits, nil
}

// func Write(botFS *fs.FS, habits []Habit) error {
// 	return nil
// }
