package postgres

// GetActiveTrades получает список активных трейдов
const GetActiveTrades = "SELECT ID FROM TRADE WHERE ACTIVE=1"

// GetExchangeByName получает информацию о бирже по имени
const GetExchangeByName = `SELECT ID, NAME, ACTIVE, URL, BASE_URL, WEBSOCKET_URL, CLASS_TO_FACTORY, DESCRIPTION, DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, DELETED FROM ct_system.EXCHANGE WHERE NAME = $1 AND DELETED = false`
