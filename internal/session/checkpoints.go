package session

import "errors"

// ErrNoCheckpoint indicates that no checkpoint exists for requested message ID.
var ErrNoCheckpoint = errors.New("no checkpoint found")
