package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched/worker"
	"zakirullin/stuffbot/internal/server"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading .env file: %s\n", err))
	}
	err = config.LoadBotConfig()
	if err != nil {
		panic(fmt.Sprintf("Error loading cfg: %s\n", err))
	}

	// TODO move to embed
	err = i18n.LoadLangFile("i18n/ru.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading i18n: %s\n", err))
	}

	api, err := tgbotapi.NewBotAPI(config.BotCfg.BotAPIToken)
	if err != nil {
		panic(fmt.Sprintf("Can't create api: %s\n", err))
	}
	telegram := tg.NewTG(api)

	// Save all renames and deletes to an append-only log.
	fs.LogRename = server.LogRename
	fs.LogDelete = server.LogDelete

	// Due tasks scheduler
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	defer func(quit chan struct{}) {
		close(quit)
	}(quit)
	go func(tg *tg.TG) {
		fsBackend := afero.NewOsFs()
		for {
			select {
			case <-ticker.C:
				err := worker.MoveDueTasks(config.BotCfg.StorageDir, config.BotCfg.ConfigFilename, fsBackend, telegram)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}(telegram)

	// TODO apphost?
	// Launch habits server if needed
	if config.BotCfg.ApiHost != "" {
		go server.Serve(
			config.BotCfg.ApiHost,
			config.BotCfg.AppHost,
			config.BotCfg.ServerCertDir,
			config.BotCfg.ServerLogFile,
			config.BotCfg.AuthToken, // TODO rem
			config.BotCfg.TokensDir,
		)
	}

	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Main bot loop.
	// Loop through updates from Telegram and process them sequentially in separate per-user goroutine.
	userChannels := make(map[int64]chan tgbotapi.Update)
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60 // TODO release, check if it's enough
	updates := api.GetUpdatesChan(tgConfig)
	for update := range updates {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Bot panic", "update", update, "err", err, "stacktrace", string(debug.Stack()))
				}
			}()

			updJSON, _ := json.Marshal(update)
			infolog.Info("Bot update", "update", string(updJSON))

			var userID int64
			upd := tg.NewTGUpd(update)
			channelID, channelIDExists := upd.ChannelID()
			if channelIDExists {
				userID, err = telegram.ChannelCreatorID(channelID)
				if err != nil {
					slog.Error("Bot error: can't get channel creator ID", "upd", string(updJSON), "err", err)
				}
			} else {
				userID = upd.UserID()
			}

			userCh, channelIDExists := userChannels[userID]
			if !channelIDExists {
				userCh = make(chan tgbotapi.Update, 100)
				userChannels[userID] = userCh
				go supervisor(userID, userCh, telegram)
			}

			userCh <- update
		}()
	}
}

// Runs per-user worker that listens for updates.
// Restarts infinitely upon panics.
func supervisor(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for {
		func() {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("Bot panic", "userID", userID, "err", err, "stacktrace", string(debug.Stack()))
				}
			}()
			processUserUpdates(userID, updates, telegram)
		}()
		time.Sleep(time.Second)
		slog.Info("Restarting worker", "userID", userID)
	}
}

func processUserUpdates(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for update := range updates {
		upd := tg.NewTGUpd(update)

		storagePath := config.BotCfg.StorageDir
		storagePath, err := filepath.Abs(storagePath)
		userPath := path.Join(storagePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, afero.NewOsFs())
		if err != nil {
			slog.Error("Bot error: can't create fs", "err", err)
			return
		}
		err = userFS.CreateDirsIfNotExist()
		if err != nil {
			slog.Error("Bot error: can't create user dirs", "err", err)
			return
		}

		confFilename := config.BotCfg.ConfigFilename
		userconf := userconfig.NewConfig(userFS, userID, confFilename)
		err = userconf.CreateDefaultIfNotExists()
		if err != nil {
			slog.Error("Bot error: can't create default user config", "err", err)
			return
		}

		bot := internal.NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)
		if err := bot.Reply(upd); err != nil {
			slog.Error("Bot error", "err", err)
		}
	}
}
