package cacheme

type Logger interface {
	Log(store string, key string, op string)
}

type NOPLogger struct{}

func (l *NOPLogger) Log(store string, key string, op string) {
}
