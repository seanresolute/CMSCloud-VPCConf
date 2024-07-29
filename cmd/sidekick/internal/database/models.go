package database

import (
	"time"

	"github.com/jmoiron/sqlx"
)

// Migration is the history of all performed migrations
type Migration struct {
	ID             int       `json:"id"`
	MigrationIndex int       `json:"migration_index"`
	AppliedAt      time.Time `json:"applied_at"`
}

// Task is the internal representation and tracking of initiated tasks
type Task struct {
	ID            int        `json:"id"`
	BatchTaskID   int        `json:"batch_task_id"`
	BatchTaskName string     `json:"batch_task_name"`
	CompletedAt   *time.Time `json:"completed_at"`
	InitiatedAt   time.Time  `json:"initiated_at"`
	Success       bool       `json:"success"`
}

type Models interface {
	CreateTask(batchTaskID int, batchTaskName string) (*int, error)
	GetAllTasks() ([]*Task, error)
	GetTaskByID(id int) (*Task, error)
	GetIncompleteTasks() ([]*Task, error)
	CompleteTask(task *Task, success bool) error
}

type SQLModels struct {
	DB *sqlx.DB
}

func (s *SQLModels) GetAllMigrations() ([]*Migration, error) {
	migrations := []*Migration{}
	rows, err := s.DB.Queryx(`SELECT * FROM migrations ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		m := &Migration{}
		rows.Scan(&m.ID, &m.MigrationIndex, &m.AppliedAt)
		migrations = append(migrations, m)
	}

	return migrations, nil
}

// CreateTask and return the ID or an error
func (s *SQLModels) CreateTask(batchTaskID int, batchTaskName string) (*int, error) {
	sql, args, err := s.DB.BindNamed(`INSERT INTO tasks (batch_task_id, batch_task_name)
									  VALUES (:batchTaskID, :batchTaskName)
									  ON CONFLICT (batch_task_id) DO NOTHING
		 							  RETURNING id`,
		map[string]interface{}{
			"batchTaskID":   batchTaskID,
			"batchTaskName": batchTaskName,
		})
	if err != nil {
		return nil, err
	}

	var taskID *int

	err = s.DB.Get(&taskID, sql, args...)
	if err != nil {
		return nil, err
	}

	return taskID, nil
}

func (s *SQLModels) GetAllTasks() ([]*Task, error) {
	tasks := []*Task{}

	rows, err := s.DB.Queryx(`SELECT id, batch_task_id, batch_task_name, completed_at, initiated_at, success FROM tasks ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		t := &Task{}
		rows.Scan(&t.ID, &t.BatchTaskID, &t.BatchTaskName, &t.CompletedAt, &t.InitiatedAt, &t.Success)
		tasks = append(tasks, t)
	}

	return tasks, nil
}

func (s *SQLModels) GetTaskByID(id int) (*Task, error) {
	task := &Task{}

	q := `SELECT id, batch_task_id, batch_task_name, completed_at, initiated_at, success FROM tasks WHERE id = $1`
	err := s.DB.QueryRow(q, id).Scan(&task.ID, &task.BatchTaskID, &task.BatchTaskName, &task.CompletedAt, &task.InitiatedAt, &task.Success)
	if err != nil {
		return nil, err
	}

	return task, nil
}

// GetIncompleteTasks returns any incomplete tasks in order of execution
func (s *SQLModels) GetIncompleteTasks() ([]*Task, error) {
	tasks := []*Task{}

	rows, err := s.DB.Queryx(`SELECT id, batch_task_id, batch_task_name, completed_at, initiated_at, success FROM tasks WHERE completed_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		t := &Task{}
		rows.Scan(&t.ID, &t.BatchTaskID, &t.BatchTaskName, &t.CompletedAt, &t.InitiatedAt, &t.Success)
		tasks = append(tasks, t)
	}

	return tasks, nil
}

// CompleteTask sets completed_at to the current time and the success flag for the given taskID
func (s *SQLModels) CompleteTask(taskID int, success bool) error {
	_, err := s.DB.NamedExec("UPDATE tasks SET completed_at = NOW(), success = :success WHERE id=:id",
		map[string]interface{}{
			"id":      taskID,
			"success": success,
		})

	return err
}
