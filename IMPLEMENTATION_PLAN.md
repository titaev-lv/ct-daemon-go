# План реализации недостающих модулей

## Фаза 1: EXCHANGE EXECUTOR MODULE (Приоритет: Критичный)

### 1.1 Структура модуля
```go
// internal/worker/executor/
├── monitor.go       // ExecutorMonitor
├── worker.go        // ExecutorWorker  
├── queue.go         // OrderQueue (FIFO)
├── types.go         // Order, Task структуры
└── router.go        // Маршрутизация задач
```

### 1.2 Ключевые компоненты

#### ExecutorMonitor
- Создает/удаляет ExecutorWorkers по биржам/пользователям
- Мониторит очереди ордеров
- Балансирует нагрузку между воркерами

#### ExecutorWorker  
- Исполняет рыночные/лимитные ордера через exchange adapters
- Работает с конкретной биржей/аккаунтом
- Обрабатывает очередь FIFO максимально быстро

#### OrderQueue
- FIFO очереди для каждой биржи/пользователя/пары
- Thread-safe операции
- Персистентность для recovery

### 1.3 Интеграция
- TradeWorker отправляет задачи через Router
- ExecutorWorker отправляет результаты в CollectorEvents
- Graceful shutdown с отменой открытых ордеров

## Фаза 2: COLLECTOR EVENTS MODULE (Приоритет: Критичный)

### 2.1 Структура модуля
```go
// internal/worker/collector/
├── monitor.go       // EventMonitor
├── worker.go        // EventWorker
├── cache.go         // EventCache (быстрый доступ)
├── types.go         // Event, Transaction структуры
└── storage.go       // Запись в БД
```

### 2.2 Ключевые компоненты

#### EventMonitor
- Создает EventWorkers по биржам/пользователям
- Мониторит WebSocket соединения
- Управляет подписками на события ордеров

#### EventWorker
- WebSocket подключение для сбора событий
- Парсинг событий исполнения ордеров
- Запись в БД + EventCache

#### EventCache
- In-memory кеш для быстрого доступа TradeWorkers
- LRU/TTL для управления памятью
- Thread-safe операции

### 2.3 Интеграция
- TradeWorker читает события для принятия решений
- ExecutorWorker использует для подтверждения исполнения
- Синхронизация с БД для persistence

## Фаза 3: TRADE STRATEGIES (Приоритет: Высокий)

### 3.1 Базовые стратегии арбитража

#### InterExchangeArbitrage
```go
type InterExchangeMarketStrategy struct {
    exchangeA, exchangeB string
    pair                 string
    minProfitThreshold   float64
}

func (s *InterExchangeMarketStrategy) Execute(ctx context.Context) error {
    // 1. Получить orderbook с обеих бирж
    // 2. Найти арбитражную возможность
    // 3. Отправить 2 рыночных ордера через ExecutorRouter
    // 4. Мониторить исполнение через CollectorEvents
}
```

#### IntraExchangeArbitrage  
```go
type IntraExchangeTriangleStrategy struct {
    exchange    string
    pairA, pairB, pairC string // например ERG/USDT, ERG/BTC, BTC/USDT
}

func (s *IntraExchangeTriangleStrategy) Execute(ctx context.Context) error {
    // 1. Получить orderbook для всех 3 пар
    // 2. Рассчитать треугольный арбитраж
    // 3. Отправить последовательность ордеров
}
```

#### LimitMarketArbitrage
```go
type LimitMarketStrategy struct {
    limitExchange  string
    marketExchange string
    pair          string
}

func (s *LimitMarketStrategy) Execute(ctx context.Context) error {
    // 1. Разместить лимитный ордер на первой бирже
    // 2. Мониторить исполнение через CollectorEvents
    // 3. При исполнении - рыночный ордер на второй бирже
    // 4. Отменить лимитный при ухудшении условий
}
```

### 3.2 Интеграция стратегий в TradeWorker

```go
type TradeWorker struct {
    tradeCase  db.TradeCase
    strategy   Strategy  // интерфейс для стратегий
    dataCache  *DataCache
    executor   *ExecutorRouter
    collector  *EventCollector
}

type Strategy interface {
    Execute(ctx context.Context) error
    ShouldExecute(market MarketData) bool
    Stop() error
}
```

## Фаза 4: SHARED MEMORY / CACHE LAYER (Приоритет: Средний)

### 4.1 DataCache для orderbook/prices
```go
// internal/cache/
├── market_cache.go  // OrderBook, BestPrice кеш
├── config_cache.go  // Настройки, комиссии
├── event_cache.go   // События ордеров
└── manager.go       // Управление кешем
```

### 4.2 Ключевые возможности
- Thread-safe доступ
- TTL для устаревших данных
- LRU eviction policy
- Метрики использования
- Optional Redis backend для cluster

## Фаза 5: GRACEFUL SHUTDOWN & RECOVERY (Приоритет: Средний)

### 5.1 Graceful Shutdown
```go
type ShutdownManager struct {
    executors []ExecutorWorker
    timeout   time.Duration
}

func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
    // 1. Остановить прием новых задач
    // 2. Отменить открытые лимитные ордера
    // 3. Дождаться завершения активных транзакций
    // 4. Сохранить состояние в БД
    // 5. Закрыть соединения
}
```

### 5.2 Recovery System
```go
type RecoveryManager struct {
    stateStorage StateStorage
}

func (rm *RecoveryManager) Recover() error {
    // 1. Загрузить сохраненное состояние
    // 2. Восстановить открытые позиции
    // 3. Пересинхронизировать с биржами
    // 4. Возобновить торговлю
}
```

## ДОПОЛНИТЕЛЬНЫЕ ТАБЛИЦЫ БД

### Новые таблицы для полного функционала

```sql
-- Типы торгов
CREATE TABLE `TRADE_TYPE` (
  `ID` int PRIMARY KEY AUTO_INCREMENT,
  `NAME` varchar(50) NOT NULL,
  `DESCRIPTION` text
);

-- Ордера
CREATE TABLE `ORDERS` (
  `ID` bigint PRIMARY KEY AUTO_INCREMENT,
  `TRADE_ID` int NOT NULL,
  `EXCHANGE_ID` int NOT NULL,
  `PAIR_ID` int NOT NULL,
  `ORDER_TYPE` enum('MARKET','LIMIT') NOT NULL,
  `SIDE` enum('BUY','SELL') NOT NULL,
  `AMOUNT` decimal(30,12) NOT NULL,
  `PRICE` decimal(30,12),
  `STATUS` enum('PENDING','FILLED','CANCELLED','PARTIAL') NOT NULL,
  `EXTERNAL_ORDER_ID` varchar(100),
  `CREATED_AT` timestamp DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (`TRADE_ID`) REFERENCES `TRADE`(`ID`),
  FOREIGN KEY (`EXCHANGE_ID`) REFERENCES `EXCHANGE`(`ID`),
  FOREIGN KEY (`PAIR_ID`) REFERENCES `SPOT_TRADE_PAIR`(`ID`)
);

-- Арбитражные транзакции
CREATE TABLE `ARBITRAGE_TRANS` (
  `ID` bigint PRIMARY KEY AUTO_INCREMENT,
  `TRADE_ID` int NOT NULL,
  `STATUS` enum('PENDING','COMPLETED','FAILED') NOT NULL,
  `PROFIT_USD` decimal(30,12),
  `START_TIME` timestamp DEFAULT CURRENT_TIMESTAMP,
  `END_TIME` timestamp NULL,
  FOREIGN KEY (`TRADE_ID`) REFERENCES `TRADE`(`ID`)
);

-- Транзакции ордеров
CREATE TABLE `ORDER_TRANSACTIONS` (
  `ID` bigint PRIMARY KEY AUTO_INCREMENT,
  `ORDER_ID` bigint NOT NULL,
  `ARBITRAGE_TRANS_ID` bigint,
  `AMOUNT_FILLED` decimal(30,12) NOT NULL,
  `PRICE_FILLED` decimal(30,12) NOT NULL,
  `FEE` decimal(30,12),
  `TIMESTAMP` timestamp DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (`ORDER_ID`) REFERENCES `ORDERS`(`ID`),
  FOREIGN KEY (`ARBITRAGE_TRANS_ID`) REFERENCES `ARBITRAGE_TRANS`(`ID`)
);

-- Мониторинг (используется в SQL запросах)
CREATE TABLE `MONITORING` (
  `ID` int PRIMARY KEY AUTO_INCREMENT,
  `USER_ID` int NOT NULL,
  `ACTIVE` tinyint(1) DEFAULT 1,
  FOREIGN KEY (`USER_ID`) REFERENCES `USER`(`ID`)
);

CREATE TABLE `MONITORING_SPOT_ARRAYS` (
  `MONITOR_ID` int NOT NULL,
  `PAIR_ID` int NOT NULL,
  FOREIGN KEY (`MONITOR_ID`) REFERENCES `MONITORING`(`ID`),
  FOREIGN KEY (`PAIR_ID`) REFERENCES `SPOT_TRADE_PAIR`(`ID`)
);
```

## ВРЕМЕННЫЕ РАМКИ

### Sprint 1 (Неделя 1): Exchange Executor
- День 1-2: ExecutorMonitor + базовая структура
- День 3-4: ExecutorWorker + OrderQueue  
- День 5-7: Интеграция, тестирование

### Sprint 2 (Неделя 2): Collector Events
- День 1-2: EventMonitor + WebSocket infrastructure
- День 3-4: EventWorker + EventCache
- День 5-7: Интеграция с БД, тестирование

### Sprint 3 (Неделя 3): Trade Strategies
- День 1-3: Базовые стратегии арбитража
- День 4-5: Интеграция с Executor/Collector
- День 6-7: Тестирование стратегий

### Sprint 4 (Неделя 4): Cache & Optimization
- День 1-3: DataCache implementation
- День 4-5: Graceful shutdown
- День 6-7: Recovery system

Общий срок: **4 недели** до production-ready системы.