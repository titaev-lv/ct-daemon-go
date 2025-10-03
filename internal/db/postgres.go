package db

import (
	"database/sql"
	"fmt"

	sqlPostgres "daemon-go/internal/sql/postgres"
	"daemon-go/pkg/log"

	_ "github.com/lib/pq"
)

var pgLogger = log.New("postgres")

// PostgresDriver реализует DBDriver для PostgreSQL
type PostgresDriver struct {
	DB       *sql.DB
	Host     string
	Port     int
	User     string
	Pass     string
	Database string
}

func (p *PostgresDriver) Connect() error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Pass, p.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	p.DB = db
	return p.Ping()
}

func (p *PostgresDriver) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}

func (p *PostgresDriver) Ping() error {
	return p.DB.Ping()
}

func (p *PostgresDriver) GetActiveTrades() ([]TradeCase, error) {
	rows, err := p.DB.Query(sqlPostgres.GetActiveTrades)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []TradeCase
	for rows.Next() {
		var t TradeCase
		if err := rows.Scan(&t.ID); err != nil {
			pgLogger.Error("Error scanning trade: %v", err)
			continue
		}
		trades = append(trades, t)
	}
	return trades, nil
}

// Заглушка для универсального метода DataMonitor (реализовать по аналогии с MySQL при необходимости)
func (p *PostgresDriver) GetActivePairsForDataMonitor() ([]DataMonitorPair, error) {
	rows, err := p.DB.Query(sqlPostgres.DataMonitorPairs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []DataMonitorPair
	for rows.Next() {
		var p DataMonitorPair
		if err := rows.Scan(&p.ExchangeID, &p.ExchangeName, &p.PairID, &p.Symbol, &p.MarketType); err != nil {
			pgLogger.Error("Error scanning DataMonitorPair: %v", err)
			continue
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// GetExchangeByName возвращает Exchange по имени
func (p *PostgresDriver) GetExchangeByName(name string) (*Exchange, error) {
	row := p.DB.QueryRow(sqlPostgres.GetExchangeByName, name)
	var ex Exchange
	err := row.Scan(&ex.ID, &ex.Name, &ex.Active, &ex.Url, &ex.BaseUrl, &ex.WebsocketUrl, &ex.ClassToFactory, &ex.Description, &ex.DateCreate, &ex.DateModify, &ex.UserCreated, &ex.UserModify, &ex.Deleted)
	if err != nil {
		return nil, err
	}
	return &ex, nil
}

// Query выполняет произвольный SQL запрос
func (p *PostgresDriver) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return p.DB.Query(query, args...)
}

// BeginTx начинает транзакцию
func (p *PostgresDriver) BeginTx() (*sql.Tx, error) {
	return p.DB.Begin()
}

// GetType возвращает тип базы данных
func (p *PostgresDriver) GetType() string {
	return "postgres"
}
