package config

import (
	"errors"

	"gopkg.in/ini.v1"
)

type Config struct {
	Daemon struct {
		PollInterval int
		HttpPort     int
		StateFile    string
	}
	Server struct {
		Port int
	}
	Database struct {
		Type     string
		Host     string
		Port     int
		User     string
		Password string
		Database string
	}
	WebSocket struct {
		PingInterval   int
		ReconnectDelay int
	}
	Logging struct {
		Level     string
		File      string
		MaxSizeMB int
		Mode      string // global или modular
		Dir       string // директория для модульных логов
	}
	OrderBook struct {
		DebugLogRaw bool // логирование чистых сообщений от и к бирже в json
		DebugLogMsg bool // логирование уже unified message в json
	}
}

// LoadConfig загружает конфиг из файла
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}
	file, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	cfg.Daemon.PollInterval = file.Section("daemon").Key("poll_interval").MustInt(5)
	cfg.Daemon.HttpPort = file.Section("daemon").Key("http_port").MustInt(8080)
	cfg.Daemon.StateFile = file.Section("daemon").Key("state_file").String()

	cfg.Database.Type = file.Section("database").Key("type").String()
	cfg.Database.Host = file.Section("database").Key("host").String()
	cfg.Database.Port = file.Section("database").Key("port").MustInt()
	cfg.Database.User = file.Section("database").Key("user").String()
	cfg.Database.Password = file.Section("database").Key("password").String()
	cfg.Database.Database = file.Section("database").Key("database").String()

	cfg.WebSocket.PingInterval = file.Section("websocket").Key("ping_interval").MustInt()
	cfg.WebSocket.ReconnectDelay = file.Section("websocket").Key("reconnect_delay").MustInt()

	cfg.Logging.Level = file.Section("logging").Key("level").MustString("INFO")
	cfg.Logging.File = file.Section("logging").Key("file").MustString("daemon.log")
	cfg.Logging.MaxSizeMB = file.Section("logging").Key("max_size_mb").MustInt()
	cfg.Logging.Mode = file.Section("logging").Key("mode").MustString("global")
	cfg.Logging.Dir = file.Section("logging").Key("dir").MustString("")

	cfg.OrderBook.DebugLogRaw = file.Section("orderbook").Key("debug_log_raw").MustBool(false)
	cfg.OrderBook.DebugLogMsg = file.Section("orderbook").Key("debug_log_msg").MustBool(false)

	return cfg, nil
}

// GetConfigForLogging returns a copy of config with masked sensitive data for logging
func GetConfigForLogging(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	// Create a copy
	cfgForLog := *cfg
	// Mask password
	if cfgForLog.Database.Password != "" {
		cfgForLog.Database.Password = "*****"
	}
	return &cfgForLog
}

// Validate проверяет корректность конфига
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if cfg.Database.Type == "" {
		return errors.New("database.type is required")
	}
	if cfg.Database.Host == "" {
		return errors.New("database.host is required")
	}
	if cfg.Database.Port == 0 {
		return errors.New("database.port is required")
	}
	if cfg.Database.User == "" {
		return errors.New("database.user is required")
	}
	if cfg.Database.Database == "" {
		return errors.New("database.database is required")
	}
	if cfg.Daemon.HttpPort == 0 {
		return errors.New("daemon.http_port is required")
	}
	if cfg.Daemon.PollInterval <= 0 {
		return errors.New("daemon.poll_interval must be > 0")
	}
	if cfg.Logging.File == "" {
		return errors.New("logging.file is required")
	}
	if cfg.Logging.Level == "" {
		return errors.New("logging.level is required")
	}
	return nil
}
