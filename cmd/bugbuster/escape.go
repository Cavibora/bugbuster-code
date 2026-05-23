package main

// EscapeWatcher removed — it breaks readline by blocking stdin.Read() in raw mode.
// Stream cancellation now only via Ctrl+C (signal handler + context cancellation).