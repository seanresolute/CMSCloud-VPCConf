package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"

	"github.com/jmoiron/sqlx"
)

var ErrExpired = errors.New("Session is expired")
var ErrNotFound = errors.New("Session not found")
var ErrInvalidKey = errors.New("Invalid session key")
var expiresAtDefault = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

type SessionStore interface {
	Create(*database.Session) error
	CreateUnauthenticated() (string, error)
	Get(key string) (*database.Session, error)
	Delete(key string) error
	DeleteOldSessions(maxAgeInMinutes int) error
	Extend(*database.Session) error
	GetAWSCredentials(username, accountID string) (*credentialservice.Credentials, error)
	UpdateAWSCredentials(username, accountID string, credentials credentialservice.Credentials) error
}

type SQLSessionStore struct {
	DB *sqlx.DB
}

func (s *SQLSessionStore) DeleteOldSessions(maxAgeInMinutes int) error {
	q := fmt.Sprintf("DELETE FROM session WHERE created_at + interval '%d minutes' < NOW() AND expires_at + interval '%d minutes' < NOW();", maxAgeInMinutes, maxAgeInMinutes)
	_, err := s.DB.Exec(q)
	return err
}

func (s *SQLSessionStore) Delete(key string) error {
	k, err := hex.DecodeString(key)
	if err != nil {
		return ErrInvalidKey
	}
	_, err = s.DB.Exec("DELETE FROM session WHERE key = $1", k)
	return err
}

const KeySizeInBytes = 16

// Returns SQL with "?" placeholders so it will need to be rebound
func generateSessionSQL(sessionID int64, accounts []*database.AWSAccount) (string, []interface{}) {
	// We need to:
	//  1. Get the database ID of the aws_account row corresponding to each authorized account,
	//     creating new rows if necessary and updating project_name and name of any existing rows.
	//  2. Insert a session_aws_account for each aws_account row created/updated linking the row
	//     to the session we just created. Each session_aws_account has its own is_approver value
	//     in the authorized account list.
	// The strategy for doing this in one query is:
	//  A. Construct an INSERT query accomplishing #1 and returning the IDs of the created/updated
	//     rows along with the aws_id (for joining later).
	//  B. Construct a static table by doing
	//        SELECT <aws id>_1, <is_approver>_1 UNION SELECT <aws id>_2, <is_approver>_2 UNION ...
	//     and filling in (as parameters) aws_id and is_approver for each authorized account
	//  C. Using "WITH", join the INSERT result with the SELECT on aws_id, thus giving us:
	//       - aws_account row ID
	//       - is_approver
	//     for each account.
	//  D. Insert the result of the join into session_aws_account.
	accountInsertPlaceholders := []string{}
	accountInsertValues := []interface{}{}
	isApproverSelects := []string{}
	isApproverValues := []interface{}{}
	// Build up the pieces of (A) and (B)
	for _, account := range accounts {
		accountInsertPlaceholders = append(accountInsertPlaceholders, "(?, ?, ?, ?, false)")
		accountInsertValues = append(accountInsertValues, account.ID, account.Name, account.ProjectName, account.IsGovCloud)
		isApproverSelects = append(isApproverSelects, "SELECT ? AS aws_id, ?::boolean AS is_approver")
		isApproverValues = append(isApproverValues, account.ID, account.IsApprover)
	}
	// Construct (A).
	accountInsert := "INSERT INTO aws_account (aws_id, name, project_name, is_gov_cloud, is_inactive) VALUES " + strings.Join(accountInsertPlaceholders, ", ") + " ON CONFLICT(aws_id) DO UPDATE SET name=excluded.name, project_name=excluded.project_name, is_inactive=false RETURNING id as db_id, aws_id"
	// Construct (B).
	isApproverSelect := strings.Join(isApproverSelects, " UNION ")
	// Do the JOIN and INSERT from (C) and (D).
	q := "WITH t AS (" + accountInsert + ") INSERT INTO session_aws_account (session_id, aws_account_id, is_approver) SELECT ?::integer, t.db_id, t2.is_approver FROM t INNER JOIN (" + isApproverSelect + ") t2 ON t2.aws_id=t.aws_id"

	// The placeholder for the session ID is in between all the account insert placeholders and the
	// is_approver placeholders in the final query.
	values := append(append(accountInsertValues, sessionID), isApproverValues...)
	return q, values
}

func (s *SQLSessionStore) Create(sess *database.Session) error {
	k, err := s.generateSessionID()
	if err != nil {
		return err
	}

	sess.Key = fmt.Sprintf("%x", k)

	q := "INSERT INTO session (key, user_id, cloud_tamer_token, is_admin, username, expires_at) VALUES (:key, :userID, :token, :isAdmin, :username, CURRENT_TIMESTAMP + (60 * interval '1 minute')) RETURNING id"
	args := map[string]interface{}{
		"key":      k,
		"token":    sess.CloudTamerToken,
		"userID":   sess.UserID,
		"isAdmin":  sess.IsAdmin,
		"username": sess.Username,
	}

	return s.createExtend(sess, q, args)
}

func (s *SQLSessionStore) Extend(sess *database.Session) error {
	k, err := hex.DecodeString(sess.Key)
	if err != nil {
		return ErrInvalidKey
	}

	q := "UPDATE session SET user_id = :userID, is_admin = :isAdmin, username = :username, expires_at = CURRENT_TIMESTAMP + (60 * interval '1 minute') WHERE key = :key RETURNING id"
	args := map[string]interface{}{
		"key":      k,
		"userID":   sess.UserID,
		"isAdmin":  sess.IsAdmin,
		"username": sess.Username,
	}

	return s.createExtend(sess, q, args)
}

func (s *SQLSessionStore) CreateUnauthenticated() (string, error) {
	key, err := s.generateSessionID()
	if err != nil {
		return "", err
	}

	q := "INSERT INTO session (key, user_id, cloud_tamer_token, is_admin, username, expires_at) VALUES (:key, :userID, :token, :isAdmin, :username, :expiresAt)"
	_, err = s.DB.NamedExec(q, map[string]interface{}{
		"key":       key,
		"token":     "",
		"userID":    0,
		"isAdmin":   false,
		"username":  "",
		"expiresAt": expiresAtDefault,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", key), nil
}

// The Create and Extend database interactions are identical
func (s *SQLSessionStore) createExtend(session *database.Session, insertQuery string, arguments map[string]interface{}) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	rewritten, args, err := s.DB.BindNamed(insertQuery, arguments)
	if err != nil {
		return err
	}

	var sessionID int64
	err = tx.Get(&sessionID, rewritten, args...)
	if err != nil {
		return err
	}

	if len(session.AuthorizedAccounts) > 0 {
		// delete all accounts for the sessionID; a noop for Create()
		_, err = tx.Exec("DELETE FROM session_aws_account WHERE session_id = $1", sessionID)
		if err != nil {
			return err
		}

		q, values := generateSessionSQL(sessionID, session.AuthorizedAccounts)
		q = s.DB.Rebind(q)
		stmt, err := tx.Prepare(q)
		if err != nil {
			return err
		}
		_, err = stmt.Exec(values...)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *SQLSessionStore) Get(key string) (*database.Session, error) {
	k, err := hex.DecodeString(key)
	if err != nil {
		return nil, ErrInvalidKey
	}

	q := `SELECT
			CURRENT_TIMESTAMP >= expires_at AS is_expired,
			user_id,
			cloud_tamer_token,
			is_admin,
			username,
			aws_account.aws_id,
			name,
			project_name,
			is_gov_cloud,
			is_approver,
			access_key_id,
			secret_access_key,
			session_token
		FROM session
		LEFT JOIN session_aws_account
			ON session_aws_account.session_id=session.id
		LEFT JOIN aws_account
			ON aws_account.id = session_aws_account.aws_account_id
		WHERE session.key = $1`
	rows, err := s.DB.Query(q, k)
	if err != nil {
		return nil, err
	}
	sess := &database.Session{
		Key: key,
	}
	found := false
	isExpired := true
	for rows.Next() {
		found = true
		account := &database.AWSAccount{}
		creds := &database.AWSCreds{}
		rows.Scan(
			&isExpired,
			&sess.UserID,
			&sess.CloudTamerToken,
			&sess.IsAdmin,
			&sess.Username,
			&account.ID,
			&account.Name,
			&account.ProjectName,
			&account.IsGovCloud,
			&account.IsApprover,
			&creds.AccessKeyID,
			&creds.SecretAccessKey,
			&creds.SessionToken,
		)
		if creds.AccessKeyID != "" {
			account.Creds = creds
		}
		if account.ID != "" {
			sess.AuthorizedAccounts = append(sess.AuthorizedAccounts, account)
		}
	}
	if !found {
		return nil, ErrNotFound
	}
	if isExpired {
		return nil, ErrExpired
	}

	return sess, nil
}

func (s *SQLSessionStore) generateSessionID() ([]byte, error) {
	k := make([]byte, KeySizeInBytes)
	_, err := rand.Read(k)
	if err != nil {
		return []byte{}, err
	}
	return k, nil
}

func (s *SQLSessionStore) GetAWSCredentials(username, accountID string) (*credentialservice.Credentials, error) {
	// Only return cached creds that still have 30 minutes before expiring
	q := `SELECT
			access_key_id, secret_access_key, session_token, expiration
		FROM session_aws_account
		INNER JOIN session
			ON session_aws_account.session_id = session.id
		INNER JOIN aws_account
			ON session_aws_account.aws_account_id = aws_account.id
		WHERE access_key_id IS NOT NULL
		AND session.username = $1
		AND aws_account.aws_id = $2
		AND expiration > NOW() + interval '30 minutes'
		ORDER BY session.id DESC
		LIMIT 1`

	credentials := &credentialservice.Credentials{}

	err := s.DB.QueryRowx(q, username, accountID).Scan(&credentials.AccessKeyID, &credentials.SecretAccessKey, &credentials.SessionToken, &credentials.Expiration)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return credentials, nil
}

func (s *SQLSessionStore) UpdateAWSCredentials(username, accountID string, credentials credentialservice.Credentials) error {
	q := `
	UPDATE session_aws_account
		SET access_key_id = :accessKeyID,
		secret_access_key = :secretAccessKey,
		session_token = :sessionToken,
		expiration = :expiration
	WHERE
		session_id = (SELECT id FROM session WHERE username=:username ORDER BY id DESC LIMIT 1)
		AND aws_account_id = (SELECT id FROM aws_account WHERE aws_id=:accountID)
	`
	_, err := s.DB.NamedExec(q, map[string]interface{}{
		"accessKeyID":     credentials.AccessKeyID,
		"secretAccessKey": credentials.SecretAccessKey,
		"sessionToken":    credentials.SessionToken,
		"username":        username,
		"accountID":       accountID,
		"expiration":      credentials.Expiration,
	})

	return err
}
