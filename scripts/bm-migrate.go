// bm-migrate.go — Génère/met à jour bookmarks.csv depuis l'historique git + bookmarks.html
// Port Go de scripts/migrate.py
// Run: go run ./scripts/bm-migrate.go  (depuis la racine du dépôt)
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	migrateHTMLFile = "bookmarks.html"
	migrateCSVFile  = "bookmarks.csv"
)

var csvHeaders = []string{"url", "name", "category", "subcategory", "description", "tags", "date_added", "stars", "pushed_at", "archived"}

type bmBookmark struct {
	URL         string
	Name        string
	Category    string
	Subcategory string
	AddDateHTML int64
}

type bmCommit struct {
	Hash string
	Date time.Time
}

type bmCSVRow struct {
	URL         string
	Name        string
	Category    string
	Subcategory string
	Description string
	Tags        string
	DateAdded   int64
	Stars       string // GitHub stargazers (rempli par bm-stars.go)
	PushedAt    string // GitHub dernier push ISO8601 (rempli par bm-stars.go)
	Archived    string // "true" si dépôt archivé (rempli par bm-stars.go)
}

var addedHrefRe = regexp.MustCompile(`(?m)^\+[^+].*<A HREF="([^"]+)"`)
var allHrefRe = regexp.MustCompile(`<A HREF="([^"]+)"`)

func main() {
	repoDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[→] Dépôt   : %s\n", repoDir)
	fmt.Printf("[→] HTML    : %s\n", migrateHTMLFile)
	fmt.Printf("[→] CSV     : %s\n\n", migrateCSVFile)

	existing, err := migrateLoadCSV(migrateCSVFile)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[ERROR] load CSV: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[i] %d entrées déjà dans le CSV\n", len(existing))

	htmlBookmarks, err := parseBookmarksHTML(migrateHTMLFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] parse HTML: %v\n", err)
		os.Exit(1)
	}
	htmlMeta := make(map[string]bmBookmark, len(htmlBookmarks))
	for _, b := range htmlBookmarks {
		htmlMeta[b.URL] = b
	}
	fmt.Printf("[i] %d URLs trouvées dans %s\n", len(htmlMeta), migrateHTMLFile)

	commits, err := getGitCommits(repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] git log: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[i] %d commits touchant %s\n", len(commits), migrateHTMLFile)

	addedCount := 0
	skippedCount := 0
	for _, commit := range commits {
		dateTS := commit.Date.UTC().Unix()
		urls, err := getURLsAddedInCommit(repoDir, commit.Hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] commit %s: %v\n", commit.Hash[:8], err)
			continue
		}
		for _, u := range urls {
			if u == "" || strings.HasPrefix(u, "#") {
				continue
			}
			if _, found := existing[u]; found {
				skippedCount++
				continue
			}
			meta := htmlMeta[u]
			existing[u] = bmCSVRow{
				URL:         u,
				Name:        meta.Name,
				Category:    meta.Category,
				Subcategory: meta.Subcategory,
				DateAdded:   dateTS,
			}
			addedCount++
		}
	}
	fmt.Printf("[i] %d nouvelles URLs datées depuis git history\n", addedCount)
	fmt.Printf("[i] %d URLs déjà présentes → non modifiées\n", skippedCount)

	fallbackCount := 0
	for u, meta := range htmlMeta {
		if _, found := existing[u]; !found {
			existing[u] = bmCSVRow{
				URL:         u,
				Name:        meta.Name,
				Category:    meta.Category,
				Subcategory: meta.Subcategory,
				DateAdded:   meta.AddDateHTML,
			}
			fallbackCount++
		}
	}
	if fallbackCount > 0 {
		fmt.Printf("[i] %d URLs avec fallback ADD_DATE HTML\n", fallbackCount)
	}

	fmt.Println()
	if err := migrateSaveCSV(migrateCSVFile, existing); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] save CSV: %v\n", err)
		os.Exit(1)
	}

	dated := 0
	for _, r := range existing {
		if r.DateAdded > 0 {
			dated++
		}
	}
	fmt.Printf("\n=== Résumé ===\n")
	fmt.Printf("Total dans CSV : %d\n", len(existing))
	fmt.Printf("Avec date      : %d\n", dated)
	fmt.Printf("Sans date      : %d\n", len(existing)-dated)
}

func parseBookmarksHTML(path string) ([]bmBookmark, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type folderEntry struct{ name string }
	var stack []folderEntry
	var bookmarks []bmBookmark
	var inA bool
	var afterH3 bool
	var currentA *bmBookmark

	z := html.NewTokenizer(f)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		tok := z.Token()

		switch tt {
		case html.StartTagToken:
			switch tok.Data {
			case "h3":
				stack = append(stack, folderEntry{})
				afterH3 = true
				inA = false
			case "a":
				attrs := htmlTokenAttrs(tok)
				addDate, _ := strconv.ParseInt(attrs["add_date"], 10, 64)
				currentA = &bmBookmark{
					URL:         attrs["href"],
					AddDateHTML: addDate,
				}
				inA = true
				afterH3 = false
			}
		case html.EndTagToken:
			switch tok.Data {
			case "a":
				inA = false
				if currentA != nil && currentA.URL != "" {
					if len(stack) > 0 {
						currentA.Category = stack[0].name
					}
					if len(stack) > 1 {
						currentA.Subcategory = stack[1].name
					}
					bookmarks = append(bookmarks, *currentA)
				}
				currentA = nil
			case "dl":
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}
		case html.TextToken:
			text := strings.TrimSpace(tok.Data)
			if text == "" {
				continue
			}
			if inA && currentA != nil {
				currentA.Name += text
			} else if afterH3 && len(stack) > 0 {
				stack[len(stack)-1].name = text
				afterH3 = false
			}
		}
	}
	return bookmarks, nil
}

func htmlTokenAttrs(tok html.Token) map[string]string {
	m := make(map[string]string, len(tok.Attr))
	for _, a := range tok.Attr {
		m[strings.ToLower(a.Key)] = a.Val
	}
	return m
}

func getGitCommits(repoDir string) ([]bmCommit, error) {
	cmd := exec.Command("git", "log", "--reverse", "--pretty=format:%H\t%ai", "--", migrateHTMLFile)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var commits []bmCommit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		// git --ai format: "2006-01-02 15:04:05 -0700"
		dt, err := time.Parse("2006-01-02 15:04:05 -0700", parts[1])
		if err != nil {
			continue
		}
		commits = append(commits, bmCommit{Hash: parts[0], Date: dt})
	}
	return commits, nil
}

func getURLsAddedInCommit(repoDir, hash string) ([]string, error) {
	cmd := exec.Command("git", "diff", hash+"^", hash, "--", migrateHTMLFile)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		// Premier commit sans parent — utiliser git show
		cmd2 := exec.Command("git", "show", hash+":"+migrateHTMLFile)
		cmd2.Dir = repoDir
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return nil, err2
		}
		matches := allHrefRe.FindAllSubmatch(out2, -1)
		urls := make([]string, 0, len(matches))
		for _, m := range matches {
			urls = append(urls, string(m[1]))
		}
		return urls, nil
	}
	matches := addedHrefRe.FindAllSubmatch(out, -1)
	urls := make([]string, 0, len(matches))
	for _, m := range matches {
		urls = append(urls, string(m[1]))
	}
	return urls, nil
}

func migrateLoadCSV(path string) (map[string]bmCSVRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return map[string]bmCSVRow{}, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	headers, err := reader.Read()
	if err != nil {
		return map[string]bmCSVRow{}, err
	}
	idx := make(map[string]int)
	for i, h := range headers {
		idx[h] = i
	}
	get := func(row []string, key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return row[i]
	}
	data := make(map[string]bmCSVRow)
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		u := get(row, "url")
		if u == "" {
			continue
		}
		dateAdded, _ := strconv.ParseInt(get(row, "date_added"), 10, 64)
		data[u] = bmCSVRow{
			URL:         u,
			Name:        get(row, "name"),
			Category:    get(row, "category"),
			Subcategory: get(row, "subcategory"),
			Description: get(row, "description"),
			Tags:        get(row, "tags"),
			DateAdded:   dateAdded,
			Stars:       get(row, "stars"),
			PushedAt:    get(row, "pushed_at"),
			Archived:    get(row, "archived"),
		}
	}
	return data, nil
}

func migrateSaveCSV(path string, data map[string]bmCSVRow) error {
	rows := make([]bmCSVRow, 0, len(data))
	for _, r := range data {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].DateAdded < rows[j].DateAdded
	})

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	if err := writer.Write(csvHeaders); err != nil {
		return err
	}
	for _, r := range rows {
		writer.Write([]string{
			r.URL, r.Name, r.Category, r.Subcategory,
			r.Description, r.Tags, strconv.FormatInt(r.DateAdded, 10),
			r.Stars, r.PushedAt, r.Archived,
		})
	}
	writer.Flush()
	fmt.Printf("[OK] %d entrées écrites dans %s\n", len(rows), path)
	return nil
}
