// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

func newDebugLogDBHandler(ctxt httpContext) http.Handler {
	return newDebugLogHandler(ctxt, handleDebugLogDBRequest)
}

func handleDebugLogDBRequest(
	st state.LogTailerState,
	reqParams *debugLogParams,
	socket debugLogSocket,
	stop <-chan struct{},
) error {
	params := makeLogTailerParams(reqParams)
	tailer, err := newLogTailer(st, params)
	if err != nil {
		return errors.Trace(err)
	}
	defer tailer.Stop()

	// Indicate that all is well.
	socket.sendOk()

	var lineCount uint
	for {
		select {
		case <-stop:
			return nil
		case rec, ok := <-tailer.Logs():
			if !ok {
				return errors.Annotate(tailer.Err(), "tailer stopped")
			}

			line := formatLogRecord(rec)
			_, err := socket.Write([]byte(line))
			if err != nil {
				return errors.Annotate(err, "sending failed")
			}

			lineCount++
			if reqParams.maxLines > 0 && lineCount == reqParams.maxLines {
				return nil
			}
		}
	}
}

func makeLogTailerParams(reqParams *debugLogParams) *state.LogTailerParams {
	params := &state.LogTailerParams{
		MinLevel:      reqParams.filterLevel,
		NoTail:        reqParams.noTail,
		InitialLines:  int(reqParams.backlog),
		IncludeEntity: reqParams.includeEntity,
		ExcludeEntity: reqParams.excludeEntity,
		IncludeModule: reqParams.includeModule,
		ExcludeModule: reqParams.excludeModule,
	}
	if reqParams.fromTheStart {
		params.InitialLines = 0
	}
	return params
}

func formatLogRecord(r *state.LogRecord) string {
	return fmt.Sprintf("%s: %s %s %s %s %s\n",
		r.Entity,
		formatTime(r.Time),
		r.Level.String(),
		r.Module,
		r.Location,
		r.Message,
	)
}

func formatTime(t time.Time) string {
	return t.In(time.UTC).Format("2006-01-02 15:04:05")
}

var newLogTailer = _newLogTailer // For replacing in tests

func _newLogTailer(st state.LogTailerState, params *state.LogTailerParams) (state.LogTailer, error) {
	return state.NewLogTailer(st, params)
}
