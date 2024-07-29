package database

import (
	"fmt"
	"log"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type Target string

func TargetTask(taskID uint64) Target {
	return Target(fmt.Sprintf("task_%d", taskID))
}

// You need this if you might create a resource share on RAM for something.
const TargetAddResourceShare = Target("ram_add")

// You need this if you are going to update IPControl.
const TargetIPControlWrite = Target("ipcontrol_write")

// You need this if you are going to write to FastDNS.
const TargetFastDNSAPI = Target("fastdns_api")

// You need one of these if you are going to update a VPC's state or issues or mark it as deleted.
func TargetVPC(vpcID string) Target {
	return Target(fmt.Sprintf("vpc_%s", vpcID))
}

// a LockSet represents a set of acquired locks.
// Acquire a LockSet before starting operations involving any of the
// targets controlled by this package.
// You *must* call ReleaseAll on any LockSet you acquire; the locks will
// never time out or be released automatically.
type LockSet interface {
	HasLock(target Target) bool
	// This will attempt to acquire an additional lock. It may fail, so
	// in general this should only be used if you haven't done any work
	// yet or you are certain that no one else could have already acquired
	// the lock.
	AcquireAdditionalLock(target Target) error
	Release(target Target)
	ReleaseAll()
}

type lockSet struct {
	targets []Target
	db      *TaskDatabase
	mu      *sync.Mutex
}

func (ls *lockSet) hasLock(target Target) bool {
	for _, s := range ls.targets {
		if s == target {
			return true
		}
	}
	return false
}

func (ls *lockSet) HasLock(target Target) bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.hasLock(target)
}

// https://www.postgresql.org/docs/10/errcodes-appendix.html
const uniqueViolationCode = "23505"

func (ls *lockSet) acquire(target Target) error {
	if ls.hasLock(target) {
		return nil
	}
	_, err := ls.db.DB.NamedExec("INSERT INTO task_lock (worker_id, target_id) VALUES (:workerID, :targetID)", map[string]interface{}{
		"workerID": ls.db.WorkerID,
		"targetID": target,
	})
	if err != nil {
		if perr, ok := err.(*pq.Error); ok && perr.Code == uniqueViolationCode {
			return &TargetAlreadyLockedError{Target: target}
		}
		return fmt.Errorf("Error acquiring lock: %s", err)
	}
	ls.targets = append(ls.targets, target)
	return nil
}

func (ls *lockSet) AcquireAdditionalLock(target Target) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.acquire(target)
}

func (ls *lockSet) Release(target Target) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.release(target)
	targets := ls.targets[:]
	ls.targets = nil
	for _, t := range targets {
		if t != target {
			ls.targets = append(ls.targets, t)
		}
	}
}

func (ls *lockSet) release(target Target) {
	_, err := ls.db.DB.NamedExec("DELETE FROM task_lock WHERE worker_id=:workerID and target_id=:targetID", map[string]interface{}{
		"workerID": ls.db.WorkerID,
		"targetID": target,
	})
	if err != nil {
		log.Printf("Error releasing lock: %s", err)
	}
}

func (ls *lockSet) ReleaseAll() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, target := range ls.targets {
		ls.release(target)
	}
	ls.targets = nil
}

type TargetAlreadyLockedError struct {
	Target Target
}

func (t *TargetAlreadyLockedError) Error() string {
	return fmt.Sprintf("Target %q was already locked", t.Target)
}

func (db *TaskDatabase) getInQuery(q string, namedArgs map[string]interface{}) (string, []interface{}, error) {
	q, args, err := sqlx.Named(q, namedArgs)
	if err != nil {
		return "", nil, err
	}
	q, args, err = sqlx.In(q, args...)
	if err != nil {
		return "", nil, err
	}
	return db.DB.Rebind(q), args, nil
}

// If failure is caused by the targets being locked, the returned error
// will be a *TargetAlreadyLockedError.
// The returned LockSet is always nil (meaning no locks are held) if any error is returned.
func (db *TaskDatabase) AcquireLocks(targets ...Target) (LockSet, error) {
	// Fast path to fail without writing in most cases where we can't get the locks:
	q, args, err := db.getInQuery(`
		SELECT target_id FROM
			task_lock
		WHERE target_id IN (:targetIDs) LIMIT 1`,
		map[string]interface{}{"targetIDs": targets})
	if err != nil {
		return nil, err
	}
	rows, err := db.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var target Target
		err := rows.Scan(&target)
		if err != nil {
			return nil, err
		}
		return nil, &TargetAlreadyLockedError{Target: target}
	}

	ls := &lockSet{db: db, mu: new(sync.Mutex)}
	for _, target := range targets {
		err := ls.acquire(target)
		if err != nil {
			ls.ReleaseAll()
			return nil, err
		}
	}
	return ls, nil
}

// To be used for testing; does not actually lock the targets!
// AcquireAdditionalLock cannot be used on the returned LockSet
func GetFakeLockSet(targets ...Target) LockSet {
	return &lockSet{
		mu:      new(sync.Mutex),
		targets: targets,
	}
}
