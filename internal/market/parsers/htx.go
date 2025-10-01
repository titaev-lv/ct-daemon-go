package parsers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"daemon-go/internal/market"
)

// HTXParser - парсер сообщений HTX (Huobi)
type HTXParser struct {
	symbolRegistry *market.SymbolRegistry
}

// HTXWebSocketMessage - общий формат WebSocket сообщений HTX
type HTXWebSocketMessage struct {
	Ch   string      `json:"ch"`
	Ts   int64       `json:"ts"`
	Tick interface{} `json:"tick"`
	ID   string      `json:"id,omitempty"`
	Rep  string      `json:"rep,omitempty"`
}

// HTXOrderBookTick - формат orderbook tick от HTX
type HTXOrderBookTick struct {
	ID      int64       `json:"id"`
	Ts      int64       `json:"ts"`
	Version int64       `json:"version"`
	Bids    [][]float64 `json:"bids"`
	Asks    [][]float64 `json:"asks"`
}

// HTXTickerTick - формат ticker tick от HTX
type HTXTickerTick struct {
	ID      int64   `json:"id"`
	Ts      int64   `json:"ts"`
	Open    float64 `json:"open"`
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Close   float64 `json:"close"`
	Amount  float64 `json:"amount"`
	Vol     float64 `json:"vol"`
	Count   int64   `json:"count"`
	Bid     float64 `json:"bid"`
	BidSize float64 `json:"bidSize"`
	Ask     float64 `json:"ask"`
	AskSize float64 `json:"askSize"`
}

// HTXBBOTick - формат best bid/offer tick от HTX
type HTXBBOTick struct {
	Symbol  string  `json:"symbol"`
	Ts      int64   `json:"ts"`
	Bid     float64 `json:"bid"`
	BidSize float64 `json:"bidSize"`
	Ask     float64 `json:"ask"`
	AskSize float64 `json:"askSize"`
}

func NewHTXParser() *HTXParser {
	return &HTXParser{
		symbolRegistry: market.NewSymbolRegistry(),
	}
}

func (p *HTXParser) CanParse(exchange string, rawData []byte) bool {
	return exchange == "htx" || exchange == "huobi"
}

// DecompressGzip - публичный метод для декомпрессии gzip данных
func (p *HTXParser) DecompressGzip(data []byte) ([]byte, error) {
	return p.decompressGzip(data)
}

// decompressGzip - декомпрессия gzip данных
func (p *HTXParser) decompressGzip(data []byte) ([]byte, error) {
	// Проверяем gzip заголовок (0x1f, 0x8b)
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil // Данные не сжаты
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress gzip data: %w", err)
	}

	return decompressed, nil
}

// isPingPongMessage проверяет, является ли сообщение ping/pong от HTX
func (p *HTXParser) isPingPongMessage(data []byte) bool {
	// HTX ping сообщения обычно имеют формат {"ping": timestamp}
	var pingMsg map[string]interface{}
	if err := json.Unmarshal(data, &pingMsg); err != nil {
		return false
	}

	// Проверяем на ping сообщение
	if _, exists := pingMsg["ping"]; exists {
		return true
	}

	// Проверяем на pong сообщение
	if _, exists := pingMsg["pong"]; exists {
		return true
	}

	return false
}

func (p *HTXParser) ParseMessage(exchange string, rawData []byte) (*market.UnifiedMessage, error) {
	// Декомпрессируем данные если они сжаты
	decompressedData, err := p.decompressGzip(rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	// Проверяем на ping/pong сообщения HTX
	if p.isPingPongMessage(decompressedData) {
		return nil, nil // Возвращаем nil для ping/pong сообщений - их не нужно обрабатывать
	}

	var wsMsg HTXWebSocketMessage
	if err := json.Unmarshal(decompressedData, &wsMsg); err != nil {
		return nil, fmt.Errorf("failed to parse HTX WebSocket message: %w", err)
	}

	timestamp := time.Now()
	if wsMsg.Ts > 0 {
		timestamp = time.Unix(wsMsg.Ts/1000, (wsMsg.Ts%1000)*1000000)
	}

	// Обрабатываем подтверждения подписок
	if wsMsg.ID != "" && wsMsg.Ch == "" {
		return nil, nil // Подтверждение подписки - не нужно обрабатывать
	}

	// Определяем тип сообщения по каналу
	switch {
	case contains(wsMsg.Ch, "depth"):
		return p.parseOrderBook(wsMsg, timestamp)
	case contains(wsMsg.Ch, "ticker"):
		return p.parseTicker(wsMsg, timestamp)
	case contains(wsMsg.Ch, "bbo"):
		return p.parseBBO(wsMsg, timestamp)
	case wsMsg.Ch == "":
		return nil, nil // Пустой канал - пропускаем
	default:
		return nil, fmt.Errorf("unknown HTX channel: %s", wsMsg.Ch)
	}
}

func (p *HTXParser) parseOrderBook(wsMsg HTXWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var tickData HTXOrderBookTick

	tickBytes, err := json.Marshal(wsMsg.Tick)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal HTX orderbook tick: %w", err)
	}

	if err := json.Unmarshal(tickBytes, &tickData); err != nil {
		return nil, fmt.Errorf("failed to parse HTX orderbook tick: %w", err)
	}

	// Извлекаем символ из канала (например: market.btcusdt.depth.step0)
	symbol := p.extractSymbolFromChannel(wsMsg.Ch)
	if symbol == "" {
		return nil, fmt.Errorf("failed to extract symbol from HTX channel: %s", wsMsg.Ch)
	}

	// Конвертируем символ HTX в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("htx", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTX symbol %s: %w", symbol, err)
	}

	bids := make([]market.PriceLevel, 0, len(tickData.Bids))
	for _, bid := range tickData.Bids {
		if len(bid) >= 2 {
			bids = append(bids, market.PriceLevel{
				Price:  bid[0],
				Volume: bid[1],
			})
		}
	}

	asks := make([]market.PriceLevel, 0, len(tickData.Asks))
	for _, ask := range tickData.Asks {
		if len(ask) >= 2 {
			asks = append(asks, market.PriceLevel{
				Price:  ask[0],
				Volume: ask[1],
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
		UpdateType:    market.OrderBookUpdateTypeSnapshot, // HTX depth - полные снимки
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "htx",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeOrderBook,
		Timestamp:     timestamp,
		Data:          orderbook,
	}, nil
}

func (p *HTXParser) parseTicker(wsMsg HTXWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var tickData HTXTickerTick

	tickBytes, err := json.Marshal(wsMsg.Tick)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal HTX ticker tick: %w", err)
	}

	if err := json.Unmarshal(tickBytes, &tickData); err != nil {
		return nil, fmt.Errorf("failed to parse HTX ticker tick: %w", err)
	}

	// Извлекаем символ из канала
	symbol := p.extractSymbolFromChannel(wsMsg.Ch)
	if symbol == "" {
		return nil, fmt.Errorf("failed to extract symbol from HTX channel: %s", wsMsg.Ch)
	}

	// Конвертируем символ HTX в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("htx", symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTX symbol %s: %w", symbol, err)
	}

	ticker := market.UnifiedTicker{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		LastPrice:     tickData.Close,
		BestBid:       tickData.Bid,
		BestAsk:       tickData.Ask,
		Volume24h:     tickData.Vol,
		Change24h:     tickData.Close - tickData.Open,
		ChangePct24h:  ((tickData.Close - tickData.Open) / tickData.Open) * 100,
		High24h:       tickData.High,
		Low24h:        tickData.Low,
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "htx",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeTicker,
		Timestamp:     timestamp,
		Data:          ticker,
	}, nil
}

func (p *HTXParser) parseBBO(wsMsg HTXWebSocketMessage, timestamp time.Time) (*market.UnifiedMessage, error) {
	var bboData HTXBBOTick

	tickBytes, err := json.Marshal(wsMsg.Tick)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal HTX BBO tick: %w", err)
	}

	if err := json.Unmarshal(tickBytes, &bboData); err != nil {
		return nil, fmt.Errorf("failed to parse HTX BBO tick: %w", err)
	}

	// Конвертируем символ HTX в унифицированный формат
	unifiedSymbol, err := p.symbolRegistry.ConvertToUnified("htx", bboData.Symbol, "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTX symbol %s: %w", bboData.Symbol, err)
	}

	bestPrice := market.UnifiedBestPrice{
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Timestamp:     timestamp,
		BestBid:       bboData.Bid,
		BestAsk:       bboData.Ask,
		BidVolume:     bboData.BidSize,
		AskVolume:     bboData.AskSize,
		Raw:           wsMsg,
	}

	return &market.UnifiedMessage{
		Exchange:      "htx",
		Symbol:        unifiedSymbol.Symbol,
		UnifiedSymbol: unifiedSymbol,
		MessageType:   market.MessageTypeBestPrice,
		Timestamp:     timestamp,
		Data:          bestPrice,
	}, nil
}

// extractSymbolFromChannel извлекает символ из канала HTX
// Примеры каналов:
// market.btcusdt.depth.step0
// market.btcusdt.ticker
// market.btcusdt.bbo
func (p *HTXParser) extractSymbolFromChannel(channel string) string {
	parts := strings.Split(channel, ".")
	if len(parts) >= 2 && parts[0] == "market" {
		return strings.ToUpper(parts[1])
	}
	return ""
}
