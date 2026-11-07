// Package privatefile opens files only their owner may read, for appending and reading: the engine's log and the
// session logs. The folders on the way are made private too, and neither a folder nor the file may be a link.
package privatefile
