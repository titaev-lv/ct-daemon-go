package exchange

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// CexWsClient — базовый клиент для WebSocket CEX

type CexWsClient struct {
	BaseURL   string
	Conn      *websocket.Conn
	Mutex     sync.Mutex
	connected bool
}

func NewCexWsClient(baseURL string) *CexWsClient {
	return &CexWsClient{
		BaseURL: baseURL,
	}
}

// Connect устанавливает WebSocket-соединение
func (c *CexWsClient) Connect() error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("CexWsClient: invalid url: %w", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("CexWsClient: dial error: %w", err)
	}
	c.Mutex.Lock()
	c.Conn = conn
	c.connected = true
	c.Mutex.Unlock()
	log.Printf("[CexWsClient] Connected to %s", c.BaseURL)
	return nil
}

// ReadMessage читает одно сообщение
func (c *CexWsClient) ReadMessage() (messageType int, p []byte, err error) {
	// Не используем мьютекс здесь, так как это может привести к deadlock
	// Проверка соединения выполняется без блокировки
	if c.Conn == nil {
		return 0, nil, fmt.Errorf("CexWsClient: not connected")
	}
	return c.Conn.ReadMessage()
}

// WriteMessage отправляет сообщение
func (c *CexWsClient) WriteMessage(messageType int, data []byte) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn == nil {
		return fmt.Errorf("CexWsClient: not connected")
	}
	return c.Conn.WriteMessage(messageType, data)
}

// Close закрывает соединение
func (c *CexWsClient) Close() error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn != nil {
		err := c.Conn.Close()
		c.Conn = nil
		c.connected = false
		return err
	}
	return nil
}

// IsConnected возвращает статус соединения
func (c *CexWsClient) IsConnected() bool {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	return c.connected
}

// Reconnect пытается восстановить соединение
func (c *CexWsClient) Reconnect() error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn != nil {
		c.Conn.Close()
		c.Conn = nil
		c.connected = false
	}

	// Подключаемся заново
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("CexWsClient: invalid url: %w", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("CexWsClient: dial error: %w", err)
	}
	c.Conn = conn
	c.connected = true
	log.Printf("[CexWsClient] Reconnected to %s", c.BaseURL)
	return nil
}

// WriteLoop запускает цикл отправки ping сообщений
func (c *CexWsClient) WriteLoop(pingPeriod time.Duration) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		c.Mutex.Lock()
		if c.Conn == nil {
			c.Mutex.Unlock()
			return
		}

		// Для CoinEx отправляем ping как JSON сообщение
		pingMsg := map[string]interface{}{
			"method": "server.ping",
			"params": []interface{}{},
			"id":     999,
		}

		pingData, err := json.Marshal(pingMsg)
		if err != nil {
			c.Mutex.Unlock()
			log.Printf("[CexWsClient] Marshal ping error: %v", err)
			continue
		}

		err = c.Conn.WriteMessage(websocket.TextMessage, pingData)
		c.Mutex.Unlock()

		if err != nil {
			log.Printf("[CexWsClient] Write ping error: %v", err)
			// Не вызываем Reconnect здесь, это должен делать readLoop
			return
		}

		log.Printf("[CexWsClient] Ping sent to %s", c.BaseURL)
	}
}
