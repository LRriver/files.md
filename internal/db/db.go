package db

import (
	"fmt"
	"sync"

	"zakirullin/stuffbot/pkg/tg"
)

// In-memory database
var filenameByMsgID sync.Map
var dirByMsgID sync.Map
var lastKeyboardMsgIDs sync.Map
var inputExpectations sync.Map

// DB Do we need a type at all?
type DB struct {
}

func NewDB() *DB {
	return &DB{}
}

// LastKeyboardMsgID returns 0 when there is no message with keyboard attached
func (db *DB) LastKeyboardMsgID(userID int64) int {
	id, ok := lastKeyboardMsgIDs.Load(lastKeyboardMsgIDKey(userID))
	if !ok {
		return 0
	}

	return id.(int)
}

func (db *DB) SetLastKeyboardMsgID(userID int64, ID int) {
	lastKeyboardMsgIDs.Store(lastKeyboardMsgIDKey(userID), ID)
}

func (db *DB) DelLastKeyboardMsgID(userID int64) {
	lastKeyboardMsgIDs.Delete(lastKeyboardMsgIDKey(userID))
}

func (db *DB) InputExpectation(userID int64) *tg.Cmd {
	val, ok := inputExpectations.Load(inputExpectationKey(userID))
	if !ok {
		return nil
	}

	cmd := val.(tg.Cmd)
	return &cmd
}

func (db *DB) SetInputExpectation(userID int64, cmd tg.Cmd) {
	inputExpectations.Store(inputExpectationKey(userID), cmd)
}

func (db *DB) DelInputExpectation(userID int64) {
	inputExpectations.Delete(inputExpectationKey(userID))
}

func (db *DB) SetFilenameByMsgID(userID int64, msgID int, filename string) {
	filenameByMsgID.Store(filenameByMsgIDKey(userID, msgID), filename)
}

func (db *DB) FilenameByMsgID(userID int64, msgID int) string {
	filename, _ := filenameByMsgID.Load(filenameByMsgIDKey(userID, msgID))

	return filename.(string)
}

func (db *DB) SetDirByMsgID(userID int64, msgID int, filename string) {
	dirByMsgID.Store(dirByMsgIDKey(userID, msgID), filename)
}

func (db *DB) dirByMsgID(userID int64, msgID int) string {
	filename, _ := dirByMsgID.Load(dirByMsgIDKey(userID, msgID))

	return filename.(string)
}

func lastKeyboardMsgIDKey(userID int64) string {
	return fmt.Sprintf("%d:lastKeyboardMsgIDs", userID)
}

func inputExpectationKey(userID int64) string {
	return fmt.Sprintf("%d:inputExpectations", userID)
}

func dirByMsgIDKey(userID int64, msgID int) string {
	return fmt.Sprintf("%d:dirByMsgID:%d", userID, msgID)
}

func filenameByMsgIDKey(userID int64, msgID int) string {
	return fmt.Sprintf("%d:filenameByMsgID:%d", userID, msgID)
}

// User-namespaced db key
func (db *DB) key(userID int64, key string) string {
	return fmt.Sprintf("%s:%d", key, userID)
}
