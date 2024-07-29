package testmocks

import (
	"fmt"
	"log"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type MockTask struct {
	ID                uint64
	Status            database.TaskStatus
	LastLoggedMessage string
}

func (t *MockTask) Log(msg string, args ...interface{}) {
	t.LastLoggedMessage = fmt.Sprintf(msg, args...)
	log.Printf(msg, args...)
}

func (t *MockTask) SetStatus(status database.TaskStatus) error {
	t.Status = status
	return nil
}

func (t *MockTask) GetID() uint64 {
	return t.ID
}
