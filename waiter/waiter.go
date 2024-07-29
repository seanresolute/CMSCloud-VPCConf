// Package waiter provides a generic wait operation
package waiter

import (
	"fmt"
	"log"
	"time"
)

type Waiter struct {
	SleepDuration  time.Duration
	StatusInterval time.Duration
	Timeout        time.Duration
	Logger         Logger
}

type resultType int

type Logger interface {
	Log(string, ...interface{})
}

const (
	resultTypeDone resultType = iota
	resultTypeError
	resultTypeContinue
)

type Result struct {
	resultType resultType
	message    string
	err        error
}

func (w *Waiter) logMessage(message string) {
	if w.Logger != nil {
		w.Logger.Log(message)
	} else {
		log.Println(message)
	}
}

func Continue(message string) Result {
	return Result{resultType: resultTypeContinue, message: message}
}

func Done() Result {
	return Result{resultType: resultTypeDone}
}

func DoneWithMessage(message string) Result {
	return Result{resultType: resultTypeDone, message: message}
}
func Error(err error) Result {
	return Result{resultType: resultTypeError, err: err}
}

// Change these values with extreme care as it may have unintended consequences where NewDefaultWaiter is used
func NewDefaultWaiter() *Waiter {
	return &Waiter{SleepDuration: time.Second * 1, StatusInterval: time.Second * 5, Timeout: time.Minute * 1}
}

func (w *Waiter) Wait(fn func() Result) error {
	startTime := time.Now()
	lastStatusTime := startTime

	for {
		if time.Since(startTime) >= w.Timeout {
			return fmt.Errorf("Timeout of %s reached", w.Timeout)
		}

		result := fn()

		if result.resultType == resultTypeDone {
			if result.message != "" {
				w.logMessage(result.message)
			}
			return nil
		}
		if result.resultType == resultTypeError {
			return result.err
		}
		if result.resultType == resultTypeContinue {
			if result.message != "" && time.Since(lastStatusTime) >= w.StatusInterval {
				lastStatusTime = time.Now()
				w.logMessage(result.message)
			}
		}

		time.Sleep(w.SleepDuration)
	}
}
