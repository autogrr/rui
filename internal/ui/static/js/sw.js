// Service Worker for rui — eliminates network RTT for static assets.
//
// Strategies:
//   /ui/static/*  → stale-while-revalidate (instant from cache, background refresh)
//   everything else → network-only (never intercept HTML, SSE, API)
//
// Precache: on install, all core JS + CSS are fetched and cached so the very
// first hx-boost navigation gets 100% cache hits. The precache manifest is
// injected server-side by the /ui/sw.js handler.
//
// Kill switch: POST a message { type: 'UNINSTALL' } to self-unregister and
// wipe all caches so the app returns to vanilla behaviour.

var CACHE = 'rui-static-v1';

// Injected by the server — replaced with a real JSON array at response time.
// Falls back to empty array if served as a raw static file.
var PRECACHE_URLS = /*PRECACHE_MANIFEST*/[]/*END_PRECACHE_MANIFEST*/;

// ── Install / Activate ──────────────────────────────────────────────────────

self.addEventListener('install', function (event) {
  // Prefetch all core assets so the cache is warm before user navigates.
  // skipWaiting is called after precache completes to ensure the cache is
  // fully populated before activation.
  event.waitUntil(
    caches.open(CACHE).then(function (cache) {
      if (PRECACHE_URLS.length > 0) {
        return cache.addAll(PRECACHE_URLS);
      }
    }).then(function () {
      return self.skipWaiting();
    })
  );
});

self.addEventListener('activate', function (event) {
  event.waitUntil(
    // Purge any caches from older SW versions.
    caches.keys().then(function (keys) {
      return Promise.all(
        keys.filter(function (k) { return k !== CACHE; })
            .map(function (k) { return caches.delete(k); })
      );
    }).then(function () {
      // Claim all open tabs so the SW controls them without a reload.
      return self.clients.claim();
    }).then(function () {
      // Notify all clients that a new SW version is now active.
      return self.clients.matchAll().then(function (clients) {
        clients.forEach(function (c) {
          c.postMessage({ type: 'SW_UPDATED' });
        });
      });
    })
  );
});

// ── Kill switch ─────────────────────────────────────────────────────────────

self.addEventListener('message', function (event) {
  if (event.data && event.data.type === 'UNINSTALL') {
    // Wipe caches, then unregister ourselves.
    caches.keys().then(function (keys) {
      return Promise.all(keys.map(function (k) { return caches.delete(k); }));
    }).then(function () {
      return self.registration.unregister();
    }).then(function () {
      // Notify all clients so they can hard-reload if desired.
      self.clients.matchAll().then(function (clients) {
        clients.forEach(function (c) {
          c.postMessage({ type: 'SW_UNINSTALLED' });
        });
      });
    });
  }
});

// ── Fetch handler ───────────────────────────────────────────────────────────

self.addEventListener('fetch', function (event) {
  // Only intercept GET requests.
  if (event.request.method !== 'GET') return;

  var url = new URL(event.request.url);

  // Only cache static assets — never intercept HTML, partials, SSE, or API.
  if (url.pathname.indexOf('/ui/static/') === -1) return;

  // Stale-while-revalidate: serve from cache instantly, refresh in background.
  event.respondWith(
    caches.open(CACHE).then(function (cache) {
      return cache.match(event.request).then(function (cached) {
        var networkFetch = fetch(event.request).then(function (response) {
          if (response.ok) {
            cache.put(event.request, response.clone());
          }
          return response;
        });
        return cached || networkFetch;
      });
    })
  );
});
