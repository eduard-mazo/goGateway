import { createRouter, createWebHistory } from 'vue-router'
import DevicesTopics    from './views/DevicesTopics.vue'
import SSFVView         from './views/SSFVView.vue'
import MqttConfig       from './views/MqttConfig.vue'
import Iec104Config     from './views/Iec104Config.vue'
import BrokerMonitorView from './views/BrokerMonitorView.vue'
import UsersView        from './views/UsersView.vue'
import TSDBView         from './views/TSDBView.vue'

// LoginView stays lazy: it's only needed before auth and is never part of the
// authenticated shell, so there's no benefit in bundling it upfront.
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    // Auth layout — no sidebar, full-screen form.
    {
      path: '/login',
      component: () => import('./views/LoginView.vue'),
      meta: { title: 'Iniciar sesión', layout: 'auth' },
    },

    // ── 4 primary workflow menus ──────────────────────────────────────────────
    {
      path: '/subscriptions',
      component: DevicesTopics,
      meta: { title: 'Tópicos y Suscripciones' },
    },
    {
      path: '/signals',
      component: SSFVView,
      meta: { title: 'Señales SSFV' },
    },
    // Gateway signal definitions (persist_to_db) — secondary, not in main nav.
    {
      path: '/gateway-signals',
      component: MqttConfig,
      meta: { title: 'Señales Gateway' },
    },
    {
      path: '/iec104',
      component: Iec104Config,
      meta: { title: 'IEC 60870-5-104' },
    },
    {
      path: '/monitor',
      component: BrokerMonitorView,
      meta: { title: 'Monitor en Tiempo Real' },
    },

    // ── Admin / secondary routes ──────────────────────────────────────────────
    { path: '/users', component: UsersView,  meta: { title: 'Gestión de Usuarios' } },
    { path: '/tsdb',  component: TSDBView,   meta: { title: 'Pipeline TSDB' } },

    // ── Redirects: keep old URLs alive ────────────────────────────────────────
    { path: '/',               redirect: '/subscriptions' },
    { path: '/devices',        redirect: '/subscriptions' },
    { path: '/mqtt',           redirect: '/subscriptions' },
    { path: '/mappings',       redirect: '/iec104' },
    { path: '/broker-monitor', redirect: '/monitor' },
    { path: '/history',        redirect: '/monitor' },
  ],
})

// Global auth guard: redirect to /login if no access token is stored.
router.beforeEach(to => {
  if (to.meta?.layout === 'auth') return true
  const token = localStorage.getItem('gw:access')
  if (!token) return { path: '/login', query: { redirect: to.fullPath } }
  return true
})
