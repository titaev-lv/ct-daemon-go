package worker

import (
	"time"

	"daemon-go/internal/db"
	"daemon-go/pkg/log"
)

type TraderWorker struct {
	Trade  db.TradeCase
	DB     db.DBDriver
	active bool
	stop   chan struct{}
	Logger *log.Logger
}

// NewTraderWorker создает нового трейдер-воркера
// logger может быть nil — тогда используется log.New("trader")
func NewTraderWorker(trade db.TradeCase, driver db.DBDriver, logger *log.Logger) *TraderWorker {
	if logger == nil {
		logger = log.New("trader")
	}
	return &TraderWorker{
		Trade:  trade,
		DB:     driver,
		active: false,
		stop:   make(chan struct{}),
		Logger: logger,
	}
}

// Start запускает воркер
func (tw *TraderWorker) Start() {
	tw.active = true
	tw.Logger.Info("TraderWorker %d started", tw.Trade.ID)
	defer func() {
		tw.active = false
		tw.Logger.Info("TraderWorker %d stopped", tw.Trade.ID)
	}()

	ticker := time.NewTicker(5 * time.Second) // пример работы воркера
	defer ticker.Stop()

	<-tw.stop
}

// Stop останавливает воркер
func (tw *TraderWorker) Stop() {
	if tw.active {
		close(tw.stop)
		tw.active = false
	}
}

// IsActive возвращает состояние воркера
func (tw *TraderWorker) IsActive() bool {
	return tw.active
}
