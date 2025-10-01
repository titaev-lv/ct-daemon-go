package db

import (
	"database/sql"
	"fmt"
	"time"

	"daemon-go/pkg/log"
)

var typesLogger = log.New("db_types")

// DataMonitorPair описывает пару для DataMonitor (универсально для всех БД)
type DataMonitorPair struct {
	ExchangeID   int
	ExchangeName string
	PairID       int
	Symbol       string // символ пары как "BTCUSDT", "BTC-USDT" и т.д.
	MarketType   string
}

// DBDriver общий интерфейс для всех БД
// Добавлен универсальный метод для DataMonitor
type DBDriver interface {
	Connect() error
	Close() error
	Ping() error
	GetActiveTrades() ([]TradeCase, error)
	GetActivePairsForDataMonitor() ([]DataMonitorPair, error)
	GetExchangeByName(name string) (*Exchange, error)
}

// TradeCase структура для торгов
type TradeCase struct {
	ID int
}

// Trade — структура, соответствующая записи из таблицы TRADE
type Trade struct {
	ID             int
	UID            int
	Type           int
	Active         bool
	Description    string
	DateCreate     time.Time
	DateModify     time.Time
	MaxAmountTrade float64
	FinProtection  bool
	BBOOnly        bool
}

// Exchange — структура для хранения данных о бирже (например, Kucoin)
type Exchange struct {
	ID             int
	Name           string
	ApiKey         string
	ApiSecret      string
	Passphrase     string
	Active         bool
	Url            string
	BaseUrl        string
	WebsocketUrl   sql.NullString
	WsUrl          sql.NullString // WebSocket endpoint for CexWsClient
	ClassToFactory string
	Description    string
	DateCreate     string // Можно заменить на time.Time, если нужно
	DateModify     string // Можно заменить на time.Time, если нужно
	UserCreated    string
	UserModify     string
	Deleted        bool
}

// NewDriver создает экземпляр драйвера в зависимости от типа
func NewDriver(dbType string, cfg map[string]string) (DBDriver, error) {
	switch dbType {
	case "mysql":
		return &MySQLDriver{
			Host:     cfg["host"],
			Port:     atoi(cfg["port"]),
			User:     cfg["user"],
			Pass:     cfg["password"],
			Database: cfg["database"],
		}, nil
	case "postgresql":
		return &PostgresDriver{
			Host:     cfg["host"],
			Port:     atoi(cfg["port"]),
			User:     cfg["user"],
			Pass:     cfg["password"],
			Database: cfg["database"],
		}, nil
	default:
		err := fmt.Errorf("unsupported database type: %s", dbType)
		typesLogger.Error("db error: %s", err.Error())
		return nil, err
	}
}

func atoi(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}
