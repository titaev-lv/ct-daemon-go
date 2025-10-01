package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"daemon-go/internal/market"
)

// BinanceParser - парсер сообщений Binance с поддержкой унифицированных символов
type BinanceParser struct {
	symbolRegistry *market.SymbolRegistry
}

// BinanceDepthMessage - формат orderbook от Binance
type BinanceDepthMessage struct {
	Stream string `json:"stream"`
	Data   struct {
		Symbol        string     `json:"s"`
		FirstUpdateID int64      `json:"U"`
		FinalUpdateID int64      `json:"u"`
		Bids          [][]string `json:"b"`
		Asks          [][]string `json:"a"`
	} `json:"data"`
}

// BinanceTickerMessage - формат ticker от Binance
type BinanceTickerMessage struct {
	Stream string `json:"stream"`
	Data   struct {
		Symbol         string `json:"s"`
		PriceChange    string `json:"p"`
		PriceChangePct string `json:"P"`
		LastPrice      string `json:"c"`
		BidPrice       string `json:"b"`
		AskPrice       string `json:"a"`
		Volume         string `json:"v"`
		High           string `json:"h"`
		Low            string `json:"l"`
	} `json:"data"`
}

// BinanceBookTickerMessage - формат best price от Binance
type BinanceBookTickerMessage struct {
	Stream string `json:"stream"`
	Data   struct {
		Symbol   string `json:"s"`
		BidPrice string `json:"b"`
		BidQty   string `json:"B"`
		AskPrice string `json:"a"`
		AskQty   string `json:"A"`
	} `json:"data"`
}

func NewBinanceParser() *BinanceParser {
	return &BinanceParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *BinanceParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "binance"
}

func (p *BinanceParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	// Определяем тип сообщения по содержимому
	var streamMessage struct {
		Stream string `json:"stream"`
	}

	if err := json.Unmarshal(rawData, &streamMessage); err != nil {
		return nil, fmt.Errorf("failed to parse stream: %w", err)
	}

	timestamp := time.Now()

	switch {
	case contains(streamMessage.Stream, "@depth"):
		return p.parseOrderBook(rawData, timestamp)
	case contains(streamMessage.Stream, "@ticker"):
		return p.parseTicker(rawData, timestamp)
	case contains(streamMessage.Stream, "@bookTicker"):
		return p.parseBestPrice(rawData, timestamp)
	default:
		return nil, fmt.Errorf("unknown stream type: %s", streamMessage.Stream)
	}
}

func (p *BinanceParser) parseOrderBook(rawData []byte, timestamp time.Time) (*market.UnifiedMessage, error) {
	var msg BinanceDepthMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse orderbook: %w", err)
	}

	// Конвертируем символ Binance в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("binance", msg.Data.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert symbol %s: %w", msg.Data.Symbol, err)
	}

	bids := make([]market.PriceLevel, 0, len(msg.Data.Bids))
	for _, bid := range msg.Data.Bids {
		if len(bid) >= 2 {
			price := parseFloat(bid[0])
			volume := parseFloat(bid[1])
			bids = append(bids, market.PriceLevel{
				Price:  price,
				Volume: volume,
			})
		}
	}

	asks := make([]market.PriceLevel, 0, len(msg.Data.Asks))
	for _, ask := range msg.Data.Asks {
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
		UpdateType:    market.OrderBookUpdateTypeSnapshot, // Binance depth streams обычно snapshot
		Raw:           msg,
	}

	return &market.UnifiedMessage{
		Exchange:      "binance",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *BinanceParser) parseTicker(rawData []byte, timestamp time.Time) (*market.UnifiedMessage, error) {
	var msg BinanceTickerMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse ticker: %w", err)
	}

	// Конвертируем символ Binance в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("binance", msg.Data.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert symbol %s: %w", msg.Data.Symbol, err)
	}

	ticker := market.UnifiedTicker{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		LastPrice:     parseFloat(msg.Data.LastPrice),
		BestBid:       parseFloat(msg.Data.BidPrice),
		BestAsk:       parseFloat(msg.Data.AskPrice),
		Volume24h:     parseFloat(msg.Data.Volume),
		Change24h:     parseFloat(msg.Data.PriceChange),
		ChangePct24h:  parseFloat(msg.Data.PriceChangePct),
		High24h:       parseFloat(msg.Data.High),
		Low24h:        parseFloat(msg.Data.Low),
		Raw:           msg,
	}

	return &market.UnifiedMessage{
		Exchange:      "binance",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTicker,
		Timestamp:     timestamp,
		Data:          ticker,
	}, nil
}

func (p *BinanceParser) parseBestPrice(rawData []byte, timestamp time.Time) (*market.UnifiedMessage, error) {
	var msg BinanceBookTickerMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse best price: %w", err)
	}

	// Конвертируем символ Binance в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("binance", msg.Data.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert symbol %s: %w", msg.Data.Symbol, err)
	}

	bestPrice := market.UnifiedBestPrice{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		BestBid:       parseFloat(msg.Data.BidPrice),
		BestAsk:       parseFloat(msg.Data.AskPrice),
		BidVolume:     parseFloat(msg.Data.BidQty),
		AskVolume:     parseFloat(msg.Data.AskQty),
		Raw:           msg,
	}

	return &market.UnifiedMessage{
		Exchange:      "binance",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeBestPrice,
		Timestamp:     timestamp,
		Data:          bestPrice,
	}, nil
}
