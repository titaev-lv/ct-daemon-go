-- Получить активные пары для DataMonitor (PostgreSQL)
SELECT id, symbol, market_type FROM pairs WHERE active = TRUE;