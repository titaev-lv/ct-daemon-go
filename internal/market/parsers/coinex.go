package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"daemon-go/internal/market"
)

// CoinexParser - парсер сообщений CoinEx
type CoinexParser struct {
	symbolRegistry *market.SymbolRegistry
}

// CoinexWebSocketMessage - общий формат WebSocket сообщений CoinEx
type CoinexWebSocketMessage struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	ID     int         `json:"id,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

// CoinexDepthParams - параметры depth сообщения от CoinEx
type CoinexDepthParams struct {
	Clean bool        `json:"clean"`
	Depth interface{} `json:"depth"`
}

// CoinexDepthData - данные depth от CoinEx
type CoinexDepthData struct {
	Asks   [][]string `json:"asks"`
	Bids   [][]string `json:"bids"`
	Last   string     `json:"last"`
	Time   int64      `json:"time"`
	Symbol string     `json:"symbol,omitempty"`
}

// CoinexTickerParams - параметры ticker сообщения от CoinEx
type CoinexTickerParams []interface{}

func NewCoinexParser() *CoinexParser {
	return &CoinexParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *CoinexParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "coinex"
}

func (p *CoinexParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	var wsMsg CoinexWebSocketMessage
	if err := json.Unmarshal(rawData, &wsMsg); err != nil {
		return nil, fmt.Errorf("failed to parse CoinEx WebSocket message: %w", err)
	}

	timestamp := time.Now()

	// Определяем тип сообщения по методу
	switch wsMsg.Method {
	case "depth.update":
		return p.parseDepthUpdate(wsMsg, timestamp)
	case "state.update":
		return p.parseStateUpdate(wsMsg, timestamp)
	case "server.ping":
		// Пинг сообщения не нужно обрабатывать как unified messages
		return nil, nil
	default:
		// Проверяем, есть ли результат (ответ на запрос)
		if wsMsg.ID != 0 {
			// Это ответ на подписку/отписку, игнорируем
			return nil, nil
		}
		return nil, fmt.Errorf("unknown CoinEx method: %s", wsMsg.Method)
	}
}

func (p *CoinexParser) parseDepthUpdate(wsMsg CoinexWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	paramsArray, ok := wsMsg.Params.([]interface{})
	if !ok || len(paramsArray) < 3 {
		return nil, fmt.Errorf("invalid CoinEx depth update params format")
	}

	// CoinEx формат: [is_full, depth_data, symbol]
	// is_full (bool) - флаг полного обновления (true) или инкрементального (false)
	// depth_data (object) - данные с bids/asks
	// symbol (string) - торговая пара

	isFull, ok := paramsArray[0].(bool)
	if !ok {
		return nil, fmt.Errorf("invalid CoinEx is_full flag")
	}

	depthDataInterface := paramsArray[1]

	symbol, ok := paramsArray[2].(string)
	if !ok {
		return nil, fmt.Errorf("invalid CoinEx symbol in depth update")
	}

	depthDataBytes, err := json.Marshal(depthDataInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CoinEx depth data: %w", err)
	}

	var depthData CoinexDepthData
	if err := json.Unmarshal(depthDataBytes, &depthData); err != nil {
		return nil, fmt.Errorf("failed to parse CoinEx depth data: %w", err)
	}

	// Определяем тип обновления на основе флага is_full
	var updateType market.OrderBookUpdateType
	if isFull {
		updateType = market.OrderBookUpdateTypeSnapshot
	} else {
		updateType = market.OrderBookUpdateTypeIncremental
	}

	// Конвертируем символ CoinEx в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("coinex", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert CoinEx symbol %s: %w", symbol, err)
	}

	bids := make([]market.PriceLevel, 0, len(depthData.Bids))
	for _, bid := range depthData.Bids {
		if len(bid) >= 2 {
			price := parseFloat(bid[0])
			volume := parseFloat(bid[1])
			bids = append(bids, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	asks := make([]market.PriceLevel, 0, len(depthData.Asks))
	for _, ask := range depthData.Asks {
		if len(ask) >= 2 {
			price := parseFloat(ask[0])
			volume := parseFloat(ask[1])
			asks = append(asks, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	orderbook := market.UnifiedOrderBook{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		Bids:          bids,
		Asks:          asks,
		Depth:         len(bids) + len(asks),
		UpdateType:    updateType,
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "coinex",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *CoinexParser) parseStateUpdate(wsMsg CoinexWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	paramsArray, ok := wsMsg.Params.([]interface{})
	if !ok || len(paramsArray) < 1 {
		return nil, fmt.Errorf("invalid CoinEx state update params format")
	}

	// CoinEx формат: [{"BTCUSDT": {"last": "114114", ...}}]
	// params[0] содержит объект с ключами-символами

	dataObject, ok := paramsArray[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid CoinEx state data object format")
	}

	// Берем первый (и обычно единственный) символ из объекта
	var symbol string
	var stateData map[string]interface{}
	for sym, data := range dataObject {
		symbol = sym
		if stateDataTyped, ok := data.(map[string]interface{}); ok {
			stateData = stateDataTyped
		} else {
			return nil, fmt.Errorf("invalid CoinEx state data format for symbol %s", sym)
		}
		break // Берем только первый символ
	}

	if symbol == "" || stateData == nil {
		return nil, fmt.Errorf("no valid symbol data found in CoinEx state update")
	}

	// Конвертируем символ CoinEx в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("coinex", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert CoinEx symbol %s: %w", symbol, err)
	}

	// Извлекаем данные ticker
	var lastPrice, high, low, volume float64
	if v, exists := stateData["last"]; exists {
		if s, ok := v.(string); ok {
			lastPrice = parseFloat(s)
		}
	}
	if v, exists := stateData["high"]; exists {
		if s, ok := v.(string); ok {
			high = parseFloat(s)
		}
	}
	if v, exists := stateData["low"]; exists {
		if s, ok := v.(string); ok {
			low = parseFloat(s)
		}
	}
	if v, exists := stateData["volume"]; exists {
		if s, ok := v.(string); ok {
			volume = parseFloat(s)
		}
	}

	ticker := market.UnifiedTicker{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		LastPrice:     lastPrice,
		Volume24h:     volume,
		High24h:       high,
		Low24h:        low,
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "coinex",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTicker,
		Timestamp:     timestamp,
		Data:          &ticker,
	}, nil
}
