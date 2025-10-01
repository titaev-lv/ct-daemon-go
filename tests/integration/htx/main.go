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

	// Настройка конфигурации для отладки
	debugConfig := &config.Config{
		OrderBook: struct {
			DebugLogRaw bool
			DebugLogMsg bool
		}{
			DebugLogRaw: true,
			DebugLogMsg: true,
		},
	}
	exchange.SetOrderBookConfig(debugConfig)

	// Инициализация message bus
	messageBus := bus.GetInstance()

	// Подписка на сообщения для тестирования
	messageChannel := messageBus.Subscribe("htx", 100)

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

	// Создание объекта Exchange для адаптера HTX
	exchangeConfig := db.Exchange{
		ID:           1,
		Name:         "HTX",
		Active:       true,
		Url:          "https://api.huobi.pro",
		BaseUrl:      "https://api.huobi.pro",
		WebsocketUrl: sql.NullString{String: "wss://api.huobi.pro/ws", Valid: true},
		WsUrl:        sql.NullString{String: "wss://api.huobi.pro/ws", Valid: true},
	}

	// Создание адаптера HTX
	adapter := exchange.NewHtxAdapter(exchangeConfig)

	// Добавляем символы для тестирования
	testSymbols := []string{
		"BTCUSDT", // Bitcoin
		"ETHUSDT", // Ethereum
		"XRPUSDT", // Ripple
	}

	log.Printf("🚀 Запуск тестирования адаптера HTX...")
	log.Printf("📈 Символы для тестирования: %s", strings.Join(testSymbols, ", "))
	log.Printf("⚙️  Настройки логирования: debug_log_raw=%v, debug_log_msg=%v",
		debugConfig.OrderBook.DebugLogRaw, debugConfig.OrderBook.DebugLogMsg)
	log.Printf("🔔 ВАЖНО: HTX сама шлет ping'и, WriteLoop не запускается!")

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
		if err := adapter.SubscribeMarkets([]string{symbol}, "spot", 0); err != nil {
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
			if err := adapter.UnsubscribeMarkets([]string{symbol}, "spot", 0); err != nil {
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

	log.Printf("🏃 Адаптер HTX запущен. Нажмите Ctrl+C для остановки...")
	log.Printf("📊 Ожидаем данные order book и ticker...")

	// Ожидание сигнала завершения
	<-sigChan

	log.Printf("🛑 Получен сигнал завершения. Остановка адаптера...")

	// Даем время на graceful shutdown
	time.Sleep(2 * time.Second)
	log.Printf("✅ Тестирование завершено")
}