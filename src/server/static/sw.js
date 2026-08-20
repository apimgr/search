// Service Worker for Search PWA
// Per AI.md PART 16: PWA Support
//
// Every respondWith() branch MUST resolve to a real Response. A promise that
// resolves to undefined - or rejects - makes the browser render
// net::ERR_FAILED instead of a page. Every branch below ends in a guaranteed
// Response. The service worker is an enhancement, never a dependency: the site
// stays fully usable if it never installs.

const CACHE_NAME = 'search-cache-v1';
const STATIC_ASSETS = [
  '/',
  '/static/css/common.css',
  '/static/css/components.css',
  '/static/css/public.css',
  '/static/js/app.js',
  '/static/img/favicon.svg',
  '/static/img/icon-192.svg',
  '/static/img/icon-512.svg',
  '/manifest.json',
  '/offline.html'
];

// INSTALL - pre-cache static assets, then activate immediately
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => cache.addAll(STATIC_ASSETS))
      .then(() => self.skipWaiting())
  );
});

// ACTIVATE - clean old caches, then take control immediately
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(
        keys
          .filter((key) => key.startsWith('search-cache-') && key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      ))
      .then(() => self.clients.claim())
  );
});

// SKIP_WAITING - let the page activate a waiting worker on demand
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});

// GUARANTEED last-resort page - synthesized in the worker so it needs no cache
// hit and can never itself miss
function offlineFallbackResponse() {
  return new Response(
    '<!doctype html><html lang="en"><head><meta charset="utf-8">'
      + '<meta name="viewport" content="width=device-width,initial-scale=1">'
      + '<title>Offline</title></head><body><main>'
      + '<h1>You are offline</h1><p>This page could not be loaded and no '
      + 'cached copy is available. Check your connection and try again.</p>'
      + '</main></body></html>',
    { status: 503, statusText: 'Service Unavailable',
      headers: { 'Content-Type': 'text/html; charset=utf-8' } }
  );
}

// FETCH - every response path resolves to a real Response
self.addEventListener('fetch', (event) => {
  const request = event.request;
  const url = new URL(request.url);

  // Only same-origin GET is handled here; everything else falls through to the
  // browser untouched (never call respondWith for it)
  if (request.method !== 'GET' || url.origin !== self.location.origin) {
    return;
  }

  // API calls are network-only - never intercept
  if (url.pathname.startsWith('/api/')) {
    return;
  }

  // Navigations (page loads): network-first, then cache, then the cached
  // offline page, then a GUARANTEED synthesized offline page
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .then((response) => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(request, clone));
          return response;
        })
        .catch(async () =>
          (await caches.match(request))
            || (await caches.match('/offline.html'))
            || offlineFallbackResponse()
        )
    );
    return;
  }

  // Static assets: cache-first, then network, then a GUARANTEED 504
  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request)
        .then((cached) => cached || fetch(request).then((response) => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(request, clone));
          return response;
        }))
        .catch(() => new Response('', { status: 504, statusText: 'Gateway Timeout' }))
    );
    return;
  }

  // Everything else: network-first, then cache, then a GUARANTEED 504
  event.respondWith(
    fetch(request)
      .catch(async () =>
        (await caches.match(request))
          || new Response('', { status: 504, statusText: 'Gateway Timeout' })
      )
  );
});
