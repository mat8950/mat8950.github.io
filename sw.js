const CACHE = 'bookmarks-v18';
// Précache minimal pour l'offline. ambient.mp3 n'y est plus (4 Mo, chargé seulement au PLAY).
// Polices : mises en cache à la volée. Favicons (Google, autre origine) : cache HTTP du navigateur.
const ASSETS = [
    '/',
    '/index.html',
    '/script.js',
    '/css/fonts.css',
    '/css/variables.css',
    '/css/base.css',
    '/css/background.css',
    '/css/layout.css',
    '/css/components.css',
    '/css/themes.css',
    '/css/responsive.css',
    '/lib/three.min.js',
    '/manifest.json',
    '/favicon.ico',
    '/icons/icon.svg',
    '/bookmarks.csv',
];

self.addEventListener('install', e => {
    // cache: 'reload' : pas le cache HTTP du navigateur (GitHub Pages : max-age=600), sinon une version
    // périmée de script.js / CSS peut être figée dans le nouveau cache juste après un déploiement
    e.waitUntil(
        caches.open(CACHE)
            .then(cache => cache.addAll(ASSETS.map(u => new Request(u, { cache: 'reload' }))))
            .then(() => self.skipWaiting())
    );
});

self.addEventListener('activate', e => {
    e.waitUntil(
        caches.keys().then(keys =>
            Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k)))
        ).then(() => self.clients.claim())
    );
});

self.addEventListener('fetch', e => {
    if (e.request.method !== 'GET') return;
    const url = new URL(e.request.url);
    const sameOrigin = url.origin === self.location.origin;
    // Met en cache les réponses same-origin complètes (200, hors Range audio)
    const store = res => {
        if (sameOrigin && res.status === 200 && !e.request.headers.has('range')) {
            const copy = res.clone();
            caches.open(CACHE).then(c => c.put(e.request, copy));
        }
        return res;
    };

    // Pages : réseau d'abord (pages par outil générées au déploiement, toujours à jour).
    // Hors ligne : page en cache, sinon index.html (l'application route /categorie/sous-categorie/app)
    if (e.request.mode === 'navigate') {
        e.respondWith(
            fetch(e.request).then(store)
                .catch(() => caches.match(e.request).then(c => c || caches.match('/index.html')))
        );
        return;
    }

    // Code et données (JS, CSS, HTML, CSV, JSON, api/) : réseau d'abord, revalidé (304 si inchangé) ;
    // le cache ne sert que hors ligne → une mise à jour du site est visible au rechargement suivant
    if (sameOrigin && !url.pathname.startsWith('/lib/') &&
        (/\.(js|css|html|csv|json|txt)$/.test(url.pathname) || url.pathname.startsWith('/api/'))) {
        e.respondWith(
            fetch(url.href, { cache: 'no-cache' }).then(store)
                .catch(() => caches.match(e.request))
        );
        return;
    }

    // Le reste (polices, Three.js, icônes, images, favicons) : cache d'abord
    e.respondWith(caches.match(e.request).then(cached => cached || fetch(e.request).then(store)));
});
