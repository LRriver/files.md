package habits

// TODO one known bug - it won't correctly work
// if our week falls into 2 different years

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/fs"
)

//go:embed testdata/month_habits.md
var monthMD string

//go:embed testdata/last_month_habits.md
var lastMonthMD string

//go:embed testdata/two_months_habits.md
var twoMonthsMD string

func TestHabits(t *testing.T) {
	r := require.New(t)

	botFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	botFS.Write(fs.DirInsights, "1970 Habits.md", monthMD)

	habits, err := Habits(botFS, 1970)
	r.NoError(err)

	r.Len(habits, 6)
	year, ok := habits["Went to gym"]
	r.True(ok)

	r.Len(year, 31)

	completed, ok := year[1]
	r.True(ok)
	r.Equal(false, completed)

	completed, ok = year[31]
	r.True(ok)
	r.Equal(true, completed)
}

func TestLastMonthHabits(t *testing.T) {
	r := require.New(t)

	botFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	botFS.Write(fs.DirInsights, "1970 Habits.md", lastMonthMD)

	habits, err := Habits(botFS, 1970)
	r.NoError(err)

	r.Len(habits, 1)
	year, ok := habits["Habit"]
	r.True(ok)

	r.Len(year, 31)

	fmt.Printf("%v", year)
	completed, ok := year[335]
	r.True(ok)
	r.Equal(false, completed)

	completed, ok = year[365]
	r.True(ok)
	r.Equal(true, completed)
}

func TestLastWeekHabitsWhenWeekFallsIntoTwoMonths(t *testing.T) {
	r := require.New(t)

	botFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	botFS.Write(fs.DirInsights, "1970 Habits.md", twoMonthsMD)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, time.September, 30, 0, 0, 0, 0, time.Local)
	}

	habits, err := LastWeekHabits(botFS)
	r.NoError(err)
	r.Len(habits, 1)
	r.Len(habits["Habit"], 7)
	r.EqualValues(map[int]bool{271: false, 272: true, 273: false, 274: false, 275: false, 276: true, 277: false}, habits["Habit"])
}
