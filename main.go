package main

import (
	"log"
	"os"

	"godollgpt/bot"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки файла .env")
	}
	botToken := os.Getenv("TELEGRAM_API_KEY")
	handler := bot.NewBotHandler(botToken)

	log.Println("Запуск Telegram-бота...")
	handler.Run()
}
