// bm-suggest.go — Suggère catégorie/sous-catégorie pour chaque entrée de app-new.meta.json.
// Combine une table de mots-clés (portée du tableau CLAUDE.md) et une recherche de
// précédent par recouvrement de tags avec bookmarks.csv existant.
// Ne modifie aucun fichier — affiche juste des suggestions à valider.
// Run: go run ./scripts/bm-suggest.go  (depuis la racine du dépôt)
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	metaFile = "app-new.meta.json"
	csvFile2 = "bookmarks.csv"
)

type metaEntry struct {
	InputURL       string   `json:"input_url"`
	Name           string   `json:"name"`
	DescriptionRaw string   `json:"description_raw"`
	IsGitHub       bool     `json:"is_github"`
	GitHubTopics   []string `json:"github_topics"`
	IsDuplicate    bool     `json:"is_duplicate"`
}

type catSub struct {
	Category    string
	Subcategory string
}

type existingRow struct {
	Name        string
	Category    string
	Subcategory string
	Tags        map[string]bool
}

// keywordTable porte le tableau de catégorisation du CLAUDE.md, clés en anglais
// (mots typiques des github topics). Ordre non significatif : tous les matches
// sont comptés comme votes, le plus fort l'emporte.
var keywordTable = map[string]catSub{
	// Monitoring & Observability
	"monitoring": {"Développement", "Monitoring & Observability"}, "observability": {"Développement", "Monitoring & Observability"},
	"logging": {"Développement", "Monitoring & Observability"}, "logs": {"Développement", "Monitoring & Observability"},
	"metrics": {"Développement", "Monitoring & Observability"}, "tracing": {"Développement", "Monitoring & Observability"},
	"apm": {"Développement", "Monitoring & Observability"}, "alerting": {"Développement", "Monitoring & Observability"},
	// Frameworks Frontend
	"frontend": {"Développement", "Frameworks Frontend"}, "react": {"Développement", "Frameworks Frontend"},
	"vue": {"Développement", "Frameworks Frontend"}, "svelte": {"Développement", "Frameworks Frontend"},
	"ssr": {"Développement", "Frameworks Frontend"}, "static-site": {"Développement", "Frameworks Frontend"},
	"ui-components": {"Développement", "Frameworks Frontend"}, "component-library": {"Développement", "Frameworks Frontend"},
	// Langages & Shells
	"golang": {"Développement", "Langages & Shells"}, "rust-lang": {"Développement", "Langages & Shells"},
	"programming-language": {"Développement", "Langages & Shells"}, "shell": {"Développement", "Langages & Shells"},
	"bash": {"Développement", "Langages & Shells"}, "powershell": {"Développement", "Langages & Shells"},
	// Git & Version Control
	"git": {"Développement", "Git & Version Control"}, "vcs": {"Développement", "Git & Version Control"},
	"forge": {"Développement", "Git & Version Control"},
	// CI/CD & Automation
	"ci": {"Développement", "CI/CD & Automation"}, "cd": {"Développement", "CI/CD & Automation"},
	"cicd": {"Développement", "CI/CD & Automation"}, "pipeline": {"Développement", "CI/CD & Automation"},
	"gitops": {"Développement", "CI/CD & Automation"}, "workflow-automation": {"Développement", "CI/CD & Automation"},
	"task-runner": {"Développement", "CI/CD & Automation"}, "orchestrator": {"Développement", "CI/CD & Automation"},
	"runbook": {"Développement", "CI/CD & Automation"}, "durable-execution": {"Développement", "CI/CD & Automation"},
	// Infrastructure as Code
	"terraform": {"Développement", "Infrastructure as Code"}, "iac": {"Développement", "Infrastructure as Code"},
	"ansible": {"Développement", "Infrastructure as Code"}, "pulumi": {"Développement", "Infrastructure as Code"},
	"provisioning": {"Développement", "Infrastructure as Code"}, "config-management": {"Développement", "Infrastructure as Code"},
	"aws-emulator": {"Développement", "Infrastructure as Code"}, "localstack-alternative": {"Développement", "Infrastructure as Code"},
	// Conteneurs & Orchestration
	"kubernetes": {"Développement", "Conteneurs & Orchestration"}, "docker": {"Développement", "Conteneurs & Orchestration"},
	"container": {"Développement", "Conteneurs & Orchestration"}, "oci": {"Développement", "Conteneurs & Orchestration"},
	"docker-compose": {"Développement", "Conteneurs & Orchestration"}, "k8s": {"Développement", "Conteneurs & Orchestration"},
	// Platform Engineering
	"paas": {"Développement", "Platform Engineering"}, "self-hosted-paas": {"Développement", "Platform Engineering"},
	"personal-cloud": {"Développement", "Platform Engineering"},
	// IA & ML — Agents & Outils IA de dev
	"mcp": {"IA & Machine Learning", "Agents & Outils IA de dev"}, "mcp-server": {"IA & Machine Learning", "Agents & Outils IA de dev"},
	"ai-agent": {"IA & Machine Learning", "Agents & Outils IA de dev"}, "coding-agent": {"IA & Machine Learning", "Agents & Outils IA de dev"},
	"claude-code": {"IA & Machine Learning", "Agents & Outils IA de dev"}, "agent-skills": {"IA & Machine Learning", "Agents & Outils IA de dev"},
	"multi-agent": {"IA & Machine Learning", "Agents & Outils IA de dev"}, "swe-bench": {"IA & Machine Learning", "Agents & Outils IA de dev"},
	"knowledge-graph": {"IA & Machine Learning", "Agents & Outils IA de dev"}, "code-analysis": {"IA & Machine Learning", "Agents & Outils IA de dev"},
	// IA & ML — Plateformes & Modèles
	"llm": {"IA & Machine Learning", "Plateformes & Modèles"}, "machine-learning": {"IA & Machine Learning", "Plateformes & Modèles"},
	"deep-learning": {"IA & Machine Learning", "Plateformes & Modèles"}, "inference": {"IA & Machine Learning", "Plateformes & Modèles"},
	"tts": {"IA & Machine Learning", "Plateformes & Modèles"}, "text-to-speech": {"IA & Machine Learning", "Plateformes & Modèles"},
	"ocr": {"IA & Machine Learning", "Plateformes & Modèles"}, "on-device-ai": {"IA & Machine Learning", "Plateformes & Modèles"},
	"edge-ai": {"IA & Machine Learning", "Plateformes & Modèles"}, "rag": {"IA & Machine Learning", "Plateformes & Modèles"},
	// IA & ML — Workflows & Automation IA
	"n8n": {"IA & Machine Learning", "Workflows & Automation IA"}, "no-code-ai": {"IA & Machine Learning", "Workflows & Automation IA"},
	// Bases de données
	"postgresql": {"Bases de données", "Bases relationnelles"}, "mysql": {"Bases de données", "Bases relationnelles"},
	"distributed-sql": {"Bases de données", "Bases relationnelles"},
	"nosql":           {"Bases de données", "Bases NoSQL & Cache"}, "redis": {"Bases de données", "Bases NoSQL & Cache"},
	"cache": {"Bases de données", "Bases NoSQL & Cache"}, "key-value": {"Bases de données", "Bases NoSQL & Cache"},
	"olap": {"Bases de données", "Analytique & OLAP"}, "columnar": {"Bases de données", "Analytique & OLAP"},
	"time-series": {"Bases de données", "Analytique & OLAP"}, "time-series-database": {"Bases de données", "Analytique & OLAP"},
	"database-client": {"Bases de données", "Outils DB"}, "orm": {"Bases de données", "Outils DB"},
	"database-backup": {"Bases de données", "Outils DB"}, "pooler": {"Bases de données", "Outils DB"},
	"sharding": {"Bases de données", "Outils DB"},
	// Cybersécurité
	"osint": {"Cybersécurité", "Threat Detection & Analysis"}, "pentest": {"Cybersécurité", "Threat Detection & Analysis"},
	"pentesting": {"Cybersécurité", "Threat Detection & Analysis"}, "red-team": {"Cybersécurité", "Threat Detection & Analysis"},
	"vulnerability": {"Cybersécurité", "Threat Detection & Analysis"}, "kerberoasting": {"Cybersécurité", "Threat Detection & Analysis"},
	"honeypot": {"Cybersécurité", "Threat Detection & Analysis"}, "threat-detection": {"Cybersécurité", "Threat Detection & Analysis"},
	"bugbounty": {"Cybersécurité", "Threat Detection & Analysis"},
	"sso":       {"Cybersécurité", "Authentication & IAM"}, "oauth": {"Cybersécurité", "Authentication & IAM"},
	"saml": {"Cybersécurité", "Authentication & IAM"}, "iam": {"Cybersécurité", "Authentication & IAM"},
	"captcha": {"Cybersécurité", "Authentication & IAM"},
	"vpn":     {"Cybersécurité", "Réseau & Accès"}, "wireguard": {"Cybersécurité", "Réseau & Accès"},
	"proxy": {"Cybersécurité", "Réseau & Accès"}, "tunnel": {"Cybersécurité", "Réseau & Accès"},
	"dns": {"Cybersécurité", "Réseau & Accès"}, "zero-trust": {"Cybersécurité", "Réseau & Accès"},
	"reverse-proxy": {"Cybersécurité", "Réseau & Accès"}, "anti-censorship": {"Cybersécurité", "Réseau & Accès"},
	"secrets": {"Cybersécurité", "Secrets & Credentials"}, "credentials": {"Cybersécurité", "Secrets & Credentials"},
	"vault": {"Cybersécurité", "Secrets & Credentials"}, "certificate": {"Cybersécurité", "Secrets & Credentials"},
	"pii": {"Cybersécurité", "Secrets & Credentials"}, "encryption": {"Cybersécurité", "Secrets & Credentials"},
	"reverse-engineering": {"Cybersécurité", "Reverse Engineering"}, "decompiler": {"Cybersécurité", "Reverse Engineering"},
	"malware-analysis": {"Cybersécurité", "Reverse Engineering"}, "binary": {"Cybersécurité", "Reverse Engineering"},
	"bastion": {"Cybersécurité", "PAM & Jump Servers"}, "jump-server": {"Cybersécurité", "PAM & Jump Servers"},
	"pam":        {"Cybersécurité", "PAM & Jump Servers"},
	"compliance": {"Cybersécurité", "Audit & Compliance"}, "hardening": {"Cybersécurité", "Audit & Compliance"},
	// Virtualisation & Infrastructure
	"hypervisor": {"Virtualisation & Infrastructure", "Hyperviseurs"}, "kvm": {"Virtualisation & Infrastructure", "Hyperviseurs"},
	"xen": {"Virtualisation & Infrastructure", "Hyperviseurs"}, "vmware": {"Virtualisation & Infrastructure", "Hyperviseurs"},
	"iaas": {"Virtualisation & Infrastructure", "Cloud Infrastructure"}, "openstack": {"Virtualisation & Infrastructure", "Cloud Infrastructure"},
	"hyperconverged": {"Virtualisation & Infrastructure", "Cloud Infrastructure"},
	// Systèmes d'exploitation
	"linux": {"Systèmes d'exploitation", "Linux"}, "distro": {"Systèmes d'exploitation", "Linux"},
	"immutable-os": {"Systèmes d'exploitation", "Linux"}, "nixos": {"Systèmes d'exploitation", "Linux"},
	"sysadmin": {"Systèmes d'exploitation", "Outils système"}, "windows": {"Systèmes d'exploitation", "Outils système"},
	"debloat": {"Systèmes d'exploitation", "Outils système"}, "window-manager": {"Systèmes d'exploitation", "Outils système"},
	"tiling": {"Systèmes d'exploitation", "Outils système"}, "menubar": {"Systèmes d'exploitation", "Outils système"},
	"wine": {"Systèmes d'exploitation", "Compatibilité Windows"}, "proton": {"Systèmes d'exploitation", "Compatibilité Windows"},
	// Data & Analytics
	"bi": {"Data & Analytics", "Business Intelligence"}, "data-visualization": {"Data & Analytics", "Business Intelligence"},
	"dashboard-bi": {"Data & Analytics", "Business Intelligence"},
	"etl":          {"Data & Analytics", "Data Platforms"}, "data-platform": {"Data & Analytics", "Data Platforms"},
	"data-lake": {"Data & Analytics", "Data Platforms"}, "data-cleaning": {"Data & Analytics", "Data Platforms"},
	"big-data": {"Data & Analytics", "Big Data & Streaming"}, "streaming": {"Data & Analytics", "Big Data & Streaming"},
	"kafka": {"Data & Analytics", "Big Data & Streaming"}, "spark": {"Data & Analytics", "Big Data & Streaming"},
	// Productivité & Collaboration
	"kanban": {"Productivité & Collaboration", "Gestion de projet"}, "project-management": {"Productivité & Collaboration", "Gestion de projet"},
	"sprint": {"Productivité & Collaboration", "Gestion de projet"},
	"crm":    {"Productivité & Collaboration", "CRM & ERP"}, "erp": {"Productivité & Collaboration", "CRM & ERP"},
	"customer-support": {"Productivité & Collaboration", "CRM & ERP"}, "live-chat": {"Productivité & Collaboration", "CRM & ERP"},
	"itam": {"Productivité & Collaboration", "Gestion d'actifs IT"}, "asset-management": {"Productivité & Collaboration", "Gestion d'actifs IT"},
	"time-tracking": {"Productivité & Collaboration", "Time Tracking"},
	"notes":         {"Productivité & Collaboration", "Notes & Documentation"}, "wiki": {"Productivité & Collaboration", "Notes & Documentation"},
	"pkm": {"Productivité & Collaboration", "Notes & Documentation"}, "knowledge-base": {"Productivité & Collaboration", "Notes & Documentation"},
	"markdown-notes": {"Productivité & Collaboration", "Notes & Documentation"},
	"budget":         {"Productivité & Collaboration", "Finance"}, "accounting": {"Productivité & Collaboration", "Finance"},
	"personal-finance": {"Productivité & Collaboration", "Finance"},
	// Utilitaires
	"pdf": {"Utilitaires", "Documents & PDF"}, "document-conversion": {"Utilitaires", "Documents & PDF"},
	"rss":   {"Utilitaires", "Documents & PDF"},
	"media": {"Utilitaires", "Médias & Multimédia"}, "video-downloader": {"Utilitaires", "Médias & Multimédia"},
	"iptv": {"Utilitaires", "Médias & Multimédia"}, "youtube": {"Utilitaires", "Médias & Multimédia"},
	"remote-desktop": {"Utilitaires", "Remote Desktop"}, "vnc": {"Utilitaires", "Remote Desktop"}, "rdp": {"Utilitaires", "Remote Desktop"},
	"homepage": {"Utilitaires", "Dashboards & Homepages"}, "homelab": {"Utilitaires", "Dashboards & Homepages"},
	"screenshot":   {"Utilitaires", "Capture d'écran"},
	"bootable-usb": {"Utilitaires", "Système & Boot"}, "live-usb": {"Utilitaires", "Système & Boot"},
	"kde": {"Utilitaires", "Applications KDE"},
	// Électronique & Hardware
	"pcb": {"Électronique & Hardware", ""}, "fpga": {"Électronique & Hardware", ""},
	"verilog": {"Électronique & Hardware", ""}, "hardware": {"Électronique & Hardware", ""},
	"electronics": {"Électronique & Hardware", ""},
	// Documentation & Learning
	"awesome-list": {"Documentation & Learning", ""}, "awesome": {"Documentation & Learning", ""},
	"tutorial": {"Documentation & Learning", ""}, "course": {"Documentation & Learning", ""},
	"roadmap": {"Documentation & Learning", ""}, "learning": {"Documentation & Learning", ""},
	// Éditeurs & IDE / UI Terminal / Diagrammes / Utilitaires Dev
	"text-editor": {"Développement", "Éditeurs & IDE"}, "ide": {"Développement", "Éditeurs & IDE"},
	"terminal": {"Outils de développement", "UI/Terminal"}, "tui": {"Outils de développement", "UI/Terminal"},
	"terminal-ui": {"Outils de développement", "UI/Terminal"},
	"diagrams":    {"Outils de développement", "Diagrammes & Visualisation"}, "visualization": {"Outils de développement", "Diagrammes & Visualisation"},
	"flowchart": {"Outils de développement", "Diagrammes & Visualisation"}, "erd": {"Outils de développement", "Diagrammes & Visualisation"},
	"api-testing": {"Outils de développement", "API & Testing"}, "rest-client": {"Outils de développement", "API & Testing"},
	"low-code": {"Outils de développement", "Low-Code / No-Code"}, "no-code": {"Outils de développement", "Low-Code / No-Code"},
	"package-manager": {"Outils de développement", "Utilitaires Dev"}, "dev-environment": {"Outils de développement", "Utilitaires Dev"},
	"nix": {"Outils de développement", "Utilitaires Dev"}, "cli-tool": {"Outils de développement", "Utilitaires Dev"},
}

func main() {
	data, err := os.ReadFile(metaFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
	var entries []metaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] parsing %s: %v\n", metaFile, err)
		os.Exit(1)
	}

	existing, err := loadExisting(csvFile2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] loading %s: %v\n", csvFile2, err)
	}

	wordRe := regexp.MustCompile(`[a-z0-9]+`)
	skipped, flagged := 0, 0

	for _, e := range entries {
		if e.IsDuplicate {
			skipped++
			continue
		}

		tokens := make([]string, 0, len(e.GitHubTopics)+8)
		for _, t := range e.GitHubTopics {
			tokens = append(tokens, strings.ToLower(t))
		}
		tokens = append(tokens, wordRe.FindAllString(strings.ToLower(e.DescriptionRaw), -1)...)

		votes := make(map[catSub]int)
		var matched []string
		for _, tok := range tokens {
			if cs, ok := keywordTable[tok]; ok {
				votes[cs]++
				matched = append(matched, tok)
			}
		}
		topCS, topScore := bestVote(votes)

		precName, precCS, precOverlap := bestPrecedent(tokens, existing)

		line := fmt.Sprintf("- %-28s", truncate(e.Name, 28))
		if topScore > 0 {
			line += fmt.Sprintf(" | règle: %s > %s (%d: %s)", topCS.Category, topCS.Subcategory, topScore, strings.Join(matched, ","))
		} else {
			line += " | règle: —"
		}
		if precOverlap > 0 {
			line += fmt.Sprintf(" | précédent: %s → %s > %s (%d tags communs)", truncate(precName, 20), precCS.Category, precCS.Subcategory, precOverlap)
		} else {
			line += " | précédent: —"
		}
		if topScore == 0 && precOverlap == 0 {
			line += " | ⚠ MANQUE INFO — à catégoriser manuellement"
			flagged++
		} else if topScore > 0 && precOverlap > 0 && topCS != precCS {
			line += " | ⚠ règle et précédent divergent"
			flagged++
		}
		fmt.Println(line)
	}

	fmt.Printf("\n[i] %d entrées analysées, %d doublons ignorés, %d à vérifier manuellement\n", len(entries)-skipped, skipped, flagged)
}

func bestVote(votes map[catSub]int) (catSub, int) {
	var best catSub
	bestN := 0
	// Deterministic order: sort keys before comparing equal scores.
	keys := make([]catSub, 0, len(votes))
	for k := range votes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Category != keys[j].Category {
			return keys[i].Category < keys[j].Category
		}
		return keys[i].Subcategory < keys[j].Subcategory
	})
	for _, k := range keys {
		if votes[k] > bestN {
			best = k
			bestN = votes[k]
		}
	}
	return best, bestN
}

func bestPrecedent(tokens []string, existing []existingRow) (name string, cs catSub, overlap int) {
	tokSet := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tokSet[t] = true
	}
	best := 0
	for _, row := range existing {
		n := 0
		for t := range tokSet {
			if row.Tags[t] {
				n++
			}
		}
		if n > best {
			best = n
			name = row.Name
			cs = catSub{row.Category, row.Subcategory}
		}
	}
	return name, cs, best
}

func loadExisting(path string) ([]existingRow, error) {
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
	idx := make(map[string]int)
	for i, h := range headers {
		idx[h] = i
	}
	var rows []existingRow
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		tagsStr := ""
		if i, ok := idx["tags"]; ok && i < len(row) {
			tagsStr = row[i]
		}
		tagSet := make(map[string]bool)
		for _, t := range strings.Split(tagsStr, "|") {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				tagSet[t] = true
			}
		}
		rows = append(rows, existingRow{
			Name:        row[idx["name"]],
			Category:    row[idx["category"]],
			Subcategory: row[idx["subcategory"]],
			Tags:        tagSet,
		})
	}
	return rows, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
