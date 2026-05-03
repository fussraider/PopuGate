<template>
  <div class="main-layout">
    <!-- Sidebar overlay for mobile -->
    <div v-if="sidebarOpen" class="sidebar-overlay" @click="sidebarOpen = false" />

    <!-- Sidebar -->
    <aside :class="['sidebar', { open: sidebarOpen }]">
      <div class="sidebar-header">
        <router-link to="/" class="logo" @click="sidebarOpen = false">
          <img src="@/assets/images/icons/icon-180x180.png" alt="PopuGate" class="logo-img" />
          <span class="logo-text">PopuGate</span>
        </router-link>
        <button class="sidebar-close" @click="sidebarOpen = false"><X :size="20" /></button>
      </div>
      <nav class="sidebar-nav">
        <template v-for="(group, gi) in groupedNavItems" :key="gi">
          <div class="nav-group-label">{{ group.label }}</div>
          <router-link
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            class="nav-item"
            :active-class="item.path === '/' ? '' : 'active'"
            exact-active-class="active"
            @click="sidebarOpen = false"
          >
            <component :is="item.icon" class="nav-icon" :size="18" />
            <span class="nav-label">{{ item.label }}</span>
          </router-link>
        </template>
      </nav>
      <div class="sidebar-footer">
        <div class="version"><a :href="APP_VERSION_URL" target="_blank" rel="noopener">{{ APP_VERSION }}</a></div>
      </div>
    </aside>

    <!-- Main Content -->
    <div class="main-content">
      <header class="topbar">
        <button class="hamburger" @click="sidebarOpen = true"><Menu :size="22" /></button>
        <div class="topbar-title">
          <h2>{{ pageTitle }}</h2>
        </div>
        <div class="topbar-actions">
          <ThemeToggle />
          <LanguageSwitcher />
          <button class="btn btn-ghost btn-sm" @click="handleLogout"><LogOut :size="16" /> {{ t('common.logout') }}</button>
        </div>
      </header>
      <main class="content">
        <router-view v-slot="{ Component }">
          <Transition name="page-slide" mode="out-in">
            <component :is="Component" />
          </Transition>
        </router-view>
      </main>
    </div>
  </div>

  <Toast />
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import Toast from '@/components/common/Toast.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue'
import {
  LayoutDashboard, KeyRound, GitBranch, Server, Play,
  Container, Globe, TrendingUp, Bot, RefreshCw, Package,
  Save, Settings, Monitor, Menu, LogOut, X, CalendarClock,
} from '@lucide/vue'

const { t } = useI18n()

// Version info injected at build time by Vite
declare const __APP_VERSION__: string
declare const __APP_VERSION_URL__: string
const APP_VERSION = __APP_VERSION__
const APP_VERSION_URL = __APP_VERSION_URL__

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const sidebarOpen = ref(false)

const navItems = computed(() => [
  { path: '/',            icon: LayoutDashboard, label: t('common.dashboard'),   group: 'overview' },
  { path: '/proxy',       icon: Play,            label: t('common.proxy'),       group: 'proxy' },
  { path: '/instances',   icon: Server,          label: t('common.instances'),   group: 'proxy' },
  { path: '/upstreams',   icon: GitBranch,       label: t('common.upstreams'),   group: 'proxy' },
  { path: '/docker',      icon: Container,       label: t('common.docker'),      group: 'proxy' },
  { path: '/secrets',     icon: KeyRound,        label: t('common.secrets'),     group: 'security' },
  { path: '/geoblock',    icon: Globe,           label: t('common.geoblock'),    group: 'security' },
  { path: '/traffic',     icon: TrendingUp,      label: t('common.traffic'),     group: 'monitoring' },
  { path: '/bot',         icon: Bot,             label: t('common.bot'),         group: 'integrations' },
  { path: '/replication', icon: RefreshCw,       label: t('common.replication'), group: 'integrations' },
  { path: '/updates',     icon: Package,         label: t('common.updates'),     group: 'system' },
  { path: '/scheduler',   icon: CalendarClock,   label: t('common.scheduler'),   group: 'system' },
  { path: '/backups',     icon: Save,            label: t('common.backups'),     group: 'system' },
  { path: '/settings',    icon: Settings,        label: t('common.settings'),    group: 'system' },
  { path: '/system',      icon: Monitor,         label: t('common.system'),      group: 'system' },
])

const groupedNavItems = computed(() => {
  const groupOrder = ['overview', 'proxy', 'security', 'monitoring', 'integrations', 'system']
  const groupLabels: Record<string, string> = {
    overview: t('nav.groups.overview'),
    proxy: t('nav.groups.proxy'),
    security: t('nav.groups.security'),
    monitoring: t('nav.groups.monitoring'),
    integrations: t('nav.groups.integrations'),
    system: t('nav.groups.system'),
  }
  return groupOrder
    .filter((g) => navItems.value.some((item) => item.group === g))
    .map((g) => ({
      label: groupLabels[g],
      items: navItems.value.filter((item) => item.group === g),
    }))
})

const pageTitle = computed(() => {
  const allItems = navItems.value
  const current = allItems.find((item) => {
    if (item.path === '/') return route.path === '/'
    return route.path.startsWith(item.path)
  })
  return current?.label ?? t('common.dashboard')
})

async function handleLogout() {
  await auth.logout()
  router.push('/auth/login')
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.main-layout {
  display: flex;
  min-height: 100vh;
}

/* Sidebar */
.sidebar {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: $sidebar-width;
  background: $bg-sidebar;
  color: $color-white;
  z-index: $z-fixed;
  display: flex;
  flex-direction: column;
  transition: transform $transition-normal;

  @media (max-width: 768px) {
    transform: translateX(-100%);
    &.open { transform: translateX(0); }
  }
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: $z-fixed - 1;

  @media (min-width: 769px) { display: none; }
}

.sidebar-header {
  height: $sidebar-header-height;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 $spacing-lg;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  text-decoration: none;
  color: $color-white;
  font-weight: $font-weight-bold;
  font-size: $font-size-lg;
}

.logo-img {
  width: 56px;
  height: 56px;
  object-fit: contain;
}

.sidebar-close {
  display: none;
  background: none;
  border: none;
  color: $color-white;
  cursor: pointer;
  padding: $spacing-xs;

  @media (max-width: 768px) { display: block; }
}

.sidebar-nav {
  flex: 1;
  padding: $spacing-md 0;
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  padding: 10px $spacing-lg;
  color: rgba(255, 255, 255, 0.7);
  text-decoration: none;
  font-size: $font-size-sm;
  transition: all $transition-fast;

  &:hover {
    background: rgba(255, 255, 255, 0.05);
    color: $color-white;
  }

  &.active {
    background: rgba(99, 102, 241, 0.3);
    color: $color-primary-light;
    border-right: 3px solid $color-primary-light;
  }
}

.nav-icon { flex-shrink: 0; }
.nav-label { white-space: nowrap; }

.nav-group-label {
  padding: 16px $spacing-lg 4px;
  font-size: $font-size-xs;
  font-weight: $font-weight-semibold;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: rgba(255, 255, 255, 0.35);
}

.sidebar-footer {
  padding: $spacing-md $spacing-lg;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.version {
  font-size: $font-size-xs;
  color: rgba(255, 255, 255, 0.4);
  text-align: center;

  a {
    color: rgba(255, 255, 255, 0.5);
    text-decoration: none;
    transition: color $transition-fast;

    &:hover {
      color: rgba(255, 255, 255, 0.8);
      text-decoration: underline;
    }
  }
}

/* Main Content */
.main-content {
  flex: 1;
  margin-left: $sidebar-width;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  min-width: 0;
  overflow-x: hidden;

  @media (max-width: 768px) { margin-left: 0; }
}

.topbar {
  height: $sidebar-header-height;
  background: $bg-card;
  border-bottom: 1px solid $border-color;
  display: flex;
  align-items: center;
  padding: 0 $spacing-lg;
  gap: $spacing-md;
  position: sticky;
  top: 0;
  z-index: $z-sticky;
}

.hamburger {
  display: none;
  background: none;
  border: none;
  cursor: pointer;
  padding: $spacing-sm;

  @media (max-width: 768px) { display: block; }
}

.topbar-title {
  flex: 1;

  h2 {
    font-size: $font-size-lg;
    font-weight: $font-weight-semibold;
    margin: 0;
  }
}

.topbar-actions {
  display: flex;
  gap: $spacing-sm;
  align-items: center;
  flex-shrink: 0;
}

.content {
  flex: 1;
  padding: $spacing-lg;

  @media (max-width: 768px) { padding: $spacing-md; }
}
</style>
