package myproxy

type Logger interface {
	Printf(format string, v ...interface{})
}
