package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"daemon-go/internal/bus"
	"daemon-go/internal/config"
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/internal/market"
)

func main() {
	// Настройка логирования
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Загрузка конфигурации
	cfg, err := config.LoadConfig("config/config.conf")
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Инициализация сообщений
	messageBus := bus.GetInstance()

	// Подписка на сообщения для тестирования
	messageChannel := messageBus.Subscribe("coinex", 100)

	// Горутина для обработки сообщений
	go func() {
		for msg := range messageChannel {
			switch msg.MessageType {
			case market.MessageTypeOrderBook:
				if orderBook, ok := msg.Data.(*market.UnifiedOrderBook); ok {
					log.Printf("📊 ORDER BOOK [%s] %s - %s: bids=%d, asks=%d",
						msg.Exchange, msg.Symbol, orderBook.UpdateType,
						len(orderBook.Bids), len(orderBook.Asks))
				}

			case market.MessageTypeTicker:
				if ticker, ok := msg.Data.(*market.UnifiedTicker); ok {
					log.Printf("💰 TICKER [%s] %s: price=%.8f, volume=%.2f",
						msg.Exchange, msg.Symbol, ticker.LastPrice, ticker.Volume24h)
				}

			default:
				log.Printf("📢 Получено сообщение: %s %s %s", msg.Exchange, msg.Symbol, msg.MessageType)
			}
		}
	}()

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

	// Добавляем символы для подписки
	testSymbols := []string{
		"BTCUSDT", // Bitcoin
		"ETHUSDT", // Ethereum
		"XRPUSDT", // Ripple
	}

	log.Printf("🚀 Запуск тестирования адаптера CoinEx...")
	log.Printf("📈 Символы для тестирования: %s", strings.Join(testSymbols, ", "))
	log.Printf("⚙️  Настройки логирования: debug_log_raw=%v, debug_log_msg=%v",
		cfg.OrderBook.DebugLogRaw, cfg.OrderBook.DebugLogMsg)

	// Запуск адаптера
	go func() {
		if err := adapter.Start(); err != nil {
			log.Printf("❌ Ошибка запуска адаптера: %v", err)
		}
	}()

	// Даем время на подключение
	time.Sleep(3 * time.Second)

	// Подписка на символы
	log.Printf("📡 Подписка на символы...")
	for _, symbol := range testSymbols {
		if err := adapter.SubscribeMarkets([]string{symbol}, "spot", 10); err != nil {
			log.Printf("❌ Ошибка подписки на %s: %v", symbol, err)
		} else {
			log.Printf("✅ Подписка на %s выполнена", symbol)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Тестирование отписки через 30 секунд
	go func() {
		time.Sleep(30 * time.Second)
		log.Printf("📡 Тестирование отписки от символов...")

		for _, symbol := range testSymbols[:1] { // Отписываемся только от первого символа
			if err := adapter.UnsubscribeMarkets([]string{symbol}, "spot", 10); err != nil {
				log.Printf("❌ Ошибка отписки от %s: %v", symbol, err)
			} else {
				log.Printf("🔇 Отписка от %s выполнена", symbol)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("🏃 Адаптер CoinEx запущен. Нажмите Ctrl+C для остановки...")
	log.Printf("📊 Ожидаем данные order book и ticker...")

	// Ожидание сигнала завершения
	<-sigChan

	log.Printf("🛑 Получен сигнал завершения. Остановка адаптера...")

	// Даем время на graceful shutdown
	time.Sleep(2 * time.Second)
	log.Printf("✅ Тестирование завершено")
}
