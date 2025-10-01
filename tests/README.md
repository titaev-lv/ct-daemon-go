# Структура тестов

Эта директория содержит все тесты для daemon-go, организованные по категориям:

## 📁 adapters/
Тесты для адаптеров бирж:
- `test_kucoin_adapter.go` - Тест адаптера KuCoin
- `test_coinex_adapter.go` - Тест адаптера CoinEx  
- `test_htx_adapter.go` - Тест адаптера HTX

## 📁 integration/
Интеграционные тесты (каждый в отдельной папке):
- `htx/` - Полный интеграционный тест HTX
- `coinex/` - Интеграционный тест CoinEx
- `coinex-simple/` - Простой тест CoinEx

## 📁 parsers/
Тесты парсеров сообщений:
- `test-parsers/` - Тесты парсеров различных форматов сообщений

## Запуск тестов

### Адаптеры
```bash
# Тест KuCoin адаптера
go run tests/adapters/test_kucoin_adapter.go

# Тест CoinEx адаптера  
go run tests/adapters/test_coinex_adapter.go

# Тест HTX адаптера
go run tests/adapters/test_htx_adapter.go
```

### Интеграционные тесты
```bash
# Интеграционный тест HTX
cd tests/integration/htx && go run .

# Интеграционный тест CoinEx
cd tests/integration/coinex && go run .

# Простой тест CoinEx
cd tests/integration/coinex-simple && go run .
```

### Парсеры
```bash
# Тесты парсеров
cd tests/parsers/test-parsers
go run .
```

## Настройки отладки

Все тесты используют отладочные настройки:
- `debug_log_raw=true` - Логирование сырых сообщений
- `debug_log_msg=true` - Логирование обработанных сообщений

## Особенности тестирования

- **KuCoin**: Стандартный WebSocket с ping/pong от клиента
- **CoinEx**: Стандартный WebSocket с ping/pong от клиента  
- **HTX**: Особенность - биржа сама шлет ping'и, gzip сжатие сообщений