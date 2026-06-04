import { createApp } from 'vue'
import './style.css'
import 'vue-sonner/style.css'
import App from './App.vue'
import { router } from './router'

// ── Stale-chunk recovery ──────────────────────────────────────────────────────
//
// Problem: after `make build` + binary restart the server has NEW content-hash
// chunk filenames. If the browser still has the OLD main bundle in memory (from
// before the restart), every lazy-loaded route navigation calls
// import('./views/Foo-OldHash.js') which returns 404 on the new binary, the
// component factory throws, and the view renders blank.
//
// Wrong fix: window.location.reload() — reloads to the CURRENT URL, not the
// route the user was trying to reach, so they never leave the current view.
//
// Correct fix: router.onError receives (error, to) where `to` is the intended
// destination. window.location.assign(to.fullPath) does a hard navigation to
// that exact URL, the browser re-fetches index.html (Cache-Control: no-cache),
// gets the fresh main bundle with correct chunk hashes, and navigation succeeds.
//
// A sessionStorage flag prevents infinite reload loops in case the binary is
// genuinely broken (missing a chunk). The flag is cleared when the tab closes.

router.onError((error, to) => {
  const isChunk =
    error?.name === 'ChunkLoadError' ||
    /dynamically imported module|Failed to fetch|Unable to preload CSS/i.test(
      error?.message ?? ''
    )
  if (!isChunk || !to) return

  const flag = 'gw:chunk-reload'
  const last = Number(sessionStorage.getItem(flag) ?? 0)
  if (Date.now() - last < 10_000) return     // tried within last 10 s — stop looping
  sessionStorage.setItem(flag, String(Date.now()))
  window.location.assign(to.fullPath)        // hard-navigate to the TARGET route
})

createApp(App).use(router).mount('#app')
