// bm-mcp.go — Serveur MCP (Model Context Protocol, transport stdio) pour chercher dans le catalogue
// du site depuis une IA (Claude Code, Claude Desktop, Cursor…).
//
// Outils exposés :
//   - search_apps      recherche par mots-clés (mots vides ignorés, repli partiel) + filtres : catégorie,
//     famille de licence, licence précise (SPDX), étoiles, activité, archivés, liens morts
//   - get_app          fiche complète d'un outil (id, permalien, URL ou nom)
//   - list_categories  arborescence catégories / sous-catégories avec compteurs
//
// Données : api/apps.json local s'il existe (dépôt cloné), sinon https://mat8950.github.io/api/apps.json.
// Forcer une source : -src <fichier ou URL>.
//
// Brancher dans Claude Code (depuis la racine du dépôt) :
//
//	claude mcp add marque-pages -- go run ./scripts/bm-mcp.go
//
// Ou compiler une fois : go build -o bm-mcp ./scripts/bm-mcp.go && claude mcp add marque-pages -- /chemin/bm-mcp
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const defaultRemote = "https://mat8950.github.io/api/apps.json"

type mcpApp struct {
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

type catalog struct {
	Site     string            `json:"site"`
	Families map[string]string `json:"license_families"`
	Apps     []*mcpApp         `json:"apps"`
}

var data *catalog

func loadCatalog(src string) (*catalog, error) {
	var rd io.Reader
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		c := &http.Client{Timeout: 20 * time.Second}
		resp, err := c.Get(src)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d sur %s", resp.StatusCode, src)
		}
		rd = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		rd = f
	}
	var c catalog
	if err := json.NewDecoder(rd).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ---------- JSON-RPC ----------

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

var out = bufio.NewWriter(os.Stdout)

func send(v any) {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.Encode(v) // ajoute le \n de fin de message
	out.Flush()
}

func main() {
	src := flag.String("src", "", "fichier ou URL de apps.json (défaut : api/apps.json local, sinon le site)")
	flag.Parse()
	if *src == "" {
		*src = defaultRemote
		if _, err := os.Stat("api/apps.json"); err == nil {
			*src = "api/apps.json"
		}
	}
	var err error
	if data, err = loadCatalog(*src); err != nil {
		fmt.Fprintln(os.Stderr, "[bm-mcp] chargement impossible :", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[bm-mcp] %d outils chargés depuis %s\n", len(data.Apps), *src)

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if line[0] == '[' { // lots JSON-RPC : retirés du protocole MCP (2025-06-18)
			send(rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32600, "requêtes groupées (batch) non prises en charge"}})
			continue
		}
		var req rpcReq
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			send(rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "JSON invalide"}})
			continue
		}
		if len(req.ID) == 0 { // notification (notifications/initialized…) : pas de réponse
			continue
		}
		res, rerr := handle(req)
		send(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: res, Error: rerr})
	}
}

// Versions du protocole MCP prises en charge (la plus récente d'abord)
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func handle(req rpcReq) (any, *rpcErr) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		// Version demandée si prise en charge, sinon la plus récente connue (le client décide de continuer ou non)
		ver := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				ver = v
			}
		}
		return map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "marque-pages", "version": "1.0.0"},
			"instructions": "Catalogue curé d'outils techniques (DevOps, IA, sécurité, data, dev, self-hosting), descriptions en français. " +
				"Utilise search_apps pour trouver des outils (filtre license_family=libre pour du 100 % logiciel libre), " +
				"get_app pour une fiche, list_categories pour l'arborescence. Cite le champ permalink ou url.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefs()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcErr{-32602, "paramètres invalides"}
		}
		known := false
		for _, t := range toolDefs() {
			known = known || t["name"] == p.Name
		}
		if !known {
			return nil, &rpcErr{-32602, "outil inconnu : " + p.Name}
		}
		text, isErr := callTool(p.Name, p.Arguments)
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}, nil
	}
	return nil, &rpcErr{-32601, "méthode inconnue : " + req.Method}
}

var licenseFamilies = []string{"libre", "libre-permissive", "libre-copyleft", "open-core", "source-available", "proprietaire", "inconnue"}

// Outils en lecture seule sur un catalogue fermé : le client peut les appeler sans risque
func readOnly(title string) map[string]any {
	return map[string]any{"title": title, "readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}
}

func toolDefs() []map[string]any {
	fam := licenseFamilies
	return []map[string]any{
		{
			"name":        "search_apps",
			"title":       "Chercher des outils",
			"annotations": readOnly("Chercher des outils"),
			"description": "Cherche des outils dans le catalogue (nom, description FR, tags, catégorie). " +
				"Mots-clés plutôt que phrases (mots vides ignorés) ; tous les mots doivent apparaître, sinon résultats partiels (match = partiel). " +
				"Triés par pertinence puis étoiles GitHub. Liens morts et dépôts archivés exclus par défaut.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":            map[string]any{"type": "string", "description": "Mots-clés, ex. « reverse proxy », « kubernetes dashboard », « tts ». Vide = tout."},
					"category":         map[string]any{"type": "string", "description": "Catégorie ou sous-catégorie (nom ou chemin, correspondance partielle), ex. « Monitoring », « /cybersecurite/ »."},
					"license_family":   map[string]any{"type": "string", "enum": fam, "description": "libre = 100 % logiciel libre (permissive + copyleft)."},
					"license":          map[string]any{"type": "string", "description": "Licence précise (identifiant SPDX), ex. « MIT », « AGPL-3.0 », « Apache-2.0 ». Inclut les « X OR Y » et les open-core « X+EE »."},
					"min_stars":        map[string]any{"type": "integer", "description": "Étoiles GitHub minimum."},
					"active_only":      map[string]any{"type": "boolean", "description": "Seulement les dépôts avec un push depuis moins de 12 mois."},
					"include_archived": map[string]any{"type": "boolean", "description": "Inclure les dépôts archivés (exclus par défaut)."},
					"include_broken":   map[string]any{"type": "boolean", "description": "Inclure les liens morts (champ link_error, exclus par défaut)."},
					"limit":            map[string]any{"type": "integer", "description": "Nombre de résultats (défaut 10, max 50)."},
				},
			},
		},
		{
			"name":        "get_app",
			"title":       "Fiche d'un outil",
			"annotations": readOnly("Fiche d'un outil"),
			"description": "Fiche complète d'un outil : par id (« categorie/sous-categorie/outil »), permalien, URL officielle ou nom exact.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"ref": map[string]any{"type": "string", "description": "id, permalien, URL ou nom de l'outil."}},
				"required":   []string{"ref"},
			},
		},
		{
			"name":        "list_categories",
			"title":       "Catégories du catalogue",
			"annotations": readOnly("Catégories du catalogue"),
			"description": "Arborescence catégories → sous-catégories avec le nombre d'outils et le chemin de chacune, plus la répartition par famille de licence.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if a := accents[r]; a != "" {
			b.WriteString(a)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return " " + strings.Join(strings.Fields(b.String()), " ") + " "
}

var accents = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ä': "a", 'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'î': "i", 'ï': "i",
	'ô': "o", 'ö': "o", 'ù': "u", 'û': "u", 'ü': "u", 'ÿ': "y", 'œ': "oe", 'æ': "ae",
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

// matchLicense : licence précise (« MIT », « AGPL-3.0 »), insensible à la casse, composant de « X OR Y » compris
func matchLicense(want, got string) bool {
	if want == "" {
		return true
	}
	for _, p := range licenseParts(got) {
		if strings.EqualFold(p, want) {
			return true
		}
	}
	return false
}

// Mots vides (FR/EN) retirés des requêtes en langage naturel : « outil de monitoring pour kubernetes » → monitoring kubernetes
var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a au aux avec d de des du en et l la le les ou par pour qui sur un une " +
		"an and for in of on or the to with app apps application applications logiciel logiciels outil outils software tool tools") {
		stopWords[w] = true
	}
}

// normURL : compare des URL sans schéma, « www. », casse ni barre finale
func normURL(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	return strings.TrimRight(strings.TrimPrefix(s, "www."), "/")
}

// decodeArgs : arguments absents = {} ; type incorrect (limit: "abc") = erreur explicite pour l'IA
func decodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("arguments invalides : %v", err)
	}
	return nil
}

func matchAny(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func matchFamily(want, got string) bool {
	if want == "" {
		return true
	}
	if want == "libre" {
		return got == "libre-permissive" || got == "libre-copyleft"
	}
	return want == got
}

func compact(a *mcpApp) map[string]any {
	m := map[string]any{
		"id": a.ID, "name": a.Name, "url": a.URL, "permalink": a.Permalink,
		"category": a.Category + " > " + a.Subcategory, "description": a.Description,
		"license": a.License, "license_family": a.LicenseFamily, "tags": a.Tags,
	}
	if a.Stars != nil {
		m["stars"] = *a.Stars
	}
	if a.PushedAt != "" {
		m["pushed_at"] = a.PushedAt
	}
	if a.Archived {
		m["archived"] = true
	}
	if a.LinkError != "" {
		m["link_error"] = a.LinkError
	}
	return m
}

func callTool(name string, raw json.RawMessage) (string, bool) {
	toJSON := func(v any) string {
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", " ")
		enc.Encode(v)
		return buf.String()
	}
	switch name {
	case "search_apps":
		var a struct {
			Query           string `json:"query"`
			Category        string `json:"category"`
			LicenseFamily   string `json:"license_family"`
			License         string `json:"license"`
			MinStars        int    `json:"min_stars"`
			ActiveOnly      bool   `json:"active_only"`
			IncludeArchived bool   `json:"include_archived"`
			IncludeBroken   bool   `json:"include_broken"`
			Limit           int    `json:"limit"`
		}
		if err := decodeArgs(raw, &a); err != nil {
			return err.Error(), true
		}
		if a.LicenseFamily != "" && !matchAny(a.LicenseFamily, licenseFamilies) {
			return "license_family inconnue : « " + a.LicenseFamily + " ». Valeurs : " + strings.Join(licenseFamilies, ", "), true
		}
		if a.Limit <= 0 {
			a.Limit = 10
		}
		if a.Limit > 50 {
			a.Limit = 50
		}
		allWords := strings.Fields(strings.TrimSpace(fold(a.Query)))
		var words []string
		for _, w := range allWords {
			if !stopWords[w] {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			words = allWords // requête faite uniquement de mots vides : on les garde
		}
		catQ := strings.TrimSpace(fold(a.Category))
		type hit struct {
			app     *mcpApp
			score   int
			matched int // mots trouvés (repli partiel)
		}
		var hits, partial []hit
		for _, app := range data.Apps {
			if !matchFamily(a.LicenseFamily, app.LicenseFamily) || !matchLicense(strings.TrimSpace(a.License), app.License) || (app.Archived && !a.IncludeArchived) || (app.LinkError != "" && !a.IncludeBroken) {
				continue
			}
			if a.MinStars > 0 && (app.Stars == nil || *app.Stars < a.MinStars) {
				continue
			}
			if a.ActiveOnly && (app.Active == nil || !*app.Active) {
				continue
			}
			if catQ != "" && !strings.Contains(fold(app.Category+" "+app.Subcategory+" "+app.CategoryPath), " "+catQ) {
				continue
			}
			name, tags, desc, cat := fold(app.Name), fold(strings.Join(app.Tags, " ")), fold(app.Description), fold(app.Category+" "+app.Subcategory)
			score, matched := 0, 0
			for _, w := range words {
				s := 0
				if strings.Contains(name, " "+w) {
					s += 6
				}
				if strings.Contains(tags, " "+w+" ") {
					s += 4
				} else if strings.Contains(tags, w) {
					s += 2
				}
				if strings.Contains(cat, " "+w) {
					s += 2
				}
				if strings.Contains(desc, " "+w) {
					s += 2
				} else if strings.Contains(desc, w) {
					s++
				}
				if s > 0 {
					matched++
				}
				score += s
			}
			switch {
			case matched == len(words):
				hits = append(hits, hit{app, score, matched})
			case matched > 0:
				partial = append(partial, hit{app, score, matched})
			}
		}
		// Aucun outil ne contient tous les mots : meilleurs résultats partiels (le plus de mots trouvés d'abord)
		match := "complet"
		if len(hits) == 0 && len(partial) > 0 {
			hits, match = partial, "partiel"
		}
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].matched != hits[j].matched {
				return hits[i].matched > hits[j].matched
			}
			if hits[i].score != hits[j].score {
				return hits[i].score > hits[j].score
			}
			si, sj := -1, -1
			if hits[i].app.Stars != nil {
				si = *hits[i].app.Stars
			}
			if hits[j].app.Stars != nil {
				sj = *hits[j].app.Stars
			}
			return si > sj
		})
		total := len(hits)
		if len(hits) > a.Limit {
			hits = hits[:a.Limit]
		}
		res := []map[string]any{}
		for _, h := range hits {
			res = append(res, compact(h.app))
		}
		out := map[string]any{"total": total, "returned": len(res), "results": res}
		if len(words) > 0 {
			out["terms"] = words
			out["match"] = match
		}
		return toJSON(out), false

	case "get_app":
		var a struct {
			Ref string `json:"ref"`
		}
		if err := decodeArgs(raw, &a); err != nil {
			return err.Error(), true
		}
		ref := strings.TrimSpace(a.Ref)
		if ref == "" {
			return "Paramètre « ref » requis : id, permalien, URL ou nom de l'outil.", true
		}
		refID := strings.Trim(strings.TrimPrefix(normURL(ref), normURL(data.Site)), "/")
		nref := normURL(ref)
		for _, app := range data.Apps {
			if app.ID == refID || normURL(app.Permalink) == nref || normURL(app.URL) == nref || (app.Repo != "" && normURL(app.Repo) == nref) {
				return toJSON(app), false
			}
		}
		for _, app := range data.Apps {
			if strings.EqualFold(app.Name, ref) {
				return toJSON(app), false
			}
		}
		return "Aucun outil trouvé pour « " + ref + " ». Utilise search_apps.", true

	case "list_categories":
		type sub struct {
			Name     string         `json:"name"`
			Path     string         `json:"path"`
			Count    int            `json:"count"`
			Licenses map[string]int `json:"licenses"`
		}
		type cat struct {
			Name     string         `json:"name"`
			Count    int            `json:"count"`
			Licenses map[string]int `json:"licenses"`
			Subs     []*sub         `json:"subcategories"`
		}
		cats, idx, subIdx := []*cat{}, map[string]*cat{}, map[string]*sub{}
		fam, ids := map[string]int{}, map[string]int{}
		for _, app := range data.Apps {
			fam[app.LicenseFamily]++
			for _, id := range licenseParts(app.License) {
				ids[id]++
			}
			c := idx[app.Category]
			if c == nil {
				c = &cat{Name: app.Category, Licenses: map[string]int{}}
				idx[app.Category] = c
				cats = append(cats, c)
			}
			c.Count++
			c.Licenses[app.LicenseFamily]++
			k := app.Category + " > " + app.Subcategory
			s := subIdx[k]
			if s == nil {
				s = &sub{Name: app.Subcategory, Path: app.CategoryPath, Licenses: map[string]int{}}
				subIdx[k] = s
				c.Subs = append(c.Subs, s)
			}
			s.Count++
			s.Licenses[app.LicenseFamily]++
		}
		sort.SliceStable(cats, func(i, j int) bool { return cats[i].Count > cats[j].Count })
		for _, c := range cats {
			sort.Slice(c.Subs, func(i, j int) bool { return c.Subs[i].Name < c.Subs[j].Name })
		}
		return toJSON(map[string]any{"total": len(data.Apps), "license_families": fam, "licenses": ids, "family_labels": data.Families, "categories": cats}), false
	}
	return "Outil inconnu : " + name, true
}
