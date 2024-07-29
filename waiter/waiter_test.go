package waiter

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDone(t *testing.T) {
	w := &Waiter{SleepDuration: time.Millisecond * 1, StatusInterval: time.Millisecond * 2, Timeout: time.Second * 10}
	executions := 0

	err := w.Wait(func() Result {
		executions++
		if executions == 6 {
			return Done()
		}
		return Continue("TestDone Continue...")
	})

	if err != nil {
		t.Errorf("Expected no error, but got %q", err)
	}
}

func TestError(t *testing.T) {
	errorString := "This is an error"

	w := &Waiter{SleepDuration: time.Millisecond * 1, StatusInterval: time.Millisecond * 2, Timeout: time.Second}
	executions := 0

	err := w.Wait(func() Result {
		executions++
		if executions == 5 {
			return Error(fmt.Errorf(errorString))
		}
		return Continue("TestError Continue...")
	})

	if err == nil {
		t.Errorf("Expected an error, but didn't get one")
	}
	if err.Error() != errorString {
		t.Errorf("Expected the error to be %q, but got %q", err, errorString)
	}
	if executions != 5 {
		t.Errorf("Expected func to be executed 5 times before cancelled, but got %d", executions)
	}
}

func TestTimeout(t *testing.T) {
	w := &Waiter{SleepDuration: time.Millisecond * 1, StatusInterval: time.Millisecond * 2, Timeout: time.Millisecond * 10}

	err := w.Wait(func() Result {
		return Continue("TestTimeout Continue...")
	})
	if err == nil {
		t.Errorf("Expected an error, but didn't get one")
	}
	if err != nil && !strings.HasPrefix(err.Error(), "Timeout of") {
		t.Errorf("Expected error string to have the prefix of 'Timeout of', but got %q", err)
	}
}
