package exchange

import (
	"daemon-go/internal/db"
	"daemon-go/pkg/log"
	"strings"
)

var factoryLogger = log.New("exchange_factory")

// NewAdapter создает биржевой адаптер по данным из db.Exchange
func NewAdapter(ex db.Exchange) Adapter {
	factoryLogger.Debug("Creating adapter for exchange: name='%s', id=%d", ex.Name, ex.ID)

	// Нормализуем имя биржи к нижнему регистру для сравнения
	exchangeName := strings.ToLower(ex.Name)

	switch exchangeName {
	case "binance":
		factoryLogger.Debug("Creating BinanceAdapter")
		return NewBinanceAdapter(ex)
	case "bybit":
		factoryLogger.Debug("Creating BybitAdapter")
		return NewBybitAdapter(ex)
	case "kucoin":
		factoryLogger.Debug("Creating KucoinAdapter")
		return NewKucoinAdapter(ex)
	case "coinex":
		factoryLogger.Debug("Creating CoinexAdapter")
		return NewCoinexAdapter(ex)
	case "htx":
		factoryLogger.Debug("Creating HtxAdapter")
		return NewHtxAdapter(ex)
	case "poloniex":
		factoryLogger.Debug("Creating PoloniexAdapter")
		return NewPoloniexAdapter(ex)
	default:
		factoryLogger.Warn("Unknown exchange name '%s' (normalized: '%s'), using StubAdapter", ex.Name, exchangeName)
		return &StubAdapter{name: ex.Name}
	}
}

// StubAdapter - для неизвестных бирж

type StubAdapter struct {
	name   string
	logger *log.Logger
}

func (a *StubAdapter) Start() error {
	if a.logger == nil {
		a.logger = log.New("stub_adapter")
	}
	a.logger.Info("[STUB_ADAPTER] Starting stub adapter for %s", a.name)
	return nil
}
func (a *StubAdapter) Stop() error {
	if a.logger == nil {
		a.logger = log.New("stub_adapter")
	}
	a.logger.Info("[STUB_ADAPTER] Stopping stub adapter for %s", a.name)
	return nil
}
func (a *StubAdapter) IsActive() bool       { return false }
func (a *StubAdapter) ExchangeName() string { return a.name }
func (a *StubAdapter) SubscribeMarkets(pairs []string, marketType string, depth int) error {
	// Заглушка: ничего не делает
	return nil
}
func (a *StubAdapter) UnsubscribeMarkets(pairs []string, marketType string, depth int) error {
	// Заглушка: ничего не делает
	return nil
}
func (a *StubAdapter) SubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Заглушка: ничего не делает
	return nil
}
func (a *StubAdapter) UnsubscribeMarketsWithPairID(marketPairs []MarketPair, marketType string, depth int) error {
	// Заглушка: ничего не делает
	return nil
}
