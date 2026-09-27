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
	"monitoring": {"DevOps & Infrastructure", "Monitoring & Observabilité"}, "observability": {"DevOps & Infrastructure", "Monitoring & Observabilité"},
	"logging": {"DevOps & Infrastructure", "Monitoring & Observabilité"}, "logs": {"DevOps & Infrastructure", "Monitoring & Observabilité"},
	"metrics": {"DevOps & Infrastructure", "Monitoring & Observabilité"}, "tracing": {"DevOps & Infrastructure", "Monitoring & Observabilité"},
	"apm": {"DevOps & Infrastructure", "Monitoring & Observabilité"}, "alerting": {"DevOps & Infrastructure", "Monitoring & Observabilité"},
	// Frameworks Frontend
	"frontend": {"Développement", "Front-end & UI"}, "react": {"Développement", "Front-end & UI"},
	"vue": {"Développement", "Front-end & UI"}, "svelte": {"Développement", "Front-end & UI"},
	"ssr": {"Développement", "Front-end & UI"}, "static-site": {"Développement", "Front-end & UI"},
	"ui-components": {"Développement", "Front-end & UI"}, "component-library": {"Développement", "Front-end & UI"},
	// Langages & Shells
	"golang": {"Développement", "Langages & Bibliothèques"}, "rust-lang": {"Développement", "Langages & Bibliothèques"},
	"programming-language": {"Développement", "Langages & Bibliothèques"}, "shell": {"Développement", "Langages & Bibliothèques"},
	"bash": {"Développement", "Langages & Bibliothèques"}, "powershell": {"Développement", "Langages & Bibliothèques"},
	// Git & Version Control
	"git": {"Développement", "Git & Forges"}, "vcs": {"Développement", "Git & Forges"},
	"forge": {"Développement", "Git & Forges"},
	// CI/CD & Automation
	"ci": {"DevOps & Infrastructure", "CI/CD & Release"}, "cd": {"DevOps & Infrastructure", "CI/CD & Release"},
	"cicd": {"DevOps & Infrastructure", "CI/CD & Release"}, "pipeline": {"DevOps & Infrastructure", "CI/CD & Release"},
	"gitops": {"DevOps & Infrastructure", "CI/CD & Release"}, "workflow-automation": {"DevOps & Infrastructure", "Automatisation & Workflows"},
	"task-runner": {"DevOps & Infrastructure", "Automatisation & Workflows"}, "orchestrator": {"DevOps & Infrastructure", "Automatisation & Workflows"},
	"runbook": {"DevOps & Infrastructure", "Automatisation & Workflows"}, "durable-execution": {"DevOps & Infrastructure", "Automatisation & Workflows"},
	// Infrastructure as Code
	"terraform": {"DevOps & Infrastructure", "Infrastructure as Code"}, "iac": {"DevOps & Infrastructure", "Infrastructure as Code"},
	"ansible": {"DevOps & Infrastructure", "Infrastructure as Code"}, "pulumi": {"DevOps & Infrastructure", "Infrastructure as Code"},
	"provisioning": {"DevOps & Infrastructure", "Infrastructure as Code"}, "config-management": {"DevOps & Infrastructure", "Infrastructure as Code"},
	"aws-emulator": {"DevOps & Infrastructure", "Infrastructure as Code"}, "localstack-alternative": {"DevOps & Infrastructure", "Infrastructure as Code"},
	// Conteneurs & Orchestration
	"kubernetes": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"}, "docker": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"},
	"container": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"}, "oci": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"},
	"docker-compose": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"}, "k8s": {"DevOps & Infrastructure", "Conteneurs & Kubernetes"},
	// Platform Engineering
	"paas": {"DevOps & Infrastructure", "PaaS & Self-hosting"}, "self-hosted-paas": {"DevOps & Infrastructure", "PaaS & Self-hosting"},
	"personal-cloud": {"DevOps & Infrastructure", "PaaS & Self-hosting"},
	// IA & ML — agents, MCP, skills, code
	"mcp": {"IA & Machine Learning", "MCP & Outils pour agents"}, "mcp-server": {"IA & Machine Learning", "MCP & Outils pour agents"},
	"ai-agent": {"IA & Machine Learning", "Frameworks d'agents"}, "coding-agent": {"IA & Machine Learning", "Agents de code"},
	"claude-code": {"IA & Machine Learning", "Agents de code"}, "agent-skills": {"IA & Machine Learning", "Skills & Prompts"},
	"multi-agent": {"IA & Machine Learning", "Frameworks d'agents"}, "swe-bench": {"IA & Machine Learning", "Agents de code"},
	"knowledge-graph": {"IA & Machine Learning", "RAG & Mémoire"}, "code-analysis": {"IA & Machine Learning", "Agents de code"},
	// IA & ML — modèles, inférence, voix, RAG
	"llm": {"IA & Machine Learning", "Inférence & LLM locaux"}, "machine-learning": {"IA & Machine Learning", "Modèles & Génération"},
	"deep-learning": {"IA & Machine Learning", "Modèles & Génération"}, "inference": {"IA & Machine Learning", "Inférence & LLM locaux"},
	"tts": {"IA & Machine Learning", "Voix & Audio"}, "text-to-speech": {"IA & Machine Learning", "Voix & Audio"},
	"ocr": {"IA & Machine Learning", "Modèles & Génération"}, "on-device-ai": {"IA & Machine Learning", "Inférence & LLM locaux"},
	"edge-ai": {"IA & Machine Learning", "Inférence & LLM locaux"}, "rag": {"IA & Machine Learning", "RAG & Mémoire"},
	// IA & ML — automatisation
	"n8n": {"DevOps & Infrastructure", "Automatisation & Workflows"}, "no-code-ai": {"IA & Machine Learning", "Frameworks d'agents"},
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
	"osint": {"Cybersécurité", "OSINT & Renseignement"}, "pentest": {"Cybersécurité", "Offensif & Reverse"},
	"pentesting": {"Cybersécurité", "Offensif & Reverse"}, "red-team": {"Cybersécurité", "Offensif & Reverse"},
	"vulnerability": {"Cybersécurité", "Détection & Vulnérabilités"}, "kerberoasting": {"Cybersécurité", "Offensif & Reverse"},
	"honeypot": {"Cybersécurité", "Détection & Vulnérabilités"}, "threat-detection": {"Cybersécurité", "Détection & Vulnérabilités"},
	"bugbounty": {"Cybersécurité", "Offensif & Reverse"},
	"sso":       {"Cybersécurité", "Identité, Accès & Bastions"}, "oauth": {"Cybersécurité", "Identité, Accès & Bastions"},
	"saml": {"Cybersécurité", "Identité, Accès & Bastions"}, "iam": {"Cybersécurité", "Identité, Accès & Bastions"},
	"captcha": {"Cybersécurité", "Identité, Accès & Bastions"},
	"vpn":     {"Cybersécurité", "Réseau, VPN & Filtrage"}, "wireguard": {"Cybersécurité", "Réseau, VPN & Filtrage"},
	"proxy": {"Cybersécurité", "Réseau, VPN & Filtrage"}, "tunnel": {"Cybersécurité", "Réseau, VPN & Filtrage"},
	"dns": {"Cybersécurité", "Réseau, VPN & Filtrage"}, "zero-trust": {"Cybersécurité", "Réseau, VPN & Filtrage"},
	"reverse-proxy": {"DevOps & Infrastructure", "PaaS & Self-hosting"}, "anti-censorship": {"Cybersécurité", "Réseau, VPN & Filtrage"},
	"secrets": {"Cybersécurité", "Secrets & Certificats"}, "credentials": {"Cybersécurité", "Secrets & Certificats"},
	"vault": {"Cybersécurité", "Secrets & Certificats"}, "certificate": {"Cybersécurité", "Secrets & Certificats"},
	"pii": {"Cybersécurité", "Secrets & Certificats"}, "encryption": {"Cybersécurité", "Secrets & Certificats"},
	"reverse-engineering": {"Cybersécurité", "Offensif & Reverse"}, "decompiler": {"Cybersécurité", "Offensif & Reverse"},
	"malware-analysis": {"Cybersécurité", "Offensif & Reverse"}, "binary": {"Cybersécurité", "Offensif & Reverse"},
	"bastion": {"Cybersécurité", "Identité, Accès & Bastions"}, "jump-server": {"Cybersécurité", "Identité, Accès & Bastions"},
	"pam":        {"Cybersécurité", "Identité, Accès & Bastions"},
	"compliance": {"Cybersécurité", "Audit & Conformité"}, "hardening": {"Cybersécurité", "Audit & Conformité"},
	// DevOps & Infrastructure — virtualisation / cloud
	"hypervisor": {"DevOps & Infrastructure", "Virtualisation & Cloud"}, "kvm": {"DevOps & Infrastructure", "Virtualisation & Cloud"},
	"xen": {"DevOps & Infrastructure", "Virtualisation & Cloud"}, "vmware": {"DevOps & Infrastructure", "Virtualisation & Cloud"},
	"iaas": {"DevOps & Infrastructure", "Virtualisation & Cloud"}, "openstack": {"DevOps & Infrastructure", "Virtualisation & Cloud"},
	"hyperconverged": {"DevOps & Infrastructure", "Virtualisation & Cloud"},
	// Systèmes d'exploitation
	"linux": {"Systèmes d'exploitation", "Distributions Linux"}, "distro": {"Systèmes d'exploitation", "Distributions Linux"},
	"immutable-os": {"Systèmes d'exploitation", "Distributions Linux"}, "nixos": {"Systèmes d'exploitation", "Distributions Linux"},
	"sysadmin": {"Systèmes d'exploitation", "Linux : Outils & Bureau"}, "windows": {"Systèmes d'exploitation", "Windows & Compatibilité"},
	"debloat": {"Systèmes d'exploitation", "Windows & Compatibilité"}, "window-manager": {"Systèmes d'exploitation", "Linux : Outils & Bureau"},
	"tiling": {"Systèmes d'exploitation", "Linux : Outils & Bureau"}, "menubar": {"Systèmes d'exploitation", "macOS"},
	"wine": {"Systèmes d'exploitation", "Windows & Compatibilité"}, "proton": {"Systèmes d'exploitation", "Windows & Compatibilité"},
	// Data & Analytics
	"bi": {"Data & Analytics", "Visualisation & BI"}, "data-visualization": {"Data & Analytics", "Visualisation & BI"},
	"dashboard-bi": {"Data & Analytics", "Visualisation & BI"},
	"etl":          {"Data & Analytics", "Pipelines & Data Engineering"}, "data-platform": {"Data & Analytics", "Pipelines & Data Engineering"},
	"data-lake": {"Data & Analytics", "Pipelines & Data Engineering"}, "data-cleaning": {"Data & Analytics", "Pipelines & Data Engineering"},
	"big-data": {"Data & Analytics", "Pipelines & Data Engineering"}, "streaming": {"Data & Analytics", "Pipelines & Data Engineering"},
	"kafka": {"Data & Analytics", "Pipelines & Data Engineering"}, "spark": {"Data & Analytics", "Pipelines & Data Engineering"},
	// Productivité & Collaboration
	"kanban": {"Productivité & Collaboration", "Projets & Tâches"}, "project-management": {"Productivité & Collaboration", "Projets & Tâches"},
	"sprint": {"Productivité & Collaboration", "Projets & Tâches"},
	"crm":    {"Productivité & Collaboration", "CRM, ERP & Gestion"}, "erp": {"Productivité & Collaboration", "CRM, ERP & Gestion"},
	"customer-support": {"Productivité & Collaboration", "CRM, ERP & Gestion"}, "live-chat": {"Productivité & Collaboration", "CRM, ERP & Gestion"},
	"itam": {"Productivité & Collaboration", "CRM, ERP & Gestion"}, "asset-management": {"Productivité & Collaboration", "CRM, ERP & Gestion"},
	"time-tracking": {"Productivité & Collaboration", "Projets & Tâches"},
	"notes":         {"Productivité & Collaboration", "Notes & Connaissances"}, "wiki": {"Productivité & Collaboration", "Notes & Connaissances"},
	"pkm": {"Productivité & Collaboration", "Notes & Connaissances"}, "knowledge-base": {"Productivité & Collaboration", "Notes & Connaissances"},
	"markdown-notes": {"Productivité & Collaboration", "Notes & Connaissances"},
	"budget":         {"Productivité & Collaboration", "Finance"}, "accounting": {"Productivité & Collaboration", "Finance"},
	"personal-finance": {"Productivité & Collaboration", "Finance"},
	// Utilitaires
	"pdf": {"Utilitaires", "Documents & PDF"}, "document-conversion": {"Utilitaires", "Documents & PDF"},
	"rss":   {"Productivité & Collaboration", "Veille & Lecture"},
	"media": {"Utilitaires", "Serveurs média & Streaming"}, "video-downloader": {"Utilitaires", "Téléchargement de médias"},
	"iptv": {"Utilitaires", "Serveurs média & Streaming"}, "youtube": {"Utilitaires", "Téléchargement de médias"},
	"remote-desktop": {"Utilitaires", "Accès distant"}, "vnc": {"Utilitaires", "Accès distant"}, "rdp": {"Utilitaires", "Accès distant"},
	"homepage": {"Utilitaires", "Dashboards & Homelab"}, "homelab": {"Utilitaires", "Dashboards & Homelab"},
	"screenshot":   {"Utilitaires", "Lecture, Édition & Capture"},
	"bootable-usb": {"Systèmes d'exploitation", "Boot & Installation"}, "live-usb": {"Systèmes d'exploitation", "Boot & Installation"},
	"kde": {"Systèmes d'exploitation", "Linux : Outils & Bureau"},
	// Électronique & Hardware
	"pcb": {"Électronique & Hardware", "Conception & Fabrication"}, "fpga": {"Électronique & Hardware", "Conception & Fabrication"},
	"verilog": {"Électronique & Hardware", "Conception & Fabrication"}, "hardware": {"Électronique & Hardware", "Conception & Fabrication"},
	"electronics": {"Électronique & Hardware", "Conception & Fabrication"},
	// Documentation & Learning
	"awesome-list": {"Documentation & Learning", "Awesome lists & Annuaires"}, "awesome": {"Documentation & Learning", "Awesome lists & Annuaires"},
	"tutorial": {"Documentation & Learning", "Cours & Formation"}, "course": {"Documentation & Learning", "Cours & Formation"},
	"roadmap": {"Documentation & Learning", "Guides & Références"}, "learning": {"Documentation & Learning", "Cours & Formation"},
	// Développement — éditeurs, terminal, diagrammes, API, low-code, environnements
	"text-editor": {"Développement", "Éditeurs & IDE"}, "ide": {"Développement", "Éditeurs & IDE"},
	"terminal": {"Développement", "Terminal & CLI"}, "tui": {"Développement", "Terminal & CLI"},
	"terminal-ui": {"Développement", "Terminal & CLI"},
	"diagrams":    {"Développement", "Diagrammes & Visualisation"}, "visualization": {"Développement", "Diagrammes & Visualisation"},
	"flowchart": {"Développement", "Diagrammes & Visualisation"}, "erd": {"Développement", "Diagrammes & Visualisation"},
	"api-testing": {"Développement", "API & Tests"}, "rest-client": {"Développement", "API & Tests"},
	"low-code": {"Développement", "Low-code & Outils internes"}, "no-code": {"Développement", "Low-code & Outils internes"},
	"package-manager": {"Développement", "Environnements & Utilitaires"}, "dev-environment": {"Développement", "Environnements & Utilitaires"},
	"nix": {"Développement", "Environnements & Utilitaires"}, "cli-tool": {"Développement", "Environnements & Utilitaires"},
	// Catégories ajoutées lors du reclassement (sept. 2026)
	"iot": {"Électronique & Hardware", "Embarqué & IoT"}, "esp32": {"Électronique & Hardware", "Embarqué & IoT"},
	"arduino": {"Électronique & Hardware", "Embarqué & IoT"}, "home-assistant": {"Électronique & Hardware", "Embarqué & IoT"},
	"macos": {"Systèmes d'exploitation", "macOS"}, "android": {"Systèmes d'exploitation", "Mobile (Android & iOS)"},
	"ios":  {"Systèmes d'exploitation", "Mobile (Android & iOS)"},
	"game": {"Loisirs & Vie perso", "Jeux & Émulation"}, "emulator": {"Loisirs & Vie perso", "Jeux & Émulation"},
	"game-engine": {"Loisirs & Vie perso", "Jeux & Émulation"}, "fitness": {"Loisirs & Vie perso", "Sport & Santé"},
	"workout": {"Loisirs & Vie perso", "Sport & Santé"}, "recipes": {"Loisirs & Vie perso", "Vie pratique"},
	"design-system": {"Développement", "Design & UI"}, "figma-alternative": {"Développement", "Design & UI"},
	"backup": {"Utilitaires", "Fichiers, Sync & Sauvegarde"}, "file-sync": {"Utilitaires", "Fichiers, Sync & Sauvegarde"},
	"speech-to-text": {"IA & Machine Learning", "Voix & Audio"}, "whisper": {"IA & Machine Learning", "Voix & Audio"},
	"prompts": {"IA & Machine Learning", "Skills & Prompts"}, "chatbot": {"IA & Machine Learning", "Assistants & Chat"},
	"vector-database": {"IA & Machine Learning", "RAG & Mémoire"},
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
