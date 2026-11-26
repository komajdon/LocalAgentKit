# Upgrading, backup and recovery

## Behaviour changes in this version

- Shell commands are sandboxed where possible: no network and no view of files
  outside the work directory. Change this in Settings → Shell sandbox.
- *Always Allow* stops covering sensitive tools once web, document or MCP
  content is in the conversation.
- File tools refuse symlinks with absolute targets and the app's data folder.
- Conversations with no work directory (imports) must be given one before file
  tools work.
- Export asks for a passphrase. Automatic backups no longer contain the key.
- Restore takes effect at the next start.
- Building needs Go 1.26 or newer.

## Data migration

No manual step. The database format is unchanged; stored messages and settings
gain optional fields.

## Going back to an older version

1. **If you moved the key to the keychain, move it back to a file first.** An
   older version does not know about the keychain and will create a new,
   wrong `agent.key`. If that happens, delete that file and use this version.
2. Copy the `data` folder before downgrading.
3. Older versions do not understand tool-role messages or stop/error rows.
   Conversations that used tools after upgrading may display or continue
   incorrectly there.
4. New backup files (`.lakbackup`) cannot be read by older versions.

## Backup

- **Export** (Settings): database and key, encrypted with your passphrase. Works
  on any machine. The passphrase cannot be recovered.
- **Automatic** (`data/backups/`, newest 7): database only. Restores only while
  the installation still has the same key.

## Restore

Settings → Restore backup, enter the passphrase if it is encrypted, then quit
and reopen. The replaced files are kept in `data/pre-restore-<time>/`. To undo,
quit and move them back over the current files.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| "agent.key is damaged" or "is missing but the database exists" | Restore the key file from a backup. The app will not create a replacement. |
| "could not provide it (is it unlocked?)" | Unlock the system keychain and restart. |
| "open db: timeout" | Another copy of the app is running. |
| "isolation: none" on Linux | Install `bubblewrap`. Some distributions also block unprivileged user namespaces. |
| "strict sandbox requested but no isolation backend" | Choose Automatic or Off, or install a backend. |
| Command cannot reach the network | Enable network access in the sandbox settings. |
| MCP server "failed 3 times" | Check the error under Settings → MCP Servers, then Restart. |
| Model misuses or ignores tools | Set Tool calling to "Text protocol". |
