package habits

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"zakirullin/stuffbot/internal/fs"
)

//go:embed templates/habits.html
var html string

func Render(botFS *fs.FS) ([]byte, error) {
	tmpl, err := template.New("habits").Parse(html)
	if err != nil {
		return nil, fmt.Errorf("can't parse habits template: %w", err)
	}

	habits, err := LastWeekHabits(botFS)
	if err != nil {
			return nil, fmt.Errorf("can't render habit: %w", err)
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, habits)
	if err != nil {
		return nil, fmt.Errorf("can't render habits template: %w", err)
	}

	return out.Bytes(), nil
}
