package simplesse_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sse "github.com/kamludwinski2/simplesse"
	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	server := sse.NewServer(func() []int { return []int{1, 2, 3} })

	assert.NotNil(t, server, "expected server to be initialised")
}

func TestAddConnection(t *testing.T) {
	server := sse.NewServer(func() []int { return []int{1, 2, 3} })
	defer server.Shutdown()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	go server.AddConnection(w, req)
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, 1, server.GetConnectionCount(), "expected connection count to be 1")
}

func TestRemoveConnectionOnDisconnect(t *testing.T) {
	server := sse.NewServer(func() []int { return []int{1, 2, 3} })
	defer server.Shutdown()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	go server.AddConnection(w, req)

	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, 1, server.GetConnectionCount(), "expected connection count to be 1")

	cancel() // simulate client disconnect
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, 0, server.GetConnectionCount(), "expected connection count to be 0 after disconnect")
}

func TestSendEvent(t *testing.T) {
	server := sse.NewServer(func() []int { return []int{1, 2, 3} })
	defer server.Shutdown()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	go server.AddConnection(w, req)
	time.Sleep(500 * time.Millisecond)

	event := sse.Event[int]{Type: "test", Payload: []int{42}}
	err := server.SendEvent(event)

	assert.NoError(t, err, "unexpected error sending event")
}

func TestMiddlewareInvocation(t *testing.T) {
	server := sse.NewServer(func() []int { return []int{1, 2, 3} })

	var connectCalled, disconnectCalled, warnCalled, errorCalled, snapshotCalled, sendCalled, flushCalled, shutdownCalled int32

	server.OnConnect(func(clientID string) { atomic.AddInt32(&connectCalled, 1) })
	server.OnDisconnect(func(clientID string, duration time.Duration) { atomic.AddInt32(&disconnectCalled, 1) })
	server.OnWarn(func(warning string) { atomic.AddInt32(&warnCalled, 1) })
	server.OnError(func(err error) { atomic.AddInt32(&errorCalled, 1) })
	server.OnSnapshot(func(clientID string, event sse.Event[int]) { atomic.AddInt32(&snapshotCalled, 1) })
	server.OnSend(func(clientIDs []string) { atomic.AddInt32(&sendCalled, 1) })
	server.OnFlush(func(clientID string, event sse.Event[int]) { atomic.AddInt32(&flushCalled, 1) })
	server.OnShutdown(func(clientIDs []string) { atomic.AddInt32(&shutdownCalled, 1) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	go server.AddConnection(w, req)
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&connectCalled), "expected connect middleware to be called once")

	event := sse.Event[int]{Type: "test", Payload: []int{42}}
	err := server.SendEvent(event)
	assert.NoError(t, err, "unexpected error sending event")

	time.Sleep(500 * time.Millisecond) // small delay to allow flush

	// call 1 is snapshot, 2 is event
	assert.Equal(t, int32(2), atomic.LoadInt32(&flushCalled), "expected flush middleware to be called twice")

	assert.Equal(t, int32(1), atomic.LoadInt32(&snapshotCalled), "expected snapshot middleware to be called once")

	server.Shutdown()
	assert.Equal(t, int32(1), atomic.LoadInt32(&shutdownCalled), "expected shutdown middleware to be called once")
}
