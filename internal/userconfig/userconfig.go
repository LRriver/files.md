// Package userconfig stores user's configuration in file.
// It stores such settings for users as: language, home, quick buttons, schedule and so on.
package userconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Language            string   `json:"language"`
	HomeCmd             string   `json:"homeCmd"`
	MoveToConfigButtons []string `json:"moveToButtons"`
	PomodoroDuration    string   `json:"pomodoroDuration"`
}

var DefaultConfig = Config{
	Language:            "en",
	HomeCmd:             "today",
	MoveToConfigButtons: []string{"tomorrow", "later", "day", "note", "checklist", "doc", "recent", "journal"},
	PomodoroDuration:    "25m",
}

var TasksOnlyConfig = Config{
	HomeCmd:             "today",
	MoveToConfigButtons: []string{"tomorrow", "later", "day"},
}

var NotesOnlyConfig = Config{
	HomeCmd:             "notes",
	MoveToConfigButtons: []string{"##NOTE_DIRS##"},
}

func NewConfig() *Config {
	return &Config{}
}

// TODO add file creation
func (c *Config) LoadOrCreate(path string) error {
	configFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: %w", err)
	}
	defer configFile.Close()

	bytes, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: %w", err)
	}

	err = json.Unmarshal(bytes, c)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: can't unmarshal: %w", err)
	}

	return nil
}

func (c *Config) MoveToButtons() {

}

func (c *Config) Schedule() {

}

func (c *Config) Merge(config Config) {

}

func (c *Config) Save(path string) {

}

func mapConfigButtonNamesToRealNames(configNames []string) []string {
	configToReal := map[string]string{
		"tomorrow":  "🌚 For tmrw",
		"later":     "⏳ For later",
		"day":       "📆 For a day",
		"note":      "📌 To Note",
		"checklist": "☑️ To Checklist",
		"doc":       "📝 To Doc",
	}

	var realNames []string
	for _, configName := range configNames {
		realName, ok := configToReal[configName]
		if !ok {
			continue
		}

		realNames = append(realNames, realName)
	}

	return realNames
}
