package main

import (
	"daemon-go/internal/config"
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/internal/worker/dataworker"
	"daemon-go/pkg/log"
	"fmt"
	"time"
)

func main() {
	// Настройка логирования на DEBUG
	log.SetGlobalLevel(log.DebugLevel)

	// Загружаем конфигурацию для тестирования OrderBook логирования
	cfg, err := config.LoadConfig("config/config.conf")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// Устанавливаем конфигурацию для OrderBook
	exchange.SetOrderBookConfig(cfg)
	fmt.Printf("OrderBook config set: DebugLogRaw=%t, DebugLogMsg=%t\n",
		cfg.OrderBook.DebugLogRaw, cfg.OrderBook.DebugLogMsg)

	logger := log.New("test")
	logger.Info("Starting KuCoin adapter test")

	// Создаем фейковый exchange для KuCoin
	fakeExchange := db.Exchange{
		ID:      1,
		Name:    "kucoin",
		BaseUrl: "https://api.kucoin.com",
		Active:  true,
	}

	// Создаем DataWorker
	worker := dataworker.NewDataWorker(fakeExchange)

	// Создаем MarketPair для тестирования
	marketPairs := []exchange.MarketPair{
		{Symbol: "BTC-USDT", PairID: 1},
		{Symbol: "ETH-USDT", PairID: 2},
	}

	logger.Info("Setting subscription with PairID")
	worker.SetSubscriptionWithPairID(marketPairs, "spot", 5)

	logger.Info("Starting DataWorker")
	if err := worker.Start(); err != nil {
		logger.Error("Failed to start worker: %v", err)
		return
	}

	logger.Info("Worker started, waiting for messages...")

	// Ждем 30 секунд для демонстрации логирования
	time.Sleep(30 * time.Second)

	logger.Info("Stopping worker")
	worker.Stop()

	fmt.Println("Test completed")
}
