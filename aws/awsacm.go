package aws

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/acm"
)

func (ctx *Context) SubmitCertificateRequest(certificateInfo *database.CertificateInfo) (string, error) {
	tags := []*acm.Tag{{Key: aws.String("Automated"), Value: aws.String("true")}}

	certificateJSON, err := json.Marshal(certificateInfo)
	if err != nil {
		return "", fmt.Errorf("Marshal error in creating idempotency token for certificate request")
	}
	idempotencyToken := fmt.Sprintf("%x", sha256.Sum256(certificateJSON))[0:32]

	arn := ""
	startedTrying := ctx.clock().Now()
	maxTryTime := time.Second * 30

	for {
		output, err := ctx.ACM().RequestCertificate(&acm.RequestCertificateInput{
			DomainName:       &certificateInfo.Name,
			ValidationMethod: aws.String(acm.ValidationMethodDns),
			IdempotencyToken: aws.String(idempotencyToken),
		})
		if err == nil {
			if output.CertificateArn == nil {
				ctx.Log("RequestCertificate() returns nil err, but CertificateArn is nil")
			} else {
				arn = aws.StringValue(output.CertificateArn)
				break
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == acm.ErrCodeLimitExceededException {
				// Sleep an extra 10 seconds?
				ctx.Log("ACM error: %s", err)
				time.Sleep(10 * time.Second)
			} else if aerr.Code() == request.ErrCodeResponseTimeout || aerr.Code() == request.ErrCodeRead {
				// These are usually going to be network level errors, which we should retry, but log
				ctx.Log("ACM error: %s", err)
			} else {
				return "", err
			}
		} else {
			return "", err
		}
		if ctx.clock().Since(startedTrying) > maxTryTime {
			return "", fmt.Errorf("Timed out creating certificate")
		}
	}

	if arn == "" {
		return arn, fmt.Errorf("RequestCertificate() returned empty ARN")
	}
	// Add tags to certificate
	startedTrying = ctx.clock().Now()
	maxTryTime = time.Second * 30

	for {
		_, err = ctx.ACM().AddTagsToCertificate(&acm.AddTagsToCertificateInput{
			CertificateArn: &arn,
			Tags:           tags,
		})
		if err == nil {
			break
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == acm.ErrCodeInvalidArnException ||
				aerr.Code() == acm.ErrCodeResourceNotFoundException {
				// Wait for the certificate to show up still
			} else if aerr.Code() == request.ErrCodeResponseTimeout || aerr.Code() == request.ErrCodeRead {
				// These are usually going to be network level errors, which we should retry, but log
				ctx.Log("ACM Error: %s", err)
			} else {
				return "", err
			}
		} else {
			return "", err
		}
		if ctx.clock().Since(startedTrying) > maxTryTime {
			return "", fmt.Errorf("Timed out tagging certificate")
		}
	}
	return arn, nil
}

func (ctx *Context) GetCertificateRequestValidationInfo(arn string) (*database.ValidationRecord, error) {
	var validationInfo = &database.ValidationRecord{}
	var finalError error
	startedTrying := ctx.clock().Now()
	maxTryTime := time.Second * 30
	for {
		output, err := ctx.ACM().DescribeCertificate(&acm.DescribeCertificateInput{
			CertificateArn: aws.String(arn),
		})

		if err == nil {
			certificate := output.Certificate
			if len(certificate.DomainValidationOptions) > 1 {
				return nil, fmt.Errorf("Expected only 1 domain validation RR, got %d: %+v", len(certificate.DomainValidationOptions), certificate.DomainValidationOptions)
			} else if len(certificate.DomainValidationOptions) == 1 && certificate.DomainValidationOptions[0].ResourceRecord != nil {
				validationInfo.Subject = aws.StringValue(certificate.DomainName)
				validationInfo.Challenge = aws.StringValue(certificate.DomainValidationOptions[0].ResourceRecord.Name)
				validationInfo.Response = aws.StringValue(certificate.DomainValidationOptions[0].ResourceRecord.Value)
				validationInfo.RecordType = aws.StringValue(certificate.DomainValidationOptions[0].ResourceRecord.Type)
				break
			} else if len(certificate.DomainValidationOptions) == 0 {
				finalError = fmt.Errorf("No domain validation RRs: %s", arn)
			} else {
				finalError = fmt.Errorf("No ResourceRecord found in DomainValidation info")
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == acm.ErrCodeInvalidArnException ||
				aerr.Code() == acm.ErrCodeResourceNotFoundException {
				// Wait for the certificate to show up still
			} else if aerr.Code() == request.ErrCodeResponseTimeout || aerr.Code() == request.ErrCodeRead {
				// These are usually going to be network level errors, which we should retry, but log
				ctx.Log("ACM: %s", err)
				finalError = err
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
		if ctx.clock().Since(startedTrying) > maxTryTime {
			if finalError != nil {
				return nil, fmt.Errorf("Timed out getting certificate request validation info: %s", finalError)
			} else {
				return nil, fmt.Errorf("Timed out getting certificate request validation info")
			}
		}
	}
	return validationInfo, nil
}

func (ctx *Context) WaitForCertificateValidation(arn string) error {
	ctwt := aws.BackgroundContext()
	ctwt, cancel := context.WithTimeout(ctwt, 5*time.Minute)
	defer cancel()

	return ctx.ACM().WaitUntilCertificateValidatedWithContext(ctwt, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	},
		request.WithWaiterMaxAttempts(45))
}

func (ctx *Context) CheckDeletedCertificateStillExists(arn string) (bool, error) {
	output, err := ctx.ACM().GetCertificate(&acm.GetCertificateInput{
		CertificateArn: aws.String(arn),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == acm.ErrCodeRequestInProgressException {
				return true, nil
			}
			if aerr.Code() == acm.ErrCodeInvalidArnException || aerr.Code() == acm.ErrCodeResourceNotFoundException {
				return false, nil
			}
		} else {
			return false, err
		}
	}
	if output.Certificate != nil {
		return true, nil
	}
	return false, nil
}

func (ctx *Context) DeleteCertificate(arn string) error {
	_, err := ctx.ACM().DeleteCertificate(&acm.DeleteCertificateInput{
		CertificateArn: aws.String(arn),
	})
	return err
}
