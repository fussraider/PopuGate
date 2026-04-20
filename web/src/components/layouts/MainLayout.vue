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
        <router-link
          v-for="item in navItems"
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
          <LanguageSwitcher />
          <button class="btn btn-ghost btn-sm" @click="handleLogout"><LogOut :size="16" /> {{ t('common.logout') }}</button>
        </div>
      </header>
      <main class="content">
        <router-view v-slot="{ Component }">
          <Transition name="page-fade" mode="out-in">
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
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue'
import {
  LayoutDashboard, KeyRound, GitBranch, Server, Play,
  Container, Globe, TrendingUp, Bot, RefreshCw, Package,
  Save, Settings, Monitor, Menu, LogOut, X,
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
  { path: '/',            icon: LayoutDashboard, label: t('common.dashboard')   },
  { path: '/secrets',     icon: KeyRound,        label: t('common.secrets')     },
  { path: '/upstreams',   icon: GitBranch,       label: t('common.upstreams')   },
  { path: '/instances',   icon: Server,           label: t('common.instances')   },
  { path: '/proxy',       icon: Play,             label: t('common.proxy')       },
  { path: '/docker',      icon: Container,        label: t('common.docker')      },
  { path: '/geoblock',    icon: Globe,            label: t('common.geoblock')    },
  { path: '/traffic',     icon: TrendingUp,       label: t('common.traffic')     },
  { path: '/bot',         icon: Bot,              label: t('common.bot')         },
  { path: '/replication', icon: RefreshCw,        label: t('common.replication') },
  { path: '/updates',     icon: Package,          label: t('common.updates')     },
  { path: '/backups',     icon: Save,             label: t('common.backups')     },
  { path: '/settings',    icon: Settings,         label: t('common.settings')    },
  { path: '/system',      icon: Monitor,          label: t('common.system')      },
])

// pageTitle берётся из navItems — единственный источник правды
const pageTitle = computed(() => {
  const current = navItems.value.find((item) => {
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
  color: $text-inverse;
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
  color: $text-inverse;
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
  color: $text-inverse;
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
    color: $text-inverse;
  }

  &.active {
    background: rgba(99, 102, 241, 0.3);
    color: $color-primary-light;
    border-right: 3px solid $color-primary-light;
  }
}

.nav-icon { flex-shrink: 0; }
.nav-label { white-space: nowrap; }

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
}

.content {
  flex: 1;
  padding: $spacing-lg;

  @media (max-width: 768px) { padding: $spacing-md; }
}
</style>
