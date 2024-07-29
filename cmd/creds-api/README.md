## Creds API
Creds API uses cross-account roles to federate into AWS accounts and provide temporary credentials.  The requested cross-account role must already be deployed in the requested account with the proper permissions.  

### Authentication

Provide a valid API key in the `Authentication` header with the format `Bearer: {api_key}`

### GET /health 
Response: 200 
```
{
	"Healthy": true
}
```

### POST /creds

If the cross-account role includes a path, you must include the full path in the `Role` field, e.g.: `{path}/{role_name}` 

Request:
```
{
  "AccountID": 123456789,
  "SessionName": "HXR1",
  "Role": “cms-cloud-admin/ct-cms-cloud-ia-operations"
}

``` 
Response: 200 
```
{
  "AccessKeyId": "ASIA2RIWNO2WZRLIT6HO",
  "SecretAccessKey": "IwFTvyoESOzfUUCgCBlY1OljUJ3e+AiZt0izM3ub",
  "SessionToken": "IQoJb3JpZ2luX2VjEC0aCXVzLWVhc3QtMSJGMEQCIAENEK"
}

```

### Running locally

Use the environment variables

```
API_KEY_CONFIG= # see vpc-automation/apikey/README.md
AWS_REGION=us-west-2
AWS_ACCESS_KEY_ID= 
AWS_SECRET_ACCESS_KEY=
AWS_SESSION_TOKEN=
```