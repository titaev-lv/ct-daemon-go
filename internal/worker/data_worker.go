package worker

import (
	"daemon-go/pkg/log"
	"sync"
	"time"
)

// DataWorker собирает данные по (EXCHANGE_ID, MARKET)
type DataWorker struct {
	exchangeID string
	market     string
	marketType string
	logger     *log.Logger
	stopChan   chan struct{}
	wg         *sync.WaitGroup
}

func NewDataWorker(exchangeID, market, marketType string, logger *log.Logger, wg *sync.WaitGroup) *DataWorker {
	return &DataWorker{
		exchangeID: exchangeID,
		market:     market,
		marketType: marketType,
		logger:     logger,
		stopChan:   make(chan struct{}),
		wg:         wg,
	}
}

// Start запускает сбор данных (order book/best price)
func (dw *DataWorker) Start() error {
	if dw.wg != nil {
		dw.wg.Add(1)
		defer dw.wg.Done()
	}
	dw.logger.Info("[DATA_WORKER] Started for %s:%s [%s]", dw.exchangeID, dw.market, dw.marketType)
	defer func() {
		if r := recover(); r != nil {
			dw.logger.Error("[DATA_WORKER] Panic in Start: %v", r)
		}
	}()

	// Пример: имитируем работу с таймером и каналом остановки
	workTicker := time.NewTicker(2 * time.Second)
	defer workTicker.Stop()

	for {
		select {
		case <-dw.stopChan:
			dw.logger.Info("[DATA_WORKER] Stop signal received for %s:%s [%s]", dw.exchangeID, dw.market, dw.marketType)
			return nil
		case <-workTicker.C:
			// Здесь основная логика сбора данных
			dw.logger.Debug("[DATA_WORKER] Tick for %s:%s [%s]", dw.exchangeID, dw.market, dw.marketType)
		}
	}
}

// Stop завершает работу DataWorker
func (dw *DataWorker) Stop() error {
	dw.logger.Info("[DATA_WORKER] Stopping for %s:%s [%s]", dw.exchangeID, dw.market, dw.marketType)
	defer func() {
		if r := recover(); r != nil {
			dw.logger.Error("[DATA_WORKER] Panic on stop: %v", r)
		}
	}()
	select {
	case <-dw.stopChan:
		// Уже закрыт
	default:
		close(dw.stopChan)
	}
	return nil
}
