import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: () => import('./views/Dashboard.vue'), meta: { title: 'Overview' } },
    { path: '/mappings', component: () => import('./views/SignalMapper.vue'), meta: { title: 'Signal Mapping' } },
    { path: '/devices', component: () => import('./views/DevicesTopics.vue'), meta: { title: 'Devices & Topics' } },
    { path: '/mqtt', component: () => import('./views/MqttConfig.vue'), meta: { title: 'MQTT Broker' } },
    { path: '/iec104', component: () => import('./views/Iec104Config.vue'), meta: { title: 'IEC 60870-5-104' } },
    { path: '/history', component: () => import('./views/History.vue'), meta: { title: 'History' } },
  ],
})
