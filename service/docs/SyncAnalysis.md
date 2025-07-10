# Analysis: Full Sync vs. Sliding Sync

This document analyzes the trade-offs between the current Full Sync implementation and a potential migration to Sliding Sync for the Matrix adapter.

## Current Implementation: Full Sync (`/sync`)

The application currently uses the standard `matrix-js-sdk` `startClient()` method. This method relies on the traditional `/sync` endpoint on the Matrix homeserver.

### How It Works

1.  **Initial Sync**: On the first connection, the client requests a complete snapshot of the user's account. This includes a list of all joined rooms, their full state (topics, members, permissions), and a recent portion of the message history for each room.
2.  **Incremental Syncs**: After the initial sync, the client performs subsequent "incremental" syncs in a long-polling loop. These syncs provide a diff of everything that has changed since the last sync *across all rooms*.

### Advantages

*   **Complete Local State**: The client has a full, local copy of the state for every room the user is in. This makes looking up room details or members very fast, as the data is already on-hand.
*   **Simpler Client Logic**: The client logic is straightforward. Once the sync is complete, the application can assume all data is present and accounted for.

### Disadvantages

*   **Performance Bottlenecks**: For users in hundreds or thousands of rooms, the initial sync can be extremely slow (taking minutes) and consume a large amount of memory and bandwidth.
*   **Resource Intensive**: The client receives updates for every single room, even if the application only cares about a small subset at any given time. This wastes CPU and network resources processing irrelevant events.
*   **Scalability Issues**: Performance degrades as the user joins more rooms.

## Proposed Alternative: Sliding Sync (MSC3575)

Sliding Sync is a newer, more efficient specification designed to solve the performance problems of Full Sync. It treats the room list like a "sliding window" or a viewport.

### How It Works

1.  **Window Subscription**: Instead of fetching everything, the client tells the server it is interested in a specific "window" of rooms (e.g., the first 20 rooms, sorted by recent activity).
2.  **Targeted Updates**: The server only sends updates for the rooms that fall within this window.
3.  **Sliding the Window**: As the user (or application) needs to see different rooms, the client "slides" the window, telling the server the new range of rooms it is interested in. The server then provides the data for that new set of rooms.

### Advantages

*   **Extremely Fast Startup**: The initial connection is nearly instantaneous because the client only requests a small, manageable number of rooms.
*   **Low Resource Usage**: Bandwidth and CPU are conserved because the client only receives and processes updates for the rooms it currently cares about.
*   **Excellent Scalability**: Performance is not tied to the total number of rooms a user has joined. A user in 10,000 rooms has the same initial load time as a user in 20.
*   **Powerful Features**: Allows for server-side sorting and filtering of the room list, which is not possible with Full Sync.

### Disadvantages & Migration Considerations

*   **Incomplete Local State**: The biggest architectural change is that the application can no longer assume it has a complete local cache of all rooms. Code that calls `matrixClient.getRoom(roomId)` may fail if that room is not within the current sliding sync window.
*   **New Logic Required**: The application must be updated to manage the sliding window. It needs to decide which rooms are "active" and request them from the server.
*   **SDK Integration**: The `matrix-js-sdk` supports Sliding Sync, but it requires a different initialization path than the standard `startClient()` method. The `MatrixAgent` and `MatrixAgentService` would need significant refactoring to use the new APIs.
*   **Handling "Unseen" Notifications**: The application needs a strategy for handling notifications or mentions in rooms that are *outside* the current window. The Sliding Sync protocol provides summaries (like total unread counts) to help with this, but it's a state that must be managed.

## Conclusion

The current **Full Sync** implementation is simpler but does not scale well and can lead to significant performance issues for power users.

**Sliding Sync** offers a vastly superior user experience and is much more efficient and scalable. However, migrating to it is a non-trivial task that requires a fundamental shift in how the application thinks about and accesses room data. The logic must move from assuming a complete local state to actively requesting the data it needs on-demand.
