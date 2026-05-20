# Discord Gateway Stability Design

## Goal

Stop frequent Discord gateway resets by making the Notification Hub gateway client follow the same core protocol patterns used by mature SDKs such as discordgo.

## Root Cause

The current gateway client is only a minimal websocket reader. It identifies and sends periodic heartbeats, but it does not fully maintain Discord gateway connection state:

- Heartbeats do not include the last dispatch sequence number or `null` when no sequence exists.
- Heartbeat ACK (`op: 11`) is ignored, so zombie connections are not detected deliberately.
- Reconnect (`op: 7`) and Invalid Session (`op: 9`) are ignored.
- READY session state is not stored, so later reconnects cannot resume.

These protocol gaps make Discord-initiated resets more likely and make reconnect behavior less stable.

## Reference Pattern

Use discordgo as the implementation reference:

- Track sequence from dispatch events only.
- Send heartbeat payloads as `op: 1` with `d` set to the latest sequence or `null`.
- Track heartbeat ACKs and close/reconnect when ACKs stop arriving.
- Handle `op: 7` by reconnecting.
- Handle `op: 9`; if resumable, attempt resume, otherwise clear session state and identify again.
- Store `session_id` and `resume_gateway_url` from READY.
- Resume with `op: 6` using token, session id, and sequence.

## Implementation Phases

### Phase 1: Protocol-compliant live connection

Implement the minimum stable gateway behavior:

- Add gateway op constants for ACK, Reconnect, Invalid Session, and Resume.
- Add sequence tracking on `Gateway`.
- Update sequence when dispatch payload `s` is non-null.
- Send heartbeat payloads with the current sequence or JSON null.
- Track whether the previous heartbeat was acknowledged.
- Mark ACK received when `op: 11` arrives.
- Treat missing ACK before the next heartbeat as a connection error so `Start` reconnects through the existing loop.
- Treat `op: 7` as a reconnect request so `Start` reconnects through the existing loop.
- Treat `op: 9` as reconnect; when payload is `false`, clear resume state and reset sequence.

### Phase 2: Resume support

Add stateful resume after the live connection is stable:

- Store `session_id` and `resume_gateway_url` from READY dispatch data.
- On reconnect, use `resume_gateway_url` when session id and sequence exist.
- Send `op: 6 Resume` instead of Identify when resuming.
- If resume fails with non-resumable Invalid Session, clear session state and identify on the next connection.

## Testing

Add focused gateway tests with local websocket servers:

- Heartbeat includes `d: null` before dispatch sequence exists.
- Heartbeat includes the latest dispatch sequence after a dispatch event.
- Missing heartbeat ACK returns an error and causes reconnect behavior.
- `op: 7` triggers reconnect.
- `op: 9` with `false` clears resume state.
- READY stores session id and resume gateway URL.
- Reconnect with stored state sends Resume instead of Identify.

Run `go test ./...` before completion.

## Scope Boundaries

This change keeps the local gateway client and does not replace it with a full SDK. It borrows the proven SDK state-machine pattern while avoiding a large dependency migration.
