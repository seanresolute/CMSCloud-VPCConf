package vpcconfapi

import (
	"testing"
)

func TestBatchTaskProgress(t *testing.T) {
	bti := &BatchTaskInfo{
		Tasks: []*Task{
			{Status: "Cancelled"},
			{Status: "Failed"},
			{Status: "Failed"},
			{Status: "In progress"},
			{Status: "In progress"},
			{Status: "In progress"},
			{Status: "Queued"},
			{Status: "Queued"},
			{Status: "Queued"},
			{Status: "Queued"},
			{Status: "Successful"},
			{Status: "Successful"},
			{Status: "Successful"},
			{Status: "Successful"},
			{Status: "Successful"},
		},
	}

	progress := bti.GetBatchTaskProgress()

	if progress.Cancelled != 1 {
		t.Errorf("Expected there be 1 cancelled status, but got %d", progress.Cancelled)
	}
	if progress.Failed != 2 {
		t.Errorf("Expected there be 2 failed statuses, but got %d", progress.Failed)
	}
	if progress.InProgress != 3 {
		t.Errorf("Expected there be 3 in progress statuses, but got %d", progress.InProgress)
	}
	if progress.Queued != 4 {
		t.Errorf("Expected there be 4 queued statuses, but got %d", progress.Queued)
	}
	if progress.Success != 5 {
		t.Errorf("Expected there be 5 successful statuses, but got %d", progress.Success)
	}
	if progress.Remaining() != 7 {
		t.Errorf("Expected there be 7 remaining statuses, but got %d", progress.Remaining())
	}
}
