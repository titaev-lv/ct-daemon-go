package parsers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daemon-go/internal/market"
)

// KucoinParser - парсер сообщений Kucoin
type KucoinParser struct {
	symbolRegistry *market.SymbolRegistry
}

// KucoinWebSocketMessage - общий формат WebSocket сообщений Kucoin
type KucoinWebSocketMessage struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Topic  string      `json:"topic"`
	UserID string      `json:"userId"`
	Data   interface{} `json:"data"`
	Ts     int64       `json:"ts"`
}

// KucoinOrderBookData - формат orderbook от Kucoin
type KucoinOrderBookData struct {
	Symbol    string     `json:"symbol"`
	Sequence  string     `json:"sequence"`
	Asks      [][]string `json:"asks"`
	Bids      [][]string `json:"bids"`
	Timestamp int64      `json:"timestamp"`
}

// KucoinLevel2DepthData - формат Level2Depth от Kucoin для spotMarket топиков
type KucoinLevel2DepthData struct {
	Asks      [][]string `json:"asks"`
	Bids      [][]string `json:"bids"`
	Timestamp int64      `json:"timestamp"`
}

// KucoinTickerData - формат ticker от Kucoin
type KucoinTickerData struct {
	Symbol      string `json:"symbol"`
	Sequence    string `json:"sequence"`
	Price       string `json:"price"`
	Size        string `json:"size"`
	BestBid     string `json:"bestBid"`
	BestBidSize string `json:"bestBidSize"`
	BestAsk     string `json:"bestAsk"`
	BestAskSize string `json:"bestAskSize"`
	Timestamp   int64  `json:"time"`
}

// KucoinMatch - формат сделки от Kucoin
type KucoinMatch struct {
	Symbol       string `json:"symbol"`
	Sequence     string `json:"sequence"`
	Side         string `json:"side"`
	Size         string `json:"size"`
	Price        string `json:"price"`
	TakerOrderID string `json:"takerOrderId"`
	MakerOrderID string `json:"makerOrderId"`
	TradeID      string `json:"tradeId"`
	Timestamp    int64  `json:"time"`
}

func NewKucoinParser() *KucoinParser {
	return &KucoinParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *KucoinParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "kucoin"
}

func (p *KucoinParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	var wsMsg KucoinWebSocketMessage
	if err := json.Unmarshal(rawData, &wsMsg); err != nil {
		return nil, fmt.Errorf("failed to parse Kucoin WebSocket message: %w", err)
	}

	timestamp := time.Now()
	if wsMsg.Ts > 0 {
		timestamp = time.Unix(wsMsg.Ts/1000, (wsMsg.Ts%1000)*1000000)
	}

	// Определяем тип сообщения по topic
	switch {
	case wsMsg.Type == "welcome" || wsMsg.Type == "ack" || wsMsg.Type == "pong":
		// Control messages - skip silently
		return nil, nil
	case contains(wsMsg.Topic, "/spotMarket/level2Depth"):
		return p.parseOrderBook(wsMsg, timestamp)
	case contains(wsMsg.Topic, "/spotMarket/level1"):
		return p.parseTicker(wsMsg, timestamp)
	case contains(wsMsg.Topic, "/market/level2"):
		return p.parseOrderBook(wsMsg, timestamp)
	case contains(wsMsg.Topic, "/market/ticker"):
		return p.parseTicker(wsMsg, timestamp)
	case contains(wsMsg.Topic, "/market/match"):
		return p.parseMatch(wsMsg, timestamp)
	default:
		return nil, fmt.Errorf("unknown Kucoin topic: %s", wsMsg.Topic)
	}
}

func (p *KucoinParser) parseOrderBook(wsMsg KucoinWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	// Извлекаем символ из топика
	symbol := ""
	if wsMsg.Topic != "" {
		// Парсим символ из топика (например, "/spotMarket/level2Depth5:ERG-USDT" -> "ERG-USDT")
		parts := strings.Split(wsMsg.Topic, ":")
		if len(parts) == 2 {
			symbol = parts[1]
		}
	}

	if symbol == "" {
		return nil, fmt.Errorf("no symbol found in orderbook message, topic: %s", wsMsg.Topic)
	}

	var asks, bids [][]string

	// Пробуем разные форматы данных в зависимости от топика
	if contains(wsMsg.Topic, "/spotMarket/level2Depth") {
		// Новый формат для Level2Depth
		var depthData KucoinLevel2DepthData
		dataBytes, err := json.Marshal(wsMsg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Kucoin level2depth data: %w", err)
		}
		if err := json.Unmarshal(dataBytes, &depthData); err != nil {
			return nil, fmt.Errorf("failed to parse Kucoin level2depth data: %w", err)
		}
		asks = depthData.Asks
		bids = depthData.Bids
	} else {
		// Старый формат для level2
		var orderBookData KucoinOrderBookData
		dataBytes, err := json.Marshal(wsMsg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Kucoin orderbook data: %w", err)
		}
		if err := json.Unmarshal(dataBytes, &orderBookData); err != nil {
			return nil, fmt.Errorf("failed to parse Kucoin orderbook data: %w", err)
		}
		asks = orderBookData.Asks
		bids = orderBookData.Bids
		// Используем символ из данных, если доступен
		if orderBookData.Symbol != "" {
			symbol = orderBookData.Symbol
		}
	}

	// Конвертируем символ Kucoin в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("kucoin", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Kucoin symbol %s: %w", symbol, err)
	}

	bidLevels := make([]market.PriceLevel, 0, len(bids))
	for _, bid := range bids {
		if len(bid) >= 2 {
			price := parseFloat(bid[0])
			volume := parseFloat(bid[1])
			bidLevels = append(bidLevels, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	askLevels := make([]market.PriceLevel, 0, len(asks))
	for _, ask := range asks {
		if len(ask) >= 2 {
			price := parseFloat(ask[0])
			volume := parseFloat(ask[1])
			askLevels = append(askLevels, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	orderbook := market.UnifiedOrderBook{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		Bids:          bidLevels,
		Asks:          askLevels,
		Depth:         len(bidLevels) + len(askLevels),
		UpdateType:    market.OrderBookUpdateTypeSnapshot, // Level2Depth - полные снимки
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "kucoin",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *KucoinParser) parseTicker(wsMsg KucoinWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var tickerData KucoinTickerData

	dataBytes, err := json.Marshal(wsMsg.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Kucoin ticker data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &tickerData); err != nil {
		return nil, fmt.Errorf("failed to parse Kucoin ticker data: %w", err)
	}

	// Извлекаем символ из топика (например, "/market/ticker:BTC-USDT" -> "BTC-USDT")
	symbol := tickerData.Symbol
	if symbol == "" && wsMsg.Topic != "" {
		// Парсим символ из топика
		parts := strings.Split(wsMsg.Topic, ":")
		if len(parts) == 2 {
			symbol = parts[1]
		}
	}

	if symbol == "" {
		return nil, fmt.Errorf("no symbol found in ticker message")
	}

	// Конвертируем символ Kucoin в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("kucoin", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Kucoin symbol %s: %w", symbol, err)
	}

	// Kucoin ticker содержит только best bid/ask, создаем UnifiedBestPrice
	bestPrice := market.UnifiedBestPrice{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		BestBid:       parseFloat(tickerData.BestBid),
		BestAsk:       parseFloat(tickerData.BestAsk),
		BidVolume:     parseFloat(tickerData.BestBidSize),
		AskVolume:     parseFloat(tickerData.BestAskSize),
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "kucoin",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeBestPrice,
		Timestamp:     timestamp,
		Data:          bestPrice,
	}, nil
}

func (p *KucoinParser) parseMatch(wsMsg KucoinWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var matchData KucoinMatch

	dataBytes, err := json.Marshal(wsMsg.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Kucoin match data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &matchData); err != nil {
		return nil, fmt.Errorf("failed to parse Kucoin match data: %w", err)
	}

	// Конвертируем символ Kucoin в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("kucoin", matchData.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Kucoin symbol %s: %w", matchData.Symbol, err)
	}

	var side market.TradeSide
	if matchData.Side == "buy" {
		side = market.TradeSideBuy
	} else {
		side = market.TradeSideSell
	}

	trade := market.UnifiedTrade{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		TradeID:       matchData.TradeID,
		Price:         parseFloat(matchData.Price),
		Volume:        parseFloat(matchData.Size),
		Side:          side,
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "kucoin",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTrade,
		Timestamp:     timestamp,
		Data:          trade,
	}, nil
}
