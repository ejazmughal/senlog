# _senlog_
A convinient Golang logger for [_Sentry_](https://sentry.io)

**_senlog_** is a wrapper arround [_sentry-go_](https://github.com/getsentry/sentry-go), the official _Sentry_ logging package for Go. It has an easy "inline" interface for structured logging with context. For example, consider the following log event with _sentry-go_:

```go
sentry.WithScope(func(scope *sentry.Scope) {
	scope.SetLevel(sentry.LevelInfo)
		
    scope.SetContext("character", map[string]interface{}{
		"name":        "Mighty Fighter",
		"age":         19,
		"attack_type": "melee",
	})		
	
    sentry.CaptureMessage("Character Info")	
})
```

This could be written with _senlog_ as the following single line of code:

```go
senlog.Cxt("character").Set("name", "Mighty Fighter").Set("age", 19).Set("attack_type", "melee").INF("Character Info")
```

Got it? ;-)

Along with Sentry server, senlog output can be written to console and local file. Each output type is called _destination_. In the following section you can find the example code for each destination with usage.

# Integration Example:


```go
package main

import (
	"errors"

	"github.com/ejazmughal/senlog"
	"github.com/getsentry/sentry-go"
)

func main() {

	senlog.INF("'console' destination is added by default, that's why you see this message!")

	err := senlog.AddDestination("sentry", sentry.ClientOptions{
		Dsn:       <YOUR_SENTRY_DSN>,
		Transport: senlog.NewSentryTransport(senlog.DEBUG),
		//Environment: "senlog_test",
	})
	if err != nil {
		senlog.FTL(err, "Could not add 'sentry' destinantion")
	}

	senlog.INF("'sentry' destination is added by you, This message is logged on sentry.io")

	senlog.Set("FirstName", "Ejaz").Set("LastName", "Mughal").INF("This is a log event with 'Default Context' and string value")

	senlog.Cxt("User Defined Context").Set("someID", 107).Set("SomeKey", "some text value").INF("This is an example of a user defined context with integer and string value")

	err = errors.New("Some error")
	senlog.Set("someIntegerValue", 7).ERR(err, "Example error log")

	// You could even write log parallel to a local file
	logFile := "sen.log"
	err = senlog.AddDestination("file", sentry.ClientOptions{
		Transport: senlog.NewFileTransport(logFile, logFile, senlog.INFO),
	})

	if err != nil {
		senlog.FTL(err, "Could not add 'file' destinantion")
	}

	senlog.INF("This message will be written to all three destinations: console, sentry and file")

	// You can disbale output to stdout/stderr by removing 'console' destination
	// This could be usefull in production environment
	senlog.RemoveDestination("console")

	senlog.INF("This message will be written to two destinations: sentry and file")

	senlog.RemoveDestination("sentry")

	senlog.INF("This message will be written to local file only")
}
```