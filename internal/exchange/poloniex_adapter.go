package exchange

import (
	"daemon-go/internal/bus"
	"daemon-go/internal/db"
	"daemon-go/internal/market/parsers"
	"daemon-go/pkg/log"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PoloniexAdapter реализует Adapter для биржи Poloniex
type PoloniexAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.PoloniexParser
	messageBus     *bus.MessageBus
	pairIDMap      map[string]int // symbol -> pairID маппинг
}

// SubscribeMarkets для Poloniex с подробным логированием
func (a *PoloniexAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[POLONIEX_ADAPTER] SubscribeMarkets started: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)

	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[POLONIEX_ADAPTER] WebSocket not connected")
		return fmt.Errorf("PoloniexAdapter: ws not connected")
	}

	a.logger.Debug("[POLONIEX_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		a.logger.Debug("[POLONIEX_ADAPTER] Processing pair: %s -> symbol: %s", pair, symbol)

		// Подписка на orderbook для Poloniex - используем /contractMarket/level2Depth5:{symbol}
		subOrderbook := map[string]interface{}{
			"command": "subscribe",
			"channel": fmt.Sprintf("contractMarket/level2Depth5:%s", symbol),
		}
		dataOrderbook, err := json.Marshal(subOrderbook)
		if err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to marshal orderbook subscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: marshal orderbook sub: %w", err)
		}

		a.logger.Debug("[POLONIEX_ADAPTER] Sending orderbook subscription: %s", string(dataOrderbook))
		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to send orderbook subscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: ws orderbook sub: %w", err)
		}

		// Подписка на ticker для Poloniex - используем /contractMarket/ticker:{symbol}
		subTicker := map[string]interface{}{
			"command": "subscribe",
			"channel": fmt.Sprintf("contractMarket/ticker:%s", symbol),
		}
		dataTicker, err := json.Marshal(subTicker)
		if err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to marshal ticker subscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: marshal ticker sub: %w", err)
		}

		a.logger.Debug("[POLONIEX_ADAPTER] Sending ticker subscription: %s", string(dataTicker))
		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to send ticker subscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: ws ticker sub: %w", err)
		}

		a.logger.Info("[POLONIEX_ADAPTER] Successfully subscribed to %s (orderbook + ticker)", symbol)
	}

	a.logger.Info("[POLONIEX_ADAPTER] All subscriptions completed successfully")
	return nil
}

// UnsubscribeMarkets реализует отписку от пар для Poloniex
func (a *PoloniexAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[POLONIEX_ADAPTER] UnsubscribeMarkets started: pairs=%v", pairs)

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[POLONIEX_ADAPTER] WebSocket not connected for unsubscribe")
		return fmt.Errorf("PoloniexAdapter: ws not connected")
	}

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		a.logger.Debug("[POLONIEX_ADAPTER] Unsubscribing from pair: %s -> symbol: %s", pair, symbol)

		// Отписка от orderbook для Poloniex
		unsubOrderbook := map[string]interface{}{
			"command": "unsubscribe",
			"channel": fmt.Sprintf("contractMarket/level2Depth5:%s", symbol),
		}
		dataOrderbook, err := json.Marshal(unsubOrderbook)
		if err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to marshal orderbook unsubscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: marshal orderbook unsub: %w", err)
		}
		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to send orderbook unsubscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: ws orderbook unsub: %w", err)
		}

		// Отписка от ticker для Poloniex
		unsubTicker := map[string]interface{}{
			"command": "unsubscribe",
			"channel": fmt.Sprintf("contractMarket/ticker:%s", symbol),
		}
		dataTicker, err := json.Marshal(unsubTicker)
		if err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to marshal ticker unsubscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: marshal ticker unsub: %w", err)
		}
		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Failed to send ticker unsubscription: %v", err)
			return fmt.Errorf("PoloniexAdapter: ws ticker unsub: %w", err)
		}

		a.logger.Info("[POLONIEX_ADAPTER] Successfully unsubscribed from %s", symbol)
	}

	a.logger.Info("[POLONIEX_ADAPTER] All unsubscriptions completed successfully")
	return nil
}

// NewPoloniexAdapter создает PoloniexAdapter на основе данных из db.Exchange
func NewPoloniexAdapter(ex db.Exchange) *PoloniexAdapter {
	var wsClient *CexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewCexWsClient(ex.WsUrl.String)
	}

	logger := log.New("poloniex_adapter")
	return &PoloniexAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         wsClient,
		active:     false,
		logger:     logger,
		parser:     parsers.NewPoloniexParser(),
		messageBus: bus.GetInstance(),
		pairIDMap:  make(map[string]int),
	}
}

func (a *PoloniexAdapter) Start() error {
	a.logger.Info("[POLONIEX_ADAPTER] Starting adapter...")

	// Ping REST API для проверки доступности
	var result map[string]interface{}
	err := a.rest.GetJSON("/markets", &result)
	if err != nil {
		a.logger.Error("[POLONIEX_ADAPTER] REST API ping failed: %v", err)
		a.active = false
		return fmt.Errorf("PoloniexAdapter: ping failed: %w", err)
	}
	a.logger.Info("[POLONIEX_ADAPTER] REST API ping successful")

	// Подключение к WebSocket
	if a.ws != nil {
		if err := a.ws.Connect(); err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] WebSocket connection failed: %v", err)
			a.active = false
			return fmt.Errorf("PoloniexAdapter: ws connect failed: %w", err)
		}
		a.logger.Info("[POLONIEX_ADAPTER] WebSocket connected successfully")

		// Запуск горутин для обработки сообщений
		go a.readLoopWithReconnect()
		go a.ws.WriteLoop(25 * time.Second)
	}

	a.active = true
	a.logger.Info("[POLONIEX_ADAPTER] Adapter started successfully")
	return nil
}

func (a *PoloniexAdapter) readLoopWithReconnect() {
	for {
		if a.ws == nil || !a.active {
			a.logger.Debug("[POLONIEX_ADAPTER] WebSocket is nil or adapter inactive, exiting readLoop")
			return
		}

		_, message, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[POLONIEX_ADAPTER] Read error: %v, reconnecting...", err)
			for {
				if !a.active {
					a.logger.Debug("[POLONIEX_ADAPTER] Adapter inactive during reconnect, exiting")
					return
				}
				time.Sleep(3 * time.Second)
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[POLONIEX_ADAPTER] Reconnected, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[POLONIEX_ADAPTER] Resubscribe error: %v", err)
					}
					break
				} else {
					a.logger.Error("[POLONIEX_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Логируем сырое сообщение для отладки
		a.logger.Info("[POLONIEX_ADAPTER] RAW MESSAGE: %s", string(message))

		// Парсим WebSocket сообщение
		if len(message) > 0 {
			unifiedMsg, err := a.parser.ParseMessage("poloniex", message)
			if err != nil {
				a.logger.Error("[POLONIEX_ADAPTER] Parse error: %v", err)
				continue
			}

			if unifiedMsg != nil {
				// Добавляем PairID в сообщение
				var msg = *unifiedMsg
				if pairID, exists := a.pairIDMap[msg.Symbol]; exists {
					msg.PairID = pairID
				}

				// Логируем парсированное сообщение для отладки
				a.logger.Info("[POLONIEX_ADAPTER] PARSED MESSAGE: Type=%s, Symbol=%s, PairID=%d",
					msg.MessageType, msg.Symbol, msg.PairID)

				a.messageBus.Publish("poloniex", msg)
			}
		}
	}
}

func (a *PoloniexAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	return nil
}

func (a *PoloniexAdapter) IsActive() bool {
	return a.active && (a.ws == nil || a.ws.IsConnected())
}

func (a *PoloniexAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *PoloniexAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	a.logger.Debug("[POLONIEX_ADAPTER] SubscribeMarketsWithPairID started with %d pairs", len(marketPairs))

	// Сохраняем маппинг symbol -> pairID
	for _, pair := range marketPairs {
		a.pairIDMap[pair.Symbol] = pair.PairID
		a.logger.Debug("[POLONIEX_ADAPTER] Mapped symbol %s to PairID %d", pair.Symbol, pair.PairID)
	}

	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод подписки
	return a.SubscribeMarkets(symbols, marketType, depth)
}

// UnsubscribeMarketsWithPairID отписывается от рынков
func (a *PoloniexAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	a.logger.Debug("[POLONIEX_ADAPTER] UnsubscribeMarketsWithPairID started with %d pairs", len(marketPairs))

	// Удаляем маппинг
	for _, pair := range marketPairs {
		delete(a.pairIDMap, pair.Symbol)
		a.logger.Debug("[POLONIEX_ADAPTER] Removed mapping for symbol %s", pair.Symbol)
	}

	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод отписки
	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
