package worker

import (
	"sync"
	"time"

	"daemon-go/internal/db"
	"daemon-go/pkg/log"
)

var tradeMonitorLogger = log.New("trade_monitor")

type TradeMonitor struct {
	driver       db.DBDriver
	traderMap    *map[int]*TraderWorker
	workersMutex *sync.Mutex
	stopChan     chan struct{}
	pollInterval int
}

// NewTradeMonitor создает новый монитор торгов
func NewTradeMonitor(driver db.DBDriver, traderMap *map[int]*TraderWorker, workersMutex *sync.Mutex, stopChan chan struct{}, pollInterval int) *TradeMonitor {
	return &TradeMonitor{
		driver:       driver,
		traderMap:    traderMap,
		workersMutex: workersMutex,
		stopChan:     stopChan,
		pollInterval: pollInterval,
	}
}

// SetPollInterval позволяет динамически менять pollInterval
func (tm *TradeMonitor) SetPollInterval(interval int) {
	tm.pollInterval = interval
}

// Start запускает мониторинг
func (tm *TradeMonitor) Start() {
	ticker := time.NewTicker(time.Duration(tm.pollInterval) * time.Second)
	defer ticker.Stop()

	tradeMonitorLogger.Debug("[DEBUG] TradeMonitor started with pollInterval=%d", tm.pollInterval)

	for {
		select {
		case <-tm.stopChan:
			tradeMonitorLogger.Debug("[DEBUG] TradeMonitor received stop signal")
			tm.stopAllWorkers()
			tradeMonitorLogger.Info("TradeMonitor stopped")
			return
		case <-ticker.C:
			tradeMonitorLogger.Debug("[DEBUG] TradeMonitor tick: checking trades...")
			tm.checkTrades()
		}
	}
}

// checkTrades проверяет активные сделки и запускает новые воркеры
func (tm *TradeMonitor) checkTrades() {
	trades, err := tm.driver.GetActiveTrades()
	if err != nil {
		tradeMonitorLogger.Error("TradeMonitor: error fetching active trades: %v", err)
		return
	}

	tradeMonitorLogger.Debug("[DEBUG] checkTrades: %d active trades fetched", len(trades))

	tm.workersMutex.Lock()
	defer tm.workersMutex.Unlock()

	// Собираем set активных id
	activeIDs := make(map[int]struct{}, len(trades))
	for _, t := range trades {
		activeIDs[t.ID] = struct{}{}
		if _, exists := (*tm.traderMap)[t.ID]; !exists {
			tradeMonitorLogger.Debug("[DEBUG] checkTrades: starting new TraderWorker for id=%d", t.ID)
			// Передаём tradeMonitorLogger как модульный логгер для трейдера
			tw := NewTraderWorker(t, tm.driver, tradeMonitorLogger)
			(*tm.traderMap)[t.ID] = tw
			go tw.Start()
			tradeMonitorLogger.Info("TradeMonitor: TraderWorker %d started", t.ID)
		} else {
			tradeMonitorLogger.Debug("[DEBUG] checkTrades: TraderWorker for id=%d already running", t.ID)
		}
	}

	// Останавливаем воркеры, которых больше нет в активных trades
	for id, w := range *tm.traderMap {
		if _, stillActive := activeIDs[id]; !stillActive {
			tradeMonitorLogger.Debug("[DEBUG] checkTrades: stopping TraderWorker for id=%d (no longer active)", id)
			tradeMonitorLogger.Info("TradeMonitor: TraderWorker %d stopped (no longer active)", id)
			w.Stop()
			delete(*tm.traderMap, id)
		}
	}
}

// stopAllWorkers останавливает всех трейдер-воркеров
func (tm *TradeMonitor) stopAllWorkers() {
	tm.workersMutex.Lock()
	defer tm.workersMutex.Unlock()

	tradeMonitorLogger.Debug("[DEBUG] stopAllWorkers: stopping %d trader workers", len(*tm.traderMap))
	for id, w := range *tm.traderMap {
		tradeMonitorLogger.Debug("[DEBUG] stopAllWorkers: stopping TraderWorker id=%d", id)
		w.Stop()
		delete(*tm.traderMap, id)
	}
}
