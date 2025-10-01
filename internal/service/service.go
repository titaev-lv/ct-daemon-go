package service

import (
	"context"
	"sync"
	"time"

	"daemon-go/internal/db"
	"daemon-go/pkg/log"
)

// Daemon представляет сервис, который управляет задачами системы
type Daemon struct {
	DB     db.DBDriver
	logger *log.Logger
	mu     sync.Mutex
	// Здесь можно добавить дополнительные поля для состояния сервисных воркеров
	// например: collectors, executors, monitors
}

// NewDaemon создает новый экземпляр сервиса
func NewDaemon(driver db.DBDriver) *Daemon {
	return &Daemon{
		DB:     driver,
		logger: log.New("service"),
	}
}

// Run запускает сервисные задачи и завершает работу при получении ctx.Done()
func (d *Daemon) Run(ctx context.Context) {
	d.logger.Info("Service Daemon running")
	ticker := time.NewTicker(10 * time.Second) // интервал выполнения задач
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("Service Daemon stopping")
			d.cleanup()
			return
		case <-ticker.C:
			d.performTasks()
		}
	}
}

// performTasks выполняет периодические задачи сервиса
func (d *Daemon) performTasks() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Info("Service Daemon performing periodic tasks")
	// Примеры сервисных задач:
	// - обновление комиссий по торговым парам
	// - обновление списка доступных торговых пар для бирж
	// - очистка устаревших данных в кэше
	// - проверка состояния подключений к биржам
	// Реализацию можно дополнять по мере необходимости
}

// cleanup выполняет корректное завершение сервисных воркеров
func (d *Daemon) cleanup() {
	d.logger.Info("Service Daemon cleaning up resources")
	// Здесь можно завершить дополнительные воркеры, закрыть соединения и т.д.
}

// AddWorker/RemoveWorker можно добавить позже для управления воркерами сервиса
