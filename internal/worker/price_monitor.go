package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"daemon-go/internal/bus"
	"daemon-go/internal/db"
	"daemon-go/internal/market"
	sqlMySQL "daemon-go/internal/sql/mysql"
	sqlPostgres "daemon-go/internal/sql/postgres"
	"daemon-go/pkg/log"
)

// PriceMonitorPair представляет пару для мониторинга цен
type PriceMonitorPair struct {
	ExchangeID   int    `db:"EXCHANGE_ID"`
	PairID       int    `db:"PAIR_ID"`
	ExchangeName string `db:"NAME"`
}

// PriceData представляет данные о ценах для записи в БД
type PriceData struct {
	PairID         int
	PriceTimestamp time.Time
	Date           time.Time
	// Asks (продажи) - от лучшей цены (самой низкой)
	Asks1Price  float64
	Asks1Volume float64
	Asks2Price  float64
	Asks2Volume float64
	Asks3Price  float64
	Asks3Volume float64
	Asks4Price  float64
	Asks4Volume float64
	Asks5Price  float64
	Asks5Volume float64
	// Bids (покупки) - от лучшей цены (самой высокой)
	Bids1Price  float64
	Bids1Volume float64
	Bids2Price  float64
	Bids2Volume float64
	Bids3Price  float64
	Bids3Volume float64
	Bids4Price  float64
	Bids4Volume float64
	Bids5Price  float64
	Bids5Volume float64
}

// PriceMonitor отвечает за мониторинг цен и запись в БД
type PriceMonitor struct {
	db       db.DBDriver
	bus      *bus.MessageBus
	logger   *log.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	wg       sync.WaitGroup

	// Кэш последних данных orderbook по PairID
	orderBooksMutex sync.RWMutex
	orderBooks      map[int]*market.UnifiedOrderBook // [pairID]
	subscriber      chan market.UnifiedMessage       // единый подписчик
	monitoringPairs map[int]PriceMonitorPair         // [pairID]
} // NewPriceMonitor создает новый экземпляр PriceMonitor
func NewPriceMonitor(dbDriver db.DBDriver, interval time.Duration) *PriceMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &PriceMonitor{
		db:              dbDriver,
		bus:             bus.GetInstance(),
		logger:          log.New("price_monitor"),
		ctx:             ctx,
		cancel:          cancel,
		interval:        interval,
		orderBooks:      make(map[int]*market.UnifiedOrderBook),
		monitoringPairs: make(map[int]PriceMonitorPair),
	}
}

// Start запускает мониторинг цен
func (pm *PriceMonitor) Start() error {
	pm.logger.Info("Starting price monitor with interval %v", pm.interval)

	// Сначала загружаем список пар для мониторинга
	pairs, err := pm.getMonitoringPairs()
	if err != nil {
		return fmt.Errorf("failed to get monitoring pairs: %w", err)
	}

	// Сохраняем пары в карту для быстрого поиска
	for _, pair := range pairs {
		pm.monitoringPairs[pair.PairID] = pair
	}

	pm.logger.Info("Loaded %d pairs for monitoring", len(pairs))

	// Подписываемся на все биржи (universal subscriber)
	// Сообщения фильтруются по PairID в процессе обработки
	exchanges := []string{"binance", "kucoin", "bybit", "htx", "poloniex", "coinex"}
	pm.subscriber = make(chan market.UnifiedMessage, 1000) // большой буфер

	for _, exchange := range exchanges {
		ch := pm.bus.Subscribe(exchange, 100)
		pm.wg.Add(1)
		go pm.forwardMessages(exchange, ch)
	}

	// Запускаем обработчик сообщений
	pm.wg.Add(1)
	go pm.processMessages()

	// Запускаем цикл мониторинга
	pm.wg.Add(1)
	go pm.monitorLoop()

	pm.logger.Info("Price monitor started")
	return nil
} // Stop останавливает мониторинг цен
func (pm *PriceMonitor) Stop() {
	pm.logger.Info("Stopping price monitor...")
	pm.cancel()

	// Закрываем основной канал
	if pm.subscriber != nil {
		close(pm.subscriber)
	}

	pm.wg.Wait()
	pm.logger.Info("Price monitor stopped")
}

// forwardMessages перенаправляет сообщения с биржи в общий канал
func (pm *PriceMonitor) forwardMessages(exchange string, ch chan market.UnifiedMessage) {
	defer pm.wg.Done()

	pm.logger.Debug("Started forwarding messages for exchange %s", exchange)

	for {
		select {
		case <-pm.ctx.Done():
			pm.logger.Debug("Message forwarding stopped for exchange %s", exchange)
			return
		case msg, ok := <-ch:
			if !ok {
				pm.logger.Debug("Message channel closed for exchange %s", exchange)
				return
			}

			// Пересылаем в общий канал
			select {
			case pm.subscriber <- msg:
			case <-pm.ctx.Done():
				return
			default:
				pm.logger.Warn("Main subscriber channel full, dropping message from %s", exchange)
			}
		}
	}
}

// processMessages обрабатывает сообщения от всех бирж
func (pm *PriceMonitor) processMessages() {
	defer pm.wg.Done()

	pm.logger.Debug("Started processing unified messages")

	for {
		select {
		case <-pm.ctx.Done():
			pm.logger.Debug("Message processing stopped")
			return
		case msg, ok := <-pm.subscriber:
			if !ok {
				pm.logger.Debug("Subscriber channel closed")
				return
			}

			// Обрабатываем только orderbook сообщения с нужными PairID
			if msg.MessageType == market.MessageTypeOrderBook && msg.PairID > 0 {
				// Проверяем, что эта пара в мониторинге
				if _, exists := pm.monitoringPairs[msg.PairID]; exists {
					if err := pm.handleOrderBookMessage(msg); err != nil {
						pm.logger.Error("Failed to handle orderbook message for pair %d: %v", msg.PairID, err)
					}
				}
			}
		}
	}
}

// handleOrderBookMessage обрабатывает сообщение orderbook
func (pm *PriceMonitor) handleOrderBookMessage(msg market.UnifiedMessage) error {
	orderBook, ok := msg.Data.(market.UnifiedOrderBook)
	if !ok {
		return fmt.Errorf("invalid orderbook data type")
	}

	pm.orderBooksMutex.Lock()
	defer pm.orderBooksMutex.Unlock()

	// Сохраняем последний orderbook по PairID
	pm.orderBooks[msg.PairID] = &orderBook

	pm.logger.Debug("Updated orderbook cache: PairID=%d %s %s - Bids: %d, Asks: %d",
		msg.PairID, msg.Exchange, msg.Symbol, len(orderBook.Bids), len(orderBook.Asks))

	return nil
} // monitorLoop основной цикл мониторинга
func (pm *PriceMonitor) monitorLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			pm.logger.Debug("Monitor loop received stop signal")
			return
		case <-ticker.C:
			if err := pm.collectAndSavePrices(); err != nil {
				pm.logger.Error("Failed to collect and save prices: %v", err)
			}
		}
	}
}

// collectAndSavePrices собирает цены и сохраняет их в БД
func (pm *PriceMonitor) collectAndSavePrices() error {
	pm.logger.Debug("Starting price collection cycle")

	// Получаем список пар для мониторинга
	pairs, err := pm.getMonitoringPairs()
	if err != nil {
		return fmt.Errorf("failed to get monitoring pairs: %w", err)
	}

	if len(pairs) == 0 {
		pm.logger.Debug("No pairs to monitor")
		return nil
	}

	pm.logger.Debug("Found %d pairs to monitor", len(pairs))

	// Собираем данные о ценах
	priceDataList := make([]PriceData, 0, len(pairs))
	timestamp := time.Now()

	for _, pair := range pairs {
		priceData, err := pm.collectPriceData(pair, timestamp)
		if err != nil {
			pm.logger.Warn("Failed to collect price data for pair %d (exchange %s): %v",
				pair.PairID, pair.ExchangeName, err)
			continue
		}
		priceDataList = append(priceDataList, *priceData)
	}

	if len(priceDataList) == 0 {
		pm.logger.Debug("No price data collected")
		return nil
	}

	// Сохраняем все данные одной транзакцией
	if err := pm.savePriceData(priceDataList); err != nil {
		return fmt.Errorf("failed to save price data: %w", err)
	}

	pm.logger.Debug("Successfully saved price data for %d pairs", len(priceDataList))
	return nil
}

// getMonitoringPairs получает список пар для мониторинга из БД
func (pm *PriceMonitor) getMonitoringPairs() ([]PriceMonitorPair, error) {
	query := pm.getMonitoringPairsQuery()

	rows, err := pm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute monitoring pairs query: %w", err)
	}
	defer rows.Close()

	var pairs []PriceMonitorPair
	for rows.Next() {
		var pair PriceMonitorPair
		if err := rows.Scan(&pair.ExchangeID, &pair.PairID, &pair.ExchangeName); err != nil {
			pm.logger.Error("Failed to scan monitoring pair row: %v", err)
			continue
		}
		pairs = append(pairs, pair)
	}

	return pairs, nil
}

// collectPriceData собирает данные о ценах для конкретной пары
func (pm *PriceMonitor) collectPriceData(pair PriceMonitorPair, timestamp time.Time) (*PriceData, error) {
	pm.logger.Debug("Collecting price data for pair %d on exchange %s",
		pair.PairID, pair.ExchangeName)

	// Получаем последний orderbook из кэша по PairID
	pm.orderBooksMutex.RLock()
	orderBook, exists := pm.orderBooks[pair.PairID]
	if !exists {
		pm.orderBooksMutex.RUnlock()
		return nil, fmt.Errorf("no orderbook data available for pair %d on exchange %s",
			pair.PairID, pair.ExchangeName)
	}

	// Создаем копию данных orderbook чтобы быстро освободить мьютекс
	bids := make([]market.PriceLevel, len(orderBook.Bids))
	asks := make([]market.PriceLevel, len(orderBook.Asks))
	copy(bids, orderBook.Bids)
	copy(asks, orderBook.Asks)
	pm.orderBooksMutex.RUnlock()

	// Проверяем что у нас есть достаточно данных
	if len(bids) == 0 || len(asks) == 0 {
		return nil, fmt.Errorf("insufficient orderbook data for pair %d on %s - bids: %d, asks: %d",
			pair.PairID, pair.ExchangeName, len(bids), len(asks))
	}

	priceData := &PriceData{
		PairID:         pair.PairID,
		PriceTimestamp: timestamp,
		Date:           timestamp.Truncate(24 * time.Hour),
	}

	// Извлекаем до 5 уровней asks (продажи, сортированы по возрастанию цены)
	if len(asks) >= 1 {
		priceData.Asks1Price = asks[0].Price
		priceData.Asks1Volume = asks[0].Volume
	}
	if len(asks) >= 2 {
		priceData.Asks2Price = asks[1].Price
		priceData.Asks2Volume = asks[1].Volume
	}
	if len(asks) >= 3 {
		priceData.Asks3Price = asks[2].Price
		priceData.Asks3Volume = asks[2].Volume
	}
	if len(asks) >= 4 {
		priceData.Asks4Price = asks[3].Price
		priceData.Asks4Volume = asks[3].Volume
	}
	if len(asks) >= 5 {
		priceData.Asks5Price = asks[4].Price
		priceData.Asks5Volume = asks[4].Volume
	}

	// Извлекаем до 5 уровней bids (покупки, сортированы по убыванию цены)
	if len(bids) >= 1 {
		priceData.Bids1Price = bids[0].Price
		priceData.Bids1Volume = bids[0].Volume
	}
	if len(bids) >= 2 {
		priceData.Bids2Price = bids[1].Price
		priceData.Bids2Volume = bids[1].Volume
	}
	if len(bids) >= 3 {
		priceData.Bids3Price = bids[2].Price
		priceData.Bids3Volume = bids[2].Volume
	}
	if len(bids) >= 4 {
		priceData.Bids4Price = bids[3].Price
		priceData.Bids4Volume = bids[3].Volume
	}
	if len(bids) >= 5 {
		priceData.Bids5Price = bids[4].Price
		priceData.Bids5Volume = bids[4].Volume
	}

	pm.logger.Debug("Collected price data for pair %d on %s: Best Ask=%.8f, Best Bid=%.8f",
		pair.PairID, pair.ExchangeName, priceData.Asks1Price, priceData.Bids1Price)

	return priceData, nil
} // savePriceData сохраняет данные о ценах в БД одной транзакцией
func (pm *PriceMonitor) savePriceData(priceDataList []PriceData) error {
	// Начинаем транзакцию
	tx, err := pm.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Автоматический rollback если не было commit

	insertQuery := pm.getInsertPriceQuery()
	stmt, err := tx.Prepare(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	// Вставляем все записи
	for _, priceData := range priceDataList {
		_, err := stmt.Exec(
			priceData.Date,
			priceData.PriceTimestamp,
			priceData.PairID,
			priceData.Asks5Price, priceData.Asks5Volume,
			priceData.Asks4Price, priceData.Asks4Volume,
			priceData.Asks3Price, priceData.Asks3Volume,
			priceData.Asks2Price, priceData.Asks2Volume,
			priceData.Asks1Price, priceData.Asks1Volume,
			priceData.Bids1Price, priceData.Bids1Volume,
			priceData.Bids2Price, priceData.Bids2Volume,
			priceData.Bids3Price, priceData.Bids3Volume,
			priceData.Bids4Price, priceData.Bids4Volume,
			priceData.Bids5Price, priceData.Bids5Volume,
		)
		if err != nil {
			return fmt.Errorf("failed to insert price data for pair %d: %w", priceData.PairID, err)
		}
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	pm.logger.Info("Successfully saved %d price records", len(priceDataList))
	return nil
}

// getMonitoringPairsQuery возвращает SQL запрос для получения пар мониторинга
func (pm *PriceMonitor) getMonitoringPairsQuery() string {
	switch pm.db.GetType() {
	case "postgres":
		return sqlPostgres.GetMonitoringPairs
	default: // MySQL
		return sqlMySQL.GetMonitoringPairs
	}
}

// getInsertPriceQuery возвращает SQL запрос для вставки данных о ценах
func (pm *PriceMonitor) getInsertPriceQuery() string {
	switch pm.db.GetType() {
	case "postgres":
		return sqlPostgres.InsertPriceData
	default: // MySQL
		return sqlMySQL.InsertPriceData
	}
}
