package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

type TaskStatus int

const (
	TaskStatusQueued TaskStatus = iota
	TaskStatusInProgress
	TaskStatusSuccessful
	TaskStatusFailed
	TaskStatusCancelled
)

const MaxTasksReturned = 10

type TaskStats struct {
	OldestNotDoneAddedAt *time.Time
	NumQueued            int
	NumInProgress        int
	NumTasksReserved     int
	MaxWorkers           int
	AllWorkersAllowed    bool
	WorkersAllowed       *string
}

type LogEntry struct {
	Time    time.Time
	Message string
}

type BatchTask struct {
	ID          uint64
	Description string
	Tasks       []*Task
	AddedAt     time.Time
}

func (s TaskStatus) String() string {
	var names = [...]string{
		"Queued",
		"In progress",
		"Successful",
		"Failed",
		"Cancelled",
	}
	if s < 0 || int(s) >= len(names) {
		return "Unknown"
	}
	return names[s]
}

type TaskDatabase struct {
	DB         *sqlx.DB
	WorkerID   string
	WorkerName string
}

const taskSelect = `
	SELECT
		task.id,
		task.description,
		task.data,
		task.status,
		aws_account.aws_id AS account_id,
		vpc.aws_id AS vpc_id,
		vpc.aws_region AS aws_region,
		task.depends_on_task_id
	FROM task
	LEFT JOIN task prereq_task
		ON prereq_task.id = task.depends_on_task_id
	LEFT JOIN vpc
		ON vpc.id=task.vpc_id 
	INNER JOIN aws_account
		ON aws_account.id=COALESCE(task.aws_account_id, vpc.aws_account_id)
`

func (d *TaskDatabase) ReleaseTask(id uint64) error {
	_, err := d.DB.Exec("DELETE FROM task_reservation WHERE task_id=$1", id)
	return err
}

func (d *TaskDatabase) AllowAllWorkers() error {
	return d.allowWorkers(nil)
}

func (d *TaskDatabase) AllowOnlyWorkersWithName(name string) error {
	return d.allowWorkers(name)
}

func (d *TaskDatabase) allowWorkers(onlyName interface{}) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// Block reservations to ensure that after this function returns no
	// disallowed workers will start any further tasks.
	_, err = tx.Exec("LOCK TABLE task_reservation")
	if err != nil {
		return err
	}

	_, err = tx.Exec("INSERT INTO allow_tasks (only_worker_name) VALUES ($1) ON CONFLICT (enforce_one_row) DO UPDATE SET only_worker_name=$1", onlyName)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true

	return nil
}

func (d *TaskDatabase) AllowNoWorkers() error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// Block reservations to ensure that after this function returns no
	// disallowed workers will start any further tasks.
	_, err = tx.Exec("LOCK TABLE task_reservation")
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM allow_tasks")
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true

	return nil
}

// If a non-nil Task is returned then the LockSet will be non-nil as well
// and the caller is responsible for releasing it when they are no longer
// working on the task.
func (d *TaskDatabase) ReserveNextQueuedTask(mm ModelsManager) (*Task, LockSet, error) {
	var lockSet LockSet
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, nil, err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
			if lockSet != nil {
				lockSet.ReleaseAll() // This is not done in the transaction
			}
		}
	}()

	// NB: currently this prevents multiple processes from running this function concurrently.
	// If we want to in the future it should be fine to remove this lock, but in that case we
	// should be sure to get the locks in a consistent order (e.g. sort first) for efficiency.
	// We would also need to update CancelTasks to use LockSets instead of acquiring this lock,
	// and we would need to update this and the Allow* functions to get a lock on allow_tasks
	// instead (but just held for the next statement).
	_, err = tx.Exec("LOCK TABLE task_reservation")
	if err != nil {
		return nil, nil, err
	}

	q := "SELECT 1 FROM allow_tasks WHERE only_worker_name IS NULL OR only_worker_name=$1"
	var ignored int // need a destination for the "1"
	err = tx.Get(&ignored, q, d.WorkerName)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("This worker is not allowed to work right now")
			return nil, nil, nil
		}
		return nil, nil, err
	}

	t := &Task{
		db:     d,
		Status: TaskStatusQueued,
	}
	q = taskSelect + "WHERE task.status=$1 AND (task.depends_on_task_id IS NULL OR (prereq_task.status != $1 AND prereq_task.status != $2)) ORDER BY task.added_at ASC"
	rows, err := tx.Query(q, TaskStatusQueued, TaskStatusInProgress)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	blockedTaskTargets := map[Target]uint64{}
	for lockSet == nil && rows.Next() {
		err := rows.Scan(&t.ID, &t.Description, &t.Data, &t.Status, &t.AccountID, &t.VPCID, &t.VPCRegion, &t.dependsOnID)
		if err != nil {
			return nil, nil, err
		}
		taskData := new(TaskData)
		err = json.Unmarshal(t.Data, taskData)
		if err != nil {
			return nil, nil, fmt.Errorf("Unmarshalling error: %s", err)
		}
		targets, err := taskData.LockTargetsNeeded(mm)
		if err != nil {
			return nil, nil, fmt.Errorf("Error getting targets for task: %s", err)
		}
		targets = append(targets, TargetTask(t.ID))
		log.Printf("Task %d needs: %v", t.ID, targets)
		blocked := false
		// Check if another blocked task is waiting for one of this task's targets. If so,
		// consider this task blocked as well so as not to starve the other task.
		for _, target := range targets {
			if blockedTaskID, ok := blockedTaskTargets[target]; ok {
				log.Printf("Task %d needs %s but blocked task %d does too", t.ID, target, blockedTaskID)
				blocked = true
				break
			}
		}
		if !blocked {
			// Try to acquire all this task's targets
			lockSet, err = d.AcquireLocks(targets...)
			if err != nil {
				// We can assume lockSet is nil
				if terr, ok := err.(*TargetAlreadyLockedError); !ok {
					return nil, nil, err
				} else {
					log.Printf("Cannot do task %d because it needs %s", t.ID, terr.Target)
				}
			}
		}
		if lockSet == nil {
			// One of our targets is blocked or locked. Mark all our targets as blocked.
			for _, target := range targets {
				blockedTaskTargets[target] = t.ID
			}
			continue
		}
		// Theoretically the targets could have changed between us checking what they are
		// and acquiring them (because the targets are based on VPC state). So check again
		// now that we have a lock and no one else should be writing the state.
		newTargets, err := taskData.LockTargetsNeeded(mm)
		if err != nil {
			return nil, nil, fmt.Errorf("Error getting targets for task: %s", err)
		}
		for _, target := range newTargets {
			if !lockSet.HasLock(target) {
				log.Printf("Target %q is now needed but wasn't before!", target)
				lockSet.ReleaseAll()
				lockSet = nil
				break
			}
		}
	}
	rows.Close()
	err = rows.Err()
	if err != nil {
		return nil, nil, err
	}
	if lockSet == nil {
		// This means we went through all the tasks but didn't find one we can do
		log.Printf("No tasks to do")
		return nil, nil, nil
	}
	log.Printf("Selected task %d (%q)", t.ID, t.Description)

	q = "INSERT INTO task_reservation (task_id, reserved_by) VALUES (:taskID, :reservedBy)"
	_, err = tx.NamedExec(q, map[string]interface{}{
		"taskID":     t.ID,
		"reservedBy": d.WorkerID,
	})
	if err != nil {
		return nil, nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, nil, err
	}
	committed = true
	return t, lockSet, nil
}

func (d *TaskDatabase) AddBatchTask(description string) (uint64, error) {
	q := `INSERT INTO batch_task (description) VALUES ($1) RETURNING id`
	var id uint64
	err := d.DB.Get(&id, q, description)
	return id, err
}

func (d *TaskDatabase) getBatchTasks(beforeID *uint64, batchTaskID *uint64) ([]*BatchTask, bool, error) {
	if beforeID != nil && batchTaskID != nil {
		return nil, false, fmt.Errorf("cannot specify both beforeID and batchTaskID")
	}

	var rows *sqlx.Rows
	var err error
	params := map[string]interface{}{
		"limit": MaxTasksReturned + 1,
	}
	innerSelect := `SELECT * FROM batch_task`
	if beforeID != nil {
		innerSelect += " WHERE batch_task.added_at < (SELECT added_at FROM batch_task WHERE id=:beforeID)"
		params["beforeID"] = *beforeID
	}
	if batchTaskID != nil {
		innerSelect += ` WHERE id = :batchTaskID`
		params["batchTaskID"] = *batchTaskID
	}
	innerSelect += ` ORDER BY batch_task.added_at DESC LIMIT :limit`
	q := `
		SELECT
			batch_task.id,
			batch_task.description,
			batch_task.added_at,
			task.id,
			COALESCE(task.description, ''),
			task.data,
			COALESCE(task.status, 0),
			COALESCE(aws_account.aws_id, '') AS account_id,
			vpc.aws_id AS vpc_id,
			COALESCE(vpc.aws_region, '') AS aws_region,
			task.depends_on_task_id
		FROM (` + innerSelect + `) batch_task
		LEFT JOIN task
			ON task.batch_task_id=batch_task.id
		LEFT JOIN vpc
			ON vpc.id=task.vpc_id 
		LEFT JOIN aws_account
			ON aws_account.id=COALESCE(task.aws_account_id, vpc.aws_account_id)`
	q += "ORDER BY batch_task.added_at DESC, task.added_at DESC"
	rows, err = d.DB.NamedQuery(q, params)
	if err != nil {
		return nil, false, err
	}
	tasks := []*BatchTask{}
	var curr *BatchTask
	for rows.Next() {
		bt := &BatchTask{}
		var taskID *uint64
		t := &Task{
			db: d,
		}
		err := rows.Scan(&bt.ID, &bt.Description, &bt.AddedAt, &taskID, &t.Description, &t.Data, &t.Status, &t.AccountID, &t.VPCID, &t.VPCRegion, &t.dependsOnID)
		if err != nil {
			return nil, false, err
		}
		if curr == nil || curr.ID != bt.ID {
			curr = bt
			tasks = append(tasks, curr)
		}
		if taskID != nil {
			t.ID = *taskID
			curr.Tasks = append(curr.Tasks, t)
		}
	}
	if len(tasks) > MaxTasksReturned {
		return tasks[:MaxTasksReturned], true, nil
	}
	return tasks, false, nil
}

func (d *TaskDatabase) GetBatchTasks() ([]*BatchTask, bool, error) {
	return d.getBatchTasks(nil, nil)
}

func (d *TaskDatabase) GetBatchTasksBefore(beforeTaskID uint64) ([]*BatchTask, bool, error) {
	return d.getBatchTasks(&beforeTaskID, nil)
}

func (d *TaskDatabase) GetBatchTaskByID(batchTaskID uint64) (*BatchTask, error) {
	batchTasks, _, err := d.getBatchTasks(nil, &batchTaskID)
	if err != nil {
		return nil, err
	}
	if len(batchTasks) != 1 {
		return nil, fmt.Errorf("Expected a single batch task with ID %d to be retrieved but got %d", batchTaskID, len(batchTasks))
	}

	return batchTasks[0], nil
}

func (d *TaskDatabase) AddAccountTask(accountID, description string, data []byte, status TaskStatus) (*Task, error) {
	t := &Task{
		db:          d,
		AccountID:   accountID,
		Description: description,
		Data:        data,
		Status:      status,
	}
	q := "INSERT INTO task (aws_account_id, description, data, status) VALUES ((SELECT id FROM aws_account WHERE aws_id=:accountID), :description, :data, :status) RETURNING id"
	rewritten, args, err := d.DB.BindNamed(q, map[string]interface{}{
		"accountID":   accountID,
		"description": description,
		"data":        types.JSONText(data),
		"status":      status,
	})
	if err != nil {
		return nil, err
	}
	err = d.DB.Get(&t.ID, rewritten, args...)
	if err != nil {
		return nil, err
	}
	return t, err
}

func (d *TaskDatabase) AddDependentAccountTask(accountID, description string, data []byte, status TaskStatus, dependsOnID uint64) (*Task, error) {
	t := &Task{
		db:          d,
		AccountID:   accountID,
		Description: description,
		Data:        data,
		Status:      status,
	}
	q := "INSERT INTO task (aws_account_id, description, data, status, depends_on_task_id) VALUES ((SELECT id FROM aws_account WHERE aws_id=:accountID), :description, :data, :status, :dependsOnID) RETURNING id"
	rewritten, args, err := d.DB.BindNamed(q, map[string]interface{}{
		"accountID":   accountID,
		"description": description,
		"data":        types.JSONText(data),
		"status":      status,
		"dependsOnID": dependsOnID,
	})
	if err != nil {
		return nil, err
	}
	err = d.DB.Get(&t.ID, rewritten, args...)
	if err != nil {
		return nil, err
	}
	return t, err
}

func (d *TaskDatabase) getTasks(accountID *string, vpcID *string, beforeID *uint64) ([]*Task, bool, error) {
	var rows *sqlx.Rows
	var err error
	q := taskSelect
	params := map[string]interface{}{
		"limit": MaxTasksReturned + 1,
	}
	if accountID != nil {
		q += "WHERE task.aws_account_id = (SELECT id FROM aws_account WHERE aws_id=:accountID) "
		params["accountID"] = *accountID
	} else {
		q += "WHERE task.vpc_id = (SELECT id FROM vpc WHERE aws_id=:vpcID) "
		params["vpcID"] = *vpcID
	}
	if beforeID != nil {
		q += "AND task.added_at < (SELECT added_at FROM task WHERE id=:beforeID) "
		params["beforeID"] = *beforeID
	}
	q += "ORDER BY task.added_at DESC LIMIT :limit"
	rows, err = d.DB.NamedQuery(q, params)
	if err != nil {
		return nil, false, err
	}
	tasks := []*Task{}
	for rows.Next() {
		t := &Task{
			db: d,
		}
		err := rows.Scan(&t.ID, &t.Description, &t.Data, &t.Status, &t.AccountID, &t.VPCID, &t.VPCRegion, &t.dependsOnID)
		if err != nil {
			return nil, false, err
		}
		tasks = append(tasks, t)
	}
	if len(tasks) > MaxTasksReturned {
		return tasks[:MaxTasksReturned], true, nil
	}
	return tasks, false, nil
}

func (d *TaskDatabase) GetAccountTasks(accountID string) ([]*Task, bool, error) {
	return d.getTasks(&accountID, nil, nil)
}

func (d *TaskDatabase) GetAccountTasksBefore(accountID string, beforeTaskID uint64) ([]*Task, bool, error) {
	return d.getTasks(&accountID, nil, &beforeTaskID)
}

func (d *TaskDatabase) GetTask(id uint64) (*Task, error) {
	t := &Task{
		db: d,
	}
	q := taskSelect + "WHERE task.id=$1"
	err := d.DB.QueryRow(q, id).Scan(&t.ID, &t.Description, &t.Data, &t.Status, &t.AccountID, &t.VPCID, &t.VPCRegion, &t.dependsOnID)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (d *TaskDatabase) AddVPCTask(accountID, vpcID, description string, data []byte, status TaskStatus, batchTaskID *uint64) (*Task, error) {
	t := &Task{
		db:          d,
		AccountID:   accountID,
		Description: description,
		Data:        data,
		Status:      status,
	}
	q := "INSERT INTO task (vpc_id, description, data, status, batch_task_id) VALUES ((SELECT id FROM vpc WHERE aws_id=:vpcID), :description, :data, :status, :batchTaskID) RETURNING id"
	rewritten, args, err := d.DB.BindNamed(q, map[string]interface{}{
		"vpcID":       vpcID,
		"description": description,
		"data":        types.JSONText(data),
		"status":      status,
		"batchTaskID": batchTaskID,
	})
	if err != nil {
		return nil, err
	}
	err = d.DB.Get(&t.ID, rewritten, args...)
	if err != nil {
		return nil, err
	}
	return t, err
}

func (d *TaskDatabase) AddDependentVPCTask(accountID, vpcID, description string, data []byte, status TaskStatus, dependsOnID uint64, batchTaskID *uint64) (*Task, error) {
	t := &Task{
		db:          d,
		AccountID:   accountID,
		Description: description,
		Data:        data,
		Status:      status,
	}
	q := "INSERT INTO task (vpc_id, description, data, status, depends_on_task_id, batch_task_id) VALUES ((SELECT id FROM vpc WHERE aws_id=:vpcID), :description, :data, :status, :dependsOnID, :batchTaskID) RETURNING id"
	rewritten, args, err := d.DB.BindNamed(q, map[string]interface{}{
		"vpcID":       vpcID,
		"description": description,
		"data":        types.JSONText(data),
		"status":      status,
		"dependsOnID": dependsOnID,
		"batchTaskID": batchTaskID,
	})
	if err != nil {
		return nil, err
	}
	err = d.DB.Get(&t.ID, rewritten, args...)
	if err != nil {
		return nil, err
	}
	return t, err
}

func (d *TaskDatabase) GetVPCTasks(accountID, vpcID string) ([]*Task, bool, error) {
	return d.getTasks(nil, &vpcID, nil)
}

func (d *TaskDatabase) GetVPCTasksBefore(accountID, vpcID string, beforeTaskID uint64) ([]*Task, bool, error) {
	return d.getTasks(nil, &vpcID, &beforeTaskID)
}

func uint64InSlice(n uint64, slice []uint64) bool {
	for _, k := range slice {
		if k == n {
			return true
		}
	}
	return false
}

// Will only cancel tasks that are not currently reserved and that have status = TaskStatusQueued
func (d *TaskDatabase) CancelTasks(taskIDs []uint64) error {
	// 1. Lock the task_reservation table to prevent concurrent task reservations.
	// 2. Get a list of currently reserved tasks and exclude their IDs.
	// 3. Cancel non-excluded tasks.
	// 4. Release task_reservation lock.

	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		// We're not making any changes in the transaction, just using it to synchronize, so we
		// can commit regardless of any errors that happened outside the transaction.
		tx.Commit()
	}()

	_, err = tx.Exec("LOCK TABLE task_reservation")
	if err != nil {
		return err
	}

	reservedIDs := []uint64{}
	q := "SELECT task_id FROM task_reservation"
	rows, err := tx.Query(q)
	if err != nil {
		return err
	}
	for rows.Next() {
		var reservedID uint64
		err := rows.Scan(&reservedID)
		if err != nil {
			return err
		}
		reservedIDs = append(reservedIDs, reservedID)
	}
	for _, taskID := range taskIDs {
		if !uint64InSlice(taskID, reservedIDs) {
			q := "UPDATE task SET status=:cancelled WHERE id=:id AND status=:queued"
			_, err := d.DB.NamedExec(q, map[string]interface{}{
				"cancelled": TaskStatusCancelled,
				"queued":    TaskStatusQueued,
				"id":        taskID,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type TaskInterface interface {
	Log(msg string, args ...interface{})
	SetStatus(status TaskStatus) error
	GetID() uint64
}

type Task struct {
	db *TaskDatabase

	ID          uint64
	AccountID   string
	VPCID       *string
	VPCRegion   *string
	Description string
	Data        []byte
	Status      TaskStatus
	dependsOnID *uint64

	failMu sync.Mutex
	failed bool
}

func (t *Task) GetID() uint64 {
	return t.ID
}

func (t *Task) Fail(msg string, args ...interface{}) error {
	t.failMu.Lock()
	defer t.failMu.Unlock()
	t.failed = true
	t.Log(msg, args...)
	return t.setStatus(TaskStatusFailed)
}

func (t *Task) DependsOn() (*Task, error) {
	if t.dependsOnID == nil {
		return nil, nil
	}
	return t.db.GetTask(*t.dependsOnID)
}

func (t *Task) LogEntries() []*LogEntry {
	q := "SELECT added_at, message FROM task_log WHERE task_id=:id ORDER BY added_at ASC"
	rows, err := t.db.DB.NamedQuery(q, map[string]interface{}{
		"id": t.ID,
	})
	if err != nil {
		return []*LogEntry{
			{
				Time:    time.Now(),
				Message: fmt.Sprintf("Unable to fetch job logs: %s", err),
			},
		}
	}
	entries := []*LogEntry{}
	for rows.Next() {
		entry := &LogEntry{}
		err := rows.Scan(&entry.Time, &entry.Message)
		if err != nil {
			return []*LogEntry{
				{
					Time:    time.Now(),
					Message: fmt.Sprintf("Unable to fetch job logs: %s", err),
				},
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func (t *Task) setStatus(status TaskStatus) error {
	q := "UPDATE task SET status=:status WHERE id=:id"
	_, err := t.db.DB.NamedExec(q, map[string]interface{}{
		"status": status,
		"id":     t.ID,
	})
	return err
}

func (t *Task) SetStatus(status TaskStatus) error {
	t.failMu.Lock()
	defer t.failMu.Unlock()
	if t.failed {
		return fmt.Errorf("Task already failed")
	}
	return t.setStatus(status)
}

func (t *Task) Log(msg string, args ...interface{}) {
	q := "INSERT INTO task_log (task_id, message) VALUES (:taskID, :message)"
	t.db.DB.NamedExec(q, map[string]interface{}{
		"taskID":  t.ID,
		"message": fmt.Sprintf(msg, args...),
	})
}

func (d *TaskDatabase) GetTaskStats() (*TaskStats, error) {
	stats := &TaskStats{}
	q := "SELECT added_at FROM task WHERE status = $1 OR status = $2 ORDER BY added_at ASC LIMIT 1"
	err := d.DB.Get(&stats.OldestNotDoneAddedAt, q, TaskStatusQueued, TaskStatusInProgress)
	if err == sql.ErrNoRows {
		// All tasks are done
	} else if err != nil {
		return nil, err
	}
	q = "SELECT COUNT(*) FROM task WHERE status = $1"
	err = d.DB.Get(&stats.NumQueued, q, TaskStatusQueued)
	if err != nil {
		return nil, err
	}
	q = "SELECT COUNT(*) FROM task WHERE status = $1"
	err = d.DB.Get(&stats.NumInProgress, q, TaskStatusInProgress)
	if err != nil {
		return nil, err
	}
	q = "SELECT COUNT(*) FROM task_reservation"
	err = d.DB.Get(&stats.NumTasksReserved, q)
	if err != nil {
		return nil, err
	}

	q = "SELECT only_worker_name FROM allow_tasks LIMIT 1"
	err = d.DB.Get(&stats.WorkersAllowed, q)
	if err != nil {
		if err == sql.ErrNoRows {
			// Indicates that no workers are allowed
		} else {
			return nil, err
		}
	} else if stats.WorkersAllowed == nil {
		stats.AllWorkersAllowed = true
	}

	return stats, nil
}

func (d *TaskDatabase) GetLastSubtaskStatuses(region Region, vpcID string) (map[string]string, error) {
	// keys are simplified for the returned structure, values are the task data keys used in the database select
	taskDataMap := map[string]string{
		"Logging":        "UpdateLoggingTaskData",
		"Networking":     "UpdateNetworkingTaskData",
		"ResolverRules":  "UpdateResolverRulesTaskData",
		"SecurityGroups": "UpdateSecurityGroupsTaskData",
	}

	statuses := map[string]string{}

	for statusKey, subTaskKey := range taskDataMap {
		q := fmt.Sprintf(`SELECT status
                          FROM task
                          WHERE task.vpc_id = (SELECT id FROM vpc WHERE vpc.aws_id = :vpcID AND aws_region = :region)
                          AND (task.data->>'%s') IS NOT NULL
                          ORDER BY task.id DESC
                          LIMIT 1`, subTaskKey)
		rows, err := d.DB.NamedQuery(q, map[string]interface{}{
			"vpcID":  vpcID,
			"region": string(region),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to select last status for %s for %s: %s", subTaskKey, vpcID, err)
		}
		if rows.Next() {
			err := func() error {
				defer rows.Close()
				var status TaskStatus
				err := rows.Scan(&status)
				if err != nil {
					return fmt.Errorf("failed to scan database row %s", err)
				}
				statuses[statusKey] = status.String()
				return nil
			}()
			if err != nil {
				return nil, err
			}
		}
	}

	return statuses, nil
}
