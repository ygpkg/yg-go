package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/task/worker"
)

type MyHealthReporter struct {
	reportEndpoint string
}

type HealthReportData struct {
	WorkerID  string    `json:"worker_id"`
	Timestamp time.Time `json:"timestamp"`
	TaskTypes []string  `json:"task_types"`
	Status    string    `json:"status"`
}

func (r *MyHealthReporter) ReportHealth(ctx context.Context, health *worker.WorkerHealth) error {
	data := &HealthReportData{
		WorkerID:  health.WorkerID,
		Timestamp: health.Timestamp,
		TaskTypes: health.TaskTypes,
		Status:    health.Status,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal health data: %w", err)
	}

	fmt.Printf("[HealthReport] %s -> %s\n", r.reportEndpoint, string(jsonData))
	return nil
}
