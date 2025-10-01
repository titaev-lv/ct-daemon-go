package log

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// ParseLevel парсит строку в Level (case-insensitive)
func ParseLevel(s string) (Level, error) {
	switch l := normalizeLevel(s); l {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "fatal":
		return FatalLevel, nil
	default:
		return InfoLevel, fmt.Errorf("unknown log level: %s", s)
	}
}

func normalizeLevel(s string) string {
	sr := []rune(s)
	for i, c := range sr {
		if c >= 'A' && c <= 'Z' {
			sr[i] = c + 32
		}
	}
	return string(sr)
}

type Logger struct {
	module   string
	file     *os.File
	filePath string
	level    Level
}

var (
	mu          sync.Mutex
	logFile     *os.File
	logFilePath string
	globalMode        = true // если true — логируем в один файл, если false — у каждого Logger свой файл
	globalLevel       = InfoLevel
	maxLogSize  int64 = 10 * 1024 * 1024 // 10 MB по умолчанию
)

// SetMaxLogSize задаёт максимальный размер файла для ротации (в байтах)
func SetMaxLogSize(size int64) {
	mu.Lock()
	defer mu.Unlock()
	maxLogSize = size
}

// Level — уровень логирования
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// SetGlobalLevel задаёт глобальный уровень логирования
func SetGlobalLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	globalLevel = level
}

// SetLevel задаёт уровень логирования для конкретного логгера
func (l *Logger) SetLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	l.level = level
}

// SetGlobalMode переключает режим логирования: true — все в один файл, false — каждый Logger в свой
func SetGlobalMode(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	globalMode = enabled
}

// Close закрывает текущий лог-файл, если он открыт
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		err := logFile.Close()
		logFile = nil
		return err
	}
	return nil
}

// Init открывает файл для глобального логирования (если путь пустой — пишет в stdout)
func Init(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("failed to close previous log file: %w", err)
		}
		logFile = nil
	}

	if filePath == "" {
		logFilePath = ""
		return nil
	}

	dir := ""
	if filePath != "" {
		// Создать директорию для лог-файла, если не существует
		if d, err := os.Stat(filePath); err == nil && d.IsDir() {
			dir = filePath
		} else {
			// Получить директорию из пути
			for i := len(filePath) - 1; i >= 0; i-- {
				if filePath[i] == '/' {
					dir = filePath[:i]
					break
				}
			}
		}
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
		}
	}
	var err error
	logFile, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	logFilePath = filePath
	return nil
}

// NewWithFile создаёт логгер для модуля с отдельным файлом
func NewWithFile(module, filePath string) (*Logger, error) {
	mu.Lock()
	defer mu.Unlock()
	if globalMode {
		return &Logger{module: module, level: globalLevel}, nil
	}
	dir := ""
	if filePath != "" {
		// Создать директорию для лог-файла, если не существует
		if d, err := os.Stat(filePath); err == nil && d.IsDir() {
			dir = filePath
		} else {
			for i := len(filePath) - 1; i >= 0; i-- {
				if filePath[i] == '/' {
					dir = filePath[:i]
					break
				}
			}
		}
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create log directory: %w", err)
			}
		}
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file for module %s: %w", module, err)
	}
	return &Logger{module: module, file: f, filePath: filePath, level: globalLevel}, nil
}

// New создаёт логгер для модуля (использует глобальный режим)
func New(module string) *Logger {
	return &Logger{module: module, level: globalLevel}
}

func (l *Logger) output(level Level, format string, args ...interface{}) {
	mu.Lock()
	loggerLevel := l.level
	mu.Unlock()
	if level < loggerLevel {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05.000000")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s\t%s\t%s\t%s\n", ts, level.String(), l.module, msg)

	mu.Lock()
	defer mu.Unlock()
	var err error
	// --- Ротация для модульного файла ---
	if !globalMode && l.file != nil && l.filePath != "" {
		if stat, statErr := l.file.Stat(); statErr == nil && stat.Size()+int64(len(line)) > maxLogSize {
			l.file.Close()
			rotated := fmt.Sprintf("%s.%s", l.filePath, time.Now().Format("20060102_150405"))
			os.Rename(l.filePath, rotated)
			l.file, err = os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		if l.file != nil {
			_, err = l.file.WriteString(line)
		}
	} else if logFile != nil && logFilePath != "" {
		// --- Ротация для глобального файла ---
		if stat, statErr := logFile.Stat(); statErr == nil && stat.Size()+int64(len(line)) > maxLogSize {
			logFile.Close()
			rotated := fmt.Sprintf("%s.%s", logFilePath, time.Now().Format("20060102_150405"))
			os.Rename(logFilePath, rotated)
			logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		if logFile != nil {
			_, err = logFile.WriteString(line)
		}
	} else {
		_, err = fmt.Fprint(os.Stdout, line)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "log write error: %v\n", err)
	}
}

// Close закрывает файл логгера (если используется отдельный файл)
func (l *Logger) Close() error {
	mu.Lock()
	defer mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

func (l *Logger) Debug(format string, args ...interface{}) { l.output(DebugLevel, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.output(InfoLevel, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.output(WarnLevel, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.output(ErrorLevel, format, args...) }
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.output(FatalLevel, format, args...)
	os.Exit(1)
}
