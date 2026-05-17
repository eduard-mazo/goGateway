import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    // Auth layout — no sidebar, full-screen form.
    {
      path: '/login',
      component: () => import('./views/LoginView.vue'),
      meta: { title: 'Iniciar sesión', layout: 'auth' },
    },
    { path: '/', component: () => import('./views/Dashboard.vue'), meta: { title: 'Panel Principal' } },
    { path: '/mappings', component: () => import('./views/SignalMapper.vue'), meta: { title: 'Mapeo de Señales' } },
    { path: '/devices', component: () => import('./views/DevicesTopics.vue'), meta: { title: 'Dispositivos y Tópicos' } },
    { path: '/mqtt', component: () => import('./views/MqttConfig.vue'), meta: { title: 'Broker MQTT' } },
    { path: '/nats', component: () => import('./views/NatsConfig.vue'), meta: { title: 'Fan-Out NATS' } },
    { path: '/iec104', component: () => import('./views/Iec104Config.vue'), meta: { title: 'IEC 60870-5-104' } },
    { path: '/history', component: () => import('./views/History.vue'), meta: { title: 'Histórico' } },
    { path: '/tsdb', component: () => import('./views/TSDBView.vue'), meta: { title: 'Pipeline TSDB' } },
    { path: '/ssfv', component: () => import('./views/SSFVView.vue'), meta: { title: 'SSFV – Plantas Solares' } },
    { path: '/broker-monitor', component: () => import('./views/BrokerMonitorView.vue'), meta: { title: 'Monitor Broker' } },
    { path: '/users', component: () => import('./views/UsersView.vue'), meta: { title: 'Gestión de Usuarios' } },
  ],
})

// Global auth guard: redirect to /login if no access token is stored.
// Pages with layout:'auth' (login itself) are always allowed through.
router.beforeEach(to => {
  if (to.meta?.layout === 'auth') return true
  const token = localStorage.getItem('gw:access')
  if (!token) return { path: '/login', query: { redirect: to.fullPath } }
  return true
})
