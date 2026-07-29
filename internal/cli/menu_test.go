package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
