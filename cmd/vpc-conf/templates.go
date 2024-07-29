package main

//go:generate esc -o gentpl.go -pkg main -prefix esc/ -private esc/

import (
	"html/template"
	text_template "text/template"
	"time"
)

func getLatestStaticModTime() time.Time {
	latestModTime := time.Time{}
	for _, item := range _escData {
		modTime := item.ModTime()
		if modTime.After(latestModTime) {
			latestModTime = modTime
		}
	}
	return latestModTime
}

var staticAssetsVersion = getLatestStaticModTime().Unix()

var tplIndex = template.Must(template.New("index").Parse(_escFSMustString(false, "/templates/index.tpl")))
var tplAuth = template.Must(template.New("auth").Parse(_escFSMustString(false, "/templates/auth.tpl")))

func (s *Server) templateIndex() *template.Template {
	if s.ReparseTemplates {
		tplIndex = template.Must(template.New("index").Parse(_escFSMustString(true, "/templates/index.tpl")))
	}
	return tplIndex
}

func (s *Server) templateAuth() *template.Template {
	if s.ReparseTemplates {
		tplAuth = template.Must(template.New("auth").Parse(_escFSMustString(true, "/templates/auth.tpl")))
	}
	return tplAuth
}

var tplVPCRequestTicket = text_template.Must(text_template.New("vpcRequestTicket").Parse(
	`
*Requester:*
{{.RequesterName}}
{{.RequesterUID}}
{{.RequesterEmail}}

*Account Details:*
CloudTamer Project: {{.ProjectName}}
Name: {{.AccountName}}
ID: {{.AccountID}}

*VPC Details:*
Name: {{.RequestedConfig.VPCName}}
Environment: {{.RequestedConfig.Stack}}
Region: {{.RequestedConfig.AWSRegion}}
Tenancy: {{if .RequestedConfig.IsDefaultDedicated}}dedicated{{else}}shared{{end}}
Availability Zones: {{.RequestedConfig.NumPrivateSubnets}}
Private Subnet Size: /{{.RequestedConfig.PrivateSize}}
Public Subnet Size: /{{.RequestedConfig.PublicSize}}
Add Container Subnets: {{.RequestedConfig.AddContainersSubnets}}
Add Network Firewall: {{.RequestedConfig.AddFirewall}}

*Related JIRA Issues:*
{{range .RelatedIssues}}{{printf "%s\n" .}}{{end}}

{{if .IPJustification}}
*IP Space Justification:*
{{.IPJustification}}
{{end}}

*Additional comments/questions:*
{{.Comment}}
`))

var tplAdditionalSubnetsRequestTicket = text_template.Must(text_template.New("additionalSubnetsRequestTicket").Parse(
	`
*Requester:*
{{.RequesterName}}
{{.RequesterUID}}
{{.RequesterEmail}}

*Account Details:*
CloudTamer Project: {{.ProjectName}}
Name: {{.AccountName}}
ID: {{.AccountID}}

*Subnet Details:*
SubnetType: {{.RequestedConfig.SubnetType}}
SubnetSize: {{.RequestedConfig.SubnetSize}}
GroupName: {{.RequestedConfig.GroupName}}

*Related JIRA Issues:*
{{range .RelatedIssues}}{{printf "%s\n" .}}{{end}}

*Additional comments/questions:*
{{.Comment}}
`))
