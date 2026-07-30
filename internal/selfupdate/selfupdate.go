package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const defaultReleasesURL = "https://api.github.com/repos/PhelipeViana/flexberry/releases?per_page=20"

type Release struct {
	Version     string
	DownloadURL string
	ChecksumURL string
	AssetName   string
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Draft   bool          `json:"draft"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func Check(ctx context.Context, currentVersion string) (Release, bool, error) {
	url := strings.TrimSpace(os.Getenv("FLEXBERRY_RELEASES_URL"))
	if url == "" {
		url = defaultReleasesURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, false, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "flexberry/"+currentVersion)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Release{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, false, fmt.Errorf("consultar releases: HTTP %d", response.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&releases); err != nil {
		return Release{}, false, err
	}
	assetName := executableAssetName()
	for _, candidate := range releases {
		if candidate.Draft || compareVersions(candidate.TagName, currentVersion) <= 0 {
			continue
		}
		release := Release{Version: strings.TrimPrefix(candidate.TagName, "v"), AssetName: assetName}
		for _, asset := range candidate.Assets {
			switch asset.Name {
			case assetName:
				release.DownloadURL = asset.URL
			case "checksums.txt":
				release.ChecksumURL = asset.URL
			}
		}
		if release.DownloadURL != "" && release.ChecksumURL != "" {
			return release, true, nil
		}
	}
	return Release{}, false, nil
}

func Install(ctx context.Context, release Release) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("a atualização automática do executável está disponível somente no Windows")
	}
	current, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("localizar executável atual: %w", err)
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return "", err
	}
	download := current + ".download"
	if err := downloadFile(ctx, release.DownloadURL, download); err != nil {
		return "", err
	}
	expected, err := fetchChecksum(ctx, release.ChecksumURL, release.AssetName)
	if err != nil {
		_ = os.Remove(download)
		return "", err
	}
	if err := verifyChecksum(download, expected); err != nil {
		_ = os.Remove(download)
		return "", err
	}

	script := current + ".update.ps1"
	body := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$process = Get-Process -Id %d -ErrorAction SilentlyContinue
if ($process) { $process.WaitForExit() }
$current = %s
$download = %s
$backup = "$current.old"
Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
Move-Item -LiteralPath $current -Destination $backup -Force
Move-Item -LiteralPath $download -Destination $current -Force
Start-Process -FilePath $current -WorkingDirectory %s
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force
`, os.Getpid(), psQuote(current), psQuote(download), psQuote(filepath.Dir(current)))
	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		_ = os.Remove(download)
		return "", fmt.Errorf("criar instalador da atualização: %w", err)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
	command.SysProcAttr = windowsHiddenProcessAttributes()
	if err := command.Start(); err != nil {
		_ = os.Remove(script)
		_ = os.Remove(download)
		return "", fmt.Errorf("iniciar atualização: %w", err)
	}
	return current, nil
}

func executableAssetName() string {
	name := "flexberry-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func downloadFile(ctx context.Context, url, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("baixar atualização: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("baixar atualização: HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, 256<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("salvar atualização: %w", copyErr)
	}
	return closeErr
}

func fetchChecksum(ctx context.Context, url, asset string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("baixar checksums: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("baixar checksums: HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum de %s não encontrado", asset)
}

func verifyChecksum(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum inválido: download descartado")
	}
	return nil
}

func compareVersions(left, right string) int {
	leftCore, leftPre := splitVersion(left)
	rightCore, rightPre := splitVersion(right)
	if comparison := compareIdentifiers(leftCore, rightCore); comparison != 0 {
		return comparison
	}
	if len(leftPre) == 0 && len(rightPre) > 0 {
		return 1
	}
	if len(leftPre) > 0 && len(rightPre) == 0 {
		return -1
	}
	return compareIdentifiers(leftPre, rightPre)
}

func compareIdentifiers(leftParts, rightParts []string) int {
	length := len(leftParts)
	if len(rightParts) > length {
		length = len(rightParts)
	}
	for index := 0; index < length; index++ {
		var a, b string
		if index < len(leftParts) {
			a = leftParts[index]
		}
		if index < len(rightParts) {
			b = rightParts[index]
		}
		aNumber, aErr := strconv.Atoi(a)
		bNumber, bErr := strconv.Atoi(b)
		var comparison int
		switch {
		case aErr == nil && bErr == nil:
			comparison = aNumber - bNumber
		case aErr == nil:
			comparison = 1
		case bErr == nil:
			comparison = -1
		default:
			comparison = strings.Compare(a, b)
		}
		if comparison < 0 {
			return -1
		}
		if comparison > 0 {
			return 1
		}
	}
	return 0
}

func splitVersion(value string) ([]string, []string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	parts := strings.SplitN(value, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(parts) == 1 {
		return core, nil
	}
	return core, strings.Split(parts[1], ".")
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
