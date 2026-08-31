export const en = {
  "shell.title": "sshc",
  "shell.starting": "Starting secure local session…",
  "shell.vaultChecking":
    "Checking that the vault is still unlocked before restoring protected content…",
  "shell.vaultCheckRetrying":
    "The vault state could not be confirmed. Protected content remains hidden while sshc retries.",
  "shell.active": "Local session active · {version}",
  "shell.bootstrapFailed":
    "Secure local session could not be started. Run sshc in a terminal to enrol this browser again.",
  "shell.bootstrapRetry": "Try again",
  "shell.sessionEndedHeading": "Session ended",
  "shell.sessionEnded":
    "Reload to recover the local session automatically. If this browser is no longer enrolled, run sshc in a terminal once.",
  "shell.sessionReload": "Reload session",
  "shell.pageNotFound": "Page not found",
  "shell.pageNotFoundDescription": "No sshc section exists at this URL.",
  "shell.goHome": "Go to Home",
  "shell.primaryNavigation": "Primary",
  "shell.navigationToggle": "Navigation",
  "shell.navigationShow": "Show navigation",
  "shell.navigationHide": "Hide navigation",
  "shell.navigationResize": "Resize navigation",
  "shell.sessions": "Sessions",
  "shell.navStart": "Start",
  "shell.navConnections": "Connection files",
  "shell.navKeysHosts": "Keys and hosts",
  "shell.navMaintenance": "Maintenance",
  "menu.open": "Open {section}",
  "shell.inspectorShowNamed": "Show {label}",
  "shell.inspectorHideNamed": "Hide {label}",
  "shell.inspectorAttention": "Needs attention",
  "palette.open": "Search everything",
  "palette.heading": "Search sessions, hosts, files, snippets and settings",
  "palette.placeholder": "Search sessions, hosts, files, snippets and settings",
  "palette.results": "Search results",
  "palette.loading": "Loading searchable items…",
  "palette.empty": "No matching items",
  "palette.hint": "↑↓ Select · Enter Open · Esc Close",
  "palette.connectHost": "Connect to {alias}",
  "palette.openHostSettings": "Open connection settings for {alias}",
  "palette.openSection": "Open section",
  "palette.kind.session": "Session",
  "palette.kind.host": "Host",
  "palette.kind.file": "File",
  "palette.kind.snippet": "Snippet",
  "palette.kind.setting": "Setting",
  "table.sortAscending": ", sort ascending",
  "table.sortDescending": ", sort descending",
  "section.files": "SFTP",
  "section.snippets": "Snippets",
  "sftp.heading": "Remote files",
  "sftp.host": "Host",
  "sftp.noHosts": "No saved hosts",
  "sftp.chooseHost": "Select a host",
  "sftp.connectionFailed": "Could not connect.",
  "sftp.path": "Remote path",
  "sftp.go": "Go",
  "sftp.up": "Up",
  "sftp.newFolder": "New folder",
  "sftp.upload": "Upload",
  "sftp.uploadFolder": "Upload folder",
  "sftp.dropHint": "or drop files/folders here",
  "sftp.dropNow": "Drop to upload recursively",
  "sftp.dropZone": "Upload files or folders to the current remote directory",
  "sftp.uploads": "File uploads",
  "sftp.upload.pending": "Waiting",
  "sftp.upload.queued": "Queued",
  "sftp.upload.uploading": "Uploading…",
  "sftp.upload.paused": "Paused",
  "sftp.upload.reattach": "Select the same file to resume",
  "sftp.upload.needs_overwrite": "Overwrite confirmation required",
  "sftp.upload.done": "Uploaded",
  "sftp.upload.failed": "Failed",
  "sftp.upload.skipped": "Skipped",
  "sftp.upload.cancelled": "Cancelled",
  "sftp.cancelTransfer": "Cancel transfer",
  "sftp.activeTransfers": "{count} active",
  "sftp.transfer.pause": "Pause",
  "sftp.transfer.resume": "Resume",
  "sftp.transfer.clear": "Clear finished",
  "sftp.manager.heading": "Transfer Manager",
  "sftp.manager.limit": "Up to {count} active transfers",
  "sftp.manager.items": "Transfer files",
  "sftp.manager.file": "File",
  "sftp.manager.folder": "Folder",
  "sftp.manager.upload": "Upload",
  "sftp.manager.download": "Download",
  "sftp.manager.remaining": "{duration} left",
  "sftp.manager.retry": "Retry",
  "sftp.manager.retryFailed": "Retry {count} failed",
  "sftp.manager.status.queued": "Queued",
  "sftp.manager.status.running": "Transferring…",
  "sftp.manager.status.paused": "Paused",
  "sftp.manager.status.reattach": "Select the same file",
  "sftp.manager.status.needs_overwrite": "Confirm overwrite",
  "sftp.manager.status.completed": "Completed",
  "sftp.manager.status.failed": "Failed",
  "sftp.manager.status.cancelled": "Cancelled",
  "sftp.notice.heading": "Transfer notifications",
  "sftp.notice.completed": "{direction} completed: {name}",
  "sftp.notice.failed": "{direction} failed: {name} ({problem})",
  "sftp.notice.dismiss": "Dismiss notification",
  "sftp.download": "Download",
  "sftp.downloads": "Downloads",
  "sftp.download.downloading": "Downloading…",
  "sftp.download.done": "Downloaded",
  "sftp.download.failed": "Failed",
  "sftp.download.cancelled": "Cancelled",
  "sftp.name": "Name",
  "sftp.size": "Bytes",
  "sftp.type": "Type",
  "sftp.type.file": "File",
  "sftp.type.directory": "Folder",
  "sftp.type.symlink": "Symlink",
  "sftp.type.other": "Other",
  "sftp.modified": "Modified",
  "sftp.permissions": "Permissions",
  "sftp.actions": "Actions",
  "sftp.entries": "Remote entries",
  "sftp.rename": "Rename",
  "sftp.delete": "Delete",
  "sftp.chmod": "Change permissions",
  "sftp.chmodPrompt": "Permissions (octal, for example 640)",
  "sftp.overwriteHeading": "Overwrite this remote file?",
  "sftp.overwrite": "Overwrite",
  "sftp.skip": "Skip",
  "sftp.cancel": "Cancel",
  "sftp.save": "Save",
  "sftp.close": "Close",
  "sftp.editorEmpty": "Select a UTF-8 text file to edit it here.",
  "sftp.editorLoading": "Loading editor…",
  "sftp.unsaved": "Unsaved",
  "sftp.unsavedBlocked": "Save or close the edited file before navigating.",
  "sftp.conflict": "The remote file changed. Reload it before saving again.",
  "sftp.binaryHint": "This is not a UTF-8 text file. Download it instead.",
  "sftp.tooLargeHint":
    "Files larger than 2 MiB can be downloaded but not edited.",
  "sftp.mkdirPrompt": "Folder name",
  "sftp.renamePrompt": "New name",
  "sftp.deleteHeading": "Delete this remote entry?",
  "workspace.saved": "Choose a saved layout",
  "workspace.new": "New saved layout",
  "workspace.save": "Save with a name",
  "workspace.reopen": "Open this layout",
  "workspace.delete": "Delete",
  "workspace.detachPane": "Remove from workspace",
  "workspace.live": "Live workspace",
  "workspace.mobilePaneSwitcher": "Workspace terminals",
  "workspace.oneLiveOnly":
    "Finish or dissolve the current live workspace before creating another one.",
  "workspace.dock.left": "Place on the left",
  "workspace.dock.right": "Place on the right",
  "workspace.dock.top": "Place above",
  "workspace.dock.bottom": "Place below",
  "workspace.groupCount": "{count} terminals",
  "workspace.rename": "Rename workspace",
  "workspace.renameLabel": "New name for {name}",
  "workspace.rowMenu": "Actions for {name}",
  "workspace.expandGroup": "Show terminals in {name}",
  "workspace.collapseGroup": "Hide terminals in {name}",
  "workspace.resizeSplit": "Resize split",
  "workspace.focusMode": "Focus {alias}",
  "workspace.exitFocusMode": "Exit focus mode",
  "workspace.movePane":
    "Move {alias} pane. Select it, then choose another pane to exchange positions.",
  "workspace.movePanePicked":
    "{alias} selected. Choose another pane to exchange positions.",
  "workspace.reconnecting": "Reconnect this pane to continue.",
  "workspace.maxPanes": "A screen can contain up to {count} terminals.",
  "workspace.namePrompt": "Saved layout name",
  "workspace.savedLayouts": "Saved layouts",
  "workspace.savedDescription":
    "Save SSH targets, local shells, and split ratios with a name, then recreate them later as new sessions. Each layout supports up to {count} terminals.",
  "workspace.broadcastCommand": "Send command…",
  "workspace.broadcastHeading": "Send to connected terminals",
  "workspace.broadcastDescription":
    "Review a command or snippet, then send it to each connected terminal's current input.",
  "workspace.commandClose": "Close command delivery",
  "workspace.commandSource": "Command source",
  "workspace.adHocCommand": "Ad-hoc command",
  "workspace.savedSnippet": "Saved snippet",
  "workspace.command": "Command",
  "workspace.chooseSnippet": "Choose a snippet",
  "workspace.useDefault": "Use default",
  "workspace.targetMode": "Connected terminals",
  "workspace.targetCount": "{count} targets",
  "workspace.paneNumber": "Pane {number}",
  "workspace.targetSkipped": "Not sent ({state})",
  "workspace.executionNotice":
    "The command and Enter are sent to each terminal's current input, preserving its working directory, environment, and shell state. Partially typed input or a running foreground process may receive the text instead of a shell prompt.",
  "workspace.previewHeading": "Review terminal input",
  "workspace.sendTargets": "Send to {count} terminals",
  "workspace.deliveryResults": "Delivery results",
  "workspace.deliveryNotice":
    "Command output appears in each terminal. Delivery does not mean the command has finished.",
  "workspace.delivered": "Sent",
  "workspace.deliveryFailed": "Not sent",
  "snippets.heading": "Snippets",
  "snippets.new": "New",
  "snippets.empty": "No snippets have been saved.",
  "snippets.name": "Name",
  "snippets.description": "Description",
  "snippets.command": "Command",
  "snippets.variableHint":
    "Use {{name}} placeholders. Values are reviewed before execution.",
  "snippets.variableType": "Variable type",
  "snippets.variableType.string": "String",
  "snippets.variableType.integer": "Integer",
  "snippets.variableType.boolean": "Boolean",
  "snippets.variableType.secret": "Secret",
  "snippets.variableType.unknown": "Unknown type",
  "snippets.value": "Value",
  "snippets.save": "Save",
  "snippets.delete": "Delete",
  "snippets.targets": "Target hosts",
  "snippets.preview": "Preview execution",
  "snippets.confirm": "Review exact commands",
  "snippets.run": "Run on these hosts",
  "snippets.results": "Results",
  "snippets.status.running": "Running",
  "snippets.status.completed": "Completed",
  "snippets.status.cancelled": "Cancelled",
  "snippets.status.queued": "Queued",
  "snippets.status.succeeded": "Succeeded",
  "snippets.status.failed": "Failed",
  "snippets.status.unknown": "Status unavailable",
  "snippets.cancel": "Cancel",
  "snippets.startup": "Connection startup",
  "snippets.startupHint":
    "Run this snippet after the selected host's shell is ready. Secret variables are not allowed.",
  "snippets.setStartup": "Set startup snippet",
  "snippets.clearStartup": "Clear",
  "host.duplicateKeyword":
    "A previous line in this block uses the same keyword. OpenSSH keeps the first one.",

  "terminal.consoleList": "Open consoles",
  "terminal.noSessions": "No console is open.",
  "terminal.openShell": "Local shell",
  "terminal.openShellOnce": "Open another local shell once",
  "terminal.rowDetail": "{status} · {destination}",
  "terminal.running": "connected",
  "terminal.connecting": "connecting",
  "terminal.connected": "connected",
  "terminal.agentWorking": "working",
  "terminal.agentAttention": "input needed",
  "terminal.agentReady": "ready",
  "terminal.agentUnknown": "state unavailable",
  "terminal.agentNotificationAttention": "{subject} is waiting for input",
  "terminal.agentNotificationCompleted": "{subject} has finished",
  "terminal.unreadAttention": "Unread: input needed",
  "terminal.unreadCompleted": "Unread: completed",
  "terminal.unreadWorkspace": "This workspace has unread Agent activity",
  "terminal.browserNotificationsHeading": "Notifications",
  "terminal.browserNotificationsDefault":
    "Allow sshc to notify you when an Agent finishes or needs input while this tab is in the background.",
  "terminal.browserNotificationsGranted":
    "Browser notifications are allowed. sshc only sends them while this tab is in the background.",
  "terminal.browserNotificationsDenied":
    "Notifications are blocked for sshc. Allow them in this site's browser settings to enable Agent notifications.",
  "terminal.browserNotificationsUnsupported":
    "This browser does not support web notifications.",
  "terminal.browserNotificationsEnable": "Enable notifications",
  "terminal.browserNotificationsTest": "Send test notification",
  "terminal.browserNotificationsEnabled": "Notifications enabled",
  "terminal.browserNotificationsReady": "Agent notifications are ready.",
  "terminal.browserNotificationsRequestFailed":
    "Notification permission could not be requested.",
  "terminal.browserNotificationsDeliveryFailed":
    "The browser allowed notifications but could not display one.",
  "terminal.notificationAttentionSound": "Input-needed sound",
  "terminal.notificationCompletedSound": "Completion sound",
  "terminal.notificationSoundHint": "Stored only in this browser.",
  "terminal.notificationSound.none": "No sound",
  "terminal.notificationSound.gentle": "Gentle",
  "terminal.notificationSound.bell": "Bell",
  "terminal.notificationSound.pulse": "Pulse",
  "terminal.notificationPreview": "Play",
  "terminal.notificationPreviewAttention": "Play the input-needed sound",
  "terminal.notificationPreviewCompleted": "Play the completion sound",
  "terminal.notificationVolume": "Notification volume",
  "terminal.notificationVolumeHint": "{volume}% · applies to both Agent sounds",
  "terminal.quickCommands": "Quick Commands",
  "terminal.quickCommandsClose": "Close Quick Commands",
  "terminal.quickCommandInsert": "Insert",
  "terminal.quickCommandRun": "Run",
  "terminal.quickCommandCopy": "Copy",
  "terminal.quickCommandSaveSelection": "Save selection as snippet",
  "terminal.quickCommandName": "Snippet name",
  "terminal.quickCommandSave": "Save snippet",
  "terminal.quickCommandSaved": "Snippet saved.",
  "terminal.quickCommandContextWarning":
    "Insert and Run send text to the pane's current input. Partially typed text or a foreground prompt—including a password or passphrase prompt—may receive it.",
  "terminal.quickCommandChanged":
    "The snippet or pane changed. Review the updated preview before continuing.",
  "terminal.quickCommandInsertUnsafe":
    "Commands with line breaks or control characters cannot be inserted without running them. Use Run or Copy instead.",
  "terminal.linkActions": "Terminal link actions",
  "terminal.linkOpenBrowser": "Open in browser",
  "terminal.linkBrowseSFTP": "Browse in SFTP",
  "terminal.linkEditSFTP": "Edit with SFTP",
  "terminal.linkDownloadSFTP": "Download with SFTP",
  "terminal.linkCopy": "Copy link",
  "sftp.linkTargetInvalid": "This terminal link is no longer available.",
  "sftp.linkTargetNotFound": "The remote path no longer exists.",
  "sftp.linkTargetNotFile": "The remote path is not a file.",
  "terminal.agentResumeAvailable": "A {agent} session can be resumed.",
  "terminal.agentResumeSamePane": "Resume here",
  "terminal.agentResumeNewPane": "Resume in new pane",
  "terminal.agentResumeFailed": "The agent session could not be resumed.",
  "terminal.agentResumeStale":
    "The agent session changed. Review it and try again.",
  "terminal.agentResumeSamePaneBusy":
    "This shell has already received input. Resume in a new pane instead.",
  "terminal.agentResumeUnavailable":
    "This agent session is no longer available.",
  "terminal.agentResumeIdentityChanged":
    "This alias now points to a different SSH destination, so the agent session was not resumed.",
  "terminal.progressDialing": "connecting to {target} · {position}",
  "terminal.progressHostKey": "checking the host key for {target} · {position}",
  "terminal.progressAuthenticating":
    "authenticating with {target} · {position}",
  "terminal.progressAuthenticated": "authenticated with {target} · {position}",
  "terminal.progressOpeningSession": "opening the session · {position}",
  "terminal.reconnectingAttempt": "reconnecting {attempt}/{limit}",
  "terminal.exitedWith": "exited {code}",
  "terminal.localhost": "localhost",
  "terminal.emptyHeading": "No console is open",
  "terminal.emptyHint":
    "Open one from the list on the left, or press Connect on a host.",
  "terminal.forwardLocal": "forwarding {listen} → {to}",
  "terminal.forwardDynamic": "SOCKS5 proxy on {listen}",
  "terminal.forwardAgent": "forwarding the SSH agent to the remote host",
  "terminal.rowMenu": "Actions for {title}",
  "terminal.rename": "Rename",
  "terminal.unpinTitle": "Use automatic name",
  "terminal.renameLabel": "New name for {title}",
  "terminal.renameFailed": "The console could not be renamed.",
  "terminal.duplicate": "Duplicate this connection",
  "terminal.moveUp": "Move up",
  "terminal.moveDown": "Move down",
  "terminal.closeSession": "Close {title}",
  "terminal.closeHeading": "Close {title}?",
  "terminal.closeBody":
    "This ends the connection. Running processes and visible output will be lost.",
  "terminal.closeForwards":
    "This also closes {count} port forward(s) for the console.",
  "terminal.closeConfirm": "Close",
  "terminal.closeCancel": "Keep it open",
  "desktop.closeAllHeading2": "Close {count} running console(s)?",
  "desktop.closeAllBody":
    "This ends every connection. Running processes and visible output will be lost.",
  "desktop.closeAllConfirm": "Close them all",
  "desktop.closeAllCancel": "Keep them open",
  "terminal.limitReached":
    "The limit of {max} open consoles has been reached. Close one to open another.",
  "terminal.limitRefused": "No more consoles can be opened. Close one first.",
  "terminal.unresolvable":
    "The settings for this connection could not be resolved. Open Analysis to see why.",
  "terminal.proxyCommandWithJump":
    "ProxyCommand and ProxyJump cannot be set together. Remove one of them. OpenSSH also rejects this configuration.",
  "terminal.jumpDepthExceeded":
    "The ProxyJump chain exceeds the supported depth.",
  "terminal.hostKeyUnknown":
    "The host key is not trusted yet. Connect interactively after reviewing it.",
  "terminal.hostKeyChanged":
    "The host key changed. Review Known Hosts before connecting again.",
  "terminal.hostKeyRevoked":
    "The host key is marked as revoked and cannot be used.",
  "terminal.identityUnavailable":
    "No usable identity or SSH agent key is available.",
  "terminal.authenticationUnavailable":
    "No supported authentication method is available for this connection.",
  "terminal.authenticationCancelled": "Authentication was cancelled.",
  "terminal.keyPassphraseRequired":
    "The private key needs a passphrase. Unlock or update its saved credential.",
  "terminal.reconnectFailed":
    "The reconnect attempt failed. sshc will retry within the configured limit.",
  "terminal.reconnectExhausted":
    "The reconnect limit was reached. Open a new connection when the network is ready.",
  "terminal.manualReconnect": "Reconnect",
  "terminal.manualReconnecting": "Connecting…",
  "terminal.manualReconnectFailed":
    "The SSH session could not be reconnected. Check the connection settings and network, then try again.",
  "terminal.openFailed": "The console could not be opened.",
  "terminal.keyBar": "On-screen keys",
  "terminal.closeFailed": "The console could not be closed.",
  "terminal.linkConnecting": "Connecting…",
  "terminal.linkRetrying": "Connecting… (attempt {attempt})",
  "terminal.linkWaiting":
    "The connection dropped. Attempt {attempt} in {seconds}s.",
  "terminal.linkStopped":
    "Retrying has stopped. The session is still available; reconnect whenever you are ready.",
  "terminal.linkGone":
    "The session no longer exists, so it cannot be reconnected.",
  "terminal.linkNow": "Connect now",
  "terminal.linkStop": "Stop retrying",
  "terminal.clipboardRefused": "The clipboard could not be accessed.",
  "terminal.search": "Find",
  "terminal.searchInput": "Search terminal output",
  "terminal.searchPlaceholder": "Search scrollback…",
  "terminal.searchNoResults": "No results",
  "terminal.searchPrevious": "Previous match",
  "terminal.searchNext": "Next match",
  "terminal.searchClose": "Close search",
  "terminal.searchCaseSensitive": "Match case",
  "terminal.searchRegex": "Use regular expression",
  "terminal.searchInvalidRegex": "Invalid",
  "terminal.copyContext": "Copy recent terminal context",
  "terminal.copyContextHint":
    "Copy up to the latest 200 terminal rows without control sequences",
  "terminal.copyContextDone": "Recent terminal context copied.",
  "terminal.copyContextEmpty": "There is no terminal context to copy.",
  "terminal.osc52Hint":
    "Allow this terminal session to write to the system clipboard via OSC 52",
  "terminal.osc52Enabled":
    "OSC 52 clipboard writes are allowed for this terminal session.",
  "terminal.osc52Disabled": "OSC 52 clipboard writes are blocked.",
  "terminal.osc52Copied":
    "The remote application copied text to the system clipboard.",
  "terminal.moreActions": "More terminal actions",
  "terminal.portForwarding": "Port forwarding",
  "terminal.forwardDescription":
    "Manage tunnels using the active SSH connection to {title}.",
  "terminal.forwardClose": "Close",
  "terminal.forwardActive": "Active forwarding",
  "terminal.forwardNone": "No forwarding is active on this connection.",
  "terminal.forwardTemporary": "This session",
  "terminal.forwardSaved": "Saved",
  "terminal.forwardSavedStopHint":
    "Stopping it here does not remove it from the connection settings.",
  "terminal.forwardRetryHint":
    "Fix the local port conflict, then reconnect this SSH session to retry the saved forwarding.",
  "terminal.forwardAgentLabel": "SSH agent",
  "terminal.forwardCopy": "Copy address",
  "terminal.forwardCopied": "The address was copied.",
  "terminal.forwardCopyFailed": "The address could not be copied.",
  "terminal.forwardStop": "Stop",
  "terminal.forwardStopping": "Stopping…",
  "terminal.forwardStopped": "The forwarding was stopped.",
  "terminal.forwardNew": "Start a forwarding",
  "terminal.forwardSaveConnection": "Save to this connection",
  "terminal.forwardSaveHint":
    "The forwarding will start now and be restored the next time this connection opens.",
  "terminal.forwardSaveUnavailable":
    "A local shell or an unsaved connection cannot store this setting.",
  "terminal.forwardNeedsConnection":
    "Reconnect this SSH session before starting a forwarding.",
  "terminal.forwardStart": "Start",
  "terminal.forwardStarting": "Starting…",
  "terminal.forwardStarted": "The forwarding is active for this session.",
  "terminal.forwardStartedAndSaved":
    "The forwarding is active and was saved to the connection.",
  "terminal.forwardPausedReconnect":
    "Forwarding listeners are unavailable while this SSH connection is reconnecting.",
  "terminal.forwardBindFailed": "The local port could not be opened: {detail}",
  "terminal.forwardUnavailable":
    "This SSH connection cannot change forwarding in its current state.",
  "terminal.forwardInvalid":
    "Check the forwarding type, port, and destination.",
  "terminal.forwardSaveFailed":
    "The forwarding started, but it could not be saved to the connection.",
  "terminal.forwardFailed": "The forwarding operation could not be completed.",
  "terminal.settingsHeading": "Terminal",
  "terminal.settingsSaved":
    "Saved. Clipboard choices apply now; new consoles use the other settings.",
  "terminal.settingsLoading": "Loading terminal settings…",
  "terminal.settingsStorageHint":
    "These settings, including terminal appearance, are stored in workspace metadata and follow backups and sync. Theme, language and notification sounds are stored only in this browser.",
  "terminal.maxSessionsLabel": "Consoles open at once",
  "terminal.maxSessionsHint":
    "Enter a value from 1 to 200. Leave it empty to use 50. When the limit is reached, no new console can be opened; existing consoles remain open.",
  "terminal.scrollbackLabel": "Engine replay buffer (bytes)",
  "terminal.scrollbackHint":
    "Output retained by the engine for browser reconnects. Enter 16384–4194304; blank uses 262144 bytes (256 KiB). It stays in memory and is not written to disk.",
  "terminal.browserScrollbackLabel": "Browser scrollback (lines)",
  "terminal.browserScrollbackHint":
    "Lines kept by each browser terminal. Enter 1000–100000; blank uses 5000. Larger values consume more browser memory.",
  "terminal.localShellProfileLabel": "Default local shell",
  "terminal.localShellProfileHint":
    "Choose from executables detected and verified on this machine. A shell can also be chosen once when opening a local terminal.",
  "terminal.localShellProfileSystem": "System login shell",
  "terminal.osc52DefaultLabel": "Allow OSC 52 clipboard writes by default",
  "terminal.osc52DefaultHint":
    "Local shells use this setting. Each SSH connection can inherit, allow, or deny it in sshc settings.",
  "terminal.jisYenBackslashLabel": "Send the JIS ¥ key as backslash",
  "terminal.jisYenBackslashHint":
    "Useful with Japanese keyboards. IME composition is not modified.",
  "terminal.fontSizeLabel": "Font size",
  "terminal.paletteLabel": "Colour scheme",
  "terminal.verbosityLabel": "Connection log",
  "terminal.verbosityHint":
    "Displays connection details in the console, like ssh -v. Applies to new connections.",
  "terminal.verbosityQuiet": "None",
  "terminal.verbosityBrief": "Basic details (-v)",
  "terminal.verbosityDetailed": "Keys, hops and timings (-vv)",
  "terminal.verbosityFull": "Everything (-vvv)",
  "terminal.reconnectLabel": "Reconnect after a dropped connection",
  "terminal.reconnectHint":
    "Number of reconnection attempts after an unexpected disconnect. Retries use 1, 2, 5, 10, and 15 second base delays with jitter, so five attempts take up to 40 seconds. A console you close manually is not reconnected.",
  "terminal.reconnectDefault": "Default (5 attempts, up to 40 seconds)",
  "terminal.reconnectNever": "Do not reconnect",
  "terminal.reconnectOnce": "Once (up to 2 seconds)",
  "terminal.reconnectTwice": "Twice (up to 4 seconds)",
  "terminal.reconnectThrice": "Three times (up to 10 seconds)",
  "terminal.reconnectFive": "Five times (up to 40 seconds)",
  "engine.heading": "Engine",
  "engine.portLabel": "Port",
  "engine.portHint":
    "Leave this empty to use a device-local stable port (initially 54447). A stable port keeps bookmarks and the installed web app working. Applies the next time the engine starts.",
  "engine.portOutOfRange": "The port must be between 1024 and 65535.",
  "engine.loading": "Loading engine settings…",
  "engine.saved": "Saved. Auto-lock applies now; a port change applies the next time the engine starts.",
  "engine.saveFailed": "The engine setting could not be saved.",
  "engine.vaultAutoLockLabel": "Vault auto-lock",
  "engine.vaultAutoLockHint":
    "Lock the Vault after no saved credential or key passphrase has been used for this long. Status checks, terminal output, and background sync do not extend the timer.",
  "engine.vaultAutoLockIdle": "Lock after inactivity",
  "engine.vaultAutoLockRestart": "Do not auto-lock",
  "engine.vaultAutoLockValue": "Time",
  "engine.vaultAutoLockUnit": "Unit",
  "engine.vaultAutoLockMinutes": "Minutes",
  "engine.vaultAutoLockHours": "Hours",
  "engine.vaultAutoLockOutOfRange": "Enter a whole number from 1 to 999 for the auto-lock time.",
  "engine.vaultAutoLockRestartWarning":
    "The Vault will not lock automatically. Unless you lock it manually, it remains unlocked until sshc is restarted. Use this setting only on a device you control.",
  "terminal.fontLabel": "Font family",
  "terminal.backgroundLabel": "Background image",
  "terminal.backgroundHint":
    "Images are stored in your workspace and included in backups and sync.",
  "terminal.backgroundNone": "No image",
  "terminal.backgroundFollowsOverall": "Use the overall setting",
  "terminal.backgroundAdd": "Add an image",
  "terminal.backgroundRemove": "Remove {name}",
  "terminal.backgroundRoom": "{megabytes} MB left",
  "terminal.backgroundTooLarge": "The image exceeds the maximum file size.",
  "terminal.backgroundsFull": "There is no room left for another image.",
  "terminal.backgroundNotAnImage":
    "The selected file is not a supported image.",
  "terminal.backgroundFailed": "The image could not be stored.",
  "terminal.tintLabel": "Image overlay opacity",
  "terminal.tintHint":
    "Increase the opacity to darken the image and improve text readability.",
  "connection.backgroundLabel": "Console background image",
  "connection.backgroundHint": "Only this connection's consoles use it.",
  "terminal.fontHint": "JetBrains Mono ships with the application.",
  "terminal.fontFollowsSystem": "Use the device's monospace font",
  "terminal.fontFollowsOverall": "Use the overall setting",
  "connection.fontLabel": "Console font family",
  "connection.fontHint": "Only this connection's consoles use it.",
  "terminal.paletteHint": "Applies to consoles that have not chosen their own.",
  "terminal.paletteFollowsTheme": "Follow the application theme",
  "terminal.paletteFollowsOverall": "Use the overall setting",
  "connection.paletteLabel": "Console colour scheme",
  "connection.paletteHint": "Only this connection's consoles use it.",
  "connection.encodingLabel": "Remote text encoding",
  "connection.encodingHint":
    "Used by this connection in the browser terminal and by sshc on the command line.",
  "connection.encodingUTF8": "UTF-8 (default)",
  "connection.encodingShiftJIS": "Shift_JIS (Japanese)",
  "connection.encodingEUCJP": "EUC-JP (Japanese)",
  "connection.encodingISO2022JP": "ISO-2022-JP (Japanese)",
  "connection.osc52Label": "OSC 52 clipboard",
  "connection.osc52Hint":
    "Choose whether remote applications on this SSH connection may write to the system clipboard.",
  "connection.osc52Inherit": "Use terminal default",
  "connection.osc52Allow": "Allow for this connection",
  "connection.osc52Deny": "Block for this connection",
  "terminal.fontSizeHint":
    "Pixels. Empty follows the screen — 15 on a narrow one, 13 otherwise.",
  "terminal.copyOnSelectLabel": "Copy selected text automatically",
  "terminal.copyOnSelectHint":
    "Copies once when you finish selecting. Turn this off if selections should not replace the system clipboard.",
  "terminal.rightClickPasteLabel": "Paste with right click",
  "terminal.rightClickPasteHint":
    "Uses terminal bracketed paste when supported. Turn this off to keep the normal context menu.",
  "terminal.limitsOutOfRange":
    "Those numbers are outside the range this build accepts.",
  "terminal.startLabel": "Starting directory",
  "terminal.startHint":
    "Directory in which local shells start. Enter ~/work or an absolute path. The ~ is stored without expansion, so the same setting can be used on another machine. Leave this empty to use your home directory.",
  "terminal.startSave": "Save",
  "terminal.startMissing": "The specified directory does not exist.",
  "terminal.startNotADirectory": "The specified path is not a directory.",
  "terminal.startUnusable":
    "Write the path as ~/something or as an absolute path.",
  "terminal.settingsSaveFailed": "The terminal settings could not be saved.",
  "terminal.screenLabel": "Console for {title}",
  "terminal.exitedWithCode":
    "The program exited with status {code}. The output above is kept until you close this console.",
  "terminal.exitedWithSignal":
    "The program was ended by {signal}. The output above is kept until you close this console.",
  "inspector.appOnly": "sshc-only settings",
  "inspector.groupLabel": "Group display settings",
  "inspector.hostSavesImmediately": "Changes here are saved immediately.",
  "inspector.groupChangesStaged":
    "Changes here are staged until you choose Save groups.",
  "inspector.notices": "Notices",
  "inspector.inherited": "Inherited values",
  "inspector.noNotices": "Nothing to report for this connection.",
  "inspector.noInherited":
    "All values for this connection are defined in its own block.",
  "shell.language": "Lang",
  "shell.languageMenu": "Lang menu",
  "shell.preferenceMenu": "Display menu",
  "shell.languageEnglish": "English",
  "shell.languageJapanese": "日本語",
  "shell.theme": "Appearance",
  "shell.themeMenu": "Theme menu",
  "shell.themeSystem": "System",
  "shell.themeLight": "Light",
  "shell.themeDark": "Dark",

  "section.home": "Home",
  "section.menu": "Menu",
  "section.connections": "Connections",
  "section.terminal": "Terminal",
  "section.config": "Config",
  "section.groups": "Groups",
  "section.keys": "Keys",
  "section.knownHosts": "Known Hosts",
  "section.remoteKeys": "Install Key on Server",
  "section.diagnostics": "Ad hoc checks",
  "section.settings": "Settings",
  "settings.heading": "Settings",
  "settings.pageDescription":
    "Configure embedded terminals, sshc's lifecycle, and protection for this machine's encrypted data.",
  "settings.engineDescription":
    "Configure the sshc engine port and automatic vault locking.",
  "settings.terminalDescription":
    "Configure the behavior, appearance, and controls used by new terminals.",
  "settings.notificationsDescription":
    "Configure browser notifications and sounds for agent status changes.",
  "settings.connectionsDescription":
    "Review the connections open in this browser and close them together.",
  "settings.passwordDescription":
    "Change the master password that protects encrypted data on this machine.",
  "secrets.heading": "The vault",
  "secrets.pageDescription":
    "Manage named account passwords and key passphrases. Values are shown only while editing.",
  "secrets.metricPasswords": "Account passwords",
  "secrets.metricPassphrases": "Key passphrases",
  "secrets.metricAssignments": "Assignments",
  "secrets.loading": "Reading the vault…",
  "secrets.explainNew":
    "One master password encrypts account passwords, key passphrases, object-storage credentials, and snippets. The master password is not stored and cannot be recovered. If you lose it, this encrypted data cannot be recovered; files read directly by OpenSSH are unaffected.",
  "secrets.explainLocked":
    "The vault is locked. Enter the master password to access it.",
  "secrets.master": "Master password",
  "secrets.create": "Create the vault",
  "secrets.unlock": "Unlock",
  "secrets.lock": "Lock sshc",
  "secrets.createFailed":
    "The vault could not be created. A master password must be at least 12 characters.",
  "secrets.unlockFailed": "The master password is incorrect.",
  "secrets.failed": "The vault could not be read.",
  "secrets.storeFailed": "The credential could not be saved.",
  "secrets.deleteFailed": "The credential could not be deleted.",
  "secrets.inUse":
    "This credential is still assigned. Change its assignments before deleting it.",
  "secrets.none": "Nothing is stored here yet.",
  "secrets.assignedHosts": "Assigned hosts",
  "secrets.noAssignedHosts": "No assigned hosts",
  "secrets.keys": "Keys",
  "secrets.noKeys": "No assigned keys",
  "secrets.dedicated": "Dedicated to this key",
  "secrets.removeDedicated": "Remove saved passphrase for {key}",
  "secrets.keyHostUsageIncomplete":
    "Key host assignments could not be fully confirmed from SSH configuration. Check Config diagnostics.",
  "secrets.keyHostsUnavailable": "Could not confirm assigned hosts",
  "secrets.delete": "Delete {name}",
  "secrets.edit": "Edit {name}",
  "secrets.editPassword": "Edit account password",
  "secrets.editPassphrase": "Edit key passphrase",
  "secrets.editNote":
    "The saved value is shown below. Renaming keeps every current assignment.",
  "secrets.credentialName": "Name",
  "secrets.passwordValue": "Password",
  "secrets.passphraseValue": "Key passphrase",
  "secrets.revealing": "Reading the saved value…",
  "secrets.revealFailed": "The saved value could not be shown.",
  "secrets.updateFailed": "The changes could not be saved.",
  "secrets.nameExists":
    "A credential with that name already exists. Choose another name.",
  "secrets.saveChanges": "Save changes",
  "secrets.saving": "Saving…",
  "secrets.cancel": "Cancel",
  "secrets.passwordsHeading": "Account passwords",
  "secrets.passphrasesHeading": "Key passphrases",
  "secrets.newPasswordName": "New account password name",
  "secrets.newPasswordValue": "New account password value",
  "secrets.storePassword": "Store account password",
  "secrets.newPassphraseName": "New key passphrase name",
  "secrets.newPassphraseValue": "New key passphrase value",
  "secrets.storePassphrase": "Store key passphrase",
  "update.version": "Version {version}",
  "update.available": "{version} is available — read what changed",
  "desktop.closeAllHeading": "Open connections",
  "desktop.closeAllNote":
    "This closes every console, port forward, and SSH agent forwarding session. The engine keeps running.",
  "desktop.openCount": "{count} open",
  "desktop.closeAll": "Close every connection",
  "secrets.changeHeading": "Master password",
  "secrets.changeNote":
    "Changing the master password re-encrypts the local vault, snippets, sync settings, and every local backup. Remote snapshots use the separate synchronization key and are not rewritten.",
  "secrets.currentMaster": "Current master password",
  "secrets.newMaster": "New master password",
  "secrets.confirmMaster": "Confirm new master password",
  "secrets.change": "Change the master password",
  "secrets.wrongCurrent":
    "The current master password is incorrect. Nothing was changed.",
  "secrets.changeFailed": "The master password could not be changed.",
  "secrets.changedMasterLocally":
    "The master password was changed. The local vault, snippets, sync settings, and local backups now use the new password. Remote snapshots were not rewritten.",
  "section.secrets": "Secrets",
  "lock.explainNew":
    "Choose a master password to encrypt stored passwords, key passphrases, snippets, sync settings, and all backups created by sshc.",
  "lock.explainOpen": "Give your master password to open sshc.",
  "lock.noRecovery":
    "The master password cannot be recovered. If you lose it, the vault, snippets, and encrypted backups cannot be opened.",
  "lock.password": "Master password",
  "lock.confirm": "Confirm master password",
  "lock.create": "Create the vault",
  "lock.open": "Open",
  "lock.wrong": "The master password is incorrect.",
  "lock.tooShort": "A master password must be at least {count} characters.",
  "lock.alreadyExists":
    "A vault appeared in app storage. Enter its master password to open it.",
  "lock.storagePermission":
    "Android denied access to the app's private storage. Copy the details below for support.",
  "lock.storageFull":
    "There is not enough free storage to create or update the vault.",
  "lock.storageReadOnly":
    "The app's private storage is read-only. Restart the device and try again.",
  "lock.storageBusy":
    "Another vault update is still finishing. Wait a moment and try again.",
  "lock.storageIO":
    "Android reported an input/output failure while accessing the app's private storage.",
  "lock.schemaOlder":
    "The vault format is old (required: {required}, current: {current}).",
  "lock.schemaNewer":
    "The vault format is newer than this sshc build (supported: {required}, current: {current}).",
  "lock.migrationFailed":
    "The vault could not be migrated from version {current} to {required}. The original vault was not changed.",
  "lock.migrationCompleted":
    "The vault was safely migrated from version {current} to {required}.",
  "lock.migrationDismiss": "Dismiss migration notice",
  "lock.envelopeUnsupported":
    "The encrypted vault container uses an unsupported format. Open diagnostic details for support.",
  "lock.schemaRecoveryHint":
    "Restore the newest compatible local backup first. If none exists, you can create an empty vault without deleting SSH configuration or key files.",
  "lock.restoreCompatibleBackup": "Restore a compatible vault backup",
  "lock.noCompatibleBackup":
    "No compatible local vault backup was found. Nothing was changed.",
  "lock.recoveryFailed": "The compatible vault backup could not be restored.",
  "lock.resetUnsupportedAcknowledge":
    "I understand that saved passwords, saved key passphrases, and synchronization settings will be reset. SSH configuration and key files remain.",
  "lock.resetUnsupported": "Create an empty vault",
  "lock.resetFailed": "The unsupported vault could not be safely replaced.",
  "lock.failed": "The vault could not be opened.",
  "section.sync": "Sync",
  "section.history": "History",

  "home.heading": "Your connections",
  "home.manageConnections": "Manage connections",
  "home.connections": "Connections",
  "home.groups": "Groups",
  "home.attention": "Needs attention",
  "home.quickConnect": "Quick connect",
  "home.quickConnectHint":
    "Recently used hosts stay first; unused hosts follow in name order.",
  "home.recentConnections": "Recent connections",
  "home.recentConnectionsHint": "Successful SSH connections on this device.",
  "home.recentConnectionList": "Recently used connections",
  "home.savedWorkspaces": "Saved layouts",
  "home.savedWorkspacesHint":
    "Recreate saved connection targets and pane placement in one action.",
  "home.savedWorkspaceList": "Saved terminal layouts",
  "home.workspacePanes": "{count} panes",
  "home.workspaceUpdated": "updated {at}",
  "home.openWorkspace": "Open layout",
  "home.lastConnected": "Last connected {at}",
  "home.connectionList": "Available connections",
  "home.search": "Search connections",
  "home.searchPlaceholder": "Search hosts, groups or tags",
  "home.viewMode": "Connection layout",
  "home.panelView": "Panels",
  "home.listView": "List",
  "home.groupFilter": "Filter connections by group",
  "home.allGroups": "All",
  "home.groupCount": "Groups {count}",
  "home.connectionCount": "Connections {count}",
  "home.groupBreadcrumb": "Selected group",
  "home.openGroup": "Open {name}, {count} connections",
  "home.noChildGroups": "No groups at this level.",
  "home.pointerHint": "Mouse: double-click · Touch: tap once",
  "home.touchHint": "Tap once to connect",
  "home.connectGesture":
    "Connect to {alias}. Double-click with a mouse or tap once on a touch screen.",
  "home.neverConnected": "Not connected yet",
  "home.loading": "Reading your SSH configuration…",
  "home.noConnections": "No concrete Host alias is configured yet.",
  "home.noMatches": "No connection matches this search.",
  "home.groupMissingDetail": "The selected group {name} no longer exists.",
  "home.ungrouped": "No group",
  "home.tagsFor": "Tags for {alias}",
  "home.connectionActions": "Actions for {alias}",
  "home.openConnectionSettings": "Open connection settings",
  "home.connect": "Connect",
  "home.opening": "Opening…",
  "home.loadFailed": "The SSH configuration could not be read.",
  "home.workspace": "Workspace",
  "home.workspaceUnavailable": "Workspace status is unavailable.",
  "home.workspaceClean":
    "No configuration problem or interrupted change needs attention.",
  "home.workspaceAttention":
    "{count} configuration or recovery item(s) need attention.",
  "home.openConfig": "Review configuration",
  "home.recoverChanges": "Recover changes",
  "home.sync": "Sync",
  "home.syncUnavailable": "Sync status is unavailable.",
  "home.syncNotConfigured": "Remote sync is not configured.",
  "home.syncNever":
    "A remote bucket is configured but has not been synced yet.",
  "home.syncLast": "Last synced {at} · {count} files",
  "home.openSync": "Open sync",

  "copy.button": "Copy {label}",
  "copy.done": "Copied.",
  "copy.refused": "The browser refused to write to the clipboard.",
  "copy.command": "command",
  "copy.terminalCommand": "Terminal command",
  "copy.privateKey": "private key",
  "copy.publicKey": "public key",
  "copy.keyLine": "key line",
  "copy.remoteCommand": "remote command",
  "copy.diagnosticReport": "diagnostic report",

  "diagnostic.requestFailed": "The operation could not be completed",
  "diagnostic.requestFailedHint":
    "sshc reported {code}. Open the safe diagnostic details when reporting the problem.",
  "diagnostic.showDetails": "Show diagnostic details",
  "diagnostic.dismiss": "Dismiss error",

  "history.requestRejected": "The request was rejected ({code}).",
  "history.pageTitle": "History",
  "history.pageDescription":
    "Review every completed change, recover interrupted writes, and restore individual files without losing the newer history.",
  "history.metricChanges": "Completed changes",
  "history.metricInterrupted": "Interrupted",
  "history.metricRestorable": "Restorable files",
  "history.operation.configuration": "Configuration change",
  "history.operation.connection": "Connection change",
  "history.operation.key": "Key change",
  "history.operation.terminal": "Terminal settings change",
  "history.operation.engine": "Engine settings change",
  "history.operation.vault": "Vault change",
  "history.operation.sync": "Sync change",
  "history.operation.other": "Application change",
  "history.status.staging": "Preparing",
  "history.status.staged": "Interrupted",
  "history.status.applied": "Applied",
  "history.status.completed": "Completed",
  "history.status.rolledBack": "Rolled back",
  "history.status.unknown": "Recorded",
  "history.interrupted": "Interrupted transactions",
  "history.interruptedDetail":
    "{operation} started {startedAt}: {committed} of {total} files were written.",
  "history.complete": "Complete",
  "history.rollBack": "Roll back",
  "history.loading": "Loading history…",
  "history.restored": "Restored {path} as transaction {id}.",
  "history.completedTransaction": "The interrupted transaction was completed.",
  "history.rolledBack": "The interrupted transaction was rolled back.",
  "history.completed": "Completed changes",
  "history.empty": "No change has been made through this application yet.",
  "history.restorePath": "Restore {path}",
  "history.backupsKept":
    "Generation backups are kept in ~/.ssh/sshc/backups and are never deleted automatically. A restore is itself a new transaction, so it can be undone the same way.",

  "notice.complex_external_rule":
    "This value cannot be edited in the simplified view because its source uses a wildcard, negation, Match block, or duplicate alias. The source is shown instead.",
  "notice.duplicate_alias":
    "Another block declares the same alias. OpenSSH uses the first one it reads.",
  "notice.wildcard_shadow":
    "A catch-all block can override values for this host.",
  "notice.negated_pattern": "A negated pattern applies here.",
  "notice.unnamed_host_block":
    "This block has no concrete alias and can only be edited as raw text.",
  "notice.match_block": "A Match block was found.",
  "notice.dangerous_directive":
    "This directive can run a command. It is saved as written and never executed by this application.",
  "notice.unstructured_line":
    "This line has unbalanced quoting and is preserved exactly as written.",
  "notice.external_file":
    "This file is outside ~/.ssh. It is shown but never written.",
  "notice.orphan_metadata":
    "The host for this note no longer exists. Review the target before reassigning it.",
  "notice.group_cycle": "This group's parents form a cycle, so it was skipped.",
  "notice.group_member_missing":
    "This group member has no host block in the configuration.",
  "refusal.directory_not_empty":
    "The directory is not empty. Delete its files first; Include lines that directly reference those files will be updated at the same time.",
  "refusal.not_a_directory": "The specified path is a file, not a directory.",
  "refusal.group_is_declared":
    "{detail} is a declared group. Rename or remove it on the Groups screen so its connections, shared settings, and keys are moved together.",
  "refusal.destination_exists":
    "A file or directory with that name already exists at the destination.",
  "refusal.alias_already_declared":
    "A connection with that name already exists, so these changes cannot be saved. Choose another name.",
  "refusal.region_damaged":
    "~/.ssh/config contains only one marker for the block generated by sshc. The generated range cannot be identified, so nothing was written.",
  "notice.group_not_declared":
    "This directory is under connections/, but no Include line references it. Declare it as a group or move its files.",
  "notice.group_directory_missing":
    "This group is declared but its directory is not there. Nothing is read for it until something is put in one.",
  "notice.group_empty": "This group is declared but contains no files.",
  "notice.generated_region_damaged":
    "~/.ssh/config has the opening marker of this application's generated block but not its closing one, so it cannot tell where its own lines stop. The Include lines in it still work; groups cannot be saved until the closing marker is put back. Add a line reading “# <<< sshc groups” after the last generated Include, or delete the whole block and save the groups again.",
  "notice.explained_values_only":
    "Part of the configuration could not be read, so these values come from what was reachable.",
  "notice.match_exec_refused":
    "This configuration has a Match exec block. Nothing here runs commands, so these values cannot be resolved. Connect from a terminal with ssh instead.",
  "notice.match_final_refused":
    "This configuration has a Match final block. OpenSSH reads the file twice for that, which this engine does not do, so these values cannot be resolved.",
  "notice.canonicalise_refused":
    "This configuration turns on CanonicalizeHostname, which re-reads the configuration. These values cannot be resolved here.",
  "notice.unknown_token_refused":
    "This configuration uses a token this engine does not expand, so these values cannot be resolved.",
  "notice.destination_not_included":
    "No Include references this file, so OpenSSH will not read the moved connection until one is added.",
  "notice.group_file_unreached":
    "This file is under connections/, but no Include references it, so OpenSSH does not read it. Move it into a declared group.",
  "notice.group_directory_leftover":
    "The group directory is now empty. sshc does not remove empty directories automatically; delete it manually if it is no longer needed.",
  "notice.include_no_longer_matches":
    "A pattern that reached this file will not match its new name, so OpenSSH will stop reading it.",
  "notice.include_not_rewritten":
    "An Include references this file in a format that sshc cannot rewrite. The Include was not changed; review it manually.",
  "notice.include_now_unreached":
    "No Include references the new path, so OpenSSH will not read this file until one is added.",

  "preview.heading": "Save preview",
  "preview.newFile": " (new file)",
  "preview.tooLarge":
    "This file is too large for a line-by-line preview, so the whole file is shown as replaced.",
  "preview.syntaxError":
    "Syntax error in {path} at line {line}, column {column}. The edit is kept here and was not written.",
  "preview.theFile": "the file",
  "preview.graphError":
    "This change would break the Include graph. Nothing was written.",
  "preview.conflictError":
    "The file changed outside this application. Nothing was written.",
  "preview.rejected": "The request was rejected ({code}). Nothing was written.",
  "preview.changedOnDisk": "Changed on disk since you loaded it",
  "preview.pendingChange": "Your pending change",
  "preview.mergeByHand":
    "Reload the file to merge the two changes by hand. Nothing was written.",
  "preview.nothingYet": "Change a value to see exactly what would be written.",
  "preview.explainedFor": "Resolved settings for {alias}",
  "preview.unset": "unset",

  "reveal.heading": "Show private key: {path}",
  "reveal.warning":
    "The private key will be displayed in this page and can be copied by anyone who can read this window. This application cannot protect it from browser extensions or from clipboard history tools. Every reveal is recorded in history, without the key itself.",
  "reveal.show": "Show private key",
  "reveal.requesting": "Requesting a one-time confirmation…",
  "reveal.privateKeyLabel": "Private key",
  "reveal.failed":
    "The private key could not be shown. Close this dialog and confirm again.",
  "reveal.close": "Close",

  "orphan.heading": "Settings without a connection",
  "orphan.explain":
    "The Host blocks associated with these notes are no longer in the configuration. Choose the connection to which each note should be assigned.",
  "orphan.chooseTarget": "Choose the connection this note belongs to.",
  "orphan.occupied":
    "{alias} already has its own settings. Clear those first, or discard this note.",
  "orphan.entry": "{alias} in {path}",
  "orphan.noSettings": "no settings",
  "orphan.tags": "tags {tags}",
  "orphan.note": "note “{note}”",
  "orphan.colour": "colour {colour}",
  "orphan.reassociateWith": "Re-associate {alias} with",
  "orphan.reassociatePlaceholder": "Re-associate with…",
  "orphan.reassociate": "Re-associate {alias}",
  "orphan.discard": "Discard {alias} settings",

  "password.blocker.authenticationOff":
    "PasswordAuthentication is off for this host, so the client will never offer a password.",
  "password.blocker.aliasNotSimple":
    "This is a pattern rather than a specific host. To save a password, specify an account on one host.",
  "password.blocker.identityFile":
    "This host has a direct private key. sshc leaves any password prompt to OpenSSH instead of storing or supplying one.",
  "password.warn.hostKeyUnknown":
    "This host key is not in known_hosts. When a saved password is used, the password helper cannot answer the host-key confirmation prompt, so the first connection will fail. Add the host key through Known Hosts first.",
  "password.warn.hostNameUnresolved":
    "No HostName could be resolved for this alias. The password will still be assigned to the alias.",
  "password.show": "Show",
  "password.showNamed": "Show {label}",
  "password.hideNamed": "Hide {label}",
  "password.hide": "Hide",
  "sync.heading": "Remote sync",
  "sync.overviewHeading": "Sync",
  "sync.manageSettings": "Manage sync settings",
  "sync.exclusions.heading": "Files to sync",
  "sync.exclusions.open": "Selection and exclusion rules",
  "sync.exclusions.loading": "Loading synchronization files…",
  "sync.exclusions.summary": "{included} included · {ignored} excluded",
  "sync.exclusions.hint":
    "Unchecked files are not sent, overwritten, or removed by a receive. Existing local copies stay in place.",
  "sync.exclusions.defaults":
    "There is no .sshcignore yet. The built-in rules exclude OS metadata, backups, temporary files, and lock files. Saving shares the rules with every machine.",
  "sync.exclusions.search": "Search file names and paths",
  "sync.exclusions.empty": "No files match this search.",
  "sync.exclusions.sensitiveWarning":
    "A connection file or key is excluded. Other machines may not be able to reproduce that connection.",
  "sync.exclusions.advanced": "Edit .sshcignore",
  "sync.exclusions.rules": "Synchronization exclusion rules",
  "sync.exclusions.syntax":
    "Use Gitignore syntax including *, **, ?, [a-z], and ! to include again. .sshcignore itself and sshc device-local state are handled independently.",
  "sync.exclusions.save": "Save exclusions",
  "sync.exclusions.shared": ".sshcignore is synchronized so every machine uses the same rules.",
  "sync.exclusions.invalid": "The exclusion rules are not valid.",
  "sync.exclusions.loadFailed": "Could not load the synchronization files.",
  "sync.exclusions.saveFailed": "Could not save the exclusion rules.",
  "sync.receiveRemote": "Receive from remote",
  "sync.autoBlockedReason": "Sync is paused. Reason: {reason}",
  "sync.autoFailedReason": "The previous sync failed. Reason: {reason}",
  "sync.setup.check": "Check connection",
  "sync.setup.empty": "Connected. This path has no sync data.",
  "sync.setup.existing": "Existing sync data was found.",
  "sync.setup.incomplete":
    "History exists, but the current snapshot is missing.",
  "sync.setup.useAnotherPath": "To start fresh, choose a different empty path.",
  "sync.setup.existingKey":
    "Enter the encryption key used by this sync data. The actual snapshot will be decrypted before anything is saved.",
  "sync.setup.emptyKey":
    "A strong encryption key will be generated. You may also choose your own.",
  "sync.setup.save": "Verify and save",
  "sync.setup.saved": "Connection details and encryption key saved.",
  "sync.setup.changed":
    "The destination changed after it was checked. Check the connection again.",
  "sync.role.main": "Main device (send and receive)",
  "sync.role.receive": "Receive-only device",
  "sync.role.advanced": "Advanced device roles",
  "sync.role.send": "Send-only device",
  "sync.pageDescription":
    "Synchronise an encrypted snapshot of this SSH workspace between machines through an object-storage bucket.",
  "sync.flowHeading": "Steps to start syncing",
  "sync.flowBucket": "Configure a bucket",
  "sync.flowKey": "Set the shared encryption key",
  "sync.flowOperate": "Inspect, send, or receive",
  "sync.loading": "Reading the sync settings…",
  "sync.warning":
    "Every file in ~/.ssh is synchronised, including private keys. The snapshot is encrypted on this machine with the key below before upload, so the storage provider does not receive plaintext. However, anyone with the bucket credentials can download the encrypted keys and attempt to guess the encryption key offline without a time limit.",
  "sync.statusFailed": "The sync settings could not be read.",
  "sync.bucketHeading": "Bucket",
  "sync.notConfigured": "No bucket is configured yet.",
  "sync.endpoint": "Endpoint",
  "sync.endpointHint":
    "Must be https. For R2 this is https://<account>.r2.cloudflarestorage.com.",
  "sync.bucket": "Bucket name",
  "sync.path": "Path in the bucket",
  "sync.pathHint":
    "Optional. Empty puts the snapshot at the root of the bucket.",
  "sync.region": "Region",
  "sync.regionHint":
    "Optional. Leave this empty to use auto. Use auto for R2; for Amazon S3, enter the bucket's region.",
  "sync.accessKeyId": "Access key ID",
  "sync.secretAccessKey": "Secret access key",
  "sync.credentialsNote":
    "Bucket credentials are encrypted with the master password and excluded from snapshots. Including them would allow anyone who obtained one snapshot to download later snapshots.",
  "sync.sealed":
    "These settings are encrypted with the master password. Unlock the vault to view them.",
  "sync.unlockFailed": "The master password is incorrect.",
  "sync.noVault":
    "This machine has no vault yet. Create one under Secrets, then come back.",
  "sync.direction": "Direction",
  "sync.direction.both": "Send and receive",
  "sync.direction.push": "Send only",
  "sync.direction.pull": "Receive only",
  "sync.direction.both.hint":
    "This machine can send local changes and apply changes sent by other machines.",
  "sync.direction.push.hint":
    "This machine only sends changes. Changes from other machines are not applied, but you can still preview them.",
  "sync.direction.pull.hint":
    "This machine only receives changes. Local changes are not sent to the bucket or other machines.",
  "sync.configure": "Use this bucket",
  "sync.editSettings": "Edit bucket settings",
  "sync.cancelSettings": "Cancel editing",
  "sync.configureFailed": "The bucket could not be configured.",
  "sync.detailsHeading": "Details and history",
  "sync.neverSynced": "This machine has not synced yet.",
  "sync.lastSynced": "Last synced {at}, {count} files.",
  "sync.key": "Encryption key",
  "sync.keyHint":
    "Snapshots are encrypted with this key, not the master password. Enter the same key on every machine that shares this bucket; each machine may use a different master password. If you lose this key, the snapshots cannot be decrypted.",
  "sync.keyMissing":
    "No key yet. Create one here, then enter the same key on your other machines.",
  "sync.keySet": "A key is set. It is not shown again.",
  "sync.keyReady": "Key configured",
  "sync.keyNeeded": "Key required",
  "sync.keyShownOnce":
    "Copy this now and enter it on your other machines. Leave this screen and it will not be shown again.",
  "sync.keyChooseOwn": "Choose the key myself",
  "sync.keyOwnValue": "Key",
  "sync.keyCreate": "Create a key",
  "sync.keyReplace": "Replace the key",
  "sync.keySaved": "Saved.",
  "sync.keyFailed": "The key could not be saved.",
  "sync.keyTooShort":
    "A key you choose yourself must be at least 12 characters.",
  "sync.wrongKey":
    "The saved key cannot decrypt the snapshot in this bucket. Check that every machine uses the same key, or explicitly replace the remote snapshot from Bucket status.",
  "sync.wrongMaster": "The master password for this machine is incorrect.",
  "sync.bucketAuthenticationFailed":
    "The object store could not authenticate the request. Nothing was saved. Check the access key and secret.",
  "sync.bucketAccessDenied":
    "The object store denied access. Nothing was saved. Check the credentials, bucket, region, and key permissions.",
  "sync.bucketRateLimited":
    "The object store is limiting requests. Wait a moment and try again.",
  "sync.bucketUnavailable":
    "The object store service is temporarily unavailable. Try again later.",
  "sync.unreachable":
    "The object store refused the request. Nothing was saved. Check the bucket, access key, secret, region, and permissions.",
  "sync.bucketTimeout":
    "The object store did not respond in time. Check the network and endpoint.",
  "sync.bucketDNSFailed":
    "The endpoint hostname could not be resolved. Check the address and this device's DNS connection.",
  "sync.bucketTLSFailed":
    "The secure connection to the endpoint could not be verified. Check the HTTPS address and this device's clock.",
  "sync.bucketUnreachable":
    "The object store could not be reached. Check this device's network and the endpoint.",
  "sync.snapshotDownloadIncomplete":
    "The encrypted snapshot download ended early. Check the network and try receiving it again.",
  "sync.snapshotCostRefused":
    "This snapshot requires more decryption work than the safety limit allows.",
  "sync.snapshotSchemaUnsupported":
    "This snapshot uses a format unsupported by this version of sshc. Update to the same or a newer version than the machine that created it.",
  "sync.snapshotRejected":
    "The downloaded data is not a valid sshc snapshot or is damaged. Nothing was overwritten.",
  "sync.snapshotTooLarge":
    "The snapshot exceeds the safe read limit. Nothing was overwritten.",
  "sync.noSnapshot":
    "There is no current snapshot at the specified bucket and path.",
  "sync.internalFailed":
    "Sync encountered an unclassified internal error. Include the diagnostic code below when reporting it.",
  "sync.failed":
    "Sync could not be completed. Nothing was overwritten. Check the connection and try again.",
  "sync.localChanged":
    "Settings on this machine changed after the preview. Nothing was overwritten. Check for changes again.",
  "sync.workspaceBusy":
    "Another operation is updating settings on this machine. Try again after it finishes.",
  "sync.endpointPath":
    "The endpoint is the account address only — no bucket name and no path. Put the bucket name in the field below.",
  "sync.auto": "Automatic sync",
  "sync.autoHint.both":
    "While the Vault is unlocked, sshc checks the remote once a minute. After a local setting changes, it waits until five seconds pass without another change and pushes once. Conflicts and changes that remove files are not applied automatically; automatic sync stops and reports them.",
  "sync.autoHint.pull":
    "While the Vault is unlocked, sshc checks the remote once a minute. Changes on this machine are never pushed. Automatic receiving stops and reports conflicts or changes that would remove files.",
  "sync.autoHint.push":
    "While the Vault is unlocked, sshc checks whether the remote has changed. After a local setting changes, it waits until five seconds pass without another change and pushes once. Remote content is never applied to this machine.",
  "sync.autoEnable": "Keep this machine in sync automatically",
  "sync.autoIdle": "Stopped",
  "sync.autoLastRan": "Last checked {at}.",
  "sync.autoBlockedConflicts":
    "Automatic sync stopped because the same files changed on this machine and another machine. Select Check for changes to compare them.",
  "sync.autoBlockedRemovals":
    "Automatic sync stopped because applying the snapshot would remove files from this machine. Select Check for changes to review them.",
  "sync.autoBlockedRemoteMoved":
    "Automatic sync stopped before uploading because another machine changed the remote snapshot. Download those changes or explicitly replace the confirmed remote snapshot.",
  "sync.autoBlockedRemoteMovedPull":
    "The current remote has diverged from the last revision received by this machine. Automatic receiving stopped to prevent an unintended rollback.",
  "sync.remoteHeadReviewHint":
    "A receive-only machine can explicitly accept the current remote as authoritative. Review when it was created, its source, and the affected files before applying it.",
  "sync.remoteHeadReview": "Review current remote",
  "sync.checkRemoteChanges": "Review remote changes",
  "sync.remoteHeadPreviewHeading": "Receive current remote",
  "sync.remoteHeadPreview":
    "Reviewing the remote created at {at} by {origin}. Apply verifies the same exact generation again and writes nothing if it changed in the meantime.",
  "sync.remoteHeadApply": "Receive this remote",
  "sync.autoBlockedRemoteDeleted":
    "The previously synchronized live snapshot was deleted from the bucket. Automatic sync stopped so it will not silently recreate or overwrite it.",
  "sync.autoFailedLast":
    "The previous check could not reach the bucket. Automatic sync will try again.",
  "sync.autoFailedWrongKey":
    "The remote snapshot cannot be opened with this machine's synchronization key. The same generation will not be downloaded again automatically.",
  "sync.autoFailedSchema":
    "The remote snapshot uses an unsupported format. The same generation will not be downloaded again automatically.",
  "sync.autoFailed": "The setting could not be saved.",
  "sync.autoNow": "Sync now",
  "sync.autoNow.both": "Sync now",
  "sync.autoNow.pull": "Receive now",
  "sync.autoNow.push": "Send now",
  "sync.autoNowFailed": "The sync check could not be run.",
  "sync.transferHeading": "Send or review changes",
  "sync.transferHint.both":
    "Push this machine's workspace, or review the remote snapshot before applying it.",
  "sync.transferHint.push":
    "Push this machine's workspace. Remote content is not applied to this machine.",
  "sync.commitMessage": "Commit message",
  "sync.commitMessagePlaceholder": "Describe this snapshot",
  "sync.commitMessageHint":
    "A local-diff summary is generated automatically. Edit it before a manual push.",
  "sync.commitMessageChanges":
    "Local diff: {added} added · {modified} modified · {removed} removed",
  "sync.commitMessageInvalid":
    "Use a single-line commit message of 240 characters or fewer.",
  "sync.bucketStateHeading": "Bucket status",
  "sync.bucketStateHint":
    "Reads the live object and dated history directly from S3 without downloading their encrypted contents. Refreshes every 30 seconds while this screen is visible and after transfers.",
  "sync.bucketRefresh": "Refresh bucket status",
  "sync.bucketNotConfigured": "Configure a bucket to inspect it.",
  "sync.bucketLoading": "Reading the bucket…",
  "sync.bucketStatusFailed": "The bucket status could not be read.",
  "sync.bucketLive": "Current snapshot",
  "sync.bucketLiveEmpty": "The bucket has no current snapshot.",
  "sync.bucketLocalCurrent":
    "This machine has acknowledged the current remote generation.",
  "sync.bucketLocalBehind":
    "The remote generation differs from this machine's last acknowledged snapshot.",
  "sync.bucketHistory": "Dated history · {count}",
  "sync.bucketHistoryShowing": "Showing the latest {shown} of {count} entries.",
  "sync.bucketHistoryExpand": "Show all history",
  "sync.bucketHistoryCollapse": "Show only the latest 5",
  "sync.bucketObjectName": "Show S3 object name",
  "sync.bucketHistoryTruncated":
    "Showing the newest 10,000 entries. The bucket contains older history too.",
  "sync.bucketHistoryEmpty": "No dated history objects were found.",
  "sync.bucketObjectMeta": "{size} · updated {at}",
  "sync.bucketCheckedAt": "Bucket checked at {at}.",
  "sync.historyHeading": "Encrypted revision history",
  "sync.historyHint":
    "Decrypts a bounded recent window on this machine and shows it as a commit graph. File contents never enter the API response.",
  "sync.historyRefresh": "Refresh revision history",
  "sync.historyNeedsKey":
    "Set the shared encryption key to read revision history.",
  "sync.historyLoading": "Decrypting recent revisions…",
  "sync.historyFailed": "The encrypted revision history could not be read.",
  "sync.historySummary": "{count} revisions · downloaded {size}",
  "sync.historyTruncated":
    "Only the newest bounded history window is decoded. Older objects remain in the bucket list above.",
  "sync.historySkipped":
    "{count} history object(s) could not be opened with the current format or key and were skipped.",
  "sync.historyTimeline": "Revision timeline",
  "sync.historyRelation.head": "HEAD",
  "sync.historyRelation.ancestor": "ancestor",
  "sync.historyRelation.branch": "branch",
  "sync.historyRevisionMeta": "{at} · {count} files · device {origin}",
  "sync.historyParent": "parent {revision}",
  "sync.historySelect":
    "Select a revision to compare it with the current remote head.",
  "sync.historySelected": "Selected revision",
  "sync.historyDiffLoading": "Comparing paths…",
  "sync.historyDiffEmpty": "Select this revision again to compare paths.",
  "sync.historyDiffFailed": "The selected revision could not be compared.",
  "sync.historyDiff.added": "Added · {count}",
  "sync.historyDiff.modified": "Modified · {count}",
  "sync.historyDiff.removed": "Removed · {count}",
  "sync.historyRestoreHint":
    "Restore previews local file changes first. It does not rewind the remote head; the next push creates a new head from this revision.",
  "sync.historyRestorePreview": "Preview restoring this revision",
  "sync.forceHeading": "Replace the remote snapshot",
  "sync.forceHint":
    "Use this when changing to a bucket that already contains an unreadable or unwanted snapshot. The current workspace is encrypted and replaces only the exact remote generation shown above. If it changes after confirmation, the operation is cancelled. Existing dated history is not deleted.",
  "sync.forceConfirm":
    "I understand that the current remote snapshot will be replaced with this machine's workspace.",
  "sync.forcePush": "Replace the remote snapshot",
  "sync.forcePushShort": "Force send",
  "sync.forcePull": "Force receive",
  "sync.dialogClose": "Close",
  "sync.forcePushed": "Replaced the confirmed remote snapshot.",
  "sync.forceFailed":
    "The remote snapshot could not be replaced. Refresh the bucket status and confirm again.",
  "sync.push": "Push this workspace",
  "sync.pushed": "Pushed.",
  "sync.pushFailed":
    "The snapshot could not be uploaded. If another machine uploaded changes after this machine's last sync, download those changes first.",
  "sync.noLocalChanges": "There are no local changes to push.",
  "sync.remoteMoved":
    "Another machine changed the current snapshot, so the update was cancelled before retrying. Download the other machine's changes or explicitly replace the confirmed remote snapshot.",
  "sync.previewStale":
    "The remote snapshot changed after this preview. Preview it again before applying.",
  "sync.remoteDeleted":
    "The live remote snapshot was deleted before apply. Refresh the bucket status.",
  "sync.keyRecoveryRequired":
    "Synchronization key replacement was interrupted. Enter the same new synchronization key again to recover.",
  "sync.keyRecoveryTargetChange":
    "Finish the interrupted key replacement before changing the bucket or path.",
  "sync.keyHistoryLossConfirm":
    "I understand that older history snapshots will remain encrypted with the previous key and will no longer be readable.",
  "sync.preview": "Check for changes",
  "sync.pullFailed": "The snapshot could not be read.",
  "sync.alreadyMatches": "This workspace already matches the snapshot.",
  "sync.previewHeading": "What a pull would change",
  "sync.conflictExplain":
    "These files changed on this machine and another machine. The same configuration block cannot be merged automatically, so nothing was applied. Merge the files manually in the Configuration files screen, or upload from the machine whose version you want to keep.",
  "sync.conflictPermissions":
    "Permissions: last sync {base} · this machine {local} · remote {remote}",
  "sync.keepMine": "Keep this machine's version",
  "sync.takeTheirs": "Use the other machine's version",
  "sync.wouldWrite": "{count} files would be written:",
  "sync.wouldRemove": "{count} files would be removed:",
  "sync.confirmOverwrite":
    "This will overwrite files in ~/.ssh and remove the files listed above from this machine. Every affected file is first copied to ~/.ssh/sshc/backups/ and can be restored from History. Continue?",
  "sync.apply": "Apply the snapshot",
  "sync.applied": "Applied. The change is in History and can be rolled back.",
  "sync.applyFailed": "The snapshot could not be applied.",
  "sync.result.pushTitle": "This push",
  "sync.result.previewTitle": "Pull preview",
  "sync.result.applyTitle": "Apply result",
  "sync.result.previousTitle": "Previous success",
  "sync.result.filesSource": "{count} files · {size}",
  "sync.result.encrypted": "Encrypted snapshot {size}",
  "sync.result.uploaded":
    "S3 transfer {size} ({count} objects, history + live)",
  "sync.result.previewDownload":
    "Downloaded {downloaded} · {source} after opening",
  "sync.result.applyDownload": "Downloaded again for apply: {size}",
  "sync.result.changes":
    "{written} written · {removed} removed · {conflicts} conflicts",
  "sync.result.created": "Snapshot created {at}",
  "sync.result.snapshotAt": "Snapshot from {at}",
  "sync.result.appliedSnapshot": "Applied the snapshot from {at}",
  "sync.result.completed": "Operation completed {at}",
  "diag.heading": "Ad hoc checks",
  "diag.pageDescription":
    "Run SSH checks against a saved alias or any one-off host. Nothing is contacted until you choose a check.",
  "diag.configUnreadable": "The configuration could not be read.",
  "diag.running": "Running the requested check…",
  "diag.idle": "No check is running.",
  "diag.hostAlias": "Host alias",
  "diag.needsAlias": "Type a host alias to run a check.",
  "diag.explain": "Explain",
  "diag.explainFailed": "The alias could not be explained.",
  "diag.checkReachability": "Check reachability",
  "diag.reachabilityFailed": "The reachability check could not be run.",
  "diag.testAuthentication": "Test authentication",
  "diag.authenticationFailed": "The authentication test could not be run.",
  "diag.configuration": "Configuration",
  "diag.missingSuffix": " (missing)",
  "diag.canRunCommand": "This configuration can run a command",
  "diag.directiveAt": "{keyword} at {path}:{line}",
  "diag.sourcesCaption":
    "Source of each value. A line marked “superseded” was read after the effective value and had no effect.",
  "diag.tableScrollHint": "Swipe sideways to see every column",
  "diag.columnKeyword": "Keyword",
  "diag.columnValue": "Value",
  "diag.columnWhere": "Read from",
  "diag.columnCondition": "Under",
  "diag.columnState": "State",
  "diag.inEffect": "in effect",
  "diag.superseded": "superseded",
  "diag.route": "Connection route",
  "diag.hopComplex":
    "this hop is not a simple alias, so its destination is not resolved here",
  "diag.reachedThrough": "reached through {parent}",
  "diag.notSimple": "These rules cannot be resolved in the simplified view",
  "diag.notSimpleDetail":
    "sshc shows the source of each value but cannot resolve the final result. Use `ssh -G` for the authoritative values.",
  "diag.inside": "inside {condition}",
  "diag.reachability": "Reachability",
  "diag.authentication": "Authentication",
  "diag.authenticationMethod": "Authenticated with {method}.",
  "diag.forHost": "Diagnostics for {host}",

  "kh.heading": "Known Hosts",
  "kh.pageDescription":
    "Review trusted host keys, scan a destination and verify its fingerprint before adding it to known_hosts.",
  "kh.metricEntries": "Trusted entries",
  "kh.metricHashed": "Hashed hosts",
  "kh.metricCandidates": "Scan candidates",
  "kh.scanHeading": "Scan a host",
  "kh.trustedHeading": "Trusted host keys",
  "kh.columnHost": "Host",
  "kh.columnType": "Key type",
  "kh.columnFingerprint": "Fingerprint",
  "kh.columnTrust": "Trust",
  "kh.columnActions": "Actions",
  "kh.unreadable": "The known_hosts file could not be read.",
  "kh.removeFailed": "The entry could not be removed. Nothing was changed.",
  "kh.scanFailed": "The host could not be scanned.",
  "kh.addFailed": "The key could not be added. Nothing was changed.",
  "kh.addFailedCode":
    "The key could not be added ({code}). Nothing was changed.",
  "kh.removed": "Removed one entry in transaction {id}.",
  "kh.added": "Added {host} in transaction {id}.",
  "kh.search": "Search",
  "kh.hashed": "(hashed)",
  "kh.delete": "Delete",
  "kh.confirmRemove":
    "Remove line {line} ({fingerprint})? The operation is recorded in History and a backup is kept.",
  "kh.confirmDelete": "Confirm delete",
  "kh.cancel": "Cancel",
  "kh.hostToScan": "Host to scan",
  "kh.scan": "Scan",
  "kh.scanCandidates": "Scan candidates",
  "kh.unverified": "unverified",
  "kh.add": "Add",
  "kh.addHeading": "Add an unverified host key",
  "kh.addExplain":
    "This key was presented when connecting to {host}, but another system reachable at that address could have presented it. Enter a fingerprint obtained through another trusted channel, or accept the risk of trusting an unverified key.",
  "kh.expectedFingerprint": "Fingerprint verified through another channel",
  "kh.acknowledge":
    "I could not verify this key and I accept the risk of trusting it",
  "kh.addToKnownHosts": "Add to known_hosts",
  "kh.fingerprintMismatch":
    "The fingerprint you typed does not match this key. You typed {typed}; the scan returned {scanned}. Nothing was added.",

  "rk.heading": "Install Key on Server",
  "rk.pageDescription":
    "Review the exact authorized_keys change before installing one of your public keys on a remote account.",
  "rk.waiting": "Waiting for the server…",
  "rk.idle": "Nothing is sent to the remote host until you confirm it.",
  "rk.added": "The key was added to the remote authorized_keys file.",
  "rk.alreadyPresent":
    "The key was already present; the remote file was left as it was.",
  "rk.valuesFromEngine": "sshc reading your configuration; ssh was not run",
  "rk.valuesFromSshG": "ssh -G, which OpenSSH itself resolved",
  "rk.pickFromSsh": "Public key from ~/.ssh",
  "rk.typeInstead": "Type one below instead",
  "rk.hostAlias": "Host alias",
  "rk.hostSearch": "Connections",
  "rk.hostSearchPlaceholder": "Search hosts or type an alias",
  "rk.hostTypeHint":
    "Type a Host alias above. Saved connections will appear here when available.",
  "rk.hostsSelected": "{count} selected",
  "rk.hostChoices": "Connection choices",
  "rk.selectMatches": "Select matches",
  "rk.clearSelection": "Clear",
  "rk.noHostMatches": "No matching connections",
  "rk.chooseHost": "Select at least one connection or type a Host alias.",
  "rk.publicKeyFile": "Public key file",
  "rk.publicKeyLine": "Public key line",
  "rk.showWhatWouldHappen": "Show what this would do",
  "rk.register": "Register the key",
  "rk.registerMany": "Register on {count} hosts",
  "rk.plannedHosts": "Reviewed {count} hosts",
  "rk.planFailed": "The change could not be described. Nothing was contacted.",
  "rk.registerFailed":
    "The key was not registered. The remote host was left as it was.",
  "rk.publicKeyUnreadable":
    "The selected public key could not be read. No connection was attempted.",
  "rk.withCode": "{message} ({code})",
  "rk.confirmHeading": "Confirm remote registration",
  "rk.confirmManyHeading": "Confirm registration on {count} hosts",
  "rk.planFor": "Registration plan for {alias}",
  "rk.alias": "Alias",
  "rk.effectiveUser": "Connection user",
  "rk.noUser": "not set in your configuration; ssh will use your local account",
  "rk.destination": "Destination",
  "rk.valuesCameFrom": "These values came from",
  "rk.keyFile": "Public key file",
  "rk.fingerprint": "Fingerprint",
  "rk.appendTo":
    "Add one line to {remotePath} for {account} on {hostname}, but only if the same line is not already present.",
  "rk.theRemoteAccount": "the remote account",
  "rk.usersAccount": "{user}’s account",
  "rk.keyLineLabel": "Public key line to append",
  "rk.remoteRuns":
    "The remote host runs this command with the key provided on standard input:",
  "rk.remoteCommandLabel": "Remote command",
  "rk.connectingRuns": "Connecting to this host runs a command",
  "rk.acknowledgeRuns":
    "I have read this command and accept that connecting runs it",
  "rk.manualHeading":
    "sshc cannot register a key on this host. Complete these steps manually:",
  "rk.result": "Result",
  "rk.someRegistrationsFailed":
    "Some hosts could not be updated. Review the result for each host.",

  "explorer.loading": "Loading configuration files…",
  "explorer.pageTitle": "Configuration files",
  "explorer.pageDescription":
    "Follow the Include graph, edit exact file contents and manage workspace files without losing OpenSSH formatting.",
  "explorer.metricFiles": "Loaded files",
  "explorer.metricEditable": "Editable files",
  "explorer.metricDiagnostics": "Diagnostics",
  "explorer.hierarchy": "Include hierarchy",
  "explorer.externalFile":
    "This file is outside ~/.ssh. It is read and shown, never written.",
  "explorer.insideCondition": "inside {condition}",
  "explorer.fileState": "{missing}{loads}{editable}",
  "explorer.missing": "missing · ",
  "explorer.readTimes": "read {count} times · ",
  "explorer.editable": "editable",
  "explorer.readOnly": "read only",
  "explorer.newFilePath": "New file path",
  "explorer.workspaceActions": "Workspace files",
  "explorer.directoryHelp": "About files and directories",
  "explorer.createFile": "Create file",
  "explorer.createDirectory": "Create directory",
  "explorer.deleteDirectory": "Delete directory",
  "explorer.directoryNote":
    "You can create and delete directories here, but only empty directories can be deleted. Delete their files first; Include lines that directly reference those files are updated at the same time. Manage declared groups on the Groups screen.",
  "explorer.fileOperations": "This file",
  "explorer.fileOperationsNote":
    "Renaming this file also updates Include lines that directly reference it. Wildcard patterns are not changed; a warning appears if the new path no longer matches.",
  "explorer.renameTo": "New path",
  "explorer.renameFile": "Rename file",
  "explorer.deleteFile": "Delete file",
  "explorer.confirmDelete": "Delete it",
  "explorer.cancelDelete": "Keep it",
  "explorer.deleteIsRecoverable":
    "Deleting this file also removes Include lines that directly reference it. A backup is kept so the file can be restored from History.",
  "explorer.saveOrDiscardFirst":
    "There are unsaved edits. Save them or reopen the file before renaming or deleting it.",
  "explorer.newFileNote":
    "To make OpenSSH read a new file, reference it from an Include in ~/.ssh/config. Move connections between groups on the Connections screen. Renaming and deleting arbitrary files and directories is not yet supported because those directory operations cannot currently be recorded in History.",
  "explorer.diagnostics": "Diagnostics",
  "explorer.noIncludeProblem": "No Include problem detected.",
  "explorer.opened": "Opened {path} at line {line}.",
  "explorer.selectFile": "Select a file to edit its full text.",
  "explorer.emptyHeading": "Select a configuration file",
  "explorer.unsaved": "Unsaved changes",
  "explorer.fileText":
    "File text — {path}. The content is written back exactly as entered.",
  "explorer.preview": "Preview",
  "explorer.saveFile": "Save file",

  "groups.loading": "Loading groups…",
  "groups.pageTitle": "Groups",
  "groups.pageDescription":
    "Organise connections and keys into nested groups, then apply shared SSH settings to each group.",
  "groups.metricGroups": "Groups",
  "groups.metricConnections": "Connections",
  "groups.metricDraft": "Unsaved drafts",
  "groups.empty":
    "No group exists yet. Create one below to organise related connections and keys.",
  "groups.nameTaken":
    "A group with this name already exists. Enter a different name.",
  "groups.chooseGroupAndKeyword": "Choose a group and a directive keyword.",
  "groups.unbalancedQuote":
    "A value has an unbalanced quote. OpenSSH has no escape inside quotes, so this cannot be saved.",
  "groups.renameNeedsName": "Enter a new name for the group.",
  "groups.renameCollides":
    "{name} already exists. Rename it to something else, or remove one of the two.",
  "groups.compileNote":
    "Groups compile into ordinary Host blocks in {file}, with child groups written before their parents so OpenSSH keeps the most specific value it reads first.",
  "groups.members": "Members:",
  "groups.noMembers": "none",
  "groups.colour": "Colour",
  "groups.clearColour": "Clear {name} colour",
  "groups.renameTo": "Rename {name} to",
  "groups.renameShort": "New name",
  "groups.rename": "Rename {name}",
  "groups.displayOrder": "Display order",
  "groups.hide": "Hide {name} from Connections",
  "groups.hideOnlyContainers":
    "This group contains direct connections, which would also be hidden. Move them into a child group first.",
  "groups.remove": "Remove {name}",
  "groups.newName": "New group name",
  "groups.invalidName":
    "A group name is a relative directory path: letters, digits, dot, dash and underscore, slash-separated, at most six levels deep.",
  "groups.directoryNote":
    "Each group is a directory. Connections are stored in {connections}/<group>/ and keys in {keys}/<group>/. sshc adds one Include line per group to ~/.ssh/config, making the read order and setting precedence explicit instead of relying on a wildcard.",
  "groups.howItWorks": "How groups map to SSH files",
  "groups.directories": "{connections}/ · {keys}/",
  "groups.nestingNote": "Use a slash to nest: work/eu is a group inside work.",
  "groups.addChild": "Add a group inside {name}",
  "groups.listLabel": "Groups, parent before child",
  "groups.orderNote":
    "Listed as a tree, parent first. The Include lines are written in the opposite order — deepest group first — because OpenSSH keeps the first value it reads, so a child's setting has to be read before its parent's.",
  "groups.removeInto": "Remove {name}",
  "groups.removeIntoShort": "Move its connections to",
  "groups.removeIntoNone": "No group (connections/ itself)",
  "groups.removeExplain":
    "Removing {name} takes away its Include line and its group settings. Its {count} connections have to go somewhere:",
  "groups.removeExplainEmpty":
    "Removing {name} deletes its Include line and group settings. The group contains no connections, so no connection files are moved.",
  "groups.removeKeepsFiles":
    "No configuration file is deleted. The change is one transaction, so History can undo it.",
  "groups.removeConfirm": "Remove {name}",
  "groups.removeCancel": "Keep it",
  "groups.unsaved": "Not saved",
  "groups.unsavedBarLabel": "Unsaved group changes",
  "groups.unsavedBarNote":
    "Group additions and display settings are not saved yet. Save or discard them before renaming or removing a group.",
  "groups.discard": "Discard group changes",
  "groups.saveDraftFirst":
    "Save or discard the pending group changes before renaming or removing a group.",
  "groups.savedNote":
    "There are no unsaved changes. Colour, display order, new groups, and new settings are applied with Save groups. Rename and Remove write to disk immediately.",
  "groups.immediateActions": "Rename and remove write to disk immediately.",
  "groups.newGroupNote":
    "This group does not have a directory yet. Save groups creates it; Rename and Remove become available afterwards.",
  "groups.addHeading": "Add a group",
  "groups.add": "Add group",
  "groups.settingHeadingFor": "Add a setting to {name}",
  "groups.directive": "Directive",
  "groups.value": "Value",
  "groups.addSetting": "Add setting",
  "groups.previewChanges": "Preview group changes",
  "groups.save": "Save groups",

  "tree.navLabel": "Connections",
  "tree.ungrouped": "Ungrouped",
  "tree.arrangeBy": "Arrange connections by",
  "tree.byGroups": "Groups",
  "tree.byFiles": "Files",
  "tree.groupFilter": "Filter by group",
  "tree.groupSection": "{name} group, {count} connections",
  "tree.filter": "Filter connections",
  "tree.filterPlaceholder": "alias, pattern, group or tag",
  "tree.filterPlaceholderExpanded":
    "Search name, destination, user, group or tag",
  "tree.allConnections": "All",
  "tree.resultsLabel": "Connection results",
  "tree.resultCount": "{visible} of {total}",
  "tree.sortLabel": "Connection order",
  "tree.sortConfigured": "Configured order",
  "tree.sortName": "Name",
  "tree.sortGroup": "Group",
  "tree.noMatch": "No connection matches this filter.",
  "tree.groupEmpty": "No connection is in this group.",
  "tree.collapse": "Collapse {name}",
  "tree.expand": "Expand {name}",
  "tree.patternRuleExternal":
    "Pattern rule in {path}, a file this editor only reads.",
  "tree.patternRuleOpen":
    "Pattern rule — open it in the Config file view ({path}:{line})",
  "tree.duplicateAlias": "duplicate alias",
  "tree.patternRule": "pattern rule",
  "tree.dragGroupHint": "Drag a group to nest or reorder it.",

  "browser.modeLabel": "Browse connections by",
  "browser.servers": "Servers",
  "browser.groups": "Groups",
  "browser.groupPath": "Group path",
  "browser.ungrouped": "Ungrouped",
  "browser.groupCountOne": "1 server",
  "browser.groupCountMany": "{count} servers",
  "browser.noMatches": "No servers match the current filters.",
  "browser.emptyGroup": "No servers are directly in this group.",
  "browser.emptyGroups": "No groups are declared yet.",
  "browser.groupMissing": "Group not found.",
  "browser.backToGroupRoot": "Back to group root",
  "browser.invalidUrl": "This connection URL is not recognised.",
  "browser.backToServers": "Back to servers",
  "browser.duplicateAlias": "duplicate alias",

  "conn.loading": "Loading connections…",
  "conn.heading": "Connections",
  "conn.count": "{count} available",
  "conn.new": "New connection",
  "conn.allConnections": "All connections",
  "conn.createAnother": "Create another connection",
  "conn.cancelCreate": "Cancel",
  "conn.create": "Create connection",
  "conn.createTitle": "Create connection",
  "conn.createDescription":
    "Save an SSH destination and choose its authentication method.",
  "conn.createConnectionSection": "Connection",
  "conn.createName": "Connection name",
  "conn.createNameRequired": "Connection name (required)",
  "conn.createGroup": "Save in group",
  "conn.createManageGroups": "Manage groups",
  "conn.createNoGroup": "No group",
  "conn.createHostName": "Host name or IP address",
  "conn.createHostNameRequired": "Host name or IP address (required)",
  "conn.createUser": "User (optional)",
  "conn.createPort": "Port (optional)",
  "conn.createPortHint": "Defaults to 22.",
  "conn.createAuthenticationSection": "Authentication",
  "conn.createAuthenticationMethod": "Authentication method",
  "conn.createDedicatedPassword": "Encrypted password for this connection",
  "conn.createSavedPassword": "Saved password",
  "conn.createNewSharedPassword": "New saved password",
  "conn.createIdentityFile": "SSH private key",
  "conn.createConnectionPassword": "Connection password",
  "conn.createDedicatedHint":
    "Encrypted in the vault and not listed as a reusable password.",
  "conn.createChooseSavedPassword": "Saved password",
  "conn.createSavedHint":
    "This connection will share the selected reusable password.",
  "conn.createNoSavedPasswords": "No saved passwords",
  "conn.createSavedPasswordName": "Saved password name",
  "conn.createNewPassword": "New password",
  "conn.createPrivateKey": "SSH private key",
  "conn.createNoPrivateKeys": "No private keys available",
  "conn.createNoPrivateKeysHint":
    "Create a private key first if you do not want to use a password.",
  "conn.createCreatePrivateKey": "Create a private key",
  "conn.createLoadingOptions": "Loading authentication options…",
  "conn.createOptionsFailed": "Authentication options could not be loaded.",
  "conn.createMasterPassword": "Master password",
  "conn.createConfirmMaster": "Confirm master password",
  "conn.createInitialiseVault": "Create encrypted vault",
  "conn.createUnlockVault": "Unlock vault",
  "conn.createVaultMissing":
    "Create the encrypted vault before saving this connection.",
  "conn.createVaultLocked":
    "Unlock the encrypted vault before saving this connection.",
  "conn.createVaultFailed": "The encrypted vault could not be created.",
  "conn.createUnlockFailed": "The encrypted vault could not be unlocked.",
  "conn.createNeedVault": "Unlock the encrypted vault to continue.",
  "conn.createNeedConnectionPassword":
    "Enter a connection password to continue.",
  "conn.createNeedSavedPassword": "Choose a saved password to continue.",
  "conn.createNeedSavedPasswordName":
    "Enter a saved password name to continue.",
  "conn.createNeedNewPassword": "Enter the new password to continue.",
  "conn.createNeedPrivateKey": "Choose or create a private key to continue.",
  "conn.createDraftWaiting": "Connection setup for {alias} is paused.",
  "conn.createUntitledDraft": "Untitled connection",
  "conn.createReturnToDraft": "Return to connection setup",
  "conn.createAliasRequired": "Enter a connection name.",
  "conn.createAliasInvalid":
    "Use letters, numbers, dot, dash, or underscore; start with a letter or number.",
  "conn.createHostRequired": "Enter a host name or IP address.",
  "conn.createHostInvalid":
    "Enter a DNS name, IPv4 address, or unbracketed IPv6 address.",
  "conn.createUserInvalid":
    "User cannot contain whitespace or control characters.",
  "conn.createPortInvalid": "Port must be a whole number from 1 to 65535.",
  "conn.creating": "Creating…",
  "conn.createFailed": "The connection could not be created.",
  "conn.createAliasTaken": "Another connection already uses that name.",
  "conn.duplicateAliasTaken":
    "{alias} already exists, so this connection cannot be duplicated.",
  "conn.createGroupMissing":
    "The selected group is no longer declared. Reload and choose another group.",
  "conn.createKeyInvalid":
    "The selected private key is no longer available. Reload and choose another key.",
  "conn.createCredentialMissing":
    "The selected saved password is no longer available. Reload and choose another one.",
  "conn.createDestinationExists":
    "A connection file with that name already exists in this group.",
  "conn.basicConnection": "Connection",
  "conn.basicAuthentication": "Authentication",
  "conn.basicHostName": "Host name or IP address",
  "conn.basicServerKeyInvalid":
    "The selected SSH private key is no longer available. Reload and choose it again.",
  "conn.basicCredentialExists":
    "A saved password with this name already exists. Choose it under Saved password or use another name.",
  "conn.basicCredentialMissing":
    "The selected saved password no longer exists. Reload and choose another one.",
  "conn.basicPasswordMissing":
    "This connection no longer has a stored password to remove.",
  "conn.basicUser": "User",
  "conn.basicPort": "Port",
  "conn.basicPrivateKey": "SSH private key",
  "conn.basicStoredPassword": "Stored password",
  "conn.basicThisConnection": "Set on this connection.",
  "conn.basicInheritedFrom": "Inherited from {path}:{line}.",
  "conn.basicSSHDefault":
    "SSH default. Saving another field will not write this value.",
  "conn.basicReadOnlyAdvanced":
    "Read-only in Basic; the original directives remain available in Advanced.",
  "conn.basicComplex":
    "This connection has multiple direct {keyword} values. Resolve them in Advanced.",
  "conn.basicUseInheritedHost": "Use inherited/default host name",
  "conn.basicUseInheritedUser": "Use inherited/default user",
  "conn.basicUseInheritedPort": "Use inherited/default port",
  "conn.basicKeepDirect": "Keep this connection value",
  "conn.basicAgentOrInherited": "SSH agent or inherited keys",
  "conn.basicManageKeyPassphrase": "Save or change key passphrase",
  "conn.basicKeyPassphraseHeading": "Saved key passphrase",
  "conn.basicKeyPassphraseUnencrypted":
    "This private key is not encrypted, so it needs no saved passphrase.",
  "conn.basicKeyPassphraseNone": "No passphrase is saved for this key.",
  "conn.basicKeyPassphraseDedicated":
    "A passphrase is saved only for this key.",
  "conn.basicKeyPassphraseShared":
    "This key uses the shared saved passphrase “{name}”.",
  "conn.basicKeyPassphraseSharedOthers":
    "It is also used by {count} other key(s).",
  "conn.basicKeyPassphraseDetach":
    "Saving here creates a passphrase dedicated to this key. The shared credential and its other assignments remain unchanged.",
  "conn.basicNewKeyPassphrase": "New saved key passphrase",
  "conn.basicConfirmKeyPassphrase": "Confirm saved key passphrase",
  "conn.basicKeyPassphraseMismatch": "The key passphrases do not match.",
  "conn.basicKeyPassphraseStoredNote":
    "This saves an unlock value; it does not change the passphrase that encrypts the private-key file.",
  "conn.basicKeyPassphraseWrong":
    "The entered passphrase does not unlock the selected private key.",
  "conn.basicKeyPassphraseChanged":
    "The selected private key changed. Reload before saving its passphrase.",
  "conn.basicGeneratedKeyStaged":
    "{path} is staged for this connection. Choose Save Basic settings to apply it.",
  "conn.basicCustomKey":
    "This connection uses the custom IdentityFile path {path}. Edit it in Advanced.",
  "conn.basicComplexKey":
    "This connection has multiple direct IdentityFile values. Resolve them in Advanced.",
  "conn.basicAssignedDedicated":
    "A connection-only password is assigned. Its value is never displayed.",
  "conn.basicAssignedNamed": "Assigned: {name}",
  "conn.basicNoPassword": "No stored password is assigned.",
  "conn.basicPasswordCleanup":
    "A stored password is still assigned, but sshc will not use it. Saving Basic settings will remove this connection's assignment; a shared credential and its other hosts remain unchanged.",
  "conn.basicPasswordAction": "Stored password action",
  "conn.basicPasswordUnchanged": "No password change",
  "conn.basicReplaceDedicated": "Replace with a connection-only password",
  "conn.basicRemovePassword": "Remove stored password",
  "conn.basicConfirmRemove": "Confirm stored password removal",
  "conn.basicEmptyPasswordUnchanged":
    "Leaving this empty makes no password change.",
  "conn.basicVaultMissing":
    "Create the encrypted vault before saving Basic settings.",
  "conn.basicVaultLocked":
    "Unlock the encrypted vault before saving Basic settings.",
  "conn.basicNeedVault": "Unlock the encrypted vault to save this draft.",
  "conn.basicPasswordBlocked":
    "The current SSH settings block adding or replacing a stored password.",
  "conn.basicNothingChanged": "No Basic setting has changed.",
  "conn.basicOptionsFailed": "Keys and password options could not be loaded.",
  "conn.basicCredentialOptionsFailed":
    "Saved password options could not be loaded.",
  "conn.basicSaveFailed":
    "Basic settings could not be saved. Nothing was changed; review the error or reload and try again.",
  "conn.basicRefreshFailed":
    "The settings were saved, but their updated password status could not be loaded. Reload this connection.",
  "conn.basicConnectionRefreshFailed":
    "The settings were saved, but the updated connection could not be loaded. Reload this connection.",
  "conn.basicSave": "Save Basic settings",
  "conn.basicSaving": "Saving…",
  "conn.discardChanges": "Discard changes",
  "conn.blockMoved":
    "This block moved on disk. Reload the connection and try again.",
  "conn.emptyHeading": "Choose a connection",
  "conn.emptyHint":
    "Select a host from the list to edit its SSH settings, or create a new connection.",
  "conn.assignKeyHeading": "Choose a connection for this key",
  "conn.assignKeyHint":
    "Select a connection to stage {path} in its Basic settings. Nothing changes until you save.",
  "conn.missingHeading": "This connection is no longer available",
  "conn.missingHint":
    "It may have been renamed, moved or deleted since this link was created.",
  "conn.backToList": "Back to connections",
  "conn.summarySaved": "Saved connection",
  "conn.summarySavedState": "Saved",
  "conn.summaryUnsaved": "Unsaved changes",
  "conn.summaryGroup": "Group",
  "conn.summaryNoGroup": "No group",
  "conn.summaryPrivateKey": "SSH private key",
  "conn.summaryKeyNone": "SSH agent or inherited keys",
  "conn.summaryKeyComplex": "Multiple IdentityFile directives",
  "conn.summaryKeyUnavailable": "{path} — key details unavailable",
  "conn.summaryKeyPassphrase": "Key passphrase",
  "conn.summaryKeyPassphraseNone": "No passphrase saved",
  "conn.summaryKeyPassphraseDedicated": "Saved only for this key",
  "conn.summaryKeyPassphraseNamed": "Saved passphrase: {name}",
  "conn.summaryKeyPassphraseNotNeeded": "Not needed for this unencrypted key",
  "conn.summaryAccountPassword": "Account password",
  "conn.summaryPasswordNone": "No saved password",
  "conn.summaryPasswordDedicated": "Connection-only password saved",
  "conn.summaryPasswordNamed": "Saved password: {name}",
  "conn.summaryPasswordCleanup":
    "A stored password is assigned but is not used and will be unassigned when Basic settings are saved.",
  "conn.summaryLocked": "Unavailable while the vault is locked",
  "conn.summaryUnavailable": "Could not load this status",
  "conn.summaryDraftBlocksActions":
    "Save or discard this draft before using the saved connection.",
  "conn.summaryRefreshing":
    "Reloading the saved connection. Actions will be available when it finishes.",
  "conn.editorLabel": "Connection editor",
  "conn.areaBasic": "Basic",
  "conn.areaAnalysis": "Analysis",
  "conn.areaAdvanced": "Advanced",
  "conn.areaSshc": "sshc",
  "conn.checksLabel": "Connection checks",
  "conn.checkReachability": "Check reachability",
  "conn.checkAuthentication": "Check authentication with saved settings",
  "conn.checking": "Checking…",
  "conn.checksExecutableHeading": "Authentication may execute SSH directives",
  "conn.checksExecutableHint":
    "Review the exact saved directives before allowing the authentication check to continue.",
  "conn.checksDirectiveAt": "{keyword} at {path}:{line}",
  "conn.checksAcknowledge": "Acknowledge and check authentication",
  "conn.analysisLabel": "Settings analysis",
  "conn.analysisExplained": "Resolved saved values",
  "conn.analysisExplainedHint":
    "These are the values used by this connection. They are resolved without executing commands. Settings that cannot be resolved are identified explicitly.",
  "conn.analysisAuthoritative": "Where each value comes from",
  "conn.analysisAuthoritativeHint":
    "Lists every line read for the saved connection, the file containing it, and whether the line takes effect.",
  "conn.analysisRun": "Show the sources",
  "conn.analysisRunning": "Reading…",
  "conn.analysisExecutableHeading": "ssh -G may execute Match directives",
  "conn.analysisSources": "Configuration lines read by OpenSSH",
  "conn.advancedLabel": "Advanced settings",
  "conn.advancedViews": "Advanced setting views",
  "conn.advancedViewLabel": "View",
  "conn.advancedDirectives": "Directives",
  "conn.portForwarding": "Port forwarding",
  "conn.forwardLoopbackOnly":
    "Listeners are bound to this device only (127.0.0.1), but other OS accounts on the same device may be able to use them. Remote forwarding is not supported.",
  "conn.forwardNoneSaved": "No port forwarding is saved for this connection.",
  "conn.forwardLocal": "Local tunnel",
  "conn.forwardDynamic": "SOCKS proxy",
  "conn.forwardType": "Type",
  "conn.forwardListenPort": "Local port",
  "conn.forwardDestination": "Destination",
  "conn.forwardDestinationHint":
    "Enter the host name or IP address and port as seen from the SSH server.",
  "conn.forwardDynamicHint":
    "The application using this SOCKS proxy chooses the destination for each connection.",
  "conn.forwardAdd": "Add forwarding",
  "conn.forwardPendingSave":
    "This forwarding will be written when you save the changes.",
  "conn.forwardInvalidPort": "Enter a local port from 1 to 65535.",
  "conn.forwardInvalidDestination": "Enter the destination as host:port.",
  "conn.advancedNoFields": "This connection has no settings in this view.",
  "conn.advancedRawBlocksFields":
    "Raw has unsaved changes. Discard or save it before editing directives.",
  "conn.advancedFieldsBlockRaw":
    "Directives have unsaved changes. Discard or save them before editing Raw.",
  "conn.connect": "Connect",
  "conn.opening": "Opening…",
  "conn.duplicate": "Duplicate connection",
  "conn.manage": "More connection actions",
  "conn.manageLabel": "Manage connection",
  "conn.manageIndependent":
    "Each action is saved independently of Basic and Advanced settings.",
  "conn.manageDraftBlocked":
    "Save or discard the editor draft before changing connection identity or storage.",
  "conn.discardPrompt":
    "Discard the unsaved connection changes and leave this connection?",
  "conn.reloadConnection": "Reload saved connection",
  "conn.moveToFile": "Storage file",
  "conn.moveToFilePlaceholder": "Choose a storage file…",
  "conn.move": "Change storage file",
  "conn.storageFileNote":
    "Primary group controls where sshc organises the connection. Storage file is an advanced override that selects the exact SSH configuration file.",
  "conn.confirmDelete": "Delete it",
  "conn.deleteHeading": "Delete {alias}?",
  "conn.deleteBody":
    "This removes the Host block from your configuration. You can restore it from History.",
  "conn.deleteCancel": "Keep it",
  "conn.delete": "Delete connection",

  "host.tabJump": "Jump Host",
  "host.tabRaw": "Raw",
  "host.unbalancedQuote":
    "A value has an unbalanced quote. OpenSSH has no escape inside quotes, so this cannot be saved.",
  "host.needsKeyword": "A directive needs a keyword.",
  "host.dangerousField":
    "{keyword} can run a command when OpenSSH evaluates this host. It is stored as written and never executed here.",
  "host.keep": "Keep",
  "host.remove": "Remove",
  "host.newDirective": "New directive",
  "host.newValue": "New value",
  "host.addDirective": "Add directive",
  "host.saveChanges": "Save changes",
  "host.blockText":
    "Block text. Comments, blank lines and unknown directives are written back exactly as typed.",
  "host.saveBlock": "Save block",
  "host.noDestination":
    "This block matches by pattern and names no destination of its own, so there is nothing to diagnose. Open a connection with a concrete alias instead.",
  "host.primaryGroup": "Primary group",
  "host.groupNone": "None",
  "host.groupNoneMeans":
    "Choosing none moves the connection back into ~/.ssh/config, at the end of the file.",
  "host.moveToGroup": "Move to this group",
  "host.comment": "Comment",
  "host.commentNote":
    "Written into the configuration file above the Host line, so it is there for anyone reading the file without sshc.",
  "host.commentFromNote":
    "This started as a note stored only by sshc. Saving it writes it into the configuration file and retires the note.",
  "host.saveComment": "Save comment",
  "host.colour": "Colour",
  "host.clearColour": "Clear colour",
  "host.displayOrder":
    "Display order — lower sorts earlier; 0 leaves this host where the file puts it",
  "host.tags": "Tags, comma separated",
  "host.renameAlias": "Rename alias",
  "host.rename": "Rename",

  "keys.heading": "Keys",
  "keys.pageDescription":
    "Review private keys, public keys, certificates, and their references, then manage creation, updates, and deletion.",
  "keys.search": "Search keys",
  "keys.searchPlaceholder": "file, host or fingerprint",
  "keys.metricFiles": "Classified SSH files",
  "keys.metricPrivate": "Private keys",
  "keys.metricAttention": "Needs attention",
  "keys.noMatches": "No key matches this search.",
  "keys.reading": "Reading the ssh directory…",
  "keys.unreadable":
    "The ssh directory could not be read. Restart sshc and try again.",
  "keys.createFailed":
    "The key could not be created. Check the name, the algorithm and the passphrase.",
  "keys.passphraseFailed":
    "The passphrase could not be changed. Check the current passphrase and try again.",
  "keys.agentFailed":
    "The key could not be added to ssh-agent. Check the passphrase and confirm that this process can connect to a running ssh-agent.",
  "keys.publicKeyFailed": "The public key could not be read.",
  "keys.trashFailed": "The key could not be moved to the trash.",
  "keys.restoreFailed": "The entry could not be restored.",
  "keys.restoreRefused": "The entry cannot be restored: {blockers}",
  "keys.purgeFailed": "The entry could not be deleted permanently.",
  "keys.tableCaption": "Files classified by content and permissions",
  "keys.inventoryEmpty": "Nothing under ~/.ssh yet. Create a key below.",
  "keys.colFile": "File",
  "keys.colKind": "Kind",
  "keys.kind.privateKey": "Private key",
  "keys.kind.publicKey": "Public key",
  "keys.kind.certificate": "Certificate",
  "keys.kind.other": "Other file",
  "keys.kind.unreadable": "Unreadable file",
  "keys.colAlgorithm": "Algorithm",
  "keys.colFingerprint": "Fingerprint",
  "keys.colPermissions": "Permissions",
  "keys.colUsedBy": "Used by",
  "keys.colState": "State",
  "keys.stateInAgent": "registered with ssh-agent",
  "keys.stateUsedBy": "referenced by {count}",
  "keys.usedByNothing": "No connection references this key",
  "keys.inspectorLabel": "Key details",
  "keys.colActions": "Actions",
  "keys.showDetails": "Show details",
  "keys.hideDetails": "Hide details",
  "keys.actionsHeading": "Actions for {path}",
  "keys.keyActions": "Key actions",
  "keys.permissionRisk": "Permissions too open",
  "keys.showPublicKey": "Show public key",
  "keys.relatedPublicFiles": "Public key files ({count})",
  "keys.showPrivateKey": "Show private key",
  "keys.changePassphrase": "Change passphrase",
  "keys.moreActions": "More actions",
  "keys.manageStoredPassphrase": "Save passphrase",
  "keys.storedPassphraseHeading": "Saved passphrase: {path}",
  "keys.storedPassphraseNote":
    "Save this encrypted key's passphrase in the sshc vault, or assign an existing named passphrase. Values are encrypted with the master password and are not displayed here after saving.",
  "keys.newStoredPassphraseName": "Passphrase name",
  "keys.newStoredPassphraseValue": "Passphrase value",
  "keys.storeAndUsePassphrase": "Save and use for this key",
  "keys.storedPassphraseExists":
    "A passphrase with this name already exists. Select it above or choose a new name.",
  "keys.unassignPassphrase": "Stop using it",
  "keys.storePassphraseFailed":
    "The passphrase could not be saved and assigned to this key.",
  "keys.unassignPassphraseFailed":
    "The stored passphrase could not be detached from this key.",
  "keys.addToAgent": "Add to ssh-agent",
  "keys.removeFromAgent": "Remove from ssh-agent",
  "keys.agentRemoveFailed":
    "The key could not be removed from ssh-agent. It may already have been removed, or the required public key may be missing.",
  "keys.trashConfirmHeading": "Move {path} to the trash",
  "keys.trashExplain": "These files move together, because they are one key:",
  "keys.trashReferences":
    "{count} Host block(s) still reference this key. After the move, their IdentityFile lines will reference a missing file. ssh reports the error and then tries other authentication methods.",
  "keys.trashNoReferences": "No Host block references this key.",
  "keys.trashIsRecoverable":
    "Nothing is deleted. The files move into the trash below, where they can be restored.",
  "keys.trashConfirm": "Move it to the trash",
  "keys.trashCancel": "Keep it",
  "keys.moveToTrash": "Move to trash",
  "keys.publicKeyHeading": "Public key: {path}",
  "keys.publicKeyLabel": "Public key",
  "keys.close": "Close",
  "keys.unreadableHeading": "Files this scan could not classify",
  "keys.unreadableNote":
    "These are inside ~/.ssh and are missing from the table above. Nothing was changed about them.",
  "keys.unresolvedHeading":
    "Configuration entries pointing at a key that is not there",
  "keys.agentHeading": "ssh-agent",
  "keys.agentEmpty": "ssh-agent is reachable, but no keys are registered.",
  "keys.agentIdentitiesCaption": "Keys registered with ssh-agent",
  "keys.colComment": "Comment",
  "keys.agentUnavailable":
    "This process cannot connect to ssh-agent, so keys cannot be registered. Both the ssh-add command and an SSH_AUTH_SOCK that identifies the agent are required.",
  "keys.agentDelegationsNote":
    "These configuration entries use a key from ssh-agent instead of referencing a key file:",
  "keys.registerHeading": "Add to ssh-agent: {path}",
  "keys.registerNote":
    "The passphrase is passed to ssh-add through standard input, so it is not included in the command line or child-process environment. sshc does not store it or retain it after this operation.",
  "keys.keyPassphrase": "Key passphrase",
  "keys.lifetime": "Lifetime",
  "keys.lifetimeForever": "Until ssh-agent exits",
  "keys.lifetimeHour": "1 hour",
  "keys.lifetimeFourHours": "4 hours",
  "keys.lifetimeTwelveHours": "12 hours",
  "keys.useStoredPassphrase": "Use a stored passphrase",
  "keys.choosePassphraseName": "— choose a name —",
  "keys.useThisPassphrase": "Use this passphrase",
  "keys.usesStoredPassphrase":
    "This key uses the stored passphrase named {name}.",
  "keys.usesDedicatedPassphrase": "A passphrase is saved only for this key.",
  "keys.typedWins":
    "Leave this empty to use the saved passphrase. If you enter a value, that value is used instead.",
  "keys.assignPassphraseFailed":
    "The selected passphrase could not be assigned to this key.",
  "keys.registerSubmit": "Add key to ssh-agent",
  "keys.cancel": "Cancel",
  "keys.passphraseHeading": "Change passphrase: {path}",
  "keys.passphraseNote":
    "The passphrases typed here are used only for this change and are not stored. Use “Stored passphrase” in the key list when you explicitly want the sshc vault to remember one.",
  "keys.currentPassphrase": "Current passphrase",
  "keys.newPassphrase": "New passphrase",
  "keys.removePassphrase":
    "Remove the passphrase and leave the key unprotected on disk",
  "keys.savePassphrase": "Save new passphrase",
  "keys.createHeading": "Create a key",
  "keys.generatedHeading": "Key created",
  "keys.generatedNext": "{path} is ready. Choose where to use it next.",
  "keys.assignGenerated": "Assign to a connection",
  "keys.installGenerated": "Install on a server",
  "keys.algorithm": "Algorithm",
  "keys.fileName": "File name",
  "keys.comment": "Comment",
  "keys.passphrase": "Passphrase",
  "keys.createUnencrypted":
    "Create without a passphrase, and accept that anyone who reads the file can use the key",
  "keys.createSubmit": "Create key",
  "keys.showTerminalCommand": "Show Terminal command",
  "keys.hardwareNote":
    "Creating a hardware-backed key requires interaction with a security key, so sshc does not support it. Run this command in a terminal:",
  "keys.trashHeading": "Trash",
  "keys.trashSummary": "Trash ({count})",
  "keys.trashNote":
    "Keys moved to Trash remain here until you delete them permanently. They are not deleted automatically.",
  "keys.trashCaption": "Soft-deleted keys",
  "keys.trashEmpty": "Nothing has been deleted.",
  "keys.colFiles": "Files",
  "keys.colAge": "Age",
  "keys.colStatus": "Status",
  "keys.ageStale": "{days} days · older than {retention} days",
  "keys.age": "{days} days",
  "keys.restorable": "Restorable",
  "keys.restore": "Restore",
  "keys.purgeWarning":
    "This cannot be undone. There is no backup of a permanently deleted key.",
  "keys.confirmPurge": "Confirm permanent delete",
  "keys.purge": "Delete permanently",
  "keys.noteFingerprintUnavailable": "Fingerprint unavailable",
  "keys.noteSymbolicLink": "Symbolic link, not followed",
  "keys.noteEmptyFile": "Empty file",
  "keys.noteNotRegularFile": "Not a regular file",
  "keys.noteCommentNotPreserved": "Comment not preserved",
  "keys.certKeyId": "key id {keyId}",
  "keys.certFor": "for {principals}",
  "keys.certAnyPrincipal": "for any principal",
  "keys.certNeverExpires": "never expires",
  "keys.certExpired": "expired {when}",
  "keys.certValidUntil": "valid until {when}",
  "keys.certSigns": "signs {keyType} {fingerprint}",
  "keys.reference": "{directive} {value} — {path}:{line}",
  "keys.referenceWithReason": "{directive} {value} — {path}:{line} ({reason})",
  "keys.unreadableEntry": "{path} — {reason}",

  "keys.colChoose": "Choose",
  "keys.chooseKey": "Choose {path}",
  "keys.listFilter": "What to list",
  "keys.listFilterKeys": "Keys only",
  "keys.listFilterAll": "Every file in ~/.ssh",
  "keys.dragKey": "Drag {path}",
  "keys.chosenCount": "{count} selected",
  "keys.moveTargetLabel": "Move into",
  "keys.moveChosen": "Move",
  "keys.clearChosen": "Clear the selection",
  "keys.moveMoved": "Moved {count}.",
  "keys.moveBlocked": "{path} cannot be moved: {reason}",
  "keys.moveFailed": "{path} could not be moved.",
  "keys.folders": "Folders",
  "keys.foldersLabel": "Key folders",
  "keys.folderRow": "{name}, {count}",
  "keys.folderAll": "All keys",
  "keys.folderUngrouped": "No group",
  "keys.relocate": "Rename or move",
  "keys.relocateHeading": "Rename or move: {path}",
  "keys.relocateNote":
    "Changing the name or group moves the key files. Every IdentityFile and CertificateFile that references this key is updated in the same transaction, so no reference to the old path remains.",
  "keys.relocateNewName": "Name",
  "keys.relocateGroup": "Group",
  "keys.relocateSubmit": "Rename or move the key",
  "keys.relocateFailed": "The key could not be moved.",
  "keys.relocateDone": "Now at {path}.",
  "keys.relocateMoved": "Files moved",
  "keys.relocateRewritten": "Configuration entries rewritten",
  "keys.relocateSkipped":
    "Not moved because the file name is not the key name followed by a suffix: {paths}",
  "keys.relocateRefused": "Nothing was moved and nothing was written:",
  "keys.relocateFilePair": "{from} → {to}",
  "keys.relocateReference": "{directive} {from} → {to} — {path}:{line}",
  "keys.groupNone": "No group (~/.ssh itself)",
  "keys.createGroup": "Group",
  "keys.blockerTargetOccupied": "{detail} already exists",
  "keys.blockerUnresolved":
    "{detail} is a path that sshc cannot resolve and may reference this key",
  "keys.blockerReferenceExternal":
    "{detail} is outside ~/.ssh and cannot be rewritten",
  "keys.blockerGroupNotDeclared": "no Include line declares the group {detail}",
  "keys.blockerDestinationIsConfig":
    "an Include would read {detail} as configuration",
  "keys.blockerStateDirectory":
    "{detail} is inside the engine's own state directory",
  "keys.blockerOther": "{detail}",
} as const;

export type MessageKey = keyof typeof en;
