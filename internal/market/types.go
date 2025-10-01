package market

import (
	"time"
)

// UnifiedMessage - единый внутренний формат сообщений от всех бирж
type UnifiedMessage struct {
	Exchange      string         `json:"exchange"`       // название биржи
	Symbol        string         `json:"symbol"`         // унифицированный символ (BTC/USDT или BTCUSDT)
	UnifiedSymbol *UnifiedSymbol `json:"unified_symbol"` // полная информация о символе
	MessageType   MessageType    `json:"message_type"`   // тип сообщения
	Timestamp     time.Time      `json:"timestamp"`      // время получения
	Data          interface{}    `json:"data"`           // данные в зависимости от типа
	PairID        int            `json:"pair_id"`        // ID пары в системе для связи с БД
}

// MessageType - типы сообщений
type MessageType string

const (
	MessageTypeOrderBook  MessageType = "orderbook"
	MessageTypeTicker     MessageType = "ticker"
	MessageTypeBestPrice  MessageType = "best_price"
	MessageTypeTrade      MessageType = "trade"
	MessageTypeKline      MessageType = "kline"
	MessageTypeOrderEvent MessageType = "order_event"
)

// OrderBookUpdateType - тип обновления order book
type OrderBookUpdateType string

const (
	OrderBookUpdateTypeSnapshot    OrderBookUpdateType = "snapshot"    // полный снимок
	OrderBookUpdateTypeIncremental OrderBookUpdateType = "incremental" // инкрементальное обновление
)

// UnifiedOrderBook - унифицированный формат orderbook
type UnifiedOrderBook struct {
	Symbol        string              `json:"symbol"`         // унифицированный символ
	UnifiedSymbol *UnifiedSymbol      `json:"unified_symbol"` // полная информация о символе
	Timestamp     time.Time           `json:"timestamp"`
	Bids          []PriceLevel        `json:"bids"` // покупки, сортированы по убыванию цены
	Asks          []PriceLevel        `json:"asks"` // продажи, сортированы по возрастанию цены
	Depth         int                 `json:"depth"`
	UpdateType    OrderBookUpdateType `json:"update_type"`   // тип обновления
	Raw           interface{}         `json:"raw,omitempty"` // оригинальные данные от биржи
}

// PriceLevel - уровень цены в orderbook
type PriceLevel struct {
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

// UnifiedTicker - унифицированный формат ticker
type UnifiedTicker struct {
	Symbol        string         `json:"symbol"`         // унифицированный символ
	UnifiedSymbol *UnifiedSymbol `json:"unified_symbol"` // полная информация о символе
	Timestamp     time.Time      `json:"timestamp"`
	LastPrice     float64        `json:"last_price"`
	BestBid       float64        `json:"best_bid"`
	BestAsk       float64        `json:"best_ask"`
	Volume24h     float64        `json:"volume_24h"`
	Change24h     float64        `json:"change_24h"`
	ChangePct24h  float64        `json:"change_pct_24h"`
	High24h       float64        `json:"high_24h"`
	Low24h        float64        `json:"low_24h"`
	Raw           interface{}    `json:"raw,omitempty"`
}

// UnifiedBestPrice - унифицированный формат лучших цен
type UnifiedBestPrice struct {
	Symbol        string         `json:"symbol"`         // унифицированный символ
	UnifiedSymbol *UnifiedSymbol `json:"unified_symbol"` // полная информация о символе
	Timestamp     time.Time      `json:"timestamp"`
	BestBid       float64        `json:"best_bid"`
	BestAsk       float64        `json:"best_ask"`
	BidVolume     float64        `json:"bid_volume"`
	AskVolume     float64        `json:"ask_volume"`
	Raw           interface{}    `json:"raw,omitempty"`
}

// UnifiedTrade - унифицированный формат сделки
type UnifiedTrade struct {
	Symbol        string         `json:"symbol"`         // унифицированный символ
	UnifiedSymbol *UnifiedSymbol `json:"unified_symbol"` // полная информация о символе
	Timestamp     time.Time      `json:"timestamp"`
	TradeID       string         `json:"trade_id"`
	Price         float64        `json:"price"`
	Volume        float64        `json:"volume"`
	Side          TradeSide      `json:"side"`
	Raw           interface{}    `json:"raw,omitempty"`
}

// TradeSide - сторона сделки
type TradeSide string

const (
	TradeSideBuy  TradeSide = "buy"
	TradeSideSell TradeSide = "sell"
)

// UnifiedOrderEvent - унифицированный формат событий ордеров
type UnifiedOrderEvent struct {
	Symbol          string         `json:"symbol"`         // унифицированный символ
	UnifiedSymbol   *UnifiedSymbol `json:"unified_symbol"` // полная информация о символе
	Timestamp       time.Time      `json:"timestamp"`
	OrderID         string         `json:"order_id"`
	ClientOrderID   string         `json:"client_order_id,omitempty"`
	Status          OrderStatus    `json:"status"`
	Side            TradeSide      `json:"side"`
	OrderType       OrderType      `json:"order_type"`
	Price           float64        `json:"price"`
	Volume          float64        `json:"volume"`
	FilledVolume    float64        `json:"filled_volume"`
	RemainingVolume float64        `json:"remaining_volume"`
	Fee             float64        `json:"fee"`
	FeeCurrency     string         `json:"fee_currency"`
	Raw             interface{}    `json:"raw,omitempty"`
}

// OrderStatus - статус ордера
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "new"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusCanceled        OrderStatus = "canceled"
	OrderStatusRejected        OrderStatus = "rejected"
	OrderStatusExpired         OrderStatus = "expired"
)

// OrderType - тип ордера
type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
)

// MessageHandler - интерфейс для обработки унифицированных сообщений
type MessageHandler interface {
	HandleMessage(msg UnifiedMessage) error
}

// MessageParser - интерфейс для парсинга сообщений от конкретной биржи
type MessageParser interface {
	ParseMessage(exchange string, rawData []byte) (*UnifiedMessage, error)
	CanParse(exchange string, rawData []byte) bool
}
