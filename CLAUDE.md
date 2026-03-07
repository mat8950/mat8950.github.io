# CLAUDE.md — Bookmark Manager

---

## 1. Contexte du projet

Site statique de gestion de marque-pages techniques, déployé sur **GitHub Pages** (branche `main`).
Test local : `docker compose up -d` → http://localhost:8080

**Stack** : HTML / CSS / JS vanilla + Three.js (fond 3D WebGL) + `bookmarks.html` (format Netscape)

**Valeurs du projet** — avant d'ajouter un outil, il doit correspondre à au moins un de ces critères :
- Open-source ou source-available
- Self-hostable (pas uniquement SaaS fermé)
- Utile dans un contexte DevOps, développement, sécurité, data ou productivité pro
- Projet sérieux : actif, documenté, avec une communauté ou un usage réel

---

## 2. Gestion des bookmarks

### Fichiers clés
- `bookmarks.html` — source unique des données, format Netscape Bookmark
- `app-new.link.txt` — liens à ajouter (effacer après traitement)
- `script.js` — parse `bookmarks.html` au chargement

### Processus d'ajout de liens

#### Étape 1 — Identifier le site officiel

Pour chaque URL reçue (surtout les GitHub) :

1. **Vérifier le champ `homepage`** du repo : WebFetch `https://github.com/[owner]/[repo]` et chercher le lien "Website" ou "Homepage" dans la page
2. **Lire la description** (1ère ligne README ou description courte du repo)
3. **Si pas de homepage** → WebSearch `"[nom outil]" official site OR documentation`
4. **Préférer** dans cet ordre : site officiel > docs officielles > page GitHub si rien d'autre n'existe

#### Étape 2 — Comprendre l'outil

Avant toute catégorisation, répondre mentalement à ces questions :

> - **Quel problème résout-il ?** (1 phrase, cas d'usage principal — pas secondaire)
> - **Qui l'utilise ?** Dev backend / Frontend / DevOps / Data engineer / Analyste / Sécu / Tout le monde ?
> - **Quand l'utilise-t-on ?** En développement, en production, en audit, en exploration ?
> - **Open-source ou commercial ?** Self-hosted ou SaaS ?
> - **A-t-il un équivalent déjà dans les bookmarks ?** Si oui, est-ce vraiment différent ?

#### Étape 3 — Choisir la catégorie

Règle principale : **catégoriser selon le cas d'usage principal, pas les capacités secondaires.**

| Si l'outil est... | Catégorie |
|---|---|
| Moteur de recherche, logs, traces, métriques, alertes | Développement > Monitoring & Observability |
| Framework web, SSR, générateur de sites | Développement > Frameworks Frontend |
| Langage de programmation, shell, runtime | Développement > Langages & Shells |
| Terminal, éditeur de code, IDE | Développement > Éditeurs & IDE ou UI/Terminal |
| CI/CD, pipeline, orchestration de tâches, automation | Développement > CI/CD & Automation |
| IaC, provisioning, config management | Développement > Infrastructure as Code |
| Docker, K8s, conteneurs, runtime OCI | Développement > Conteneurs & Orchestration |
| Déploiement simplifié, PaaS self-hosted | Développement > Platform Engineering |
| LLM, IA générative, TTS, STT, vision, agent IA | IA & Machine Learning |
| MCP server, skill marketplace pour agents | IA & Machine Learning |
| SQL client, ORM, migration, DB admin | Bases de données > Outils DB |
| Base relationnelle, NoSQL, cache, OLAP | Bases de données > sous-catégorie correspondante |
| Dashboard BI, visualisation de données | Data & Analytics > Business Intelligence |
| Pipeline de données, ETL, data platform | Data & Analytics > Data Platforms ou Big Data & Streaming |
| Scanner réseau, OSINT, threat intel | Cybersécurité > Threat Detection & Analysis |
| IAM, SSO, auth | Cybersécurité > Authentication & IAM |
| VPN, tunnel, proxy, accès réseau | Cybersécurité > Réseau & Accès |
| Gestion de secrets, credentials | Cybersécurité > Secrets & Credentials |
| Reverse engineering, forensics | Cybersécurité > Reverse Engineering |
| PAM, bastion, jump server | Cybersécurité > PAM & Jump Servers |
| Hyperviseur (bare-metal, type 1/2) | Virtualisation & Infrastructure > Hyperviseurs |
| OS immuable, distro Linux, firmware | Systèmes d'exploitation > Linux |
| Outil système, monitoring OS, gestionnaire | Systèmes d'exploitation > Outils système |
| Suite bureautique, PDF, signature | Utilitaires > Documents & PDF |
| Media, streaming, download, IPTV | Utilitaires > Médias & Multimédia |
| Remote desktop, accès distant | Utilitaires > Remote Desktop |
| Homepage, dashboard perso | Utilitaires > Dashboards & Homepages |
| Gestion de projet, kanban, ticketing | Productivité & Collaboration > Gestion de projet |
| CRM, ERP, gestion commerciale | Productivité & Collaboration > CRM & ERP |
| Notes, wiki, documentation d'équipe | Productivité & Collaboration > Notes & Documentation |
| Comptabilité, budget personnel | Productivité & Collaboration > Finance |
| Blog technique, cours, roadmap, newsletter | Documentation & Learning |
| Prototype, expérimental, projet indie sans site | Expérimental & Projets GitHub |
| PCB, hardware, électronique | Électronique & Hardware |

#### Cas ambigus — comment trancher

- **Un outil fait du monitoring ET de l'IaC** → quel est l'usage premier ? Si on le déploie pour surveiller → Monitoring.
- **Un outil IA qui génère du code** → s'il est principalement un LLM/assistant → IA & ML. S'il est un IDE avec IA intégrée → Éditeurs & IDE.
- **Un outil de data qui fait aussi de la viz** → si la viz est le produit → BI. Si c'est un moteur de données → Data Platforms.
- **Un outil de sécurité qui automatise** → la sécurité prime toujours sur l'automation.
- **Pas de catégorie évidente** → créer une nouvelle sous-catégorie si au moins 2-3 outils similaires existent, sinon mettre dans Expérimental.

#### Étape 4 — Vérification anti-doublons

Avant d'ajouter, grep le domaine dans `bookmarks.html` :
```
Grep pattern: "domaine.com" dans bookmarks.html
```

**Outils déjà présents (ne pas dupliquer)** :
`kestra.io`, `n8n.io`, `grafana.com`, `prometheus.io`, `github.com`, `docker.com`, `kubernetes.io`, `ansible.com`, `terraform.io`

#### Étape 5 — Format d'ajout

```html
<DT><A HREF="https://site-officiel.com" ADD_DATE="1741000000" LAST_MODIFIED="1741000000">Nom Outil</A>
```

Nouvelle sous-catégorie si besoin :
```html
<DT><H3 ADD_DATE="1741000000" LAST_MODIFIED="1741000000">Nom Catégorie</H3>
<DL><p>
    <DT><A HREF="...">...</A>
</DL><p>
```

---

## 3. Site web — référence technique

### Fichiers et rôles

| Fichier | Rôle |
|---|---|
| `index.html` | Structure + scripts audio, Three.js, persistance localStorage |
| `styles.css` | Thème dark/light, animations, layout sidebar/content |
| `script.js` | Parse bookmarks, rendu, navigation clavier, favoris, lazy loading |
| `bookmarks.html` | Données sources (format Netscape) |
| `ambient.mp3` | Musique d'ambiance |
| `docker-compose.yml` | Serveur nginx local pour les tests |

### Logique audio (localStorage)

| Valeur `audioEnabled` | Comportement au chargement |
|---|---|
| `null` (première visite) | Autoplay au premier clic/touche |
| `'true'` | Autoplay au premier clic/touche |
| `'false'` | Pas d'autoplay — l'utilisateur a explicitement mis pause |

Le son ne doit **jamais** se relancer automatiquement si `audioEnabled === 'false'`.

### Thème (localStorage)

- `theme = 'light'` → classe `light-mode` sur `body`
- Par défaut : dark

### Persistance scroll

Sauvegardé dans `beforeunload` : `scrollPosition`, `sidebarScroll`, `contentScroll`
Restauré au `load` avec un `setTimeout(200ms)` pour le contenu (qui charge de manière async).

### Navigation clavier

- `↑` / `↓` → dossiers visibles seulement (filtre `.folder-children:not(.show)`)
- `→` → passer aux bookmarks
- `←` → revenir aux dossiers
- `Enter` → ouvrir
- `Espace` → toggle audio
- `M` → mute/unmute
- `/` → focus recherche
- `Esc` → quitter recherche ou désélectionner

---

## 4. Notes de décisions passées

- **Incus OS** = OS immuable pour hôtes LXC/Incus — **ce n'est pas un hyperviseur** → catégorie Linux
- **Maester** = framework de tests PowerShell — va dans Langages & Shells, pas CI/CD
- **Datus** = data engineering agent IA — va dans Data & Analytics > Big Data, pas IA & ML (le produit est data, pas IA)
- **Bamqam** = carte de suivi d'opérations militaires — Expérimental (pas d'outil dev)
- La section `IA & Machine Learning` est à plat (pas de sous-catégories) car trop hétérogène pour l'instant
