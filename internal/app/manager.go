package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"daemon-go/internal/api"
	"daemon-go/internal/config"
	"daemon-go/internal/db"
	"daemon-go/internal/service"
	"daemon-go/internal/state"
	"daemon-go/internal/worker"
	"daemon-go/pkg/log"
)

type Manager struct {
	cfg           *config.Config
	db            db.DBDriver
	logger        *log.Logger
	apiServer     *api.Server
	serviceDaemon *service.Daemon
	tradeMonitor  *worker.TradeMonitor
	dataMonitor   *worker.DataMonitor
	priceMonitor  *worker.PriceMonitor
	traderWorkers map[int]*worker.TraderWorker
	workersMutex  sync.Mutex
	stopChan      chan struct{}
	stopOnce      sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
	workStarted   bool // singleton-флаг
}

func NewManager(cfg *config.Config, dbDriver db.DBDriver, logger *log.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cfg:           cfg,
		db:            dbDriver,
		logger:        logger,
		traderWorkers: make(map[int]*worker.TraderWorker),
		stopChan:      make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// ReloadConfig загружает и применяет новый конфиг
func (m *Manager) ReloadConfig(path string) error {
	newCfg, err := config.LoadConfig(path)
	if err != nil {
		m.logger.Error("[RELOAD] Failed to load config: %v", err)
		return err
	}
	if err := config.Validate(newCfg); err != nil {
		m.logger.Error("[RELOAD] Config validation failed: %v", err)
		return err
	}
	m.cfg = newCfg
	// Hot-reload log level
	if lvl, err := log.ParseLevel(m.cfg.Logging.Level); err == nil {
		m.logger.SetLevel(lvl)
		m.logger.Info("[RELOAD] Log level set to %s", lvl.String())
	} else {
		m.logger.Warn("[RELOAD] Invalid log level in config: %s, using previous", m.cfg.Logging.Level)
	}
	m.logger.Info("[RELOAD] Config reloaded from %s", path)
	// Можно добавить hot-reload pollInterval/logLevel и т.д.
	return nil
}

// Start инициализирует API и сервисы, но НЕ запускает воркеры/монитор до команды start
func (m *Manager) Start() {
	m.logger.Info("[START] Initializing manager (API only, no workers/services)...")

	// API
	apiCfg := api.ServerConfig{
		Port:       m.cfg.Daemon.HttpPort,
		ConfigPath: "config/config.conf",
	}
	reloadConfig := func(path string) error {
		return m.ReloadConfig(path)
	}
	// Передаем методы управления воркерами в API через замыкания
	startWork := func() error { return m.StartWork() }
	stopWork := func() { m.StopWork() }
	getDataMonitor := func() *worker.DataMonitor { return m.dataMonitor }
	m.logger.Debug("[START][DEBUG] Creating API server with config: %+v", apiCfg)
	m.logger.Info("[START] Initializing API server on :%d", apiCfg.Port)
	m.apiServer = api.NewServer(apiCfg, m.db, m.traderWorkers, &m.workersMutex, m.stopChan, reloadConfig, startWork, stopWork, getDataMonitor)
	go func() {
		m.logger.Debug("[START][DEBUG] API server goroutine about to start")
		m.logger.Info("[START] API server goroutine started")
		m.apiServer.Start()
	}()

	m.logger.Info("[START] Manager initialized: API and service daemon running, waiting for start command...")
}

// StartWork запускает бизнес-логику: TradeMonitor и трейдер-воркеры
func (m *Manager) StartWork() error {
	if m.workStarted {
		m.logger.Warn("[WORK] Daemon already started")
		return fmt.Errorf("daemon already started")
	}
	m.workStarted = true
	// Сохраняем состояние: Active=true
	state.SetActive(m.cfg.Daemon.StateFile, true)
	m.logger.Info("[WORK] Starting ServiceDaemon...")
	m.logger.Debug("[WORK][DEBUG] Creating ServiceDaemon with db=%T", m.db)
	m.serviceDaemon = service.NewDaemon(m.db)
	go func() {
		m.logger.Debug("[WORK][DEBUG] ServiceDaemon goroutine about to start")
		m.logger.Info("[WORK] ServiceDaemon goroutine started")
		m.serviceDaemon.Run(m.ctx)
	}()

	m.logger.Info("[WORK] Starting TradeMonitor, DataMonitor и trader workers...")
	// TradeMonitor
	m.logger.Info("[WORK] Initializing TradeMonitor (pollInterval=%d)", m.cfg.Daemon.PollInterval)
	m.logger.Debug("[WORK][DEBUG] Creating TradeMonitor with db=%T, traderWorkers=%d", m.db, len(m.traderWorkers))
	m.tradeMonitor = worker.NewTradeMonitor(m.db, &m.traderWorkers, &m.workersMutex, m.stopChan, m.cfg.Daemon.PollInterval)
	go func() {
		m.logger.Debug("[WORK][DEBUG] TradeMonitor goroutine about to start")
		m.logger.Info("[WORK] TradeMonitor goroutine started")
		m.tradeMonitor.Start()
	}()
	// DataMonitor
	m.logger.Info("[WORK] Initializing DataMonitor...")
	m.dataMonitor = worker.NewDataMonitor(m.logger, m.db)
	go func() {
		m.logger.Debug("[WORK][DEBUG] DataMonitor goroutine about to start")
		m.logger.Info("[WORK] DataMonitor goroutine started")
		m.dataMonitor.Start() // больше не нужно передавать драйвер
	}()

	// PriceMonitor
	m.logger.Info("[WORK] Initializing PriceMonitor (interval=15s)...")
	m.priceMonitor = worker.NewPriceMonitor(m.db, 500*time.Millisecond)
	go func() {
		m.logger.Debug("[WORK][DEBUG] PriceMonitor goroutine about to start")
		m.logger.Info("[WORK] PriceMonitor goroutine started")
		if err := m.priceMonitor.Start(); err != nil {
			m.logger.Error("Failed to start PriceMonitor: %v", err)
		}
	}()

	m.logger.Info("[WORK] ServiceDaemon, TradeMonitor, DataMonitor, PriceMonitor и workers started")
	return nil
}

// StopWork останавливает TradeMonitor и всех трейдер-воркеров
func (m *Manager) StopWork() {
	if !m.workStarted {
		m.logger.Warn("[WORK] Daemon not running")
		return
	}
	m.workStarted = false
	// Сохраняем состояние: Active=false
	state.SetActive(m.cfg.Daemon.StateFile, false)
	m.logger.Info("[WORK] Stopping ServiceDaemon, TradeMonitor, DataMonitor, PriceMonitor и всех trader workers...")
	// Остановить сервис-демон (через cancel контекста)
	if m.cancel != nil {
		m.logger.Info("[WORK] Stopping ServiceDaemon...")
		m.cancel()
		// Пересоздать контекст для возможности повторного запуска
		m.ctx, m.cancel = context.WithCancel(context.Background())
	}
	// Остановить TradeMonitor, DataMonitor и воркеры
	if m.tradeMonitor != nil {
		m.logger.Info("[WORK] Stopping TradeMonitor...")
		m.stopChan <- struct{}{}
		m.tradeMonitor = nil
	}
	if m.dataMonitor != nil {
		m.logger.Info("[WORK] Stopping DataMonitor...")
		m.dataMonitor.Stop()
		m.dataMonitor = nil
	}
	if m.priceMonitor != nil {
		m.logger.Info("[WORK] Stopping PriceMonitor...")
		m.priceMonitor.Stop()
		m.priceMonitor = nil
	}
	m.workersMutex.Lock()
	for id, w := range m.traderWorkers {
		if w != nil {
			m.logger.Info("[WORK] Stopping trader worker id=%d", id)
			w.Stop()
		}
		delete(m.traderWorkers, id)
	}
	m.workersMutex.Unlock()
	m.logger.Info("[WORK] ServiceDaemon, TradeMonitor и все trader workers остановлены")
}

func (m *Manager) Stop() {
	m.logger.Warn("[STOP] Shutdown signal received, stopping manager...")
	m.cancel()
	m.stopOnce.Do(func() { close(m.stopChan) })

	m.logger.Debug("[STOP][DEBUG] Manager state before shutdown: traderWorkers=%d", len(m.traderWorkers))
	m.logger.Info("[STOP] Stopping all trader workers...")
	m.workersMutex.Lock()
	for id, w := range m.traderWorkers {
		if w != nil {
			m.logger.Debug("[STOP][DEBUG] Stopping trader worker struct: %+v", w)
			m.logger.Info("[STOP] Stopping trader worker id=%d", id)
			w.Stop()
		}
		delete(m.traderWorkers, id)
	}
	m.workersMutex.Unlock()

	wait := 3 * time.Second
	m.logger.Info("[STOP] Waiting %s for graceful shutdown...", wait)
	time.Sleep(wait)

	m.logger.Info("[STOP] Closing database connection...")
	m.logger.Debug("[STOP][DEBUG] DB driver type: %T", m.db)
	if err := m.db.Close(); err != nil {
		m.logger.Error("[STOP] Error closing DB: %v", err)
	} else {
		m.logger.Info("[STOP] DB closed")
	}

	m.logger.Info("[STOP] Manager stopped gracefully")
}
