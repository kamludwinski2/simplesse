package simplesse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Event[T any] struct {
	Type    string `json:"type"`
	Payload []T    `json:"payload"`
}

type connection struct {
	writer        http.ResponseWriter
	flusher       http.Flusher
	connectedTime time.Time
	cancel        context.CancelFunc
}

type WarnMiddleware func(string)
type ErrorMiddleware func(error)

type ConnectMiddleware func(string)
type DisconnectMiddleware func(string, time.Duration)

type SnapshotMiddleware[T any] func(string, Event[T])
type SendMiddleware func([]string)
type FlushMiddleware[T any] func(string, Event[T])

type ShutdownMiddleware func([]string)

type Server[T any] struct {
	mutex        sync.Mutex
	connections  map[string]connection
	snapshotFunc func() []T // used to retrieve snapshot

	warnMiddlewares  []WarnMiddleware
	errorMiddlewares []ErrorMiddleware

	connectMiddlewares    []ConnectMiddleware
	disconnectMiddlewares []DisconnectMiddleware

	snapshotMiddlewares []SnapshotMiddleware[T]
	sendMiddlewares     []SendMiddleware
	flushMiddlewares    []FlushMiddleware[T]

	shutdownMiddlewares []ShutdownMiddleware
}

func NewServer[T any](snapshotFunc func() []T) *Server[T] {
	return &Server[T]{
		connections:  make(map[string]connection),
		snapshotFunc: snapshotFunc,
	}
}

func (s *Server[T]) GetConnectionCount() int {
	return len(s.connections)
}

func (s *Server[T]) AddConnection(w http.ResponseWriter, r *http.Request) {
	clientId := r.RemoteAddr

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		s.callOnErrorMiddleware(fmt.Errorf("streaming unsupported for client %s", clientId))

		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	s.mutex.Lock()
	s.connections[clientId] = connection{
		writer:        w,
		flusher:       flusher,
		connectedTime: time.Now(),
		cancel:        cancel,
	}
	s.mutex.Unlock()

	s.callOnConnectMiddleware(clientId)
	defer func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		cancel()
		delete(s.connections, clientId)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	event := Event[T]{
		Type:    "SNAPSHOT",
		Payload: s.snapshotFunc(),
	}

	s.callOnSnapshotMiddleware(clientId, event)
	err := s.SendEvent(event, clientId)
	if err != nil {
		http.Error(w, "Failed to send event", http.StatusInternalServerError)
		s.callOnErrorMiddleware(fmt.Errorf("failed to send event: %w", err))

		return
	}

	<-ctx.Done()

	s.callOnDisconnectMiddleware(clientId, time.Now().Sub(s.connections[clientId].connectedTime))
}

func (s *Server[T]) SendEvent(event Event[T], clientIds ...string) error {
	if len(clientIds) == 0 {
		clientIds = s.getClientIds()
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	for _, clientId := range clientIds {
		conn, exists := s.connections[clientId]
		if !exists {
			s.callOnWarnMiddleware(fmt.Sprintf("client %s not found", clientId))

			continue
		}

		_, err := fmt.Fprintf(conn.writer, "data: %s\n\n", eventData)
		if err != nil {
			http.Error(conn.writer, "Failed to send event", http.StatusInternalServerError)

			return err
		}

		conn.flusher.Flush()
		s.callOnFlushMiddleware(clientId, event)
	}

	return nil
}

func (s *Server[T]) getClientIds() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	clientIds := make([]string, 0, len(s.connections))
	for clientId := range s.connections {
		clientIds = append(clientIds, clientId)
	}

	return clientIds
}

func (s *Server[T]) Shutdown() {
	clientIds := s.getClientIds()

	s.callOnShutdownMiddleware(clientIds)

	event := Event[T]{
		Type:    "SHUTDOWN",
		Payload: nil,
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, clientId := range clientIds {
		s.closeConnectionLocked(clientId, event)
	}
}

// closeConnectionLocked helper func to prevent race conditions.
// it can access s.connections as lock context is the parent
func (s *Server[T]) closeConnectionLocked(clientId string, event Event[T]) {
	conn, exists := s.connections[clientId]
	if !exists {
		return
	}

	defer conn.cancel()

	err := s.SendEvent(event, clientId)
	if err != nil {
		s.callOnErrorMiddleware(err)
	}

}

func (s *Server[T]) CloseConnection(clientId string, event ...Event[T]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, exists := s.connections[clientId]
	if !exists {
		return
	}

	if len(event) > 0 {
		s.closeConnectionLocked(clientId, event[0])
	} else {
		s.closeConnectionLocked(clientId, Event[T]{Type: "SHUTDOWN", Payload: nil})
	}
}

func (s *Server[T]) OnConnect(f ConnectMiddleware) *Server[T] {
	s.connectMiddlewares = append(s.connectMiddlewares, f)

	return s
}

func (s *Server[T]) OnDisconnect(f DisconnectMiddleware) *Server[T] {
	s.disconnectMiddlewares = append(s.disconnectMiddlewares, f)

	return s
}

func (s *Server[T]) OnWarn(f WarnMiddleware) *Server[T] {
	s.warnMiddlewares = append(s.warnMiddlewares, f)

	return s
}

func (s *Server[T]) OnError(f ErrorMiddleware) *Server[T] {
	s.errorMiddlewares = append(s.errorMiddlewares, f)

	return s
}

func (s *Server[T]) OnSnapshot(f SnapshotMiddleware[T]) *Server[T] {
	s.snapshotMiddlewares = append(s.snapshotMiddlewares, f)

	return s
}

func (s *Server[T]) OnSend(f SendMiddleware) *Server[T] {
	s.sendMiddlewares = append(s.sendMiddlewares, f)

	return s
}

func (s *Server[T]) OnFlush(f FlushMiddleware[T]) *Server[T] {
	s.flushMiddlewares = append(s.flushMiddlewares, f)

	return s
}

func (s *Server[T]) OnShutdown(f ShutdownMiddleware) *Server[T] {
	s.shutdownMiddlewares = append(s.shutdownMiddlewares, f)

	return s
}

func (s *Server[T]) callOnConnectMiddleware(clientId string) {
	for _, middleware := range s.connectMiddlewares {
		middleware(clientId)
	}
}

func (s *Server[T]) callOnDisconnectMiddleware(clientId string, duration time.Duration) {
	for _, middleware := range s.disconnectMiddlewares {
		middleware(clientId, duration)
	}
}

func (s *Server[T]) callOnWarnMiddleware(warning string) {
	for _, middleware := range s.warnMiddlewares {
		middleware(warning)
	}
}

func (s *Server[T]) callOnErrorMiddleware(err error) {
	for _, middleware := range s.errorMiddlewares {
		middleware(err)
	}
}

func (s *Server[T]) callOnSnapshotMiddleware(clientId string, event Event[T]) {
	for _, middleware := range s.snapshotMiddlewares {
		middleware(clientId, event)
	}
}

func (s *Server[T]) callOnSendMiddleware(clientIds []string) {
	for _, middleware := range s.sendMiddlewares {
		middleware(clientIds)
	}
}

func (s *Server[T]) callOnFlushMiddleware(clientId string, event Event[T]) {
	for _, middleware := range s.flushMiddlewares {
		middleware(clientId, event)
	}
}

func (s *Server[T]) callOnShutdownMiddleware(clientIds []string) {
	for _, middleware := range s.shutdownMiddlewares {
		middleware(clientIds)
	}
}
