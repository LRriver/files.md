package db

import (
	"zakirullin/stuffbot/pkg/tg"
)

type FakeDB struct {
}

func NewFakeDB() *FakeDB {
	return &FakeDB{}
}

func (db *FakeDB) LastKeyboardMsgID(userID int64) int {
	return 0
}

func (db *FakeDB) SetLastKeyboardMsgID(userID int64, ID int) {

}

func (db *FakeDB) DelLastKeyboardMsgID(userID int64) {
}

func (db *FakeDB) InputExpectation(userID int64) *tg.Cmd {
	return nil
}

func (db *FakeDB) SetInputExpectation(userID int64, cmd tg.Cmd) {
}

func (db *FakeDB) DelInputExpectation(userID int64) {
}

func (db *FakeDB) SetFilenameByMsgID(userID int64, msgID int, filename string) {
}

func (db *FakeDB) FilenameByMsgID(userID int64, msgID int) string {
	return ""
}

func (db *FakeDB) SetDirByMsgID(userID int64, msgID int, filename string) {
}
