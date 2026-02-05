package task

import (
	"testing"
	"time"
)

func TestTask_IsPending(t *testing.T) {
	task := &TaskEntity{TaskStatus: TaskStatusPending}
	if !task.IsPending() {
		t.Error("Expected task to be pending")
	}

	task.TaskStatus = TaskStatusRunning
	if task.IsPending() {
		t.Error("Expected task to not be pending")
	}
}

func TestTask_IsRunning(t *testing.T) {
	task := &TaskEntity{TaskStatus: TaskStatusRunning}
	if !task.IsRunning() {
		t.Error("Expected task to be running")
	}

	task.TaskStatus = TaskStatusSuccess
	if task.IsRunning() {
		t.Error("Expected task to not be running")
	}
}

func TestTask_IsFinished(t *testing.T) {
	testCases := []struct {
		status   TaskStatus
		finished bool
	}{
		{TaskStatusSuccess, true},
		{TaskStatusFailed, true},
		{TaskStatusCanceled, true},
		{TaskStatusTimeout, true},
		{TaskStatusPending, false},
		{TaskStatusRunning, false},
	}

	for _, tc := range testCases {
		task := &TaskEntity{TaskStatus: tc.status}
		if task.IsFinished() != tc.finished {
			t.Errorf("Status %s: expected finished=%v, got %v", tc.status, tc.finished, task.IsFinished())
		}
	}
}

func TestTask_CanRetry(t *testing.T) {
	testCases := []struct {
		name     string
		redo     int
		maxRedo  int
		status   TaskStatus
		expected bool
	}{
		{"can retry failed", 1, 3, TaskStatusFailed, true},
		{"can retry timeout", 2, 3, TaskStatusTimeout, true},
		{"cannot retry - max reached", 3, 3, TaskStatusFailed, false},
		{"cannot retry - success", 1, 3, TaskStatusSuccess, false},
		{"cannot retry - running", 1, 3, TaskStatusRunning, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &TaskEntity{
				Redo:       tc.redo,
				MaxRedo:    tc.maxRedo,
				TaskStatus: tc.status,
			}
			if task.CanRetry() != tc.expected {
				t.Errorf("Expected CanRetry=%v, got %v", tc.expected, task.CanRetry())
			}
		})
	}
}

func TestTask_MarkAsRunning(t *testing.T) {
	task := &TaskEntity{}
	workerID := "worker-001"

	task.MarkAsRunning(workerID)

	if task.TaskStatus != TaskStatusRunning {
		t.Errorf("Expected status Running, got %s", task.TaskStatus)
	}
	if task.WorkerID != workerID {
		t.Errorf("Expected workerID %s, got %s", workerID, task.WorkerID)
	}
	if task.StartAt == nil {
		t.Error("Expected StartAt to be set")
	}
}

func TestTask_MarkAsSuccess(t *testing.T) {
	task := &TaskEntity{}
	now := time.Now()
	task.StartAt = &now
	result := "test result"

	task.MarkAsSuccess(result)

	if task.TaskStatus != TaskStatusSuccess {
		t.Errorf("Expected status Success, got %s", task.TaskStatus)
	}
	if task.Result != result {
		t.Errorf("Expected result %s, got %s", result, task.Result)
	}
	if task.EndAt == nil {
		t.Error("Expected EndAt to be set")
	}
	if task.Cost <= 0 {
		t.Error("Expected Cost to be calculated")
	}
}

func TestTask_MarkAsFailed(t *testing.T) {
	task := &TaskEntity{Redo: 1}
	now := time.Now()
	task.StartAt = &now
	errMsg := "test error"

	task.MarkAsFailed(errMsg)

	if task.TaskStatus != TaskStatusFailed {
		t.Errorf("Expected status Failed, got %s", task.TaskStatus)
	}
	if task.ErrMsg != errMsg {
		t.Errorf("Expected error message %s, got %s", errMsg, task.ErrMsg)
	}
	if task.Redo != 2 {
		t.Errorf("Expected Redo=2, got %d", task.Redo)
	}
	if task.EndAt == nil {
		t.Error("Expected EndAt to be set")
	}
}

func TestTask_MarkAsTimeout(t *testing.T) {
	task := &TaskEntity{Redo: 0}
	now := time.Now()
	task.StartAt = &now

	task.MarkAsTimeout()

	if task.TaskStatus != TaskStatusTimeout {
		t.Errorf("Expected status Timeout, got %s", task.TaskStatus)
	}
	if task.ErrMsg == "" {
		t.Error("Expected error message to be set")
	}
	if task.Redo != 1 {
		t.Errorf("Expected Redo=1, got %d", task.Redo)
	}
}

func TestTask_MarkAsCanceled(t *testing.T) {
	task := &TaskEntity{}
	now := time.Now()
	task.StartAt = &now
	reason := "user requested"

	task.MarkAsCanceled(reason)

	if task.TaskStatus != TaskStatusCanceled {
		t.Errorf("Expected status Canceled, got %s", task.TaskStatus)
	}
	if task.ErrMsg != reason {
		t.Errorf("Expected error message %s, got %s", reason, task.ErrMsg)
	}
}

func TestTask_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		task    *TaskEntity
		wantErr bool
	}{
		{
			name: "valid task",
			task: &TaskEntity{
				TaskType:  "test",
				SubjectID: 1,
				Payload:   "data",
				Timeout:   time.Minute,
			},
			wantErr: false,
		},
		{
			name: "empty task type",
			task: &TaskEntity{
				SubjectID: 1,
				Payload:   "data",
				Timeout:   time.Minute,
			},
			wantErr: true,
		},
		{
			name: "invalid subject id",
			task: &TaskEntity{
				TaskType: "test",
				Payload:  "data",
				Timeout:  time.Minute,
			},
			wantErr: true,
		},
		{
			name: "empty payload",
			task: &TaskEntity{
				TaskType:  "test",
				SubjectID: 1,
				Timeout:   time.Minute,
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			task: &TaskEntity{
				TaskType:  "test",
				SubjectID: 1,
				Payload:   "data",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.task.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
