package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry"
	"github.com/PhelipeViana/flexberry/internal/cliui"
	"github.com/PhelipeViana/flexberry/internal/selfupdate"
	"golang.org/x/term"
)

const DefaultMenuURL = "https://raw.githubusercontent.com/PhelipeViana/flexberry/main/config/menu.json"

var allowedCommands = map[string]bool{
	"connection report": true,
	"config install":    true,
	"config update":     true,
	"config remove":     true,
	"orm sync":          true,
	"orm reload":        true,
	"orm run":           true,
	"migrate reload":    true,
	"migrate run":       true,
	"factory create":    true,
	"factory reload":    true,
	"factory run":       true,
	"version":           true,
	"help":              true,
	"self update":       true,
	"exit":              true,
}

type Manifest struct {
	Version int         `json:"version"`
	Message string      `json:"message"`
	Items   []MenuEntry `json:"items"`
}

type MenuEntry struct {
	Label   string `json:"label"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

func LoadManifest(ctx context.Context) (Manifest, bool) {
	url := strings.TrimSpace(os.Getenv("FLEXBERRY_MENU_URL"))
	if url == "" {
		url = DefaultMenuURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fallbackManifest(), false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fallbackManifest(), false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fallbackManifest(), false
	}
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(response.Body, 256<<10))
	if err := decoder.Decode(&manifest); err != nil || validateManifest(manifest) != nil {
		return fallbackManifest(), false
	}
	manifest.Items = enabledItems(manifest.Items)
	if len(manifest.Items) == 0 {
		return fallbackManifest(), false
	}
	return manifest, true
}

func Select() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	manifest, online := LoadManifest(ctx)
	release, outdated, updateErr := selfupdate.Check(ctx, flexberry.Version)
	if outdated {
		manifest.Message = fmt.Sprintf("⚠ Versão %s desatualizada. Nova versão disponível: %s", flexberry.Version, release.Version)
		manifest.Items = []MenuEntry{
			{Label: "Baixar e instalar a nova versão automaticamente", Command: "self update", Enabled: true},
			{Label: "Sair", Command: "exit", Enabled: true},
		}
	}
	if len(manifest.Items) == 0 {
		return "", fmt.Errorf("nenhuma opção disponível")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("menu interativo requer um terminal; use flexberry help")
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(os.Stdin.Fd()), state)

	renderMenuHeader(os.Stdout, manifest, online, updateErr)
	selected := 0
	render(manifest.Items, selected)
	buffer := make([]byte, 3)
	for {
		n, readErr := os.Stdin.Read(buffer)
		if readErr != nil {
			return "", readErr
		}
		key := buffer[:n]
		switch {
		case len(key) == 1 && (key[0] == '\r' || key[0] == '\n'):
			fmt.Print("\r\n")
			return manifest.Items[selected].Command, nil
		case len(key) == 1 && key[0] == 3:
			return "exit", nil
		case arrowUp(key):
			selected = (selected - 1 + len(manifest.Items)) % len(manifest.Items)
		case arrowDown(key):
			selected = (selected + 1) % len(manifest.Items)
		default:
			continue
		}
		fmt.Printf("\033[%dA\r", len(manifest.Items))
		render(manifest.Items, selected)
	}
}

func validateManifest(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("versão de menu não suportada")
	}
	if len(manifest.Items) == 0 {
		return fmt.Errorf("menu vazio")
	}
	for _, item := range manifest.Items {
		item.Command = normalizeCommand(item.Command)
		if strings.TrimSpace(item.Label) == "" || !allowedCommands[item.Command] {
			return fmt.Errorf("opção inválida")
		}
	}
	return nil
}

func fallbackManifest() Manifest {
	return Manifest{Version: 1, Message: "Ferramentas do projeto", Items: []MenuEntry{
		{Label: "Connection (relatório das conexões do YAML)", Command: "connection report", Enabled: true},
		{Label: "MIGRATE Reload (atualiza e recria)", Command: "migrate reload", Enabled: true},
		{Label: "MIGRATE Run (roda a atualização)", Command: "migrate run", Enabled: true},
		{Label: "Factories Reload (atualiza e recria)", Command: "factory reload", Enabled: true},
		{Label: "Factories Run (executa as factories)", Command: "factory run", Enabled: true},
		{Label: "ORM Reload (atualiza e recria)", Command: "orm reload", Enabled: true},
		{Label: "ORM Run (roda a atualização)", Command: "orm run", Enabled: true},
		{Label: "Flexberry Init (instala ou repara a configuração)", Command: "config install", Enabled: true},
		{Label: "Sair", Command: "exit", Enabled: true},
	}}
}

func enabledItems(items []MenuEntry) []MenuEntry {
	result := make([]MenuEntry, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			item.Command = normalizeCommand(item.Command)
			result = append(result, item)
		}
	}
	return result
}

func render(items []MenuEntry, selected int) {
	for index, item := range items {
		cursor, color := "     ", cliui.ColorReset
		if index == selected {
			cursor, color = "  ❯  ", cliui.ColorGreen
		}
		fmt.Printf("\033[2K\r%s%s%s%s\r\n", color, cursor, item.Label, cliui.ColorReset)
	}
}

func renderMenuHeader(writer io.Writer, manifest Manifest, online bool, updateErr error) {
	version := strings.TrimPrefix(flexberry.Version, "v")
	fmt.Fprintf(writer, "\r\n%s╭─ FLEXBERRY CLI%s\r\n", cliui.ColorBlue, cliui.ColorReset)
	fmt.Fprintf(writer, "%s│%s Versão instalada  %sv%s%s\r\n",
		cliui.ColorBlue, cliui.ColorReset, cliui.ColorCyan, version, cliui.ColorReset)
	if manifest.Message != "" {
		fmt.Fprintf(writer, "%s│%s %s%s%s\r\n",
			cliui.ColorBlue, cliui.ColorReset, cliui.ColorGray, manifest.Message, cliui.ColorReset)
	}
	fmt.Fprintf(writer, "%s╰────────────────────────────────────────────────────────%s\r\n",
		cliui.ColorBlue, cliui.ColorReset)
	if !online {
		fmt.Fprintf(writer, "\r\n%s⚠ Menu online indisponível; usando opções locais.%s\r\n",
			cliui.ColorYellow, cliui.ColorReset)
	}
	if updateErr != nil {
		fmt.Fprintf(writer, "\r\n%s⚠ Não foi possível verificar se há uma nova versão.%s\r\n",
			cliui.ColorYellow, cliui.ColorReset)
	}
	fmt.Fprintf(writer, "\r\n%s↑/↓%s navegar  %s•%s  %sEnter%s selecionar  %s•%s  %sCtrl+C%s sair\r\n",
		cliui.ColorCyan, cliui.ColorReset,
		cliui.ColorGray, cliui.ColorReset,
		cliui.ColorCyan, cliui.ColorReset,
		cliui.ColorGray, cliui.ColorReset,
		cliui.ColorCyan, cliui.ColorReset)
	fmt.Fprintf(writer, "\r\n%sAÇÕES DISPONÍVEIS%s\r\n\r\n", cliui.ColorGray, cliui.ColorReset)
}

func normalizeCommand(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func arrowUp(key []byte) bool {
	return len(key) >= 3 && key[0] == 27 && key[1] == '[' && key[2] == 'A'
}

func arrowDown(key []byte) bool {
	return len(key) >= 3 && key[0] == 27 && key[1] == '[' && key[2] == 'B'
}
