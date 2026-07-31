package zap

type Logger struct{}

func NewProduction() (*Logger, error) {
	return &Logger{}, nil
}

func (l *Logger) Info(msg string, fields ...any) {}
func (l *Logger) Error(msg string, fields ...any) {}
