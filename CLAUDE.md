# CLAUDE.md — Bookmark Manager

## 1. Contexte du projet

Site statique de gestion de marque-pages techniques, déployé sur **GitHub Pages** (branche `main`).
Test local : `docker compose up -d` → http://localhost:8080 (nginx avec volumes)
Test avec build Docker : `docker compose -f compose.yml up` → http://localhost:80

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
| `index.html` | Structure + scripts audio, Three.js, persistance localStorage, panels, thème |
| `styles.css` | 7 lignes `@import` vers `css/` — ne pas éditer directement |
| `css/variables.css` | Variables CSS + tokens Glass Apple + Google Fonts import |
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
| `app.link.txt` | Archive historique de tous les liens jamais traités (append only) |
| `scripts/migrate.py` | Génère/met à jour `bookmarks.csv` depuis git history + `bookmarks.html` |
| `scripts/enrich_meta.py` | Script d'injection manuelle de description+tags dans `bookmarks.csv` |
| `ambient.mp3` | Musique d'ambiance |
| `docker-compose.yml` | Nginx:alpine avec volumes — test local rapide |
| `compose.yml` | Build via Dockerfile — test de l'image de prod |
| `Dockerfile` | Image nginx:stable-alpine avec copie des fichiers statiques |
| `nginx.conf` | Config nginx personnalisée |
| `manifest.json` | PWA manifest |
| `sw.js` | Service Worker (cache offline) |

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

`bookmarks.html` est conservé comme artifact browser-importable et comme source pour `scripts/migrate.py` (qui traque les dates d'ajout via git diff). Il **n'est plus fetchépar le frontend**.

### Format bookmarks.csv

```csv
url,name,category,subcategory,description,tags,date_added
https://example.com,Nom Outil,Catégorie,Sous-catégorie,Description courte en français,tag1|tag2|tag3,1767298944
```

- `tags` : séparés par `|`
- `date_added` : timestamp Unix (rempli auto par `scripts/migrate.py`)
- `description` : 1 phrase en français, cas d'usage principal (rempli manuellement)

---

## 4. Processus d'ajout de liens (`app-new.link.txt` → `bookmarks.html`)

### Étape 1 — Identifier le site officiel

Pour chaque URL (surtout les GitHub) :
1. WebFetch la page GitHub → chercher le champ "Website/Homepage"
2. Lire la description courte du repo
3. Si pas de homepage → WebSearch `"[nom outil]" official site OR documentation`
4. Préférer : site officiel > docs officielles > page GitHub en dernier recours

### Étape 2 — Comprendre l'outil

Répondre mentalement avant toute catégorisation :
- **Quel problème résout-il ?** (cas d'usage principal, pas secondaire)
- **Qui l'utilise ?** Dev / DevOps / Data / Sécu / Tout le monde ?
- **Open-source ou SaaS fermé ? Self-hostable ?**
- **Existe-t-il déjà dans les bookmarks sous une autre URL ?**

### Étape 3 — Vérification doublons

```
Grep pattern: "domaine.com" dans bookmarks.html
```

### Étape 4 — Choisir la catégorie

**Règle : catégoriser selon le cas d'usage PRINCIPAL, pas les capacités secondaires.**

| Si l'outil est… | Catégorie |
|---|---|
| Logs, traces, métriques, alertes, APM | Développement > Monitoring & Observability |
| Framework web, SSR, générateur de sites, composants UI | Développement > Frameworks Frontend |
| Langage de programmation, shell, runtime | Développement > Langages & Shells |
| Terminal, éditeur de code, IDE, outils CLI shell | Développement > Éditeurs & IDE ou UI/Terminal |
| CI/CD, pipeline, orchestration de tâches, GitOps | Développement > CI/CD & Automation |
| IaC, provisioning, config management | Développement > Infrastructure as Code |
| Docker, K8s, conteneurs, runtime OCI, Tailscale+Docker | Développement > Conteneurs & Orchestration |
| PaaS self-hosted, déploiement simplifié | Développement > Platform Engineering |
| LLM, IA générative, TTS, STT, vision, inférence locale | IA & Machine Learning > Plateformes & Modèles |
| Agent IA de dev, MCP server, skill, coding assistant | IA & Machine Learning > Agents & Outils IA de dev |
| Automatisation IA, templates de workflows | IA & Machine Learning > Workflows & Automation IA |
| SQL client, ORM, migration, admin DB | Bases de données > Outils DB |
| Base relationnelle, distribution PostgreSQL | Bases de données > Bases relationnelles |
| NoSQL, cache, key-value | Bases de données > Bases NoSQL & Cache |
| OLAP, analytique colonaire | Bases de données > Analytique & OLAP |
| Dashboard BI, visualisation de données | Data & Analytics > Business Intelligence |
| ETL, data platform, data lake, data cleaning | Data & Analytics > Data Platforms |
| Streaming, big data, pipeline temps-réel | Data & Analytics > Big Data & Streaming |
| Scanner réseau, OSINT, threat intel, honeypot | Cybersécurité > Threat Detection & Analysis |
| IAM, SSO, auth, OIDC | Cybersécurité > Authentication & IAM |
| VPN, tunnel, proxy, accès réseau, DNS | Cybersécurité > Réseau & Accès |
| Secrets, credentials, certificats | Cybersécurité > Secrets & Credentials |
| Reverse engineering, forensics, désassemblage | Cybersécurité > Reverse Engineering |
| PAM, bastion, jump server | Cybersécurité > PAM & Jump Servers |
| Audit, compliance, scan de sécurité | Cybersécurité > Audit & Compliance |
| Hyperviseur (bare-metal, type 1/2) | Virtualisation & Infrastructure > Hyperviseurs |
| Cloud infrastructure, IaaS | Virtualisation & Infrastructure > Cloud Infrastructure |
| OS immuable, distro Linux, firmware | Systèmes d'exploitation > Linux |
| Outil système, monitoring OS, gestionnaire de paquets | Systèmes d'exploitation > Outils système |
| Compatibilité Wine, Proton | Systèmes d'exploitation > Compatibilité Windows |
| Suite bureautique, PDF, conversion de fichiers | Utilitaires > Documents & PDF |
| Media, streaming, download, IPTV, audio | Utilitaires > Médias & Multimédia |
| Remote desktop, accès distant | Utilitaires > Remote Desktop |
| Homepage, dashboard perso, homelab dashboard | Utilitaires > Dashboards & Homepages |
| Boot, live USB, scan disque | Utilitaires > Système & Boot |
| Gestion de projet, kanban, ticketing, sprints | Productivité & Collaboration > Gestion de projet |
| CRM, ERP, gestion commerciale | Productivité & Collaboration > CRM & ERP |
| Gestion d'actifs IT, CMDB | Productivité & Collaboration > Gestion d'actifs IT |
| Notes, wiki, PKM, knowledge management | Productivité & Collaboration > Notes & Documentation |
| Comptabilité, budget, facturation | Productivité & Collaboration > Finance |
| PCB, FPGA, Verilog, hardware design, télémétrie série | Électronique & Hardware |
| Blog technique, cours, roadmap, guide, awesome-list | Documentation & Learning |
| Émulateur, projet indie, fitness, sans catégorie claire | Expérimental & Projets GitHub |

**Cas ambigus :**
- Monitoring ET IaC → l'usage premier ? Si surveiller → Monitoring.
- IA qui génère du code → si principalement LLM/assistant → IA & ML. Si IDE avec IA → Éditeurs & IDE.
- Data ET viz → si la viz est le produit → BI. Si moteur de données → Data Platforms.
- Sécurité ET automation → la sécurité prime toujours.
- Awesome-list sur un sujet sécu → Documentation & Learning (pas Cybersécurité).
- Pas de catégorie évidente → Expérimental (créer une sous-catégorie si 2-3 outils similaires).

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
PYTHONIOENCODING=utf-8 python3 scripts/migrate.py
```

Puis enrichir **manuellement** les nouvelles entrées dans `bookmarks.csv` :
- `description` : 1 phrase en français résumant le cas d'usage principal
- `tags` : mots-clés séparés par `|` (ex: `docker|container|devops`)

### Étape 8 — Rapport final

| URL | Nom | Catégorie | Sous-catégorie | Statut |
|---|---|---|---|---|

Statut = `ajouté`, `doublon ignoré`, ou `catégorie à confirmer`

### Interdictions

- Ne jamais catégoriser sans avoir visité l'URL (WebFetch obligatoire)
- Ne jamais mettre dans "Expérimental" si une catégorie existe
- Ne jamais modifier les liens existants ni la structure HTML
- Ne jamais oublier l'archivage dans `app.link.txt` et la mise à jour du CSV

---

## 5. Site web — référence technique

### Système de thèmes

Sélecteur dans le header → attribut `data-theme` sur `body` → override CSS dans `css/themes.css`.

| Thème | data-theme | Particularité |
|---|---|---|
| Dark (défaut) | `dark` | Grille Three.js blanche |
| Light | `light` | Canvas Three.js masqué |
| Apple Dark | `apple-dark` | Liquid Glass : backdrop-filter saturate(180%) blur(24px) |
| Apple Light | `apple-light` | Liquid Glass sur fond clair |
| Nord | `nord` | Palette nordique, canvas 40% opacity |
| Dracula | `dracula` | Palette violette, canvas 35% opacity |

Les couleurs Three.js sont mises à jour via `window.updateScene3DColors(cfg)` depuis `script.js`.

### Typographie fluide

```css
html { font-size: clamp(13px, 0.43vw + 9.7px, 22px); }
--sidebar-width: clamp(220px, 20vw, 500px);
```

Toutes les tailles sont en `rem` pour s'adapter automatiquement à la résolution.

### Features UI

| Feature | Déclencheur | localStorage |
|---|---|---|
| Nouveautés | Bouton header | `lastVisitDate` |
| Favoris | Bouton header ★ | `favorites` (tableau d'URLs) |
| Tags | Clic pill tag | Filtre la recherche |
| Thème | Sélecteur header | `theme` |
| Audio | Bouton speaker | `audioEnabled` |
| Scroll | Auto `beforeunload` | `scrollPosition`, `sidebarScroll`, `contentScroll` |

**Logique audio** : si `audioEnabled === 'false'` → ne jamais relancer automatiquement.

### Navigation clavier

`↑/↓` dossiers · `→` bookmarks · `←` retour dossiers · `Enter` ouvrir · `Espace` toggle audio · `M` mute · `/` recherche · `Esc` quitter · `F` plein écran sidebar · `C` copier URL · `?` aide raccourcis

---

## 6. Notes de décisions de catégorisation

| Outil | Décision | Raison |
|---|---|---|
| Incus OS | Systèmes d'exploitation > Linux | OS immuable pour hôtes LXC/Incus, pas un hyperviseur |
| Maester | Développement > Langages & Shells | Framework de tests PowerShell, pas CI/CD |
| Datus | Data & Analytics > Big Data & Streaming | Le produit est data engineering, pas IA |
| Bamqam | Expérimental | Carte de suivi militaire, pas un outil dev |
| fzf / bat / Starship / Yazi | Outils de développement > UI/Terminal | Outils CLI/shell améliorant le terminal, pas des langages |
| wger | Expérimental | Application fitness, aucune catégorie dev existante |
| melonDS / Tanuki3DS | Expérimental | Émulateurs, hors périmètre dev |
| GhostVM | Expérimental | Workspaces macOS isolés, pas un hyperviseur classique |
| Digital Forensics Guide | Documentation & Learning | Guide pédagogique, malgré le sujet sécu |
| Awesome Connected Things Sec | Documentation & Learning | Awesome-list éducative, pas un outil actif |
| Serial Studio | Électronique & Hardware | Télémétrie UART/CAN/BLE, usage hardware embarqué |
| Verilator | Électronique & Hardware | Simulateur Verilog/SystemVerilog pour FPGA |
| DockTail | Développement > Conteneurs & Orchestration | Expose Docker via Tailscale, usage conteneurs |
| httpSMS / SMS Gateway | Outils de développement > API & Testing | API SMS via Android, usage dev/intégration |
| OpenRefine | Data & Analytics > Data Platforms | Nettoyage de données, pas BI (pas de viz) |
| KNIME | Data & Analytics > Data Platforms | ETL et pipelines data avant tout |
