// Package stats generates fancy reports
// containing completed tasks and habits, checked items and so on
package stats

import (
	"fmt"
	"strings"
	"time"

	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched"
)

type DBInterface interface {
}

var now = time.Now

func TodayReport(userFS *fs.FS, db DBInterface, userID int64) (string, error) {
	files, err := DoneToday(userFS, db, userID)
	if err != nil {
		return "", fmt.Errorf("stats.TodayReport: %w", err)
	}

	var stats []string
	for _, file := range files {
		ico := icon(file)
		stats = append(stats, fmt.Sprintf("%s <b>%s</b>", ico, fs.Title(file)))
	}

	trashedFiles, err := userFS.FilesAndDirs(fs.DirArchive)
	if err != nil {
		return "", fmt.Errorf("stats.TodayReport: can't get trashed files: %w", err)
	}
	doneTotal := len(trashedFiles)
	stats = append(stats, fmt.Sprintf("📊 %d tasks done in total", doneTotal))

	return strings.Join(stats, "\n"), nil
}

func icon(filename string) string {
	if strings.HasPrefix("-read-", filename) {
		return "📚"
	}

	if strings.HasPrefix("-watch-", filename) {
		return "📺"
	}

	if strings.HasPrefix("-shop-", filename) {
		return "🛒"
	}

	if fs.IsChecklistItem(filename) {
		return "☑️"
	}

	return "✅"
}

func DoneToday(userFS *fs.FS, db DBInterface, userID int64) ([]string, error) {
	return doneToday(userFS, db, userID, false)
}

func DoneTodayScheduled(userFS *fs.FS, db *db.DB, userID int64) ([]string, error) {
	return doneToday(userFS, db, userID, true)
}

func doneToday(userFS *fs.FS, db DBInterface, userID int64, withScheduled bool) ([]string, error) {
	files, err := userFS.FilesAndDirs(fs.DirArchive)
	if err != nil {
		return nil, fmt.Errorf("stats.DoneTasks: %w", err)
	}

	var todayFiles []fs.File
	for _, task := range files {
		if task.Ctime > sched.BeginningOfTheDay(now()).Unix() {
			todayFiles = append(todayFiles, task)
		}
	}

	//sch, err := db.Schedule(userID)
	//if err != nil {
	//	return nil, fmt.Errorf("stats.DoneTasks: %w", err)
	//}

	var todayFiltered []string
	//for _, todayFile := range todayFiles {
	//	if _, scheduled := sch[todayFile.Name]; scheduled == withScheduled {
	//		todayFiltered = append(todayFiltered, todayFile.Title)
	//	}
	//}

	return todayFiltered, nil
}
