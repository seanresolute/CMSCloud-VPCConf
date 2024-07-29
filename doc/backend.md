# Backend Architecture

## HTTP Server
We use the basic Go HTTP server with minimal third-party middleware.

### Dynamic requests
Dynamic requests are sent to a routing function which chooses a function to handle the request by matching against regular expressions. Actions which require authentication (see below) are flagged as such and credentials are checked before calling the handler.

### Static files
Static files are handled by a minimal wrapper around [http.FileSystem](https://golang.org/pkg/net/http/#FileSystem) which strips `/ver:*/` before serving. This allows us to break browser caching with each new release by changing the `ver` prefix before each static file path.

We also embed the files into the executable with [esc](https://github.com/mjibson/esc) so that we can ship a single binary with no dependencies.

### Authentication
For paths requiring authentication (basically all dynamic paths) we require one of two methods of authentication:
- Azure AD authentication via oauth: this is for human users. It happens in a new tab, so we use a somewhat complicated flow:
  1. When an unauthenticated user (no session cookie or invalid session cookie) requests an authenticated endpoint, we create an "unauthenticated" session, which is alread expired and which does not identify any user. They are then directed to do the oauth flow in a new tab.
  2. At the end of the oauth flow, the new tab (which will use the newly set session cookie from step 1) will hit the oauth/validate endpoint, which will set the username for the session and give it a valid expiration date.
  3. Future authenticated requests (in the original tab) will use the newly valid session and will succeed.

  If a session is valid but expired, then at step 1 the server will not create a new session but will instead just start the oauth flow in a new tab (with the expired session). About an hour after a session is expired it will be garbage-collected from the database, at which point a user returning to the application must repeat the process from step 1.
- API key authentication: this is for automated users. They are not given a session cookie, but the implementation of the sever endpoints requires a session in the database for AWS account access to work. So the server creates a session. The API key used identifies a unique "principal" and the server internally caches each principal's most recent session ID in memory, to avoid the repetitive work of creating a new session with every request. If the cached ID refers to a deleted or expired session than the server simply creates a new session for that principal.

## Database

The application expects to have a Postgres database, which is used for the following things:
- Configuration and state of VPCs
- User-created VPC requests
- HTTP session information
- Non-interactive tasks and associated logs
- Task locks, used to make sure that parallel tasks don't require the same targets
- Configuration options such as security group and transit gateway templates
- Information about migrations (see below)

Most entities that are stored in the database have corresponding structs and functions to modify them in `database/models.go`.

### Migrations

When a change to database schema or a data migration is required, a new "migration" is created in `database/migrations.go`. This is a very primitive migrations system: there is no mechanism for rolling back migrations or resolving conflicts, or verification of past migrations. So when doing local development you will need to wipe your database, manually roll back migrations, or use multiple databases if you want to change them or develop multiple different migrations at the same time. And on production the only remedy for a bad migration that was successfully applied is to make a new migration that fixes the damage. Editing a migration that has already been applied will have no effect, and doing a manual rollback on production is too dangerous.

## Non-interactive tasks

Most actions that modify any infrastructure are performed non-interactively: the server writes them to the database and then a worker process performs them later. Currently the worker process is a goroutine running on the server but it could be independent.

The task definition stored in the database tells the worker process what work needs to be done and typically includes a CloudTamer token that the worker can use to get AWS credentials as needed. The server copies the CloudTamer token to the task definition from the row in the session table that corresponds to the session ID of the user who requested the tasks, so the worker will only be able to perform AWS API calls that the user is authorized to perform.

The implementations of the worker tasks typically assume that there are no concurrent modifications happening to the state of the VPCs that the task applies to. This is enforced by a lock on a table that is acquired before any task is started. Currently we only allow one task to be performed globally at a time. In the future we can make the lock based on which accounts or VPCs the task applies to so that tasks can happen concurrently.

Parallel tasks are enabled using a `LockSet` interface, implemented using a dedicated table `task_lock`. Each type of task defines a list of targets that are required by the task. These targets are locked while the task is in progress and released when the task is completed.  Tasks can be run in parallel as long as they do not require any targets currently locked by a running task.  