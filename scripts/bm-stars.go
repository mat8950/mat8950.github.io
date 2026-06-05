// bm-stars.go — Enrichit bookmarks.csv depuis l'API GitHub (un seul appel par dépôt) :
//   - stars      : stargazers_count
//   - pushed_at  : date du dernier push (YYYY-MM-DD)
//   - archived   : "true" si le dépôt est archivé (signal d'abandon fort)
//   - tags       : dérivés des topics GitHub (+ langage) UNIQUEMENT si la colonne est vide
//                  (les tags curés manuellement ou par bm-enrich ne sont jamais écrasés)
// Les colonnes manquantes sont créées automatiquement.
//
// Run:     go run ./scripts/bm-stars.go
// Token:   GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-stars.go   (recommandé — 5000 req/h)
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const starsCSVPath = "bookmarks.csv"

type ghRepoStats struct {
	Stars    int      `json:"stargazers_count"`
	PushedAt string   `json:"pushed_at"`
	Topics   []string `json:"topics"`
	Language string   `json:"language"`
	Archived bool     `json:"archived"`
}

// tagsFromGitHub construit une liste de tags pipe-séparée depuis les topics
// GitHub (+ le langage principal), dédupliquée et limitée à 8 entrées.
func tagsFromGitHub(s ghRepoStats) string {
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] || len(out) >= 8 {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, t := range s.Topics {
		add(t)
	}
	add(s.Language)
	return strings.Join(out, "|")
}

func main() {
	f, err := os.Open(starsCSVPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // tolère un nombre de colonnes variable
	headers, err := reader.Read()
	if err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "[ERROR] lecture headers: %v\n", err)
		os.Exit(1)
	}
	var rows [][]string
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		rows = append(rows, row)
	}
	f.Close()

	// Indices des colonnes, création si absentes
	colIdx := func(name string) int {
		for i, h := range headers {
			if h == name {
				return i
			}
		}
		headers = append(headers, name)
		return len(headers) - 1
	}
	urlIdx := colIdx("url")
	tagsIdx := colIdx("tags")
	starsIdx := colIdx("stars")
	pushedIdx := colIdx("pushed_at")
	archivedIdx := colIdx("archived")
	width := len(headers)

	// Normalise la largeur de chaque ligne
	pad := func(row []string) []string {
		for len(row) < width {
			row = append(row, "")
		}
		return row
	}

	client := &http.Client{Timeout: 15 * time.Second}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("[!] GITHUB_TOKEN non défini — limite 60 req/h. Recommandé pour 100+ dépôts.")
	}

	ghCount, okCount, errCount := 0, 0, 0
	for i := range rows {
		rows[i] = pad(rows[i])
		rawURL := rows[i][urlIdx]
		owner, repo, ok := parseGitHubRepo(rawURL)
		if !ok {
			continue
		}
		ghCount++
		stats, err := fetchRepoStats(owner, repo, token, client)
		if err != nil {
			errCount++
			fmt.Fprintf(os.Stderr, "[WARN] %s/%s : %v\n", owner, repo, err)
			continue
		}
		rows[i][starsIdx] = strconv.Itoa(stats.Stars)
		if len(stats.PushedAt) >= 10 {
			rows[i][pushedIdx] = stats.PushedAt[:10] // YYYY-MM-DD
		}
		if stats.Archived {
			rows[i][archivedIdx] = "true"
		} else {
			rows[i][archivedIdx] = ""
		}
		// Tags : remplir depuis les topics seulement si la colonne est vide
		tagsFilled := ""
		if strings.TrimSpace(rows[i][tagsIdx]) == "" {
			if t := tagsFromGitHub(stats); t != "" {
				rows[i][tagsIdx] = t
				tagsFilled = " +tags:" + t
			}
		}
		okCount++
		flag := ""
		if stats.Archived {
			flag = " 🗄 archivé"
		}
		fmt.Printf("[%d] %s/%s → ⭐ %d (push %s)%s%s\n", okCount, owner, repo, stats.Stars, rows[i][pushedIdx], flag, tagsFilled)
	}

	// Réécriture
	out, err := os.Create(starsCSVPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
	defer out.Close()
	writer := csv.NewWriter(out)
	writer.Write(headers)
	for _, row := range rows {
		writer.Write(pad(row))
	}
	writer.Flush()

	fmt.Printf("\n[OK] %d dépôts GitHub — %d enrichis, %d erreurs\n", ghCount, okCount, errCount)
}

// parseGitHubRepo extrait owner/repo d'une URL github.com (ignore /tree/..., etc.)
func parseGitHubRepo(rawURL string) (owner, repo string, ok bool) {
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

func fetchRepoStats(owner, repo, token string, client *http.Client) (ghRepoStats, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return ghRepoStats{}, err
	}
	req.Header.Set("User-Agent", "bm-stars/1.0 (+https://github.com/mat89500/bookmarks-tools)")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ghRepoStats{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return ghRepoStats{}, fmt.Errorf("rate limit atteint — définir GITHUB_TOKEN")
	}
	if resp.StatusCode == 404 {
		return ghRepoStats{}, fmt.Errorf("dépôt introuvable (404)")
	}
	if resp.StatusCode != 200 {
		return ghRepoStats{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var stats ghRepoStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return ghRepoStats{}, err
	}
	return stats, nil
}
