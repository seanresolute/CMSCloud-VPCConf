package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/apikey"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

type mockSTS struct {
	stsiface.STSAPI
	ExpectedRoleARN string
	ExpectedCreds   *sts.Credentials
}

type testRequest struct {
	ExtraField  string
	AccountID   string
	SessionName string
	Role        string
}

func stringPointer(str string) *string {
	return &str
}

const expectedAccessKeyID = "123"
const expectedSecretAccessKey = "456"
const expectedSessionToken = "789"

func (m mockSTS) AssumeRole(input *sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error) {
	if *input.RoleArn == m.ExpectedRoleARN {
		output := &sts.AssumeRoleOutput{
			Credentials: &sts.Credentials{
				AccessKeyId:     stringPointer(expectedAccessKeyID),
				SecretAccessKey: stringPointer(expectedSecretAccessKey),
				SessionToken:    stringPointer(expectedSessionToken),
			},
		}
		return output, nil
	}
	return nil, fmt.Errorf("AWS Error")
}

func verifyResponse(body io.ReadCloser) error {
	response := credsResponse{}
	json.NewDecoder(body).Decode(&response)
	expectedResponse := credsResponse{
		AccessKeyID:     expectedAccessKeyID,
		SecretAccessKey: expectedSecretAccessKey,
		SessionToken:    expectedSessionToken,
	}
	if !reflect.DeepEqual(expectedResponse, response) {
		return fmt.Errorf("Expected %+v, but got %+v", expectedResponse, response)
	}
	return nil
}

func TestHandleCreds(t *testing.T) {
	testCases := []struct {
		Name               string
		Request            credsRequest
		TestRequest        *testRequest
		Method             string
		Key                string
		ExpectedRoleARN    string
		ExpectedStatusCode int
		ExpectedErrorStr   string
	}{
		{
			Name: "Valid request for test-role",
			Request: credsRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "test-role",
			},
			Method:             http.MethodPost,
			Key:                "test-key",
			ExpectedRoleARN:    "arn:aws:iam::1234:role/test-role",
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name: "Valid request for other-role",
			Request: credsRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "other-role",
			},
			Method:             http.MethodPost,
			Key:                "other-key",
			ExpectedRoleARN:    "arn:aws:iam::1234:role/other-role",
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name:               "Invalid method",
			Method:             http.MethodGet,
			Key:                "test-key",
			ExpectedStatusCode: http.StatusBadRequest,
			ExpectedErrorStr:   "Bad Request",
		},
		{
			Name:               "Missing body",
			Method:             http.MethodPost,
			Key:                "test-key",
			ExpectedStatusCode: http.StatusBadRequest,
			ExpectedErrorStr:   "Invalid request: The following required fields are missing: AccountID, SessionName, Role",
		},
		{
			Name: "Missing required field",
			Request: credsRequest{
				SessionName: "HXR1",
				Role:        "test-role",
			},
			Method:             http.MethodPost,
			Key:                "test-key",
			ExpectedStatusCode: http.StatusBadRequest,
			ExpectedErrorStr:   "Invalid request: The following required fields are missing: AccountID",
		},
		{
			Name: "Extra field",
			TestRequest: &testRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "test-role",
				ExtraField:  "extra",
			},
			Method:             http.MethodPost,
			Key:                "test-key",
			ExpectedStatusCode: http.StatusBadRequest,
			ExpectedErrorStr:   "error parsing request: json: unknown field \"ExtraField\"",
		},
		{
			Name: "Invalid API key",
			Request: credsRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "test-role",
			},
			Method:             http.MethodPost,
			Key:                "invalid-key",
			ExpectedStatusCode: http.StatusUnprocessableEntity,
			ExpectedErrorStr:   "invalid API key",
		},
		{
			Name: "Key is not valid for requested role",
			Request: credsRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "test-role",
			},
			Method:             http.MethodPost,
			Key:                "other-key",
			ExpectedStatusCode: http.StatusUnauthorized,
			ExpectedErrorStr:   "API key is not authorized for the requested role",
		},
		{
			Name: "Role not found for key",
			Request: credsRequest{
				AccountID:   "1234",
				SessionName: "HXR1",
				Role:        "no-role",
			},
			Method:             http.MethodPost,
			Key:                "test-key",
			ExpectedStatusCode: http.StatusUnauthorized,
			ExpectedErrorStr:   "API key is not authorized for the requested role",
		},
	}

	log.SetOutput(ioutil.Discard)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			testCtx := context{
				STSClient: mockSTS{
					ExpectedRoleARN: tc.ExpectedRoleARN,
				},
				APIKey: apikey.APIKey{
					Config: []*apikey.APIKeyConfig{
						{
							Principal: "tester-1",
							Keys:      []string{"test-key"},
						},
						{
							Principal: "tester-2",
							Keys:      []string{"other-key"},
						},
					},
				},
				AuthConfig: authConfig{
					{
						Principal: "tester-1",
						Roles:     []string{"test-role"},
					},
					{
						Principal: "tester-2",
						Roles:     []string{"other-role"},
					},
					{
						Principal: "no-roles-key",
						Roles:     []string{},
					},
				},
				ARNContainer: "aws",
			}

			var b []byte
			if tc.TestRequest != nil {
				b, _ = json.Marshal(tc.TestRequest)
			} else {
				b, _ = json.Marshal(tc.Request)
			}

			req, _ := http.NewRequest(tc.Method, "/creds", bytes.NewBuffer(b))
			req.Header = http.Header{
				"Authorization": []string{
					fmt.Sprintf("Bearer %s", tc.Key),
				},
			}

			rr := httptest.NewRecorder()
			mux := http.NewServeMux()
			mux.HandleFunc("/creds", testCtx.handleCreds)
			wrapped := testCtx.APIKey.ValidateHandler(mux)
			wrapped.ServeHTTP(rr, req)

			result := rr.Result()

			if tc.ExpectedStatusCode != result.StatusCode {
				t.Errorf("Expected status %d but got status %d", tc.ExpectedStatusCode, result.StatusCode)
			}

			if result.StatusCode == 200 {
				err := verifyResponse(result.Body)
				if err != nil {
					t.Errorf("Expected response didn't match actual response. %s", err)
				}
			} else {
				actualErrorStr := strings.TrimSpace(rr.Body.String())
				if tc.ExpectedErrorStr != actualErrorStr {
					t.Errorf("Didn't get expected error message.  Expected %q, got %q", tc.ExpectedErrorStr, actualErrorStr)
				}
			}
		})
	}
}
