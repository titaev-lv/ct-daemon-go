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

// CoinexAdapter реализует Adapter для биржи CoinEx
type CoinexAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.CoinexParser
	messageBus     *bus.MessageBus
}

// CoinexAdapter реализует интерфейс Adapter
func (a *CoinexAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	fmt.Printf("[DEBUG] SubscribeMarkets called with pairs=%v\n", pairs)

	fmt.Printf("[DEBUG] About to call logger.Debug\n")
	a.logger.Debug("[COINEX_ADAPTER] SubscribeMarkets started: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)

	fmt.Printf("[DEBUG] Setting last pairs\n")
	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	fmt.Printf("[DEBUG] Checking WebSocket connection\n")
	if a.ws == nil || !a.ws.IsConnected() {
		fmt.Printf("[DEBUG] WebSocket not connected\n")
		a.logger.Error("[COINEX_ADAPTER] WebSocket not connected")
		return fmt.Errorf("CoinexAdapter: ws not connected")
	}

	fmt.Printf("[DEBUG] WebSocket is connected, proceeding\n")
	a.logger.Debug("[COINEX_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	for _, pair := range pairs {
		symbol := strings.ReplaceAll(pair, " ", "")
		a.logger.Debug("[COINEX_ADAPTER] Processing pair: %s -> symbol: %s", pair, symbol)

		// Подписка на order book
		subOrderbook := map[string]interface{}{
			"method": "depth.subscribe",
			"params": []interface{}{symbol, depth, "0"},
			"id":     1,
		}

		dataOrderbook, err := json.Marshal(subOrderbook)
		if err != nil {
			return fmt.Errorf("CoinexAdapter: marshal orderbook sub: %w", err)
		}

		a.logger.Debug("[COINEX_ADAPTER] Sending orderbook subscription for %s", symbol)

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[COINEX_ADAPTER] SENDING orderbook subscribe to CoinEx: %s", string(dataOrderbook))
		}

		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			return fmt.Errorf("CoinexAdapter: ws orderbook sub: %w", err)
		}

		a.logger.Debug("[COINEX_ADAPTER] Orderbook subscription sent successfully for pair %s", pair)

		// Подписка на best price (ticker)
		subTicker := map[string]interface{}{
			"method": "state.subscribe",
			"params": []interface{}{symbol},
			"id":     2,
		}

		dataTicker, err := json.Marshal(subTicker)
		if err != nil {
			return fmt.Errorf("CoinexAdapter: marshal ticker sub: %w", err)
		}

		a.logger.Debug("[COINEX_ADAPTER] Sending ticker subscription for %s", symbol)

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[COINEX_ADAPTER] SENDING ticker subscribe to CoinEx: %s", string(dataTicker))
		}

		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			return fmt.Errorf("CoinexAdapter: ws ticker sub: %w", err)
		}

		a.logger.Debug("[COINEX_ADAPTER] Ticker subscription sent successfully for pair %s", pair)
	}

	a.logger.Debug("[COINEX_ADAPTER] SubscribeMarkets completed successfully")
	return nil
}

// UnsubscribeMarkets реализует отписку от пар для CoinEx
func (a *CoinexAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("CoinexAdapter: ws not connected")
	}

	for _, pair := range pairs {
		symbol := strings.ReplaceAll(pair, " ", "")

		// Отписка от order book
		unsubOrderbook := map[string]interface{}{
			"method": "depth.unsubscribe",
			"params": []interface{}{symbol, depth, "0"},
			"id":     3,
		}

		dataOrderbook, err := json.Marshal(unsubOrderbook)
		if err != nil {
			return fmt.Errorf("CoinexAdapter: marshal orderbook unsub: %w", err)
		}

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[COINEX_ADAPTER] SENDING unsubscribe to CoinEx: %s", string(dataOrderbook))
		}

		if err := a.ws.WriteMessage(1, dataOrderbook); err != nil {
			return fmt.Errorf("CoinexAdapter: ws orderbook unsub: %w", err)
		}

		// Отписка от ticker
		unsubTicker := map[string]interface{}{
			"method": "state.unsubscribe",
			"params": []interface{}{symbol},
			"id":     4,
		}

		dataTicker, err := json.Marshal(unsubTicker)
		if err != nil {
			return fmt.Errorf("CoinexAdapter: marshal ticker unsub: %w", err)
		}

		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[COINEX_ADAPTER] SENDING unsubscribe to CoinEx: %s", string(dataTicker))
		}

		if err := a.ws.WriteMessage(1, dataTicker); err != nil {
			return fmt.Errorf("CoinexAdapter: ws ticker unsub: %w", err)
		}
	}

	return nil
}

// NewCoinexAdapter создает CoinexAdapter на основе данных из db.Exchange
func NewCoinexAdapter(ex db.Exchange) *CoinexAdapter {
	var wsClient *CexWsClient
	if ex.WsUrl.Valid && ex.WsUrl.String != "" {
		wsClient = NewCexWsClient(ex.WsUrl.String)
	}

	return &CoinexAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         wsClient,
		active:     false,
		logger:     log.New("coinex_adapter"),
		parser:     parsers.NewCoinexParser(),
		messageBus: bus.GetInstance(),
	}
}

func (a *CoinexAdapter) Start() error {
	// Тестируем REST API - используем правильный endpoint для CoinEx
	var result map[string]interface{}
	err := a.rest.GetJSON("/v1/market/list", &result)
	if err != nil {
		a.active = false
		return fmt.Errorf("CoinexAdapter: ping failed: %w", err)
	}

	a.logger.Info("[COINEX_ADAPTER] REST API ping successful")

	// Подключаем WebSocket
	if a.ws != nil {
		if err := a.ws.Connect(); err != nil {
			a.active = false
			return fmt.Errorf("CoinexAdapter: ws connect failed: %w", err)
		}

		a.logger.Info("[COINEX_ADAPTER] WebSocket connected successfully")

		// Запускаем циклы чтения и записи
		go a.readLoopWithReconnect()
		go a.ws.WriteLoop(25 * time.Second)
	}

	a.active = true
	a.logger.Info("[COINEX_ADAPTER] Adapter started successfully")
	return nil
}

func (a *CoinexAdapter) readLoopWithReconnect() {
	for {
		if a.ws == nil || !a.active {
			a.logger.Debug("[COINEX_ADAPTER] WebSocket is nil or adapter inactive, exiting readLoop")
			return
		}

		_, msgData, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[COINEX_ADAPTER] Read error: %v, reconnecting...", err)

			// Переподключение с ретраями
			for {
				if !a.active {
					a.logger.Debug("[COINEX_ADAPTER] Adapter inactive during reconnect, exiting")
					return
				}
				time.Sleep(3 * time.Second)
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[COINEX_ADAPTER] Reconnected, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[COINEX_ADAPTER] Resubscribe error: %v", err)
					}
					break
				} else {
					a.logger.Error("[COINEX_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Логируем полученное сообщение
		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[COINEX_ADAPTER] RECEIVED from CoinEx: %s", string(msgData))
		}

		// Обрабатываем сообщение
		a.processMessage(msgData)
	}
}

func (a *CoinexAdapter) processMessage(data []byte) {
	// Всегда логируем сырое сообщение для отладки
	a.logger.Info("[COINEX_ADAPTER] RAW MESSAGE: %s", string(data))

	// Парсим сообщение через парсер
	unifiedMsg, err := a.parser.ParseMessage("coinex", data)
	if err != nil {
		a.logger.Warn("[COINEX_ADAPTER] Failed to parse message: %v", err)
		return
	}

	if unifiedMsg == nil {
		return
	}

	// Логируем unified сообщение в JSON формате
	if getOrderBookConfig().OrderBook.DebugLogMsg {
		if jsonData, err := json.Marshal(unifiedMsg); err == nil {
			a.logger.Debug("[COINEX_ADAPTER] Publishing unified message: %s", string(jsonData))
		}
	}

	// Публикуем в message bus (передаем по значению, а не по указателю)
	a.messageBus.Publish("coinex", *unifiedMsg)
	a.logger.Debug("[COINEX_ADAPTER] Message published to message bus")
}

func (a *CoinexAdapter) Stop() error {
	a.logger.Info("[COINEX_ADAPTER] Stopping adapter...")
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	a.logger.Info("[COINEX_ADAPTER] Adapter stopped")
	return nil
}

func (a *CoinexAdapter) IsActive() bool {
	return a.active && (a.ws == nil || a.ws.IsConnected())
}

func (a *CoinexAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *CoinexAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод подписки
	return a.SubscribeMarkets(symbols, marketType, depth)
}

// UnsubscribeMarketsWithPairID отписывается от рынков
func (a *CoinexAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод отписки
	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
