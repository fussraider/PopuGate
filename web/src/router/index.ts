import {createRouter, createWebHistory} from 'vue-router'
import {useAuthStore} from '@/stores/auth'

const routes = [
  {
    path: '/auth',
    component: () => import('@/components/layouts/AuthLayout.vue'),
    children: [
      {
        path: 'login',
        name: 'Login',
        component: () => import('@/views/LoginView.vue'),
      },
      {
        path: 'setup',
        name: 'Setup',
        component: () => import('@/views/SetupView.vue'),
      },
    ],
  },
  {
    path: '/',
    component: () => import('@/components/layouts/MainLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'Dashboard',
        component: () => import('@/views/DashboardView.vue'),
      },
      {
        path: 'secrets',
        name: 'Secrets',
        component: () => import('@/views/SecretsView.vue'),
      },
      {
        path: 'upstreams',
        name: 'Upstreams',
        component: () => import('@/views/UpstreamsView.vue'),
      },
      {
        path: 'instances',
        name: 'Instances',
        component: () => import('@/views/InstancesView.vue'),
      },
      {
        path: 'proxy',
        redirect: '/instances',
      },
      {
        path: 'docker',
        redirect: '/system',
      },
      {
        path: 'geoblock',
        name: 'Geoblock',
        component: () => import('@/views/GeoblockView.vue'),
      },
      {
        path: 'traffic',
        name: 'Traffic',
        component: () => import('@/views/TrafficView.vue'),
      },
      {
        path: 'bot',
        name: 'Bot',
        component: () => import('@/views/BotView.vue'),
      },
      {
        path: 'replication',
        name: 'Replication',
        component: () => import('@/views/ReplicationView.vue'),
      },
      {
        path: 'updates',
        redirect: '/system',
      },
      {
        path: 'backups',
        name: 'Backups',
        component: () => import('@/views/BackupsView.vue'),
      },
      {
        path: 'scheduler',
        name: 'Scheduler',
        component: () => import('@/views/SchedulerView.vue'),
      },
      {
        path: 'audit',
        name: 'Audit',
        component: () => import('@/views/AuditView.vue'),
      },
      {
        path: 'templates',
        name: 'Templates',
        component: () => import('@/views/TemplatesView.vue'),
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/SettingsView.vue'),
      },
      {
        path: 'system',
        name: 'System',
        component: () => import('@/views/SystemView.vue'),
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  // Redirect authenticated users away from setup
  if (to.name === 'Setup' && auth.isAuthenticated) {
    return { path: '/' }
  }

  // Redirect authenticated users away from login
  if (to.name === 'Login' && auth.isAuthenticated) {
    return { path: '/' }
  }

  // Protect authenticated routes
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    if (auth.refreshToken) {
      const refreshed = await auth.refresh()
      if (refreshed) return true
      // Refresh failed - clear invalid tokens
      auth.logout()
    }
    return { name: 'Login' }
  }
})

export default router
