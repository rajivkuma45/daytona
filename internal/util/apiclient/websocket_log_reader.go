// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package apiclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/daytonaio/daytona/cmd/daytona/config"
	"github.com/daytonaio/daytona/pkg/logs"
	logs_view "github.com/daytonaio/daytona/pkg/views/logs"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var workspaceLogsStarted bool

func ReadWorkspaceLogs(ctx context.Context, activeProfile config.Profile, workspaceId string, projectNames []string) {
	var wg sync.WaitGroup
	query := "follow=true"

	logs_view.CalculateLongestPrefixLength(projectNames)

	for index, projectName := range projectNames {
		wg.Add(1)
		go func(projectName string) {
			defer wg.Done()

			for {
				// Make sure workspace logs started before showing any project logs
				if !workspaceLogsStarted {
					time.Sleep(250 * time.Millisecond)
					continue
				}

				ws, res, err := GetWebsocketConn(ctx, fmt.Sprintf("/log/workspace/%s/%s", workspaceId, projectName), &activeProfile, &query)
				// We want to retry getting the logs if it fails
				if err != nil {
					log.Trace(HandleErrorResponse(res, err))
					time.Sleep(500 * time.Millisecond)
					continue
				}

				readJSONLog(ctx, ws, index)
				ws.Close()
				break
			}
		}(projectName)
	}

	for {
		ws, res, err := GetWebsocketConn(ctx, fmt.Sprintf("/log/workspace/%s", workspaceId), &activeProfile, &query)
		// We want to retry getting the logs if it fails
		if err != nil {
			log.Trace(HandleErrorResponse(res, err))
			time.Sleep(250 * time.Millisecond)
			continue
		}

		readJSONLog(ctx, ws, logs_view.STATIC_INDEX)
		ws.Close()
		break
	}

	wg.Wait()
}

func ReadBuildLogs(ctx context.Context, activeProfile config.Profile, buildId string, query string) {
	logs_view.CalculateLongestPrefixLength([]string{buildId})

	for {
		ws, res, err := GetWebsocketConn(ctx, fmt.Sprintf("/log/build/%s", buildId), &activeProfile, &query)
		// We want to retry getting the logs if it fails
		if err != nil {
			log.Trace(HandleErrorResponse(res, err))
			time.Sleep(250 * time.Millisecond)
			continue
		}

		readJSONLog(ctx, ws, logs_view.FIRST_PROJECT_INDEX)
		ws.Close()
		break
	}
}

func readJSONLog(ctx context.Context, ws *websocket.Conn, index int) {
	logEntriesChan := make(chan logs.LogEntry)

	go func() {
		for {
			var logEntry logs.LogEntry

			err := ws.ReadJSON(&logEntry)
			if err != nil {
				log.Trace(err)
				return
			}

			logEntriesChan <- logEntry
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case logEntry := <-logEntriesChan:
			logs_view.DisplayLogEntry(logEntry, index)
		}

		if !workspaceLogsStarted && index == logs_view.STATIC_INDEX {
			workspaceLogsStarted = true
		}
	}
}
