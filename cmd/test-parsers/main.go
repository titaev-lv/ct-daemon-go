package main

import (
	"daemon-go/internal/market"
	"daemon-go/internal/market/parsers"
	"daemon-go/pkg/log"
	"encoding/json"
)

// Тестирование всех парсеров бирж
func main() {
	logger := log.New("test-parsers")
	logger.Info("=== ТЕСТИРОВАНИЕ ПАРСЕРОВ ВСЕХ БИРЖ ===")

	// Создаем процессор сообщений
	processor := market.NewMessageProcessor()

	// Регистрируем все парсеры
	exchangeParsers := map[string]market.MessageParser{
		"binance":  parsers.NewBinanceParser(),
		"bybit":    parsers.NewBybitParser(),
		"kucoin":   parsers.NewKucoinParser(),
		"htx":      parsers.NewHTXParser(),
		"coinex":   parsers.NewCoinexParser(),
		"poloniex": parsers.NewPoloniexParser(),
	}

	for exchange, parser := range exchangeParsers {
		processor.RegisterParser(exchange, parser)
		logger.Info("✓ Парсер %s зарегистрирован", exchange)
	}

	// Тестовые данные для каждой биржи
	testData := map[string]string{
		"binance": `{
			"stream": "btcusdt@depth5@1000ms",
			"data": {
				"s": "BTCUSDT",
				"lastUpdateId": 123456789,
				"bids": [["43000.1", "1.234"], ["43000.0", "2.456"]],
				"asks": [["43000.2", "1.567"], ["43000.3", "2.789"]]
			}
		}`,

		"bybit": `{
			"topic": "orderbook.1.BTCUSDT",
			"type": "snapshot",
			"ts": 1672916400000,
			"data": {
				"s": "BTCUSDT",
				"b": [["43000.1", "1.234"], ["43000.0", "2.456"]],
				"a": [["43000.2", "1.567"], ["43000.3", "2.789"]],
				"u": 123456,
				"seq": 789012
			}
		}`,

		"kucoin": `{
			"type": "message",
			"topic": "/market/level2:BTC-USDT",
			"subject": "trade.l2update",
			"data": {
				"sequenceStart": 1672916400,
				"sequenceEnd": 1672916401,
				"symbol": "BTC-USDT",
				"changes": {
					"asks": [["43000.5", "1.2", "1672916400"]],
					"bids": [["43000.1", "2.5", "1672916401"]]
				}
			}
		}`,

		"htx": `{
			"ch": "market.btcusdt.depth.step0",
			"ts": 1672916400000,
			"tick": {
				"bids": [[43000.1, 1.234], [43000.0, 2.456]],
				"asks": [[43000.2, 1.567], [43000.3, 2.789]],
				"ts": 1672916400000,
				"version": 123456
			}
		}`,

		"coinex": `{
			"method": "depth.update",
			"params": [true, "BTCUSDT", {"bids": [["43000.1", "1.234"]], "asks": [["43000.2", "1.567"]]}],
			"id": null
		}`,

		"poloniex": `{
			"channel": "book_lv2",
			"data": [{
				"symbol": "BTC_USDT",
				"createTime": 1672916400000,
				"asks": [["43000.2", "1.567"]],
				"bids": [["43000.1", "1.234"]],
				"lastUpdateId": 123456
			}]
		}`,
	}

	logger.Info("\n=== ТЕСТИРОВАНИЕ ПАРСИНГА ===")

	// Тестируем каждую биржу
	for exchange, jsonData := range testData {
		logger.Info("\n--- Тестируем %s ---", exchange)

		// Проверяем, что JSON валидный
		var temp interface{}
		if err := json.Unmarshal([]byte(jsonData), &temp); err != nil {
			logger.Error("❌ %s: невалидный JSON: %v", exchange, err)
			continue
		}

		// Парсим через процессор
		err := processor.ProcessRawMessage(exchange, []byte(jsonData))
		if err != nil {
			logger.Error("❌ %s: ошибка обработки: %v", exchange, err)
		} else {
			logger.Info("✅ %s: сообщение успешно обработано", exchange)
		}
	}

	logger.Info("\n=== ТЕСТИРОВАНИЕ ЗАВЕРШЕНО ===")
	logger.Info("Поддерживаемые биржи: %v", processor.GetSupportedExchanges())
}
