# TODO - Améliorations du site Marque-pages

## En cours

(Aucune tâche en cours)

## À faire

(Toutes les tâches sont terminées !)

## Idées futures

### Raccourcis & UX rapide
- [x] Raccourci `C` pour copier l'URL du bookmark sélectionné au clavier
- [x] Raccourci `?` pour afficher un overlay de tous les raccourcis clavier
- [x] Raccourci `F` pour masquer/afficher la sidebar (plein écran)
- [x] Bouton "Tout replier" les dossiers dans la sidebar

### Affichage & informations
- [x] Afficher le nombre de bookmarks par dossier dans la sidebar
- [x] Compteur total de bookmarks dans le header
- [x] Afficher la `date_added` sur les cartes bookmark
- [x] Tooltip au hover sur les cartes (description depuis le CSV)

### Recherche & filtres
- [x] Recherche dans les descriptions (actuellement nom + URL seulement)
- [x] Tri des bookmarks : alphabétique, par date, par domaine
- [x] Filtres combinés : tags + catégorie + date

### Existant à améliorer
- [x] Drag & drop pour réorganiser les favoris
- [x] Export/import des favoris en JSON
- [x] Mode compact pour afficher plus de bookmarks

### Ambitieux
- [x] Page de stats : top domaines, distribution par catégorie, timeline d'ajout
- [x] PWA (installable, cache offline via Service Worker)
- [x] Import direct depuis Chrome/Firefox (glisser-déposer un export `.html`)
- [x] Suggestions de bookmarks similaires (par tags communs)

## Terminé

### Navigation clavier
- [x] Flèches haut/bas pour naviguer dans les dossiers
- [x] Flèches haut/bas pour naviguer dans les bookmarks
- [x] Entrée pour ouvrir le bookmark/dossier sélectionné
- [x] Indicateur visuel de l'élément sélectionné au clavier
- [x] "/" pour focus recherche, Escape pour annuler

### Favoris rapides
- [x] Ajouter une étoile cliquable sur chaque bookmark
- [x] Sauvegarder les favoris dans localStorage
- [x] Section "Favoris" en haut de la liste des bookmarks
- [x] Possibilité de retirer un favori

### Lazy loading
- [x] Charger les bookmarks par lots (30 à la fois)
- [x] Détecter le scroll pour charger plus
- [x] Indicateur de chargement

### Accessibilité
- [x] Navigation au clavier complète (tabindex)
- [x] Labels ARIA sur les éléments interactifs
- [x] Rôles ARIA (navigation, main, list, listitem)
- [x] Focus visible sur tous les éléments

### Animations subtiles
- [x] Transition douce entre dossiers (fade in/out)
- [x] Effet de "typing" sur le titre principal
- [x] Animation d'ouverture/fermeture des dossiers (slideDown)
- [x] Icône dossier animée (▶/▼)
- [x] Animation slideUp sur les cartes

### Design & Thème
- [x] Style Terminal/Hacker sobre (noir & blanc)
- [x] Fond 3D animé avec Three.js
- [x] Fallback CSS pour navigateurs sans WebGL
- [x] Mode clair/sombre fonctionnel
- [x] Musique d'ambiance avec visualizer audio
- [x] Icônes en couleur avec hover lumineux
- [x] Design responsive (grandes résolutions)

### Persistance
- [x] Position de scroll sauvegardée
- [x] État audio (play/pause) mémorisé
- [x] Dossier sélectionné restauré au refresh
- [x] Thème sauvegardé
- [x] Favoris sauvegardés

---

*Dernière mise à jour : 17 avril 2026*
