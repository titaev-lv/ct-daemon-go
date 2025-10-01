package exchange

import (
	"fmt"
	"strings"

	"daemon-go/pkg/log"

	"github.com/gorilla/websocket"
)

// DebugLogger - логгер для debug сообщений WebSocket
type DebugLogger struct {
	enabled  bool
	exchange string
	logger   *log.Logger
}

func NewDebugLogger(exchange string, enabled bool) *DebugLogger {
	return &DebugLogger{
		enabled:  enabled,
		exchange: exchange,
		logger:   log.New(fmt.Sprintf("websocket-%s", exchange)),
	}
}

// LogRawReceived логирует полученное сырое сообщение
func (dl *DebugLogger) LogRawReceived(msgType int, data []byte) {
	if !dl.enabled {
		return
	}

	msgTypeStr := dl.getMessageTypeString(msgType)
	dataStr := string(data)

	// Обрезаем очень длинные сообщения
	if len(dataStr) > 1000 {
		dataStr = dataStr[:1000] + "... [TRUNCATED]"
	}

	dl.logger.Debug("[RX][%s] %s", msgTypeStr, dataStr)
}

// LogRawSent логирует отправленное сырое сообщение
func (dl *DebugLogger) LogRawSent(msgType int, data []byte) {
	if !dl.enabled {
		return
	}

	msgTypeStr := dl.getMessageTypeString(msgType)
	dataStr := string(data)

	dl.logger.Debug("[TX][%s] %s", msgTypeStr, dataStr)
}

// LogPingPong логирует ping/pong сообщения
func (dl *DebugLogger) LogPingPong(direction string, msgType int, data []byte) {
	if !dl.enabled {
		return
	}

	msgTypeStr := dl.getMessageTypeString(msgType)
	dl.logger.Debug("[%s][%s] %s", direction, msgTypeStr, string(data))
}

// LogConnection логирует события подключения
func (dl *DebugLogger) LogConnection(event string, details ...interface{}) {
	if !dl.enabled {
		return
	}

	if len(details) > 0 {
		dl.logger.Debug("[CONN] %s: %v", event, details)
	} else {
		dl.logger.Debug("[CONN] %s", event)
	}
}

// LogSubscription логирует подписки/отписки
func (dl *DebugLogger) LogSubscription(action string, pairs []string, marketType string, depth int) {
	if !dl.enabled {
		return
	}

	dl.logger.Debug("[SUB] %s: pairs=%v, type=%s, depth=%d", action, pairs, marketType, depth)
}

// LogParsedMessage логирует преобразованное сообщение
func (dl *DebugLogger) LogParsedMessage(symbol, messageType string, data interface{}) {
	if !dl.enabled {
		return
	}

	dl.logger.Debug("[PARSED] %s %s: %+v", messageType, symbol, data)
}

// LogError логирует ошибки
func (dl *DebugLogger) LogError(operation string, err error) {
	if !dl.enabled {
		return
	}

	dl.logger.Error("[ERROR] %s: %v", operation, err)
}

// getMessageTypeString конвертирует тип сообщения в строку
func (dl *DebugLogger) getMessageTypeString(msgType int) string {
	switch msgType {
	case websocket.TextMessage:
		return "TEXT"
	case websocket.BinaryMessage:
		return "BINARY"
	case websocket.CloseMessage:
		return "CLOSE"
	case websocket.PingMessage:
		return "PING"
	case websocket.PongMessage:
		return "PONG"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", msgType)
	}
}

// LogFilteredMessage логирует сообщение с фильтрацией конфиденциальных данных
func (dl *DebugLogger) LogFilteredMessage(direction, msgType string, data []byte) {
	if !dl.enabled {
		return
	}

	dataStr := string(data)

	// Фильтруем конфиденциальную информацию
	filtered := dl.filterSensitiveData(dataStr)

	// Обрезаем длинные сообщения
	if len(filtered) > 1000 {
		filtered = filtered[:1000] + "... [TRUNCATED]"
	}

	dl.logger.Debug("[%s][%s] %s", direction, msgType, filtered)
}

// filterSensitiveData удаляет конфиденциальные данные из логов
func (dl *DebugLogger) filterSensitiveData(data string) string {
	// Список полей для скрытия
	sensitiveFields := []string{
		"apiKey", "secretKey", "passphrase", "signature",
		"timestamp", "recv_window", "recvWindow",
	}

	filtered := data
	for _, field := range sensitiveFields {
		// Простая замена значений чувствительных полей
		if strings.Contains(filtered, field) {
			start := strings.Index(filtered, field)
			if start != -1 {
				// Находим конец значения
				valueStart := strings.Index(filtered[start:], ":") + start + 1
				if valueStart > start {
					valueEnd := valueStart
					for i, char := range filtered[valueStart:] {
						if char == ',' || char == '}' || char == '&' || char == ' ' {
							valueEnd = valueStart + i
							break
						}
					}
					if valueEnd == valueStart {
						valueEnd = len(filtered)
					}

					// Заменяем значение на ***
					filtered = filtered[:valueStart] + `"***"` + filtered[valueEnd:]
				}
			}
		}
	}

	return filtered
}

// IsEnabled возвращает состояние debug логирования
func (dl *DebugLogger) IsEnabled() bool {
	return dl.enabled
}

// SetEnabled включает/выключает debug логирование
func (dl *DebugLogger) SetEnabled(enabled bool) {
	dl.enabled = enabled
	if enabled {
		dl.logger.Info("Debug logging ENABLED")
	} else {
		dl.logger.Info("Debug logging DISABLED")
	}
}

// LogStatistics логирует статистику сообщений
func (dl *DebugLogger) LogStatistics(stats map[string]interface{}) {
	if !dl.enabled {
		return
	}

	dl.logger.Debug("[STATS] %+v", stats)
}
