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
		Level:      slog.LevelDebug,
		TimeFormat: time.Kitchen,
	})))

	s, err := server.NewServer()

	if err != nil {
		panic(err)
	}

	panic(s.ListenAndServe())
}
