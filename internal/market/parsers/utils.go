package parsers

import (
	"strconv"
	"strings"
)

// Утилиты для парсеров

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
