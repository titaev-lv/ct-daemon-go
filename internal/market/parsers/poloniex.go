package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"daemon-go/internal/market"
)

// PoloniexParser - парсер сообщений Poloniex
type PoloniexParser struct {
	symbolRegistry *market.SymbolRegistry
}

// PoloniexWebSocketMessage - общий формат WebSocket сообщений Poloniex
type PoloniexWebSocketMessage struct {
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"` // Может быть массивом или объектом
}

// PoloniexOrderBookUpdate - обновление orderbook от Poloniex
type PoloniexOrderBookUpdate struct {
	Symbol     string     `json:"symbol"`
	CreateTime int64      `json:"createTime"`
	Asks       [][]string `json:"asks"`
	Bids       [][]string `json:"bids"`
	ID         int64      `json:"id"`
	Ts         int64      `json:"ts"`
}

// PoloniexTickerUpdate - обновление ticker от Poloniex
type PoloniexTickerUpdate struct {
	Symbol      string `json:"symbol"`
	Open        string `json:"open"`
	Low         string `json:"low"`
	High        string `json:"high"`
	Close       string `json:"close"`
	Quantity    string `json:"quantity"`
	Amount      string `json:"amount"`
	TradeCount  int    `json:"tradeCount"`
	StartTime   int64  `json:"startTime"`
	CloseTime   int64  `json:"closeTime"`
	DisplayName string `json:"displayName"`
	DailyChange string `json:"dailyChange"`
	Bid         string `json:"bid"`
	BidQuantity string `json:"bidQuantity"`
	Ask         string `json:"ask"`
	AskQuantity string `json:"askQuantity"`
	Ts          int64  `json:"ts"`
}

func NewPoloniexParser() *PoloniexParser {
	return &PoloniexParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *PoloniexParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "poloniex"
}

func (p *PoloniexParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	var wsMsg PoloniexWebSocketMessage
	if err := json.Unmarshal(rawData, &wsMsg); err != nil {
		return nil, fmt.Errorf("failed to parse Poloniex WebSocket message: %w", err)
	}

	timestamp := time.Now()

	// Определяем тип сообщения по каналу
	switch {
	case contains(wsMsg.Channel, "book"):
		return p.parseOrderBook(wsMsg, timestamp)
	case contains(wsMsg.Channel, "ticker"):
		return p.parseTicker(wsMsg, timestamp)
	default:
		return nil, fmt.Errorf("unknown Poloniex channel: %s", wsMsg.Channel)
	}
}

func (p *PoloniexParser) parseOrderBook(wsMsg PoloniexWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	// Пытаемся преобразовать данные в массив
	var orderBookUpdate PoloniexOrderBookUpdate

	// Если данные уже массив
	if dataArray, ok := wsMsg.Data.([]interface{}); ok {
		if len(dataArray) == 0 {
			return nil, fmt.Errorf("empty Poloniex orderbook data")
		}

		dataBytes, err := json.Marshal(dataArray[0])
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Poloniex orderbook data: %w", err)
		}

		if err := json.Unmarshal(dataBytes, &orderBookUpdate); err != nil {
			return nil, fmt.Errorf("failed to parse Poloniex orderbook update: %w", err)
		}
	} else {
		// Если данные объект
		dataBytes, err := json.Marshal(wsMsg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Poloniex orderbook data: %w", err)
		}

		if err := json.Unmarshal(dataBytes, &orderBookUpdate); err != nil {
			return nil, fmt.Errorf("failed to parse Poloniex orderbook update: %w", err)
		}
	}

	// Конвертируем символ Poloniex в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("poloniex", orderBookUpdate.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Poloniex symbol %s: %w", orderBookUpdate.Symbol, err)
	}

	bids := make([]market.PriceLevel, 0, len(orderBookUpdate.Bids))
	for _, bid := range orderBookUpdate.Bids {
		if len(bid) >= 2 {
			price := parseFloat(bid[0])
			volume := parseFloat(bid[1])
			bids = append(bids, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	asks := make([]market.PriceLevel, 0, len(orderBookUpdate.Asks))
	for _, ask := range orderBookUpdate.Asks {
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
		UpdateType:    market.OrderBookUpdateTypeIncremental, // Poloniex - инкрементальные обновления
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "poloniex",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *PoloniexParser) parseTicker(wsMsg PoloniexWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	// Пытаемся преобразовать данные в массив
	var tickerUpdate PoloniexTickerUpdate

	// Если данные уже массив
	if dataArray, ok := wsMsg.Data.([]interface{}); ok {
		if len(dataArray) == 0 {
			return nil, fmt.Errorf("empty Poloniex ticker data")
		}

		dataBytes, err := json.Marshal(dataArray[0])
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Poloniex ticker data: %w", err)
		}

		if err := json.Unmarshal(dataBytes, &tickerUpdate); err != nil {
			return nil, fmt.Errorf("failed to parse Poloniex ticker update: %w", err)
		}
	} else {
		// Если данные объект
		dataBytes, err := json.Marshal(wsMsg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Poloniex ticker data: %w", err)
		}

		if err := json.Unmarshal(dataBytes, &tickerUpdate); err != nil {
			return nil, fmt.Errorf("failed to parse Poloniex ticker update: %w", err)
		}
	}

	// Конвертируем символ Poloniex в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("poloniex", tickerUpdate.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert Poloniex symbol %s: %w", tickerUpdate.Symbol, err)
	}

	// Рассчитываем изменение за 24 часа
	closePrice := parseFloat(tickerUpdate.Close)
	openPrice := parseFloat(tickerUpdate.Open)
	change24h := closePrice - openPrice
	changePct24h := float64(0)
	if openPrice != 0 {
		changePct24h = (change24h / openPrice) * 100
	}

	ticker := market.UnifiedTicker{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		LastPrice:     closePrice,
		BestBid:       parseFloat(tickerUpdate.Bid),
		BestAsk:       parseFloat(tickerUpdate.Ask),
		Volume24h:     parseFloat(tickerUpdate.Quantity),
		Change24h:     change24h,
		ChangePct24h:  changePct24h,
		High24h:       parseFloat(tickerUpdate.High),
		Low24h:        parseFloat(tickerUpdate.Low),
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "poloniex",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTicker,
		Timestamp:     timestamp,
		Data:          ticker,
	}, nil
}
