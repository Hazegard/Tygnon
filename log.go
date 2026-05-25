package main

import (
	"fmt"
	"github.com/rs/zerolog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func GetRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(file)
}

func NewLogger() zerolog.Logger {
	root := GetRoot()
	l := zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Caller().Logger()
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return fmt.Sprintf("%s:%d", strings.TrimPrefix(file, root+"/"), line)
	}
	l = l.Level(zerolog.InfoLevel)
	return l
}
