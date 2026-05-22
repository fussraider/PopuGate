package handler

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/gin-gonic/gin"
)

var sseLog = logger.WithScope("sse")

// streamLogs handles SSE and plain-text log streaming from a Docker container.
// It unifies the log streaming logic used by both instance and proxy log endpoints.
func streamLogs(c *gin.Context, docker *dockerutil.DockerClient, containerName, tail string, follow bool) {
	if follow {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
	} else {
		c.Header("Content-Type", "text/plain")
	}

	logs, err := docker.InstanceLogs(c.Request.Context(), containerName, tail, follow)
	if err != nil {
		if follow {
			c.SSEvent("error", fmt.Sprintf("failed to get logs: %v", err))
		} else {
			HandleError(c, 500, "failed to get logs", err)
		}
		return
	}
	defer func() { _ = logs.Close() }()

	scanner := bufio.NewScanner(logs)

	if !follow {
		streamPlainText(c, scanner)
		return
	}

	streamSSE(c, scanner, logs)
}

func streamPlainText(c *gin.Context, scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 8 {
			line = line[8:]
		}
		_, _ = fmt.Fprintln(c.Writer, line)
	}
}

func streamSSE(c *gin.Context, scanner *bufio.Scanner, logs io.Reader) {
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }
	defer closeDone()

	var writeMu sync.Mutex
	writeSSE := func(event string, data interface{}) {
		writeMu.Lock()
		c.SSEvent(event, data)
		c.Writer.Flush()
		writeMu.Unlock()
	}

	// Cancel context closes done when client disconnects
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sseLog.Debugf("goroutine panic (context monitor): %v", r)
			}
		}()
		select {
		case <-c.Request.Context().Done():
			closeDone()
		case <-done:
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				sseLog.Debugf("goroutine panic (heartbeat): %v", r)
			}
		}()
		for {
			select {
			case <-heartbeat.C:
				writeSSE("heartbeat", time.Now().Unix())
			case <-done:
				return
			}
		}
	}()

	ctx := c.Request.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		if len(line) > 8 {
			line = line[8:]
		}
		writeSSE("message", line)
	}
}
