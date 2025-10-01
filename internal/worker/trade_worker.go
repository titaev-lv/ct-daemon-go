package worker

import (
	"daemon-go/internal/bus"
	"daemon-go/internal/market"
	"fmt"
	"log"
	"sync"
	"time"
)

// ArbitrageOpportunity - структура для арбитражной возможности
type ArbitrageOpportunity struct {
	Symbol          string                   `json:"symbol"`           // унифицированный символ
	UnifiedSymbol   *market.UnifiedSymbol    `json:"unified_symbol"`   // полная информация о символе
	BuyExchange     string                   `json:"buy_exchange"`     // биржа для покупки
	SellExchange    string                   `json:"sell_exchange"`    // биржа для продажи
	BuyPrice        float64                  `json:"buy_price"`        // цена покупки
	SellPrice       float64                  `json:"sell_price"`       // цена продажи
	Spread          float64                  `json:"spread"`           // спред
	ProfitPercent   float64                  `json:"profit_percent"`   // процент профита
	BuyVolume       float64                  `json:"buy_volume"`       // доступный объем для покупки
	SellVolume      float64                  `json:"sell_volume"`      // доступный объем для продажи
	MaxVolume       float64                  `json:"max_volume"`       // максимальный объем сделки
	Timestamp       time.Time                `json:"timestamp"`        // время обнаружения
	BuyOrderBook    *market.UnifiedOrderBook `json:"buy_orderbook"`    // orderbook биржи покупки
	SellOrderBook   *market.UnifiedOrderBook `json:"sell_orderbook"`   // orderbook биржи продажи
	EstimatedProfit float64                  `json:"estimated_profit"` // оценочная прибыль в USDT
}

// TradeWorker - воркер для поиска и исполнения арбитражных сделок
type TradeWorker struct {
	mu             sync.RWMutex
	orderBooks     map[string]map[string]*market.UnifiedOrderBook // [exchange][symbol]
	bestPrices     map[string]map[string]*market.UnifiedBestPrice // [exchange][symbol]
	symbolRegistry *market.SymbolRegistry
	opportunities  []ArbitrageOpportunity
	config         *TradeWorkerConfig
	active         bool
	stopChan       chan struct{}
	messageBus     *bus.MessageBus
	subscriptions  map[string]chan market.UnifiedMessage // [exchange] -> channel

	// Статистика
	totalOpportunities  int64
	executedTrades      int64
	totalProfit         float64
	lastOpportunityTime time.Time
}

// TradeWorkerConfig - конфигурация trade worker
type TradeWorkerConfig struct {
	MinProfitPercent   float64       `json:"min_profit_percent"`  // минимальный процент профита (например, 0.1%)
	MinVolumeUSDT      float64       `json:"min_volume_usdt"`     // минимальный объем в USDT
	MaxVolumeUSDT      float64       `json:"max_volume_usdt"`     // максимальный объем в USDT
	MaxOpportunities   int           `json:"max_opportunities"`   // максимальное количество сохраняемых возможностей
	UpdateInterval     time.Duration `json:"update_interval"`     // интервал проверки арбитража
	EnableExecution    bool          `json:"enable_execution"`    // включить исполнение сделок
	AllowedExchanges   []string      `json:"allowed_exchanges"`   // разрешенные биржи
	BlacklistedSymbols []string      `json:"blacklisted_symbols"` // заблокированные символы
	RequiredSpreadBps  int           `json:"required_spread_bps"` // требуемый спред в базисных пунктах
}

// DefaultTradeWorkerConfig возвращает конфигурацию по умолчанию
func DefaultTradeWorkerConfig() *TradeWorkerConfig {
	return &TradeWorkerConfig{
		MinProfitPercent:   0.1,   // 0.1%
		MinVolumeUSDT:      100,   // $100
		MaxVolumeUSDT:      10000, // $10,000
		MaxOpportunities:   100,
		UpdateInterval:     time.Second,
		EnableExecution:    false, // по умолчанию только мониторинг
		AllowedExchanges:   []string{"binance", "bybit", "kucoin", "htx", "coinex", "poloniex"},
		BlacklistedSymbols: []string{},
		RequiredSpreadBps:  10, // 0.1%
	}
}

// NewTradeWorker создает новый trade worker
func NewTradeWorker(config *TradeWorkerConfig) *TradeWorker {
	if config == nil {
		config = DefaultTradeWorkerConfig()
	}

	return &TradeWorker{
		orderBooks:     make(map[string]map[string]*market.UnifiedOrderBook),
		bestPrices:     make(map[string]map[string]*market.UnifiedBestPrice),
		symbolRegistry: market.NewSymbolRegistry(),
		opportunities:  make([]ArbitrageOpportunity, 0),
		config:         config,
		stopChan:       make(chan struct{}),
		messageBus:     bus.GetInstance(),
		subscriptions:  make(map[string]chan market.UnifiedMessage),
	}
}

// Start запускает trade worker
func (tw *TradeWorker) Start() error {
	tw.mu.Lock()
	if tw.active {
		tw.mu.Unlock()
		return fmt.Errorf("trade worker already active")
	}
	tw.active = true
	tw.mu.Unlock()

	log.Printf("[TradeWorker] Starting with config: MinProfit=%.2f%%, MinVolume=$%.0f, MaxVolume=$%.0f",
		tw.config.MinProfitPercent, tw.config.MinVolumeUSDT, tw.config.MaxVolumeUSDT)

	// Подписываемся на сообщения всех разрешенных бирж
	for _, exchange := range tw.config.AllowedExchanges {
		ch := tw.messageBus.Subscribe(exchange, 100) // буфер на 100 сообщений
		tw.subscriptions[exchange] = ch
		go tw.messageProcessor(exchange, ch)
		log.Printf("[TradeWorker] Subscribed to %s", exchange)
	}

	// Запускаем фоновый процесс поиска арбитража
	go tw.arbitrageLoop()

	return nil
}

// Stop останавливает trade worker
func (tw *TradeWorker) Stop() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if !tw.active {
		return fmt.Errorf("trade worker not active")
	}

	tw.active = false

	// Отписываемся от всех сообщений
	for exchange, ch := range tw.subscriptions {
		tw.messageBus.Unsubscribe(exchange, ch)
		log.Printf("[TradeWorker] Unsubscribed from %s", exchange)
	}
	tw.subscriptions = make(map[string]chan market.UnifiedMessage)

	close(tw.stopChan)
	log.Printf("[TradeWorker] Stopped")
	return nil
}

// messageProcessor обрабатывает сообщения от конкретной биржи
func (tw *TradeWorker) messageProcessor(exchange string, ch chan market.UnifiedMessage) {
	log.Printf("[TradeWorker] Message processor started for %s", exchange)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				log.Printf("[TradeWorker] Message channel closed for %s", exchange)
				return
			}

			// Обрабатываем сообщение
			if err := tw.HandleMessage(msg); err != nil {
				log.Printf("[TradeWorker] Error handling message from %s: %v", exchange, err)
			}

		case <-tw.stopChan:
			log.Printf("[TradeWorker] Message processor stopping for %s", exchange)
			return
		}
	}
}

// HandleMessage обрабатывает унифицированное сообщение (реализует MessageHandler)
func (tw *TradeWorker) HandleMessage(msg market.UnifiedMessage) error {
	switch msg.MessageType {
	case market.MessageTypeOrderBook:
		return tw.handleOrderBook(msg)
	case market.MessageTypeBestPrice:
		return tw.handleBestPrice(msg)
	case market.MessageTypeTicker:
		return tw.handleTicker(msg)
	default:
		// Игнорируем другие типы сообщений
		return nil
	}
}

// handleOrderBook обрабатывает обновления orderbook
func (tw *TradeWorker) handleOrderBook(msg market.UnifiedMessage) error {
	orderBook, ok := msg.Data.(market.UnifiedOrderBook)
	if !ok {
		return fmt.Errorf("invalid orderbook data type")
	}

	tw.mu.Lock()
	if tw.orderBooks[msg.Exchange] == nil {
		tw.orderBooks[msg.Exchange] = make(map[string]*market.UnifiedOrderBook)
	}
	tw.orderBooks[msg.Exchange][msg.Symbol] = &orderBook
	tw.mu.Unlock()

	return nil
}

// handleBestPrice обрабатывает обновления лучших цен
func (tw *TradeWorker) handleBestPrice(msg market.UnifiedMessage) error {
	bestPrice, ok := msg.Data.(market.UnifiedBestPrice)
	if !ok {
		return fmt.Errorf("invalid best price data type")
	}

	tw.mu.Lock()
	if tw.bestPrices[msg.Exchange] == nil {
		tw.bestPrices[msg.Exchange] = make(map[string]*market.UnifiedBestPrice)
	}
	tw.bestPrices[msg.Exchange][msg.Symbol] = &bestPrice
	tw.mu.Unlock()

	return nil
}

// handleTicker обрабатывает обновления ticker (пока не используется для арбитража)
func (tw *TradeWorker) handleTicker(msg market.UnifiedMessage) error {
	// Ticker используется только для статистики, не для арбитража
	return nil
}

// arbitrageLoop основной цикл поиска арбитража
func (tw *TradeWorker) arbitrageLoop() {
	ticker := time.NewTicker(tw.config.UpdateInterval)
	defer ticker.Stop()

	log.Printf("[TradeWorker] Starting arbitrage loop with interval %v", tw.config.UpdateInterval)

	for {
		select {
		case <-tw.stopChan:
			log.Printf("[TradeWorker] Arbitrage loop stopped")
			return
		case <-ticker.C:
			tw.findArbitrageOpportunities()
		}
	}
}

// findArbitrageOpportunities ищет арбитражные возможности
func (tw *TradeWorker) findArbitrageOpportunities() {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	newOpportunities := make([]ArbitrageOpportunity, 0)

	// Получаем все символы
	symbols := tw.getAllSymbols()

	for _, symbol := range symbols {
		opportunities := tw.findArbitrageForSymbol(symbol)
		newOpportunities = append(newOpportunities, opportunities...)
	}

	// Обновляем список возможностей
	tw.mu.RUnlock()
	tw.mu.Lock()
	tw.opportunities = newOpportunities
	tw.totalOpportunities += int64(len(newOpportunities))
	if len(newOpportunities) > 0 {
		tw.lastOpportunityTime = time.Now()
	}
	tw.mu.Unlock()
	tw.mu.RLock()

	// Логируем найденные возможности
	for _, opp := range newOpportunities {
		log.Printf("[ARBITRAGE] %s: Buy %s@%.8f → Sell %s@%.8f | Profit: %.4f%% | Volume: $%.2f",
			opp.Symbol,
			opp.BuyExchange, opp.BuyPrice,
			opp.SellExchange, opp.SellPrice,
			opp.ProfitPercent,
			opp.EstimatedProfit)

		// Исполняем сделку если включено
		if tw.config.EnableExecution {
			go tw.executeTrade(opp)
		}
	}
}

// getAllSymbols возвращает все уникальные символы
func (tw *TradeWorker) getAllSymbols() []string {
	symbolSet := make(map[string]bool)

	// Собираем символы из orderbooks
	for _, exchangeData := range tw.orderBooks {
		for symbol := range exchangeData {
			if !tw.isSymbolBlacklisted(symbol) {
				symbolSet[symbol] = true
			}
		}
	}

	// Собираем символы из best prices
	for _, exchangeData := range tw.bestPrices {
		for symbol := range exchangeData {
			if !tw.isSymbolBlacklisted(symbol) {
				symbolSet[symbol] = true
			}
		}
	}

	symbols := make([]string, 0, len(symbolSet))
	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	return symbols
}

// isSymbolBlacklisted проверяет, заблокирован ли символ
func (tw *TradeWorker) isSymbolBlacklisted(symbol string) bool {
	for _, blacklisted := range tw.config.BlacklistedSymbols {
		if symbol == blacklisted {
			return true
		}
	}
	return false
}

// findArbitrageForSymbol ищет арбитраж для конкретного символа
func (tw *TradeWorker) findArbitrageForSymbol(symbol string) []ArbitrageOpportunity {
	opportunities := make([]ArbitrageOpportunity, 0)

	// Получаем данные по всем биржам для этого символа
	exchangeData := make(map[string]*ArbitrageData)

	for _, exchange := range tw.config.AllowedExchanges {
		data := tw.getArbitrageDataForExchange(exchange, symbol)
		if data != nil {
			exchangeData[exchange] = data
		}
	}

	// Ищем арбитражные возможности между всеми парами бирж
	exchanges := make([]string, 0, len(exchangeData))
	for exchange := range exchangeData {
		exchanges = append(exchanges, exchange)
	}

	for i, buyExchange := range exchanges {
		for j, sellExchange := range exchanges {
			if i >= j {
				continue // Избегаем дублирования и самоарбитража
			}

			buyData := exchangeData[buyExchange]
			sellData := exchangeData[sellExchange]

			// Проверяем возможность арбитража: покупаем на buyExchange, продаем на sellExchange
			if opp := tw.calculateArbitrage(symbol, buyExchange, sellExchange, buyData, sellData); opp != nil {
				opportunities = append(opportunities, *opp)
			}

			// Проверяем обратную возможность
			if opp := tw.calculateArbitrage(symbol, sellExchange, buyExchange, sellData, buyData); opp != nil {
				opportunities = append(opportunities, *opp)
			}
		}
	}

	return opportunities
}

// ArbitrageData - данные для расчета арбитража
type ArbitrageData struct {
	BestBid   float64
	BestAsk   float64
	BidVolume float64
	AskVolume float64
	OrderBook *market.UnifiedOrderBook
	BestPrice *market.UnifiedBestPrice
}

// getArbitrageDataForExchange получает данные для арбитража с биржи
func (tw *TradeWorker) getArbitrageDataForExchange(exchange, symbol string) *ArbitrageData {
	data := &ArbitrageData{}

	// Пробуем получить данные из best prices
	if bestPrice, exists := tw.bestPrices[exchange][symbol]; exists {
		data.BestBid = bestPrice.BestBid
		data.BestAsk = bestPrice.BestAsk
		data.BidVolume = bestPrice.BidVolume
		data.AskVolume = bestPrice.AskVolume
		data.BestPrice = bestPrice
	}

	// Пробуем получить данные из orderbook
	if orderBook, exists := tw.orderBooks[exchange][symbol]; exists {
		data.OrderBook = orderBook
		if len(orderBook.Bids) > 0 {
			data.BestBid = orderBook.Bids[0].Price
			data.BidVolume = orderBook.Bids[0].Volume
		}
		if len(orderBook.Asks) > 0 {
			data.BestAsk = orderBook.Asks[0].Price
			data.AskVolume = orderBook.Asks[0].Volume
		}
	}

	// Проверяем, что у нас есть минимальные данные
	if data.BestBid <= 0 || data.BestAsk <= 0 {
		return nil
	}

	return data
}

// calculateArbitrage рассчитывает арбитражную возможность
func (tw *TradeWorker) calculateArbitrage(symbol, buyExchange, sellExchange string, buyData, sellData *ArbitrageData) *ArbitrageOpportunity {
	// Цена покупки - лучший ask на бирже покупки
	buyPrice := buyData.BestAsk
	// Цена продажи - лучший bid на бирже продажи
	sellPrice := sellData.BestBid

	// Проверяем, что есть прибыль
	if sellPrice <= buyPrice {
		return nil
	}

	spread := sellPrice - buyPrice
	profitPercent := (spread / buyPrice) * 100

	// Проверяем минимальную прибыльность
	if profitPercent < tw.config.MinProfitPercent {
		return nil
	}

	// Рассчитываем максимальный объем
	maxVolume := tw.calculateMaxVolume(buyData, sellData, buyPrice, sellPrice)
	if maxVolume <= 0 {
		return nil
	}

	// Рассчитываем оценочную прибыль в USDT
	estimatedProfit := maxVolume * spread

	// Проверяем минимальный объем
	volumeUSDT := maxVolume * buyPrice
	if volumeUSDT < tw.config.MinVolumeUSDT {
		return nil
	}

	// Ограничиваем максимальный объем
	if volumeUSDT > tw.config.MaxVolumeUSDT {
		maxVolume = tw.config.MaxVolumeUSDT / buyPrice
		estimatedProfit = maxVolume * spread
	}

	return &ArbitrageOpportunity{
		Symbol:          symbol,
		BuyExchange:     buyExchange,
		SellExchange:    sellExchange,
		BuyPrice:        buyPrice,
		SellPrice:       sellPrice,
		Spread:          spread,
		ProfitPercent:   profitPercent,
		BuyVolume:       buyData.AskVolume,
		SellVolume:      sellData.BidVolume,
		MaxVolume:       maxVolume,
		Timestamp:       time.Now(),
		BuyOrderBook:    buyData.OrderBook,
		SellOrderBook:   sellData.OrderBook,
		EstimatedProfit: estimatedProfit,
	}
}

// calculateMaxVolume рассчитывает максимальный объем для арбитража
func (tw *TradeWorker) calculateMaxVolume(buyData, sellData *ArbitrageData, buyPrice, sellPrice float64) float64 {
	// Лимитируем объемом доступным для покупки
	maxBuyVolume := buyData.AskVolume
	// Лимитируем объемом доступным для продажи
	maxSellVolume := sellData.BidVolume

	// Берем минимум
	maxVolume := maxBuyVolume
	if maxSellVolume < maxVolume {
		maxVolume = maxSellVolume
	}

	return maxVolume
}

// executeTrade исполняет арбитражную сделку (заглушка)
func (tw *TradeWorker) executeTrade(opportunity ArbitrageOpportunity) {
	// TODO: Реализовать исполнение сделок через exchange adapters
	log.Printf("[TradeWorker] EXECUTING TRADE: %s %.6f %s→%s (Profit: %.4f%%)",
		opportunity.Symbol, opportunity.MaxVolume,
		opportunity.BuyExchange, opportunity.SellExchange,
		opportunity.ProfitPercent)

	// Обновляем статистику
	tw.mu.Lock()
	tw.executedTrades++
	tw.totalProfit += opportunity.EstimatedProfit
	tw.mu.Unlock()
}

// GetOpportunities возвращает текущие арбитражные возможности
func (tw *TradeWorker) GetOpportunities() []ArbitrageOpportunity {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	// Возвращаем копию
	opportunities := make([]ArbitrageOpportunity, len(tw.opportunities))
	copy(opportunities, tw.opportunities)
	return opportunities
}

// GetStats возвращает статистику trade worker
func (tw *TradeWorker) GetStats() map[string]interface{} {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	return map[string]interface{}{
		"active":                tw.active,
		"total_opportunities":   tw.totalOpportunities,
		"current_opportunities": len(tw.opportunities),
		"executed_trades":       tw.executedTrades,
		"total_profit_usdt":     tw.totalProfit,
		"last_opportunity_time": tw.lastOpportunityTime,
		"tracked_exchanges":     len(tw.orderBooks),
		"tracked_symbols":       len(tw.getAllSymbols()),
		"config":                tw.config,
	}
}
