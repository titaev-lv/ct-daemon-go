package main

import (
	"database/sql"
	"log"
	"time"

	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
)

func main() {
	// Настройка логирования
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Создание объекта Exchange для адаптера
	exchangeConfig := db.Exchange{
		ID:           1,
		Name:         "CoinEx",
		Active:       true,
		Url:          "https://api.coinex.com",
		BaseUrl:      "https://api.coinex.com",
		WebsocketUrl: sql.NullString{String: "wss://socket.coinex.com/", Valid: true},
		WsUrl:        sql.NullString{String: "wss://socket.coinex.com/", Valid: true},
	}

	// Создание адаптера CoinEx
	adapter := exchange.NewCoinexAdapter(exchangeConfig)

	log.Printf("🚀 Запуск простого тестирования адаптера CoinEx...")

	// Запуск адаптера
	log.Printf("🔗 Подключение к адаптеру...")
	if err := adapter.Start(); err != nil {
		log.Fatalf("❌ Ошибка запуска адаптера: %v", err)
	}

	log.Printf("✅ Адаптер запущен успешно")

	// Даем время на подключение
	time.Sleep(2 * time.Second)

	// Пробуем подписаться на один символ
	log.Printf("📡 Подписка на BTCUSDT...")

	// Проверим статус до подписки
	log.Printf("🔍 Проверка соединения перед подпиской...")

	if err := adapter.SubscribeMarkets([]string{"BTCUSDT"}, "spot", 10); err != nil {
		log.Printf("❌ Ошибка подписки: %v", err)
	} else {
		log.Printf("✅ Подписка отправлена")
	}

	// Ждем некоторое время для получения данных
	log.Printf("⏰ Ожидание данных 10 секунд...")
	time.Sleep(10 * time.Second)

	log.Printf("✅ Простой тест завершен")
}
