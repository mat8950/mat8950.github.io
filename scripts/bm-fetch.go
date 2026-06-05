// bm-fetch.go — Pré-fetch les métadonnées des URLs dans app-new.link.txt
// Pour chaque URL : appel API GitHub ou parsing og: tags, vérification doublons.
// Produit app-new.meta.json — à lire par Claude avant de catégoriser.
// Run: go run ./scripts/bm-fetch.go  (depuis la racine du dépôt)
// Optionnel: GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-fetch.go
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	inputFile   = "app-new.link.txt"
	outputFile  = "app-new.meta.json"
	csvFile     = "bookmarks.csv"
	archiveFile = "app.link.txt"
)

type MetaEntry struct {
	InputURL       string   `json:"input_url"`
	OfficialURL    string   `json:"official_url"`
	Name           string   `json:"name"`
	DescriptionRaw string   `json:"description_raw"`
	IsGitHub       bool     `json:"is_github"`
	GitHubTopics   []string `json:"github_topics"`
	IsDuplicate    bool     `json:"is_duplicate"`
	ExistingURL    string   `json:"existing_url,omitempty"`
}

type gitHubRepo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Homepage    string   `json:"homepage"`
	Topics      []string `json:"topics"`
}

func main() {
	lines, err := readLines(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] reading %s: %v\n", inputFile, err)
		os.Exit(1)
	}
	// Filter blank lines and comments
	var urls []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		urls = append(urls, l)
	}
	if len(urls) == 0 {
		fmt.Println("[i] Aucune URL dans", inputFile)
		os.Exit(0)
	}
	fmt.Printf("[i] %d URLs à traiter\n\n", len(urls))

	domains, err := loadCSVDomains(csvFile)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[WARN] loading CSV domains: %v\n", err)
	}

	// Also check app.link.txt — any URL already processed is a duplicate
	if archLines, err := readLines(archiveFile); err == nil {
		for _, l := range archLines {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			if key := normURLKey(l); key != "" {
				if _, exists := domains[key]; !exists {
					domains[key] = l
				}
			}
			// GitHub canonical key (github.com/owner/repo)
			if owner, repo, ok := normalizeGitHubURL(l); ok {
				canonical := fmt.Sprintf("github.com/%s/%s", strings.ToLower(owner), strings.ToLower(repo))
				if _, exists := domains[canonical]; !exists {
					domains[canonical] = l
				}
			}
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}

	var entries []MetaEntry
	for i, rawURL := range urls {
		prefix := fmt.Sprintf("[%d/%d]", i+1, len(urls))

		isDup, existingURL := checkDuplicate(rawURL, domains)
		if isDup {
			fmt.Printf("%s %s → DUPLICATE de %s\n", prefix, rawURL, existingURL)
			entries = append(entries, MetaEntry{
				InputURL:    rawURL,
				OfficialURL: rawURL,
				IsDuplicate: true,
				ExistingURL: existingURL,
			})
			continue
		}

		owner, repo, isGH := normalizeGitHubURL(rawURL)
		if isGH {
			ghRepo, err := fetchGitHub(owner, repo, client)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s → ERROR: %v\n", prefix, rawURL, err)
				entries = append(entries, MetaEntry{InputURL: rawURL, OfficialURL: rawURL, IsGitHub: true})
				continue
			}
			officialURL := strings.TrimSpace(ghRepo.Homepage)
			if officialURL == "" {
				officialURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
			}
			fmt.Printf("%s github.com/%s/%s → %s\n", prefix, owner, repo, officialURL)
			entries = append(entries, MetaEntry{
				InputURL:       rawURL,
				OfficialURL:    officialURL,
				Name:           ghRepo.Name,
				DescriptionRaw: ghRepo.Description,
				IsGitHub:       true,
				GitHubTopics:   ghRepo.Topics,
			})
			// Polite delay between GitHub API calls
			time.Sleep(100 * time.Millisecond)
		} else {
			name, desc, canonURL, err := fetchHTML(rawURL, client)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s → ERROR: %v\n", prefix, rawURL, err)
				entries = append(entries, MetaEntry{InputURL: rawURL, OfficialURL: rawURL})
				continue
			}
			officialURL := rawURL
			if canonURL != "" {
				officialURL = canonURL
			}
			fmt.Printf("%s %s → fetched (%q)\n", prefix, rawURL, name)
			entries = append(entries, MetaEntry{
				InputURL:       rawURL,
				OfficialURL:    officialURL,
				Name:           name,
				DescriptionRaw: desc,
			})
		}
	}

	if err := writeJSON(outputFile, entries); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] writing %s: %v\n", outputFile, err)
		os.Exit(1)
	}

	dups := 0
	for _, e := range entries {
		if e.IsDuplicate {
			dups++
		}
	}
	fmt.Printf("\n[OK] %s écrit — %d entrées (%d doublons)\n", outputFile, len(entries), dups)
	fmt.Printf("[→] Lire %s et compléter category, subcategory, description (FR), tags\n", outputFile)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

// loadCSVDomains returns a map of normalised "host+path" → full stored URL
func loadCSVDomains(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	urlIdx := -1
	for i, h := range headers {
		if h == "url" {
			urlIdx = i
			break
		}
	}
	if urlIdx < 0 {
		return nil, fmt.Errorf("no 'url' column in CSV")
	}
	domains := make(map[string]string)
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if urlIdx >= len(row) {
			continue
		}
		stored := row[urlIdx]
		key := normURLKey(stored)
		if key != "" {
			domains[key] = stored
		}
	}
	return domains, nil
}

func normURLKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	key := strings.ToLower(u.Host + strings.TrimRight(u.Path, "/"))
	return key
}

// normalizeGitHubURL returns owner/repo for any github.com URL, stripping /tree/... etc.
func normalizeGitHubURL(rawURL string) (owner, repo string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host != "github.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func fetchGitHub(owner, repo string, client *http.Client) (gitHubRepo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := url.Parse(apiURL)
	if err != nil {
		return gitHubRepo{}, err
	}
	httpReq, err := http.NewRequest("GET", req.String(), nil)
	if err != nil {
		return gitHubRepo{}, err
	}
	httpReq.Header.Set("User-Agent", "bm-fetch/1.0 (+https://github.com/mat89500/bookmarks-tools)")
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return gitHubRepo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
			reset := resp.Header.Get("X-RateLimit-Reset")
			return gitHubRepo{}, fmt.Errorf("GitHub API rate limit atteint (reset: %s) — définir GITHUB_TOKEN pour augmenter la limite", reset)
		}
	}
	if resp.StatusCode != 200 {
		return gitHubRepo{}, fmt.Errorf("HTTP %d pour %s/%s", resp.StatusCode, owner, repo)
	}

	var ghRepo gitHubRepo
	if err := json.NewDecoder(resp.Body).Decode(&ghRepo); err != nil {
		return gitHubRepo{}, err
	}
	return ghRepo, nil
}

func fetchHTML(rawURL string, client *http.Client) (name, desc, canonURL string, err error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("User-Agent", "bm-fetch/1.0 (+https://github.com/mat89500/bookmarks-tools)")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	name, desc, canonURL = parseMeta(resp.Body)
	return name, desc, canonURL, nil
}

// parseMeta extracts og:title, og:description, og:url (and <title>, meta description) from HTML.
func parseMeta(r io.Reader) (title, description, ogURL string) {
	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		tok := z.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			switch tok.Data {
			case "title":
				if tt == html.StartTagToken && title == "" {
					if z.Next() == html.TextToken {
						title = strings.TrimSpace(z.Token().Data)
					}
				}
			case "meta":
				attrs := fetchTokenAttrs(tok)
				prop := strings.ToLower(attrs["property"])
				name := strings.ToLower(attrs["name"])
				content := attrs["content"]
				switch prop {
				case "og:title":
					if title == "" {
						title = strings.TrimSpace(content)
					}
				case "og:description":
					if description == "" {
						description = strings.TrimSpace(content)
					}
				case "og:url":
					if ogURL == "" {
						ogURL = strings.TrimSpace(content)
					}
				}
				if name == "description" && description == "" {
					description = strings.TrimSpace(content)
				}
			case "body":
				return
			}
		}
	}
	return
}

func fetchTokenAttrs(tok html.Token) map[string]string {
	m := make(map[string]string, len(tok.Attr))
	for _, a := range tok.Attr {
		m[strings.ToLower(a.Key)] = a.Val
	}
	return m
}

func checkDuplicate(rawURL string, domains map[string]string) (bool, string) {
	// Exact match first
	if stored, ok := domains[normURLKey(rawURL)]; ok {
		return true, stored
	}
	// For GitHub URLs, also check without /tree/... path suffixes
	owner, repo, isGH := normalizeGitHubURL(rawURL)
	if isGH {
		canonical := fmt.Sprintf("github.com/%s/%s", strings.ToLower(owner), strings.ToLower(repo))
		if stored, ok := domains[canonical]; ok {
			return true, stored
		}
	}
	return false, ""
}

func writeJSON(path string, entries []MetaEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
