package logging

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Entry struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

type Logger struct {
	mu      sync.Mutex
	entries []Entry
	std     *log.Logger
}

func NewLogger() *Logger {
	return &Logger{
		entries: make([]Entry, 0, 128),
		std:     log.New(os.Stdout, "[excelabel] ", log.LstdFlags|log.Lmicroseconds),
	}
}

func (logger *Logger) Info(source, message string) {
	logger.append(LevelInfo, source, message)
}

func (logger *Logger) Warn(source, message string) {
	logger.append(LevelWarn, source, message)
}

func (logger *Logger) Error(source, message string) {
	logger.append(LevelError, source, message)
}

func (logger *Logger) Snapshot() []Entry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	copied := make([]Entry, len(logger.entries))
	copy(copied, logger.entries)
	return copied
}

func (logger *Logger) append(level Level, source, message string) {
	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Source:  source,
		Message: message,
	}

	logger.mu.Lock()
	logger.entries = append(logger.entries, entry)
	logger.mu.Unlock()

	logger.std.Println(fmt.Sprintf("[%s] [%s] %s", level, source, message))
}
