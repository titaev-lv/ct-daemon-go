package db

import (
	"database/sql"
	"fmt"
	"strings"

	"daemon-go/internal/queries"
	"daemon-go/pkg/log"

	_ "github.com/go-sql-driver/mysql"
)

var mysqlLogger = log.New("mysql")

// MySQLDriver реализует DBDriver для MySQL
type MySQLDriver struct {
	DB       *sql.DB
	Host     string
	Port     int
	User     string
	Pass     string
	Database string
}

// GetActivePairsForDataMonitor возвращает пары (EXCHANGE_ID, PAIR_ID, SYMBOL, MARKET_TYPE) для DataMonitor
func (m *MySQLDriver) GetActivePairsForDataMonitor() ([]DataMonitorPair, error) {
	rows, err := m.DB.Query(queries.DataMonitorPairsSQLMySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []DataMonitorPair
	for rows.Next() {
		var p DataMonitorPair
		if err := rows.Scan(&p.ExchangeID, &p.ExchangeName, &p.PairID, &p.Symbol, &p.MarketType); err != nil {
			mysqlLogger.Error("Error scanning DataMonitorPair: %v", err)
			continue
		}
		// Приводим MarketType к нижнему регистру для совместимости с адаптерами
		p.MarketType = strings.ToLower(p.MarketType)
		pairs = append(pairs, p)
	}
	return pairs, nil
}

func (m *MySQLDriver) Connect() error {
	dsn := m.User + ":" + m.Pass + "@tcp(" + m.Host + ":" + itoa(m.Port) + ")/" + m.Database + "?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	m.DB = db
	return m.Ping()
}

func (m *MySQLDriver) Close() error {
	if m.DB != nil {
		return m.DB.Close()
	}
	return nil
}

func (m *MySQLDriver) Ping() error {
	return m.DB.Ping()
}

func (m *MySQLDriver) GetActiveTrades() ([]TradeCase, error) {
	rows, err := m.DB.Query("SELECT ID FROM TRADE WHERE ACTIVE=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []TradeCase
	for rows.Next() {
		var t TradeCase
		if err := rows.Scan(&t.ID); err != nil {
			mysqlLogger.Error("Error scanning trade: %v", err)
			continue
		}
		trades = append(trades, t)
	}
	return trades, nil
}

// GetExchangeByName возвращает Exchange по имени
func (m *MySQLDriver) GetExchangeByName(name string) (*Exchange, error) {
	row := m.DB.QueryRow("SELECT ID, NAME, ACTIVE, URL, BASE_URL, WEBSOCKET_URL, CLASS_TO_FACTORY, DESCRIPTION, DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, DELETED FROM ct_system.EXCHANGE WHERE NAME = ? AND DELETED = 0", name)
	var ex Exchange
	err := row.Scan(&ex.ID, &ex.Name, &ex.Active, &ex.Url, &ex.BaseUrl, &ex.WebsocketUrl, &ex.ClassToFactory, &ex.Description, &ex.DateCreate, &ex.DateModify, &ex.UserCreated, &ex.UserModify, &ex.Deleted)
	if err != nil {
		return nil, err
	}
	return &ex, nil
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
