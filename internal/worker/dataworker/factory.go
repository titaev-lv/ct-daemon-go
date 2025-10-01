package dataworker

import (
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/pkg/log"
)

// DataWorker - интерфейс для воркеров разных бирж
// Каждый воркер реализует этот интерфейс для своей биржи

// Factory функция для создания воркера по имени биржи

// DataWorker агрегирует exchange.Adapter
type DataWorker struct {
	adapter     exchange.Adapter
	pairs       []string              // для обратной совместимости
	marketPairs []exchange.MarketPair // новые пары с PairID
	marketType  string
	depth       int
}

var logger = log.New("DataWorker")

// NewDataWorker создаёт DataWorker по данным db.Exchange
func NewDataWorker(ex db.Exchange) *DataWorker {
	logger.Info("Creating DataWorker for exchange: %s (ID: %d)", ex.Name, ex.ID)
	return &DataWorker{
		adapter:    exchange.NewAdapter(ex),
		pairs:      nil,
		marketType: "spot",
		depth:      5,
	}
}

func (w *DataWorker) Start() error {
	logger.Debug("DataWorker.Start() called for %s", w.ExchangeName())

	// Сначала запускаем адаптер
	err := w.adapter.Start()
	if err != nil {
		logger.Error("Start failed for %s: %v", w.ExchangeName(), err)
		return err
	}
	logger.Info("Adapter started for %s", w.ExchangeName())

	// Затем устанавливаем подписки
	if len(w.marketPairs) > 0 {
		logger.Debug("Setting up subscriptions with PairID for %s: %v", w.ExchangeName(), w.marketPairs)
		if err := w.adapter.SubscribeMarketsWithPairID(w.marketPairs, w.marketType, w.depth); err != nil {
			logger.Error("Adapter subscribe with PairID error for %s: %v", w.ExchangeName(), err)
		} else {
			logger.Info("Subscriptions with PairID established for %s", w.ExchangeName())
		}
	} else if len(w.pairs) > 0 {
		logger.Debug("Setting up legacy subscriptions for %s: %v", w.ExchangeName(), w.pairs)
		if err := w.adapter.SubscribeMarkets(w.pairs, w.marketType, w.depth); err != nil {
			logger.Error("Adapter subscribe error for %s: %v", w.ExchangeName(), err)
		} else {
			logger.Info("Legacy subscriptions established for %s", w.ExchangeName())
		}
	} else {
		logger.Warn("No pairs configured for %s", w.ExchangeName())
	}

	logger.Info("Start successful for %s", w.ExchangeName())
	return nil
}

// SetSubscription позволяет задать параметры подписки (используется DataMonitor)
func (w *DataWorker) SetSubscription(pairs []string, marketType string, depth int) {
	// Unsubscribe from pairs that are no longer present
	if len(w.pairs) > 0 {
		// Find pairs to unsubscribe
		toUnsub := difference(w.pairs, pairs)
		if len(toUnsub) > 0 {
			if err := w.adapter.UnsubscribeMarkets(toUnsub, marketType, depth); err != nil {
				logger.Error("Adapter unsubscribe error: %v", err)
			}
		}
	}
	w.pairs = pairs
	w.marketType = marketType
	w.depth = depth
	// Subscribe to new pairs (existing logic)
	if len(pairs) > 0 {
		if err := w.adapter.SubscribeMarkets(pairs, marketType, depth); err != nil {
			logger.Error("Adapter subscribe error: %v", err)
		}
	}
	// End of SetSubscription
}

// SetSubscriptionWithPairID позволяет задать параметры подписки с PairID (новый метод)
func (w *DataWorker) SetSubscriptionWithPairID(marketPairs []exchange.MarketPair, marketType string, depth int) {
	logger.Debug("SetSubscriptionWithPairID called for %s: pairs=%v, marketType=%s, depth=%d", w.ExchangeName(), marketPairs, marketType, depth)

	// Отписываемся от старых пар
	if len(w.marketPairs) > 0 {
		logger.Debug("Unsubscribing from old pairs for %s: %v", w.ExchangeName(), w.marketPairs)
		if err := w.adapter.UnsubscribeMarketsWithPairID(w.marketPairs, w.marketType, w.depth); err != nil {
			logger.Error("Adapter unsubscribe error for %s: %v", w.ExchangeName(), err)
		}
	}

	w.marketPairs = marketPairs
	w.marketType = marketType
	w.depth = depth

	logger.Info("Updated subscription config for %s: %d pairs, marketType=%s, depth=%d", w.ExchangeName(), len(marketPairs), marketType, depth)

	// Подписываемся на новые пары только если адаптер уже запущен
	if len(marketPairs) > 0 && w.adapter.IsActive() {
		logger.Debug("Adapter is active, subscribing to new pairs for %s", w.ExchangeName())
		if err := w.adapter.SubscribeMarketsWithPairID(marketPairs, marketType, depth); err != nil {
			logger.Error("Adapter subscribe error for %s: %v", w.ExchangeName(), err)
		} else {
			logger.Info("Successfully subscribed to %d pairs for %s", len(marketPairs), w.ExchangeName())
		}
	} else if len(marketPairs) > 0 {
		logger.Debug("Adapter not active yet for %s, subscription will be done during Start()", w.ExchangeName())
	}
}

func (w *DataWorker) Stop() error {
	err := w.adapter.Stop()
	if err != nil {
		logger.Error("Stop failed for %s: %v", w.ExchangeName(), err)
	} else {
		logger.Info("Stop successful for %s", w.ExchangeName())
	}
	return err
}

func (w *DataWorker) IsActive() bool {
	active := w.adapter.IsActive()
	logger.Debug("IsActive for %s: %v", w.ExchangeName(), active)
	return active
}

func (w *DataWorker) ExchangeName() string {
	name := w.adapter.ExchangeName()
	logger.Debug("ExchangeName called: %s", name)
	return name
}

// difference returns elements in a but not in b
func difference(a, b []string) []string {
	m := make(map[string]struct{}, len(b))
	for _, x := range b {
		m[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := m[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}
