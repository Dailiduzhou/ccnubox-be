package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class_v2/cron"
	"github.com/asynccnu/ccnubox-be/common/pkg/grpcx"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/joho/godotenv"
)

func init() { _ = godotenv.Load() }

func main() {
	app, cleanup, err := InitApp()
	if err != nil {
		panic(err)
	}
	defer cleanup()
	app.Start()
}

type App struct {
	server     grpcx.Server
	httpServer *http.Server
	metrics    *metricsx.Server
	tasks      *cron.Task
	shutdown   func(context.Context) error
	logger     logger.Logger
}

func NewApp(server grpcx.Server, httpServer *http.Server, metrics *metricsx.Server, tasks *cron.Task, shutdown func(context.Context) error, l logger.Logger) *App {
	return &App{server: server, httpServer: httpServer, metrics: metrics, tasks: tasks, shutdown: shutdown, logger: l}
}

func (app *App) Start() {
	app.tasks.StartAll()
	defer app.tasks.Stop()
	defer app.close()

	go func() {
		if err := app.metrics.Serve(); err != nil {
			app.logger.Warn("metrics server exited", logger.Error(err))
		}
	}()
	go func() {
		if err := app.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("http server exited", logger.Error(err))
		}
	}()

	if err := app.server.Serve(); err != nil {
		panic(err)
	}
}

func (app *App) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.httpServer.Shutdown(ctx); err != nil {
		panic(fmt.Sprintln("http shutdown error:", err))
	}
	if err := app.shutdown(ctx); err != nil {
		panic(fmt.Sprintln("otel shutdown error:", err))
	}
	if err := app.metrics.Close(); err != nil {
		panic(fmt.Sprintln("metrics shutdown error:", err))
	}
}
