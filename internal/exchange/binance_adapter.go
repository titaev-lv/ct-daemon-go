package exchange

import (
	"daemon-go/internal/bus"
	"daemon-go/internal/db"
	"daemon-go/internal/market"
	"daemon-go/internal/market/parsers"
	"daemon-go/pkg/log"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BinanceAdapter реализует Adapter для биржи Binance
type BinanceAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.BinanceParser
	messageBus     *bus.MessageBus
	pairIDMap      map[string]int // symbol -> pairID маппинг
}

// SubscribeMarkets для Binance с подробным логированием
func (a *BinanceAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[BINANCE_ADAPTER] SubscribeMarkets started: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)

	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[BINANCE_ADAPTER] WebSocket not connected")
		return fmt.Errorf("BinanceAdapter: ws not connected")
	}

	a.logger.Debug("[BINANCE_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	var streams []string
	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		a.logger.Debug("[BINANCE_ADAPTER] Processing pair: %s -> symbol: %s", pair, symbol)

		streams = append(streams, fmt.Sprintf("%s@depth%d", symbol, depth))
		streams = append(streams, fmt.Sprintf("%s@bookTicker", symbol))
	}

	sub := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": streams,
		"id":     1,
	}
	data, err := json.Marshal(sub)
	if err != nil {
		a.logger.Error("[BINANCE_ADAPTER] Failed to marshal subscription: %v", err)
		return fmt.Errorf("BinanceAdapter: marshal sub: %w", err)
	}

	a.logger.Debug("[BINANCE_ADAPTER] Sending subscription: %s", string(data))
	if err := a.ws.WriteMessage(1, data); err != nil {
		a.logger.Error("[BINANCE_ADAPTER] Failed to send subscription: %v", err)
		return fmt.Errorf("BinanceAdapter: ws sub: %w", err)
	}

	a.logger.Info("[BINANCE_ADAPTER] Successfully subscribed to %d pairs with streams: %v", len(pairs), streams)
	return nil
}

// UnsubscribeMarkets реализует отписку от пар для Binance
func (a *BinanceAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[BINANCE_ADAPTER] UnsubscribeMarkets started: pairs=%v", pairs)

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[BINANCE_ADAPTER] WebSocket not connected for unsubscribe")
		return fmt.Errorf("BinanceAdapter: ws not connected")
	}

	var streams []string
	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		a.logger.Debug("[BINANCE_ADAPTER] Unsubscribing from pair: %s -> symbol: %s", pair, symbol)

		streams = append(streams, fmt.Sprintf("%s@depth%d", symbol, depth))
		streams = append(streams, fmt.Sprintf("%s@bookTicker", symbol))
	}

	unsub := map[string]interface{}{
		"method": "UNSUBSCRIBE",
		"params": streams,
		"id":     2,
	}
	data, err := json.Marshal(unsub)
	if err != nil {
		a.logger.Error("[BINANCE_ADAPTER] Failed to marshal unsubscription: %v", err)
		return fmt.Errorf("BinanceAdapter: marshal unsub: %w", err)
	}

	a.logger.Debug("[BINANCE_ADAPTER] Sending unsubscription: %s", string(data))
	if err := a.ws.WriteMessage(1, data); err != nil {
		a.logger.Error("[BINANCE_ADAPTER] Failed to send unsubscription: %v", err)
		return fmt.Errorf("BinanceAdapter: ws unsub: %w", err)
	}

	a.logger.Info("[BINANCE_ADAPTER] Successfully unsubscribed from %d pairs", len(pairs))
	return nil
}

// NewBinanceAdapter создает BinanceAdapter на основе данных из db.Exchange
func NewBinanceAdapter(ex db.Exchange) *BinanceAdapter {
	var wsClient *CexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewCexWsClient(ex.WsUrl.String)
	}
	return &BinanceAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         wsClient,
		active:     false,
		logger:     log.New("binance_adapter"),
		parser:     parsers.NewBinanceParser(),
		messageBus: bus.GetInstance(),
		pairIDMap:  make(map[string]int),
	}
}

func (a *BinanceAdapter) Start() error {
	a.logger.Info("[BINANCE_ADAPTER] Starting adapter...")

	var result map[string]interface{}
	a.logger.Debug("[BINANCE_ADAPTER] Testing connectivity with ping...")
	err := a.rest.GetJSON("/api/v3/ping", &result)
	if err != nil {
		a.active = false
		a.logger.Error("[BINANCE_ADAPTER] Ping failed: %v", err)
		return fmt.Errorf("BinanceAdapter: ping failed: %w", err)
	}
	a.logger.Debug("[BINANCE_ADAPTER] Ping successful")

	if a.ws != nil {
		a.logger.Debug("[BINANCE_ADAPTER] Connecting WebSocket...")
		if err := a.ws.Connect(); err != nil {
			a.active = false
			a.logger.Error("[BINANCE_ADAPTER] WebSocket connect failed: %v", err)
			return fmt.Errorf("BinanceAdapter: ws connect failed: %w", err)
		}
		a.logger.Info("[BINANCE_ADAPTER] WebSocket connected successfully")

		go a.readLoopWithReconnect()
		go a.ws.WriteLoop(25 * time.Second)
		a.logger.Debug("[BINANCE_ADAPTER] Read and write loops started")
	}

	a.active = true
	a.logger.Info("[BINANCE_ADAPTER] Adapter started successfully")
	return nil

}

func (a *BinanceAdapter) readLoopWithReconnect() {
	for {
		if a.ws == nil || !a.active {
			a.logger.Debug("[BINANCE_ADAPTER] WebSocket is nil or adapter inactive, stopping read loop")
			return
		}

		_, message, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[BINANCE_ADAPTER] Read error: %v, reconnecting...", err)
			for {
				if !a.active {
					a.logger.Debug("[BINANCE_ADAPTER] Adapter inactive during reconnect, exiting")
					return
				}
				time.Sleep(3 * time.Second)
				a.logger.Debug("[BINANCE_ADAPTER] Attempting reconnection...")
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[BINANCE_ADAPTER] Reconnected successfully, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[BINANCE_ADAPTER] Resubscribe error: %v", err)
					} else {
						a.logger.Debug("[BINANCE_ADAPTER] Resubscribed successfully")
					}
					break
				} else {
					a.logger.Error("[BINANCE_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Парсим WebSocket сообщение
		if len(message) > 0 {
			a.logger.Debug("[BINANCE_ADAPTER] Received message: %s", string(message))
			unifiedMsg, err := a.parser.ParseMessage("binance", message)
			if err != nil {
				a.logger.Error("[BINANCE_ADAPTER] Parse error: %v", err)
				continue
			}

			if unifiedMsg != nil {
				// Добавляем PairID в сообщение
				var msg market.UnifiedMessage = *unifiedMsg
				if pairID, exists := a.pairIDMap[msg.Symbol]; exists {
					msg.PairID = pairID
					a.logger.Debug("[BINANCE_ADAPTER] Publishing message for pair %s (ID: %d)", msg.Symbol, pairID)
				} else {
					a.logger.Debug("[BINANCE_ADAPTER] Publishing message for pair %s (no ID mapping)", msg.Symbol)
				}
				a.messageBus.Publish("binance", msg)
			}
		}
	}
}

func (a *BinanceAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	return nil
}

func (a *BinanceAdapter) IsActive() bool {
	return a.active && (a.ws == nil || a.ws.IsConnected())
}

func (a *BinanceAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *BinanceAdapter) SubscribeMarketsWithPairID(pairs []MarketPair, marketType string, depth int) error {
	// Сохраняем маппинг symbol -> pairID
	for _, pair := range pairs {
		a.pairIDMap[pair.Symbol] = pair.PairID
	}

	// Извлекаем только символы для обычной подписки
	symbols := make([]string, len(pairs))
	for i, pair := range pairs {
		symbols[i] = pair.Symbol
	}

	return a.SubscribeMarkets(symbols, marketType, depth)
}

// UnsubscribeMarketsWithPairID отписывается от рынков
func (a *BinanceAdapter) UnsubscribeMarketsWithPairID(pairs []MarketPair, marketType string, depth int) error {
	// Удаляем маппинг
	for _, pair := range pairs {
		delete(a.pairIDMap, pair.Symbol)
	}

	// Извлекаем только символы для обычной отписки
	symbols := make([]string, len(pairs))
	for i, pair := range pairs {
		symbols[i] = pair.Symbol
	}

	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
