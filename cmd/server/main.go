package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/adrianliechti/tunnel/pkg/server"
	"github.com/lmittmann/tint"
)

func main() {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.Kitchen,
	})))

	s := server.NewServer()
	panic(s.ListenAndServe())
}
