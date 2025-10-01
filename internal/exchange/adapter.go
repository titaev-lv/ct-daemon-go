package exchange

// MarketPair - структура для передачи пары символ + ID
type MarketPair struct {
	Symbol string `json:"symbol"`  // символ пары (BTC/USDT)
	PairID int    `json:"pair_id"` // ID пары в системе БД
}

// Adapter - интерфейс для адаптеров бирж
// Реализуется для каждой биржи

type Adapter interface {
	Start() error
	Stop() error
	IsActive() bool
	ExchangeName() string
	SubscribeMarkets(pairs []string, marketType string, depth int) error
	UnsubscribeMarkets(pairs []string, marketType string, depth int) error
	// Новые методы с поддержкой PairID
	SubscribeMarketsWithPairID(pairs []MarketPair, marketType string, depth int) error
	UnsubscribeMarketsWithPairID(pairs []MarketPair, marketType string, depth int) error
}
