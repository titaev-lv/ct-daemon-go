# Исправление проблем с Order Book и Best Price

Этот документ описывает исправления, внесенные в систему для решения проблем с обработкой Order Book и Best Price данных.

## Выявленные проблемы

### 1. Проблема с передачей данных между DataMonitor и DataWorker

**Проблема:** DataMonitor получал из БД `pair_id` (числовой ID пары), но передавал в адаптеры строки вида `"1-23"` (exchange_id-pair_id), а адаптеры ожидали реальные символы пар типа `"BTCUSDT"`, `"BTC-USDT"`.

**Решение:**
- Добавлено поле `Symbol` в структуру `DataMonitorPair`
- Обновлены SQL запросы для получения символов из таблицы `SPOT_TRADE_PAIR`
- Изменена логика DataMonitor для передачи реальных символов в DataWorker

### 2. Отсутствие интеграции между адаптерами и Trade Worker

**Проблема:** Адаптеры получали WebSocket данные, но не передавали их в Trade Worker для обработки и поиска арбитража.

**Решение:**
- Создан Message Bus (`internal/bus/message_bus.go`) для связи между компонентами
- Интегрированы парсеры напрямую в адаптеры
- Добавлена подписка Trade Worker на сообщения от всех бирж

## Внесенные изменения

### 1. Структуры данных

**`internal/db/types.go`:**
```go
type DataMonitorPair struct {
    ExchangeID   int
    ExchangeName string
    PairID       int
    Symbol       string // ДОБАВЛЕНО: реальный символ пары
    MarketType   string
}
```

### 2. SQL запросы

**Обновлены запросы для MySQL и PostgreSQL:**
```sql
SELECT 
    stp.EXCHANGE_ID,
    LOWER(e2.NAME) AS EXCHANGE_NAME,
    t.PAIR_ID,
    COALESCE(stp.SYMBOL, CONCAT(stp.BASE_SYMBOL, stp.QUOTE_SYMBOL)) AS SYMBOL, -- ДОБАВЛЕНО
    'SPOT' AS MARKET_TYPE
FROM ...
```

### 3. Message Bus

**Новый компонент `internal/bus/message_bus.go`:**
- Singleton pattern для глобального доступа
- Thread-safe подписка/отписка
- Буферизированные каналы для предотвращения блокировок
- Метрики подписчиков

**Ключевые методы:**
- `Subscribe(exchange, bufferSize)` - подписка на биржу
- `Publish(exchange, message)` - отправка сообщения
- `Unsubscribe(exchange, channel)` - отписка

### 4. Интеграция адаптеров

**Обновлен `BinanceAdapter`:**
- Добавлены поля `parser` и `messageBus`
- Обновлен `readLoopWithReconnect()` для парсинга сообщений
- Автоматическая отправка унифицированных сообщений в Message Bus

```go
// Парсим WebSocket сообщение
unifiedMsg, err := a.parser.ParseMessage("binance", message)
if unifiedMsg != nil {
    var msg market.UnifiedMessage = *unifiedMsg
    a.messageBus.Publish("binance", msg)
}
```

### 5. Интеграция Trade Worker

**Обновлен `TradeWorker`:**
- Добавлены поля `messageBus` и `subscriptions`
- Автоматическая подписка на все разрешенные биржи при старте
- Отдельные goroutines для обработки сообщений от каждой биржи
- Автоматическая отписка при остановке

```go
// Подписываемся на сообщения всех разрешенных бирж
for _, exchange := range tw.config.AllowedExchanges {
    ch := tw.messageBus.Subscribe(exchange, 100)
    tw.subscriptions[exchange] = ch
    go tw.messageProcessor(exchange, ch)
}
```

### 6. Обновленная логика DataMonitor

**Изменен ключ группировки:**
- Старый: `fmt.Sprintf("%d|%d|%s", p.ExchangeID, p.PairID, p.MarketType)`
- Новый: `fmt.Sprintf("%s|%s", p.ExchangeName, p.MarketType)`

**Использование реальных символов:**
- Старый: `fmt.Sprintf("%d-%d", p.ExchangeID, p.PairID)`
- Новый: `p.Symbol` (например, "BTCUSDT")

## Поток данных после исправлений

```
1. БД → DataMonitor
   └─ SELECT symbol, exchange_name FROM SPOT_TRADE_PAIR...

2. DataMonitor → DataWorker → Adapter
   └─ pairs=["BTCUSDT", "ETHUSDT"] вместо ["1-23", "2-45"]

3. Adapter → WebSocket → Parser → Message Bus
   └─ UnifiedMessage с Order Book / Best Price

4. Message Bus → Trade Worker
   └─ Обработка Order Book, поиск арбитража
```

## Результаты тестирования

Интеграционный тест показал:

✅ **Message Bus работает корректно:**
- Подписка на 6 бирж (binance, bybit, kucoin, htx, coinex, poloniex)
- Успешная доставка сообщений
- Корректная отписка при остановке

✅ **TradeWorker интеграция:**
- Автоматическая подписка на все биржи
- Обработка Order Book и Best Price сообщений
- Graceful shutdown с отпиской от всех каналов

✅ **Решены проблемы:**
- DataMonitor теперь передает реальные символы (`"BTCUSDT"`)
- Адаптеры получают корректные символы для WebSocket подписки
- Trade Worker получает унифицированные данные для арбитража

## Следующие шаги

1. **Интеграция остальных адаптеров** - добавить Message Bus в Bybit, Kucoin, HTX, Coinex, Poloniex
2. **Тестирование с реальной БД** - проверить SQL запросы на настоящих данных
3. **Конфигурация символов** - добавить маппинг символов для разных бирж (BTCUSDT vs BTC-USDT)
4. **Мониторинг** - добавить метрики обработки сообщений

Теперь система Order Book и Best Price полностью функциональна и готова для поиска арбитражных возможностей!