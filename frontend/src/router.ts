import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: () => import('./views/Dashboard.vue'), meta: { title: 'Panel Principal' } },
    { path: '/mappings', component: () => import('./views/SignalMapper.vue'), meta: { title: 'Mapeo de Señales' } },
    { path: '/devices', component: () => import('./views/DevicesTopics.vue'), meta: { title: 'Dispositivos y Tópicos' } },
    { path: '/mqtt', component: () => import('./views/MqttConfig.vue'), meta: { title: 'Broker MQTT' } },
    { path: '/nats', component: () => import('./views/NatsConfig.vue'), meta: { title: 'Fan-Out NATS' } },
    { path: '/iec104', component: () => import('./views/Iec104Config.vue'), meta: { title: 'IEC 60870-5-104' } },
    { path: '/history', component: () => import('./views/History.vue'), meta: { title: 'Histórico' } },
    { path: '/tsdb', component: () => import('./views/TSDBView.vue'), meta: { title: 'Pipeline TSDB' } },
    { path: '/ssfv', component: () => import('./views/SSFVView.vue'), meta: { title: 'SSFV – Plantas Solares' } },
  ],
})
