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
	"factory create":    true,
	"factory reload":    true,
	"factory run":       true,
	"version":           true,
	"help":              true,
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

	fmt.Print("\r\n\033[34mFlexberry\033[0m\r\n")
	if manifest.Message != "" {
		fmt.Printf("\033[36m%s\033[0m\r\n", manifest.Message)
	}
	if !online {
		fmt.Print("\033[33m⚠ Menu online indisponível; usando opções locais.\033[0m\r\n")
	}
	fmt.Print("Use ↑/↓ e Enter:\r\n\r\n")
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
		{Label: "ORM Reload (atualiza e recria)", Command: "orm reload", Enabled: true},
		{Label: "ORM Run (roda a atualização)", Command: "orm run", Enabled: true},
		{Label: "Factories Reload (atualiza e recria)", Command: "factory reload", Enabled: true},
		{Label: "Factories Run (executa as factories)", Command: "factory run", Enabled: true},
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
		arrow, color := "  ", "\033[0m"
		if index == selected {
			arrow, color = "➜ ", "\033[32m"
		}
		fmt.Printf("\033[2K\r%s%s%s\033[0m\r\n", color, arrow, item.Label)
	}
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
