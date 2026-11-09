// bm-check.go — Vérifie les liens de bookmarks.csv et remplit la colonne `error`
// (vide = lien OK). Le frontend affiche un badge « ⚠ erreur » sur ces entrées et
// le filtre « ⚠ Liens en erreur » (?etat=erreur) permet de les retrouver pour les corriger.
//
// Une erreur n'est notée que si elle est sûre :
//   - dépôt GitHub (url github.com/owner/repo, ou colonne `repo`) introuvable via l'API (404, 451) ;
//   - site : HTTP 404 / 410, HTTP 5xx répété, domaine introuvable (DNS), connexion refusée,
//     certificat TLS invalide, délai dépassé ou boucle de redirections (deux essais), domaine à vendre.
//
// Ne comptent pas comme erreur : 401 / 403 / 429 et défis anti-bot (le site répond).
// Résultat incertain (limite d'API, coupure réseau ponctuelle, connexion réinitialisée) :
// la valeur précédente de la colonne est conservée.
// Garde-fou : si plus de 25 % des liens échouent (réseau coupé ?), le CSV n'est pas modifié.
//
// Run: GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-check.go
package main

import (
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const checkCSVPath = "bookmarks.csv"

// Résultat d'une vérification : ok, erreur sûre, ou incertain (valeur précédente conservée).
type verdict int

const (
	vOK verdict = iota
	vError
	vUnsure
)

type result struct {
	v      verdict
	detail string
}

var ghRepoRe = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// Hébergeurs de domaines en vente / parqués : une redirection vers eux = domaine expiré.
var parkingHosts = []string{"hugedomains.com", "dan.com", "sedo.com", "afternic.com", "undeveloped.com", "bodis.com", "parkingcrew.net", "domainmarket.com", "atom.com"}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("[!] GITHUB_TOKEN non défini — limite 60 req/h : les dépôts non vérifiés gardent leur valeur.")
	}
	f, err := os.Open(checkCSVPath)
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
	urlI, nameI, repoI, errI := col("url"), col("name"), col("repo"), col("error")
	rows := all[1:]
	for i := range rows {
		for len(rows[i]) < len(headers) {
			rows[i] = append(rows[i], "")
		}
	}

	jar, _ := cookiejar.New(nil) // certains sites posent un cookie puis redirigent : sans jar = fausse boucle
	c := &checker{
		token: token,
		web: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errTooManyRedirects
				}
				return nil
			},
		},
		api: &http.Client{Timeout: 15 * time.Second},
	}

	results := make([]result, len(rows))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				res := c.checkURL(rows[i][urlI])
				if res.v == vOK && strings.TrimSpace(rows[i][repoI]) != "" {
					rr := c.checkRepoField(rows[i][repoI])
					if rr.v != vOK {
						res = rr
					} else if rr.detail != "" && res.detail == "" {
						res.detail = "(colonne repo) " + rr.detail
					}
				}
				results[i] = res
				mu.Lock()
				done++
				if done%100 == 0 {
					fmt.Printf("  … %d/%d\n", done, len(rows))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range rows {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var errs, unsure, cleared, moved []string
	nErr := 0
	for i, res := range results {
		name, prev := rows[i][nameI], rows[i][errI]
		switch res.v {
		case vOK:
			if res.detail != "" {
				moved = append(moved, fmt.Sprintf("  %-34s %s %s", name, rows[i][urlI], res.detail))
			}
			if prev != "" {
				cleared = append(cleared, fmt.Sprintf("  %-34s %s (était : %s)", name, rows[i][urlI], prev))
			}
			rows[i][errI] = ""
		case vError:
			nErr++
			rows[i][errI] = res.detail
			errs = append(errs, fmt.Sprintf("  %-34s %-22s %s", name, res.detail, rows[i][urlI]))
		case vUnsure:
			unsure = append(unsure, fmt.Sprintf("  %-34s %-22s %s", name, res.detail, rows[i][urlI]))
		}
	}
	sort.Strings(errs)
	sort.Strings(unsure)
	fmt.Printf("\nLiens en erreur (%d) :\n%s\n", len(errs), strings.Join(errs, "\n"))
	if len(unsure) > 0 {
		fmt.Printf("\nIncertains, valeur précédente conservée (%d) :\n%s\n", len(unsure), strings.Join(unsure, "\n"))
	}
	if len(moved) > 0 {
		sort.Strings(moved)
		fmt.Printf("\nDépôts renommés, URL à mettre à jour (%d) :\n%s\n", len(moved), strings.Join(moved, "\n"))
	}
	if len(cleared) > 0 {
		fmt.Printf("\nRéparés depuis la dernière vérification (%d) :\n%s\n", len(cleared), strings.Join(cleared, "\n"))
	}

	if nErr*4 > len(rows) {
		fmt.Fprintf(os.Stderr, "\n[ABANDON] %d/%d liens en erreur : réseau coupé ? CSV non modifié.\n", nErr, len(rows))
		os.Exit(2)
	}

	out, err := os.Create(checkCSVPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Write(headers)
	w.WriteAll(rows)
	fmt.Printf("\n[OK] %d liens vérifiés — %d en erreur, %d incertains (colonne error)\n", len(rows), nErr, len(unsure))
}

var errTooManyRedirects = errors.New("boucle de redirections")

type checker struct {
	token string
	web   *http.Client
	api   *http.Client
}

// checkURL : dépôt GitHub via l'API, sinon requête HTTP (deux essais si l'échec peut être passager).
func (c *checker) checkURL(raw string) result {
	if m := ghRepoRe.FindStringSubmatch(raw); m != nil {
		return c.checkRepo(m[1] + "/" + m[2])
	}
	res := c.fetch(raw)
	if res.v != vOK && res.detail != "HTTP 404" && res.detail != "HTTP 410" && res.detail != "DNS introuvable" {
		time.Sleep(3 * time.Second)
		res = c.fetch(raw)
	}
	return res
}

func (c *checker) checkRepoField(raw string) result {
	m := ghRepoRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return result{vOK, ""}
	}
	res := c.checkRepo(m[1] + "/" + m[2])
	if res.v == vError {
		res.detail = "dépôt (colonne repo) introuvable"
	}
	return res
}

func (c *checker) checkRepo(full string) result {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+full, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bm-check/1.0")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.api.Do(req)
	if err != nil {
		return result{vUnsure, "API GitHub injoignable"}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		// Dépôt renommé / transféré : l'API suit la redirection. Pas une erreur, mais signalé.
		var ri struct {
			FullName string `json:"full_name"`
		}
		if json.NewDecoder(resp.Body).Decode(&ri) == nil && ri.FullName != "" && !strings.EqualFold(ri.FullName, full) {
			return result{vOK, "renommé en " + ri.FullName}
		}
		return result{vOK, ""}
	case 404:
		return result{vError, "dépôt GitHub introuvable"}
	case 451:
		return result{vError, "dépôt GitHub bloqué (DMCA)"}
	default:
		return result{vUnsure, fmt.Sprintf("API GitHub HTTP %d", resp.StatusCode)}
	}
}

func (c *checker) fetch(raw string) result {
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return result{vError, "URL invalide"}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr,en;q=0.8")
	resp, err := c.web.Do(req)
	if err != nil {
		return classifyNetErr(err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 256<<10))
	resp.Body.Close()

	final := strings.TrimPrefix(strings.ToLower(resp.Request.URL.Hostname()), "www.")
	orig, _ := url.Parse(raw)
	if orig != nil && final != strings.TrimPrefix(strings.ToLower(orig.Hostname()), "www.") {
		for _, p := range parkingHosts {
			if final == p || strings.HasSuffix(final, "."+p) {
				return result{vError, "domaine à vendre"}
			}
		}
	}

	code := resp.StatusCode
	switch {
	case code == 404 || code == 410:
		return result{vError, fmt.Sprintf("HTTP %d", code)}
	case code >= 500:
		// Défi anti-bot (Cloudflare…) : le site répond, ce n'est pas une panne
		if resp.Header.Get("Cf-Mitigated") != "" {
			return result{vOK, ""}
		}
		return result{vError, fmt.Sprintf("HTTP %d", code)}
	default:
		return result{vOK, ""} // 2xx, 3xx, 401, 403, 429…
	}
}

func classifyNetErr(err error) result {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, errTooManyRedirects):
		return result{vError, "boucle de redirections"}
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return result{vError, "DNS introuvable"}
	case errors.As(err, &dnsErr):
		return result{vUnsure, "DNS temporaire"}
	case errors.As(err, &certErr):
		return result{vError, "certificat TLS invalide"}
	case strings.Contains(err.Error(), "connection refused"):
		return result{vError, "connexion refusée"}
	case isTimeout(err):
		return result{vError, "délai dépassé"}
	}
	return result{vUnsure, "réseau : " + shortErr(err)}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
