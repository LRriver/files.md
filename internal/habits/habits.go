package habits

import (
	"errors"
	"fmt"
	"slices"
	"sort"
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

	mood = "Mood"
)

var (
	moodEmojis            = []string{"⚪️", "🤕", "😔", "😐", "🙂", "😊"}
	errMalformedMonthLine = errors.New("malformed month line")
	now                   = time.Now
)

// What if there's no file? Create empty from ./habits
func Habits(userFS *fs.FS, year int) (map[string]Year, error) {
	filename := fmt.Sprintf("%d Habits.md", year)
	habitsStr, err := userFS.Read(fs.DirInsights, filename)
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
		// TODO preserve gibberish between parsing seesions
		daysAndHabit := strings.SplitN(line, " ", 2)
		if len(daysAndHabit) < 2 {
			continue
		}
		days, habit := daysAndHabit[0], daysAndHabit[1]

		firstDayOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		dayOfTheYear := firstDayOfMonth.YearDay()

		// Moods line
		moodsMarker := mood
		if strings.Contains(habit, moodsMarker) {
			gr := uniseg.NewGraphemes(days)
			dayOffset := 0
			habits[mood] = make(Year)
			for gr.Next() {
				power := slices.Index(moodEmojis, gr.Str())
				habits[mood][dayOfTheYear+dayOffset] = power
				dayOfTheYear++
			}
			continue
		}

		// Skip gibberish
		habitsMarker := fmt.Sprintf("%s%s%s", habitSkipped, habitCompletedAtWeekend, habitCompleted)
		if !strings.ContainsAny(days, habitsMarker) {
			continue
		}

		// Habits line
		// [⚪️🟢... Habit name] i.e. completion status
		// for every day of the above found month
		habitName := strings.TrimSpace(habit)
		if _, ok := habits[habitName]; !ok {
			habits[habitName] = make(Year)
		}

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

func LastWeekHabits(userFS *fs.FS) (map[string]Year, error) {
	habitsForYear, err := Habits(userFS, now().Year())
	if err != nil {
		return nil, fmt.Errorf("can't get habits for last week: %w", err)
	}

	currentDay := now()
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

func Write(userFS *fs.FS, year int, habits map[string]Year) error {
	habitKeys := make([]string, 0)
	for k, _ := range habits {
		if k == mood {
			continue
		}
		habitKeys = append(habitKeys, k)
	}
	sort.Strings(habitKeys)
	if _, ok := habits[mood]; ok {
		habitKeys = append(habitKeys, mood)
	}

	for _, k := range habitKeys {
		fmt.Println(k, habits[k])
	}

	content := ""
	day := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	for day.Year() < year+1 {
		// Generating all habits for a month
		habitsForMonth := ""
		for _, habitName := range habitKeys {
			statuses := ""
			dayOfMonth := day
			atLeastOneCompletion := false
			for dayOfMonth.Month() == day.Month() {
				emoji := habitSkipped
				if status, ok := habits[habitName][dayOfMonth.YearDay()]; ok {
					emoji = emojiForStatus(habitName, dayOfMonth, status)
				}
				if emoji != habitSkipped {
					atLeastOneCompletion = true
				}
				statuses += emoji

				dayOfMonth = dayOfMonth.AddDate(0, 0, 1)
			}
			if atLeastOneCompletion {
				habitsForMonth += fmt.Sprintf("%s %s\n", statuses, habitName)
			}
		}

		if len(habitsForMonth) != 0 {
			content += fmt.Sprintf("### %s\n%s\n", day.Month(), habitsForMonth)
		}

		day = day.AddDate(0, 1, 0)
	}

	filename := fmt.Sprintf("%d Habits.md", year)
	err := userFS.Write(fs.DirInsights, filename, content)
	if err != nil {
		return fmt.Errorf("can't write habits: %w", err)
	}

	return nil
}

func emojiForStatus(habitName string, day time.Time, status int) string {
	if habitName == mood {
		if status < len(moodEmojis) {
			return moodEmojis[status]
		}

		return habitSkipped
	}

	if status == 1 {
		isWeekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		if isWeekend {
			return habitCompletedAtWeekend
		} else {
			return habitCompleted
		}
	} 

	return habitSkipped
}

// func dayOfYearToTime(dayOfYear int, year int) time.Time {
// 	startOfYear := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)

// 	return startOfYear.AddDate(0, 0, dayOfYear-1)
// }
