// Bookmark Manager
let bookmarkData = { folders: [], bookmarks: [] };
let currentFolder = null;
let allBookmarks = [];
let searchQuery = '';
let favorites = JSON.parse(localStorage.getItem('favorites') || '[]');

// ===== MÉTADONNÉES CSV =====
let bookmarkMeta = new Map(); // url → { name, category, subcategory, description, tags, date_added }

// ===== LAZY LOADING =====
const BATCH_SIZE = 30;
let displayedCount = 0;
let currentFilteredBookmarks = [];
let isLoading = false;

// ===== FAVORIS =====
function isFavorite(url) {
    return favorites.includes(url);
}

function toggleFavorite(url) {
    if (isFavorite(url)) {
        favorites = favorites.filter(f => f !== url);
    } else {
        favorites.push(url);
    }
    localStorage.setItem('favorites', JSON.stringify(favorites));
}

function getFavoriteBookmarks() {
    return allBookmarks.filter(b => isFavorite(b.url));
}

// Initialize the application
document.addEventListener('DOMContentLoaded', async () => {
    console.log('App initializing...');
    initThemeToggle();
    initSearch();
    initKeyboardNavigation();
    initLazyLoading();
    await loadBookmarks();
    initFavoritesPanel();
    initUpdatesPanel();
    console.log('Rendering folder tree...');
    renderFolderTree();
    console.log('Showing initial folder...');

    // Restaurer le dossier sauvegardé ou afficher l'accueil
    const savedFolder = localStorage.getItem('currentFolder');
    if (savedFolder && savedFolder !== '') {
        showFolder(savedFolder);
    } else {
        showFolder(null);
    }

    console.log('App initialized!');
});

// Load and parse bookmarks.html + bookmarks.csv
async function loadBookmarks() {
    try {
        console.log('Loading bookmarks.html...');
        const response = await fetch('bookmarks.html');
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const html = await response.text();
        console.log('Bookmarks file loaded, size:', html.length);

        const parser = new DOMParser();
        const doc = parser.parseFromString(html, 'text/html');
        console.log('Document parsed');

        bookmarkData = parseBookmarkTree(doc);
        console.log('Bookmarks parsed:', {
            folders: bookmarkData.folders.length,
            bookmarks: allBookmarks.length
        });

        await loadMetadata();
        buildSuggestionPool();
    } catch (error) {
        console.error('Error loading bookmarks:', error);
        alert('Erreur lors du chargement des marque-pages: ' + error.message);
    }
}

// ─── Chargement métadonnées CSV ────────────────────────────────────────────
async function loadMetadata() {
    try {
        const res = await fetch('bookmarks.csv');
        if (!res.ok) return;
        const text = await res.text();
        const rows = parseCSV(text);
        rows.forEach(row => {
            if (row.url) bookmarkMeta.set(row.url, {
                name:        row.name        || '',
                category:    row.category    || '',
                subcategory: row.subcategory || '',
                description: row.description || '',
                tags:        row.tags        ? row.tags.split('|').filter(Boolean) : [],
                date_added:  row.date_added  ? parseInt(row.date_added) : 0,
            });
        });
        console.log(`Metadata loaded: ${bookmarkMeta.size} entries`);
    } catch (e) {
        console.warn('Metadata CSV not available:', e);
    }
}

function parseCSV(text) {
    const lines = text.trim().split('\n');
    if (lines.length < 2) return [];
    const headers = lines[0].split(',').map(h => h.trim());
    return lines.slice(1).map(line => {
        const values = parseCSVLine(line);
        const obj = {};
        headers.forEach((h, i) => { obj[h] = (values[i] || '').trim(); });
        return obj;
    });
}

function parseCSVLine(line) {
    const result = [];
    let current = '';
    let inQuotes = false;
    for (let i = 0; i < line.length; i++) {
        const ch = line[i];
        if (ch === '"') {
            inQuotes = !inQuotes;
        } else if (ch === ',' && !inQuotes) {
            result.push(current);
            current = '';
        } else {
            current += ch;
        }
    }
    result.push(current);
    return result;
}

// Parse the bookmark HTML structure - Simple approach
function parseBookmarkTree(doc) {
    const folders = [];
    const bookmarks = [];

    // Find all DT elements in the document
    const allDTs = doc.querySelectorAll('DT');
    console.log(`Found ${allDTs.length} DT elements total`);

    // Build a path map based on nesting level
    const pathStack = [];

    for (const dt of allDTs) {
        const h3 = dt.querySelector('H3');
        const a = dt.querySelector('A');

        // Calculate nesting level by counting parent DL elements
        let level = 0;
        let parent = dt.parentElement;
        while (parent) {
            if (parent.tagName === 'DL') level++;
            parent = parent.parentElement;
        }

        // Adjust path stack to current level
        pathStack.length = Math.max(0, level - 1);

        if (h3) {
            // It's a folder
            const folderName = h3.textContent.trim();
            pathStack.push(folderName);
            const folderPath = [...pathStack];

            folders.push({
                name: folderName,
                path: folderPath,
                pathString: folderPath.join(' > ')
            });

            console.log(`Folder [level ${level}]:`, folderPath.join(' > '));
        } else if (a) {
            // It's a bookmark
            const url = a.getAttribute('HREF');
            const title = a.textContent.trim();
            const icon = a.getAttribute('ICON');
            const iconUri = a.getAttribute('ICON_URI');

            if (url && title) {
                const bookmark = {
                    title,
                    url,
                    icon,
                    iconUri,
                    folder: pathStack.join(' > ') || 'Root',
                    folderPath: [...pathStack]
                };

                bookmarks.push(bookmark);
                allBookmarks.push(bookmark);
            }
        }
    }

    return { folders, bookmarks };
}

// Render folder tree in sidebar
function renderFolderTree() {
    const folderTree = document.getElementById('folder-tree');
    folderTree.innerHTML = '';

    // Add "All Bookmarks" option
    const allItem = createFolderItem('Tous les marque-pages', null, '📚');
    folderTree.appendChild(allItem);

    // Build folder hierarchy
    const rootFolders = bookmarkData.folders.filter(f => f.path.length === 1);
    rootFolders.forEach(folder => {
        folderTree.appendChild(createFolderTreeItem(folder));
    });
}

// Récupérer les dossiers dépliés depuis localStorage
function getExpandedFolders() {
    return JSON.parse(localStorage.getItem('expandedFolders') || '[]');
}

// Sauvegarder les dossiers dépliés
function saveExpandedFolders(folders) {
    localStorage.setItem('expandedFolders', JSON.stringify(folders));
}

// Ajouter/retirer un dossier de la liste des dépliés
function toggleExpandedFolder(folderPath) {
    let expanded = getExpandedFolders();
    if (expanded.includes(folderPath)) {
        expanded = expanded.filter(f => f !== folderPath);
    } else {
        expanded.push(folderPath);
    }
    saveExpandedFolders(expanded);
}

// Create folder tree item with children
function createFolderTreeItem(folder, level = 0) {
    const container = document.createElement('div');

    const item = createFolderItem(folder.name, folder.pathString, '📁', level);
    container.appendChild(item);

    // Find child folders
    const childFolders = bookmarkData.folders.filter(f =>
        f.path.length === folder.path.length + 1 &&
        f.pathString.startsWith(folder.pathString)
    );

    if (childFolders.length > 0) {
        item.classList.add('has-children');
        const childContainer = document.createElement('div');
        childContainer.className = 'folder-children';

        childFolders.forEach(child => {
            childContainer.appendChild(createFolderTreeItem(child, level + 1));
        });

        container.appendChild(childContainer);

        // Restaurer l'état déplié si sauvegardé
        const expandedFolders = getExpandedFolders();
        if (expandedFolders.includes(folder.pathString)) {
            item.classList.add('expanded');
            childContainer.classList.add('show');
        }

        // Toggle children on click
        item.addEventListener('click', (e) => {
            e.stopPropagation();
            item.classList.toggle('expanded');
            childContainer.classList.toggle('show');
            toggleExpandedFolder(folder.pathString);
            showFolder(folder.pathString);
        });
    } else {
        item.addEventListener('click', () => {
            showFolder(folder.pathString);
        });
    }

    return container;
}

// Create folder item element
function createFolderItem(name, pathString, icon, level = 0) {
    const item = document.createElement('div');
    item.className = 'folder-item';
    item.style.paddingLeft = `${level * 1.2 + 0.8}rem`;
    item.dataset.folder = pathString || '';

    // Accessibilité
    item.setAttribute('role', 'treeitem');
    item.setAttribute('tabindex', '0');
    item.setAttribute('aria-label', `Dossier ${name}`);

    const iconSpan = document.createElement('span');
    iconSpan.className = 'icon';
    iconSpan.textContent = icon;
    iconSpan.setAttribute('aria-hidden', 'true');

    const nameSpan = document.createElement('span');
    nameSpan.textContent = name;

    item.appendChild(iconSpan);
    item.appendChild(nameSpan);

    if (pathString === null) {
        item.addEventListener('click', () => showFolder(null));
    }

    return item;
}

// Show bookmarks for a specific folder
function showFolder(folderPath) {
    currentFolder = folderPath;

    // Sauvegarder le dossier sélectionné
    if (folderPath !== null && folderPath !== undefined) {
        localStorage.setItem('currentFolder', folderPath);
    } else {
        localStorage.removeItem('currentFolder');
    }

    // Update active state in sidebar
    document.querySelectorAll('.folder-item').forEach(item => {
        item.classList.remove('active');
        if ((folderPath === null && item.dataset.folder === '') || item.dataset.folder === folderPath) {
            item.classList.add('active');
        }
    });

    // Update breadcrumb
    updateBreadcrumb(folderPath);

    // Filter and display bookmarks
    displayBookmarks();
}

// Update breadcrumb navigation
function updateBreadcrumb(folderPath) {
    const breadcrumb = document.getElementById('breadcrumb');
    breadcrumb.innerHTML = '';

    const home = document.createElement('span');
    home.className = 'breadcrumb-item';
    home.textContent = 'Accueil';
    home.addEventListener('click', () => showFolder(null));
    breadcrumb.appendChild(home);

    if (folderPath) {
        const parts = folderPath.split(' > ');
        parts.forEach((part, index) => {
            const separator = document.createElement('span');
            separator.className = 'breadcrumb-separator';
            separator.textContent = '/';
            breadcrumb.appendChild(separator);

            const item = document.createElement('span');
            item.className = 'breadcrumb-item';
            item.textContent = part;
            const path = parts.slice(0, index + 1).join(' > ');
            item.addEventListener('click', () => showFolder(path));
            breadcrumb.appendChild(item);
        });
    }
}

// Display bookmarks based on current folder and search
function displayBookmarks() {
    const grid = document.getElementById('bookmarks-grid');
    const emptyState = document.getElementById('empty-state');

    // Animation de transition
    grid.classList.add('transitioning');

    setTimeout(() => {
        grid.innerHTML = '';
        grid.classList.remove('transitioning');

    let filtered = allBookmarks;

    // Filter by folder
    if (currentFolder) {
        filtered = filtered.filter(b => b.folder === currentFolder);
    } else if (!searchQuery) {
        // On home page without search, show only bookmarks without folder
        filtered = filtered.filter(b => !b.folder || b.folder === 'Root');
    }

    // Filter by search query (title, URL, description, tags)
    if (searchQuery) {
        const q = searchQuery.toLowerCase();
        filtered = filtered.filter(b => {
            const meta = bookmarkMeta.get(b.url);
            return b.title.toLowerCase().includes(q) ||
                   b.url.toLowerCase().includes(q) ||
                   (meta?.description || '').toLowerCase().includes(q) ||
                   (meta?.tags || []).some(t => t.toLowerCase().includes(q));
        });
    }

    // Afficher les favoris en haut sur la page d'accueil
    if (!currentFolder && !searchQuery) {
        const favoriteBookmarks = getFavoriteBookmarks();
        if (favoriteBookmarks.length > 0) {
            const favSection = document.createElement('div');
            favSection.className = 'favorites-section';

            const favHeader = document.createElement('div');
            favHeader.className = 'favorites-header';
            favHeader.innerHTML = '★ FAVORIS';
            favSection.appendChild(favHeader);

            const favGrid = document.createElement('div');
            favGrid.className = 'favorites-grid';
            favoriteBookmarks.forEach(bookmark => {
                favGrid.appendChild(createBookmarkCard(bookmark));
            });
            favSection.appendChild(favGrid);

            grid.appendChild(favSection);
        }
    }

    // Show subfolders if in a folder
    if (currentFolder) {
        const subfolders = bookmarkData.folders.filter(f => {
            const currentParts = currentFolder.split(' > ');
            return f.path.length === currentParts.length + 1 &&
                   f.pathString.startsWith(currentFolder);
        });

        subfolders.forEach(folder => {
            grid.appendChild(createFolderCard(folder));
        });
    } else if (!searchQuery) {
        // Show top-level folders on home
        const rootFolders = bookmarkData.folders.filter(f => f.path.length === 1);
        rootFolders.forEach(folder => {
            grid.appendChild(createFolderCard(folder));
        });
    }

    // Lazy loading: stocker les bookmarks filtrés et afficher le premier lot
    currentFilteredBookmarks = filtered;
    displayedCount = 0;
    loadMoreBookmarks(grid);

    // Show empty state if needed
    if (grid.children.length === 0) {
        emptyState.style.display = 'flex';
        grid.style.display = 'none';
    } else {
        emptyState.style.display = 'none';
        grid.style.display = 'grid';
    }
    }, 150); // Fin du setTimeout pour la transition
}

// Charger plus de bookmarks (lazy loading)
function loadMoreBookmarks(grid) {
    if (!grid) grid = document.getElementById('bookmarks-grid');
    if (isLoading || displayedCount >= currentFilteredBookmarks.length) return;

    isLoading = true;
    const batch = currentFilteredBookmarks.slice(displayedCount, displayedCount + BATCH_SIZE);

    batch.forEach(bookmark => {
        grid.appendChild(createBookmarkCard(bookmark));
    });

    displayedCount += batch.length;
    isLoading = false;

    // Supprimer l'indicateur de chargement s'il existe
    const loader = grid.querySelector('.lazy-loader');
    if (loader) loader.remove();

    // Ajouter un indicateur s'il reste des bookmarks à charger
    if (displayedCount < currentFilteredBookmarks.length) {
        const loader = document.createElement('div');
        loader.className = 'lazy-loader';
        loader.textContent = `Chargement... (${displayedCount}/${currentFilteredBookmarks.length})`;
        grid.appendChild(loader);
    }
}

// Détecter le scroll pour charger plus
function initLazyLoading() {
    const content = document.querySelector('.content');
    content.addEventListener('scroll', () => {
        if (isLoading) return;

        const scrollTop = content.scrollTop;
        const scrollHeight = content.scrollHeight;
        const clientHeight = content.clientHeight;

        // Charger plus quand on approche de la fin (200px avant)
        if (scrollTop + clientHeight >= scrollHeight - 200) {
            loadMoreBookmarks();
        }
    });
}

// Create folder card
function createFolderCard(folder) {
    const card = document.createElement('div');
    card.className = 'folder-card';

    // Accessibilité
    card.setAttribute('role', 'listitem');
    card.setAttribute('tabindex', '0');
    card.setAttribute('aria-label', `Ouvrir le dossier ${folder.name}`);

    const icon = document.createElement('span');
    icon.className = 'icon';
    icon.textContent = '📁';
    icon.setAttribute('aria-hidden', 'true');

    const name = document.createElement('span');
    name.className = 'folder-name';
    name.textContent = folder.name;

    card.appendChild(icon);
    card.appendChild(name);

    card.addEventListener('click', () => {
        showFolder(folder.pathString);
    });

    // Navigation clavier
    card.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            showFolder(folder.pathString);
        }
    });

    return card;
}

// Get favicon URL for a bookmark
function getFaviconUrl(bookmark) {
    try {
        const url = new URL(bookmark.url);
        // Use Google's favicon service which retrieves the actual favicon from the site
        return `https://www.google.com/s2/favicons?domain=${url.hostname}&sz=64`;
    } catch {
        return null;
    }
}

// Create bookmark card
function createBookmarkCard(bookmark) {
    const card = document.createElement('div');
    card.className = 'bookmark-card';

    // Accessibilité
    card.setAttribute('role', 'listitem');
    card.setAttribute('tabindex', '0');
    card.setAttribute('aria-label', `${bookmark.title}, ouvrir dans un nouvel onglet`);

    const icon = document.createElement('div');
    icon.className = 'bookmark-icon';
    icon.setAttribute('aria-hidden', 'true');

    // Try to load favicon using Google's service
    const faviconUrl = getFaviconUrl(bookmark);
    if (faviconUrl) {
        const img = document.createElement('img');
        img.src = faviconUrl;
        img.onerror = () => {
            icon.innerHTML = '🔖';
            icon.classList.add('default');
        };
        icon.appendChild(img);
    } else {
        icon.innerHTML = '🔖';
        icon.classList.add('default');
    }

    const info = document.createElement('div');
    info.className = 'bookmark-info';

    const title = document.createElement('div');
    title.className = 'bookmark-title';
    title.textContent = bookmark.title;

    const url = document.createElement('div');
    url.className = 'bookmark-url';
    try {
        const urlObj = new URL(bookmark.url);
        url.textContent = urlObj.hostname;
    } catch {
        url.textContent = bookmark.url;
    }

    info.appendChild(title);
    info.appendChild(url);

    // Métadonnées enrichies depuis CSV
    const meta = bookmarkMeta.get(bookmark.url);
    if (meta) {
        if (meta.description) {
            const desc = document.createElement('div');
            desc.className = 'bookmark-description';
            desc.textContent = meta.description.length > 90
                ? meta.description.slice(0, 87) + '…'
                : meta.description;
            info.appendChild(desc);
        }

        if (meta.tags && meta.tags.length > 0) {
            const tagsRow = document.createElement('div');
            tagsRow.className = 'bookmark-tags';
            meta.tags.slice(0, 4).forEach(tag => {
                const pill = document.createElement('span');
                pill.className = 'tag-pill';
                pill.textContent = tag;
                pill.addEventListener('click', (e) => {
                    e.stopPropagation();
                    const searchInput = document.getElementById('search');
                    searchInput.value = tag;
                    searchQuery = tag;
                    displayBookmarks();
                });
                tagsRow.appendChild(pill);
            });
            info.appendChild(tagsRow);
        }

        if (meta.date_added > 0) {
            const dateEl = document.createElement('span');
            dateEl.className = 'bookmark-date';
            dateEl.textContent = formatDateShort(meta.date_added);
            dateEl.title = new Date(meta.date_added * 1000).toLocaleDateString('fr-FR');
            info.appendChild(dateEl);
        }
    }

    // Bouton favori
    const favoriteBtn = document.createElement('button');
    favoriteBtn.className = 'favorite-btn';
    favoriteBtn.innerHTML = '☆';
    favoriteBtn.title = 'Ajouter aux favoris';

    if (isFavorite(bookmark.url)) {
        favoriteBtn.classList.add('active');
        favoriteBtn.innerHTML = '★';
        favoriteBtn.title = 'Retirer des favoris';
    }

    favoriteBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleFavorite(bookmark.url);
        if (isFavorite(bookmark.url)) {
            favoriteBtn.classList.add('active');
            favoriteBtn.innerHTML = '★';
            favoriteBtn.title = 'Retirer des favoris';
        } else {
            favoriteBtn.classList.remove('active');
            favoriteBtn.innerHTML = '☆';
            favoriteBtn.title = 'Ajouter aux favoris';
        }
        // Rafraîchir l'affichage si on est sur la page d'accueil (pour mettre à jour la section favoris)
        if (currentFolder === null) {
            displayBookmarks();
        }
    });

    card.appendChild(icon);
    card.appendChild(info);
    card.appendChild(favoriteBtn);

    card.addEventListener('click', () => {
        window.open(bookmark.url, '_blank');
    });

    // Navigation clavier
    card.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            window.open(bookmark.url, '_blank');
        }
    });

    return card;
}

// ===== COULEURS 3D PAR THÈME =====
const THEME_3D = {
    'dark':        { grid: '#ffffff', particles: '#ffffff', lines: '#ffffff', cubes: '#ffffff', rings: '#ffffff' },
    'light':       { grid: '#000000', particles: '#000000', lines: '#000000', cubes: '#000000', rings: '#000000' },
    'apple-dark':  { grid: '#2997ff', particles: '#2997ff', lines: '#0071e3', cubes: '#2997ff', rings: '#0071e3' },
    'apple-light': { grid: '#0071e3', particles: '#0071e3', lines: '#0066cc', cubes: '#0071e3', rings: '#0066cc' },
    'nord':        { grid: '#88c0d0', particles: '#81a1c1', lines: '#88c0d0', cubes: '#5e81ac', rings: '#88c0d0' },
    'dracula':     { grid: '#ff79c6', particles: '#bd93f9', lines: '#ff79c6', cubes: '#6272a4', rings: '#bd93f9' },
};

// ===== THEME SELECTOR =====
const THEMES = {
    'dark':        { label: 'DARK',        light: false },
    'light':       { label: 'LIGHT',       light: true  },
    'apple-dark':  { label: 'APPLE DARK',  light: false },
    'apple-light': { label: 'APPLE LIGHT', light: true  },
    'nord':        { label: 'NORD',        light: false },
    'dracula':     { label: 'DRACULA',     light: false },
};

function applyTheme(theme) {
    const cfg = THEMES[theme] || THEMES['dark'];
    document.body.dataset.theme = theme;
    document.body.classList.toggle('light-mode', cfg.light);
    document.body.classList.remove('dark-mode');

    const label = document.getElementById('theme-label');
    if (label) label.textContent = cfg.label;

    document.querySelectorAll('.theme-option').forEach(btn => {
        const active = btn.dataset.theme === theme;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });

    localStorage.setItem('theme', theme);

    // Mise à jour fond 3D (disponible seulement après chargement Three.js)
    if (window.updateScene3DColors) {
        window.updateScene3DColors(THEME_3D[theme] || THEME_3D['dark']);
    }
}

function initThemeToggle() {
    const btn   = document.getElementById('theme-selector-btn');
    const panel = document.getElementById('theme-panel');

    if (!btn || !panel) {
        console.error('Theme selector not found!');
        return;
    }

    // Restaurer le thème sauvegardé (rétrocompat : 'light' → 'light', tout le reste → 'dark')
    const saved = localStorage.getItem('theme') || 'dark';
    const theme = THEMES[saved] ? saved : (saved === 'light' ? 'light' : 'dark');
    applyTheme(theme);

    // Toggle panel
    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const open = panel.classList.toggle('open');
        btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });

    // Sélection d'un thème
    panel.querySelectorAll('.theme-option').forEach(opt => {
        opt.addEventListener('click', () => {
            applyTheme(opt.dataset.theme);
            panel.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
        });
    });

    // Fermer panel au clic extérieur
    document.addEventListener('click', (e) => {
        if (!e.target.closest('#theme-selector')) {
            panel.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
        }
    });

    // Fermer panel à Escape
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && panel.classList.contains('open')) {
            panel.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
            btn.focus();
        }
    });
}

// Initialize search
function initSearch() {
    const searchInput = document.getElementById('search');

    searchInput.addEventListener('input', (e) => {
        searchQuery = e.target.value.trim();
        displayBookmarks();
        updateSuggestions(searchQuery);
    });

    searchInput.addEventListener('focus', () => {
        if (searchInput.value.trim()) updateSuggestions(searchInput.value.trim());
    });

    // Raccourci "/" pour focus sur la recherche
    document.addEventListener('keydown', (e) => {
        if (e.key === '/' && document.activeElement !== searchInput) {
            e.preventDefault();
            searchInput.focus();
        }
        if (e.key === 'Escape' && document.activeElement === searchInput) {
            hideSuggestions();
            searchInput.blur();
            searchInput.value = '';
            searchQuery = '';
            displayBookmarks();
        }
    });
}

// ─── Suggestions de recherche ─────────────────────────────────────────────

// Pool de suggestions : tags + catégories + noms — construit après loadMetadata()
let suggestionPool = [];

function buildSuggestionPool() {
    const tagSet  = new Set();
    const catSet  = new Set();
    const nameSet = new Set();

    bookmarkMeta.forEach(meta => {
        (meta.tags || []).forEach(t => t && tagSet.add(t.toLowerCase()));
    });

    allBookmarks.forEach(b => {
        if (b.folder) b.folder.split(' > ').forEach(part => catSet.add(part.trim()));
        if (b.title)  nameSet.add(b.title);
    });

    // tags en premier (plus utiles), puis catégories, puis noms
    suggestionPool = [
        ...Array.from(tagSet).map(t  => ({ label: t,    type: 'tag'  })),
        ...Array.from(catSet).map(c  => ({ label: c,    type: 'cat'  })),
        ...Array.from(nameSet).map(n => ({ label: n,    type: 'name' })),
    ];
}

function getSuggestions(query) {
    if (!query || query.length < 2) return [];
    const q = query.toLowerCase();
    const results = suggestionPool.filter(s => s.label.toLowerCase().includes(q));
    // Trier : commence par la query en premier, puis contient
    results.sort((a, b) => {
        const aStarts = a.label.toLowerCase().startsWith(q);
        const bStarts = b.label.toLowerCase().startsWith(q);
        if (aStarts && !bStarts) return -1;
        if (!aStarts && bStarts) return 1;
        return a.label.localeCompare(b.label);
    });
    return results.slice(0, 8);
}

let selectedSuggestionIndex = -1;

function updateSuggestions(query) {
    const box = document.getElementById('search-suggestions');
    if (!box) return;

    const suggestions = getSuggestions(query);
    if (!suggestions.length) { hideSuggestions(); return; }

    selectedSuggestionIndex = -1;
    box.innerHTML = '';

    suggestions.forEach((s, i) => {
        const item = document.createElement('div');
        item.className = 'suggestion-item';
        item.setAttribute('role', 'option');
        item.setAttribute('data-index', i);

        const label = document.createElement('span');
        label.className = 'suggestion-label';
        // Mettre en gras la partie qui correspond
        const q = query.toLowerCase();
        const idx = s.label.toLowerCase().indexOf(q);
        if (idx >= 0) {
            label.innerHTML = escapeHtml(s.label.slice(0, idx))
                + `<strong>${escapeHtml(s.label.slice(idx, idx + query.length))}</strong>`
                + escapeHtml(s.label.slice(idx + query.length));
        } else {
            label.textContent = s.label;
        }

        const type = document.createElement('span');
        type.className = `suggestion-type suggestion-type--${s.type}`;
        type.textContent = s.type === 'tag' ? '#' : s.type === 'cat' ? '/' : '→';

        item.appendChild(type);
        item.appendChild(label);

        item.addEventListener('mousedown', (e) => {
            e.preventDefault(); // Éviter le blur du input
            applySuggestion(s.label);
        });

        box.appendChild(item);
    });

    box.classList.remove('hidden');
}

function applySuggestion(value) {
    const input = document.getElementById('search');
    input.value = value;
    searchQuery = value;
    displayBookmarks();
    hideSuggestions();
    input.focus();
}

function hideSuggestions() {
    const box = document.getElementById('search-suggestions');
    if (box) { box.classList.add('hidden'); box.innerHTML = ''; }
    selectedSuggestionIndex = -1;
}

function escapeHtml(str) {
    return str.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

// Navigation clavier dans les suggestions
document.addEventListener('keydown', (e) => {
    const box  = document.getElementById('search-suggestions');
    const input = document.getElementById('search');
    if (!box || box.classList.contains('hidden')) return;
    if (document.activeElement !== input) return;

    const items = box.querySelectorAll('.suggestion-item');
    if (!items.length) return;

    if (e.key === 'ArrowDown') {
        e.stopPropagation();
        e.preventDefault();
        selectedSuggestionIndex = Math.min(selectedSuggestionIndex + 1, items.length - 1);
        items.forEach((it, i) => it.classList.toggle('selected', i === selectedSuggestionIndex));
    } else if (e.key === 'ArrowUp') {
        e.stopPropagation();
        e.preventDefault();
        selectedSuggestionIndex = Math.max(selectedSuggestionIndex - 1, -1);
        items.forEach((it, i) => it.classList.toggle('selected', i === selectedSuggestionIndex));
    } else if (e.key === 'Enter' && selectedSuggestionIndex >= 0) {
        e.preventDefault();
        applySuggestion(items[selectedSuggestionIndex].querySelector('.suggestion-label').textContent);
    } else if (e.key === 'Escape') {
        hideSuggestions();
    }
});

// Fermer au clic extérieur
document.addEventListener('click', (e) => {
    if (!e.target.closest('.search-wrapper')) hideSuggestions();
});

// ===== NAVIGATION CLAVIER =====
let selectedFolderIndex = -1;
let selectedBookmarkIndex = -1;
let keyboardMode = null; // 'folders' ou 'bookmarks'

function initKeyboardNavigation() {
    document.addEventListener('keydown', handleKeyboardNavigation);
}

// Récupérer uniquement les dossiers visibles (pas dans un conteneur caché)
function getVisibleFolderItems() {
    const allFolderItems = document.querySelectorAll('.folder-item');
    return Array.from(allFolderItems).filter(item => {
        // Vérifier si l'élément est visible (pas dans un folder-children caché)
        const parent = item.closest('.folder-children');
        if (parent && !parent.classList.contains('show')) {
            return false;
        }
        return true;
    });
}

function handleKeyboardNavigation(e) {
    // Ignorer si on est dans un input
    if (document.activeElement.tagName === 'INPUT') return;

    const visibleFolderItems = getVisibleFolderItems();
    const bookmarkCards = document.querySelectorAll('.bookmark-card');

    switch (e.key) {
        case 'ArrowUp':
            e.preventDefault();
            if (keyboardMode === 'bookmarks' && selectedBookmarkIndex > 0) {
                selectBookmark(selectedBookmarkIndex - 1, bookmarkCards);
            } else if (keyboardMode === 'folders' && selectedFolderIndex > 0) {
                selectFolder(selectedFolderIndex - 1, visibleFolderItems);
            } else if (keyboardMode === null) {
                // Commencer par les dossiers
                keyboardMode = 'folders';
                selectFolder(visibleFolderItems.length - 1, visibleFolderItems);
            }
            break;

        case 'ArrowDown':
            e.preventDefault();
            if (keyboardMode === 'bookmarks' && selectedBookmarkIndex < bookmarkCards.length - 1) {
                selectBookmark(selectedBookmarkIndex + 1, bookmarkCards);
            } else if (keyboardMode === 'folders' && selectedFolderIndex < visibleFolderItems.length - 1) {
                selectFolder(selectedFolderIndex + 1, visibleFolderItems);
            } else if (keyboardMode === null) {
                // Commencer par les dossiers
                keyboardMode = 'folders';
                selectFolder(0, visibleFolderItems);
            }
            break;

        case 'ArrowRight':
            e.preventDefault();
            // Passer aux bookmarks
            if (bookmarkCards.length > 0) {
                keyboardMode = 'bookmarks';
                clearFolderSelection(visibleFolderItems);
                selectBookmark(0, bookmarkCards);
            }
            break;

        case 'ArrowLeft':
            e.preventDefault();
            // Revenir aux dossiers
            if (visibleFolderItems.length > 0) {
                keyboardMode = 'folders';
                clearBookmarkSelection(bookmarkCards);
                selectFolder(selectedFolderIndex >= 0 && selectedFolderIndex < visibleFolderItems.length ? selectedFolderIndex : 0, visibleFolderItems);
            }
            break;

        case 'Enter':
            e.preventDefault();
            if (keyboardMode === 'folders' && selectedFolderIndex >= 0 && selectedFolderIndex < visibleFolderItems.length) {
                visibleFolderItems[selectedFolderIndex].click();
            } else if (keyboardMode === 'bookmarks' && selectedBookmarkIndex >= 0) {
                bookmarkCards[selectedBookmarkIndex].click();
            }
            break;

        case 'Escape':
            clearFolderSelection(visibleFolderItems);
            clearBookmarkSelection(bookmarkCards);
            keyboardMode = null;
            selectedFolderIndex = -1;
            selectedBookmarkIndex = -1;
            break;
    }
}

function selectFolder(index, folderItems) {
    // Nettoyer TOUS les folder-items, pas seulement ceux passés en paramètre
    document.querySelectorAll('.folder-item').forEach(item => item.classList.remove('keyboard-selected'));
    selectedFolderIndex = index;
    if (folderItems[index]) {
        folderItems[index].classList.add('keyboard-selected');
        // Scroll dans la sidebar
        scrollIntoViewIfNeeded(folderItems[index], document.querySelector('.sidebar'));
    }
}

function selectBookmark(index, bookmarkCards) {
    clearBookmarkSelection(bookmarkCards);
    selectedBookmarkIndex = index;
    if (bookmarkCards[index]) {
        bookmarkCards[index].classList.add('keyboard-selected');
        // Scroll dans le contenu principal
        scrollIntoViewIfNeeded(bookmarkCards[index], document.querySelector('.content'));
    }
}

// Scroll automatique pour garder l'élément visible
function scrollIntoViewIfNeeded(element, container) {
    if (!element || !container) return;

    const elementRect = element.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();

    // Vérifier si l'élément est hors du viewport visible
    const isAbove = elementRect.top < containerRect.top;
    const isBelow = elementRect.bottom > containerRect.bottom;

    if (isAbove || isBelow) {
        // Utiliser scrollIntoView natif qui fonctionne mieux
        element.scrollIntoView({
            block: isAbove ? 'start' : 'end',
            behavior: 'smooth'
        });
    }
}

function clearFolderSelection(folderItems) {
    if (folderItems && folderItems.forEach) {
        folderItems.forEach(item => item.classList.remove('keyboard-selected'));
    }
    // Nettoyer aussi tous les autres au cas où
    document.querySelectorAll('.folder-item.keyboard-selected').forEach(item => item.classList.remove('keyboard-selected'));
}

function clearBookmarkSelection(bookmarkCards) {
    if (bookmarkCards && bookmarkCards.forEach) {
        bookmarkCards.forEach(card => card.classList.remove('keyboard-selected'));
    }
}

// Réinitialiser la sélection quand on change de dossier
const originalShowFolder = showFolder;
showFolder = function(folderPath) {
    selectedBookmarkIndex = -1;
    if (keyboardMode === 'bookmarks') {
        keyboardMode = 'folders';
    }
    originalShowFolder(folderPath);
};

// ─── Feature FAVORIS ──────────────────────────────────────────────────────
function initFavoritesPanel() {
    const btn   = document.getElementById('favorites-btn');
    const badge = document.getElementById('favorites-badge');
    const panel = document.getElementById('favorites-panel');
    const list  = document.getElementById('favorites-list');
    const close = panel?.querySelector('.panel-close');

    if (!btn || !panel) return;

    function refreshBadge() {
        const count = favorites.length;
        badge.textContent = count;
        badge.classList.toggle('hidden', count === 0);
    }

    function renderFavorites() {
        list.innerHTML = '';
        const favBMs = getFavoriteBookmarks();

        if (favBMs.length === 0) {
            list.innerHTML = '<p class="panel-empty">Aucun favori pour l\'instant. Cliquez sur ☆ sur un marque-page pour l\'ajouter.</p>';
            return;
        }

        favBMs.forEach(b => {
            const meta = bookmarkMeta.get(b.url);
            const entry = document.createElement('div');
            entry.className = 'timeline-entry fav-entry';

            const rootCat = (b.folder || '').split(' > ')[0];
            let hostname = '';
            try { hostname = new URL(b.url).hostname; } catch {}

            const cat = document.createElement('span');
            cat.className = 'tl-cat';
            cat.textContent = rootCat;

            const descText = meta?.description
                ? (meta.description.length > 70 ? meta.description.slice(0, 67) + '…' : meta.description)
                : '';

            const name = document.createElement('span');
            name.className = 'tl-name';
            name.innerHTML = `<a href="${b.url}" target="_blank" rel="noopener">${b.title}</a>`
                + `<span class="tl-url">${hostname}</span>`
                + (descText ? `<span class="tl-desc">${descText}</span>` : '');

            const removeBtn = document.createElement('button');
            removeBtn.className = 'fav-remove-btn';
            removeBtn.textContent = '★';
            removeBtn.title = 'Retirer des favoris';
            removeBtn.addEventListener('click', () => {
                toggleFavorite(b.url);
                refreshBadge();
                renderFavorites();
                if (currentFolder === null) displayBookmarks();
            });

            entry.appendChild(cat);
            entry.appendChild(name);
            entry.appendChild(removeBtn);
            list.appendChild(entry);
        });
    }

    refreshBadge();

    btn.addEventListener('click', () => {
        renderFavorites();
        panel.classList.remove('hidden');
        panel.setAttribute('aria-hidden', 'false');
    });

    close?.addEventListener('click', closeFavorites);
    panel.addEventListener('click', (e) => { if (e.target === panel) closeFavorites(); });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !panel.classList.contains('hidden')) closeFavorites();
    });

    function closeFavorites() {
        panel.classList.add('hidden');
        panel.setAttribute('aria-hidden', 'true');
    }

    // Mettre à jour le badge quand toggleFavorite est appelé
    const _origToggle = toggleFavorite;
    toggleFavorite = function(url) {
        _origToggle(url);
        refreshBadge();
    };
}

// ─── Utilitaires date ─────────────────────────────────────────────────────
function formatDateShort(timestamp) {
    const d = new Date(timestamp * 1000);
    return d.toLocaleDateString('fr-FR', { month: 'short', year: 'numeric' });
}

function formatDateFull(timestamp) {
    const d = new Date(timestamp * 1000);
    return d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
}

function groupByMonth(bookmarks) {
    const groups = new Map();
    bookmarks.forEach(b => {
        const meta = bookmarkMeta.get(b.url);
        const ts = meta?.date_added || 0;
        const d = new Date(ts * 1000);
        const key = ts > 0
            ? d.toLocaleDateString('fr-FR', { month: 'long', year: 'numeric' })
            : 'Date inconnue';
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key).push(b);
    });
    return groups;
}

// ─── Feature NOUVEAUTÉS ────────────────────────────────────────────────────
function initUpdatesPanel() {
    const btn    = document.getElementById('updates-btn');
    const badge  = document.getElementById('updates-badge');
    const panel  = document.getElementById('updates-panel');
    const list   = document.getElementById('updates-list');
    const close  = panel?.querySelector('.panel-close');

    if (!btn || !panel) return;

    // Calculer le max date_added pour la prochaine visite
    const maxDateAdded = Math.max(0, ...Array.from(bookmarkMeta.values()).map(m => m.date_added || 0));

    // Récupérer la date de dernière visite de l'onglet Nouveautés
    const lastVisitDate = parseInt(localStorage.getItem('lastVisitDate') || '0');

    // Bookmarks nouveaux depuis la dernière visite
    function getNewBookmarks() {
        return allBookmarks
            .filter(b => {
                const meta = bookmarkMeta.get(b.url);
                return meta?.date_added > lastVisitDate;
            })
            .sort((a, b) => {
                const da = bookmarkMeta.get(a.url)?.date_added || 0;
                const db = bookmarkMeta.get(b.url)?.date_added || 0;
                return db - da;
            });
    }

    // Mettre à jour le badge
    function refreshBadge() {
        const count = getNewBookmarks().length;
        if (count > 0) {
            badge.textContent = count;
            badge.classList.remove('hidden');
        } else {
            badge.classList.add('hidden');
        }
    }

    // Remplir le panel
    function renderUpdates() {
        list.innerHTML = '';
        const newBMs = getNewBookmarks();

        if (newBMs.length === 0) {
            list.innerHTML = '<p class="panel-empty">Aucun ajout depuis votre dernière visite.</p>';
            return;
        }

        const groups = groupByMonth(newBMs);
        groups.forEach((items, monthLabel) => {
            const group = document.createElement('div');
            group.className = 'timeline-group';

            const header = document.createElement('div');
            header.className = 'timeline-group-header';
            header.textContent = `${monthLabel} — ${items.length} ajout${items.length > 1 ? 's' : ''}`;
            group.appendChild(header);

            items.forEach(b => {
                const meta = bookmarkMeta.get(b.url);
                const entry = document.createElement('div');
                entry.className = 'timeline-entry';
                const rootCat = (b.folder || '').split(' > ')[0];
                let hostname = '';
                try { hostname = new URL(b.url).hostname; } catch {}
                entry.innerHTML = `
                    <span class="tl-cat">${rootCat}</span>
                    <span class="tl-name"><a href="${b.url}" target="_blank" rel="noopener">${b.title}</a><span class="tl-url">${hostname}</span></span>
                `;
                group.appendChild(entry);
            });

            list.appendChild(group);
        });
    }

    refreshBadge();

    btn.addEventListener('click', () => {
        renderUpdates();
        panel.classList.remove('hidden');
        panel.setAttribute('aria-hidden', 'false');
        // Sauvegarder la date de visite (max date existante)
        if (maxDateAdded > 0) {
            localStorage.setItem('lastVisitDate', maxDateAdded.toString());
        }
        // Cacher le badge
        badge.classList.add('hidden');
    });

    close?.addEventListener('click', closeUpdates);
    panel.addEventListener('click', (e) => { if (e.target === panel) closeUpdates(); });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !panel.classList.contains('hidden')) closeUpdates();
    });

    function closeUpdates() {
        panel.classList.add('hidden');
        panel.setAttribute('aria-hidden', 'true');
    }
}

