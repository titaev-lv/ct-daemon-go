package exchange

import (
	"daemon-go/internal/db"
	"daemon-go/internal/market"
	"daemon-go/internal/market/parsers"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// EnhancedBinanceAdapter - улучшенный адаптер Binance с поддержкой унифицированных сообщений
type EnhancedBinanceAdapter struct {
	exchange         db.Exchange
	rest             *CexRestClient
	ws               *EnhancedCexWsClient
	active           bool
	lastPairs        []string
	lastMarketType   string
	lastDepth        int
	messageProcessor *market.MessageProcessor
}

// NewEnhancedBinanceAdapter создает улучшенный BinanceAdapter
func NewEnhancedBinanceAdapter(ex db.Exchange, processor *market.MessageProcessor) *EnhancedBinanceAdapter {
	var wsClient *EnhancedCexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewEnhancedCexWsClient(ex.WsUrl.String, "binance", processor)
	}

	// Регистрируем парсер Binance
	if processor != nil {
		binanceParser := parsers.NewBinanceParser()
		processor.RegisterParser("binance", binanceParser)
	}

	return &EnhancedBinanceAdapter{
		exchange:         ex,
		rest:             NewCexRestClient(ex.BaseUrl),
		ws:               wsClient,
		active:           false,
		messageProcessor: processor,
	}
}

// Start запускает адаптер
func (a *EnhancedBinanceAdapter) Start() error {
	// Проверяем подключение к REST API
	var result map[string]interface{}
	err := a.rest.GetJSON("/api/v3/ping", &result)
	if err != nil {
		a.active = false
		return fmt.Errorf("EnhancedBinanceAdapter: ping failed: %w", err)
	}

	// Подключаемся к WebSocket
	if a.ws != nil {
		if err := a.ws.Connect(); err != nil {
			return fmt.Errorf("EnhancedBinanceAdapter: websocket connect failed: %w", err)
		}
		// Запускаем обработку сообщений
		a.ws.StartMessageProcessing()
	}

	a.active = true
	log.Printf("[EnhancedBinanceAdapter] Started successfully")
	return nil
}

// Stop останавливает адаптер
func (a *EnhancedBinanceAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		return a.ws.Close()
	}
	return nil
}

// SubscribeMarkets подписывается на торговые пары
func (a *EnhancedBinanceAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("EnhancedBinanceAdapter: ws not connected")
	}

	var streams []string
	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")

		// Подписываемся на orderbook, ticker и best price
		streams = append(streams, fmt.Sprintf("%s@depth%d", symbol, depth))
		streams = append(streams, fmt.Sprintf("%s@ticker", symbol))
		streams = append(streams, fmt.Sprintf("%s@bookTicker", symbol))
	}

	sub := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": streams,
		"id":     1,
	}

	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("EnhancedBinanceAdapter: marshal sub: %w", err)
	}

	if err := a.ws.WriteMessage(1, data); err != nil {
		return fmt.Errorf("EnhancedBinanceAdapter: ws sub: %w", err)
	}

	log.Printf("[EnhancedBinanceAdapter] Subscribed to %d streams for %d pairs", len(streams), len(pairs))
	return nil
}

// UnsubscribeMarkets отписывается от торговых пар
func (a *EnhancedBinanceAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("EnhancedBinanceAdapter: ws not connected")
	}

	var streams []string
	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		streams = append(streams, fmt.Sprintf("%s@depth%d", symbol, depth))
		streams = append(streams, fmt.Sprintf("%s@ticker", symbol))
		streams = append(streams, fmt.Sprintf("%s@bookTicker", symbol))
	}

	unsub := map[string]interface{}{
		"method": "UNSUBSCRIBE",
		"params": streams,
		"id":     2,
	}

	data, err := json.Marshal(unsub)
	if err != nil {
		return fmt.Errorf("EnhancedBinanceAdapter: marshal unsub: %w", err)
	}

	if err := a.ws.WriteMessage(1, data); err != nil {
		return fmt.Errorf("EnhancedBinanceAdapter: ws unsub: %w", err)
	}

	log.Printf("[EnhancedBinanceAdapter] Unsubscribed from %d streams for %d pairs", len(streams), len(pairs))
	return nil
}

// GetExchange возвращает информацию о бирже
func (a *EnhancedBinanceAdapter) GetExchange() db.Exchange {
	return a.exchange
}

// IsActive возвращает статус активности
func (a *EnhancedBinanceAdapter) IsActive() bool {
	return a.active
}

// GetPairs возвращает список торговых пар
func (a *EnhancedBinanceAdapter) GetPairs() []string {
	// Получаем список торговых пар через REST API
	var result map[string]interface{}
	err := a.rest.GetJSON("/api/v3/exchangeInfo", &result)
	if err != nil {
		log.Printf("[EnhancedBinanceAdapter] Failed to get pairs: %v", err)
		return []string{}
	}

	var pairs []string
	if symbols, ok := result["symbols"].([]interface{}); ok {
		for _, symbolData := range symbols {
			if symbol, ok := symbolData.(map[string]interface{}); ok {
				if symbolName, ok := symbol["symbol"].(string); ok {
					pairs = append(pairs, symbolName)
				}
			}
		}
	}

	return pairs
}

// Реализуем остальные методы интерфейса Adapter
func (a *EnhancedBinanceAdapter) GetBalance(currency string) (float64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (a *EnhancedBinanceAdapter) PlaceOrder(pair, side, orderType string, amount, price float64) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (a *EnhancedBinanceAdapter) CancelOrder(pair, orderID string) error {
	return fmt.Errorf("not implemented")
}

func (a *EnhancedBinanceAdapter) GetOrderStatus(pair, orderID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
