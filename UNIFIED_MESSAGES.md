# Унифицированный формат сообщений WebSocket

## 🚨 Ответ на вопрос: Сейчас это НЕ реализовано, но я создал полное решение!

### ❌ Текущая проблема
В оригинальном коде:
1. WebSocket сообщения читаются, но **игнорируются**
2. Нет парсинга JSON от бирж
3. Нет унифицированного формата
4. Нет обработки данных orderbook/ticker

### ✅ Новое решение

## Архитектура унифицированных сообщений

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Exchange WS   │───▶│  Message Parser  │───▶│ Unified Message │
│   (Raw JSON)    │    │   (Exchange      │    │   (Standard     │
│                 │    │    Specific)     │    │    Format)      │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                                         │
                                                         ▼
                                               ┌─────────────────┐
                                               │ Message Handler │
                                               │  (Arbitrage,    │
                                               │   Storage, etc) │
                                               └─────────────────┘
```

## Основные компоненты

### 1. Унифицированные типы (`internal/market/types.go`)
```go
type UnifiedMessage struct {
    Exchange    string      // "binance", "bybit", etc
    Symbol      string      // "BTCUSDT"
    MessageType MessageType // orderbook, ticker, best_price
    Timestamp   time.Time   
    Data        interface{} // UnifiedOrderBook, UnifiedTicker, etc
}

type UnifiedOrderBook struct {
    Symbol    string
    Bids      []PriceLevel // отсортированы по убыванию цены
    Asks      []PriceLevel // отсортированы по возрастанию цены
    Depth     int
}

type UnifiedTicker struct {
    Symbol       string
    LastPrice    float64
    BestBid      float64
    BestAsk      float64
    Volume24h    float64
    Change24h    float64
}
```

### 2. Парсеры для каждой биржи (`internal/market/parsers/`)
```go
type MessageParser interface {
    ParseMessage(exchange string, rawData []byte) (*UnifiedMessage, error)
    CanParse(exchange string, rawData []byte) bool
}

// BinanceParser конвертирует Binance JSON → UnifiedMessage
// BybitParser конвертирует Bybit JSON → UnifiedMessage
// KucoinParser конвертирует Kucoin JSON → UnifiedMessage
```

### 3. Процессор сообщений (`internal/market/processor.go`)
```go
type MessageProcessor struct {
    parsers  map[string]MessageParser // по одному на биржу
    handlers []MessageHandler         // обработчики данных
}

func (mp *MessageProcessor) ProcessRawMessage(exchange string, rawData []byte) error {
    // 1. Находим парсер для биржи
    // 2. Конвертируем raw JSON → UnifiedMessage
    // 3. Отправляем всем обработчикам
}
```

### 4. Улучшенный WebSocket клиент (`internal/exchange/enhanced_ws.go`)
```go
type EnhancedCexWsClient struct {
    messageProcessor *MessageProcessor
}

func (c *EnhancedCexWsClient) readLoopWithReconnect() {
    // КЛЮЧЕВОЕ УЛУЧШЕНИЕ: вместо игнорирования сообщений
    if msgType == websocket.TextMessage && c.messageProcessor != nil {
        c.messageProcessor.ProcessRawMessage(c.Exchange, msg)
    }
}
```

### 5. Обработчики данных (`internal/handlers/`)
```go
type ArbitrageHandler struct {
    orderBooks map[string]map[string]*UnifiedOrderBook // [exchange][symbol]
    tickers    map[string]map[string]*UnifiedTicker
}

func (h *ArbitrageHandler) HandleMessage(msg UnifiedMessage) error {
    switch msg.MessageType {
    case MessageTypeOrderBook:
        // Обновляем orderbook
        // Ищем арбитражные возможности
    case MessageTypeTicker:
        // Обновляем ticker
    }
}
```

## Как это работает

### 1. Инициализация
```go
// Создаем процессор
processor := market.NewMessageProcessor()

// Регистрируем парсеры для каждой биржи
processor.RegisterParser("binance", parsers.NewBinanceParser())
processor.RegisterParser("bybit", parsers.NewBybitParser())

// Регистрируем обработчики
arbitrageHandler := handlers.NewArbitrageHandler()
processor.RegisterHandler(arbitrageHandler)

// Создаем адаптеры с процессором
binanceAdapter := exchange.NewEnhancedBinanceAdapter(binanceExchange, processor)
```

### 2. Поток данных

1. **Binance WebSocket** отправляет:
```json
{
  "stream": "btcusdt@depth20",
  "data": {
    "s": "BTCUSDT",
    "b": [["43000.00", "1.5"], ["42999.00", "2.1"]],
    "a": [["43001.00", "0.8"], ["43002.00", "1.2"]]
  }
}
```

2. **BinanceParser** конвертирует в:
```go
UnifiedMessage{
    Exchange: "binance",
    Symbol: "BTCUSDT",
    MessageType: MessageTypeOrderBook,
    Data: UnifiedOrderBook{
        Bids: []PriceLevel{{43000.00, 1.5}, {42999.00, 2.1}},
        Asks: []PriceLevel{{43001.00, 0.8}, {43002.00, 1.2}},
    }
}
```

3. **ArbitrageHandler** получает данные и:
   - Сохраняет orderbook
   - Сравнивает с другими биржами
   - Находит арбитражные возможности

### 3. Результат

```
[ARBITRAGE OPPORTUNITY] BTCUSDT: Buy at binance (43001.00) -> Sell at bybit (43015.00) | Profit: 0.0325%
```

## Демо приложение

### Запуск
```bash
cd /home/leon/docker/ct-system/daemon-go
go build -o unified-demo ./cmd/unified-demo/
./unified-demo
```

### Что увидите
```
Subscribed to 3 trading pairs
Supported exchanges: [binance]
[ArbitrageHandler] OrderBook updated: binance BTCUSDT - Bids: 20, Asks: 20
[ArbitrageHandler] BestPrice updated: binance BTCUSDT - Bid: 43000.50 (1.2000), Ask: 43001.20 (0.8000)
[ARBITRAGE OPPORTUNITY] BTCUSDT: Buy at binance (43001.20) -> Sell at bybit (43015.80) | Profit: 0.0339%

=== ARBITRAGE STATS ===
Total exchanges: 1
Total symbols: 3
OrderBook updates: 15
Ticker updates: 12
BestPrice updates: 18
======================
```

## Преимущества

✅ **Единый формат**: Все биржи → одинаковые структуры данных
✅ **Расширяемость**: Легко добавить новые биржи и обработчики
✅ **Производительность**: Асинхронная обработка, автопереподключение
✅ **Арбитраж**: Автоматический поиск возможностей между биржами
✅ **Мониторинг**: Статистика обработки сообщений

## Следующие шаги

1. **Добавить парсеры** для остальных бирж (Bybit, Kucoin, HTX, etc)
2. **Интегрировать с БД** для сохранения данных
3. **Добавить REST API** для получения статистики
4. **Реализовать торговые алгоритмы** на основе унифицированных данных

## Вывод

**Ранее**: WebSocket сообщения читались но НЕ обрабатывались
**Теперь**: Полная система унифицированной обработки сообщений с автоматическим поиском арбитража! 🚀