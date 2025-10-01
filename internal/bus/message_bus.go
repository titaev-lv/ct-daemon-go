package bus

import (
	"daemon-go/internal/market"
	"daemon-go/pkg/log"
	"sync"
)

// MessageBus - простая шина сообщений для передачи данных между адаптерами и trade workers
type MessageBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan market.UnifiedMessage // exchange -> channels
	logger      *log.Logger
}

var (
	instance *MessageBus
	once     sync.Once
)

// GetInstance возвращает singleton instance шины сообщений
func GetInstance() *MessageBus {
	once.Do(func() {
		instance = &MessageBus{
			subscribers: make(map[string][]chan market.UnifiedMessage),
			logger:      log.New("message_bus"),
		}
	})
	return instance
}

// Subscribe подписывается на сообщения от конкретной биржи
func (mb *MessageBus) Subscribe(exchange string, bufferSize int) chan market.UnifiedMessage {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	ch := make(chan market.UnifiedMessage, bufferSize)
	mb.subscribers[exchange] = append(mb.subscribers[exchange], ch)
	mb.logger.Debug("[MESSAGE_BUS] Subscribed to exchange %s, total subscribers: %d", exchange, len(mb.subscribers[exchange]))
	return ch
}

// Publish отправляет сообщение всем подписчикам конкретной биржи
func (mb *MessageBus) Publish(exchange string, msg market.UnifiedMessage) {
	mb.mu.RLock()
	subscribers, exists := mb.subscribers[exchange]
	mb.mu.RUnlock()

	if !exists || len(subscribers) == 0 {
		mb.logger.Debug("[MESSAGE_BUS] No subscribers for exchange %s", exchange)
		return
	}

	// Отправляем сообщение всем подписчикам
	for i, ch := range subscribers {
		select {
		case ch <- msg:
			mb.logger.Debug("[MESSAGE_BUS] Message sent to subscriber %d for exchange %s", i, exchange)
		default:
			mb.logger.Warn("[MESSAGE_BUS] Subscriber %d channel full for exchange %s, dropping message", i, exchange)
		}
	}
}

// Unsubscribe отписывается от сообщений (закрывает канал)
func (mb *MessageBus) Unsubscribe(exchange string, ch chan market.UnifiedMessage) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	subscribers, exists := mb.subscribers[exchange]
	if !exists {
		return
	}

	// Находим и удаляем канал
	for i, subscriber := range subscribers {
		if subscriber == ch {
			close(ch)
			// Удаляем из слайса
			mb.subscribers[exchange] = append(subscribers[:i], subscribers[i+1:]...)
			mb.logger.Debug("[MESSAGE_BUS] Unsubscribed from exchange %s, remaining subscribers: %d", exchange, len(mb.subscribers[exchange]))
			break
		}
	}
}

// GetSubscriberCount возвращает количество подписчиков для биржи
func (mb *MessageBus) GetSubscriberCount(exchange string) int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if subscribers, exists := mb.subscribers[exchange]; exists {
		return len(subscribers)
	}
	return 0
}

// GetTotalSubscribers возвращает общее количество подписчиков
func (mb *MessageBus) GetTotalSubscribers() int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	total := 0
	for _, subscribers := range mb.subscribers {
		total += len(subscribers)
	}
	return total
}
