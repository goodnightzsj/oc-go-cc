# Security and Streaming P0 Verification

## Goals

- Prevent every configuration export response from exposing resolved credentials.
- Preserve existing raw `${VAR}` placeholders or keys when a redacted export is imported.
- Confirm the previously reported normal-save and stream-escaping bugs are already fixed.

## Non-Goals

- Add authentication or remote-user management to the local GUI.
- Redesign the settings page.

## Constraints

- Redaction is enforced server-side; frontend state is not a security boundary.
- Imported redaction masks must never overwrite real credentials.
- Add no dependencies.

## Done-When

- [ ] Export cannot return a real key for any query value.
- [ ] Redacted export/import preserves raw secrets and placeholders on disk.
- [ ] Focused GUI, config, and transformer tests pass.
