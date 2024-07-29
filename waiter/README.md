# waiter

Provides a general user configurable wait interface.

## ```NewDefaultWaiter()```

This constructs a waiter instance with the predefined parameters.

## ```Wait(fn func() Result) error```

Execute the provided function and check the Result for the next action or timeout.

## ```Result```
```
type Result struct {
	resultType resultType
	message    string
	err        error
}
```

The Result struct or the appropriate type can be created with the provided helper functions.

### ```Continue(message string) Result```
Waiting continues and if the ```WaitOptions.StatusInterval``` has been met the provided message is printed to the log.

`message` can be an empty string if you wish to continue without logging.

### ```Done() Result```
Waiting exits and returns nil.

### ```Error(err error) Result```
Exit and return the given error to the caller.

# Example

```
w := &waiter.Waiter(SleepInterval: time.Second * 1,
    StatusInterval: time.Second * 10,
    Timeout: time.Minute * 1)

err := w.Wait(func() Result {
    status, err := checkSomething()

    if err != nil {
        return Error(err)
    }
    if status == "OK" {
        return Done()
    }

    return Continue(fmt.Sprintf("Current status:  %s", status))
})

if err != nil {
    // do error handling
}

```

