/* 静态资源 cache-first，API 一律 network-only，保证数据永远是最新的。 */
var CACHE = 'fl-v4';
var STATIC = [
  '/', '/index.html', '/style.css', '/app.js', '/views.js', '/features.js', '/manifest.json',
  '/icon-192.png', '/icon-512.png', '/icon-180.png'
];

self.addEventListener('install', function (e) {
  e.waitUntil(caches.open(CACHE).then(function (c) {
    return Promise.all(STATIC.map(function (u) {
      return c.add(u).catch(function () { /* 单个失败不阻塞 */ });
    }));
  }).then(function () { return self.skipWaiting(); }));
});

self.addEventListener('activate', function (e) {
  e.waitUntil(
    caches.keys().then(function (keys) {
      return Promise.all(keys.map(function (k) {
        return k === CACHE ? null : caches.delete(k);
      }));
    }).then(function () { return self.clients.claim(); })
  );
});

self.addEventListener('fetch', function (e) {
  var req = e.request;
  if (req.method !== 'GET') return;

  var url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  // API 与登录一律走网络
  if (url.pathname.indexOf('/api/') === 0) {
    e.respondWith(fetch(req).catch(function () {
      return new Response(JSON.stringify({ error: '当前离线' }),
        { status: 503, headers: { 'Content-Type': 'application/json' } });
    }));
    return;
  }

  // 静态资源：cache-first，后台校验
  e.respondWith(
    caches.match(req).then(function (hit) {
      var net = fetch(req).then(function (res) {
        if (res && res.status === 200) {
          var copy = res.clone();
          caches.open(CACHE).then(function (c) { c.put(req, copy); });
        }
        return res;
      }).catch(function () { return hit; });
      return hit || net;
    })
  );
});
