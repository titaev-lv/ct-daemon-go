package market

import (
	"fmt"
	"strings"
)

// UnifiedSymbol - унифицированный формат торговой пары
type UnifiedSymbol struct {
	BaseCurrency   string `json:"base_currency"`   // BTC
	QuoteCurrency  string `json:"quote_currency"`  // USDT
	MarketType     string `json:"market_type"`     // spot, futures
	Symbol         string `json:"symbol"`          // унифицированный символ
	OriginalSymbol string `json:"original_symbol"` // оригинальный символ от биржи
}

// String реализует интерфейс fmt.Stringer для корректного отображения в логах
func (us *UnifiedSymbol) String() string {
	if us == nil {
		return "<nil>"
	}
	return us.Symbol
}

// NewUnifiedSymbol создает унифицированный символ
func NewUnifiedSymbol(baseCurrency, quoteCurrency, marketType string) *UnifiedSymbol {
	symbol := formatUnifiedSymbol(baseCurrency, quoteCurrency, marketType)
	return &UnifiedSymbol{
		BaseCurrency:  strings.ToUpper(baseCurrency),
		QuoteCurrency: strings.ToUpper(quoteCurrency),
		MarketType:    strings.ToLower(marketType),
		Symbol:        symbol,
	}
}

// formatUnifiedSymbol создает унифицированный символ по правилам:
// spot: BTC/USDT
// futures: BTCUSDT
func formatUnifiedSymbol(base, quote, marketType string) string {
	base = strings.ToUpper(base)
	quote = strings.ToUpper(quote)
	marketType = strings.ToLower(marketType)

	switch marketType {
	case "spot":
		return fmt.Sprintf("%s/%s", base, quote)
	case "futures", "future":
		return fmt.Sprintf("%s%s", base, quote)
	default:
		// По умолчанию как spot
		return fmt.Sprintf("%s/%s", base, quote)
	}
}

// ParseSymbol парсит различные форматы символов в унифицированный
func ParseSymbol(symbol, marketType string) (*UnifiedSymbol, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	marketType = strings.ToLower(strings.TrimSpace(marketType))

	var base, quote string

	// Обработка различных форматов
	switch {
	case strings.Contains(symbol, "/"):
		// BTC/USDT
		parts := strings.Split(symbol, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid symbol format with /: %s", symbol)
		}
		base, quote = parts[0], parts[1]

	case strings.Contains(symbol, "-"):
		// BTC-USDT (Kucoin, OKX)
		parts := strings.Split(symbol, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid symbol format with -: %s", symbol)
		}
		base, quote = parts[0], parts[1]

	case strings.Contains(symbol, "_"):
		// BTC_USDT (некоторые биржи)
		parts := strings.Split(symbol, "_")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid symbol format with _: %s", symbol)
		}
		base, quote = parts[0], parts[1]

	default:
		// BTCUSDT - нужно разделить
		base, quote = splitConcatenatedSymbol(symbol)
		if base == "" || quote == "" {
			return nil, fmt.Errorf("cannot parse concatenated symbol: %s", symbol)
		}
	}

	unifiedSymbol := NewUnifiedSymbol(base, quote, marketType)
	unifiedSymbol.OriginalSymbol = symbol
	return unifiedSymbol, nil
}

// splitConcatenatedSymbol разделяет склеенные символы типа BTCUSDT
func splitConcatenatedSymbol(symbol string) (base, quote string) {
	// Общие quote валюты в порядке убывания длины
	commonQuotes := []string{
		"USDT", "USDC", "BUSD", "TUSD", "USDP",
		"BTC", "ETH", "BNB", "USD", "EUR", "GBP",
		"DAI", "FDUSD", "BTTC", "TRX",
	}

	for _, q := range commonQuotes {
		if strings.HasSuffix(symbol, q) && len(symbol) > len(q) {
			base = symbol[:len(symbol)-len(q)]
			quote = q
			return
		}
	}

	// Если не нашли, пробуем общие паттерны
	if len(symbol) >= 6 {
		// Попробуем разделить пополам для коротких символов
		if len(symbol) == 6 {
			return symbol[:3], symbol[3:]
		}
		// Для длинных символов предполагаем, что quote - последние 3-4 символа
		if strings.HasSuffix(symbol, "USDT") {
			return symbol[:len(symbol)-4], "USDT"
		}
		if len(symbol) >= 6 {
			return symbol[:len(symbol)-3], symbol[len(symbol)-3:]
		}
	}

	return "", ""
}

// ExchangeSymbolConverter - интерфейс для конвертации символов биржи
type ExchangeSymbolConverter interface {
	ToExchangeSymbol(unified *UnifiedSymbol) string
	FromExchangeSymbol(exchangeSymbol, marketType string) (*UnifiedSymbol, error)
}

// BinanceSymbolConverter - конвертер для Binance
type BinanceSymbolConverter struct{}

func (c *BinanceSymbolConverter) ToExchangeSymbol(unified *UnifiedSymbol) string {
	// Binance использует формат BTCUSDT для всех рынков
	return fmt.Sprintf("%s%s", unified.BaseCurrency, unified.QuoteCurrency)
}

func (c *BinanceSymbolConverter) FromExchangeSymbol(exchangeSymbol, marketType string) (*UnifiedSymbol, error) {
	return ParseSymbol(exchangeSymbol, marketType)
}

// KucoinSymbolConverter - конвертер для Kucoin
type KucoinSymbolConverter struct{}

func (c *KucoinSymbolConverter) ToExchangeSymbol(unified *UnifiedSymbol) string {
	// Kucoin использует формат BTC-USDT для спот, XBTUSDTM для фьючерсов
	if unified.MarketType == "futures" {
		// Специальная логика для фьючерсов Kucoin
		return fmt.Sprintf("%s%sM", unified.BaseCurrency, unified.QuoteCurrency)
	}
	return fmt.Sprintf("%s-%s", unified.BaseCurrency, unified.QuoteCurrency)
}

func (c *KucoinSymbolConverter) FromExchangeSymbol(exchangeSymbol, marketType string) (*UnifiedSymbol, error) {
	// Обработка специфики Kucoin
	if marketType == "futures" && strings.HasSuffix(exchangeSymbol, "M") {
		exchangeSymbol = strings.TrimSuffix(exchangeSymbol, "M")
	}
	return ParseSymbol(exchangeSymbol, marketType)
}

// SymbolRegistry - реестр конвертеров символов
type SymbolRegistry struct {
	converters map[string]ExchangeSymbolConverter
}

func NewSymbolRegistry() *SymbolRegistry {
	registry := &SymbolRegistry{
		converters: make(map[string]ExchangeSymbolConverter),
	}

	// Регистрируем стандартные конвертеры
	registry.RegisterConverter("binance", &BinanceSymbolConverter{})
	registry.RegisterConverter("kucoin", &KucoinSymbolConverter{})

	return registry
}

func (r *SymbolRegistry) RegisterConverter(exchange string, converter ExchangeSymbolConverter) {
	r.converters[exchange] = converter
}

func (r *SymbolRegistry) GetConverter(exchange string) ExchangeSymbolConverter {
	if converter, exists := r.converters[exchange]; exists {
		return converter
	}
	// Возвращаем дефолтный конвертер (как Binance)
	return &BinanceSymbolConverter{}
}

// ConvertToUnified конвертирует символ биржи в унифицированный формат
func (r *SymbolRegistry) ConvertToUnified(exchange, exchangeSymbol, marketType string) (*UnifiedSymbol, error) {
	converter := r.GetConverter(exchange)
	return converter.FromExchangeSymbol(exchangeSymbol, marketType)
}

// ConvertToExchange конвертирует унифицированный символ в формат биржи
func (r *SymbolRegistry) ConvertToExchange(exchange string, unified *UnifiedSymbol) string {
	converter := r.GetConverter(exchange)
	return converter.ToExchangeSymbol(unified)
}
