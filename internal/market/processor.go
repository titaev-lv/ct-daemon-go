package market

import (
	"fmt"
	"sync"
)

// MessageProcessor - процессор унифицированных сообщений
type MessageProcessor struct {
	parsers  map[string]MessageParser
	handlers []MessageHandler
	mu       sync.RWMutex
}

// NewMessageProcessor создает новый процессор сообщений
func NewMessageProcessor() *MessageProcessor {
	return &MessageProcessor{
		parsers:  make(map[string]MessageParser),
		handlers: make([]MessageHandler, 0),
	}
}

// RegisterParser регистрирует парсер для биржи
func (mp *MessageProcessor) RegisterParser(exchange string, parser MessageParser) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.parsers[exchange] = parser
}

// RegisterHandler регистрирует обработчик сообщений
func (mp *MessageProcessor) RegisterHandler(handler MessageHandler) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.handlers = append(mp.handlers, handler)
}

// ProcessRawMessage обрабатывает сырое сообщение от биржи
func (mp *MessageProcessor) ProcessRawMessage(exchange string, rawData []byte) error {
	mp.mu.RLock()
	parser, exists := mp.parsers[exchange]
	mp.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no parser registered for exchange: %s", exchange)
	}

	if !parser.CanParse(exchange, rawData) {
		return fmt.Errorf("parser cannot handle message from %s", exchange)
	}

	unifiedMsg, err := parser.ParseMessage(exchange, rawData)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Отправляем сообщение всем обработчикам
	mp.mu.RLock()
	handlers := make([]MessageHandler, len(mp.handlers))
	copy(handlers, mp.handlers)
	mp.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler.HandleMessage(*unifiedMsg); err != nil {
			// Логируем ошибку, но продолжаем обработку другими обработчиками
			fmt.Printf("Handler error: %v\n", err)
		}
	}

	return nil
}

// GetSupportedExchanges возвращает список поддерживаемых бирж
func (mp *MessageProcessor) GetSupportedExchanges() []string {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	exchanges := make([]string, 0, len(mp.parsers))
	for exchange := range mp.parsers {
		exchanges = append(exchanges, exchange)
	}
	return exchanges
}
