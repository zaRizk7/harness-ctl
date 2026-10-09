package providers

import "testing"

func TestEmbeddedContractCorruptionFailsClosed(t *testing.T) {
	old := providerData
	defer func() { providerData = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("corrupt bundled contract accepted")
		}
	}()
	providerData = []byte("invalid-json")
	Defaults()
}
