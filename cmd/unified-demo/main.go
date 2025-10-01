package main

import (
	"context"
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/internal/handlers"
	"daemon-go/internal/market"
	"daemon-go/internal/market/parsers"
	"daemon-go/internal/worker"
	"daemon-go/pkg/log"
	"database/sql"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Демонстрация объединенного формата сообщений для всех бирж с debug логированием
func main() {
	logger := log.New("unified-demo")

	logger.Info("=== UNIFIED MESSAGE FORMAT DEMO ===")
	logger.Info("Демонстрация объединенного формата сообщений для всех бирж")
	logger.Info("Включен debug режим для всех WebSocket соединений")
	logger.Info("")

	// 1. Создаем процессор сообщений
	messageProcessor := market.NewMessageProcessor()

	// 2. Регистрируем парсеры для ВСЕХ бирж
	exchangeParsers := map[string]market.MessageParser{
		"binance":  parsers.NewBinanceParser(),
		"bybit":    parsers.NewBybitParser(),
		"kucoin":   parsers.NewKucoinParser(),
		"htx":      parsers.NewHTXParser(),
		"coinex":   parsers.NewCoinexParser(),
		"poloniex": parsers.NewPoloniexParser(),
	}

	// Регистрируем все парсеры
	for exchange, parser := range exchangeParsers {
		messageProcessor.RegisterParser(exchange, parser)
		logger.Info("✓ Зарегистрирован парсер для биржи: %s", exchange)
	}

	logger.Info("")

	// 3. Создаем обработчик для сбора данных
	dataCollector := handlers.NewDataCollector()
	messageProcessor.RegisterHandler(dataCollector)

	// 4. Создаем и запускаем Trade Worker для поиска арбитража
	tradeWorkerConfig := worker.DefaultTradeWorkerConfig()
	tradeWorkerConfig.MinProfitPercent = 0.05 // 0.05% минимальный профит
	tradeWorkerConfig.EnableExecution = false // только мониторинг, не торговля

	tradeWorker := worker.NewTradeWorker(tradeWorkerConfig)
	messageProcessor.RegisterHandler(tradeWorker) // TradeWorker тоже MessageHandler

	if err := tradeWorker.Start(); err != nil {
		logger.Fatal("Failed to start trade worker: %v", err)
	}

	// 5. Создаем демо данные для всех бирж
	exchanges := map[string]db.Exchange{
		"binance": {
			ID:      1,
			Name:    "Binance",
			BaseUrl: "https://api.binance.com",
			WsUrl:   sql.NullString{String: "wss://stream.binance.com:9443/ws/", Valid: true},
			Active:  true,
		},
		"bybit": {
			ID:      2,
			Name:    "Bybit",
			BaseUrl: "https://api.bybit.com",
			WsUrl:   sql.NullString{String: "wss://stream-testnet.bybit.com/v5/public/linear", Valid: true},
			Active:  true,
		},
	}

	// 6. Создаем адаптер только для Binance (реальные данные)
	binanceAdapter := exchange.NewEnhancedBinanceAdapter(exchanges["binance"], messageProcessor)

	// 7. Запускаем Binance адаптер
	if err := binanceAdapter.Start(); err != nil {
		logger.Fatal("Failed to start Binance adapter: %v", err)
	}

	// 8. Подписываемся на тестовые пары
	testPairs := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}
	if err := binanceAdapter.SubscribeMarkets(testPairs, "spot", 20); err != nil {
		logger.Fatal("Failed to subscribe to markets: %v", err)
	}

	logger.Info("✓ Подписка на %d торговых пар Binance", len(testPairs))
	logger.Info("✓ Поддерживаемые биржи: %v", messageProcessor.GetSupportedExchanges())

	// 9. Демонстрация парсеров других бирж с mock данными
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Mock данные для других бирж каждые 10 секунд
	wg.Add(1)
	go func() {
		defer wg.Done()
		demonstrateExchangeParsers(ctx, messageProcessor, logger)
	}()

	// 7. Запускаем мониторинг статистики
	wg.Add(1)
	go func() {
		defer wg.Done()
		statsLogger := log.New("stats")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Статистика по сбору данных
				dataStats := dataCollector.GetStats()
				statsLogger.Info("=== DATA COLLECTION STATS ===")
				statsLogger.Info("Total exchanges: %v", dataStats["total_exchanges"])
				statsLogger.Info("Total symbols: %v", dataStats["total_symbols"])
				statsLogger.Info("OrderBook updates: %v", dataStats["orderbook_updates"])
				statsLogger.Info("Ticker updates: %v", dataStats["ticker_updates"])
				statsLogger.Info("BestPrice updates: %v", dataStats["best_price_updates"])

				// Статистика по арбитражу
				tradeStats := tradeWorker.GetStats()
				statsLogger.Info("=== ARBITRAGE STATS ===")
				statsLogger.Info("Active: %v", tradeStats["active"])
				statsLogger.Info("Total opportunities found: %v", tradeStats["total_opportunities"])
				statsLogger.Info("Current opportunities: %v", tradeStats["current_opportunities"])
				statsLogger.Info("Executed trades: %v", tradeStats["executed_trades"])
				statsLogger.Info("Total profit USDT: %.2f", tradeStats["total_profit_usdt"])
				statsLogger.Info("Tracked exchanges: %v", tradeStats["tracked_exchanges"])
				statsLogger.Info("Tracked symbols: %v", tradeStats["tracked_symbols"])
				statsLogger.Info("======================")
			}
		}
	}()

	// 8. Ожидаем сигнал завершения
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	logger.Info("=== Unified message processing with ALL exchange parsers started ===")
	logger.Info("Получение реальных данных с Binance + демонстрация парсеров всех бирж")
	logger.Info("Нажмите Ctrl+C для остановки...")
	<-c

	// 9. Graceful shutdown
	logger.Info("Останавливаем...")
	cancel() // Останавливаем демонстрацию
	if err := tradeWorker.Stop(); err != nil {
		logger.Error("Error stopping trade worker: %v", err)
	}
	if err := binanceAdapter.Stop(); err != nil {
		logger.Error("Error stopping Binance adapter: %v", err)
	}

	wg.Wait() // Ждем завершения всех горутин
	logger.Info("Демонстрация завершена")
}

// demonstrateExchangeParsers - демонстрирует работу парсеров всех бирж с mock данными
func demonstrateExchangeParsers(ctx context.Context, processor *market.MessageProcessor, logger *log.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Mock данные для разных бирж
	mockData := map[string][]byte{
		"bybit": []byte(`{
			"topic": "orderbook.1.BTCUSDT",
			"type": "snapshot",
			"ts": 1672916400000,
			"data": {
				"s": "BTCUSDT",
				"b": [["43000.1", "1.234"], ["43000.0", "2.456"]],
				"a": [["43000.2", "1.567"], ["43000.3", "2.789"]],
				"u": 123456,
				"seq": 789012
			}
		}`),

		"kucoin": []byte(`{
			"type": "message",
			"topic": "/market/level2:BTC-USDT",
			"subject": "trade.l2update",
			"data": {
				"sequenceStart": 1672916400,
				"sequenceEnd": 1672916401,
				"symbol": "BTC-USDT",
				"changes": {
					"asks": [["43000.5", "1.2", "1672916400"]],
					"bids": [["43000.1", "2.5", "1672916401"]]
				}
			}
		}`),

		"htx": []byte(`{
			"ch": "market.btcusdt.depth.step0",
			"ts": 1672916400000,
			"tick": {
				"bids": [[43000.1, 1.234], [43000.0, 2.456]],
				"asks": [[43000.2, 1.567], [43000.3, 2.789]],
				"ts": 1672916400000,
				"version": 123456
			}
		}`),

		"coinex": []byte(`{
			"method": "depth.update",
			"params": [true, {"bids": [["43000.1", "1.234"]], "asks": [["43000.2", "1.567"]]}, "BTCUSDT"],
			"id": null
		}`),

		"poloniex": []byte(`{
			"channel": "book_lv2",
			"data": {
				"symbol": "BTC_USDT",
				"createTime": 1672916400000,
				"asks": [["43000.2", "1.567"]],
				"bids": [["43000.1", "1.234"]],
				"lastUpdateId": 123456
			}
		}`),
	}

	logger.Info("🧪 Начинаем демонстрацию парсеров всех бирж с mock данными...")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Отправляем mock данные для каждой биржи
			for exchange, data := range mockData {
				err := processor.ProcessRawMessage(exchange, data)
				if err != nil {
					logger.Error("[%s] ❌ Mock ошибка обработки: %v", exchange, err)
					continue
				}

				logger.Info("[%s] ✅ Mock сообщение успешно обработано", exchange)
			}
		}
	}
}
