package cacheme

type Logger interface {
	Log(key string, op string)
}

type NOPLogger struct{}

func (l *NOPLogger) Log(key string, op string) {
}
