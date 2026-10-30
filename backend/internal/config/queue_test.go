package config

import (
	"reflect"
	"testing"
)

func TestParseQueueQueues(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    map[string]int
		invalid bool
	}{
		{"", nil, false}, {"  ", nil, false}, {"sync", map[string]int{"sync": 1}, false},
		{"default:6,critical:10", map[string]int{"default": 6, "critical": 10}, false},
		{" sync : 2 , default ", map[string]int{"sync": 2, "default": 1}, false},
		{"sync:0", nil, true}, {"sync:-1", nil, true}, {"sync:x", nil, true}, {"sync:", nil, true},
		{":2", nil, true}, {"sync,", nil, true}, {"sync,,default", nil, true}, {"sync:1:2", nil, true},
		{"sync,sync:2", nil, true}, {"bad name", nil, true}, {"sync:9999999999999999999999", nil, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseQueueQueues(tc.raw)
			if (err != nil) != tc.invalid || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v invalid=%v", got, err, tc.want, tc.invalid)
			}
		})
	}
}

func TestQueueEnvironmentStartupValidation(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("QUEUE_SCHEDULER_ENABLED", "")
	t.Setenv("QUEUE_QUEUES", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Queue.SchedulerEnabled {
		t.Fatal("scheduler must default to enabled")
	}
	t.Setenv("QUEUE_SCHEDULER_ENABLED", "false")
	t.Setenv("QUEUE_QUEUES", "sync:2")
	cfg, err = Load()
	if err != nil || cfg.Queue.SchedulerEnabled || cfg.Queue.Queues != "sync:2" {
		t.Fatalf("config=%+v err=%v", cfg.Queue, err)
	}
	t.Setenv("QUEUE_QUEUES", "sync:0")
	if _, err = Load(); err == nil {
		t.Fatal("accepted invalid queues")
	}
	t.Setenv("QUEUE_QUEUES", "sync")
	t.Setenv("QUEUE_SCHEDULER_ENABLED", "invalid")
	if _, err = Load(); err == nil {
		t.Fatal("accepted invalid scheduler flag")
	}
}
