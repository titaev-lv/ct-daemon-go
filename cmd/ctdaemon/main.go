package main

import (
	"database/sql"
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"daemon-go/internal/app"
	"daemon-go/internal/bus"
	"daemon-go/internal/config"
	"daemon-go/internal/db"
	"daemon-go/internal/exchange"
	"daemon-go/internal/market"
	"daemon-go/internal/state"
	"daemon-go/pkg/log"
)

func testHTXAdapter() {
	// Настройка логирования
	stdlog.SetFlags(stdlog.LstdFlags | stdlog.Lshortfile)

	// Настройка конфигурации для отладки
	debugConfig := &config.Config{
		OrderBook: struct {
			DebugLogRaw bool
			DebugLogMsg bool
		}{
			DebugLogRaw: true,
			DebugLogMsg: true,
		},
	}
	exchange.SetOrderBookConfig(debugConfig)

	// Инициализация message bus
	messageBus := bus.GetInstance()

	// Подписка на сообщения для тестирования
	messageChannel := messageBus.Subscribe("htx", 100)

	// Горутина для обработки сообщений
	go func() {
		for msg := range messageChannel {
			switch msg.MessageType {
			case market.MessageTypeOrderBook:
				if orderBook, ok := msg.Data.(*market.UnifiedOrderBook); ok {
					fmt.Printf("📊 ORDER BOOK [%s] %s - %s: bids=%d, asks=%d\n",
						msg.Exchange, msg.Symbol, orderBook.UpdateType,
						len(orderBook.Bids), len(orderBook.Asks))
				}

			case market.MessageTypeTicker:
				if ticker, ok := msg.Data.(*market.UnifiedTicker); ok {
					fmt.Printf("💰 TICKER [%s] %s: price=%.8f, volume=%.2f\n",
						msg.Exchange, msg.Symbol, ticker.LastPrice, ticker.Volume24h)
				}

			default:
				fmt.Printf("📢 Получено сообщение: %s %s %s\n", msg.Exchange, msg.Symbol, msg.MessageType)
			}
		}
	}()

	// Создание объекта Exchange для адаптера HTX
	exchangeConfig := db.Exchange{
		ID:           1,
		Name:         "HTX",
		Active:       true,
		Url:          "https://api.huobi.pro",
		BaseUrl:      "https://api.huobi.pro",
		WebsocketUrl: sql.NullString{String: "wss://api.huobi.pro/ws", Valid: true},
		WsUrl:        sql.NullString{String: "wss://api.huobi.pro/ws", Valid: true},
	}

	// Создание адаптера HTX
	adapter := exchange.NewHtxAdapter(exchangeConfig)

	// Добавляем символы для тестирования
	testSymbols := []string{
		"btcusdt", // Bitcoin - HTX использует lowercase
		"ethusdt", // Ethereum
		"xrpusdt", // Ripple
	}

	fmt.Printf("🚀 Запуск тестирования адаптера HTX...\n")
	fmt.Printf("📈 Символы для тестирования: %s\n", strings.Join(testSymbols, ", "))
	fmt.Printf("⚙️  Настройки логирования: debug_log_raw=%v, debug_log_msg=%v\n",
		debugConfig.OrderBook.DebugLogRaw, debugConfig.OrderBook.DebugLogMsg)
	fmt.Printf("🔔 ВАЖНО: HTX сама шлет ping'и, WriteLoop не запускается!\n")

	// Запуск адаптера
	go func() {
		if err := adapter.Start(); err != nil {
			fmt.Printf("❌ Ошибка запуска адаптера: %v\n", err)
		}
	}()

	// Даем время на подключение
	time.Sleep(3 * time.Second)

	// Подписка на символы
	fmt.Printf("📡 Подписка на символы...\n")
	for _, symbol := range testSymbols {
		if err := adapter.SubscribeMarkets([]string{symbol}, "spot", 0); err != nil {
			fmt.Printf("❌ Ошибка подписки на %s: %v\n", symbol, err)
		} else {
			fmt.Printf("✅ Подписка на %s выполнена\n", symbol)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Тестирование отписки через 30 секунд
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Printf("📡 Тестирование отписки от символов...\n")

		for _, symbol := range testSymbols[:1] { // Отписываемся только от первого символа
			if err := adapter.UnsubscribeMarkets([]string{symbol}, "spot", 0); err != nil {
				fmt.Printf("❌ Ошибка отписки от %s: %v\n", symbol, err)
			} else {
				fmt.Printf("🔇 Отписка от %s выполнена\n", symbol)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("🏃 Адаптер HTX запущен. Нажмите Ctrl+C для остановки...\n")
	fmt.Printf("📊 Ожидаем данные order book и ticker...\n")

	// Ожидание сигнала завершения
	<-sigChan

	fmt.Printf("🛑 Получен сигнал завершения. Остановка адаптера...\n")

	// Даем время на graceful shutdown
	time.Sleep(2 * time.Second)
	fmt.Printf("✅ Тестирование завершено\n")
}

func handleStartCommand() {
	// Проверяем, не запущен ли уже daemon
	stateFile := "state/daemon.state"
	daemonState := state.LoadState(stateFile)
	if daemonState.Active {
		fmt.Printf("Daemon is already running\n")
		return
	}

	// Запускаем daemon как основной процесс
	fmt.Printf("Starting daemon...\n")

	// Запускаем основную логику daemon (состояние установится внутри)
	mainDaemonWithAutoStart()
}

func handleStopCommand() {
	const cfgPath = "config/config.conf"

	// Загружаем конфиг для получения StateFile
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// Проверяем состояние daemon
	daemonState := state.LoadState(cfg.Daemon.StateFile)
	if !daemonState.Active {
		fmt.Printf("Daemon is not running\n")
		return
	}

	// Ищем процесс daemon
	process, err := findDaemonProcess()
	if err != nil {
		fmt.Printf("Failed to find daemon process: %v\n", err)
		return
	}

	if process == nil {
		fmt.Printf("Daemon process not found\n")
		// Сбрасываем состояние
		daemonState.Active = false
		state.SaveState(cfg.Daemon.StateFile, daemonState)
		return
	}

	// Отправляем SIGTERM процессу
	fmt.Printf("Stopping daemon (PID: %d)...\n", process.Pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Failed to send SIGTERM: %v\n", err)
		return
	}

	// Ждем завершения процесса
	time.Sleep(2 * time.Second)

	// Проверяем, завершился ли процесс
	if isProcessRunning(process.Pid) {
		fmt.Printf("Process still running, sending SIGKILL...\n")
		process.Signal(syscall.SIGKILL)
		time.Sleep(1 * time.Second)
	}

	// Сбрасываем состояние
	daemonState.Active = false
	state.SaveState(cfg.Daemon.StateFile, daemonState)

	fmt.Printf("Daemon stopped\n")
}

func handleStatusCommand() {
	const cfgPath = "config/config.conf"

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	daemonState := state.LoadState(cfg.Daemon.StateFile)
	process, _ := findDaemonProcess()

	if daemonState.Active && process != nil {
		fmt.Printf("Daemon is running (PID: %d)\n", process.Pid)
	} else if daemonState.Active && process == nil {
		fmt.Printf("Daemon state is active but process not found (stale state)\n")
	} else {
		fmt.Printf("Daemon is not running\n")
	}
}

func findDaemonProcess() (*os.Process, error) {
	// Читаем список процессов
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, proc := range processes {
		if !proc.IsDir() {
			continue
		}

		// Проверяем, что это PID
		pid, err := strconv.Atoi(proc.Name())
		if err != nil {
			continue
		}

		// Читаем cmdline
		cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}

		cmdline := string(cmdlineBytes)
		// Ищем наш daemon (daemon-go или ./daemon-go)
		if strings.Contains(cmdline, "daemon-go") && strings.Contains(cmdline, "start") {
			return os.FindProcess(pid)
		}
	}

	return nil, nil
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func mainDaemon() {
	const cfgPath = "config/config.conf"

	// Минимальный logger для ошибок до парса конфига
	preLogger := log.New("preinit")

	fmt.Printf("[LOG][DEBUG] Loading config from %s\n", cfgPath)
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		preLogger.Error("[DEBUG] Failed to load config: %v", err)
		preLogger.Fatal("failed to load config (%s): %v", cfgPath, err)
	}
	fmt.Printf("[LOG][DEBUG] Config loaded: %+v\n", config.GetConfigForLogging(cfg))
	if err := config.Validate(cfg); err != nil {
		preLogger.Error("[DEBUG] Config validation failed: %v", err)
		preLogger.Fatal("invalid config: %v", err)
	}
	fmt.Printf("[LOG][DEBUG] Config validated successfully\n")

	// Установить конфигурацию для OrderBook логирования
	exchange.SetOrderBookConfig(cfg)
	fmt.Printf("[LOG][DEBUG] OrderBook config set: DebugLogRaw=%t, DebugLogMsg=%t\n",
		cfg.OrderBook.DebugLogRaw, cfg.OrderBook.DebugLogMsg)

	// Выбор режима логирования: global или modular
	if cfg.Logging.Mode == "modular" {
		log.SetGlobalMode(false)
		fmt.Printf("[LOG][DEBUG] Modular logging mode enabled\n")
	} else {
		log.SetGlobalMode(true)
		fmt.Printf("[LOG][DEBUG] Global logging mode enabled\n")
	}

	// Инициализация глобального лог-файла (для global-режима)
	if cfg.Logging.Mode == "global" {
		if err := log.Init(cfg.Logging.File); err != nil {
			fmt.Printf("[LOG][ERROR] Failed to init log file: %v\n", err)
		} else {
			fmt.Printf("[LOG][DEBUG] Log file initialized: %s\n", cfg.Logging.File)
		}
	}

	// Установить уровень логирования из конфига
	if lvl, err := log.ParseLevel(cfg.Logging.Level); err == nil {
		log.SetGlobalLevel(lvl)
		fmt.Printf("[LOG][DEBUG] Log level set to %s\n", lvl.String())
	} else {
		fmt.Printf("[LOG][ERROR] Invalid log level in config: %s\n", cfg.Logging.Level)
	}

	// Создать основной логгер (daemon)
	var logger *log.Logger
	if cfg.Logging.Mode == "modular" && cfg.Logging.Dir != "" {
		logPath := cfg.Logging.Dir + "/daemon.log"
		l, err := log.NewWithFile("daemon", logPath)
		if err != nil {
			fmt.Printf("[LOG][ERROR] Failed to create modular logger: %v\n", err)
			logger = log.New("daemon")
		} else {
			logger = l
			fmt.Printf("[LOG][DEBUG] Modular logger created: %s\n", logPath)
		}
	} else {
		logger = log.New("daemon")
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic: %v", r)
			log.Close()
			os.Exit(2)
		}
		log.Close()
	}()

	logger.Info("Daemon starting...")
	logger.Debug("[DEBUG] Process PID: %d", os.Getpid())
	logger.Debug("[DEBUG] Config: %+v", cfg)
	// ...existing code up to the end of the state restore block...

	// ...logger.Info, logger.Debug, etc. уже вызваны выше...

	// Создаём DB драйвер
	dbCfg := map[string]string{
		"host":     cfg.Database.Host,
		"port":     strconv.Itoa(cfg.Database.Port),
		"user":     cfg.Database.User,
		"password": cfg.Database.Password,
		"database": cfg.Database.Database,
	}
	logger.Debug("[DEBUG] Creating DB driver: type=%s, cfg=%+v", cfg.Database.Type, dbCfg)
	driver, err := db.NewDriver(cfg.Database.Type, dbCfg)
	if err != nil {
		logger.Error("[DEBUG] Failed to create DB driver: %v", err)
		logger.Fatal("failed to create db driver: %v", err)
	}

	logger.Debug("[DEBUG] Connecting to DB with retry...")
	maxAttempts := 10
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := driver.Connect(); err != nil {
			lastErr = err
			logger.Warn("[DB] Connect attempt %d/%d failed: %v", attempt, maxAttempts, err)
			time.Sleep(2 * time.Second)
		} else {
			logger.Info("DB connected (attempt %d)", attempt)
			lastErr = nil
			break
		}
	}
	if lastErr != nil {
		logger.Error("[DB] All connect attempts failed: %v", lastErr)
		logger.Fatal("db connect failed: %v", lastErr)
	}

	logger.Debug("[DEBUG] Initializing Manager...")
	manager := app.NewManager(cfg, driver, logger)
	logger.Debug("[DEBUG] Manager initialized: %+v", manager)
	logger.Debug("[DEBUG] Starting Manager components...")
	manager.Start()
	logger.Debug("[DEBUG] Manager started")

	// Восстанавливаем состояние работы из файла
	logger.Debug("[DEBUG] Checking daemon state from %s", cfg.Daemon.StateFile)
	daemonState := state.LoadState(cfg.Daemon.StateFile)
	logger.Debug("[DEBUG] Loaded daemon state: active=%v", daemonState.Active)
	if daemonState.Active {
		logger.Info("Restoring active daemon state, starting work...")
		if err := manager.StartWork(); err != nil {
			logger.Error("Failed to start work during state restore: %v", err)
		} else {
			logger.Info("Work started successfully during state restore")
		}
	} else {
		logger.Info("Daemon state is inactive, waiting for start command")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM /*, syscall.SIGHUP*/)
	logger.Debug("[DEBUG] Signal handler registered, waiting for signals...")
	for {
		sig := <-sigChan
		logger.Debug("[DEBUG] Signal received: %v", sig)
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Debug("[DEBUG] Initiating graceful shutdown...")
			if err := safeStop(manager, logger); err != nil {
				logger.Error("error during shutdown: %v", err)
			}
			logger.Info("Daemon stopped gracefully")
			return
			// case syscall.SIGHUP:
			//      logger.Info("SIGHUP received: reload config not implemented")
		}
	}
}

func mainDaemonWithAutoStart() {
	const cfgPath = "config/config.conf"

	// Минимальный logger для ошибок до парса конфига
	preLogger := log.New("preinit")

	fmt.Printf("[LOG][DEBUG] Loading config from %s\n", cfgPath)
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		preLogger.Error("[DEBUG] Failed to load config: %v", err)
		preLogger.Fatal("failed to load config (%s): %v", cfgPath, err)
	}
	fmt.Printf("[LOG][DEBUG] Config loaded: %+v\n", config.GetConfigForLogging(cfg))
	if err := config.Validate(cfg); err != nil {
		preLogger.Error("[DEBUG] Config validation failed: %v", err)
		preLogger.Fatal("invalid config: %v", err)
	}
	fmt.Printf("[LOG][DEBUG] Config validated successfully\n")

	// Установить конфигурацию для OrderBook логирования
	exchange.SetOrderBookConfig(cfg)
	fmt.Printf("[LOG][DEBUG] OrderBook config set: DebugLogRaw=%t, DebugLogMsg=%t\n",
		cfg.OrderBook.DebugLogRaw, cfg.OrderBook.DebugLogMsg)

	// Выбор режима логирования: global или modular
	if cfg.Logging.Mode == "modular" {
		log.SetGlobalMode(false)
		fmt.Printf("[LOG][DEBUG] Modular logging mode enabled\n")
	} else {
		log.SetGlobalMode(true)
		fmt.Printf("[LOG][DEBUG] Global logging mode enabled\n")
	}

	// Инициализация глобального лог-файла (для global-режима)
	if cfg.Logging.Mode == "global" {
		if err := log.Init(cfg.Logging.File); err != nil {
			fmt.Printf("[LOG][ERROR] Failed to init log file: %v\n", err)
		} else {
			fmt.Printf("[LOG][DEBUG] Log file initialized: %s\n", cfg.Logging.File)
		}
	}

	// Установить уровень логирования из конфига
	if lvl, err := log.ParseLevel(cfg.Logging.Level); err == nil {
		log.SetGlobalLevel(lvl)
		fmt.Printf("[LOG][DEBUG] Log level set to %s\n", lvl.String())
	} else {
		fmt.Printf("[LOG][ERROR] Invalid log level in config: %s\n", cfg.Logging.Level)
	}

	// Создать основной логгер (daemon)
	var logger *log.Logger
	if cfg.Logging.Mode == "modular" && cfg.Logging.Dir != "" {
		logPath := cfg.Logging.Dir + "/daemon.log"
		l, err := log.NewWithFile("daemon", logPath)
		if err != nil {
			fmt.Printf("[LOG][ERROR] Failed to create modular logger: %v\n", err)
			logger = log.New("daemon")
		} else {
			logger = l
			fmt.Printf("[LOG][DEBUG] Modular logger created: %s\n", logPath)
		}
	} else {
		logger = log.New("daemon")
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic: %v", r)
			log.Close()
		}

		// При завершении daemon сбрасываем состояние на inactive
		daemonState := &state.DaemonState{Active: false}
		state.SaveState(cfg.Daemon.StateFile, daemonState)
		log.Close()
	}()

	logger.Info("Daemon starting...")
	logger.Debug("[DEBUG] Process PID: %d", os.Getpid())
	logger.Debug("[DEBUG] Config: %+v", cfg)

	// Создаём DB драйвер
	dbCfg := map[string]string{
		"host":     cfg.Database.Host,
		"port":     strconv.Itoa(cfg.Database.Port),
		"user":     cfg.Database.User,
		"password": cfg.Database.Password,
		"database": cfg.Database.Database,
	}
	logger.Debug("[DEBUG] Creating DB driver: type=%s, cfg=%+v", cfg.Database.Type, dbCfg)
	driver, err := db.NewDriver(cfg.Database.Type, dbCfg)
	if err != nil {
		logger.Error("[DEBUG] Failed to create DB driver: %v", err)
		logger.Fatal("failed to create db driver: %v", err)
	}

	logger.Debug("[DEBUG] Connecting to DB with retry...")
	maxAttempts := 10
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := driver.Connect(); err != nil {
			lastErr = err
			logger.Warn("[DB] Connect attempt %d/%d failed: %v", attempt, maxAttempts, err)
			time.Sleep(2 * time.Second)
		} else {
			logger.Info("DB connected (attempt %d)", attempt)
			lastErr = nil
			break
		}
	}
	if lastErr != nil {
		logger.Error("[DB] All connect attempts failed: %v", lastErr)
		logger.Fatal("db connect failed: %v", lastErr)
	}

	logger.Debug("[DEBUG] Initializing Manager...")
	manager := app.NewManager(cfg, driver, logger)
	logger.Debug("[DEBUG] Manager initialized: %+v", manager)
	logger.Debug("[DEBUG] Starting Manager components...")
	manager.Start()
	logger.Debug("[DEBUG] Manager started")

	// Устанавливаем состояние active = true после успешной инициализации
	state.SetActive(cfg.Daemon.StateFile, true)
	logger.Info("Daemon state set to active")

	// Автоматически запускаем work (для команды start)
	logger.Info("Auto-starting work (daemon start command)...")
	if err := manager.StartWork(); err != nil {
		logger.Error("Failed to auto-start work: %v", err)
	} else {
		logger.Info("Work auto-started successfully")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM /*, syscall.SIGHUP*/)
	logger.Debug("[DEBUG] Signal handler registered, waiting for signals...")
	for {
		sig := <-sigChan
		logger.Debug("[DEBUG] Signal received: %v", sig)
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Debug("[DEBUG] Initiating graceful shutdown...")
			if err := safeStop(manager, logger); err != nil {
				logger.Error("error during shutdown: %v", err)
			}
			logger.Info("Daemon stopped gracefully")
			return
			// case syscall.SIGHUP:
			//      logger.Info("SIGHUP received: reload config not implemented")
		}
	}
}

func safeStop(manager interface{ Stop() }, logger *log.Logger) (err error) {
	logger.Debug("[DEBUG] safeStop called")
	defer func() {
		if r := recover(); r != nil {
			logger.Error("[DEBUG] Panic during Stop: %v", r)
			err, _ = r.(error)
		}
		logger.Debug("[DEBUG] safeStop completed, closing log...")
		log.Close()
	}()
	logger.Debug("[DEBUG] Calling manager.Stop()...")
	manager.Stop()
	logger.Debug("[DEBUG] manager.Stop() finished")
	return nil
}

func main() {
	// Проверяем аргументы командной строки
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "test-htx":
			testHTXAdapter()
			return
		case "start":
			handleStartCommand()
			return
		case "stop":
			handleStopCommand()
			return
		case "status":
			handleStatusCommand()
			return
		}
	}

	// Если аргументов нет, запускаем как daemon
	mainDaemon()
}
