const CACHE = 'bookmarks-v17';
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
    e.waitUntil(
        caches.open(CACHE).then(cache => cache.addAll(ASSETS)).then(() => self.skipWaiting())
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

    // Lien profond /categorie/sous-categorie/app : c'est l'application (index.html) qui affiche la fiche
    const path = new URL(e.request.url).pathname;
    if (e.request.mode === 'navigate' && /^(\/[a-z0-9-]+){1,3}\/?$/.test(path)) {
        e.respondWith(caches.match('/index.html').then(c => c || fetch('/index.html')));
        return;
    }

    // Network-first pour les données (bookmarks.html / .csv, api/*.json) : fraîches si dispo, cache hors ligne
    const p = new URL(e.request.url).pathname;
    if (p === '/bookmarks.html' || p === '/bookmarks.csv' || p.startsWith('/api/')) {
        e.respondWith(
            fetch(e.request)
                .then(res => { const copy = res.clone(); caches.open(CACHE).then(c => c.put(e.request, copy)); return res; })
                .catch(() => caches.match(e.request))
        );
        return;
    }

    // Cache-first pour tout le reste ; réponses same-origin complètes (200, hors Range audio) mises en cache
    e.respondWith(
        caches.match(e.request).then(cached => cached || fetch(e.request).then(res => {
            const sameOrigin = new URL(e.request.url).origin === self.location.origin;
            if (sameOrigin && res.status === 200 && !e.request.headers.has('range')) {
                const copy = res.clone();
                caches.open(CACHE).then(c => c.put(e.request, copy));
            }
            return res;
        }))
    );
});
