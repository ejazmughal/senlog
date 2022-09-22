/*
BSD 2-Clause License

Copyright (c) 2022, Muhammad Ejaz Mughal
All rights reserved.

Complete license aggreement:
https://github.com/ejazmughal/senlog/blob/main/LICENSE
*/

package senlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

const loggerName = "senlog"

// log levels
const (
	DEBUG = 1
	INFO  = 2
	WARN  = 3
	ERROR = 4
	FATAL = 5
)
const FlushTimeout = 2 * time.Second

// log levels (index) to sentry levels (value) maping
var sentryLevels = [5]sentry.Level{
	sentry.LevelDebug,
	sentry.LevelInfo,
	sentry.LevelWarning,
	sentry.LevelError,
	sentry.LevelFatal}

var senlogLevels = map[sentry.Level]int{
	sentry.LevelDebug:   DEBUG,
	sentry.LevelInfo:    INFO,
	sentry.LevelWarning: WARN,
	sentry.LevelError:   ERROR,
	sentry.LevelFatal:   FATAL,
}

var hubs = make(map[string]*sentry.Hub)

func init() {

	err := AddDestination("console", sentry.ClientOptions{
		Dsn:       "",
		Transport: NewIoTransport(os.Stdout, os.Stderr, DEBUG),
	})

	if err != nil {
		fmt.Println(err, "Could not initiate log destination: console")
	}
}

func AddDestination(key string, options sentry.ClientOptions) error {

	_, exists := hubs[key]
	if exists {
		//Set("key", key).WRN("Destination key already exists")
		return errors.New("Destination key already exists: " + key)
	}

	hub := sentry.NewHub(nil, sentry.NewScope())

	client, err := sentry.NewClient(options)
	if err != nil {
		return err
	}

	hub.BindClient(client)

	hubs[key] = hub

	//Set("destination", key).INF("Log destination added")
	if options.Dsn == "" { // sentry DSN exists
		Set("destination", key).WRN("\033[5m!\033[0m Sentry client initialized with empty DSN. No events will be delivered to sentry.")
	} else {
		Set("destination", key).INF("Sentry client initialized with DSN. Events will be delivered to sentry.")
	}

	return nil
}

func RemoveDestination(key string) {

	_, exists := hubs[key]
	if !exists { // destination doesn't exist
		Set("destination", key).WRN("Log destination to remove doesn't exist")
	} else { // destination exists
		Set("destination", key).INF("About to remove log destination, no events will be delivered")
		delete(hubs, key)

	}
}

// set min log level for a destinition
func SetLogLevel(destinationKey string, minLevel int) {

	_, exists := hubs[destinationKey]
	if !exists { // destination doesn't exist
		Set("destination", destinationKey).WRN("Cannot set log level, log destination doesn't exist.")
	} else { // destination exists
		Set("destination", destinationKey).Set("LogLevel", minLevel).INF("Changing log level")

		tr := hubs[destinationKey].Client().Transport
		tr.(LeveledLogger).SetLogLevel(minLevel)
	}
}

type Context struct {
	current  string
	contexts map[string]interface{}
}

func Cxt(k string) *Context {
	x := new(Context)
	x.current = k
	x.contexts = make(map[string]interface{})
	x.contexts[k] = make(map[string]interface{})

	return x
}

func (x *Context) Cxt(k string) *Context {
	x.current = k
	x.contexts[k] = make(map[string]interface{})

	return x
}

func (x *Context) Set(k string, v interface{}) *Context {

	x.contexts[x.current].(map[string]interface{})[k] = v

	return x
}

func (x *Context) DBG(v ...interface{}) {
	capture(DEBUG, nil, x, fmt.Sprint(v...))
}

func (x *Context) INF(v ...interface{}) {
	capture(INFO, nil, x, fmt.Sprint(v...))
}

func (x *Context) WRN(v ...interface{}) {
	capture(WARN, nil, x, fmt.Sprint(v...))
}

func (x *Context) ERR(e error, v ...interface{}) {
	capture(ERROR, e, x, fmt.Sprint(v...))
}

func (x *Context) FTL(e error, v ...interface{}) {
	capture(FATAL, e, x, fmt.Sprint(v...))

	sentry.Flush(FlushTimeout)
	os.Exit(1)
}

func Set(k string, v interface{}) *Context {
	x := Cxt("Default Context")
	x.Set(k, v)
	return x
}

// Multiple parameter values will be concated without spaces!
func INF(v ...interface{}) {
	capture(INFO, nil, nil, fmt.Sprint(v...)) // 1 = level info
}

func WRN(v ...interface{}) {
	capture(WARN, nil, nil, fmt.Sprint(v...)) // 2 = level warn
}

func DBG(v ...interface{}) {
	capture(DEBUG, nil, nil, fmt.Sprint(v...))
}

func ERR(e error, v ...interface{}) {
	capture(ERROR, e, nil, fmt.Sprint(v...))
}

func FTL(e error, v ...interface{}) {
	capture(FATAL, e, nil, fmt.Sprint(v...))

	sentry.Flush(FlushTimeout)
	os.Exit(1)
}

func capture(level int, e error, x *Context, msg string) {

	event := sentry.Event{
		Timestamp: time.Now(),
		Level:     sentryLevels[level-1],
		Logger:    loggerName,
		Message:   msg,
	}

	if x != nil {
		event.Contexts = x.contexts
	}

	st := sentry.NewStacktrace()

	// drop senlog module frames
	if st != nil {
		threshold := len(st.Frames) - 1
		for ; threshold > 0 && st.Frames[threshold].Module == "github.com/ejazmughal/senlog"; threshold-- {
		}
		st.Frames = st.Frames[:threshold+1]
	}

	if e != nil {
		event.Exception = append(event.Exception, sentry.Exception{
			Value:      e.Error(),
			Type:       reflect.TypeOf(e).String(),
			Stacktrace: st,
		})
	}

	// broadcast event to all destinitions
	for _, hub := range hubs {

		if hub != nil {
			hub.CaptureEvent(&event)
		}
	}
}

type LeveledLogger interface {
	SetLogLevel(minLevel int)
	MinLogLevel() int
}

type Logger struct {
	minLevel int // Minimum severity level for logging
}

func (l *Logger) SetLogLevel(level int) {
	l.minLevel = level
}

func (l *Logger) MinLogLevel() int {
	return l.minLevel
}

func (tr *Logger) Call(SendEventFunc func(*sentry.Event), ev *sentry.Event) {

	if senlogLevels[ev.Level] < tr.minLevel {
		return
	}

	SendEventFunc(ev)
}

// CONSOLE TRANSPORT IMPLIMENTATION

type Colors struct {
	RESET_COLOR   string
	TIME_COLOR    string
	CXT_KEY_COLOR string
	STACK_COLOR   string //stacktrack
}

type ioTransport struct {
	Logger

	DbgLog *log.Logger
	InfLog *log.Logger
	WrnLog *log.Logger
	ErrLog *log.Logger
	FtlLog *log.Logger

	Colors        *Colors
	PrintRawEvent bool // Console only option, print sentry event as JSON instead of formated lines
}

// returns ioTransport with time only line prefix
func NewIoTransport(stdout io.Writer, stderr io.Writer, minLogLevel int) *ioTransport {

	t := new(ioTransport)

	t.minLevel = minLogLevel // minimum severity level for logging
	t.PrintRawEvent = false  // console only option, print sentry event as JSON instead of formated lines

	t.Colors = &Colors{ // default colors, could be changed after initialization
		RESET_COLOR:   "\033[0m",
		TIME_COLOR:    "\033[90m",
		CXT_KEY_COLOR: "\033[36m",
		STACK_COLOR:   "\033[31m",
	}

	stdout.Write([]byte(t.Colors.TIME_COLOR)) // set time color start

	t.DbgLog = log.New(stdout, "\033[95mDBG\033[37m ", //blue
		log.Lmsgprefix|log.Ltime)

	t.InfLog = log.New(stdout, "\033[92mINF\033[37m ", //green
		log.Lmsgprefix|log.Ltime)

	t.WrnLog = log.New(stdout, "\033[93mWRN\033[37m ", //yellow
		log.Lmsgprefix|log.Ltime)

	t.ErrLog = log.New(stderr, "\033[31mERR\033[37m ", //red
		log.Lmsgprefix|log.Ltime)

	t.FtlLog = log.New(stderr, "\033[91mFTL\033[37m ", //red
		log.Lmsgprefix|log.Ltime)

	return t
}

// returns ioTransport with time and date
func NewFileTransport(outFile string, errFile string, minLogLevel int) *ioTransport {

	// If the file doesn't exist, create it, or append to the file
	stdout, err := os.OpenFile(outFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		FTL(err)
	}

	var stderr *os.File
	if outFile == errFile {
		stderr = stdout
	} else {
		stderr, err = os.OpenFile(errFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			FTL(err)
		}
	}

	t := new(ioTransport)

	t.minLevel = minLogLevel // Minimum severity level for logging
	t.PrintRawEvent = false  // Console only option, print sentry event as JSON instead of formated lines

	t.Colors = &Colors{} // empty colors strings

	t.DbgLog = log.New(stdout, "DBG ",
		log.Lmsgprefix|log.LstdFlags)

	t.InfLog = log.New(stdout, "INF ",
		log.Lmsgprefix|log.LstdFlags)

	t.WrnLog = log.New(stdout, "WRN ",
		log.Lmsgprefix|log.LstdFlags)

	t.ErrLog = log.New(stderr, "ERR ",
		log.Lmsgprefix|log.LstdFlags)

	t.FtlLog = log.New(stderr, "FTL ",
		log.Lmsgprefix|log.LstdFlags)

	return t
}
func (t *ioTransport) Configure(options sentry.ClientOptions) {
}

func (t *ioTransport) SendEvent(ev *sentry.Event) {

	if senlogLevels[ev.Level] < t.minLevel {
		return
	}

	var log string

	if t.PrintRawEvent {
		//b, _ := json.Marshal(event)
		b, _ := json.MarshalIndent(ev, "", "\t")
		//fmt.Println("[SENTRY EVENT] " + string(b))
		log = string(b)
	} else {

		var out = new(out)
		if len(ev.Exception) > 0 {
			out.write(ev.Message, " | ", ev.Exception[len(ev.Exception)-1].Value) //last execption concates all error msgs
			out.writeContexts(ev.Contexts, t.Colors.CXT_KEY_COLOR, t.Colors.RESET_COLOR)
			out.writeStacktrace(*ev.Exception[0].Stacktrace, t.Colors.STACK_COLOR)
		} else {
			out.write(ev.Message)
			out.writeContexts(ev.Contexts, t.Colors.CXT_KEY_COLOR, t.Colors.RESET_COLOR)
		}
		out.write(t.Colors.TIME_COLOR) // set color for the next line time header

		log = out.String()
	}

	switch ev.Level {
	case sentry.LevelInfo:
		t.InfLog.Output(2, log)
	case sentry.LevelWarning:
		t.WrnLog.Output(2, log)
	case sentry.LevelDebug:
		t.DbgLog.Output(2, log)
	case sentry.LevelError:
		t.ErrLog.Output(2, log)
	case sentry.LevelFatal:
		t.FtlLog.Output(2, log)
	}
}

func (t *ioTransport) Flush(_ time.Duration) bool {
	return true
}

func (t *ioTransport) SetColors(c *Colors) {

	t.Colors = c
}

// output buffer
type out struct {
	bytes.Buffer
}

func (b *out) write(a ...any) {
	fmt.Fprint(b, a...)
}

// Print key value pairs of contexts
func (b *out) writeContexts(ctxs map[string]interface{}, keyColor string, resetColor string) {

	for ctxKey, ctxValue := range ctxs {
		switch ctxKey {
		case "os", "device", "runtime":
			// ignore
		default:
			//TODO: write context name (ctxKey)
			for k, v := range ctxValue.(map[string]interface{}) {
				bValue, _ := json.MarshalIndent(v, "", "\t")
				fmt.Fprintf(b, " %s%s=%s%s", keyColor, k, resetColor, bValue)
			}
		}
	}
}

func (b *out) writeStacktrace(st sentry.Stacktrace, stackColor string) {

	fmt.Fprintf(b, "\n%s%s\n", stackColor, "Stacktrace:")

	for _, f := range st.Frames {

		if f.ContextLine != "" {
			fmt.Fprintf(b, "\t%s:%d >>  %s\n", f.AbsPath, f.Lineno, strings.TrimSpace(f.ContextLine))

		} else {
			fmt.Fprintf(b, "\t%s:%d\n", f.AbsPath, f.Lineno)
		}
	}
}

type SentryTransport struct {
	httpSyncTransport *sentry.HTTPSyncTransport
	Logger
}

func NewSentryTransport(minLogLevel int) *SentryTransport {

	tr := new(SentryTransport)
	tr.httpSyncTransport = sentry.NewHTTPSyncTransport()
	tr.minLevel = minLogLevel
	return tr
}

func (tr *SentryTransport) Configure(options sentry.ClientOptions) {

	//options.Transport = nil
	tr.httpSyncTransport.Configure(options)
}

func (tr *SentryTransport) SendEvent(ev *sentry.Event) {

	tr.Call(func(ev *sentry.Event) {
		tr.httpSyncTransport.SendEvent(ev)
	}, ev)

}

func (tr *SentryTransport) Flush(t time.Duration) bool {

	return tr.httpSyncTransport.Flush(t)
}

//
//see also:
//https://stackoverflow.com/questions/67539244/how-to-report-custom-go-error-types-to-sentry
//https://stackoverflow.com/questions/51752779/sentry-go-integration-how-to-specify-error-level
//https://stackoverflow.com/questions/62199284/is-there-a-way-to-output-sentry-messages-to-the-console
//https://tech.even.in/posts/go118-error-handling

//TODO
//- time with date in file (line header)
//- time formate in Set()
//- space in print any
//- reset bg color of msg line
//review CONTS capital names
//close/defer io writers on exit

/*
// websocket transport
type WebsocketTransport struct {
	ws *websocket.Conn
	Logger
}

func NewWebsocketTransport(ws *websocket.Conn, minLogLevel int) *WebsocketTransport {

	tr := new(WebsocketTransport)

	tr.ws = ws
	tr.minLogLevel = minLogLevel
	return tr
}

func (tr *WebsocketTransport) Configure(options sentry.ClientOptions) {

}

func (tr *WebsocketTransport) SendEvent(ev *sentry.Event) {

	tr.Call(func(ev *sentry.Event) {
		if len(ev.Exception) > 0 {
			tr.ws.Write([]byte("<span style=\"color:green\">" + ev.Message + " | " + ev.Exception[len(ev.Exception)-1].Value + "</span>"))
		} else {
			tr.ws.Write([]byte("<span style=\"color:green\">" + ev.Message + "</span>"))
		}
	}, ev)
}

func (tr *WebsocketTransport) Flush(t time.Duration) bool {

	return true
}
*/
/*
//depricated
func capture_(level int, e error, x *Context, msg string) {

	if level < MinLogLevel {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentryLevels[level-1])
		if x != nil {
			scope.SetContexts(x.contexts)
		}

		if e == nil {
			sentry.CaptureMessage(msg)
		} else {
			if msg != "" { //i is msg
				e = fmt.Errorf("%s: %w", msg, e)
			}
			sentry.CaptureException(e)
		}
	})
}

type MultiTransport struct {
	transports []sentry.Transport
}

func NewMultiTransport(t ...sentry.Transport) *MultiTransport {

	t := new(MultiTransport)
	t.transports = t
	return t
}

func (t *MultiTransport) Configure(options sentry.ClientOptions) {
}

func (t *MultiTransport) SendEvent(ev *sentry.Event) {
	for _, transport := range t.transports {
		transport.SendEvent(ev)
	}
}

func (t *MultiTransport) Flush(_ time.Duration) bool {
	return true
}
*/
