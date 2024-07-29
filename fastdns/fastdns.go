package fastdns

import (
	"fmt"
	"log"
	"strings"
	"time"

	dnsv2 "github.com/akamai/AkamaiOPEN-edgegrid-golang/configdns-v2"
	"github.com/akamai/AkamaiOPEN-edgegrid-golang/edgegrid"
)

const DefaultTTL = 300
const DefaultValidationTTL = 3600

type Logger interface {
	Log(string, ...interface{})
}

type ConsoleLogger struct{}

func (t *ConsoleLogger) Log(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

type FastDNS interface {
	Init(config edgegrid.Config)
	DomainRecordExists(l Logger, zone, name, recordType string) (bool, error)
	CreateDomainRecord(l Logger, zone, name, recordType string, target []string, ttl int) error
	DeleteDomainRecord(l Logger, zone, name, recordType string) error
}

type fastDNSAPI struct {
}

func NewFastDNSAPI(config edgegrid.Config) FastDNS {
	f := &fastDNSAPI{}
	f.Init(config)
	return f
}

func (f *fastDNSAPI) Init(config edgegrid.Config) {
	dnsv2.Init(config)
}

func (f *fastDNSAPI) DomainRecordExists(l Logger, zone, name, recordType string) (bool, error) {
	if zone == "" || name == "" || recordType == "" {
		return false, fmt.Errorf("Invalid parameters to fastdns.DomainRecordExists")
	}
	startedTrying := time.Now()
	maxTryTime := time.Second * 30

	trimmedName := strings.TrimSuffix(name, ".")

	for {
		if _, err := dnsv2.GetRecord(zone, trimmedName, recordType); err == nil {
			// Record exists, return true
			return true, nil
		} else {
			if aerr, ok := err.(dnsv2.ConfigDNSError); ok {
				if aerr.Network() {
					// Network error - try again
					l.Log("FastDNS: %s", err)
					time.Sleep(time.Second)
				} else if aerr.NotFound() {
					// Record not found, return false
					return false, nil
				} else {
					return false, err
				}
			} else {
				return false, err
			}
		}
		if time.Since(startedTrying) > maxTryTime {
			return false, fmt.Errorf("Timed out checking domain record")
		}
	}
}

func (f *fastDNSAPI) CreateDomainRecord(l Logger, zone, name, recordType string, target []string, ttl int) error {
	if zone == "" || name == "" || recordType == "" || ttl == 0 {
		return fmt.Errorf("Invalid parameters to fastdns.CreateDomainRecord")
	}
	trimmedName := strings.TrimSuffix(name, ".")

	newRecord := dnsv2.RecordBody{
		Name:       trimmedName,
		RecordType: recordType,
		Target:     target,
		TTL:        ttl,
	}
	startedTrying := time.Now()
	maxTryTime := time.Second * 30

	for {
		if err := newRecord.Save(zone); err == nil {
			break
		} else {
			if aerr, ok := err.(dnsv2.ConfigDNSError); ok {
				if aerr.Network() {
					// Network error - try again
					l.Log("FastDNS: %s", err)
					time.Sleep(time.Second)
				} else {
					return err
				}
			} else {
				return err
			}
		}
		if time.Since(startedTrying) > maxTryTime {
			return fmt.Errorf("Timed out creating domain record")
		}
	}
	return nil
}

func (f *fastDNSAPI) DeleteDomainRecord(l Logger, zone, name, recordType string) error {
	if zone == "" || name == "" || recordType == "" {
		return fmt.Errorf("Invalid parameters to fastdns.DeleteDomainRecord")
	}
	trimmedName := strings.TrimSuffix(name, ".")

	recordBody := &dnsv2.RecordBody{
		Name:       trimmedName,
		RecordType: recordType,
	}

	startedTrying := time.Now()
	maxTryTime := time.Second * 30

	for {
		if err := recordBody.Delete(zone); err == nil {
			break
		} else {
			if aerr, ok := err.(dnsv2.ConfigDNSError); ok {
				if aerr.Network() {
					// Network error - try again
					l.Log("FastDNS: %s", err)
					time.Sleep(time.Second)
				} else {
					return err
				}
			} else {
				return err
			}
		}
		if time.Since(startedTrying) > maxTryTime {
			return fmt.Errorf("Timed out deleting domain record")
		}
	}
	return nil
}
