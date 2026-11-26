// Service Worker for Search PWA
// Per AI.md PART 16: PWA Support
//
// Every respondWith() branch MUST resolve to a real Response. A promise that
// resolves to undefined - or rejects - makes the browser render
// net::ERR_FAILED instead of a page. Every branch below ends in a guaranteed
// Response. The service worker is an enhancement, never a dependency: the site
// stays fully usable if it never installs.

// CACHE_VERSION is substituted with the running build's version and commit by
// handleServiceWorker (src/server/pwa.go) before this file is served, per
// AI.md PART 16: the cache name MUST embed the build stamp so every new
// deploy gets its own cache and `activate` below evicts the previous one -
// otherwise a browser that installed the worker before a fix shipped keeps
// serving the stale cached app.js/CSS forever, no matter how many times the
// server is redeployed.
const CACHE_VERSION = '__CACHE_VERSION__';
const CACHE_NAME = 'search-cache-' + CACHE_VERSION;
const STRINGS_CACHE = 'search-offline-strings';
const DEFAULT_STRINGS = {
  lang: 'en', dir: 'ltr', title: 'Offline', heading: 'You are offline',
  message: 'This page could not be loaded and no cached copy is available. Check your connection and try again.'
};
const STATIC_ASSETS = [
  '/static/css/common.css?v=__CACHE_VERSION__',
  '/static/css/components.css?v=__CACHE_VERSION__',
  '/static/css/public.css?v=__CACHE_VERSION__',
  '/static/js/app.js?v=__CACHE_VERSION__',
  '/static/img/favicon.svg?v=__CACHE_VERSION__',
  '/static/img/favicon-16.png?v=__CACHE_VERSION__',
  '/static/img/favicon-32.png?v=__CACHE_VERSION__',
  '/static/img/favicon-48.png?v=__CACHE_VERSION__',
  '/static/img/ios-share.svg?v=__CACHE_VERSION__',
  '/static/img/icon-72.png?v=__CACHE_VERSION__',
  '/static/img/icon-96.png?v=__CACHE_VERSION__',
  '/static/img/icon-128.png?v=__CACHE_VERSION__',
  '/static/img/icon-144.png?v=__CACHE_VERSION__',
  '/static/img/icon-152.png?v=__CACHE_VERSION__',
  '/static/img/icon-180.png?v=__CACHE_VERSION__',
  '/static/img/icon-192.png?v=__CACHE_VERSION__',
  '/static/img/icon-384.png?v=__CACHE_VERSION__',
  '/static/img/icon-192.svg?v=__CACHE_VERSION__',
  '/static/img/icon-512.png?v=__CACHE_VERSION__',
  '/static/img/icon-512.svg?v=__CACHE_VERSION__',
  '/static/img/icon-maskable-192.png?v=__CACHE_VERSION__',
  '/static/img/icon-maskable-512.png?v=__CACHE_VERSION__',
  '/static/splash/iphone-1179x2556.png?v=__CACHE_VERSION__',
  '/static/splash/iphone-1284x2778.png?v=__CACHE_VERSION__',
  '/static/screenshots/desktop.png?v=__CACHE_VERSION__',
  '/static/screenshots/mobile.png?v=__CACHE_VERSION__',
  '/manifest.json'
];

// Pre-cache static assets only when they return successful non-redirect responses.
function precacheStaticAssets() {
  return caches.open(CACHE_NAME).then((cache) => Promise.all(STATIC_ASSETS.map((asset) =>
    fetch(new Request(asset, { redirect: 'manual' }))
      .then((response) => {
        if (response.ok && !response.redirected && response.type !== 'opaqueredirect') {
          return cache.put(asset, response);
        }
      })
      .catch(() => {})
  )));
}

// INSTALL - pre-cache static assets. An updated worker waits for user approval.
self.addEventListener('install', (event) => {
  event.waitUntil(
    precacheStaticAssets()
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

// A navigation never waits on the network forever. Search requests can take
// longer than ordinary pages because the server aggregates remote engines.
const NAV_TIMEOUT_MS = 8000;
function withTimeout(promise, ms) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms);
    promise.then(
      (value) => { clearTimeout(timer); resolve(value); },
      (error) => { clearTimeout(timer); reject(error); }
    );
  });
}

// The strings cache contains the visitor's last server-translated offline text.
// English is used only before a page has stored that translation.
function escapeHtml(text) {
  return String(text).replace(/[&<>\"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'
  }[character]));
}

async function offlineFallbackResponse() {
  let strings = DEFAULT_STRINGS;
  try {
    const stored = await (await caches.open(STRINGS_CACHE)).match('/offline-strings.json');
    if (stored) strings = Object.assign({}, DEFAULT_STRINGS, await stored.json());
  } catch (error) {
    strings = DEFAULT_STRINGS;
  }
  return new Response(
    '<!doctype html><html lang="' + escapeHtml(strings.lang) + '" dir="' + escapeHtml(strings.dir) + '"><head><meta charset="utf-8">'
      + '<meta name="viewport" content="width=device-width,initial-scale=1">'
      + '<title>' + escapeHtml(strings.title) + '</title></head><body><main>'
      + '<h1>' + escapeHtml(strings.heading) + '</h1><p>' + escapeHtml(strings.message) + '</p>'
      + '</main></body></html>',
    { status: 503, statusText: 'Service Unavailable',
      headers: { 'Content-Type': 'text/html; charset=utf-8' } }
  );
}

// Bang searches are server-recognized redirects. Leave them to the browser so
// the HTTP redirect is followed as a top-level navigation, never converted to
// the offline fallback or stored in the page cache.
function isPotentialBangNavigation(url) {
  if (url.pathname !== '/search') return false;
  const query = (url.searchParams.get('q') || '').trim();
  return /(^|\s)![^\s]+/.test(query);
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

  // The browser must see the server's direct bang redirect and follow it.
  if (request.mode === 'navigate' && isPotentialBangNavigation(url)) {
    return;
  }

  // Navigations (page loads): redirected responses are treated as network
  // failures. Search bangs bypass interception above and follow their direct
  // redirects in the browser.
  if (request.mode === 'navigate') {
    event.respondWith(
      withTimeout(fetch(request), NAV_TIMEOUT_MS)
        .then((response) => {
          if (response.redirected || response.type === 'opaqueredirect') {
            throw new Error('redirected navigation response');
          }
          if (response.ok && !response.redirected && response.type !== 'opaqueredirect'
              && response.headers.get('X-Offline-Cache') === 'public') {
            const clone = response.clone();
            caches.open(CACHE_NAME).then((cache) => cache.put(request, clone)).catch(() => {});
          } else {
            return caches.open(CACHE_NAME).then((cache) => cache.delete(request)).then(() => response);
          }
          return response;
        })
        .catch(async () =>
          (await caches.match(request))
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
          if (response.ok && !response.redirected) {
            const clone = response.clone();
            caches.open(CACHE_NAME).then((cache) => cache.put(request, clone)).catch(() => {});
          }
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
