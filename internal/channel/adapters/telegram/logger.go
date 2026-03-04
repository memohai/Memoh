package telegram

import (
	"fmt"
	"log/slog"
	"sync"
)

// slogBotLogger adapts slog.Logger to tgbotapi.BotLogger so library logs go through slog.
type slogBotLogger struct {
	mu  sync.RWMutex
	log *slog.Logger
}

func newSlogBotLogger(log *slog.Logger) *slogBotLogger {
	logger := &slogBotLogger{}
	logger.SetLogger(log)
	return logger
}

func (s *slogBotLogger) SetLogger(log *slog.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	s.log = log
}

func (s *slogBotLogger) current() *slog.Logger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.log == nil {
		return slog.Default()
	}
	return s.log
}

func (s *slogBotLogger) Println(v ...interface{}) {
	s.current().Warn("telegram bot sdk log", slog.String("message", fmt.Sprint(v...)))
}

func (s *slogBotLogger) Printf(format string, v ...interface{}) {
	s.current().Warn(
		"telegram bot sdk log",
		slog.String("message", fmt.Sprintf(format, v...)),
	)
}
