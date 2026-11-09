// bm-api.go — Génère les données structurées du site pour les IA et les outils, depuis bookmarks.csv :
//
//	api/apps.json        catalogue complet (un objet par outil, permalien, licence, famille de licence, activité…)
//	api/categories.json  arborescence catégories → sous-catégories, avec chemins et compteurs
//	llms.txt             guide lisible par une IA : à quoi sert le site, comment chercher, schéma des données
//
// Les slugs et familles de licence suivent exactement les règles de script.js (slugify, buildRoutes,
// licenseFamily) : un permalien de l'API ouvre la bonne fiche sur le site. Garder les deux alignés.
//
// Option -pages <dossier> (déploiement) : écrit aussi une page HTML par outil (<cat>/<sous-cat>/<outil>.html),
// sous-catégorie et catégorie (…/index.html), plus sitemap.xml. Chaque page = index.html avec titre, description,
// Open Graph, JSON-LD et un résumé lisible sans JavaScript : les liens profonds répondent 200 (au lieu de passer
// par 404.html), lisibles par les moteurs et les IA ; l'application prend ensuite la main (même routage).
//
// Run:  go run ./scripts/bm-api.go                     (api/ + llms.txt)
//
//	go run ./scripts/bm-api.go -pages _site        (déploiement GitHub Pages : + pages + sitemap.xml)
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
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
	pagesDir := flag.String("pages", "", "dossier du site à publier : y écrire une page HTML par outil / catégorie + sitemap.xml")
	flag.Parse()
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
	if *pagesDir != "" {
		writePages(*pagesDir, apps, catList, now)
	}
}

// ---------- Pages statiques (liens profonds en HTTP 200) ----------

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", a...)
	os.Exit(1)
}

// Remplace exactement une occurrence (sinon index.html a changé : on échoue plutôt que publier des pages fausses)
func replaceOnce(s, old, new string) string {
	if n := strings.Count(s, old); n != 1 {
		fail("modèle index.html : %d occurrence(s) de %q (1 attendue)", n, old)
	}
	return strings.Replace(s, old, new, 1)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

var (
	reTitleTag = regexp.MustCompile(`<title>[^<]*</title>`)
	reMetaDesc = regexp.MustCompile(`<meta name="description" content="[^"]*">`)
	reOGTitle  = regexp.MustCompile(`<meta property="og:title" content="[^"]*">`)
	reOGDesc   = regexp.MustCompile(`<meta property="og:description" content="[^"]*">`)
	reSPDXID   = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)
)

type pageInfo struct {
	path, title, desc, canonical, jsonld, body string
}

func renderPage(tpl string, p pageInfo) string {
	e := html.EscapeString
	one := func(re *regexp.Regexp, s, new string) string {
		if n := len(re.FindAllString(s, -1)); n != 1 {
			fail("modèle index.html : %d occurrence(s) de %s (1 attendue)", n, re)
		}
		return re.ReplaceAllLiteralString(s, new)
	}
	out := one(reTitleTag, tpl, "<title>"+e(p.title)+"</title>")
	out = one(reMetaDesc, out, `<meta name="description" content="`+e(p.desc)+`">`)
	out = one(reOGTitle, out, `<meta property="og:title" content="`+e(p.title)+`">`)
	out = one(reOGDesc, out, `<meta property="og:description" content="`+e(p.desc)+`">`)
	out = replaceOnce(out, `<link rel="canonical" href="`+apiSite+`/">`, `<link rel="canonical" href="`+e(p.canonical)+`">`)
	out = replaceOnce(out, `<meta property="og:url" content="`+apiSite+`/">`, `<meta property="og:url" content="`+e(p.canonical)+`">`)
	out = replaceOnce(out, "</head>", `    <script type="application/ld+json">`+p.jsonld+"</script>\n</head>")
	// Résumé statique dans la grille : visible sans JavaScript, remplacé par l'application au chargement
	return replaceOnce(out, `<div id="bookmarks-grid" role="list" aria-label="Marque-pages"></div>`,
		`<div id="bookmarks-grid" role="list" aria-label="Marque-pages"><div class="static-fiche" role="listitem">`+p.body+`</div></div>`)
}

func ldJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(true) // « < » échappé : pas de fermeture de <script> possible
	enc.Encode(v)
	return strings.TrimSpace(b.String())
}

func writePages(dir string, apps []*App, cats []*Cat, now time.Time) {
	tplBytes, err := os.ReadFile("index.html")
	if err != nil {
		fail("lecture index.html : %v", err)
	}
	tpl := string(tplBytes)
	e := html.EscapeString
	reserved := map[string]bool{"api": true, "css": true, "fonts": true, "lib": true, "icons": true}
	written := 0
	write := func(rel string, content string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fail("%v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			fail("%v", err)
		}
		written++
	}
	byFolder := map[string][]*App{}
	for _, a := range apps {
		byFolder[a.CategoryPath] = append(byFolder[a.CategoryPath], a)
	}
	urls := []string{apiSite + "/", apiSite + "/licences.html", apiSite + "/mentions-legales.html"}

	for _, c := range cats {
		top := strings.Trim(c.Path, "/")
		if reserved[top] {
			fail("catégorie %q : chemin %q réservé (dossier du site)", c.Name, top)
		}
		// Page catégorie
		var b strings.Builder
		fmt.Fprintf(&b, `<h2>%s</h2><p>%d outils, %d %% sous licence libre.</p><ul>`, e(c.Name), c.Count, c.LibrePercent)
		for _, s := range c.Subcategories {
			fmt.Fprintf(&b, `<li><a href="%s">%s</a> (%d)</li>`, e(s.Path), e(s.Name), s.Count)
		}
		b.WriteString(`</ul>`)
		write(top+"/index.html", renderPage(tpl, pageInfo{
			title:     fmt.Sprintf("%s — %d outils · Marque-pages tech", c.Name, c.Count),
			desc:      truncate(fmt.Sprintf("%d outils %s triés par sous-catégorie, %d %% sous licence libre. Open-source et self-hostable en priorité.", c.Count, c.Name, c.LibrePercent), 160),
			canonical: c.URL,
			jsonld:    ldJSON(map[string]any{"@context": "https://schema.org", "@type": "CollectionPage", "name": c.Name, "url": c.URL, "isPartOf": apiSite + "/"}),
			body:      b.String(),
		}))
		urls = append(urls, c.URL)

		for _, s := range c.Subcategories {
			list := byFolder[s.Path]
			var sb strings.Builder
			var items []map[string]any
			fmt.Fprintf(&sb, `<h2>%s</h2><p><a href="%s">%s</a> · %d outils, %d %% sous licence libre.</p><ul>`, e(s.Name), e(c.Path), e(c.Name), s.Count, s.LibrePercent)
			names := []string{}
			for i, a := range list {
				lic := a.License
				if lic == "" {
					lic = "licence inconnue"
				}
				fmt.Fprintf(&sb, `<li><a href="%s">%s</a> — %s (%s)</li>`, e(strings.TrimPrefix(a.Permalink, apiSite)), e(a.Name), e(a.Description), e(lic))
				items = append(items, map[string]any{"@type": "ListItem", "position": i + 1, "name": a.Name, "url": a.Permalink})
				if i < 5 {
					names = append(names, a.Name)
				}
			}
			sb.WriteString(`</ul>`)
			write(strings.Trim(s.Path, "/")+"/index.html", renderPage(tpl, pageInfo{
				title:     fmt.Sprintf("%s (%s) — %d outils · Marque-pages tech", s.Name, c.Name, s.Count),
				desc:      truncate(fmt.Sprintf("%d outils %s : %s… %d %% sous licence libre.", s.Count, s.Name, strings.Join(names, ", "), s.LibrePercent), 160),
				canonical: s.URL,
				jsonld:    ldJSON(map[string]any{"@context": "https://schema.org", "@type": "CollectionPage", "name": s.Name + " (" + c.Name + ")", "url": s.URL, "mainEntity": map[string]any{"@type": "ItemList", "itemListElement": items}}),
				body:      sb.String(),
			}))
			urls = append(urls, s.URL)
		}
	}

	// Pages outils : <cat>/<sous-cat>/<outil>.html → servie sous /<cat>/<sous-cat>/<outil> (sans extension)
	for _, a := range apps {
		if strings.HasSuffix(a.ID, "/index") {
			fail("outil %q : slug « index » en conflit avec la page de sous-catégorie", a.Name)
		}
		lic := a.License
		if lic == "" {
			lic = "inconnue"
		}
		var b strings.Builder
		fmt.Fprintf(&b, `<h2>%s</h2><p>%s</p><ul>`, e(a.Name), e(a.Description))
		fmt.Fprintf(&b, `<li>Catégorie : <a href="%s">%s</a> › <a href="%s">%s</a></li>`,
			e("/"+slugify(a.Category)+"/"), e(a.Category), e(a.CategoryPath), e(a.Subcategory))
		fmt.Fprintf(&b, `<li>Licence : %s (%s)</li>`, e(lic), e(familyLabels[a.LicenseFamily]))
		fmt.Fprintf(&b, `<li>Site : <a href="%s" rel="noopener">%s</a></li>`, e(a.URL), e(a.URL))
		if a.Repo != "" && a.Repo != a.URL {
			fmt.Fprintf(&b, `<li>Dépôt : <a href="%s" rel="noopener">%s</a></li>`, e(a.Repo), e(a.Repo))
		}
		if a.Stars != nil {
			fmt.Fprintf(&b, `<li>Étoiles GitHub : %d`, *a.Stars)
			if a.PushedAt != "" {
				fmt.Fprintf(&b, ` · dernier push : %s`, e(a.PushedAt))
			}
			b.WriteString(`</li>`)
		}
		if len(a.Tags) > 0 {
			fmt.Fprintf(&b, `<li>Tags : %s</li>`, e(strings.Join(a.Tags, ", ")))
		}
		b.WriteString(`</ul>`)

		ld := map[string]any{
			"@context": "https://schema.org", "@type": "SoftwareApplication",
			"name": a.Name, "description": a.Description, "url": a.URL,
			"applicationCategory": a.Category + " > " + a.Subcategory,
			"mainEntityOfPage":    a.Permalink,
			"keywords":            strings.Join(a.Tags, ", "),
		}
		if a.Repo != "" {
			ld["sameAs"] = a.Repo
		}
		// Licence libre simple → URL SPDX
		if parts := licenseParts(a.License); len(parts) == 1 && strings.HasPrefix(a.LicenseFamily, "libre") && reSPDXID.MatchString(parts[0]) {
			ld["license"] = "https://spdx.org/licenses/" + parts[0] + ".html"
		}
		desc := a.Description
		if lic != "inconnue" {
			desc += " Licence " + lic + "."
		}
		write(a.ID+".html", renderPage(tpl, pageInfo{
			title:     fmt.Sprintf("%s — %s · Marque-pages tech", a.Name, a.Subcategory),
			desc:      truncate(desc, 160),
			canonical: a.Permalink,
			jsonld:    ldJSON(ld),
			body:      b.String(),
		}))
		urls = append(urls, a.Permalink)
	}

	// sitemap.xml complet (remplace la version statique du dépôt dans le site publié)
	var sm strings.Builder
	sm.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for i, u := range urls {
		if i == 0 {
			fmt.Fprintf(&sm, "  <url><loc>%s</loc><lastmod>%s</lastmod></url>\n", e(u), now.Format("2006-01-02"))
		} else {
			fmt.Fprintf(&sm, "  <url><loc>%s</loc></url>\n", e(u))
		}
	}
	sm.WriteString("</urlset>\n")
	write("sitemap.xml", sm.String())
	fmt.Printf("[OK] %d pages statiques + sitemap.xml (%d URL) → %s\n", written-1, len(urls), dir)
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
