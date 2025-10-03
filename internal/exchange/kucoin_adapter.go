package exchange

import (
	"daemon-go/internal/bus"
	"daemon-go/internal/config"
	"daemon-go/internal/db"
	"daemon-go/internal/market"
	"daemon-go/internal/market/parsers"
	"daemon-go/pkg/log"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Глобальная переменная для конфигурации OrderBook
var globalOrderBookConfig *config.Config

// SetOrderBookConfig устанавливает глобальную конфигурацию для OrderBook логирования
func SetOrderBookConfig(cfg *config.Config) {
	globalOrderBookConfig = cfg
}

// getOrderBookConfig возвращает конфигурацию OrderBook с дефолтными значениями
func getOrderBookConfig() config.Config {
	if globalOrderBookConfig != nil {
		return *globalOrderBookConfig
	}
	// Дефолтные значения
	return config.Config{
		OrderBook: struct {
			DebugLogRaw bool
			DebugLogMsg bool
		}{
			DebugLogRaw: false,
			DebugLogMsg: false,
		},
	}
}

// SubscribeOrderBookAndBestPrice подписывает на order book (5/10) и best price для указанных пар
func (a *KucoinAdapter) SubscribeOrderBookAndBestPrice(pairs []string, depth int) error {
	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("KucoinAdapter: ws not connected")
	}
	for _, pair := range pairs {
		// Конвертируем символ из формата "ERG/USDT" в "ERG-USDT" для KuCoin
		kucoinPair := strings.Replace(pair, "/", "-", -1)

		// Используем правильные топики KuCoin для спот данных
		var orderbookTopic, tickerTopic string
		if depth == 5 {
			orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
		} else if depth == 20 {
			orderbookTopic = "/spotMarket/level2Depth20:" + kucoinPair
		} else {
			orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
		}
		tickerTopic = "/spotMarket/level1:" + kucoinPair

		subMsg := map[string]interface{}{
			"id":       fmt.Sprintf("sub-%s-%d", kucoinPair, depth),
			"type":     "subscribe",
			"topic":    orderbookTopic,
			"response": true,
		}
		data, err := json.Marshal(subMsg)
		if err != nil {
			return fmt.Errorf("KucoinAdapter: marshal orderbook sub: %w", err)
		}
		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[KUCOIN_ADAPTER] SENDING subscribe to KuCoin: %s", string(data))
		}
		if err := a.ws.WriteMessage(1, data); err != nil {
			return fmt.Errorf("KucoinAdapter: ws orderbook sub: %w", err)
		}
		// Best price (Level 1)
		tickerMsg := map[string]interface{}{
			"id":       fmt.Sprintf("sub-ticker-%s", kucoinPair),
			"type":     "subscribe",
			"topic":    tickerTopic,
			"response": true,
		}
		tickerData, err := json.Marshal(tickerMsg)
		if err != nil {
			return fmt.Errorf("KucoinAdapter: marshal ticker sub: %w", err)
		}
		if getOrderBookConfig().OrderBook.DebugLogRaw {
			a.logger.Debug("[KUCOIN_ADAPTER] SENDING ticker subscribe to KuCoin: %s", string(tickerData))
		}
		if err := a.ws.WriteMessage(1, tickerData); err != nil {
			return fmt.Errorf("KucoinAdapter: ws ticker sub: %w", err)
		}
	}
	return nil
}

// KucoinAdapter реализует Adapter для биржи Kucoin

type KucoinAdapter struct {
	exchange       db.Exchange
	rest           *CexRestClient
	ws             *CexWsClient
	active         bool
	lastPairs      []string
	lastMarketType string
	lastDepth      int
	logger         *log.Logger
	parser         *parsers.KucoinParser
	messageBus     *bus.MessageBus
	pairIDMap      map[string]int // symbol -> pairID маппинг
}

// UnsubscribeMarkets реализует отписку от пар для Kucoin
func (a *KucoinAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	if a.ws == nil || !a.ws.IsConnected() {
		return fmt.Errorf("KucoinAdapter: ws not connected")
	}
	for _, pair := range pairs {
		// Конвертируем символ из формата "ERG/USDT" в "ERG-USDT" для KuCoin
		kucoinPair := strings.Replace(pair, "/", "-", -1)
		if strings.ToLower(marketType) == "spot" {
			// Используем правильные топики KuCoin для спот данных
			var orderbookTopic, tickerTopic string
			if depth == 5 {
				orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
			} else if depth == 20 {
				orderbookTopic = "/spotMarket/level2Depth20:" + kucoinPair
			} else {
				orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
			}
			tickerTopic = "/spotMarket/level1:" + kucoinPair

			unsubMsg := map[string]interface{}{
				"id":       fmt.Sprintf("unsub-%s-%d", kucoinPair, depth),
				"type":     "unsubscribe",
				"topic":    orderbookTopic,
				"response": true,
			}
			data, err := json.Marshal(unsubMsg)
			if err != nil {
				return fmt.Errorf("KucoinAdapter: marshal orderbook unsub: %w", err)
			}
			if getOrderBookConfig().OrderBook.DebugLogRaw {
				a.logger.Debug("[KUCOIN_ADAPTER] SENDING unsubscribe to KuCoin: %s", string(data))
			}
			if err := a.ws.WriteMessage(1, data); err != nil {
				return fmt.Errorf("KucoinAdapter: ws orderbook unsub: %w", err)
			}
			// Best price (Level 1)
			tickerMsg := map[string]interface{}{
				"id":       fmt.Sprintf("unsub-ticker-%s", kucoinPair),
				"type":     "unsubscribe",
				"topic":    tickerTopic,
				"response": true,
			}
			tickerData, err := json.Marshal(tickerMsg)
			if err != nil {
				return fmt.Errorf("KucoinAdapter: marshal ticker unsub: %w", err)
			}
			if getOrderBookConfig().OrderBook.DebugLogRaw {
				a.logger.Debug("[KUCOIN_ADAPTER] SENDING unsubscribe to KuCoin: %s", string(tickerData))
			}
			if err := a.ws.WriteMessage(1, tickerData); err != nil {
				return fmt.Errorf("KucoinAdapter: ws ticker unsub: %w", err)
			}
		} else {
			return fmt.Errorf("KucoinAdapter: marketType %s not supported", marketType)
		}
	}
	return nil
}

// SubscribeMarkets реализует универсальную подписку для Kucoin
func (a *KucoinAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	a.logger.Debug("[KUCOIN_ADAPTER] SubscribeMarkets called: pairs=%v, marketType=%s, depth=%d", pairs, marketType, depth)
	a.lastPairs = pairs
	a.lastMarketType = marketType
	a.lastDepth = depth

	if a.ws == nil {
		a.logger.Error("[KUCOIN_ADAPTER] WebSocket is nil")
		return fmt.Errorf("KucoinAdapter: ws is nil")
	}

	if !a.ws.IsConnected() {
		a.logger.Error("[KUCOIN_ADAPTER] WebSocket not connected")
		return fmt.Errorf("KucoinAdapter: ws not connected")
	}

	a.logger.Debug("[KUCOIN_ADAPTER] WebSocket is connected, proceeding with subscriptions")

	for _, pair := range pairs {
		a.logger.Debug("[KUCOIN_ADAPTER] Processing subscription for pair: %s", pair)
		// Для Kucoin только spot поддерживается, но можно расширить
		if strings.ToLower(marketType) == "spot" {
			// Конвертируем символ из формата "ERG/USDT" в "ERG-USDT" для KuCoin
			kucoinPair := strings.Replace(pair, "/", "-", -1)
			a.logger.Debug("[KUCOIN_ADAPTER] Converting symbol %s to KuCoin format: %s", pair, kucoinPair)

			// Используем правильные топики KuCoin для спот данных
			var orderbookTopic, tickerTopic string
			if depth == 5 {
				orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
			} else if depth == 20 {
				orderbookTopic = "/spotMarket/level2Depth20:" + kucoinPair
			} else {
				// По умолчанию depth 5
				orderbookTopic = "/spotMarket/level2Depth5:" + kucoinPair
			}
			tickerTopic = "/spotMarket/level1:" + kucoinPair

			a.logger.Debug("[KUCOIN_ADAPTER] Orderbook topic for %s: %s", pair, orderbookTopic)
			subMsg := map[string]interface{}{
				"id":       fmt.Sprintf("sub-%s-%d", kucoinPair, depth),
				"type":     "subscribe",
				"topic":    orderbookTopic,
				"response": true,
			}
			data, err := json.Marshal(subMsg)
			if err != nil {
				a.logger.Error("[KUCOIN_ADAPTER] Failed to marshal orderbook subscription: %v", err)
				return fmt.Errorf("KucoinAdapter: marshal orderbook sub: %w", err)
			}
			if getOrderBookConfig().OrderBook.DebugLogRaw {
				a.logger.Debug("[KUCOIN_ADAPTER] SENDING orderbook subscribe to KuCoin: %s", string(data))
			}
			if err := a.ws.WriteMessage(1, data); err != nil {
				a.logger.Error("[KUCOIN_ADAPTER] Failed to send orderbook subscription: %v", err)
				return fmt.Errorf("KucoinAdapter: ws orderbook sub: %w", err)
			}
			a.logger.Debug("[KUCOIN_ADAPTER] Orderbook subscription sent successfully for pair %s", pair)

			// Best price (Level 1 data)
			a.logger.Debug("[KUCOIN_ADAPTER] Ticker topic for %s: %s", pair, tickerTopic)
			tickerMsg := map[string]interface{}{
				"id":       fmt.Sprintf("sub-ticker-%s", kucoinPair),
				"type":     "subscribe",
				"topic":    tickerTopic,
				"response": true,
			}
			tickerData, err := json.Marshal(tickerMsg)
			if err != nil {
				a.logger.Error("[KUCOIN_ADAPTER] Failed to marshal ticker subscription: %v", err)
				return fmt.Errorf("KucoinAdapter: marshal ticker sub: %w", err)
			}
			if getOrderBookConfig().OrderBook.DebugLogRaw {
				a.logger.Debug("[KUCOIN_ADAPTER] SENDING ticker subscribe to KuCoin: %s", string(tickerData))
			}
			if err := a.ws.WriteMessage(1, tickerData); err != nil {
				a.logger.Error("[KUCOIN_ADAPTER] Failed to send ticker subscription: %v", err)
				return fmt.Errorf("KucoinAdapter: ws ticker sub: %w", err)
			}
			a.logger.Debug("[KUCOIN_ADAPTER] Ticker subscription sent successfully for pair %s", pair)
		} else {
			// TODO: добавить поддержку futures, если появится у Kucoin
			a.logger.Error("[KUCOIN_ADAPTER] Unsupported market type: %s", marketType)
			return fmt.Errorf("KucoinAdapter: marketType %s not supported", marketType)
		}
	}
	a.logger.Info("[KUCOIN_ADAPTER] All subscriptions completed successfully")
	return nil
}

// NewKucoinAdapter создает KucoinAdapter на основе данных из db.Exchange
func NewKucoinAdapter(ex db.Exchange) *KucoinAdapter {
	return &KucoinAdapter{
		exchange:   ex,
		rest:       NewCexRestClient(ex.BaseUrl),
		ws:         nil, // ws будет инициализирован динамически
		active:     false,
		logger:     log.New("kucoin_adapter"),
		parser:     parsers.NewKucoinParser(),
		messageBus: bus.GetInstance(),
		pairIDMap:  make(map[string]int),
	}
}

// getWsUrlAndTokenKucoin — получает WS URL и токен через REST
func getWsUrlAndTokenKucoin(rest *CexRestClient) (string, string, error) {
	// Log the base URL for debugging
	fmt.Printf("[DEBUG] KuCoin WebSocket token fetch starting with BaseURL: %s\n", rest.BaseURL)

	// Попробуем более полную структуру ответа согласно документации KuCoin
	var resp struct {
		Code string `json:"code"`
		Data struct {
			Token           string `json:"token"`
			InstanceServers []struct {
				Endpoint     string `json:"endpoint"`
				Encrypt      bool   `json:"encrypt"`
				Protocol     string `json:"protocol"`
				PingInterval int    `json:"pingInterval"`
				PingTimeout  int    `json:"pingTimeout"`
			} `json:"instanceServers"`
			WsUrl      string `json:"endpoint"`   // Fallback поле
			InstanceId string `json:"instanceId"` // Может присутствовать
		} `json:"data"`
		Message string `json:"msg"`
	}

	// Debug: логируем URL и параметры запроса
	fmt.Printf("[DEBUG] KuCoin WS token request URL: %s/api/v1/bullet-public\n", rest.BaseURL)

	// Kucoin требует пустой JSON-объект {} как тело запроса
	body := map[string]interface{}{}
	bodyJson, _ := json.Marshal(body)
	fmt.Printf("[DEBUG] KuCoin WS token request body: %s\n", string(bodyJson))

	fmt.Printf("[DEBUG] Making POST request to KuCoin API...\n")
	err := rest.PostJSON("/api/v1/bullet-public", body, &resp)
	if err != nil {
		fmt.Printf("[DEBUG] KuCoin WS token fetch error: %v\n", err)
		return "", "", fmt.Errorf("kucoin ws token fetch error: %w", err)
	}

	// Добавим также полный JSON ответ для анализа
	respJson, _ := json.Marshal(resp)
	fmt.Printf("[DEBUG] KuCoin WS full response JSON: %s\n", string(respJson))

	// Проверяем код ответа
	if resp.Code != "200000" {
		fmt.Printf("[DEBUG] KuCoin API returned error code: %s, message: %s\n", resp.Code, resp.Message)
		return "", "", fmt.Errorf("kucoin api error: code=%s, msg=%s", resp.Code, resp.Message)
	}

	// Проверяем токен
	if resp.Data.Token == "" {
		fmt.Printf("[DEBUG] KuCoin WS token is empty\n")
		return "", "", fmt.Errorf("kucoin ws token is empty")
	}

	// Пытаемся получить WebSocket URL из разных полей
	var wsUrl string

	// Сначала проверяем новую структуру instanceServers
	if len(resp.Data.InstanceServers) > 0 {
		wsUrl = resp.Data.InstanceServers[0].Endpoint
		fmt.Printf("[DEBUG] Got WS URL from instanceServers: %s\n", wsUrl)
	} else if resp.Data.WsUrl != "" {
		// Fallback на старое поле endpoint
		wsUrl = resp.Data.WsUrl
		fmt.Printf("[DEBUG] Got WS URL from direct endpoint field: %s\n", wsUrl)
	}

	if wsUrl == "" {
		fmt.Printf("[DEBUG] No WebSocket URL found in response\n")
		return "", "", fmt.Errorf("kucoin ws url not found in response")
	}

	fmt.Printf("[DEBUG] KuCoin WS token fetch successful: URL=%s, Token=%s\n", wsUrl, resp.Data.Token)
	return wsUrl, resp.Data.Token, nil
}

func (a *KucoinAdapter) Start() error {
	a.logger.Info("[KUCOIN_ADAPTER] Starting KuCoin adapter")
	// REST ping
	a.logger.Debug("[KUCOIN_ADAPTER] Performing REST API ping")
	var result map[string]interface{}
	err := a.rest.GetJSON("/timestamp", &result)
	if err != nil {
		a.active = false
		a.logger.Error("[KUCOIN_ADAPTER] REST ping failed: %v", err)
		return fmt.Errorf("KucoinAdapter: ping failed: %w", err)
	}
	a.logger.Debug("[KUCOIN_ADAPTER] REST ping successful: %+v", result)

	// Получить WS URL и токен
	a.logger.Debug("[KUCOIN_ADAPTER] Fetching WebSocket URL and token")
	wsUrl, token, err := getWsUrlAndTokenKucoin(a.rest)
	if err != nil {
		a.active = false
		a.logger.Error("[KUCOIN_ADAPTER] Failed to fetch WS URL/token: %v", err)
		return fmt.Errorf("KucoinAdapter: ws url/token fetch failed: %w", err)
	}
	a.logger.Debug("[KUCOIN_ADAPTER] WS URL and token obtained: wsUrl=%s, token=%s", wsUrl, token)

	// Инициализировать WS-клиент
	fullWsUrl := wsUrl + "?token=" + token
	a.logger.Debug("[KUCOIN_ADAPTER] Initializing WebSocket client with URL: %s", fullWsUrl)
	a.ws = NewCexWsClient(fullWsUrl)
	if a.ws != nil {
		a.logger.Debug("[KUCOIN_ADAPTER] Connecting to WebSocket")
		if err := a.ws.Connect(); err != nil {
			a.active = false
			a.logger.Error("[KUCOIN_ADAPTER] WebSocket connection failed: %v", err)
			return fmt.Errorf("KucoinAdapter: ws connect failed: %w", err)
		}
		a.logger.Info("[KUCOIN_ADAPTER] WebSocket connected successfully")
		go a.readLoopWithReconnect()
		a.logger.Debug("[KUCOIN_ADAPTER] Started custom readLoop goroutine")
	}
	a.active = true
	a.logger.Info("[KUCOIN_ADAPTER] KuCoin adapter started successfully")
	return nil
}

func (a *KucoinAdapter) readLoopWithReconnect() {
	a.logger.Debug("[KUCOIN_ADAPTER] Starting readLoop")
	for {
		if a.ws == nil || !a.active {
			a.logger.Debug("[KUCOIN_ADAPTER] WebSocket is nil or adapter inactive, exiting readLoop")
			return
		}

		_, message, err := a.ws.ReadMessage()
		if err != nil {
			a.logger.Error("[KUCOIN_ADAPTER] Read error: %v, reconnecting...", err)
			for {
				if !a.active {
					a.logger.Debug("[KUCOIN_ADAPTER] Adapter inactive during reconnect, exiting")
					return
				}
				time.Sleep(3 * time.Second)
				if err := a.ws.Reconnect(); err == nil {
					a.logger.Info("[KUCOIN_ADAPTER] Reconnected, resubscribing...")
					if err := a.SubscribeMarkets(a.lastPairs, a.lastMarketType, a.lastDepth); err != nil {
						a.logger.Error("[KUCOIN_ADAPTER] Resubscribe error: %v", err)
					} else {
						a.logger.Info("[KUCOIN_ADAPTER] Resubscribed successfully")
					}
					break
				} else {
					a.logger.Error("[KUCOIN_ADAPTER] Reconnect failed: %v, retrying...", err)
				}
			}
			continue
		}

		// Парсим WebSocket сообщение
		if len(message) > 0 {
			if getOrderBookConfig().OrderBook.DebugLogRaw {
				a.logger.Debug("[KUCOIN_ADAPTER] RECEIVED from KuCoin: %s", string(message))
			}
			unifiedMsg, err := a.parser.ParseMessage("kucoin", message)
			if err != nil {
				a.logger.Error("[KUCOIN_ADAPTER] Parse error: %v", err)
				continue
			}

			// Control messages return nil - skip them
			if unifiedMsg != nil {
				// Добавляем PairID в сообщение
				var msg market.UnifiedMessage = *unifiedMsg
				if pairID, exists := a.pairIDMap[msg.Symbol]; exists {
					msg.PairID = pairID
					a.logger.Debug("[KUCOIN_ADAPTER] Added PairID %d for symbol %s", pairID, msg.Symbol)
				} else {
					a.logger.Warn("[KUCOIN_ADAPTER] No PairID found for symbol %s", msg.Symbol)
				}

				if getOrderBookConfig().OrderBook.DebugLogMsg {
					if msgJSON, err := json.Marshal(msg); err == nil {
						a.logger.Debug("[KUCOIN_ADAPTER] Publishing unified message: %s", string(msgJSON))
					} else {
						a.logger.Debug("[KUCOIN_ADAPTER] Publishing unified message: %+v", msg)
					}
				}
				a.messageBus.Publish("kucoin", msg)
				a.logger.Debug("[KUCOIN_ADAPTER] Message published to message bus")
			} else {
				a.logger.Debug("[KUCOIN_ADAPTER] Parsed message is nil (probably ack/pong)")
			}
		}
	}
}

func (a *KucoinAdapter) Stop() error {
	a.active = false
	if a.ws != nil {
		_ = a.ws.Close()
	}
	return nil
}

func (a *KucoinAdapter) IsActive() bool {
	active := a.active && (a.ws == nil || a.ws.IsConnected())
	a.logger.Debug("[KUCOIN_ADAPTER] IsActive called: a.active=%v, ws=%v, wsConnected=%v, result=%v",
		a.active, a.ws != nil, a.ws != nil && a.ws.IsConnected(), active)
	return active
}

func (a *KucoinAdapter) ExchangeName() string {
	return a.exchange.Name
}

// SubscribeMarketsWithPairID подписывается на рынки с сохранением PairID
func (a *KucoinAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	a.logger.Debug("[KUCOIN_ADAPTER] SubscribeMarketsWithPairID called with %d pairs, marketType=%s, depth=%d", len(marketPairs), marketType, depth)

	// Обновляем pairIDMap
	for _, mp := range marketPairs {
		a.pairIDMap[mp.Symbol] = mp.PairID
		a.logger.Debug("[KUCOIN_ADAPTER] Updated pairIDMap: %s -> PairID=%d", mp.Symbol, mp.PairID)
	}

	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
		a.logger.Debug("[KUCOIN_ADAPTER] Will subscribe to symbol: %s (PairID=%d)", mp.Symbol, mp.PairID)
	}

	a.logger.Debug("[KUCOIN_ADAPTER] Calling SubscribeMarkets with symbols: %v", symbols)
	// Используем существующий метод подписки
	return a.SubscribeMarkets(symbols, marketType, depth)
}

// UnsubscribeMarketsWithPairID отписывается от рынков
func (a *KucoinAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Извлекаем символы из MarketPair для совместимости с существующим методом
	symbols := make([]string, len(marketPairs))
	for i, mp := range marketPairs {
		symbols[i] = mp.Symbol
	}

	// Используем существующий метод отписки
	return a.UnsubscribeMarkets(symbols, marketType, depth)
}
