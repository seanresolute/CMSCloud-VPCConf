package session

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

func TestGenerateSessionSQL(t *testing.T) {
	accounts := []*database.AWSAccount{
		{
			ID:          "12",
			Name:        "n12",
			ProjectName: "p12",
			IsGovCloud:  false,
			IsApprover:  true,
		},
		{
			ID:          "34",
			Name:        "n34",
			ProjectName: "p34",
			IsGovCloud:  false,
			IsApprover:  false,
		},
		{
			ID:          "56",
			Name:        "n56",
			ProjectName: "p56",
			IsGovCloud:  true,
			IsApprover:  false,
		},
		{
			ID:          "78",
			Name:        "n78",
			ProjectName: "p78",
			IsGovCloud:  true,
			IsApprover:  true,
		},
		{
			ID:          "90",
			Name:        "n90",
			ProjectName: "p90",
			IsGovCloud:  false,
			IsApprover:  false,
		},
	}
	expectedQuery := `
		WITH t AS (
			INSERT INTO aws_account (aws_id, name, project_name, is_gov_cloud, is_inactive)
			VALUES
				(?, ?, ?, ?, false),
				(?, ?, ?, ?, false),
				(?, ?, ?, ?, false),
				(?, ?, ?, ?, false),
				(?, ?, ?, ?, false)
			ON CONFLICT(aws_id) DO UPDATE SET name=excluded.name, project_name=excluded.project_name, is_inactive=false RETURNING id as db_id, aws_id)
		INSERT INTO session_aws_account
			(session_id, aws_account_id, is_approver)
			SELECT ?::integer, t.db_id, t2.is_approver
			FROM t
			INNER JOIN (
				SELECT ? AS aws_id, ?::boolean AS is_approver
				UNION SELECT ? AS aws_id, ?::boolean AS is_approver
				UNION SELECT ? AS aws_id, ?::boolean AS is_approver
				UNION SELECT ? AS aws_id, ?::boolean AS is_approver
				UNION SELECT ? AS aws_id, ?::boolean AS is_approver) t2
			ON t2.aws_id=t.aws_id`
	expectedValues := []interface{}{
		// INSERTs
		"12", "n12", "p12", false,
		"34", "n34", "p34", false,
		"56", "n56", "p56", true,
		"78", "n78", "p78", true,
		"90", "n90", "p90", false,
		// Session ID
		int64(101),
		// SELECTs
		"12", true,
		"34", false,
		"56", false,
		"78", true,
		"90", false,
	}
	gotQuery, gotValues := generateSessionSQL(101, accounts)
	whitespace := regexp.MustCompile(`\s+`)
	leadingWhitespace := regexp.MustCompile(`(^|\()\s*`)

	gotQuery = whitespace.ReplaceAllString(leadingWhitespace.ReplaceAllString(gotQuery, "$1"), " ")
	expectedQuery = whitespace.ReplaceAllString(leadingWhitespace.ReplaceAllString(expectedQuery, "$1"), " ")

	if expectedQuery != gotQuery {
		t.Errorf("Got incorrect query from generateSessionSQL.\nExpected:\n%s\nGot:\n%s", expectedQuery, gotQuery)
	}
	if !reflect.DeepEqual(expectedValues, gotValues) {
		t.Errorf("Got incorrect values from generateSessionSQL.\nExpected:\n%v\nGot:\n%v", expectedValues, gotValues)
	}
}
