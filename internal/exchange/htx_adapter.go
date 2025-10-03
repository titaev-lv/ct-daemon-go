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

// HtxAdapter реализует Adapter для биржи HTX
type HtxAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.HTXParser
	messageBus     *bus.MessageBus
}

// SubscribeMarkets для HTX с подробным логированием
func (a *HtxAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[HTX_ADAPTER] SubscribeMarkets started: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)

	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil || !a.ws.IsConnected() {
		a.logger.Error("[HTX_ADAPTER] WebSocket not connected")
		return fmt.Errorf("HtxAdapter: ws not connected")
	}

	a.logger.Debug("[HTX_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")
		a.logger.Debug("[HTX_ADAPTER] Processing pair: %s -> symbol: %s", pair, symbol)

		// Подписка на order book для HTX
		subOrderbook := map[string]interface{}{
			"sub": fmt.Sprintf("market.%s.depth.step%d", symbol, depth),
			"id":  fmt.Sprintf("sub-%s-%d", symbol, depth),
		}

		dataOrderbook, err := json.Marshal(subOrderbook)
		if err != nil {
			return fmt.Errorf("HtxAdapter: marshal orderbook sub: %w", err)
		}

		a.logger.Debug("[HTX_ADAPTER] Sending orderbook subscription for %s", symbol)

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[HTX_ADAPTER] SENDING orderbook subscribe to HTX: %s", string(dataOrderbook))
		}

		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			return fmt.Errorf("HtxAdapter: ws orderbook sub: %w", err)
		}

		a.logger.Debug("[HTX_ADAPTER] Orderbook subscription sent successfully for pair %s", pair)

		// Подписка на ticker для HTX
		subTicker := map[string]interface{}{
			"sub": fmt.Sprintf("market.%s.ticker", symbol),
			"id":  fmt.Sprintf("sub-ticker-%s", symbol),
		}

		dataTicker, err := json.Marshal(subTicker)
		if err != nil {
			return fmt.Errorf("HtxAdapter: marshal ticker sub: %w", err)
		}

		a.logger.Debug("[HTX_ADAPTER] Sending ticker subscription for %s", symbol)

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[HTX_ADAPTER] SENDING ticker subscribe to HTX: %s", string(dataTicker))
		}

		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			return fmt.Errorf("HtxAdapter: ws ticker sub: %w", err)
		}

		a.logger.Debug("[HTX_ADAPTER] Ticker subscription sent successfully for pair %s", pair)
	}

	a.logger.Debug("[HTX_ADAPTER] SubscribeMarkets completed successfully")
	return nil
}

// NewHtxAdapter создает HtxAdapter на основе данных из db.Exchange
func NewHtxAdapter(ex db.Exchange) *HtxAdapter {
	var wsClient *CexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewCexWsClient(ex.WsUrl.String)
	}

	return &HtxAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         wsClient,
		active:     false,
		logger:     log.New("htx_adapter"),
		parser:     parsers.NewHTXParser(),
		messageBus: bus.GetInstance(),
	}
}
func (a *HtxAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("HtxAdapter: ws not connected")
	}

	for _, pair := range pairs {
		symbol := pair
		symbol = strings.ReplaceAll(strings.ToLower(symbol), "-", "")

		// Отписка от order book
		unsubOrderbook := map[string]interface{}{
			"unsub": fmt.Sprintf("market.%s.depth.step%d", symbol, depth),
			"id":    fmt.Sprintf("unsub-%s-%d", symbol, depth),
		}

		dataOrderbook, err := json.Marshal(unsubOrderbook)
		if err != nil {
			return fmt.Errorf("HtxAdapter: marshal orderbook unsub: %w", err)
		}

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[HTX_ADAPTER] SENDING unsubscribe from HTX: %s", string(dataOrderbook))
		}

		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			return fmt.Errorf("HtxAdapter: ws orderbook unsub: %w", err)
		}

		// Отписка от ticker
		unsubTicker := map[string]interface{}{
			"unsub": fmt.Sprintf("market.%s.ticker", symbol),
			"id":    fmt.Sprintf("unsub-ticker-%s", symbol),
		}

		dataTicker, err := json.Marshal(unsubTicker)
		if err != nil {
			return fmt.Errorf("HtxAdapter: marshal ticker unsub: %w", err)
		}

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[HTX_ADAPTER] SENDING unsubscribe from HTX: %s", string(dataTicker))
		}

		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			return fmt.Errorf("HtxAdapter: ws ticker unsub: %w", err)
		}
	}

	return nil
}

func (a *HtxAdapter) Start() error {
	// Тестируем REST API для HTX
	var result map[string]interface{}
	err := a.rest.GetJSON("/v1/common/timestamp", &result)
	if err != nil {
		a.active = false
		return fmt.Errorf("HtxAdapter: ping failed: %w", err)
	}

	a.logger.Info("[HTX_ADAPTER] REST API ping successful")

	// Подключаем WebSocket
	if a.ws != nil {
		if err := a.ws.Connect(); err != nil {
			a.active = false
			return fmt.Errorf("HtxAdapter: ws connect failed: %w", err)
		}

		a.logger.Info("[HTX_ADAPTER] WebSocket connected successfully")

		// Запускаем только цикл чтения (HTX сама шлет ping'и, поэтому WriteLoop НЕ НУЖЕН!)
		go a.readLoopWithReconnect()
		// ВАЖНО: НЕ запускаем WriteLoop для HTX!
	}

	a.active = true
	a.logger.Info("[HTX_ADAPTER] Adapter started successfully")
	return nil
}

// readLoopWithReconnect читает сообщения и восстанавливает подписки при reconnect
func (a *HtxAdapter) readLoopWithReconnect() {
	for {
		if a.ws == nil || !a.active {
			a.logger.Debug("[HTX_ADAPTER] WebSocket is nil or adapter inactive, exiting readLoop")
			return
		}

		_, msgData, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[HTX_ADAPTER] Read error: %v, reconnecting...", err)

			// Переподключение с ретраями
			for {
				if !a.active {
					a.logger.Debug("[HTX_ADAPTER] Adapter inactive during reconnect, exiting")
					return
				}
				time.Sleep(3 * time.Second)
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[HTX_ADAPTER] Reconnected, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[HTX_ADAPTER] Resubscribe error: %v", err)
					}
					break
				} else {
					a.logger.Error("[HTX_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Логируем полученное сообщение
		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[HTX_ADAPTER] RECEIVED from HTX: %s", string(msgData))
		}

		// Обрабатываем сообщение
		a.processMessage(msgData)
	}
}

func (a *HtxAdapter) processMessage(data []byte) {
	// Сначала декомпрессируем данные для правильного логирования
	decompressedData, err := a.parser.DecompressGzip(data)
	if err != nil {
		a.logger.Error("[HTX_ADAPTER] Failed to decompress message: %v", err)
		return
	}

	// Логируем распакованное JSON сообщение для отладки
	a.logger.Info("[HTX_ADAPTER] RAW MESSAGE: %s", string(decompressedData))

	// Сначала проверяем, является ли сообщение ping от HTX
	if a.handlePingPong(data) {
		return // Ping/pong обработан, дальше не продолжаем
	}

	// Парсим сообщение через парсер
	unifiedMsg, err := a.parser.ParseMessage("htx", data)
	if err != nil {
		a.logger.Warn("[HTX_ADAPTER] Failed to parse message: %v", err)
		return
	}

	if unifiedMsg == nil {
		return
	}

	// Логируем обработанное сообщение
	if getOrderBookConfig().OrderBook.DebugLogMsg {
		a.logger.Debug("[HTX_ADAPTER] Processed message: %s %s %s", unifiedMsg.Exchange, unifiedMsg.Symbol, unifiedMsg.MessageType)
	}

	// Отправляем в message bus
	a.messageBus.Publish("htx", *unifiedMsg)
}

// handlePingPong обрабатывает ping/pong сообщения от HTX
func (a *HtxAdapter) handlePingPong(data []byte) bool {
	// Декомпрессируем данные для проверки ping
	decompressedData, err := a.parser.DecompressGzip(data)
	if err != nil {
		return false
	}

	var pingMsg map[string]interface{}
	if err := json.Unmarshal(decompressedData, &pingMsg); err != nil {
		return false
	}

	// Проверяем на ping сообщение HTX
	if pingValue, exists := pingMsg["ping"]; exists {
		a.logger.Debug("[HTX_ADAPTER] Received ping: %v", pingValue)

		// Отправляем pong ответ
		pongMsg := map[string]interface{}{
			"pong": pingValue,
		}

		pongData, err := json.Marshal(pongMsg)
		if err != nil {
			a.logger.Error("[HTX_ADAPTER] Failed to marshal pong: %v", err)
			return true
		}

		if err := a.ws.WriteMessage(1, pongData); err != nil {
			a.logger.Error("[HTX_ADAPTER] Failed to send pong: %v", err)
		} else {
			a.logger.Debug("[HTX_ADAPTER] Sent pong response: %s", string(pongData))
		}

		return true
	}

	// Проверяем на pong сообщение (для полноты)
	if _, exists := pingMsg["pong"]; exists {
		a.logger.Debug("[HTX_ADAPTER] Received pong confirmation")
		return true
	}

	return false
}

func (a *HtxAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	return nil
}

func (a *HtxAdapter) IsActive() bool {
	return a.active && (a.ws == nil || a.ws.IsConnected())
}

func (a *HtxAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *HtxAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод подписки
	return a.SubscribeMarkets(symbols, marketType, depth)
}

// UnsubscribeMarketsWithPairID отписывается от рынков
func (a *HtxAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод отписки
	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
