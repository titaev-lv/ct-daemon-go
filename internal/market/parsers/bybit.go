package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"daemon-go/internal/market"
)

// BybitParser - парсер сообщений Bybit
type BybitParser struct {
	symbolRegistry *market.SymbolRegistry
}

// BybitWebSocketMessage - общий формат WebSocket сообщений Bybit
type BybitWebSocketMessage struct {
	Topic string      `json:"topic"`
	Type  string      `json:"type"`
	Data  interface{} `json:"data"`
	Ts    int64       `json:"ts"`
}

// BybitOrderBookData - формат orderbook от Bybit
type BybitOrderBookData struct {
	Symbol string      `json:"s"`
	Bids   [][]string  `json:"b"`
	Asks   [][]string  `json:"a"`
	Update interface{} `json:"u"` // может быть int или string
	Seq    int64       `json:"seq"`
}

// BybitTickerData - формат ticker от Bybit
type BybitTickerData struct {
	Symbol       string `json:"symbol"`
	LastPrice    string `json:"lastPrice"`
	BidPrice     string `json:"bid1Price"`
	BidSize      string `json:"bid1Size"`
	AskPrice     string `json:"ask1Price"`
	AskSize      string `json:"ask1Size"`
	Volume24h    string `json:"volume24h"`
	Turnover24h  string `json:"turnover24h"`
	Price24hPcnt string `json:"price24hPcnt"`
	HighPrice24h string `json:"highPrice24h"`
	LowPrice24h  string `json:"lowPrice24h"`
	PrevPrice24h string `json:"prevPrice24h"`
}

func NewBybitParser() *BybitParser {
	return &BybitParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *BybitParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "bybit"
}

func (p *BybitParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	var wsMsg BybitWebSocketMessage
	if err := json.Unmarshal(rawData, &wsMsg); err != nil {
		return nil, fmt.Errorf("failed to parse Bybit WebSocket message: %w", err)
	}

	timestamp := time.Now()
	if wsMsg.Ts > 0 {
		timestamp = time.Unix(wsMsg.Ts/1000, (wsMsg.Ts%1000)*1000000)
	}

	// Определяем тип сообщения по topic
	switch {
	case contains(wsMsg.Topic, "orderbook"):
		return p.parseOrderBook(wsMsg, timestamp)
	case contains(wsMsg.Topic, "tickers"):
		return p.parseTicker(wsMsg, timestamp)
	default:
		return nil, fmt.Errorf("unknown Bybit topic: %s", wsMsg.Topic)
	}
}

func (p *BybitParser) parseOrderBook(wsMsg BybitWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	// Bybit может отправлять данные как объект или массив
	var orderBookData BybitOrderBookData

	dataBytes, err := json.Marshal(wsMsg.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Bybit orderbook data: %w", err)
	}

	// Попробуем сначала как объект
	if err := json.Unmarshal(dataBytes, &orderBookData); err != nil {
		// Если не получилось, попробуем как массив
		var dataArray []BybitOrderBookData
		if err := json.Unmarshal(dataBytes, &dataArray); err != nil {
			return nil, fmt.Errorf("failed to parse Bybit orderbook data: %w", err)
		}
		if len(dataArray) == 0 {
			return nil, fmt.Errorf("empty Bybit orderbook data array")
		}
		orderBookData = dataArray[0]
	}

	// Конвертируем символ Bybit в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("bybit", orderBookData.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Bybit symbol %s: %w", orderBookData.Symbol, err)
	}

	bids := make([]market.PriceLevel, 0, len(orderBookData.Bids))
	for _, bid := range orderBookData.Bids {
		if len(bid) >= 2 {
			price := parseFloat(bid[0])
			volume := parseFloat(bid[1])
			bids = append(bids, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	asks := make([]market.PriceLevel, 0, len(orderBookData.Asks))
	for _, ask := range orderBookData.Asks {
		if len(ask) >= 2 {
			price := parseFloat(ask[0])
			volume := parseFloat(ask[1])
			asks = append(asks, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	// Определяем тип обновления на основе поля type
	var updateType market.OrderBookUpdateType
	if wsMsg.Type == "snapshot" {
		updateType = market.OrderBookUpdateTypeSnapshot
	} else {
		updateType = market.OrderBookUpdateTypeIncremental // delta или другие типы
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
		Exchange:      "bybit",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *BybitParser) parseTicker(wsMsg BybitWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var tickerData BybitTickerData

	dataBytes, err := json.Marshal(wsMsg.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Bybit ticker data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, &tickerData); err != nil {
		return nil, fmt.Errorf("failed to parse Bybit ticker data: %w", err)
	}

	// Конвертируем символ Bybit в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("bybit", tickerData.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Bybit symbol %s: %w", tickerData.Symbol, err)
	}

	ticker := market.UnifiedTicker{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		LastPrice:     parseFloat(tickerData.LastPrice),
		BestBid:       parseFloat(tickerData.BidPrice),
		BestAsk:       parseFloat(tickerData.AskPrice),
		Volume24h:     parseFloat(tickerData.Volume24h),
		Change24h:     parseFloat(tickerData.PrevPrice24h) - parseFloat(tickerData.LastPrice),
		ChangePct24h:  parseFloat(tickerData.Price24hPcnt),
		High24h:       parseFloat(tickerData.HighPrice24h),
		Low24h:        parseFloat(tickerData.LowPrice24h),
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "bybit",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTicker,
		Timestamp:     timestamp,
		Data:          ticker,
	}, nil
}
