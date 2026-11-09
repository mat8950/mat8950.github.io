// bm-api.go — Génère les données structurées du site pour les IA et les outils, depuis bookmarks.csv :
//
//	api/apps.json        catalogue complet (un objet par outil, permalien, licence, famille de licence, activité…)
//	api/categories.json  arborescence catégories → sous-catégories, avec chemins et compteurs
//	llms.txt             guide lisible par une IA : à quoi sert le site, comment chercher, schéma des données
//
// Les slugs et familles de licence suivent exactement les règles de script.js (slugify, buildRoutes,
// licenseFamily) : un permalien de l'API ouvre la bonne fiche sur le site. Garder les deux alignés.
//
// Run:  go run ./scripts/bm-api.go            (relancé aussi par le déploiement GitHub Pages)
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	apiCSV   = "bookmarks.csv"
	apiSite  = "https://mat8950.github.io"
	apiDir   = "api"
	llmsPath = "llms.txt"
)

var accents = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'œ': "oe", 'æ': "ae", 'ß': "ss",
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify : identique à slugify() de script.js
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r < 128:
			b.WriteRune(r)
		case accents[r] != "":
			b.WriteString(accents[r])
		default:
			b.WriteByte(' ')
		}
	}
	out := strings.Trim(nonAlnum.ReplaceAllString(b.String(), "-"), "-")
	if out == "" {
		return "x"
	}
	return out
}

var (
	reSrcAvail  = regexp.MustCompile(`busl|sspl|elastic|fsl-|polyform|commons-clause|-nc\b|-nc-|\bnc-|sul\b|community|modifi|rsal|openrail|source-available|-licen[cs]e$|^aucune$`)
	reCopyleft  = regexp.MustCompile(`^(a|l)?gpl|^mpl|^epl|^eupl|^cc-by-sa|^osl|^cddl|^cecill`)
	rePermissiv = regexp.MustCompile(`^(mit|apache|bsd|isc|unlicense|cc0|0bsd|zlib|bsl-1\.0|postgresql|wtfpl|cc-by-\d|ofl|lppl|python|artistic|x11|ncsa|upl)`)
	reOrAnd     = regexp.MustCompile(`\s+(?:or|and)\s+`)
)

var famRank = []string{"libre-permissive", "libre-copyleft", "source-available", "inconnue"}

func singleFamily(l string) string {
	switch {
	case reSrcAvail.MatchString(l):
		return "source-available"
	case reCopyleft.MatchString(l):
		return "libre-copyleft"
	case rePermissiv.MatchString(l):
		return "libre-permissive"
	}
	return "inconnue"
}

// licenseFamily : identique à licenseFamily() de script.js
func licenseFamily(license string) string {
	l := strings.ToLower(strings.TrimSpace(license))
	switch {
	case l == "" || l == "noassertion":
		return "inconnue"
	case strings.HasPrefix(l, "propri"):
		return "proprietaire"
	case l == "open-core":
		return "open-core"
	case strings.HasSuffix(l, "+ee"):
		// open-core seulement si le cœur est libre (n8n « SUL+EE » reste source disponible)
		if strings.HasPrefix(licenseFamily(strings.TrimSuffix(l, "+ee")), "libre") {
			return "open-core"
		}
		return "source-available"
	case strings.Contains(l, "+"):
		return "source-available"
	}
	isOr := strings.Contains(l, " or ")
	best := -1
	for _, part := range reOrAnd.Split(l, -1) {
		r := 0
		for i, f := range famRank {
			if f == singleFamily(part) {
				r = i
			}
		}
		if best == -1 || (isOr && r < best) || (!isOr && r > best) {
			best = r
		}
	}
	return famRank[best]
}

var (
	reEESuffix  = regexp.MustCompile(`(?i)\+EE$`)
	reNoPrecise = regexp.MustCompile(`(?i)^(aucune|noassertion|propri|open-core$)`)
	reOrAndCase = regexp.MustCompile(`(?i)\s+(?:or|and)\s+`)
)

// licenseParts : licences précises d'une valeur (même règle que licenseParts() de script.js).
// « AGPL-3.0 OR SSPL-1.0 » → [AGPL-3.0 SSPL-1.0] ; « MIT+EE » → [MIT] ; « MIT+Commons-Clause » reste un tout.
func licenseParts(license string) []string {
	l := reEESuffix.ReplaceAllString(strings.TrimSpace(license), "")
	if l == "" || reNoPrecise.MatchString(l) {
		return nil
	}
	if strings.Contains(l, "+") {
		return []string{l}
	}
	var out []string
	for _, p := range reOrAndCase.Split(l, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var familyOrder = []string{"libre-permissive", "libre-copyleft", "open-core", "source-available", "proprietaire", "inconnue"}

type licCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// libre % = (permissive + copyleft) / total
func librePercent(fam map[string]int, total int) int {
	if total == 0 {
		return 0
	}
	return (fam["libre-permissive"] + fam["libre-copyleft"]) * 100 / total
}

var familyLabels = map[string]string{
	"libre-permissive": "Logiciel libre, licence permissive (MIT, Apache-2.0, BSD…)",
	"libre-copyleft":   "Logiciel libre, licence copyleft (GPL, AGPL, MPL…)",
	"open-core":        "Open-core : cœur sous licence libre, modules entreprise sous licence commerciale (valeur « X+EE »)",
	"source-available": "Code consultable mais non libre (BUSL, SSPL, Elastic, PolyForm, Commons Clause, CC-NC, ou dépôt sans licence)",
	"proprietaire":     "Propriétaire / SaaS fermé",
	"inconnue":         "Licence non déterminée",
}

type App struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Permalink     string   `json:"permalink"`
	Category      string   `json:"category"`
	Subcategory   string   `json:"subcategory"`
	CategoryPath  string   `json:"category_path"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	License       string   `json:"license,omitempty"`
	LicenseFamily string   `json:"license_family"`
	Repo          string   `json:"repo,omitempty"`
	Stars         *int     `json:"stars,omitempty"`
	PushedAt      string   `json:"pushed_at,omitempty"`
	Archived      bool     `json:"archived,omitempty"`
	Active        *bool    `json:"active,omitempty"`
	LinkError     string   `json:"link_error,omitempty"`
	AddedAt       string   `json:"added_at,omitempty"`
}

type Sub struct {
	Name         string         `json:"name"`
	Path         string         `json:"path"`
	URL          string         `json:"url"`
	Count        int            `json:"count"`
	LibrePercent int            `json:"libre_percent"`
	Licenses     map[string]int `json:"licenses"` // famille → nombre d'outils
}

type Cat struct {
	Name          string         `json:"name"`
	Path          string         `json:"path"`
	URL           string         `json:"url"`
	Count         int            `json:"count"`
	LibrePercent  int            `json:"libre_percent"`
	Licenses      map[string]int `json:"licenses"` // famille → nombre d'outils
	Subcategories []*Sub         `json:"subcategories"`
}

func main() {
	f, err := os.Open(apiCSV)
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
	idx := map[string]int{}
	for i, h := range all[0] {
		idx[h] = i
	}
	get := func(row []string, k string) string {
		if i, ok := idx[k]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	now := time.Now().UTC()
	activeCut := now.AddDate(-1, 0, 0).Format("2006-01-02")
	var apps []*App
	groups := map[string][]*App{}
	cats := map[string]*Cat{}
	subs := map[string]*Sub{}
	var catOrder []string

	for _, row := range all[1:] {
		name, u, cat, sub := get(row, "name"), get(row, "url"), get(row, "category"), get(row, "subcategory")
		if name == "" || u == "" || cat == "" {
			continue
		}
		catPath := "/" + slugify(cat)
		folder := catPath
		if sub != "" {
			folder += "/" + slugify(sub)
		}
		a := &App{
			Name: name, URL: u, Category: cat, Subcategory: sub,
			CategoryPath:  folder + "/",
			Description:   get(row, "description"),
			Tags:          []string{},
			License:       get(row, "license"),
			LicenseFamily: licenseFamily(get(row, "license")),
			Repo:          get(row, "repo"),
			PushedAt:      get(row, "pushed_at"),
			Archived:      get(row, "archived") == "true",
			LinkError:     get(row, "error"),
		}
		if a.Repo == "" && strings.HasPrefix(u, "https://github.com/") && strings.Count(strings.Trim(strings.TrimPrefix(u, "https://github.com/"), "/"), "/") >= 1 {
			a.Repo = u
		}
		for _, t := range strings.Split(get(row, "tags"), "|") {
			if t = strings.TrimSpace(t); t != "" {
				a.Tags = append(a.Tags, t)
			}
		}
		if s, err := strconv.Atoi(get(row, "stars")); err == nil {
			a.Stars = &s
		}
		if a.PushedAt != "" {
			act := !a.Archived && a.PushedAt >= activeCut
			a.Active = &act
		}
		if ts, err := strconv.ParseInt(get(row, "date_added"), 10, 64); err == nil && ts > 0 {
			a.AddedAt = time.Unix(ts, 0).UTC().Format("2006-01-02")
		}
		key := folder + "/" + slugify(name)
		groups[key] = append(groups[key], a)
		apps = append(apps, a)

		c := cats[cat]
		if c == nil {
			c = &Cat{Name: cat, Path: catPath + "/", URL: apiSite + catPath + "/", Licenses: map[string]int{}}
			cats[cat] = c
			catOrder = append(catOrder, cat)
		}
		c.Count++
		c.Licenses[a.LicenseFamily]++
		if sub != "" {
			s := subs[cat+" > "+sub]
			if s == nil {
				s = &Sub{Name: sub, Path: folder + "/", URL: apiSite + folder + "/", Licenses: map[string]int{}}
				subs[cat+" > "+sub] = s
				c.Subcategories = append(c.Subcategories, s)
			}
			s.Count++
			s.Licenses[a.LicenseFamily]++
		}
	}

	// Slugs uniques : doublons dans une même sous-catégorie triés par URL → nom, nom-2, nom-3 (comme buildRoutes)
	for key, list := range groups {
		sort.SliceStable(list, func(i, j int) bool { return list[i].URL < list[j].URL })
		for i, a := range list {
			p := key
			if i > 0 {
				p = fmt.Sprintf("%s-%d", key, i+1)
			}
			a.ID = strings.TrimPrefix(p, "/")
			a.Permalink = apiSite + p
		}
	}
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })

	// Catégories : par nombre d'outils décroissant, sous-catégories alphabétiques (comme le site)
	var catList []*Cat
	for _, name := range catOrder {
		c := cats[name]
		sort.Slice(c.Subcategories, func(i, j int) bool { return c.Subcategories[i].Name < c.Subcategories[j].Name })
		catList = append(catList, c)
	}
	sort.SliceStable(catList, func(i, j int) bool { return catList[i].Count > catList[j].Count })

	famCount := map[string]int{}
	idCount := map[string]*licCount{}
	for _, a := range apps {
		famCount[a.LicenseFamily]++
		for _, id := range licenseParts(a.License) {
			k := strings.ToLower(id)
			if idCount[k] == nil {
				idCount[k] = &licCount{ID: id}
			}
			idCount[k].Count++
		}
	}
	var topLicenses []licCount
	for _, c := range idCount {
		topLicenses = append(topLicenses, *c)
	}
	sort.Slice(topLicenses, func(i, j int) bool {
		if topLicenses[i].Count != topLicenses[j].Count {
			return topLicenses[i].Count > topLicenses[j].Count
		}
		return topLicenses[i].ID < topLicenses[j].ID
	})
	for _, c := range catList {
		c.LibrePercent = librePercent(c.Licenses, c.Count)
		for _, s := range c.Subcategories {
			s.LibrePercent = librePercent(s.Licenses, s.Count)
		}
	}
	licenseStats := map[string]any{
		"families":      famCount,
		"libre_percent": librePercent(famCount, len(apps)),
		"licenses":      topLicenses, // licences précises (composants de « X OR Y », sans « +EE »), par fréquence
	}

	os.MkdirAll(apiDir, 0o755)
	writeJSON(apiDir+"/apps.json", map[string]any{
		"site":             apiSite,
		"generated_at":     now.Format(time.RFC3339),
		"count":            len(apps),
		"license_families": familyLabels,
		"license_stats":    licenseStats,
		"apps":             apps,
	})
	writeJSON(apiDir+"/categories.json", map[string]any{
		"site":          apiSite,
		"generated_at":  now.Format(time.RFC3339),
		"count":         len(apps),
		"license_stats": licenseStats,
		"categories":    catList,
	})
	writeLLMs(catList, famCount, topLicenses, len(apps), now)
	fmt.Printf("[OK] %d outils, %d catégories → api/apps.json, api/categories.json, llms.txt\n", len(apps), len(catList))
}

func writeJSON(path string, v any) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // « & » et « > » lisibles tels quels (catégories)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

func writeLLMs(cats []*Cat, fam map[string]int, top []licCount, total int, now time.Time) {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# Marque-pages tech\n\n")
	w("> Catalogue curé de %d outils techniques (DevOps, IA, cybersécurité, data, développement, self-hosting), ", total)
	w("en priorité open-source et auto-hébergeables. Chaque outil a une catégorie, une description en français, des tags, ")
	w("sa licence et, pour les dépôts GitHub, ses étoiles et sa date de dernière activité.\n\n")
	w("Mis à jour le %s. Site : %s/\n\n", now.Format("2006-01-02"), apiSite)

	w("## Données pour les IA\n\n")
	w("- [Catalogue complet (JSON)](%s/api/apps.json) : tableau `apps`, un objet par outil. Champs : ", apiSite)
	w("`id` (catégorie/sous-catégorie/outil), `name`, `url` (site ou dépôt officiel), `permalink` (fiche sur ce site), ")
	w("`category`, `subcategory`, `category_path`, `description` (FR), `tags`, `license` (SPDX ou valeur manuelle), ")
	w("`license_family`, `repo`, `stars`, `pushed_at` (AAAA-MM-JJ), `archived`, `active` (push < 12 mois), `link_error` (lien mort à la dernière vérification, ex. « HTTP 404 »), `added_at`.\n")
	w("- [Arborescence des catégories (JSON)](%s/api/categories.json) : catégories, sous-catégories, chemins, compteurs.\n", apiSite)
	w("- Les deux fichiers sont statiques : télécharge-les une fois puis filtre localement (pas de recherche côté serveur).\n")
	w("- Serveur MCP (recherche par outils) : `go run ./scripts/bm-mcp.go` dans le dépôt du site, outils `search_apps`, `get_app`, `list_categories`.\n\n")

	w("## Adresses du site\n\n")
	w("- Sous-catégorie : `%s/<categorie>/<sous-categorie>/` (valeur `category_path`).\n", apiSite)
	w("- Fiche d'un outil : `%s/<categorie>/<sous-categorie>/<outil>` (valeur `permalink`).\n", apiSite)
	w("- Recherche : `%s/?q=<mots>` ; liens morts : `%s/?etat=erreur` (combinables avec un chemin).\n", apiSite, apiSite)
	w("- Filtre de licence : `%s/?licence=<famille>` (ex. `?licence=libre`) ou licence précise `?licence=<SPDX>` (ex. `?licence=AGPL-3.0`).\n\n", apiSite)

	w("## Licences\n\n")
	w("%d %% du catalogue est 100 %% libre. Ce que chaque famille autorise (usage interne, modification, SaaS) : [guide des licences](%s/licences.html).\n\n", librePercent(fam, total), apiSite)
	w("Répartition par famille (`license_family`, filtre `?licence=`) :\n\n")
	w("- `libre` (filtre seulement) : regroupe `libre-permissive` et `libre-copyleft` = 100 %% logiciel libre.\n")
	for _, k := range familyOrder {
		pct := 0
		if total > 0 {
			pct = fam[k] * 100 / total
		}
		w("- `%s` (%d, %d %%) : %s\n", k, fam[k], pct, familyLabels[k])
	}
	var ids []string
	for i, c := range top {
		if i == 15 {
			break
		}
		ids = append(ids, fmt.Sprintf("%s (%d)", c.ID, c.Count))
	}
	w("\nLicences précises les plus fréquentes : %s. Liste complète : `license_stats` dans apps.json.\n", strings.Join(ids, ", "))
	w("\n## Catégories\n\n")
	w("Entre parenthèses : nombre d'outils, part 100 %% libre (`libre_percent` dans categories.json).\n\n")
	for _, c := range cats {
		w("- [%s](%s) (%d, %d %% libre)\n", c.Name, c.URL, c.Count, c.LibrePercent)
		for _, s := range c.Subcategories {
			w("  - [%s](%s) (%d, %d %% libre)\n", s.Name, s.URL, s.Count, s.LibrePercent)
		}
	}
	if err := os.WriteFile(llmsPath, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}
