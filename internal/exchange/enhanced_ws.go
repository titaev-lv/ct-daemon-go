package exchange

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"daemon-go/internal/market"
	"daemon-go/pkg/log"

	"github.com/gorilla/websocket"
)

// EnhancedCexWsClient — улучшенный базовый клиент для WebSocket CEX с поддержкой унифицированных сообщений
type EnhancedCexWsClient struct {
	BaseURL          string
	Exchange         string
	Conn             *websocket.Conn
	Mutex            sync.Mutex
	connected        bool
	logger           *log.Logger
	messageProcessor *market.MessageProcessor
	debugLogger      *DebugLogger
	stopChan         chan struct{}
	reconnectDelay   time.Duration
}

func NewEnhancedCexWsClient(baseURL, exchange string, processor *market.MessageProcessor) *EnhancedCexWsClient {
	return &EnhancedCexWsClient{
		BaseURL:          baseURL,
		Exchange:         exchange,
		messageProcessor: processor,
		debugLogger:      NewDebugLogger(exchange, true), // включаем debug по умолчанию
		stopChan:         make(chan struct{}),
		reconnectDelay:   5 * time.Second,
		logger:           log.New(fmt.Sprintf("enhanced-ws-%s", exchange)),
	}
}

// Connect устанавливает WebSocket-соединение
func (c *EnhancedCexWsClient) Connect() error {
	c.debugLogger.LogConnection("CONNECTING", c.BaseURL)

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		c.debugLogger.LogError("CONNECT_PARSE_URL", err)
		return fmt.Errorf("EnhancedCexWsClient: invalid url: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.debugLogger.LogError("CONNECT_DIAL", err)
		return fmt.Errorf("EnhancedCexWsClient: dial error: %w", err)
	}

	c.Mutex.Lock()
	c.Conn = conn
	c.connected = true
	c.Mutex.Unlock()

	c.debugLogger.LogConnection("CONNECTED", c.BaseURL)
	c.logger.Info("Connected to %s", c.BaseURL)
	return nil
} // ReadMessage читает одно сообщение
func (c *EnhancedCexWsClient) ReadMessage() (messageType int, p []byte, err error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn == nil {
		return 0, nil, fmt.Errorf("EnhancedCexWsClient: not connected")
	}
	return c.Conn.ReadMessage()
}

// WriteMessage отправляет сообщение
func (c *EnhancedCexWsClient) WriteMessage(messageType int, data []byte) error {
	c.debugLogger.LogRawSent(messageType, data)

	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn == nil {
		return fmt.Errorf("EnhancedCexWsClient: not connected")
	}
	return c.Conn.WriteMessage(messageType, data)
}

// Close закрывает соединение
func (c *EnhancedCexWsClient) Close() error {
	c.debugLogger.LogConnection("CLOSING")

	// Сигнализируем остановку циклов
	close(c.stopChan)

	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Conn != nil {
		err := c.Conn.Close()
		c.Conn = nil
		c.connected = false
		c.debugLogger.LogConnection("CLOSED")
		return err
	}
	return nil
} // IsConnected возвращает статус соединения
func (c *EnhancedCexWsClient) IsConnected() bool {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	return c.connected
}

// Reconnect пытается восстановить соединение
func (c *EnhancedCexWsClient) Reconnect() error {
	c.debugLogger.LogConnection("RECONNECTING")
	c.logger.Info("Reconnecting...")

	c.Mutex.Lock()
	if c.Conn != nil {
		c.Conn.Close()
		c.Conn = nil
		c.connected = false
	}
	c.Mutex.Unlock()

	// Задержка перед переподключением
	time.Sleep(c.reconnectDelay)

	return c.Connect()
} // StartMessageProcessing запускает обработку сообщений с автоматическим переподключением
func (c *EnhancedCexWsClient) StartMessageProcessing() {
	go c.readLoopWithReconnect()
	go c.pingLoop()
}

// readLoopWithReconnect цикл чтения с автоматическим переподключением
func (c *EnhancedCexWsClient) readLoopWithReconnect() {
	for {
		select {
		case <-c.stopChan:
			c.logger.Info("Stopping read loop")
			return
		default:
			if !c.IsConnected() {
				if err := c.Reconnect(); err != nil {
					c.logger.Error("Reconnect failed: %v", err)
					time.Sleep(c.reconnectDelay)
					continue
				}
			}

			msgType, msg, err := c.ReadMessage()
			if err != nil {
				c.debugLogger.LogError("READ_MESSAGE", err)
				c.logger.Error("Read error: %v", err)
				c.Mutex.Lock()
				c.connected = false
				c.Mutex.Unlock()
				continue
			}

			// Debug логирование всех полученных сообщений
			c.debugLogger.LogRawReceived(msgType, msg)

			// Обработка ping/pong
			if c.handlePingPong(msgType, msg) {
				continue
			}

			// КЛЮЧЕВОЕ УЛУЧШЕНИЕ: Обработка сообщений через процессор
			if msgType == websocket.TextMessage && c.messageProcessor != nil {
				if err := c.messageProcessor.ProcessRawMessage(c.Exchange, msg); err != nil {
					c.debugLogger.LogError("PROCESS_MESSAGE", err)
					c.logger.Error("Message processing error: %v", err)
					// Продолжаем работу даже при ошибках парсинга
				}
			}
		}
	}
}

// handlePingPong обрабатывает ping/pong сообщения
func (c *EnhancedCexWsClient) handlePingPong(msgType int, msg []byte) bool {
	switch msgType {
	case websocket.PingMessage:
		c.debugLogger.LogPingPong("RX", msgType, msg)
		c.logger.Debug("Received PING, sending PONG")
		c.WriteMessage(websocket.PongMessage, msg)
		return true
	case websocket.TextMessage:
		msgStr := string(msg)
		if msgStr == "ping" || msgStr == "PING" {
			c.debugLogger.LogPingPong("RX", msgType, msg)
			c.logger.Debug("Received ping (text), sending pong")
			c.WriteMessage(websocket.TextMessage, []byte("pong"))
			return true
		}
	}
	return false
}

// pingLoop отправляет периодические ping
func (c *EnhancedCexWsClient) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			c.logger.Info("Stopping ping loop")
			return
		case <-ticker.C:
			if c.IsConnected() {
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					c.logger.Error("Ping error: %v", err)
				}
			}
		}
	}
}
