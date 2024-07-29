package database

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

type migration interface {
	fn(tx *sqlx.Tx, forward bool) error
}

type stringMigration struct {
	do   []string
	undo []string
}

func (sm *stringMigration) fn(tx *sqlx.Tx, forward bool) error {
	var statements []string
	if forward {
		statements = sm.do
	} else {
		statements = sm.undo
	}

	for _, sql := range statements {
		_, err := tx.Exec(sql)
		if err != nil {
			return err
		}
	}

	return nil
}

type funcMigration struct {
	do   func(tx *sqlx.Tx) error
	undo func(tx *sqlx.Tx) error
}

func (fm *funcMigration) fn(tx *sqlx.Tx, forward bool) error {
	var err error

	if fm.do == nil || fm.undo == nil {
		err = fmt.Errorf("Migration function is nil")
	} else if forward {
		err = fm.do(tx)
	} else if !forward {
		err = fm.undo(tx)
	}

	return err
}

// Migrate performs migrations to the desired level
func Migrate(db *sqlx.DB) error {
	err := initialSchema(db)
	if err != nil {
		return err
	}

	maxMigrationID := -1

	migrations := getMigrations()

	if len(migrations) > 0 {
		maxMigrationID = len(migrations) - 1
	}
	if targetMigrationIdx > maxMigrationID {
		panic(fmt.Sprintf("Migrate requires a targetIdx no larger than %d, but got %d", len(migrations), targetMigrationIdx))
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	committed := false

	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec("LOCK TABLE migrations")
	if err != nil {
		return err
	}

	var currentIdx *int
	err = tx.Get(&currentIdx, "SELECT migration_index FROM migrations ORDER BY id DESC")
	if err != nil {
		return err
	}

	if *currentIdx == targetMigrationIdx { // no migration necessary
		return nil
	}

	forward := *currentIdx < targetMigrationIdx

	if forward { // add migrations
		log.Printf("DB MIGRATION: upgrading database from %d to %d", *currentIdx, targetMigrationIdx)
		for idx := *currentIdx + 1; idx < targetMigrationIdx+1; idx++ {
			err := migrations[idx].fn(tx, true)
			if err != nil {
				return err
			}
			_, err = tx.Exec("INSERT INTO migrations (migration_index) VALUES ($1)", idx)
			if err != nil {
				return err
			}
			log.Printf("DB MIGRATION: completed upgrade to %d", idx)
		}
	} else { // unwind migrations
		log.Printf("DB MIGRATION: downgrading database from %d to %d", *currentIdx, targetMigrationIdx)
		for idx := *currentIdx; idx > targetMigrationIdx; idx-- {
			err := migrations[idx].fn(tx, false)
			if err != nil {
				return err
			}
			_, err = tx.Exec("INSERT INTO migrations (migration_index) VALUES ($1)", idx-1)
			if err != nil {
				return err
			}
			log.Printf("DB MIGRATION: completed downgrade to %d", idx-1)
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	committed = true

	return nil
}

// bootstrap a new database
var bootstrap = []string{
	`CREATE TABLE migrations (
			id serial PRIMARY KEY,
			migration_index int NOT NULL,
			applied_at timestamp with time zone DEFAULT current_timestamp)`,
	`INSERT INTO migrations (migration_index) VALUES (-1)`,
}

func initialSchema(db *sqlx.DB) error {
	rows, err := db.Query("SELECT 1 FROM information_schema.tables WHERE table_name = 'migrations'")
	if err != nil {
		panic(err)
	}
	if !rows.Next() {
		for _, sql := range bootstrap {
			_, err := db.Exec(sql)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

const targetMigrationIdx int = 3

// These first few migrations are mostly to test additions and unwinding
func getMigrations() []migration {
	return []migration{
		&stringMigration{ // 0
			do: []string{
				`CREATE TABLE tasks (
					id serial PRIMARY KEY,
					batch_task_id int NOT NULL,
					batch_task_name varchar NOT NULL,
					completed_at timestamp with time zone,
					initiated_at timestamp with time zone DEFAULT current_timestamp
				)`,
			},
			undo: []string{
				`DROP TABLE tasks`,
			},
		},
		&stringMigration{ // 1
			do: []string{
				`ALTER TABLE tasks ADD COLUMN success boolean DEFAULT false`,
				`UPDATE tasks SET success = true WHERE completed_at IS NOT NULL`,
			},
			undo: []string{
				`ALTER TABLE tasks DROP COLUMN IF EXISTS success`,
			},
		},
		&stringMigration{ // 2
			do: []string{
				`CREATE UNIQUE INDEX tasks_batch_task_id ON tasks (batch_task_id)`,
				`ALTER TABLE tasks ADD CONSTRAINT unique_batch_task_id UNIQUE USING INDEX tasks_batch_task_id`,
			},
			undo: []string{
				`ALTER TABLE tasks DROP CONSTRAINT IF EXISTS unique_batch_task_id`,
				`DROP INDEX IF EXISTS tasks_batch_task_id`,
			},
		},
		&funcMigration{ // 3
			do: func(tx *sqlx.Tx) error {
				_, err := tx.Exec("ALTER TABLE tasks ADD COLUMN other boolean DEFAULT false")

				return err
			},
			undo: func(tx *sqlx.Tx) error {
				_, err := tx.Exec("ALTER TABLE tasks DROP COLUMN IF EXISTS other")

				return err
			},
		},
	}
}
