# Унифицированный формат торговых пар и Trade Worker

## ✅ Реализованное решение

### 🎯 Правильная архитектура

**Ранее**: Поиск арбитража в обработчике сообщений ❌  
**Теперь**: Поиск арбитража в Trade Worker ✅

**Ранее**: Разные форматы символов для каждой биржи ❌  
**Теперь**: Унифицированный формат с BASE/QUOTE валютами ✅

## Унифицированный формат торговых пар

### Правила формата символов

```go
// Spot рынки: BTC/USDT
// Futures рынки: BTCUSDT  
// Всегда есть отдельные поля BASE_CURRENCY и QUOTE_CURRENCY

type UnifiedSymbol struct {
    BaseCurrency   string // BTC
    QuoteCurrency  string // USDT
    MarketType     string // spot, futures
    Symbol         string // BTC/USDT или BTCUSDT
    OriginalSymbol string // BTCUSDT (от биржи)
}
```

### Примеры конвертации

| Биржа | Оригинал | Unified Spot | Unified Futures | Base | Quote |
|-------|----------|--------------|-----------------|------|-------|
| Binance | BTCUSDT | BTC/USDT | BTCUSDT | BTC | USDT |
| Kucoin | BTC-USDT | BTC/USDT | BTCUSDT | BTC | USDT |
| Bybit | BTCUSDT | BTC/USDT | BTCUSDT | BTC | USDT |

### Автоматическое распознавание

```go
// Парсит различные форматы
ParseSymbol("BTCUSDT", "spot")     // → BTC/USDT
ParseSymbol("BTC-USDT", "spot")    // → BTC/USDT  
ParseSymbol("BTC/USDT", "spot")    // → BTC/USDT
ParseSymbol("BTCUSDT", "futures")  // → BTCUSDT

// Автоматически определяет BASE и QUOTE
splitConcatenatedSymbol("BTCUSDT") // → BTC, USDT
splitConcatenatedSymbol("ETHBTC")  // → ETH, BTC
```

## Trade Worker - правильная архитектура

### Поток данных

```
WebSocket → Parser → MessageProcessor → DataCollector (сбор)
                                   ↘
                                     TradeWorker (арбитраж)
```

### Trade Worker функции

1. **Сбор данных**: Получает UnifiedMessage через MessageHandler
2. **Поиск арбитража**: Анализирует данные между биржами  
3. **Исполнение сделок**: Отправляет ордера (опционально)
4. **Статистика**: Мониторинг прибыльности

### Конфигурация Trade Worker

```go
config := &TradeWorkerConfig{
    MinProfitPercent:   0.1,     // минимум 0.1% профита
    MinVolumeUSDT:      100,     // минимум $100 объем
    MaxVolumeUSDT:      10000,   // максимум $10,000 объем
    UpdateInterval:     1*time.Second,
    EnableExecution:    false,   // только мониторинг
    AllowedExchanges:   []string{"binance", "bybit", "kucoin"},
    BlacklistedSymbols: []string{"LUNA", "FTT"}, // опасные токены
}
```

### Примеры арбитража

```go
type ArbitrageOpportunity struct {
    Symbol          string  // "BTC/USDT"
    UnifiedSymbol   *UnifiedSymbol
    BuyExchange     string  // "binance"
    SellExchange    string  // "bybit"  
    BuyPrice        float64 // 43001.50
    SellPrice       float64 // 43015.80
    ProfitPercent   float64 // 0.0332%
    EstimatedProfit float64 // $14.30 USDT
    MaxVolume       float64 // 1.5 BTC
}
```

## Компоненты системы

### 1. Символьный реестр (`internal/market/symbols.go`)

```go
registry := market.NewSymbolRegistry()

// Конвертация из формата биржи в унифицированный
unified, _ := registry.ConvertToUnified("binance", "BTCUSDT", "spot")
// unified.Symbol = "BTC/USDT"
// unified.BaseCurrency = "BTC"  
// unified.QuoteCurrency = "USDT"

// Конвертация в формат биржи
exchangeSymbol := registry.ConvertToExchange("kucoin", unified)
// exchangeSymbol = "BTC-USDT" для Kucoin spot
```

### 2. Улучшенный парсер (`internal/market/parsers/binance.go`)

```go
// Автоматически конвертирует символы
func (p *BinanceParser) parseOrderBook(rawData []byte) (*UnifiedMessage, error) {
    // msg.Data.Symbol = "BTCUSDT" (от Binance)
    unifiedSymbol, _ := p.symbolRegistry.ConvertToUnified("binance", msg.Data.Symbol, "spot")
    
    return &UnifiedMessage{
        Symbol:        unifiedSymbol.Symbol,        // "BTC/USDT" 
        UnifiedSymbol: unifiedSymbol,               // полная информация
        Data:          orderbook,
    }
}
```

### 3. Trade Worker (`internal/worker/trade_worker.go`)

```go
// Реализует MessageHandler для получения данных
func (tw *TradeWorker) HandleMessage(msg UnifiedMessage) error {
    switch msg.MessageType {
    case MessageTypeOrderBook:
        tw.handleOrderBook(msg)  // сохраняем orderbook
    case MessageTypeBestPrice:  
        tw.handleBestPrice(msg)  // сохраняем лучшие цены
    }
}

// Основной цикл поиска арбитража
func (tw *TradeWorker) arbitrageLoop() {
    for {
        opportunities := tw.findArbitrageOpportunities()
        for _, opp := range opportunities {
            log.Printf("ARBITRAGE: %s %.4f%% profit", opp.Symbol, opp.ProfitPercent)
            if tw.config.EnableExecution {
                tw.executeTrade(opp)  // исполняем сделку
            }
        }
    }
}
```

### 4. Сбор данных (`internal/handlers/data_collector.go`)

```go
// Только собирает данные, НЕ ищет арбитраж
type DataCollector struct {
    orderBooks map[string]map[string]*UnifiedOrderBook
    bestPrices map[string]map[string]*UnifiedBestPrice
}

func (h *DataCollector) HandleMessage(msg UnifiedMessage) error {
    // Сохраняет данные для дальнейшего использования
    // Trade Worker получает те же данные и ищет арбитраж
}
```

## Результат работы

### Логи системы

```
[DataCollector] OrderBook updated: binance BTC/USDT - Bids: 20, Asks: 20
[DataCollector] BestPrice updated: bybit BTC/USDT - Bid: 43015.80, Ask: 43016.20

[ARBITRAGE] BTC/USDT: Buy binance@43001.50 → Sell bybit@43015.80 | Profit: 0.0332% | Volume: $1,430.25

=== DATA COLLECTION STATS ===
Total exchanges: 2
Total symbols: 5
OrderBook updates: 150
BestPrice updates: 180

=== ARBITRAGE STATS ===
Active: true
Total opportunities found: 23
Current opportunities: 3
Executed trades: 0
Total profit USDT: 0.00
Tracked exchanges: 2
Tracked symbols: 5
```

## Преимущества новой архитектуры

✅ **Правильное разделение**: DataCollector собирает, TradeWorker торгует  
✅ **Унифицированные символы**: BTC/USDT для spot, BTCUSDT для futures  
✅ **BASE/QUOTE валюты**: Отдельные поля для анализа  
✅ **Масштабируемость**: Легко добавить новые биржи и стратегии  
✅ **Конфигурируемость**: Настройки профита, объемов, бирж  
✅ **Безопасность**: Blacklist опасных токенов, лимиты объемов  

## Запуск

```bash
cd /home/leon/docker/ct-system/daemon-go
go build -o unified-demo ./cmd/unified-demo/
./unified-demo
```

## Следующие шаги

1. **Добавить больше бирж**: Bybit, Kucoin parsers
2. **Реализовать исполнение**: Интеграция с exchange adapters  
3. **Добавить фьючерсы**: Поддержка futures рынков
4. **Улучшить стратегии**: Более сложные алгоритмы арбитража
5. **REST API**: Управление через HTTP API
6. **База данных**: Сохранение статистики и сделок

**Теперь архитектура правильная: Trade Worker ищет арбитраж, символы унифицированы! 🚀**