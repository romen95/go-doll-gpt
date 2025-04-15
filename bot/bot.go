package bot

import (
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// NewBotHandler - создаёт нового обработчика бота
func NewBotHandler(botToken string) *BotHandler {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	bot.Debug = false

	log.Printf("Бот запущен: %s", bot.Self.UserName)

	handler := &BotHandler{
		Bot:        bot,
		UserStates: make(map[int64]string),
	}

	return handler
}

func (h *BotHandler) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := h.Bot.GetUpdatesChan(u)

	botStartTime := time.Now()

	for update := range updates {
		if update.CallbackQuery != nil {
			callbackTime := time.Unix(int64(update.CallbackQuery.Message.Date), 0)
			if callbackTime.Before(botStartTime) {
				hint := tgbotapi.NewCallback(update.CallbackQuery.ID, "Бот был перезагружен, используйте /start")
				_, _ = h.Bot.Request(hint)
				continue
			}
			h.HandleUpdate(update)
			continue
		}

		if update.Message != nil {
			messageTime := time.Unix(int64(update.Message.Date), 0)
			if messageTime.Before(botStartTime) {
				continue
			}
			h.HandleUpdate(update)
		}
	}
}
