package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"daemon-go/internal/db"
	"daemon-go/internal/worker"
	"daemon-go/pkg/log"
)

// ServerConfig задаёт конфигурацию HTTP-сервера
type ServerConfig struct {
	Port       int
	ConfigPath string
}

// Server описывает HTTP API сервера
// Добавить ссылку на DataMonitor
type Server struct {
	cfg            ServerConfig
	driver         db.DBDriver
	traderWorkers  map[int]*worker.TraderWorker
	workersMutex   *sync.Mutex
	stopChan       chan struct{}
	reloadConfig   func(path string) error
	startWork      func() error
	stopWork       func()
	logger         *log.Logger
	getDataMonitor func() *worker.DataMonitor // изменено на функцию getter
}

// NewServer создаёт новый API-сервер
func NewServer(cfg ServerConfig, driver db.DBDriver, traderWorkers map[int]*worker.TraderWorker, workersMutex *sync.Mutex, stopChan chan struct{}, reloadConfig func(string) error, startWork func() error, stopWork func(), getDataMonitor func() *worker.DataMonitor) *Server {
	logger := log.New("api")
	return &Server{
		cfg:            cfg,
		driver:         driver,
		traderWorkers:  traderWorkers,
		workersMutex:   workersMutex,
		stopChan:       stopChan,
		reloadConfig:   reloadConfig,
		startWork:      startWork,
		stopWork:       stopWork,
		logger:         logger,
		getDataMonitor: getDataMonitor,
	}
}

// Start запускает HTTP-сервер
func (s *Server) Start() {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		s.logger.Debug("[API][DEBUG] /status request from %s", r.RemoteAddr)
		s.handleStatus(w, r)
	})
	http.HandleFunc("/daemon", func(w http.ResponseWriter, r *http.Request) {
		s.logger.Debug("[API][DEBUG] /daemon request from %s, params: %v", r.RemoteAddr, r.URL.Query())
		s.handleDaemon(w, r)
	})
	addr := fmt.Sprintf("0.0.0.0:%d", s.cfg.Port)
	s.logger.Info("API Server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		s.logger.Fatal("HTTP server error: %v", err)
	}
}

// handleStatus возвращает статус всех трейдеров и воркеров
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	status := make(map[string]interface{})

	// Состояние БД
	if err := s.driver.Ping(); err != nil {
		s.logger.Debug("[API][DEBUG] DB ping error: %v", err)
		status["db_status"] = "DISCONNECTED"
	} else {
		status["db_status"] = "CONNECTED"
	}

	// Состояние воркеров
	s.workersMutex.Lock()
	traders := make([]map[string]interface{}, 0, len(s.traderWorkers))
	activeCount := 0
	for id, w := range s.traderWorkers {
		isActive := w.IsActive()
		s.logger.Debug("[API][DEBUG] TraderWorker id=%d active=%v", id, isActive)
		if isActive {
			activeCount++
		}
		traders = append(traders, map[string]interface{}{
			"id":     id,
			"active": isActive,
		})
	}
	s.workersMutex.Unlock()
	status["trader_workers"] = traders

	// Метрики DataMonitor
	dataMonitor := s.getDataMonitor()
	if dataMonitor != nil {
		active, started, stopped, errors := dataMonitor.Metrics()
		status["data_monitor"] = map[string]interface{}{
			"active_workers": active,
			"total_started":  started,
			"total_stopped":  stopped,
			"total_errors":   errors,
			"uptime":         dataMonitor.GetUptime(),
			"uptime_string":  dataMonitor.GetUptimeString(),
			"start_time":     dataMonitor.GetStartTime().Unix(),
		}
		s.logger.Debug("[API][DEBUG] DataMonitor metrics: active=%d started=%d stopped=%d errors=%d uptime=%ds", active, started, stopped, errors, dataMonitor.GetUptime())

		// Детальная информация о data workers
		workersInfo := dataMonitor.GetWorkersInfo()
		status["data_workers"] = workersInfo
		s.logger.Debug("[API][DEBUG] DataWorkers info: %d workers", len(workersInfo))
	} // Статус демона: RUNNING если есть активные воркеры, иначе STOPPED
	if activeCount > 0 {
		status["daemon_status"] = "RUNNING"
	} else {
		status["daemon_status"] = "STOPPED"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.logger.Debug("[API][DEBUG] JSON encode error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDaemon управляет демоном (stop/reload)
func (s *Server) handleDaemon(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	s.logger.Debug("[API][DEBUG] handleDaemon action=%s", action)
	switch action {
	case "stop":
		s.logger.Debug("[API][DEBUG] Stopping business logic via API (stopWork)")
		if s.stopWork != nil {
			go s.stopWork()
			fmt.Fprintln(w, "Daemon stopped")
		} else {
			fmt.Fprintln(w, "StopWork not supported")
		}
	case "shutdown":
		s.logger.Debug("[API][DEBUG] Sending shutdown signal to daemon")
		go func() { s.stopChan <- struct{}{} }()
		fmt.Fprintln(w, "Daemon shutting down...")
	case "reload":
		s.logger.Debug("[API][DEBUG] Reloading config via API")
		if err := s.reloadConfig(s.cfg.ConfigPath); err != nil {
			s.logger.Debug("[API][DEBUG] Reload config error: %v", err)
			fmt.Fprintf(w, "Failed to reload config: %v\n", err)
		} else {
			fmt.Fprintln(w, "Config reloaded")
		}
	case "start":
		s.logger.Debug("[API][DEBUG] Starting workers via API")
		if s.startWork != nil {
			err := s.startWork()
			if err != nil {
				fmt.Fprintln(w, "Error: Daemon already started")
			} else {
				fmt.Fprintln(w, "Daemon started")
			}
		} else {
			fmt.Fprintln(w, "Start not supported")
		}
	case "stopwork":
		s.logger.Debug("[API][DEBUG] Stopping workers via API")
		if s.stopWork != nil {
			go s.stopWork()
			fmt.Fprintln(w, "Daemon stopped")
		} else {
			fmt.Fprintln(w, "StopWork not supported")
		}
	default:
		s.logger.Debug("[API][DEBUG] Unknown action: %s", action)
		fmt.Fprintln(w, "Unknown action")
	}
}
