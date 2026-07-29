package config

import "testing"

func TestPackageFromPath(t *testing.T) {
	tests := map[string]string{
		"internal/orm":                "orm",
		"internal/teste/factories":    "factories",
		"internal/database/generated": "generated",
	}
	for path, expected := range tests {
		actual, err := packageFromPath(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if actual != expected {
			t.Fatalf("%s: package %q, esperado %q", path, actual, expected)
		}
	}
}
