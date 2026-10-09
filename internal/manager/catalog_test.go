package manager

import (
	"path/filepath"
	"testing"
)

func TestCatalogConfigurationIsEngineScoped(t *testing.T) {
	c := defaultConfig(t.TempDir())
	c.Harnesses = []harnessSpec{{ID: "custom", Name: "Custom", Command: "custom", Kind: "npm", Package: "custom-package", DefaultHome: ".custom"}}
	e, err := newEngine(c, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := e.specFor("custom"); err != nil || s.Package != "custom-package" {
		t.Fatal(s, err)
	}
	if _, err := e.specFor("pi"); err == nil {
		t.Fatal("engine ignored configured catalog")
	}
	if _, err := specFor("pi"); err != nil {
		t.Fatal("catalog leaked between engines")
	}
}

func TestCatalogRejectsUnsafeData(t *testing.T) {
	for _, mutate := range []func(*harnessSpec){
		func(s *harnessSpec) { s.ID = "../escape" },
		func(s *harnessSpec) { s.Command = "/tmp/command" },
		func(s *harnessSpec) { s.HomeEnv = "BAD;ENV" },
		func(s *harnessSpec) { s.DefaultHome = "../state" },
		func(s *harnessSpec) { s.Kind = "shell" },
		func(s *harnessSpec) { s.ConfigFiles = []string{"../config"} },
	} {
		c := defaultConfig(t.TempDir())
		mutate(&c.Harnesses[0])
		if err := c.validate(); err == nil {
			t.Fatal("unsafe catalog accepted")
		}
	}
	c := defaultConfig(t.TempDir())
	c.Harnesses = append(c.Harnesses, c.Harnesses[0])
	if err := c.validate(); err == nil {
		t.Fatal("duplicate catalog accepted")
	}
	c = defaultConfig(t.TempDir())
	c.CatalogFile = filepath.Join(c.Home, "catalog.json")
	if err := writeJSON(c.CatalogFile, []harnessSpec{{ID: "custom", Name: "Custom", Command: "custom", DefaultHome: ".custom", Kind: "npm", Package: "custom"}}); err != nil {
		t.Fatal(err)
	}
	e, err := newEngine(c, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.specFor("custom"); err != nil {
		t.Fatal(err)
	}
}

var catalog = defaultCatalog()

func specFor(id string) (harnessSpec, error) {
	return (&engine{cfg: config{Harnesses: defaultCatalog()}}).specFor(id)
}
