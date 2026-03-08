# Instructions pour Claude Code

## Ajout de nouveaux liens (app-new.link.txt → bookmarks.html)

Quand l'utilisateur demande d'ajouter les liens de `app-new.link.txt` dans `bookmarks.html`, suivre ces regles :

### 1. Recherche web obligatoire
Pour chaque URL du fichier, utilise `WebFetch` pour visiter la page et comprendre ce que le projet fait reellement. Ne devine jamais en te basant uniquement sur le nom.

### 2. Verification des doublons
Avant d'ajouter un lien, verifie qu'il n'existe pas deja dans bookmarks.html (meme URL ou meme projet sous une URL differente, ex: site officiel vs repo GitHub).

### 3. Classement dans la bonne sous-categorie

Structure actuelle :

```
Developpement/
  Editeurs & IDE
  Frameworks Frontend
  Langages & Shells
  Git & Version Control
  CI/CD & Automation
  Conteneurs & Orchestration
  Infrastructure as Code
  Monitoring & Observability
  Platform Engineering

Bases de donnees/
  Bases relationnelles
  Bases NoSQL & Cache
  Analytique & OLAP
  Outils DB

IA & Machine Learning/
  Assistants & LLM
  Plateformes & Modeles
  Agents & Outils IA de dev
  Workflows & Automation IA

Outils de developpement/
  API & Testing
  Low-Code / No-Code
  Diagrammes & Visualisation
  Utilitaires Dev
  UI/Terminal

Cybersecurite/
  Audit & Compliance
  Authentication & IAM
  Reseau & Acces
  Secrets & Credentials
  Threat Detection & Analysis
  Reverse Engineering
  PAM & Jump Servers

Virtualisation & Infrastructure/
  Hyperviseurs
  Cloud Infrastructure

Systemes d'exploitation/
  Linux
  Outils systeme
  Compatibilite Windows

Data & Analytics/
  Business Intelligence
  Data Platforms
  Big Data & Streaming

Productivite & Collaboration/
  Gestion de projet
  CRM & ERP
  Gestion d'actifs IT
  Time Tracking
  Notes & Documentation
  Finance

Utilitaires/
  Documents & PDF
  Medias & Multimedia
  Systeme & Boot
  Remote Desktop
  Dashboards & Homepages
  Applications KDE

Experimental & Projets GitHub   (UNIQUEMENT si aucune categorie ne correspond)

Documentation & Learning
```

### 4. Format HTML

Chaque lien doit suivre ce format exact (indentation avec des espaces, pas de tabs) :
```html
            <DT><A HREF="https://example.com" ADD_DATE="1740000000" LAST_MODIFIED="1740000000">Nom du Projet</A>
```
- Utiliser le timestamp Unix actuel pour ADD_DATE et LAST_MODIFIED
- Le nom doit etre le nom officiel du projet (pas l'URL)

### 5. Nouvelles sous-categories
Si un lien ne rentre dans aucune sous-categorie existante mais correspond a une categorie principale, creer une nouvelle sous-categorie. Demander confirmation a l'utilisateur avant.

### 6. Rapport final
A la fin, afficher un tableau recapitulatif :

| URL | Nom | Categorie | Sous-categorie | Statut |

Statut = "ajoute", "doublon ignore", ou "categorie a confirmer"

### 7. Interdictions
- Ne jamais mettre un lien dans "Experimental" par defaut si une categorie existe
- Ne jamais deviner la categorie sans avoir visite l'URL
- Ne jamais modifier les liens existants
- Ne jamais changer la structure HTML existante
