package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry"
)

func TestLoadManifestUsesOnlineEnabledOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"version": 1,
			"message": "teste",
			"items": [
				{"label":"ORM","command":"orm sync","enabled":true},
				{"label":"Oculto","command":"factory run","enabled":false}
			]
		}`))
	}))
	defer server.Close()
	t.Setenv("FLEXBERRY_MENU_URL", server.URL)
	manifest, online := LoadManifest(context.Background())
	if !online {
		t.Fatal("manifesto online não foi aceito")
	}
	if len(manifest.Items) != 1 || manifest.Items[0].Command != "orm sync" {
		t.Fatalf("opções inesperadas: %#v", manifest.Items)
	}
}

func TestRenderMenuHeaderShowsInstalledVersion(t *testing.T) {
	previousVersion := flexberry.Version
	flexberry.Version = "0.1.0-alpha.16"
	t.Cleanup(func() {
		flexberry.Version = previousVersion
	})

	var output strings.Builder
	renderMenuHeader(&output, Manifest{Message: "Versão experimental"}, true, nil)

	if !strings.Contains(output.String(), "Versão instalada") ||
		!strings.Contains(output.String(), "v0.1.0-alpha.16") {
		t.Fatalf("cabeçalho não exibe a versão instalada: %q", output.String())
	}
}

func TestLoadManifestRejectsArbitraryCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"version":1,
			"items":[{"label":"Perigoso","command":"powershell calc.exe","enabled":true}]
		}`))
	}))
	defer server.Close()
	t.Setenv("FLEXBERRY_MENU_URL", server.URL)
	manifest, online := LoadManifest(context.Background())
	if online {
		t.Fatal("manifesto perigoso deveria usar fallback")
	}
	if len(manifest.Items) == 0 {
		t.Fatal("fallback vazio")
	}
}
