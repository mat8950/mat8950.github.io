const CACHE = 'bookmarks-v2';
const ASSETS = [
    '/',
    '/index.html',
    '/script.js',
    '/bookmarks.html',
    '/bookmarks.csv',
    '/ambient.mp3',
    '/css/variables.css',
    '/css/layout.css',
    '/css/components.css',
    '/css/background.css',
    '/css/themes.css',
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
    // Network-first pour bookmarks.html et bookmarks.csv (données fraîches si dispo)
    if (e.request.url.includes('bookmarks.html') || e.request.url.includes('bookmarks.csv')) {
        e.respondWith(
            fetch(e.request)
                .then(res => { caches.open(CACHE).then(c => c.put(e.request, res.clone())); return res; })
                .catch(() => caches.match(e.request))
        );
        return;
    }
    // Cache-first pour tout le reste
    e.respondWith(
        caches.match(e.request).then(cached => cached || fetch(e.request))
    );
});
