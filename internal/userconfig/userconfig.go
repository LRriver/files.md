// Package userconfig stores user's configuration in file.
// It stores such settings for users as: language, home, quick buttons, schedule and so on.
package userconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/exp/slog"

	"zakirullin/dumpbot/i18n"
)

var DefaultConfig = Config{
	raw: raw{
		Language:               "en",
		HomeCmd:                "today",
		MoveToCommands:         []string{"tomorrow", "later", "day", "note", "checklist", "doc", "recent", "journal"},
		PomodoroDurationMinute: 25,
	},
}

var TasksOnlyConfig = Config{
	raw: raw{
		HomeCmd:        "today",
		MoveToCommands: []string{"tomorrow", "later", "day"},
	},
}

var NotesOnlyConfig = Config{
	raw: raw{
		HomeCmd:        "notes",
		MoveToCommands: []string{"##NOTE_DIRS##"},
	},
}

type Config struct {
	raw
}

type raw struct {
	Language               string   `json:"language"`
	HomeCmd                string   `json:"homeCmd"`
	MoveToCommands         []string `json:"moveToCommands"`
	PomodoroDurationMinute float64  `json:"pomodoroDurationMinute"`
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &c.raw)
}

// TODO add file creation
func (c *Config) LoadOrCreate(path string) error {
	configFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}
	defer configFile.Close()

	bytes, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	err = json.Unmarshal(bytes, c)
	if err != nil {
		return fmt.Errorf("config load: can't unmarshal: %w", err)
	}

	return nil
}

func (c *Config) Save(path string) {

}

func (c *Config) MoveToCmds() []string {
	configToReal := map[string]string{
		"tomorrow":  i18n.StrForTomorrow,
		"later":     i18n.StrForLater,
		"day":       i18n.StrForDay,
		"note":      i18n.StrToNote,
		"checklist": i18n.StrToChecklist,
		"doc":       i18n.StrToDoc,
	}

	var realCmds []string
	for _, configName := range c.raw.MoveToCommands {
		realName, ok := configToReal[configName]
		if !ok {
			continue
		}

		realCmds = append(realCmds, realName)
	}

	return realCmds
}

func (c *Config) SetPomodoroDuration(value time.Duration) error {
	if value <= 0 || value > 24*time.Hour {
		return fmt.Errorf("raw.SetPomodoroDuration: value is invalid: %v", value)
	}
	c.raw.PomodoroDurationMinute = value.Minutes()
	return nil
}

func (c *Config) PomodoroDuration() time.Duration {
	minutes := c.raw.PomodoroDurationMinute
	if minutes <= 0 {
		slog.Error("Pomodoro duration is invalid. Using default value", "duration",
			c.raw.PomodoroDurationMinute, "default", DefaultConfig.raw.PomodoroDurationMinute)
		//I don't use DefaultConfig.PomodoroDuration() because it may cause infinite recursion
		minutes = DefaultConfig.raw.PomodoroDurationMinute
	}
	return time.Duration(minutes * float64(time.Minute))
}
