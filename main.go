package main

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

var (
	bot          *tgbotapi.BotAPI
	openaiClient *openai.Client
	sessions     = make(map[int64]*Session)
	mu           sync.Mutex
)

type Session struct {
	Step        string
	Photos      []string
	Name        string
	Accessories string
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки файла .env")
	}

	botToken := os.Getenv("TELEGRAM_API_KEY")
	openaiToken := os.Getenv("OPENAI_API_KEY")

	if botToken == "" || openaiToken == "" {
		log.Fatal("Требуется TELEGRAM_API_KEY и OPENAI_API_KEY")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}
	openaiClient = openai.NewClient(openaiToken)

	bot.Debug = false

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			handleCallback(update.CallbackQuery)
		}
	}
}

func handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	mu.Lock()
	session, exists := sessions[chatID]
	if !exists {
		session = &Session{}
		sessions[chatID] = session
	}
	mu.Unlock()

	if msg.Text != "" {
		switch session.Step {
		case "await_name":
			handleName(msg, session)
		case "await_accessories":
			handleAccessories(msg, session)
		default:
			if strings.ToLower(msg.Text) == "/start" {
				button := tgbotapi.NewInlineKeyboardButtonData("🎁 Сделать фигурку в упаковке", "make_figure")
				keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(button))
				msgCfg := tgbotapi.NewMessage(chatID, "Привет! Хочешь стать игрушкой?")
				msgCfg.ReplyMarkup = keyboard
				bot.Send(msgCfg)
			} else if strings.ToLower(msg.Text) == "готово" && len(session.Photos) > 0 {
				session.Step = "await_name"
				bot.Send(tgbotapi.NewMessage(chatID, "Теперь введи имя для фигурки 👤"))
			}
		}
	}

	if msg.Photo != nil && len(msg.Photo) > 0 {
		handlePhoto(msg, session)
	}
}

func handleCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID

	mu.Lock()
	sessions[chatID] = &Session{Step: "await_photos"}
	mu.Unlock()

	msg := tgbotapi.NewMessage(chatID, "Отправь мне одно, два или три фото своего лица 📸")
	bot.Send(msg)
}

func handlePhoto(msg *tgbotapi.Message, session *Session) {
	if msg.Photo != nil && len(msg.Photo) > 0 {
		photos := msg.Photo
		lastPhoto := photos[len(photos)-1]
		fileID := lastPhoto.FileID

		fileURL, err := bot.GetFileDirectURL(fileID)
		if err != nil {
			log.Println("Ошибка при получении URL файла:", err)
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Не удалось получить фото. Попробуй снова!"))
			return
		}

		session.Photos = append(session.Photos, fileURL)
		log.Println("Фото добавлено:", fileURL)

		if len(session.Photos) >= 3 {
			session.Step = "await_name"
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Теперь введи имя для фигурки 👤"))
		} else {
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Фото получено. Можешь отправить ещё, или напиши \"Готово\""))
		}
	}
}

func handleName(msg *tgbotapi.Message, session *Session) {
	session.Name = msg.Text
	session.Step = "await_accessories"
	bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Теперь опиши одежду и аксессуары для фигурки (например: «в чёрной куртке, с рюкзаком и кроссовками») 🎽🎧🎒"))
}

func handleAccessories(msg *tgbotapi.Message, session *Session) {
	session.Accessories = msg.Text

	description, err := analyzeImage(session.Photos)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Ошибка при анализе изображения: "+err.Error()))
		return
	}

	prompt := generatePrompt(session.Name, session.Accessories, description)

	imageURL, err := generateImage(prompt)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Ошибка при генерации изображения: "+err.Error()))
		return
	}

	photo := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileURL(imageURL))
	photo.Caption = "Вот твоя фигурка! 🔥"
	bot.Send(photo)

	delete(sessions, msg.Chat.ID)
}

func analyzeImage(photoURLs []string) (string, error) {
	ctx := context.Background()
	var media []openai.ChatMessagePart

	for _, url := range photoURLs {
		media = append(media, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL:    url,
				Detail: openai.ImageURLDetailHigh,
			},
		})
	}

	req := openai.ChatCompletionRequest{
		Model: openai.GPT4VisionPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: "user",
				MultiContent: append(media, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: "Опиши внешность человека на фото в контексте создания фигурки.",
				}),
			},
		},
		MaxTokens: 300,
	}

	resp, err := openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func generatePrompt(name, accessories, visionDescription string) string {
	return `Создай изображение в стиле пластиковой игровой фигурки в упаковке.

Имя: ` + name + `
Внешность (по фото): ` + visionDescription + `
Одежда и аксессуары (от пользователя): ` + accessories + `
Фон: картонный, минималистичный.
Все объекты должны выглядеть пластиковыми, как настоящая игрушка в упаковке.`
}

func generateImage(prompt string) (string, error) {
	ctx := context.Background()

	req := openai.ImageRequest{
		Model:          openai.CreateImageModelDallE3,
		Prompt:         prompt,
		Size:           openai.CreateImageSize1024x1024,
		ResponseFormat: openai.CreateImageResponseFormatURL,
		N:              1,
	}

	resp, err := openaiClient.CreateImage(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Data[0].URL, nil
}
