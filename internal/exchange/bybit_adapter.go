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

// BybitAdapter реализует Adapter для биржи Bybit
type BybitAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.BybitParser
	messageBus     *bus.MessageBus
	pairIDMap      map[string]int // symbol -> pairID маппинг
}

// SubscribeMarkets для Bybit с подробным логированием
func (a *BybitAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[BYBIT_ADAPTER] SubscribeMarkets started: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)

	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[BYBIT_ADAPTER] WebSocket not connected")
		return fmt.Errorf("BybitAdapter: ws not connected")
	}

	a.logger.Debug("[BYBIT_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(symbol, "-", "")
		a.logger.Debug("[BYBIT_ADAPTER] Processing pair: %s -> symbol: %s", pair, symbol)

		// Подписка на orderbook для Bybit - используем формат orderbook.{depth}.{symbol}
		subOrderbook := map[string]interface{}{
			"op":   "subscribe",
			"args": []string{fmt.Sprintf("orderbook.%d.%s", depth, symbol)},
		}
		dataOrderbook, err := json.Marshal(subOrderbook)
		if err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to marshal orderbook subscription: %v", err)
			return fmt.Errorf("BybitAdapter: marshal orderbook sub: %w", err)
		}

		a.logger.Debug("[BYBIT_ADAPTER] Sending orderbook subscription: %s", string(dataOrderbook))
		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to send orderbook subscription: %v", err)
			return fmt.Errorf("BybitAdapter: ws orderbook sub: %w", err)
		}

		// Подписка на ticker для Bybit - используем формат tickers.{symbol}
		subTicker := map[string]interface{}{
			"op":   "subscribe",
			"args": []string{fmt.Sprintf("tickers.%s", symbol)},
		}
		dataTicker, err := json.Marshal(subTicker)
		if err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to marshal ticker subscription: %v", err)
			return fmt.Errorf("BybitAdapter: marshal ticker sub: %w", err)
		}

		a.logger.Debug("[BYBIT_ADAPTER] Sending ticker subscription: %s", string(dataTicker))
		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to send ticker subscription: %v", err)
			return fmt.Errorf("BybitAdapter: ws ticker sub: %w", err)
		}

		a.logger.Info("[BYBIT_ADAPTER] Successfully subscribed to %s (orderbook + ticker)", symbol)
	}

	a.logger.Info("[BYBIT_ADAPTER] All subscriptions completed successfully")
	return nil
}

// NewBybitAdapter создает BybitAdapter на основе данных из db.Exchange
func NewBybitAdapter(ex db.Exchange) *BybitAdapter {
	var wsClient *CexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewCexWsClient(ex.WsUrl.String)
	}

	logger := log.New("bybit_adapter")
	return &BybitAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         wsClient,
		active:     false,
		logger:     logger,
		parser:     parsers.NewBybitParser(),
		messageBus: bus.GetInstance(),
		pairIDMap:  make(map[string]int),
	}
}

// UnsubscribeMarkets реализует отписку от пар для Bybit
func (a *BybitAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[BYBIT_ADAPTER] UnsubscribeMarkets started: pairs=%v", pairs)

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[BYBIT_ADAPTER] WebSocket not connected for unsubscribe")
		return fmt.Errorf("BybitAdapter: ws not connected")
	}

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(symbol, "-", "")
		a.logger.Debug("[BYBIT_ADAPTER] Unsubscribing from pair: %s -> symbol: %s", pair, symbol)

		// Отписка от orderbook для Bybit
		unsubOrderbook := map[string]interface{}{
			"op":   "unsubscribe",
			"args": []string{fmt.Sprintf("orderbook.%d.%s", depth, symbol)},
		}
		dataOrderbook, err := json.Marshal(unsubOrderbook)
		if err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to marshal orderbook unsubscription: %v", err)
			return fmt.Errorf("BybitAdapter: marshal orderbook unsub: %w", err)
		}
		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to send orderbook unsubscription: %v", err)
			return fmt.Errorf("BybitAdapter: ws orderbook unsub: %w", err)
		}

		// Отписка от ticker для Bybit
		unsubTicker := map[string]interface{}{
			"op":   "unsubscribe",
			"args": []string{fmt.Sprintf("tickers.%s", symbol)},
		}
		dataTicker, err := json.Marshal(unsubTicker)
		if err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to marshal ticker unsubscription: %v", err)
			return fmt.Errorf("BybitAdapter: marshal ticker unsub: %w", err)
		}
		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Failed to send ticker unsubscription: %v", err)
			return fmt.Errorf("BybitAdapter: ws ticker unsub: %w", err)
		}

		a.logger.Info("[BYBIT_ADAPTER] Successfully unsubscribed from %s", symbol)
	}

	a.logger.Info("[BYBIT_ADAPTER] All unsubscriptions completed successfully")
	return nil
}

func (a *BybitAdapter) Start() error {
	a.logger.Info("[BYBIT_ADAPTER] Starting adapter...")

	// Ping REST API для проверки доступности
	var result map[string]interface{}
	err := a.rest.GetJSON("/v5/market/time", &result)
	if err != nil {
		a.logger.Error("[BYBIT_ADAPTER] REST API ping failed: %v", err)
		a.active = false
		return fmt.Errorf("BybitAdapter: ping failed: %w", err)
	}
	a.logger.Info("[BYBIT_ADAPTER] REST API ping successful")

	// Подключение к WebSocket
	if a.ws != nil {
		if err := a.ws.Connect(); err != nil {
			a.logger.Error("[BYBIT_ADAPTER] WebSocket connection failed: %v", err)
			a.active = false
			return fmt.Errorf("BybitAdapter: ws connect failed: %w", err)
		}
		a.logger.Info("[BYBIT_ADAPTER] WebSocket connected successfully")

		// Запуск горутин для обработки сообщений
		go a.readLoopWithReconnect()
		go a.ws.WriteLoop(25 * time.Second)
	}

	a.active = true
	a.logger.Info("[BYBIT_ADAPTER] Adapter started successfully")
	return nil
}

func (a *BybitAdapter) readLoopWithReconnect() {
	for {
		if a.ws == nil {
			return
		}

		_, message, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[BYBIT_ADAPTER] Read error: %v, reconnecting...", err)
			for {
				time.Sleep(3 * time.Second)
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[BYBIT_ADAPTER] Reconnected, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[BYBIT_ADAPTER] Resubscribe error: %v", err)
					}
					break
				} else {
					a.logger.Error("[BYBIT_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Логируем сырое сообщение для отладки
		a.logger.Info("[BYBIT_ADAPTER] RAW MESSAGE: %s", string(message))

		// Парсим WebSocket сообщение
		if len(message) > 0 {
			unifiedMsg, err := a.parser.ParseMessage("bybit", message)
			if err != nil {
				a.logger.Error("[BYBIT_ADAPTER] Parse error: %v", err)
				continue
			}

			if unifiedMsg != nil {
				// Добавляем PairID в сообщение
				var msg = *unifiedMsg
				if pairID, exists := a.pairIDMap[msg.Symbol]; exists {
					msg.PairID = pairID
				}

				// Логируем парсированное сообщение для отладки
				a.logger.Info("[BYBIT_ADAPTER] PARSED MESSAGE: Type=%s, Symbol=%s, PairID=%d",
					msg.MessageType, msg.Symbol, msg.PairID)

				a.messageBus.Publish("bybit", msg)
			}
		}
	}
}

func (a *BybitAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	return nil
}

func (a *BybitAdapter) IsActive() bool {
	return a.active && (a.ws == nil || a.ws.IsConnected())
}

func (a *BybitAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *BybitAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	a.logger.Debug("[BYBIT_ADAPTER] SubscribeMarketsWithPairID started with %d pairs", len(marketPairs))

	// Сохраняем маппинг symbol -> pairID
	for _, pair := range marketPairs {
		a.pairIDMap[pair.Symbol] = pair.PairID
		a.logger.Debug("[BYBIT_ADAPTER] Mapped symbol %s to PairID %d", pair.Symbol, pair.PairID)
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
func (a *BybitAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	a.logger.Debug("[BYBIT_ADAPTER] UnsubscribeMarketsWithPairID started with %d pairs", len(marketPairs))

	// Удаляем маппинг
	for _, pair := range marketPairs {
		delete(a.pairIDMap, pair.Symbol)
		a.logger.Debug("[BYBIT_ADAPTER] Removed mapping for symbol %s", pair.Symbol)
	}

	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод отписки
	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
