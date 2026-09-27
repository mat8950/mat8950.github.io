# CLAUDE.md — Bookmark Manager

## 1. Contexte du projet

Site statique de gestion de marque-pages techniques, déployé sur **GitHub Pages** (branche `main`).
Déploiement : `.github/workflows/static.yml` publie une **liste blanche** de fichiers (dossier `_site/`). Tout nouveau fichier servi par le site doit y être ajouté (et dans `Dockerfile`), sinon il n'est pas en ligne.
Test local : `docker compose -f docker-compose.yml up -d` → http://localhost:8080 (nginx avec volumes, config nginx par défaut : pas de page 404 perso)
Test avec build Docker (proche prod) : `docker compose up -d --build` → http://localhost (sans `-f`, Compose prend `compose.yml` en priorité sur `docker-compose.yml`)

**Stack** : HTML / CSS / JS vanilla + Three.js (fond 3D WebGL) + `bookmarks.html` (format Netscape)

**Valeurs du projet** — avant d'ajouter un outil, il doit correspondre à au moins un de ces critères :
- Open-source ou source-available
- Self-hostable (pas uniquement SaaS fermé)
- Utile dans un contexte DevOps, développement, sécurité, data ou productivité pro
- Projet sérieux : actif, documenté, avec une communauté ou un usage réel

---

## 2. Fichiers clés

| Fichier | Rôle |
|---|---|
| `index.html` | Structure + scripts audio, Three.js, persistance localStorage, panels, thème. Charge directement les 8 fichiers `css/` (ordre = cascade : fonts → variables → base → background → layout → components → themes → responsive) |
| `css/fonts.css` | `@font-face` des polices auto-hébergées (`fonts/`, latin + latin-ext) — plus aucun appel Google Fonts |
| `css/variables.css` | Variables CSS + tokens Glass Apple (`--text-muted` calé à ≥ 4.5:1 de contraste par thème) |
| `css/base.css` | Reset, typographie fluide (`clamp`), body, animations |
| `css/background.css` | Fond CSS de fallback + overrides canvas par thème |
| `css/layout.css` | Header, sidebar, audio, recherche, sélecteur de thème |
| `css/components.css` | Bookmarks, dossiers, breadcrumb, état vide |
| `css/themes.css` | Overrides complets par thème (dark, light, apple-dark, apple-light, nord, dracula…) |
| `css/responsive.css` | Media queries 1440px+, 1920px+, 2560px+, tablet, mobile |
| `script.js` | Parse bookmarks + CSV, rendu, navigation clavier, favoris, lazy loading, thèmes 3D |
| `bookmarks.html` | Structure des données (catégories + liens, format Netscape) |
| `bookmarks.csv` | Métadonnées enrichies : `url, name, category, subcategory, description, tags, date_added` |
| `app-new.link.txt` | Liens à traiter (vider après traitement) |
| `app-new.meta.json` | Métadonnées pré-fetchées par `bm-fetch` (temporaire, non commité) |
| `app.link.txt` | Archive historique de tous les liens jamais traités (append only) |
| `go.mod` / `go.sum` | Module Go — outils scripts/ |
| `scripts/bm-fetch.go` | Pré-fetch metadata URLs → `app-new.meta.json` (API GitHub + og: tags), dédup par URL + par nom |
| `scripts/bm-suggest.go` | Suggère catégorie/sous-catégorie (mots-clés + précédent par tags) depuis `app-new.meta.json` — lecture seule, ne modifie rien |
| `scripts/bm-migrate.go` | Met à jour `bookmarks.csv` depuis git history + `bookmarks.html` |
| `scripts/bm-enrich.go` | Injecte descriptions/tags depuis la map META dans `bookmarks.csv` |
| `scripts/bm-stars.go` | Enrichit `bookmarks.csv` via API GitHub : `stars`, `pushed_at`, `archived`, `tags` (topics si vides) |
| `ambient.mp3` | Musique d'ambiance |
| `docker-compose.yml` | Nginx:alpine avec volumes — test local rapide |
| `compose.yml` | Build via Dockerfile — test de l'image de prod |
| `Dockerfile` | Image nginx:stable-alpine avec copie des fichiers statiques |
| `nginx.conf` | Config nginx personnalisée |
| `manifest.json` | PWA manifest |
| `sw.js` | Service Worker (cache offline) — incrémenter `CACHE` à chaque changement d'assets |
| `404.html` / `mentions-legales.html` | Pages statiques autonomes (style `css/page.css`, pas de `script.js`) |
| `robots.txt` / `sitemap.xml` | SEO — mettre à jour `sitemap.xml` si une page est ajoutée |
| `favicon.ico` / `icons/` / `og-image.png` | Favicon, icônes PWA (SVG + PNG + maskable + apple-touch), image de partage 1200×630 |
| `fonts/` / `lib/three.min.js` | Polices woff2 et Three.js r128 auto-hébergés (pas de CDN). Ne pas nommer un dossier `vendor/` : Go le prend pour du vendoring |
| Favicons des marque-pages | Chargés à l'affichage depuis `www.google.com/s2/favicons` (`referrerpolicy=no-referrer`), rien n'est stocké dans le dépôt. Seules les icônes du site (`favicon.ico`, `icons/`) sont versionnées |

---

## 3. Architecture données

`bookmarks.csv` est l'**unique source** chargée par le frontend :

```
bookmarks.csv  →  loadBookmarks() → parseCSV() → buildFromCSV()
                       ↓
          bookmarkData.folders[]   (catégories/sous-catégories)
          allBookmarks[]           (tous les liens)
          bookmarkMeta Map         (url → {description, tags, date_added, …})
```

`bookmarks.html` est conservé comme artifact browser-importable et comme source pour `scripts/bm-migrate.go` (qui traque les dates d'ajout via git diff). Il **n'est plus fetché par le frontend**.

### Format bookmarks.csv

```csv
url,name,category,subcategory,description,tags,date_added,stars,pushed_at,archived
https://example.com,Nom Outil,Catégorie,Sous-catégorie,Description courte en français,tag1|tag2|tag3,1767298944,1234,2026-05-01,
```

- `tags` : séparés par `|` (curés à la main / `bm-enrich`, sinon auto-remplis depuis les topics GitHub par `bm-stars`)
- `date_added` : timestamp Unix (rempli auto par `scripts/bm-migrate.go`)
- `description` : 1 phrase en français, cas d'usage principal (rempli manuellement)
- `stars` : étoiles GitHub (auto `bm-stars.go`)
- `pushed_at` : date du dernier push `YYYY-MM-DD` (auto `bm-stars.go`) → badge « inactif » au frontend si > 12 mois
- `archived` : `true` si dépôt archivé (auto `bm-stars.go`) → badge « 🗄 archivé » au frontend

---

## 4. Processus d'ajout de liens (`app-new.link.txt` → `bookmarks.html`)

### Étape 0 — Pré-fetch des métadonnées (**AVANT d'ouvrir Claude**)

```bash
go run ./scripts/bm-fetch.go
# Recommandé pour >20 URLs (évite la limite de 60 req/h GitHub non authentifié) :
GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-fetch.go
```

Lit `app-new.link.txt`, appelle l'API GitHub ou parse les og: tags pour chaque URL,
vérifie les doublons contre `bookmarks.csv`, et écrit `app-new.meta.json`.

### Étape 1 — Lire app-new.meta.json

```bash
go run ./scripts/bm-suggest.go   # optionnel, lecture seule — suggestions catégorie/précédent
```

Claude lit `app-new.meta.json` **sans effectuer aucun WebFetch**.

- `official_url` → URL à insérer dans bookmarks.html (si vide → utiliser `input_url`)
- `name` → nom de l'outil
- `description_raw` → base pour rédiger la description française
- `github_topics` → aide à la catégorisation
- `is_duplicate: true` → ignorer (noter dans le rapport)
- `possible_dup_by_name: true` → nom proche d'une entrée existante (`similar_existing_url`) sous un domaine différent (ex: fork, site officiel vs repo GitHub) — vérifier avant d'ajouter, ne pas ignorer automatiquement
- Suggestions de `bm-suggest.go` : indicatives seulement, le jugement (table de la section 4 étape 4) prime en cas de divergence

### Étape 2 — Comprendre l'outil

À partir des données de `app-new.meta.json` :
- **Quel problème résout-il ?** (cas d'usage principal, pas secondaire)
- **Qui l'utilise ?** Dev / DevOps / Data / Sécu / Tout le monde ?
- **Open-source ou SaaS fermé ? Self-hostable ?**

### Étape 3 — Vérification doublons

`is_duplicate` dans `app-new.meta.json` est déjà calculé par `bm-fetch`.
Vérification complémentaire si doute :
```
Grep pattern: "domaine.com" dans bookmarks.csv
```

### Étape 4 — Choisir la catégorie

**Règle : catégoriser selon le cas d'usage PRINCIPAL, pas les capacités secondaires.**

Arborescence réorganisée le 2026-09-21 : 12 catégories, toutes avec sous-catégories (pas de lien « à la racine » d'une catégorie). Le frontend trie les catégories par nombre de liens et les sous-catégories par ordre alphabétique : l'ordre dans le CSV / HTML n'a pas d'importance.

| Si l'outil est… | Catégorie |
|---|---|
| Assistant conversationnel, moteur de recherche IA, assistant perso | IA & Machine Learning > Assistants & Chat |
| Exécution locale de LLM, moteur d'inférence, passerelle LLM, fine-tuning | IA & Machine Learning > Inférence & LLM locaux |
| Modèle ou framework ML, génération image / 3D / vidéo / musique, OCR, vision | IA & Machine Learning > Modèles & Génération |
| TTS, STT, dictée, clonage de voix, traduction vocale | IA & Machine Learning > Voix & Audio |
| Agent de code, outil autour de Claude Code / Codex, intelligence de codebase | IA & Machine Learning > Agents de code |
| Skill, prompt, registre / collection de skills | IA & Machine Learning > Skills & Prompts |
| Serveur MCP, navigateur ou connecteur pour agents | IA & Machine Learning > MCP & Outils pour agents |
| Framework / plateforme / sandbox / évaluation d'agents, builder visuel | IA & Machine Learning > Frameworks d'agents |
| RAG, GraphRAG, mémoire d'agent, index vectoriel | IA & Machine Learning > RAG & Mémoire |
| Application IA finale (rédaction, recherche scientifique, génération de contenu) | IA & Machine Learning > Applications IA |
| Langage, shell, framework back-end, bibliothèque | Développement > Langages & Bibliothèques |
| Framework web, composants UI, CMS headless, templates de dashboard | Développement > Front-end & UI |
| Outil de design, maquettage, systèmes de design (DESIGN.md) | Développement > Design & UI |
| Terminal, outil CLI / TUI, prompt shell | Développement > Terminal & CLI |
| Éditeur de code, IDE | Développement > Éditeurs & IDE |
| Forge Git, client Git | Développement > Git & Forges |
| Client API, mock, données de test, passerelle SMS / email pour dev | Développement > API & Tests |
| Diagrammes as code, schémas, visualisation d'architecture | Développement > Diagrammes & Visualisation |
| Low-code, outils internes | Développement > Low-code & Outils internes |
| Environnement de dev, gestionnaire de paquets, boîte à outils dev | Développement > Environnements & Utilitaires |
| Docker, Kubernetes, runtime OCI, templates compose | DevOps & Infrastructure > Conteneurs & Kubernetes |
| CI/CD, GitOps, release, feature flags | DevOps & Infrastructure > CI/CD & Release |
| Orchestration de tâches / workflows, n8n, runbooks | DevOps & Infrastructure > Automatisation & Workflows |
| IaC, provisioning, config management, émulateur cloud | DevOps & Infrastructure > Infrastructure as Code |
| Logs, traces, métriques, alertes, APM, supervision réseau | DevOps & Infrastructure > Monitoring & Observabilité |
| PaaS self-hosted, panneau d'hébergement, gestion de reverse proxy | DevOps & Infrastructure > PaaS & Self-hosting |
| Hyperviseur, VM, cloud privé / IaaS, stockage objet | DevOps & Infrastructure > Virtualisation & Cloud |
| OSINT, fuite de données, reconnaissance | Cybersécurité > OSINT & Renseignement |
| Pentest, outil offensif, reverse engineering | Cybersécurité > Offensif & Reverse |
| IDS, SIEM, honeypot, threat intel, CVE, gestion des vulnérabilités | Cybersécurité > Détection & Vulnérabilités |
| Audit, conformité, hardening | Cybersécurité > Audit & Conformité |
| IAM, SSO, auth, captcha, PAM, bastion | Cybersécurité > Identité, Accès & Bastions |
| Secrets, credentials, certificats, ACME | Cybersécurité > Secrets & Certificats |
| VPN, tunnel, zero trust, pare-feu, DNS / filtrage de pubs, anti-censure | Cybersécurité > Réseau, VPN & Filtrage |
| SQL client, ORM, sauvegarde, pooler, admin DB | Bases de données > Outils DB |
| Base relationnelle, distribution PostgreSQL | Bases de données > Bases relationnelles |
| NoSQL, cache, key-value | Bases de données > Bases NoSQL & Cache |
| OLAP, time-series, analytique colonaire | Bases de données > Analytique & OLAP |
| ETL, data engineering, big data, streaming, nettoyage de données | Data & Analytics > Pipelines & Data Engineering |
| BI, dashboards data, visualisation, géodonnées | Data & Analytics > Visualisation & BI |
| Distribution Linux, OS immuable, firmware | Systèmes d'exploitation > Distributions Linux |
| Outil / personnalisation Linux, admin serveur Linux | Systèmes d'exploitation > Linux : Outils & Bureau |
| Application ou tweak macOS | Systèmes d'exploitation > macOS |
| Outil / debloat Windows, Wine, Proton, apps Windows sous Linux | Systèmes d'exploitation > Windows & Compatibilité |
| App Android / iOS, Android sous Linux | Systèmes d'exploitation > Mobile (Android & iOS) |
| Clé USB multiboot, bootloader, test de distributions | Systèmes d'exploitation > Boot & Installation |
| Suite bureautique, PDF, conversion, scan, composition (Typst) | Utilitaires > Documents & PDF |
| Serveur média, IPTV, streaming, outils Jellyfin / Emby | Utilitaires > Serveurs média & Streaming |
| Téléchargement vidéo / musique | Utilitaires > Téléchargement de médias |
| Lecteur, édition audio / vidéo / photo, capture d'écran | Utilitaires > Lecture, Édition & Capture |
| Synchronisation, partage, sauvegarde de fichiers | Utilitaires > Fichiers, Sync & Sauvegarde |
| Remote desktop, accès SSH / VDI | Utilitaires > Accès distant |
| Homepage, dashboard perso, homelab | Utilitaires > Dashboards & Homelab |
| Gestion de projet, kanban, tâches, suivi du temps | Productivité & Collaboration > Projets & Tâches |
| Messagerie d'équipe, email, suite collaborative, réseaux sociaux | Productivité & Collaboration > Communication & Suites |
| Notes, wiki, PKM, base de connaissances | Productivité & Collaboration > Notes & Connaissances |
| CRM, ERP, support client, gestion d'actifs IT | Productivité & Collaboration > CRM, ERP & Gestion |
| Budget, comptabilité, facturation, trading | Productivité & Collaboration > Finance |
| RSS, newsletter, lecture, tendances GitHub | Productivité & Collaboration > Veille & Lecture |
| Cours, formation interactive, labs | Documentation & Learning > Cours & Formation |
| Guide, blog technique, roadmap, papers, system design | Documentation & Learning > Guides & Références |
| Awesome-list, annuaire, landscape, répertoire d'alternatives | Documentation & Learning > Awesome lists & Annuaires |
| Microcontrôleur, ESP32, IoT, domotique, télémétrie série | Électronique & Hardware > Embarqué & IoT |
| PCB, FPGA, Verilog, impression 3D, hardware open-source | Électronique & Hardware > Conception & Fabrication |
| Jeu, moteur de jeu, émulateur, cloud gaming | Loisirs & Vie perso > Jeux & Émulation |
| Fitness, nutrition, sport | Loisirs & Vie perso > Sport & Santé |
| Cuisine, voyage, cartes, immobilier, suivi de prix, apprentissage perso | Loisirs & Vie perso > Vie pratique |

**Cas ambigus :**
- Monitoring ET IaC → l'usage premier ? Si surveiller → Monitoring.
- IA qui génère du code → agent / assistant de code → IA > Agents de code. IDE avec IA intégrée → Développement > Éditeurs & IDE.
- Data ET viz → si la viz est le produit → Visualisation & BI. Si moteur de données → Pipelines & Data Engineering.
- Sécurité ET automation → la sécurité prime toujours.
- Awesome-list, quel que soit le sujet (sécu, IA, jeux…) → Documentation & Learning > Awesome lists & Annuaires. Exception : les collections de skills / prompts installables → IA > Skills & Prompts.
- Outil destiné à un OS précis (app macOS, debloat Windows) → la sous-catégorie de cet OS, même si c'est « média » ou « système ».
- Pas de catégorie évidente → chercher la plus proche ; créer une sous-catégorie si 3+ outils similaires. Il n'y a plus de catégorie fourre-tout « Expérimental ».

### Étape 5 — Insérer dans bookmarks.html

Format exact (espaces, pas de tabs) :
```html
            <DT><A HREF="https://site-officiel.com" ADD_DATE="1775433600" LAST_MODIFIED="1775433600">Nom Outil</A>
```

Nouvelle sous-catégorie si besoin :
```html
        <DT><H3 ADD_DATE="1775433600" LAST_MODIFIED="1775433600">Nom Sous-catégorie</H3>
        <DL><p>
            <DT><A HREF="...">...</A>
        </DL><p>
```

Utiliser le timestamp Unix du jour pour ADD_DATE et LAST_MODIFIED.

### Étape 6 — Archiver et vider

1. Copier les URLs traitées à la fin de `app.link.txt`
2. Vider `app-new.link.txt`

### Étape 7 — Mettre à jour le CSV

```bash
go run ./scripts/bm-migrate.go                       # CSV ← bookmarks.html + git history (dates)
go run ./scripts/bm-enrich.go                        # descriptions/tags curés depuis la map META
GITHUB_TOKEN=ghp_xxx go run ./scripts/bm-stars.go    # stars + pushed_at + archived + tags (topics si vides)
```

Ordre important : `bm-stars` en dernier — il ne remplit les `tags` que s'ils sont vides, donc les tags curés par `bm-enrich` priment. Token recommandé (181+ dépôts, limite 60 req/h sans token).

Puis enrichir **manuellement** dans `bookmarks.csv` les nouvelles entrées non couvertes par `bm-enrich` :
- `description` : 1 phrase en français résumant le cas d'usage principal
- `tags` : mots-clés séparés par `|` (ex: `docker|container|devops`)

Ajouter les nouvelles descriptions à `scripts/bm-enrich.go` (map META) pour les batchs futurs.

### Étape 8 — Rapport final

| URL | Nom | Catégorie | Sous-catégorie | Statut |
|---|---|---|---|---|

Statut = `ajouté`, `doublon ignoré`, ou `catégorie à confirmer`

### Interdictions

- Ne jamais catégoriser sans avoir lu l'entrée dans `app-new.meta.json` (généré par `bm-fetch`)
- Ne jamais laisser un lien à la racine d'une catégorie : toujours une sous-catégorie (existante, ou nouvelle si 3+ outils similaires)
- Ne jamais modifier les liens existants ni la structure HTML
- Ne jamais oublier l'archivage dans `app.link.txt` et la mise à jour du CSV

---

## 5. Site web — référence technique

### Système de thèmes

Sélecteur dans le header → attribut `data-theme` sur `body` → override CSS dans `css/themes.css`.

| Thème | data-theme | Particularité |
|---|---|---|
| Dark (défaut) | `dark` | Grille Three.js blanche, OLED pur |
| Light | `light` | Canvas Three.js masqué |
| Apple Dark | `apple-dark` | Liquid Glass : backdrop-filter saturate(180%) blur(24px) |
| Apple Light | `apple-light` | Liquid Glass chromatique sur fond lavande-gray (#e8eaf2), orbes bleu/violet/pêche |
| Nord | `nord` | Palette nordique aurora, canvas 35% opacity |
| Dracula | `dracula` | Palette violette, canvas 30% opacity, URLs cyan |
| Catppuccin Mocha | `catppuccin` | Pastels mauve/bleu sur #1e1e2e |
| Gruvbox Dark | `gruvbox` | Accents jaune/orange chaud sur #282828 |

Les couleurs Three.js sont mises à jour via `window.updateScene3DColors(cfg)` depuis `script.js`.

### Conventions CSS

- **Tokens d'état** (`css/variables.css`) : `--hover-bg`, `--active-bg`, `--gold` — surchargés par les thèmes clairs. Ne jamais écrire `rgba(255,255,255,…)` pour un survol : invisible sur Light / Apple Light.
- **Transitions** : `transition: var(--transition-ui)` (propriétés ciblées). Pas de `transition: all`.
- **Contraste** : pas d'`opacity` sur un texte informatif (URL, date, description, compteurs) — utiliser `--text-muted` / `--text-dim`, calés à ≥ 4.5:1 par thème. axe ne mesure pas les zones en verre (backdrop-filter) : vérifier au calcul.
- **Header desktop** de hauteur fixe `--header-h` : la sidebar collante s'y cale (`top` / `height`). Sur mobile (≤ 768px), header `auto` et sidebar `static`.
- **Thèmes Apple** : ne pas remettre `position: relative` sur `header` / `.sidebar` (casse le `sticky`).
- **Mouvement réduit** : bloc `prefers-reduced-motion` dans `base.css` ; Three.js n'est pas lancé (index.html).

### Typographie fluide

```css
html { font-size: clamp(13px, 0.43vw + 9.7px, 22px); }
--sidebar-width: clamp(220px, 20vw, 500px);
```

Toutes les tailles sont en `rem` pour s'adaater automatiquement à la résolution.

### Features UI

| Feature | Déclencheur | localStorage |
|---|---|---|
| Nouveautés | Bouton header | `lastVisitDate` |
| Favoris | Bouton header ★ | `favorites` (tableau d'URLs ordonné) |
| Stats | Bouton header `~ Stats` | — |
| Tags | Clic pill tag | Filtre actif (chip) |
| Tri | Barre sort-bar | `currentSort` |
| Filtre date | Select sort-bar | `activeFilters.dateRange` |
| Mode compact | Bouton `⊟` sort-bar | `compactMode` |
| Sidebar toggle | Raccourci `F` | `sidebarCollapsed` |
| Thème | Sélecteur header | `theme` |
| Audio | Bouton speaker | `audioEnabled`, `audioVolume` |
| Popup de bienvenue | 1re visite (aucun `audioEnabled` ni `welcomeSeen`) | `welcomeSeen` |
| Scroll | Auto `beforeunload` | `scrollPosition`, `sidebarScroll`, `contentScroll` |

**Logique audio** (anti « screamer ») :
- 1re visite → popup `<dialog id="welcome-dialog">` : « Lancer la musique » ou « Continuer sans musique » (focus par défaut, Échap = sans musique). Aucun son ni téléchargement de `ambient.mp3` avant ce choix.
- `audioEnabled === 'true'` → lecture au 1er clic / touche (geste requis par les navigateurs).
- `audioEnabled === 'false'` → ne jamais relancer automatiquement.
- Chaque démarrage fait un fondu d'entrée de 1,5 s jusqu'au volume réglé (30 % par défaut).

### Favoris — features
- Export JSON (bouton ↓ Export dans le panel)
- Import JSON (bouton ↑ Import, merge sans doublons)
- Drag & drop pour réordonner (handle `⠿`, ordre persisté dans `favorites[]`)

### Navigation clavier

`↑/↓` dossiers · `→` bookmarks · `←` retour dossiers · `Enter` ouvrir · `C` copier URL · `/` recherche · `Esc` quitter · `F` toggle sidebar · `?` overlay aide raccourcis · `Espace` toggle audio · `M` mute

### PWA

- `manifest.json` + `sw.js` (service worker)
- Cache-first pour assets statiques (polices mises en cache à la volée), network-first pour `bookmarks.csv`
- `ambient.mp3` n'est pas précaché et l'`<audio>` est en `preload="none"` : 4 Mo chargés seulement au premier PLAY
- Installable depuis Chrome/Edge via le bouton dans la barre d'adresse

### Import bookmarks externe

Zone `↑ Import .html` en bas de la sidebar : drag & drop ou clic → parse Netscape Bookmark (export Chrome/Firefox), merge sans doublons, re-render immédiat. **Ne modifie pas `bookmarks.html` sur disque** (merge en mémoire uniquement).

### Suggestions similaires

Bouton `~` sur chaque carte (visible au hover) → popover avec les 5 bookmarks ayant le plus de tags en commun. Calculé par `getSimilarBookmarks()` via overlap de tags CSV.

---

## 6. Notes de décisions de catégorisation

| Outil | Décision | Raison |
|---|---|---|
| Incus OS | Systèmes d'exploitation > Distributions Linux | OS immuable pour hôtes LXC/Incus, pas un hyperviseur |
| Maester | Cybersécurité > Audit & Conformité | Tests PowerShell de conformité sécurité (M365) : la sécurité prime sur le langage |
| Datus | Data & Analytics > Pipelines & Data Engineering | Le produit est data engineering, pas IA |
| Bamqam | Cybersécurité > OSINT & Renseignement | Carte collaborative d'opérations militaires : renseignement en sources ouvertes |
| fzf / bat / Starship / Yazi | Développement > Terminal & CLI | Outils CLI/shell améliorant le terminal, pas des langages |
| wger / Workout Cool / lyftr | Loisirs & Vie perso > Sport & Santé | Applications fitness |
| melonDS / Tanuki3DS | Loisirs & Vie perso > Jeux & Émulation | Émulateurs |
| GhostVM / VirtualBuddy | DevOps & Infrastructure > Virtualisation & Cloud | VM macOS isolées : virtualisation |
| Digital Forensics Guide | Documentation & Learning > Guides & Références | Guide pédagogique, malgré le sujet sécu |
| Awesome Connected Things Security | Documentation & Learning > Awesome lists & Annuaires | Awesome-list éducative, pas un outil actif |
| Serial Studio | Électronique & Hardware > Embarqué & IoT | Télémétrie UART/CAN/BLE, usage hardware embarqué |
| Verilator | Électronique & Hardware > Conception & Fabrication | Simulateur Verilog/SystemVerilog pour FPGA |
| DockTail | DevOps & Infrastructure > Conteneurs & Kubernetes | Expose Docker via Tailscale, usage conteneurs |
| httpSMS / SMS Gateway | Développement > API & Tests | API SMS via Android, usage dev/intégration |
| OpenRefine / KNIME | Data & Analytics > Pipelines & Data Engineering | Nettoyage de données / ETL, pas de viz |
| Awesome Agent Skills / awesome-claude-skills | IA & Machine Learning > Skills & Prompts | Catalogues de skills installables (exception à la règle awesome-list) |
| Sniffnet / NetFluss / LibreSpeed | DevOps & Infrastructure > Monitoring & Observabilité | Supervision réseau, pas de la détection de menace |
| CaddyManager / Traefik Manager / Pingora Proxy Manager | DevOps & Infrastructure > PaaS & Self-hosting | Interfaces d'hébergement / reverse proxy, pas de la sécurité |
| n8n / Kestra / Temporal / Windmill | DevOps & Infrastructure > Automatisation & Workflows | Orchestration de workflows, distincte du CI/CD |
| Apache Airflow | Data & Analytics > Pipelines & Data Engineering | Orchestration de pipelines data |
| Codeburn / Clawdmeter | IA > Agents de code / Électronique > Embarqué & IoT | Suivi de consommation Claude Code (TUI) / afficheur ESP32 |
