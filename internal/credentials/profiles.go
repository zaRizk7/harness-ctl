// Package credentials describes encrypted, harness-specific native credential
// profiles. It never performs OAuth, refreshes tokens or exports OS secrets.
package credentials

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// Location names a file relative to one selected native state root. Root is the
// documented root index, so data/config stores remain distinct after relocation.
type Location struct {
	Root int    `json:"root"`
	Path string `json:"path"`
}

// File contains one native credential payload. Bytes are encrypted at rest and
// excluded from profile inventory, previews and operation journals.
type File struct {
	Location Location `json:"location"`
	Data     []byte   `json:"data"`
}

// Profile captures one harness format for explicit, compatible application.
// Captured timestamps describe the copy, not token validity or native refresh.
type Profile struct {
	ID       string    `json:"id"`
	Harness  string    `json:"harness"`
	Captured time.Time `json:"captured"`
	Files    []File    `json:"files"`
}

// View is secret-free inventory metadata for one stored credential profile.
type View struct {
	ID, Harness string
	Captured    time.Time
	FileCount   int
}

// ValidateLocations rejects duplicate, absolute or escaping native file paths.
func ValidateLocations(locations []Location) error {
	seen := map[Location]bool{}
	for _, loc := range locations {
		if loc.Root < 0 || !filepath.IsLocal(loc.Path) || loc.Path == "." || seen[loc] {
			return fmt.Errorf("invalid native credential file location")
		}
		seen[loc] = true
	}
	return nil
}

// ValidateStored checks profile identities, payloads and confined file locations.
// It permits obsolete native contracts so their encrypted copies remain removable.
func ValidateStored(p Profile) error {
	if !identityPattern.MatchString(p.ID) || !identityPattern.MatchString(p.Harness) || len(p.Files) == 0 {
		return fmt.Errorf("invalid credential profile")
	}
	var locations []Location
	for _, file := range p.Files {
		if len(file.Data) == 0 {
			return fmt.Errorf("credential profile contains an empty native file")
		}
		locations = append(locations, file.Location)
	}
	return ValidateLocations(locations)
}

// Validate binds valid stored profile bytes to current harness/file contracts.
// Cross-harness restoration and unconfigured paths fail before publication.
func Validate(p Profile, harness string, allowed []Location) error {
	if err := ValidateStored(p); err != nil {
		return err
	}
	if p.Harness != harness {
		return fmt.Errorf("incompatible credential profile harness")
	}
	for _, file := range p.Files {
		found := false
		for _, loc := range allowed {
			if loc == file.Location {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("credential profile contains an undeclared native file")
		}
	}
	return nil
}

// Views returns metadata only, without native file paths or credential bytes.
func Views(profiles []Profile) []View {
	views := []View{}
	for _, p := range profiles {
		views = append(views, View{p.ID, p.Harness, p.Captured, len(p.Files)})
	}
	return views
}
