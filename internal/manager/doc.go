// Package manager implements harness-ctl's macOS TUI, CLI and native adapter engine.
//
// Inventory and planning are read-only. Approved plans pass ownership and stale
// preview checks under a single mutation lock before encrypted recovery and
// native commands. Failed operations restore state or leave an explicit recovery
// journal. Installation prefixes, user state and launch profiles have separate
// ownership. Batch requests share one approval and lock, with independent item
// recovery. Provider credentials use a separate encrypted vault. Account reads
// distinguish API costs, explicitly selected native quota reporting and provider
// account pages. Native codecs and local component planning have separate owners. Tests
// use synthetic homes and runners, never live harness mutations.
package manager
