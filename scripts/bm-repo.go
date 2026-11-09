// bm-repo.go — Retrouve le dépôt GitHub des entrées qui pointent vers un site officiel
// (colonne `repo` de bookmarks.csv), pour que bm-stars.go puisse ensuite récupérer
// stars, activité, archivage et licence de ces outils aussi.
//
// Ne remplit `repo` que si le lien est sûr :
//   - site hébergé sur owner.github.io/repo → owner/repo (s'il existe) ;
//   - ou dépôt cité par la page dont le champ « homepage » GitHub pointe vers le même domaine ;
//   - ou dépôt le plus cité par la page dont le nom correspond au nom de l'outil.
//
// Sinon la colonne reste vide (rien n'est deviné). Les valeurs déjà présentes ne sont jamais modifiées.
//
// Run:     GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-repo.go
// Ensuite: GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-stars.go
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const repoCSVPath = "bookmarks.csv"

var ghLinkRe = regexp.MustCompile(`(?i)github\.com/([a-z0-9][a-z0-9-]{0,38})/([a-z0-9_.-]{1,100})`)

// Premiers segments de chemin GitHub qui ne sont pas des comptes.
var ghReserved = map[string]bool{
	"sponsors": true, "features": true, "topics": true, "orgs": true, "about": true, "pricing": true,
	"login": true, "join": true, "marketplace": true, "apps": true, "settings": true, "collections": true,
	"enterprise": true, "security": true, "site": true, "readme": true, "customer-stories": true,
	"trending": true, "explore": true, "notifications": true, "contact": true, "team": true,
	"resources": true, "solutions": true, "events": true, "users": true, "search": true, "github": true,
	"advisories": true, "codespaces": true, "issues": true, "pulls": true, "new": true, "account": true,
}

type repoInfo struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Homepage string `json:"homepage"`
	Fork     bool   `json:"fork"`
}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("[!] GITHUB_TOKEN non défini — limite 60 req/h, insuffisant pour vérifier les dépôts.")
	}
	f, err := os.Open(repoCSVPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	f.Close()
	if err != nil || len(all) < 2 {
		fmt.Fprintln(os.Stderr, "[ERROR] lecture CSV:", err)
		os.Exit(1)
	}
	headers := all[0]
	col := func(name string) int {
		for i, h := range headers {
			if h == name {
				return i
			}
		}
		headers = append(headers, name)
		return len(headers) - 1
	}
	urlI, nameI, repoI := col("url"), col("name"), col("repo")
	rows := all[1:]
	for i := range rows {
		for len(rows[i]) < len(headers) {
			rows[i] = append(rows[i], "")
		}
	}

	client := &http.Client{Timeout: 12 * time.Second}
	api := func(fullName string) (*repoInfo, error) {
		req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+fullName, nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "bm-repo/1.0")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var ri repoInfo
		return &ri, json.NewDecoder(resp.Body).Decode(&ri)
	}

	jobs := make(chan int)
	var mu sync.Mutex
	found, checked := 0, 0
	var report []string
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				full, why := resolveRepo(rows[i][urlI], rows[i][nameI], client, api)
				mu.Lock()
				checked++
				if full != "" {
					rows[i][repoI] = "https://github.com/" + full
					found++
					report = append(report, fmt.Sprintf("  %-40s → %s (%s)", rows[i][nameI], full, why))
				}
				mu.Unlock()
			}
		}()
	}
	for i, row := range rows {
		u, err := url.Parse(row[urlI])
		if err != nil || u.Host == "" || strings.TrimPrefix(u.Host, "www.") == "github.com" || strings.TrimSpace(row[repoI]) != "" || row[nameI] == "" {
			continue
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	sort.Strings(report)
	for _, l := range report {
		fmt.Println(l)
	}

	out, err := os.Create(repoCSVPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Write(headers)
	w.WriteAll(rows)
	fmt.Printf("\n[OK] %d sites analysés — %d dépôts trouvés (colonne repo)\n", checked, found)
}

func bareHost(h string) string { return strings.TrimPrefix(strings.ToLower(h), "www.") }

// sameSite : même hôte, ou l'un est sous-domaine de l'autre (docs.x.io ≈ x.io).
func sameSite(a, b string) bool {
	a, b = bareHost(a), bareHost(b)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// docsLike : dépôts de site / documentation, pas le code de l'outil.
var docsLikeRe = regexp.MustCompile(`(?i)(\.github\.io|[-_.]?docs?|[-_]web(site)?|[-_]site|[-_]homepage|[-_]landing|[-_]artwork|^website|^homepage|^site)$`)

// Suffixes d'un dépôt satellite (SDK, client, site, données…) : pas le dépôt principal de l'outil.
var satelliteRe = regexp.MustCompile(`(?i)(sdk|go|golang|py|python|js|node|ts|rs|rust|java|swift|kotlin|dart|php|ruby|dotnet|csharp|client|plugin|odbc|jdbc|drupal|wordpress|announce|data|www|org|com|io|net|code|cli|action|helm|charts?|operator|docker|examples?|tests?|benchmarks?)$`)

// nameScore : 3 = nom identique, 2 = nom du dépôt contenu dans celui de l'outil,
// 1 = nom de l'outil contenu dans celui du dépôt (dépôt satellite exclu), 0 = sans rapport.
func nameScore(repoName, toolName string) int {
	a, b := alnum(repoName), alnum(toolName)
	switch {
	case len(a) < 3 || len(b) < 3:
		return 0
	case a == b:
		return 3
	case strings.Contains(b, a):
		return 2
	case strings.Contains(a, b) && !satelliteRe.MatchString(strings.TrimPrefix(a, b)) && !satelliteRe.MatchString(strings.TrimSuffix(a, b)):
		return 1
	}
	return 0
}

func resolveRepo(rawURL, name string, client *http.Client, api func(string) (*repoInfo, error)) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	host := bareHost(u.Host)

	// 1. owner.github.io/repo (le dépôt owner.github.io lui-même n'est que le site : ignoré)
	if strings.HasSuffix(host, ".github.io") {
		owner := strings.TrimSuffix(host, ".github.io")
		if seg := strings.Split(strings.Trim(u.Path, "/"), "/"); seg[0] != "" && !docsLikeRe.MatchString(seg[0]) {
			if ri, err := api(owner + "/" + seg[0]); err == nil && !ri.Fork && !docsLikeRe.MatchString(ri.Name) {
				return ri.FullName, "github.io"
			}
		}
	}

	// 2. Liens github.com/owner/repo présents dans la page
	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) bm-repo/1.0")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	resp.Body.Close()
	counts := map[string]int{}
	var order []string
	for _, m := range ghLinkRe.FindAllStringSubmatch(string(body), -1) {
		owner, repo := strings.ToLower(m[1]), strings.ToLower(strings.TrimSuffix(strings.TrimRight(m[2], "."), ".git"))
		if ghReserved[owner] || repo == "" || docsLikeRe.MatchString(repo) {
			continue
		}
		k := owner + "/" + repo
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	if len(order) == 0 {
		return "", ""
	}
	sort.SliceStable(order, func(a, b int) bool { return counts[order[a]] > counts[order[b]] })
	if len(order) > 6 {
		order = order[:6]
	}
	// Le nom doit correspondre. Nom identique : accepté si homepage = ce site ou dépôt le plus cité.
	// Nom contenu : exige homepage = ce site (ou, pour un dépôt au nom plus court, le plus cité).
	best, bestScore, why := "", 0, ""
	for rank, k := range order {
		ri, err := api(k)
		if err != nil || ri.Fork || docsLikeRe.MatchString(ri.Name) || strings.Contains(strings.ToLower(ri.FullName), "docs/") {
			continue
		}
		sc := nameScore(ri.Name, name)
		hp, perr := url.Parse(ri.Homepage)
		home := perr == nil && ri.Homepage != "" && sameSite(hp.Host, host)
		ok := (sc == 3 && (home || rank == 0)) || (sc == 2 && (home || rank == 0)) || (sc == 1 && home)
		// À score égal, le nom de dépôt le plus long (le plus spécifique) l'emporte
		if ok && (sc > bestScore || (sc == bestScore && len(ri.Name) > len(best[strings.Index(best, "/")+1:]))) {
			best, bestScore = ri.FullName, sc
			why = map[bool]string{true: "homepage", false: "plus cité"}[home] + fmt.Sprintf("+nom%d", sc)
		}
	}
	if best != "" {
		return best, why
	}
	return "", ""
}
