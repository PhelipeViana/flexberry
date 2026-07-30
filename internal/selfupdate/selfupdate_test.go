package selfupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckFindsNewerPrereleaseWithMatchingAssets(t *testing.T) {
	asset := executableAssetName()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		fmt.Fprintf(w, `[{
			"tag_name":"v0.1.0-alpha.16",
			"draft":false,
			"assets":[
				{"name":%q,"browser_download_url":"https://example.test/flexberry"},
				{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"}
			]
		}]`, asset)
	}))
	defer server.Close()
	t.Setenv("FLEXBERRY_RELEASES_URL", server.URL)

	release, outdated, err := Check(context.Background(), "0.1.0-alpha.15")
	if err != nil {
		t.Fatal(err)
	}
	if !outdated || release.Version != "0.1.0-alpha.16" {
		t.Fatalf("release inesperado: %#v, outdated=%v", release, outdated)
	}
}

func TestCheckIgnoresCurrentAndOlderVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v0.1.0-alpha.15","draft":false,"assets":[]},
			{"tag_name":"v0.1.0-alpha.14","draft":false,"assets":[]}
		]`)
	}))
	defer server.Close()
	t.Setenv("FLEXBERRY_RELEASES_URL", server.URL)

	_, outdated, err := Check(context.Background(), "0.1.0-alpha.15")
	if err != nil {
		t.Fatal(err)
	}
	if outdated {
		t.Fatal("a versão atual não deveria ser considerada desatualizada")
	}
}

func TestCompareVersionsUsesNumericPrereleaseSequence(t *testing.T) {
	if compareVersions("0.1.0-alpha.16", "0.1.0-alpha.9") <= 0 {
		t.Fatal("alpha.16 deveria ser posterior a alpha.9")
	}
	if compareVersions("0.1.0", "0.1.0-alpha.16") <= 0 {
		t.Fatal("uma versão estável deveria ser posterior à sua pré-release")
	}
}
