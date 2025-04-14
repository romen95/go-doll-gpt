package main

import (
	"log"
	"os"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Session struct {
	Step    string
	Photos  []string // file_id
	Name    string
	Clothes string
	Objects string
}

var (
	userSessions = make(map[int64]*Session)
	sessionMutex sync.Mutex
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is not set")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, update.Message)
		}
		if update.CallbackQuery != nil {
			handleCallback(bot, update.CallbackQuery)
		}
	}
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	session := getSession(chatID)

	switch session.Step {
	case "await_photos":
		if msg.Photo != nil {
			lastPhoto := msg.Photo[len(msg.Photo)-1]
			session.Photos = append(session.Photos, lastPhoto.FileID)

			if len(session.Photos) >= 3 {
				session.Step = "await_name"
				bot.Send(tgbotapi.NewMessage(chatID, "Теперь введи имя для фигурки 👤"))
			} else {
				bot.Send(tgbotapi.NewMessage(chatID, "Фото получено. Можешь отправить ещё, или напиши \"Готово\""))
			}
		} else if msg.Text == "Готово" && len(session.Photos) > 0 {
			session.Step = "await_name"
			bot.Send(tgbotapi.NewMessage(chatID, "Отлично! Теперь введи имя для фигурки 👤"))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "Пожалуйста, отправь фото или напиши \"Готово\""))
		}
	case "await_name":
		session.Name = msg.Text
		session.Step = "await_clothes"
		bot.Send(tgbotapi.NewMessage(chatID, "Какая одежда на фигурке? 🧥"))
	case "await_clothes":
		session.Clothes = msg.Text
		session.Step = "await_objects"
		bot.Send(tgbotapi.NewMessage(chatID, "Какие объекты будут рядом с игрушкой? 🎁"))
	case "await_objects":
		session.Objects = msg.Text
		session.Step = "ready"

		// Здесь будет отправка в OpenAI
		bot.Send(tgbotapi.NewMessage(chatID, "Отлично! Начинаю создавать фигурку... 🛠️"))

		// Вызываем заглушку для генерации
		go generateToyFigure(bot, chatID, session)
	default:
		bot.Send(tgbotapi.NewMessage(chatID, "Нажми кнопку /start, чтобы начать 👋"))
	}
}

func handleCallback(bot *tgbotapi.BotAPI, cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	if cb.Data == "make_figure" {
		resetSession(chatID)
		session := getSession(chatID)
		session.Step = "await_photos"

		bot.Send(tgbotapi.NewMessage(chatID, "Отправь мне одно, два или три фото своего лица 📸"))
	}
}

func getSession(chatID int64) *Session {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	if session, exists := userSessions[chatID]; exists {
		return session
	}

	userSessions[chatID] = &Session{}
	return userSessions[chatID]
}

func resetSession(chatID int64) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	userSessions[chatID] = &Session{}
}

func generateToyFigure(bot *tgbotapi.BotAPI, chatID int64, s *Session) {
	// Здесь будет логика отправки в OpenAI и генерации картинки
	// Сейчас просто заглушка:

	prompt := generatePrompt(s)
	log.Println("PROMPT:\n", prompt)

	// Тут можно добавить вызов OpenAI API
	bot.Send(tgbotapi.NewMessage(chatID, "Фигурка почти готова! (Здесь будет результат генерации) 🧸"))
}

func generatePrompt(s *Session) string {
	return "Дорогой искусственный интеллект, создай пожалуйста изображение в стиле игровой фигурки упакованной в пластик, на основе изображения, которое я прикрепил к сообщению.\n\n" +
		"Изображение должно содержать:\n" +
		"- Имя: " + s.Name + "\n" +
		"- Одежда: " + s.Clothes + "\n" +
		"- Объекты рядом с фигуркой:\n'Accessories'\nОбъекты: " + s.Objects + "\n" +
		"- Стиль фона: Картонный минимализм, как в описании\n" +
		"Оставь все объекты пластиковыми.\n\n" +
		"Сделайте изображение как можно более реалистичным - как будто это настоящая игрушка, которую можно найти в магазине."
}
