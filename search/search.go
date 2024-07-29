package search

import (
	"fmt"
	"net"
	"regexp"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/jmoiron/sqlx"
)

type AccountResult struct {
	Name    string
	URL     string
	Project string
	VPCs    []*VPCResult
}

type VPCResult struct {
	Name          string
	URL           string
	AWSConsoleURL string
	VPCType       *database.VPCType
}

type SearchResult struct {
	ElapsedTime string
	Results     []*AccountResult
	SearchTerm  string
}

type SearchManager struct {
	DB         *sqlx.DB
	PathPrefix string
}

var cidrIPMatcher = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?:\/\d{2,3})?$`)

func Search(sm *SearchManager, searchTerm string, authorizedAccounts []*database.AWSAccount) (*SearchResult, error) {
	timeStart := time.Now()
	var searchResult *SearchResult
	query := `SELECT v.aws_id, v.name, v.aws_region, v.state IS NOT NULL, v.state -> 'VPCType', aa.aws_id, aa.name, aa.project_name
			  FROM vpc v
			  RIGHT JOIN aws_account aa ON
			    v.aws_account_id = aa.id
			    AND (v.is_deleted IS NULL OR v.is_deleted IS FALSE)
			  WHERE ((v.name ILIKE :term)
			  OR (v.aws_id ILIKE :term)
			  OR (aa.aws_id LIKE :term)
			  OR (aa.name ILIKE :term)
			  OR (aa.project_name ILIKE :term))
			  AND aa.aws_id IN (:authorizedAccountIDs)
			  ORDER BY aa.project_name, v.name ASC`
	term := fmt.Sprintf("%%%s%%", searchTerm)

	if cidrIPMatcher.Match([]byte(searchTerm)) {
		query = `SELECT v.aws_id, v.name, v.aws_region, v.state IS NOT NULL, v.state -> 'VPCType', aa.aws_id, aa.name, aa.project_name
				FROM vpc v
				INNER JOIN aws_account aa ON v.aws_account_id = aa.id
				INNER JOIN vpc_cidr vc on v.id = vc.vpc_id
				WHERE vc.cidr >>= :term
				AND v.is_deleted IS FALSE
				AND aa.aws_id IN (:authorizedAccountIDs)
				ORDER BY aa.project_name, v.name ASC`
		term = searchTerm

		ip := net.ParseIP(term)
		_, _, err := net.ParseCIDR(term)
		if ip == nil && err != nil {
			return &SearchResult{SearchTerm: term}, err
		}
	}

	authorizedAccountIDs := []string{}
	for _, account := range authorizedAccounts {
		authorizedAccountIDs = append(authorizedAccountIDs, account.ID)
	}

	args := map[string]interface{}{
		"term":                 term,
		"authorizedAccountIDs": authorizedAccountIDs,
	}

	searchResult, err := doSearch(sm, query, args)
	if err != nil {
		return searchResult, err
	}

	searchResult.SearchTerm = searchTerm
	searchResult.ElapsedTime = time.Since(timeStart).Round(time.Millisecond).String()

	return searchResult, err
}

func doSearch(sm *SearchManager, query string, args map[string]interface{}) (*SearchResult, error) {
	query, namedArgs, err := sqlx.Named(query, args)
	if err != nil {
		return nil, err
	}
	query, namedArgs, err = sqlx.In(query, namedArgs...)
	if err != nil {
		return nil, err
	}
	query = sm.DB.Rebind(query)
	rows, err := sm.DB.Query(query, namedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	searchResult := &SearchResult{}
	resultItem := &AccountResult{}

	for rows.Next() {
		var vpcID *string
		var vpcName *string
		var vpcRegion *string
		var accountID *string
		var accountName *string
		var projectName *string
		var hasState *bool
		var vpcType *database.VPCType

		err = rows.Scan(&vpcID, &vpcName, &vpcRegion, &hasState, &vpcType, &accountID, &accountName, &projectName)
		if err != nil {
			return nil, err
		}
		if resultItem.Name != *accountName {
			if resultItem.Name != "" {
				searchResult.Results = append(searchResult.Results, resultItem)
			}
			resultItem = &AccountResult{
				Name:    *accountName,
				Project: *projectName,
				URL:     fmt.Sprintf("%saccounts/%s", sm.PathPrefix, *accountID),
			}
		}
		if vpcID != nil {
			vpcResult := &VPCResult{
				VPCType:       vpcType,
				AWSConsoleURL: fmt.Sprintf("%saccounts/%s/console?region=%s&vpc=%s", sm.PathPrefix, *accountID, *vpcRegion, *vpcID),
			}

			if *vpcName != "" { // automated VPCs use name
				vpcResult.Name = *vpcName
			} else { // non-automated VPCs use the AWS vpc ID
				vpcResult.Name = *vpcID
			}
			if *hasState && *vpcType != database.VPCTypeException { // only automated VPCs get a vpc-conf link
				vpcResult.URL = fmt.Sprintf("%saccounts/%s/vpc/%s/%s", sm.PathPrefix, *accountID, *vpcRegion, *vpcID)
			}

			resultItem.VPCs = append(resultItem.VPCs, vpcResult)
		}
	}

	if resultItem.Name != "" {
		searchResult.Results = append(searchResult.Results, resultItem)
	}

	return searchResult, nil
}
