// Package manager implements harness-ctl's macOS TUI and native adapter engine.
//
// Inventory and planning are read-only. Approved plans pass ownership and stale
// preview checks under a single mutation lock before encrypted recovery and
// native commands. Failed operations restore state or leave an explicit recovery
// journal. Installation prefixes, user state and launch profiles have separate
// ownership. Tests use synthetic homes and runners, never live harness mutations.
package manager
