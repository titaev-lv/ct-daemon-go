package handlers

import (
	"daemon-go/internal/market"
	"fmt"
	"log"
)

// DataCollector - обработчик для сбора рыночных данных (не ищет арбитраж)
type DataCollector struct {
	orderBooks map[string]map[string]*market.UnifiedOrderBook // [exchange][symbol]
	tickers    map[string]map[string]*market.UnifiedTicker    // [exchange][symbol]
	bestPrices map[string]map[string]*market.UnifiedBestPrice // [exchange][symbol]
}

func NewDataCollector() *DataCollector {
	return &DataCollector{
		orderBooks: make(map[string]map[string]*market.UnifiedOrderBook),
		tickers:    make(map[string]map[string]*market.UnifiedTicker),
		bestPrices: make(map[string]map[string]*market.UnifiedBestPrice),
	}
}

// HandleMessage обрабатывает унифицированное сообщение (только собирает данные)
func (h *DataCollector) HandleMessage(msg market.UnifiedMessage) error {
	switch msg.MessageType {
	case market.MessageTypeOrderBook:
		return h.handleOrderBook(msg)
	case market.MessageTypeTicker:
		return h.handleTicker(msg)
	case market.MessageTypeBestPrice:
		return h.handleBestPrice(msg)
	default:
		log.Printf("[DataCollector] Unknown message type: %s", msg.MessageType)
		return nil
	}
}

func (h *DataCollector) handleOrderBook(msg market.UnifiedMessage) error {
	orderBook, ok := msg.Data.(market.UnifiedOrderBook)
	if !ok {
		return fmt.Errorf("invalid orderbook data type")
	}

	// Инициализируем карты если нужно
	if h.orderBooks[msg.Exchange] == nil {
		h.orderBooks[msg.Exchange] = make(map[string]*market.UnifiedOrderBook)
	}

	// Сохраняем orderbook
	h.orderBooks[msg.Exchange][msg.Symbol] = &orderBook

	log.Printf("[DataCollector] OrderBook updated: %s %s - Bids: %d, Asks: %d",
		msg.Exchange, msg.Symbol, len(orderBook.Bids), len(orderBook.Asks))

	return nil
}

func (h *DataCollector) handleTicker(msg market.UnifiedMessage) error {
	ticker, ok := msg.Data.(market.UnifiedTicker)
	if !ok {
		return fmt.Errorf("invalid ticker data type")
	}

	// Инициализируем карты если нужно
	if h.tickers[msg.Exchange] == nil {
		h.tickers[msg.Exchange] = make(map[string]*market.UnifiedTicker)
	}

	// Сохраняем ticker
	h.tickers[msg.Exchange][msg.Symbol] = &ticker

	log.Printf("[DataCollector] Ticker updated: %s %s - Last: %.8f, Bid: %.8f, Ask: %.8f",
		msg.Exchange, msg.Symbol, ticker.LastPrice, ticker.BestBid, ticker.BestAsk)

	return nil
}

func (h *DataCollector) handleBestPrice(msg market.UnifiedMessage) error {
	bestPrice, ok := msg.Data.(market.UnifiedBestPrice)
	if !ok {
		return fmt.Errorf("invalid best price data type")
	}

	// Инициализируем карты если нужно
	if h.bestPrices[msg.Exchange] == nil {
		h.bestPrices[msg.Exchange] = make(map[string]*market.UnifiedBestPrice)
	}

	// Сохраняем best price
	h.bestPrices[msg.Exchange][msg.Symbol] = &bestPrice

	log.Printf("[DataCollector] BestPrice updated: %s %s - Bid: %.8f (%.4f), Ask: %.8f (%.4f)",
		msg.Exchange, msg.Symbol,
		bestPrice.BestBid, bestPrice.BidVolume,
		bestPrice.BestAsk, bestPrice.AskVolume)

	return nil
}

// GetStats возвращает статистику по собранным данным
func (h *DataCollector) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["total_exchanges"] = len(h.bestPrices)
	stats["total_symbols"] = h.getTotalSymbols()
	stats["orderbook_updates"] = h.getTotalOrderBooks()
	stats["ticker_updates"] = h.getTotalTickers()
	stats["best_price_updates"] = h.getTotalBestPrices()

	return stats
}

func (h *DataCollector) getTotalSymbols() int {
	symbols := make(map[string]bool)
	for _, exchangeData := range h.bestPrices {
		for symbol := range exchangeData {
			symbols[symbol] = true
		}
	}
	return len(symbols)
}

func (h *DataCollector) getTotalOrderBooks() int {
	count := 0
	for _, exchangeData := range h.orderBooks {
		count += len(exchangeData)
	}
	return count
}

func (h *DataCollector) getTotalTickers() int {
	count := 0
	for _, exchangeData := range h.tickers {
		count += len(exchangeData)
	}
	return count
}

func (h *DataCollector) getTotalBestPrices() int {
	count := 0
	for _, exchangeData := range h.bestPrices {
		count += len(exchangeData)
	}
	return count
}
