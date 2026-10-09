package manager

import "github.com/zaRizk7/harness-ctl/internal/storage"

// validateOwnedPath delegates filesystem ownership to the storage module.
var validateOwnedPath = storage.ValidateOwnedPath

// rejectLinkedAncestors delegates filesystem ownership to the storage module.
var rejectLinkedAncestors = storage.RejectLinkedAncestors

// within delegates filesystem ownership to the storage module.
var within = storage.Within

// canonicalSystemPath delegates filesystem ownership to the storage module.
var canonicalSystemPath = storage.CanonicalSystemPath

// fingerprint delegates filesystem ownership to the storage module.
var fingerprint = storage.Fingerprint

// atomicWrite delegates filesystem ownership to the storage module.
var atomicWrite = storage.AtomicWrite

// writeJSON delegates filesystem ownership to the storage module.
var writeJSON = storage.WriteJSON

// readJSON delegates filesystem ownership to the storage module.
var readJSON = storage.ReadJSON
