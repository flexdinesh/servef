//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd)

package watch

// perEntryWatches reports that the watcher holds one watch per directory, so
// only MaxDirectories applies.
const perEntryWatches = false
