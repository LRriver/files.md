package internal

import (
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/pkg/tg"
)

func TestReadMessagesEmpty(t *testing.T) {
	r := require.New(t)
	result := readMessages("")
	r.Empty(result)
}

func TestReadMessagesOnlyHeader(t *testing.T) {
	r := require.New(t)
	result := readMessages("#### 27 June, Friday")
	r.Equal([]string{"#### 27 June, Friday"}, result)
}

func TestReadMessagesSingleRecord(t *testing.T) {
	r := require.New(t)
	result := readMessages("`01:01` Simple record")
	r.Equal([]string{"`01:01` Simple record"}, result)
}

func TestReadMessagesHeaderWithRecord(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` Simple record"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Simple record"}, result)
}

func TestReadMessagesMultilineRecord(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` Multiline\nc\nontent"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Multiline\nc\nontent"}, result)
}

func TestReadMessagesMultipleRecords(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` First record\n`02:02` Second record"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` First record", "`02:02` Second record"}, result)
}

func TestReadMessagesMultipleHeaders(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` First day\n#### 28 June, Saturday\n`02:02` Second day"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` First day", "#### 28 June, Saturday", "`02:02` Second day"}, result)
}

func TestReadMessagesWindowsLineEndings(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\r\n`01:01` Windows record"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Windows record"}, result)
}

func TestReadMessagesWithEmptyLines(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n\n`01:01` Record with\n\nempty lines"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Record with\n\nempty lines"}, result)
}

func TestReadMessagesInvalidTimestamp(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`not timestamp` Should be continuation\n`01:01` Real record"
	result := readMessages(content)
	r.Equal([]string{"#### 27 June, Friday", "`not timestamp` Should be continuation", "`01:01` Real record"}, result)
}

func TestSaveToChatNewFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToChat("Test content", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` Test content\n", content)
}

func TestSaveToChatExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.ChatFilename, "#### 27 June, Thursday\n`00:30` Existing content\n")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToChat("New content", time.UTC)
	r.NoError(err)
	r.Equal(1, index)

	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`00:30` Existing content\n`01:01` New content\n", content)
}

func TestSaveToChatNewDay(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 28, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.ChatFilename, "#### 27 June, Thursday\n`00:30` Yesterday content\n")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToChat("Today content", time.UTC)
	r.NoError(err)
	r.Equal(1, index)

	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`00:30` Yesterday content\n#### 28 June, Friday\n`01:01` Today content\n", content)
}

func TestSaveToChatWithImage(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToChat("![](image.jpg) Image description", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` ![](image.jpg) Image description\n", content)
}

func TestSaveToChatEmptyFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.ChatFilename, "")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToChat("Test content", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` Test content\n", content)
}

//func TestSaveToChatWithTimezone(t *testing.T) {
//	r := require.New(t)
//
//	savedNow := now
//	defer func() { now = savedNow }()
//	now = func() time.Time {
//		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
//	}
//
//	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//	r.NoError(err)
//
//	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
//
//	// Use EST timezone (UTC-5)
//	est, err := time.LoadLocation("America/New_York")
//	r.NoError(err)
//
//	index, err := bot.saveToChat("Test content", est)
//	r.NoError(err)
//	r.Equal(1, index)
//
//	content, err := userFS.Read(fs.DirRoot, fs.ChatFilename)
//	r.NoError(err)
//	// Should use EST time which is 20:01 (8:01 PM) the previous day
//	r.Contains(content, "`20:01` Test content")
//}
