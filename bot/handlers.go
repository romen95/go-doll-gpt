package bot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotHandler struct {
	Bot        *tgbotapi.BotAPI
	UserStates map[int64]string
}

func (h *BotHandler) HandleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		h.HandleMessage(update.Message)
	}

	if update.CallbackQuery != nil {
		h.HandleCallbackQuery(update.CallbackQuery)
		return
	}
}

func (h *BotHandler) HandleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	// Если пользователь отправляет фото
	if h.UserStates[userID] == "awaiting_photos" && len(message.Photo) > 0 {
		h.HandleUserPhotos(message)
		return
	}

	switch message.Text {
	case "/start":
		h.HandleStart(message)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "Неизвестная команда. Введите /start")
		if _, err := h.Bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}
	}
}

func (h *BotHandler) HandleCallbackQuery(callback *tgbotapi.CallbackQuery) {
	switch {
	case callback.Data == "make_figure":
		h.HandleMakeFigure(callback)
	}
}

func (h *BotHandler) HandleStart(message *tgbotapi.Message) {
	text := "Здесь ты можешь создать фигурку"

	buttonMakeFigure := tgbotapi.NewInlineKeyboardButtonData("🎁 Сделать фигурку в упаковке", "make_figure")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(buttonMakeFigure),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	if _, err := h.Bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}

func (h *BotHandler) HandleMakeFigure(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	userID := callback.From.ID

	msg := tgbotapi.NewMessage(chatID, "Пожалуйста, пришли от 1 до 3 своих фото.")
	if _, err := h.Bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}

	// Ответ на callback, чтобы убрать "загрузка..." у пользователя
	answerCallback := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.Bot.Request(answerCallback); err != nil {
		log.Printf("Ошибка при ответе на callback: %v", err)
	}

	// Сохраняем состояние, что ожидаем фото
	h.UserStates[userID] = "awaiting_photos"
}

func (h *BotHandler) HandleUserPhotos(message *tgbotapi.Message) {
	userID := message.From.ID
	chatID := message.Chat.ID

	if len(message.Photo) == 0 {
		log.Println("Нет фото в сообщении")
		return
	}

	photo := message.Photo[len(message.Photo)-1]

	file, err := h.Bot.GetFile(tgbotapi.FileConfig{FileID: photo.FileID})
	if err != nil {
		log.Printf("Ошибка получения файла: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Ошибка получения файла с сервера Telegram.")
		h.Bot.Send(msg)
		return
	}

	fileURL := "https://api.telegram.org/file/bot" + h.Bot.Token + "/" + file.FilePath
	log.Printf("Ссылка на файл: %s", fileURL)

	description, err := GetPhotoDescription(fileURL)
	if err != nil {
		log.Printf("Ошибка от GPT-4 Vision: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ GPT-4 Vision не смог описать фото: "+err.Error())
		h.Bot.Send(msg)
		return
	}

	finalPrompt := description + `
	Дорогой искусственный интеллект, создай пожалуйста изображение в стиле игровой фигурки упакованной в пластик, на основе prompt в предыдущей строке. 

Изображение должно содержать:
- Имя: Toy
- Второе имя: Romen
- Одежда: Черный худи, темносерые джинсы, светлосерые кроссовки
- Обьекты рядом с фигуркой:
'Accessories'
Объекты: макбук, собака шпиц, шайба снюса, бутылка колы
- Стиль фона: Картонный
минимализм, как в описании
оставь все объекты пластиковыми.

Сделайте изображение как можно более реалистичным - как будто это настоящая игрушка, которую можно найти в магазине.`

	imageURL, err := GenerateFigureImage(finalPrompt)
	if err != nil {
		log.Printf("Ошибка генерации изображения: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Не удалось создать изображение: "+err.Error())
		h.Bot.Send(msg)
		return
	}

	// Отправка пользователю
	photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(imageURL))
	photoMsg.Caption = "Вот твоя фигурка!"
	h.Bot.Send(photoMsg)

	h.UserStates[userID] = ""
}

func GetPhotoDescription(imageURL string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", errors.New("OpenAI API ключ не задан (проверь переменные окружения)")
	}

	requestBody := map[string]interface{}{
		"model": "gpt-4-turbo",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "Посмотри на это фото и сгенерируй prompt для DALL·E 3, чтобы сделать из этого человека игрушечную фигурку в упаковке.",
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageURL,
						},
					},
				},
			},
		},
		"max_tokens": 300,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		resBody, _ := io.ReadAll(resp.Body)
		log.Printf("Ошибка от OpenAI: %s", resBody)
		return "", errors.New("OpenAI API ответил ошибкой: " + string(resBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", errors.New("пустой ответ от GPT")
	}

	return result.Choices[0].Message.Content, nil
}

func GenerateFigureImage(prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", errors.New("OpenAI API ключ не задан")
	}

	requestBody := map[string]interface{}{
		"model":  "dall-e-3",
		"prompt": prompt,
		"n":      1,
		"size":   "1024x1024",
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		resBody, _ := io.ReadAll(resp.Body)
		log.Printf("Ошибка от OpenAI (DALL·E): %s", resBody)
		return "", errors.New("OpenAI API ответил ошибкой: " + string(resBody))
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Data) == 0 {
		return "", errors.New("DALL·E не вернул изображение")
	}

	return result.Data[0].URL, nil
}
