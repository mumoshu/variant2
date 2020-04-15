package app

import (
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/zclconf/go-cty/cty"
)

type Event struct {
	Type string
	Time time.Time
	Run  *RunEvent
	Exec *ExecEvent
}

type RunEvent struct {
	Job  string
	Args map[string]interface{}
}

type ExecEvent struct {
	Command string
	Args    []string
}

func (evt Event) toCty() cty.Value {
	m := map[string]cty.Value{
		"type": cty.StringVal(evt.Type),
	}
	m["time"] = cty.StringVal(evt.Time.Format(time.RFC3339))

	if evt.Run != nil {
		m["run"] = evt.Run.toCty()
	}

	if evt.Exec != nil {
		m["exec"] = evt.Exec.toCty()
	}

	return cty.ObjectVal(m)
}

func (e *RunEvent) toCty() cty.Value {
	var args cty.Value

	if len(e.Args) > 0 {
		var err error

		args, err = goToCty(e.Args)
		if err != nil {
			panic(err)
		}
	} else {
		// We can't call goToCty for empty map because it results in go-cty panic with "must not call MapVal with empty map"
		args = cty.MapValEmpty(cty.DynamicPseudoType)
	}

	return cty.ObjectVal(map[string]cty.Value{
		"job":  cty.StringVal(e.Job),
		"args": args,
	})
}

func (e *ExecEvent) toCty() cty.Value {
	vals := []cty.Value{}
	for _, a := range e.Args {
		vals = append(vals, cty.StringVal(a))
	}

	return cty.ObjectVal(map[string]cty.Value{
		"command": cty.StringVal(e.Command),
		"args":    cty.ListVal(vals),
	})
}

type EventLogger struct {
	lastIndex int

	Command    string
	Args, Opts map[string]interface{}

	Stream string

	Stderr io.Writer

	Events []Event

	collectors map[int]*LogCollector

	collectorsMutex sync.Mutex
	eventsMutex     sync.Mutex
}

func NewEventLogger(cmd string, args map[string]interface{}, opts map[string]interface{}) *EventLogger {
	return &EventLogger{
		collectors: map[int]*LogCollector{},
		Events:     []Event{},
		Command:    cmd,
		Args:       args,
		Opts:       opts,
	}
}

func (l *EventLogger) LogRun(job string, args map[string]interface{}) error {
	return l.append(Event{Type: "run", Time: time.Now(), Run: &RunEvent{
		Job:  job,
		Args: args,
	}})
}

func (l *EventLogger) LogExec(cmd string, args []string) error {
	return l.append(Event{Type: "exec", Time: time.Now(), Exec: &ExecEvent{
		Command: cmd,
		Args:    args,
	}})
}

func (l *EventLogger) append(evt Event) error {
	l.eventsMutex.Lock()
	l.Events = append(l.Events, evt)
	l.eventsMutex.Unlock()

	l.collectorsMutex.Lock()
	defer l.collectorsMutex.Unlock()

	for _, c := range l.collectors {
		line, err := c.Collect(evt)
		if err != nil {
			return err
		}

		// Non-nil line means that any collect block's condition matched the logged event
		if line != nil && l.Stream == "stderr" {
			if _, err := l.Stderr.Write([]byte(*line + "\n")); err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *EventLogger) Register(logCollector LogCollector) func() error {
	id := l.lastIndex + 1
	l.lastIndex = id

	l.collectorsMutex.Lock()
	l.collectors[id] = &logCollector
	l.collectorsMutex.Unlock()

	return func() error {
		defer func() {
			l.collectorsMutex.Lock()
			delete(l.collectors, id)
			l.collectorsMutex.Unlock()
		}()

		var file string

		if logCollector.FilePath == "" {
			tmpFile, _ := ioutil.TempFile("", "tmp")
			file = tmpFile.Name()
		} else {
			file = logCollector.FilePath
		}

		if err := ioutil.WriteFile(file, []byte(strings.Join(logCollector.lines, "\n")), 0644); err != nil {
			return err
		}

		log := Log{
			File: file,
		}

		return logCollector.ForwardFn(log)
	}
}

type Log struct {
	File string
}

type LogCollector struct {
	FilePath  string
	CollectFn func(Event) (*string, bool, error)
	ForwardFn func(log Log) error
	lines     []string
}

func (c *LogCollector) Collect(evt Event) (*string, error) {
	text, shouldCollect, err := c.CollectFn(evt)
	if err != nil {
		return nil, err
	}

	if shouldCollect {
		if c.lines == nil {
			c.lines = []string{}
		}

		c.lines = append(c.lines, *text)
	}

	return text, nil
}
