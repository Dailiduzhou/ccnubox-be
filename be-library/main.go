package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/asynccnu/ccnubox-be/be-library/service"
	"github.com/asynccnu/ccnubox-be/common/pkg/grpcx"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/joho/godotenv"
)

func init() {
	// 预加载.env文件,用于本地开发
	_ = godotenv.Load()
}

func main() {
	app := InitApp()
	app.Run()
}

type App struct {
	server   grpcx.Server
	metrics  *metricsx.Server
	reminder *service.ReminderScheduler
	shutdown func(ctx context.Context) error
}

func NewApp(server grpcx.Server, metrics *metricsx.Server, reminder *service.ReminderScheduler, shutdown func(ctx context.Context) error) App {
	return App{
		server:   server,
		metrics:  metrics,
		reminder: reminder,
		shutdown: shutdown,
	}
}

func (app App) Run() {
	// 优雅关闭
	defer func() {
		if err := app.close(); err != nil {
			panic(fmt.Sprintln("shutdown error:", err))
		}
	}()
	if err := app.reminder.Start(); err != nil {
		panic(fmt.Sprintln("reminder startup error:", err))
	}

	go func() {
		if err := app.metrics.Serve(); err != nil {
			log.Printf("metrics server exit: addr=%s err=%v", app.metrics.Addr(), err)
		}
	}()

	if err := app.server.Serve(); err != nil {
		panic(err)
	}
}

func (app App) close() error {
	var results []error
	if app.reminder != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := app.reminder.Stop(ctx); err != nil {
			results = append(results, fmt.Errorf("reminder shutdown error: %w", err))
		}
		cancel()
	}
	if app.metrics != nil {
		if err := app.metrics.Close(); err != nil {
			results = append(results, fmt.Errorf("metrics shutdown error: %w", err))
		}
	}
	if app.shutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := app.shutdown(ctx); err != nil {
			results = append(results, fmt.Errorf("resource shutdown error: %w", err))
		}
		cancel()
	}
	return errors.Join(results...)
}
