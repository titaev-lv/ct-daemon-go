# Order Book Update Types

Эта документация описывает, как каждая биржа обрабатывает обновления order book и как наши парсеры определяют тип обновления.

## Типы обновлений

### OrderBookUpdateType

```go
type OrderBookUpdateType string

const (
    OrderBookUpdateTypeSnapshot    OrderBookUpdateType = "snapshot"    // Полная замена order book
    OrderBookUpdateTypeIncremental OrderBookUpdateType = "incremental" // Инкрементальное обновление
)
```

## Парсеры по биржам

### Binance
- **Тип**: `snapshot` (всегда)
- **Описание**: Binance depth stream отправляет полные снимки order book
- **Парсер**: `binance.go`
- **Поле**: `UpdateType = OrderBookUpdateTypeSnapshot`

### Bybit
- **Тип**: Определяется по полю `type` в сообщении
- **Описание**: Bybit явно указывает тип обновления в поле `type`
- **Парсер**: `bybit.go`
- **Логика**: 
  ```go
  updateType := OrderBookUpdateTypeIncremental
  if wsMsg.Type == "snapshot" {
      updateType = OrderBookUpdateTypeSnapshot
  }
  ```

### Kucoin
- **Тип**: `incremental` (всегда)
- **Описание**: Kucoin level2 отправляет только инкрементальные обновления
- **Парсер**: `kucoin.go`
- **Поле**: `UpdateType = OrderBookUpdateTypeIncremental`

### HTX (Huobi)
- **Тип**: `snapshot` (всегда)
- **Описание**: HTX depth отправляет полные снимки order book
- **Парсер**: `htx.go`
- **Поле**: `UpdateType = OrderBookUpdateTypeSnapshot`

### Coinex
- **Тип**: Определяется по флагу `is_full` в сообщении
- **Описание**: Coinex использует флаг `is_full` для указания типа обновления
- **Парсер**: `coinex.go`
- **Логика**:
  ```go
  updateType := OrderBookUpdateTypeIncremental
  if isFull {
      updateType = OrderBookUpdateTypeSnapshot
  }
  ```
- **Формат сообщения**: `[is_full, depth_data, symbol]`

### Poloniex
- **Тип**: `incremental` (всегда)
- **Описание**: Poloniex book отправляет только инкрементальные обновления
- **Парсер**: `poloniex.go`
- **Поле**: `UpdateType = OrderBookUpdateTypeIncremental`

## Важность правильного определения типа

### Snapshot (Полный снимок)
- **Действие**: Полная замена текущего order book
- **Использование**: Инициализация или периодическое обновление всего состояния
- **Обработка**: Заменить весь order book новыми данными

### Incremental (Инкрементальное обновление)
- **Действие**: Обновление отдельных уровней в order book
- **Использование**: Добавление, изменение или удаление конкретных price levels
- **Обработка**: Слияние с существующим order book
  - Если volume > 0: добавить или обновить уровень
  - Если volume = 0: удалить уровень

## Примеры использования

### Trade Worker
```go
func (tw *TradeWorker) processOrderBook(msg *market.UnifiedMessage) {
    orderbook := msg.Data.(market.UnifiedOrderBook)
    
    switch orderbook.UpdateType {
    case market.OrderBookUpdateTypeSnapshot:
        // Полная замена order book
        tw.orderbooks[msg.Symbol] = orderbook
        
    case market.OrderBookUpdateTypeIncremental:
        // Инкрементальное обновление
        tw.mergeOrderBookUpdate(msg.Symbol, orderbook)
    }
}
```

### Arbitrage Engine
```go
func (ae *ArbitrageEngine) handleOrderBookUpdate(update market.UnifiedOrderBook) {
    if update.UpdateType == market.OrderBookUpdateTypeSnapshot {
        // Сброс всех pending арбитражных позиций для этого символа
        ae.resetPendingPositions(update.Symbol)
    }
    
    // Обновление order book и поиск арбитражных возможностей
    ae.updateOrderBook(update)
    ae.findArbitrageOpportunities(update.Symbol)
}
```

## Тестирование

Все парсеры протестированы на корректное определение типов обновлений:

- ✅ Binance: snapshot
- ✅ Bybit: определяется по полю 'type'
- ✅ Kucoin: incremental
- ✅ HTX: snapshot
- ✅ Coinex: определяется по флагу 'is_full'
- ✅ Poloniex: incremental

Корректная обработка типов обновлений критически важна для поддержания целостности данных order book и стабильной работы торговых алгоритмов.