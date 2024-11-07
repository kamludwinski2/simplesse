# SSE
Simplistic server that provides Server-Sent Events(**SSE**) functionality for real-time updates to connected clients.

## Functionality
- add and manage multiple client connections.
- send events to specific clients or broadcast to all clients.
- middleware:
    - **OnConnect** triggered when a client connects
    - **OnDisconnect** triggered when a client disconnects
    - **OnWarn** triggered when server throws a warning (client not found etc...)
    - **OnError** triggered when server throws an error
    - **OnSnapshot** triggered when sending snapshot(latest up-to-date state)
    - **OnSend** triggered when an event is being sent
    - **OnFlush** triggered when flushing the events to clients
    - **OnShutdown** triggered when server is shutting down

## Example
Can be found in **example/main.go**