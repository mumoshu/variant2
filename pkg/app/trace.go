package app

import (
	"encoding/json"
)

func (app *App) newTracingLogCollector() LogCollector {
	logCollector := LogCollector{
		CollectFn: func(evt Event) (*string, bool, error) {
			bs, err := json.Marshal(evt)
			if err != nil {
				return nil, false, err
			}

			app.Stderr.Write(append([]byte("TRACE\t"), bs...))

			return nil, false, nil
		},
		ForwardFn: func(log Log) error {
			return nil
		},
	}

	return logCollector
}
