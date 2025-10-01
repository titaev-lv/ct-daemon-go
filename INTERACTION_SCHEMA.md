# Детальная схема взаимодействия модулей торгового робота

## Общая архитектура системы

```
                    ┌─────────────────┐
                    │   HTTP API      │ :8080
                    │   (управление)  │
                    └─────────────────┘
                             │
                    ┌─────────────────┐
                    │     MANAGER     │ (главный менеджер)
                    │   (app/manager) │
                    └─────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
   ┌─────────┐     ┌─────────────────┐     ┌─────────┐
   │ SERVICE │     │   CORE MODULES  │     │   DB    │
   │ DAEMON  │     │                 │     │ LAYER   │
   └─────────┘     └─────────────────┘     └─────────┘
                            │
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
  ┌──────────┐    ┌──────────────┐    ┌──────────────┐
  │   TRADE  │    │  TRADE DATA  │    │  EXCHANGE    │
  │  MODULE  │    │    MODULE    │    │  EXECUTOR    │
  └──────────┘    └──────────────┘    └──────────────┘
        │                   │                   │
  ┌──────────┐    ┌──────────────┐    ┌──────────────┐
  │ COLLECTOR│    │    CACHE     │    │   MESSAGE    │
  │  EVENTS  │    │    LAYER     │    │    QUEUE     │
  └──────────┘    └──────────────┘    └──────────────┘
```

## Детальное взаимодействие модулей

### 1. TRADE MODULE (Стратегии арбитража)

```
TradeMonitor (мониторит таблицу TRADE каждые 5-10 сек)
    │
    ├── Создает TradeWorker для каждого активного TRADE
    │
TradeWorker (исполняет стратегию арбитража)
    │
    ├── Читает данные из DataCache (orderbook/prices)
    ├── Анализирует арбитражные возможности  
    ├── Принимает решение о сделке
    ├── Отправляет задачи в ExecutorRouter
    ├── Мониторит результаты через EventCache
    └── Записывает статистику в БД

Поток данных:
TradeWorker → DataCache (read orderbook)
TradeWorker → ExecutorRouter (send order task)
TradeWorker ← EventCache (read order results)
TradeWorker → Database (write trade statistics)
```

### 2. TRADE DATA MODULE (Сбор рыночных данных)

```
DataMonitor (мониторит активные пары из БД каждые 5-10 сек)
    │
    ├── Создает DataWorker для каждой (биржа, рынок)
    │
DataWorker (собирает orderbook/best prices)
    │
    ├── Подключается к бирже через ExchangeAdapter
    ├── Подписывается на WebSocket потоки
    ├── Получает orderbook/ticker данные
    ├── Обновляет DataCache
    └── Логирует статистику

Поток данных:
DataWorker ← ExchangeAdapter (WebSocket data)
DataWorker → DataCache (write market data)
DataMonitor ← Database (read active pairs)
```

### 3. EXCHANGE EXECUTOR MODULE (Исполнение ордеров)

```
ExecutorMonitor (мониторит активных пользователей/биржи)
    │
    ├── Создает ExecutorWorker для каждой (биржа, пользователь)
    │
ExecutorWorker (исполняет ордера)
    │
    ├── Читает задачи из OrderQueue (FIFO)
    ├── Исполняет ордера через ExchangeAdapter
    ├── Отправляет результаты в EventStorage
    └── Обновляет статус в БД

OrderQueue (FIFO очереди по биржам/пользователям)
    │
    ├── Thread-safe операции
    ├── Персистентность для recovery
    └── Маршрутизация по ключам

Поток данных:
TradeWorker → ExecutorRouter → OrderQueue
ExecutorWorker ← OrderQueue (read tasks)
ExecutorWorker → ExchangeAdapter (execute orders)
ExecutorWorker → EventStorage (write results)
ExecutorWorker → Database (update order status)
```

### 4. COLLECTOR EVENTS MODULE (Сбор событий ордеров)

```
EventMonitor (мониторит активных пользователей/биржи)
    │
    ├── Создает EventWorker для каждой (биржа, пользователь)
    │
EventWorker (собирает события ордеров)
    │
    ├── Подключается к WebSocket событий биржи
    ├── Получает события исполнения ордеров
    ├── Парсит и нормализует данные
    ├── Записывает в БД + EventCache
    └── Уведомляет TradeWorkers

EventCache (быстрый доступ к событиям)
    │
    ├── In-memory кеш событий
    ├── Thread-safe операции
    └── TTL для управления памятью

Поток данных:
EventWorker ← ExchangeAdapter (WebSocket events)
EventWorker → Database (write events)
EventWorker → EventCache (cache events)
TradeWorker ← EventCache (read events)
```

### 5. CACHE LAYER (Быстрый доступ к данным)

```
DataCache (orderbook/best prices)
    ├── Thread-safe операции
    ├── TTL для устаревших данных
    └── LRU eviction policy

EventCache (события ордеров)
    ├── Быстрый поиск по order_id
    ├── История событий
    └── Уведомления о новых событиях

ConfigCache (настройки/комиссии)
    ├── Кеш настроек торгов
    ├── Комиссии по парам/аккаунтам
    └── Периодическое обновление
```

## Схема потоков данных

### Поток 1: Сбор рыночных данных
```
Exchange WebSocket → ExchangeAdapter → DataWorker → DataCache
                                                      ↓
                                               TradeWorker (читает)
```

### Поток 2: Принятие торгового решения  
```
TradeWorker читает DataCache → Анализ арбитража → Решение о торговле
     ↓
ExecutorRouter → OrderQueue → ExecutorWorker → ExchangeAdapter → Exchange
```

### Поток 3: Обратная связь
```
Exchange Events → ExchangeAdapter → EventWorker → EventCache
                                                     ↓
                                              TradeWorker (читает)
```

### Поток 4: Персистентность
```
Все Workers → Database (MySQL/PostgreSQL)
              ↓
         Recovery System (при restart)
```

## Конкретные стратегии арбитража

### 1. Межбиржевой рыночный арбитраж

```go
func (s *InterExchangeStrategy) Execute() error {
    // 1. Получить orderbook с двух бирж
    bookA := s.dataCache.GetOrderBook("binance", "BTC-USDT")
    bookB := s.dataCache.GetOrderBook("kucoin", "BTC-USDT") 
    
    // 2. Найти арбитражную возможность
    if bookA.BestBid > bookB.BestAsk * (1 + s.minProfit) {
        // 3. Отправить задачи на исполнение
        taskBuy := OrderTask{
            Exchange: "kucoin",
            Pair: "BTC-USDT", 
            Side: "BUY",
            Type: "MARKET",
            Amount: s.maxAmount,
        }
        taskSell := OrderTask{
            Exchange: "binance",
            Pair: "BTC-USDT",
            Side: "SELL", 
            Type: "MARKET",
            Amount: s.maxAmount,
        }
        
        s.executorRouter.SendTask(taskBuy)
        s.executorRouter.SendTask(taskSell)
        
        // 4. Мониторить исполнение
        s.monitorExecution(taskBuy, taskSell)
    }
}
```

### 2. Внутрибиржевой треугольный арбитраж

```go
func (s *TriangleStrategy) Execute() error {
    // Пример: ERG/USDT, ERG/BTC, BTC/USDT на Binance
    
    // 1. Получить все три orderbook
    ergUsdt := s.dataCache.GetOrderBook("binance", "ERG-USDT")
    ergBtc := s.dataCache.GetOrderBook("binance", "ERG-BTC") 
    btcUsdt := s.dataCache.GetOrderBook("binance", "BTC-USDT")
    
    // 2. Рассчитать треугольный арбитраж
    // Путь: USDT → BTC → ERG → USDT
    crossRate := btcUsdt.BestAsk * ergBtc.BestAsk
    directRate := ergUsdt.BestBid
    
    if directRate > crossRate * (1 + s.minProfit) {
        // 3. Последовательность ордеров
        tasks := []OrderTask{
            {Exchange: "binance", Pair: "BTC-USDT", Side: "BUY", Type: "MARKET"},
            {Exchange: "binance", Pair: "ERG-BTC", Side: "BUY", Type: "MARKET"},
            {Exchange: "binance", Pair: "ERG-USDT", Side: "SELL", Type: "MARKET"},
        }
        
        s.executeSequential(tasks)
    }
}
```

### 3. Лимитный + рыночный арбитраж

```go
func (s *LimitMarketStrategy) Execute() error {
    // 1. Разместить лимитный ордер на выгодной цене
    limitTask := OrderTask{
        Exchange: "binance",
        Pair: "BTC-USDT",
        Side: "BUY", 
        Type: "LIMIT",
        Price: s.targetPrice,
        Amount: s.maxAmount,
    }
    
    orderID := s.executorRouter.SendTask(limitTask)
    
    // 2. Мониторить рынок и исполнение
    go func() {
        for {
            // Проверить исполнение лимитного ордера
            event := s.eventCache.GetOrderEvent(orderID)
            if event.Status == "FILLED" {
                // Исполнен! Закрыть рыночным ордером на другой бирже
                marketTask := OrderTask{
                    Exchange: "kucoin",
                    Pair: "BTC-USDT",
                    Side: "SELL",
                    Type: "MARKET", 
                    Amount: event.FilledAmount,
                }
                s.executorRouter.SendTask(marketTask)
                break
            }
            
            // Проверить изменение рынка
            currentMarket := s.dataCache.GetOrderBook("kucoin", "BTC-USDT")
            if !s.isStillProfitable(currentMarket) {
                // Отменить лимитный ордер
                s.executorRouter.CancelOrder(orderID)
                break
            }
            
            time.Sleep(100 * time.Millisecond)
        }
    }()
}
```

## Управление состоянием и recovery

### Graceful Shutdown
```go
func (m *Manager) GracefulShutdown() error {
    // 1. Остановить прием новых торговых сигналов
    m.tradeMonitor.StopNewTrades()
    
    // 2. Отменить все открытые лимитные ордера
    openOrders := m.database.GetOpenOrders()
    for _, order := range openOrders {
        m.executorRouter.CancelOrder(order.ID)
    }
    
    // 3. Дождаться завершения активных транзакций (timeout 30s)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    m.waitForActiveTransactions(ctx)
    
    // 4. Сохранить состояние
    state := m.captureState()
    m.database.SaveSystemState(state)
    
    // 5. Закрыть соединения
    m.closeAllConnections()
}
```

### Recovery после аварии
```go
func (m *Manager) RecoverFromCrash() error {
    // 1. Загрузить сохраненное состояние
    state := m.database.LoadSystemState()
    
    // 2. Восстановить открытые позиции из БД
    openOrders := m.database.GetOpenOrders()
    for _, order := range openOrders {
        // Проверить статус на бирже
        realStatus := m.checkOrderStatusOnExchange(order)
        if realStatus != order.Status {
            m.database.UpdateOrderStatus(order.ID, realStatus)
        }
    }
    
    // 3. Восстановить активные арбитражные сделки  
    activeTrades := m.database.GetActiveArbitrageTrades()
    for _, trade := range activeTrades {
        // Проверить все ордера сделки
        m.reconcileTradeState(trade)
    }
    
    // 4. Пересинхронизировать балансы
    m.syncAccountBalances()
    
    // 5. Возобновить торговлю
    m.resumeTrading()
}
```

Эта архитектура обеспечивает:
- **Масштабируемость** через множественные воркеры
- **Устойчивость** через graceful shutdown и recovery
- **Производительность** через in-memory cache
- **Надежность** через персистентность и мониторинг
- **Гибкость** через модульную структуру