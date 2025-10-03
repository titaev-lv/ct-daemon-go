package dataworker

import (
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/pkg/log"
	"fmt"
	"strings"
	"time"
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
	startTime   time.Time // время запуска worker'а
	restEnabled bool      // включен ли REST API
}

var logger = log.New("DataWorker")

// NewDataWorker создаёт DataWorker по данным db.Exchange
func NewDataWorker(ex db.Exchange) *DataWorker {
	logger.Info("Creating DataWorker for exchange: %s (ID: %d)", ex.Name, ex.ID)
	return &DataWorker{
		adapter:     exchange.NewAdapter(ex),
		pairs:       nil,
		marketType:  "spot",
		depth:       5,
		startTime:   time.Now(),
		restEnabled: true, // по умолчанию REST API включен
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

// GetPairs возвращает список торговых пар в виде строки через запятую
func (w *DataWorker) GetPairs() string {
	if len(w.marketPairs) > 0 {
		var pairs []string
		for _, mp := range w.marketPairs {
			pairs = append(pairs, mp.Symbol)
		}
		return strings.Join(pairs, ",")
	}
	return strings.Join(w.pairs, ",")
}

// GetMarketType возвращает тип рынка (spot/futures)
func (w *DataWorker) GetMarketType() string {
	return w.marketType
}

// GetDepth возвращает глубину стакана
func (w *DataWorker) GetDepth() int {
	return w.depth
}

// GetPairCount возвращает количество отслеживаемых пар
func (w *DataWorker) GetPairCount() int {
	if len(w.marketPairs) > 0 {
		return len(w.marketPairs)
	}
	return len(w.pairs)
}

// GetStartTime возвращает время запуска worker'а
func (w *DataWorker) GetStartTime() time.Time {
	return w.startTime
}

// GetUptime возвращает время работы в секундах
func (w *DataWorker) GetUptime() int64 {
	return int64(time.Since(w.startTime).Seconds())
}

// GetUptimeString возвращает время работы в читаемом формате
func (w *DataWorker) GetUptimeString() string {
	uptime := time.Since(w.startTime)
	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60
	seconds := int(uptime.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// GetWSConnectionStatus возвращает статус WebSocket подключения
func (w *DataWorker) GetWSConnectionStatus() string {
	if w.adapter != nil {
		// Предполагаем, что у адаптера есть метод для проверки WS статуса
		// Если адаптер активен, значит WS подключен
		if w.adapter.IsActive() {
			return "CONNECTED"
		}
	}
	return "DISCONNECTED"
}

// GetRestAPIStatus возвращает статус работы через REST API
func (w *DataWorker) GetRestAPIStatus() bool {
	return w.restEnabled
}

// SetRestAPIStatus устанавливает статус работы через REST API
func (w *DataWorker) SetRestAPIStatus(enabled bool) {
	w.restEnabled = enabled
	logger.Debug("REST API status set to %v for %s", enabled, w.ExchangeName())
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
