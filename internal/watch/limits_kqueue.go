//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package watch

// perEntryWatches reports that fsnotify's kqueue backend holds a descriptor
// for every entry of a watched directory, so MaxEntries applies.
const perEntryWatches = true
