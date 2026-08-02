package cron

import (
	"testing"

	"github.com/asynccnu/ccnubox-be/be-class/pkg/logx"
	"github.com/robfig/cron/v3"
)

func TestAddTask(t *testing.T) {
	taskManager := &Task{c: cron.New(), logger: logx.Nop()}
	if err := taskManager.AddTask("* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}
	if got := len(taskManager.c.Entries()); got != 1 {
		t.Fatalf("registered entries = %d, want 1", got)
	}
	if err := taskManager.AddTask("invalid", func() {}); err == nil {
		t.Fatal("AddTask() accepted invalid cron expression")
	}
}
