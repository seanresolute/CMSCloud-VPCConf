package aws

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/networkfirewall/networkfirewalliface"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/aws/aws-sdk-go/service/route53resolver"
	"github.com/aws/aws-sdk-go/service/route53resolver/route53resolveriface"

	"github.com/benbjohnson/clock"
)

type Context struct {
	*AWSAccountAccess
	VPCName string
	VPCID   string
	Logger

	destructors []destructor

	// Interfaces that can be mocked out. If not mocked out they
	// will be automatically filled in with real implementations.
	Clock clock.Clock

	mu sync.Mutex
}

func (ctx *Context) clock() clock.Clock {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.Clock == nil {
		ctx.Clock = clock.New()
	}
	return ctx.Clock
}

type Logger interface {
	Log(string, ...interface{})
}

func (ctx *Context) Fail(msg string, args ...interface{}) {
	ctx.Logger.Log(msg+"\n", args...)
	for i := len(ctx.destructors) - 1; i >= 0; i-- {
		err := ctx.destructors[i]()
		if err != nil {
			ctx.Logger.Log("Error cleaning up: %s", err)
		}
	}
	if len(ctx.destructors) > 0 {
		// Repeat error message
		ctx.Logger.Log(msg+"\n", args...)
	}
}

type AWSAccountAccess struct {
	Session *session.Session

	// Interfaces that can be mocked out. If not mocked out they
	// will be automatically filled in with real implementations
	// based on Session
	ACMsvc            acmiface.ACMAPI
	EC2svc            ec2iface.EC2API
	RAMsvc            ramiface.RAMAPI
	R53Rsvc           route53resolveriface.Route53ResolverAPI
	CloudWatchLogssvc cloudwatchlogsiface.CloudWatchLogsAPI
	NFsvc             networkfirewalliface.NetworkFirewallAPI

	mu sync.Mutex
}

func (p *AWSAccountAccess) CloudWatchLogs() cloudwatchlogsiface.CloudWatchLogsAPI {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.CloudWatchLogssvc == nil {
		p.CloudWatchLogssvc = cloudwatchlogs.New(p.Session)
	}
	return p.CloudWatchLogssvc
}

func (p *AWSAccountAccess) ACM() acmiface.ACMAPI {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ACMsvc == nil {
		p.ACMsvc = acm.New(p.Session)
	}
	return p.ACMsvc
}

func (p *AWSAccountAccess) EC2() ec2iface.EC2API {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.EC2svc == nil {
		p.EC2svc = ec2.New(p.Session)
	}
	return p.EC2svc
}

func (p *AWSAccountAccess) NetworkFirewall() networkfirewalliface.NetworkFirewallAPI {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.NFsvc == nil {
		// Temporary fix: https://github.com/aws/aws-sdk-go/issues/4040
		if aws.StringValue(p.Session.Config.Region) == "us-gov-west-1" {
			p.NFsvc = networkfirewall.New(p.Session, aws.NewConfig().WithEndpoint("https://network-firewall-fips.us-gov-west-1.amazonaws.com"))
		} else {
			p.NFsvc = networkfirewall.New(p.Session)
		}
	}
	return p.NFsvc
}

func (p *AWSAccountAccess) R53R() route53resolveriface.Route53ResolverAPI {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.R53Rsvc == nil {
		p.R53Rsvc = route53resolver.New(p.Session)
	}
	return p.R53Rsvc
}

func (p *AWSAccountAccess) RAM() ramiface.RAMAPI {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.RAMsvc == nil {
		p.RAMsvc = ram.New(p.Session)
	}
	return p.RAMsvc
}
