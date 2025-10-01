package worker

import (
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/internal/worker/dataworker"
	"daemon-go/pkg/log"
	"fmt"
	"sync"
	"time"
)

// DataMonitor управляет DataWorker по (EXCHANGE_ID, MARKET)
type DataMonitor struct {
	workers      map[string]*dataworker.DataWorker // ключ: EXCHANGE_ID|MARKET
	workersMutex sync.Mutex
	logger       *log.Logger
	stopChan     chan struct{}
	wg           sync.WaitGroup
	// Метрики
	totalStarted int
	totalStopped int
	totalErrors  int

	dbDriver db.DBDriver // добавлено: ссылка на драйвер БД
}

func NewDataMonitor(logger *log.Logger, driver db.DBDriver) *DataMonitor {
	return &DataMonitor{
		workers:  make(map[string]*dataworker.DataWorker),
		logger:   logger,
		stopChan: make(chan struct{}),
		dbDriver: driver, // сохраняем драйвер
	}
}

// Start запускает DataMonitor (цикл обновления воркеров)
func (dm *DataMonitor) Start() {
	dm.logger.Info("[DATA_MONITOR] Started")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				dm.logger.Error("[DATA_MONITOR] Panic in Start: %v", r)
			}
		}()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-dm.stopChan:
				dm.logger.Info("[DATA_MONITOR] Stop signal received")
				return
			case <-ticker.C:
				// Получаем пары из БД
				defer func() {
					if r := recover(); r != nil {
						dm.logger.Error("[DATA_MONITOR] Panic in ticker: %v", r)
					}
				}()
				if dm.dbDriver == nil {
					dm.logger.Error("[DATA_MONITOR] dbDriver is nil")
					continue
				}
				pairs, err := dm.dbDriver.GetActivePairsForDataMonitor()
				if err != nil {
					dm.logger.Error("[DATA_MONITOR] SQL error: %v", err)
					continue
				}
				dm.UpdateWorkersFromPairs(pairs)
			}
		}
	}()
}

// Stop останавливает все DataWorker
func (dm *DataMonitor) Stop() {
	dm.logger.Info("[DATA_MONITOR] Stopping all data workers...")
	dm.workersMutex.Lock()
	for k, w := range dm.workers {
		w.Stop()
		dm.logger.Info("[DATA_MONITOR] Stopped worker %s", k)
		delete(dm.workers, k)
	}
	dm.workersMutex.Unlock()
	dm.wg.Wait()
	close(dm.stopChan)
	dm.logger.Info("[DATA_MONITOR] All data workers stopped")
}

// UpdateWorkers обновляет список активных DataWorker по рынкам/биржам
func (dm *DataMonitor) UpdateWorkers(activeMarkets map[string][]string) {
	// TODO: реализовать динамическое добавление/удаление воркеров
}

// UpdateWorkersFromPairs управляет воркерами на основе данных из БД
func (dm *DataMonitor) UpdateWorkersFromPairs(pairs []db.DataMonitorPair) {
	dm.logger.Debug("[DATA_MONITOR] UpdateWorkersFromPairs called, pairs count: %d", len(pairs))
	dm.workersMutex.Lock()
	defer dm.workersMutex.Unlock()

	actual := make(map[string]struct{})
	// Собираем пары для каждой биржи и рынка
	exchangePairs := make(map[string]map[string][]exchange.MarketPair) // exchangeName -> marketType -> []MarketPair

	// Сначала группируем все пары
	for _, p := range pairs {
		key := fmt.Sprintf("%s|%s", p.ExchangeName, p.MarketType)
		dm.logger.Debug("[DATA_MONITOR] Checking pair: exchange=%s, symbol=%s, pair_id=%d, market_type=%s (key=%s)", p.ExchangeName, p.Symbol, p.PairID, p.MarketType, key)
		actual[key] = struct{}{}

		if _, ok := exchangePairs[p.ExchangeName]; !ok {
			exchangePairs[p.ExchangeName] = make(map[string][]exchange.MarketPair)
		}

		// Используем MarketPair с PairID
		marketPair := exchange.MarketPair{
			Symbol: p.Symbol,
			PairID: p.PairID,
		}
		exchangePairs[p.ExchangeName][p.MarketType] = append(exchangePairs[p.ExchangeName][p.MarketType], marketPair)
	}

	// Теперь управляем воркерами
	for exchangeName, marketTypes := range exchangePairs {
		for marketType, marketPairs := range marketTypes {
			key := fmt.Sprintf("%s|%s", exchangeName, marketType)

			if _, exists := dm.workers[key]; !exists {
				dm.logger.Info("[DATA_MONITOR] Creating new DataWorker for %s", key)
				// Получаем Exchange по имени
				ex, err := dm.getExchangeByName(exchangeName)
				if err != nil {
					dm.logger.Error("[DATA_MONITOR] Error getting exchange %s: %v", exchangeName, err)
					dm.incErrors()
					continue
				}
				worker := dataworker.NewDataWorker(*ex)
				// Передать параметры подписки с PairID
				worker.SetSubscriptionWithPairID(marketPairs, marketType, 5)
				dm.workers[key] = worker
				go func(w *dataworker.DataWorker, k string) {
					dm.logger.Debug("[DATA_MONITOR] Starting DataWorker goroutine for %s", k)
					if err := w.Start(); err != nil {
						dm.logger.Error("[DATA_MONITOR] Worker start error for %s: %v", k, err)
						dm.incErrors()
					}
				}(worker, key)
				dm.totalStarted++
				dm.logger.Info("[DATA_MONITOR] Started worker for %s", key)
			} else {
				// Если воркер уже есть — обновить параметры подписки
				dm.workers[key].SetSubscriptionWithPairID(marketPairs, marketType, 5)
			}
		}
	}

	// Остановить и удалить неактуальных воркеров
	for key, w := range dm.workers {
		if _, stillActive := actual[key]; !stillActive {
			dm.logger.Info("[DATA_MONITOR] Stopping and removing DataWorker for %s (no longer active)", key)
			if err := w.Stop(); err != nil {
				dm.logger.Error("[DATA_MONITOR] Worker stop error for %s: %v", key, err)
				dm.incErrors()
			}
			delete(dm.workers, key)
			dm.totalStopped++
			dm.logger.Info("[DATA_MONITOR] Stopped worker for %s", key)
		}
	}
	dm.logger.Debug("[DATA_MONITOR] UpdateWorkersFromPairs completed. Active workers: %d", len(dm.workers))
}

// Методы для работы с метриками
func (dm *DataMonitor) incErrors() {
	dm.totalErrors++
}

func (dm *DataMonitor) Metrics() (active, started, stopped, errors int) {
	dm.workersMutex.Lock()
	defer dm.workersMutex.Unlock()
	return len(dm.workers), dm.totalStarted, dm.totalStopped, dm.totalErrors
}

// getExchangeByName получает Exchange по имени через драйвер БД
func (dm *DataMonitor) getExchangeByName(name string) (*db.Exchange, error) {
	driver, ok := dm.getDBDriver()
	if !ok {
		return nil, fmt.Errorf("DB driver not available")
	}
	return driver.GetExchangeByName(name)
}

// getDBDriver возвращает DBDriver, если возможно (заглушка, доработать под вашу архитектуру)
func (dm *DataMonitor) getDBDriver() (db.DBDriver, bool) {
	if dm.dbDriver != nil {
		return dm.dbDriver, true
	}
	return nil, false
}
