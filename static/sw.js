// PlanesGo Express Service Worker
const CACHE_NAME = 'planesgo-express-v1.2.26';
const PRECACHE_ASSETS = [
    '/m',
    '/static/manifest.json',
    '/static/img/icon-192.png',
    '/static/img/icon-512.png',
    '/static/img/logo.png',
    '/static/css/tailwind.css',
    '/static/js/app.js',
    '/static/js/views.js',
    '/static/js/timer.js',
    '/static/js/timesheets.js',
    '/static/js/sidebar.js',
    '/static/js/utils.js'
];

self.addEventListener('install', event => {
    // Forzar activación inmediata sin esperar a que se cierren pestañas
    self.skipWaiting();
    event.waitUntil(
        caches.open(CACHE_NAME).then(cache => {
            return cache.addAll(PRECACHE_ASSETS).catch(err => {
                console.warn('[PlanesGo SW] Error precaching assets:', err);
            });
        })
    );
});

self.addEventListener('activate', event => {
    // Reclamar control de todos los clientes abiertos de inmediato
    event.waitUntil(
        Promise.all([
            self.clients.claim(),
            // Purgar cachés antiguas
            caches.keys().then(keys => {
                return Promise.all(
                    keys.filter(key => key !== CACHE_NAME).map(key => caches.delete(key))
                );
            })
        ])
    );
});

self.addEventListener('fetch', event => {
    const url = new URL(event.request.url);

    // Las llamadas a /api/ o endpoints dinámicos siempre van a la red
    if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/auth/') || url.pathname.startsWith('/login')) {
        return;
    }

    // Navegación (HTML como /m): Network-First para asegurar versión fresca al instante
    if (event.request.mode === 'navigate' || url.pathname === '/m' || url.pathname === '/express') {
        event.respondWith(
            fetch(event.request)
                .then(networkResponse => {
                    if (networkResponse && networkResponse.status === 200) {
                        const responseClone = networkResponse.clone();
                        caches.open(CACHE_NAME).then(cache => cache.put(event.request, responseClone));
                    }
                    return networkResponse;
                })
                .catch(() => caches.match(event.request))
        );
        return;
    }

    // Recursos estáticos: Stale-While-Revalidate
    if (url.pathname.startsWith('/static/')) {
        event.respondWith(
            caches.match(event.request).then(cachedResponse => {
                const fetchPromise = fetch(event.request).then(networkResponse => {
                    if (networkResponse && networkResponse.status === 200) {
                        const responseClone = networkResponse.clone();
                        caches.open(CACHE_NAME).then(cache => cache.put(event.request, responseClone));
                    }
                    return networkResponse;
                }).catch(() => {});
                return cachedResponse || fetchPromise;
            })
        );
        return;
    }

    // Por defecto, dejar pasar a la red
    event.respondWith(
        fetch(event.request).catch(() => caches.match(event.request))
    );
});

self.addEventListener('message', event => {
    if (event.data && event.data.action === 'skipWaiting') {
        self.skipWaiting();
    }
});
