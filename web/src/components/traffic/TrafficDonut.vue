<template>
  <div class="traffic-donut">
    <canvas ref="chartCanvas"></canvas>
  </div>
</template>

<script setup lang="ts">
import {onMounted, onUnmounted, ref, watch} from 'vue'
import type {ChartOptions} from 'chart.js'
import {ArcElement, Chart, DoughnutController, Legend, Tooltip} from 'chart.js'
import type {UserTraffic} from '@/types/models'

Chart.register(DoughnutController, ArcElement, Tooltip, Legend)

const props = defineProps<{
  users: UserTraffic[]
  activeIndex: number | null
}>()

const emit = defineEmits<{
  hover: [index: number | null]
}>()

const chartCanvas = ref<HTMLCanvasElement | null>(null)
let chart: Chart | null = null

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const val = bytes / Math.pow(1024, i)
  return val.toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

const palette = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#ec4899', '#06b6d4', '#84cc16', '#f97316', '#6366f1',
]

function highlightPalette(idx: number | null): string[] {
  return palette.slice(0, props.users.length).map((c, i) => {
    if (idx === null) return c
    return i === idx ? c : c + '33'
  })
}

function buildChart() {
  if (!chartCanvas.value || props.users.length === 0) return

  if (chart) {
    chart.destroy()
    chart = null
  }

  const totals = props.users.map(u => (u.bytes_in || 0) + (u.bytes_out || 0))
  const grandTotal = totals.reduce((a, b) => a + b, 0)
  const borderColor = getComputedStyle(document.documentElement).getPropertyValue('--bg-card').trim() || '#fff'

  chart = new Chart(chartCanvas.value, {
    type: 'doughnut',
    data: {
      labels: props.users.map(u => u.label),
      datasets: [{
        data: totals,
        backgroundColor: highlightPalette(props.activeIndex),
        borderWidth: 2,
        borderColor,
        hoverOffset: 6,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      cutout: '60%',
      onHover: (_event, elements) => {
        emit('hover', elements.length > 0 ? elements[0].index : null)
      },
      plugins: {
        tooltip: {
          callbacks: {
            label: (ctx) => {
              const val = ctx.parsed ?? 0
              const pct = grandTotal > 0 ? ((val / grandTotal) * 100).toFixed(1) : '0'
              return `${ctx.label}: ${formatBytes(val)} (${pct}%)`
            },
          },
        },
        legend: {
          display: false,
        },
      },
    } satisfies ChartOptions<'doughnut'>,
  })
}

function updateHighlight() {
  if (!chart || !props.users.length) return
  const ds = chart.data.datasets[0]
  ds.backgroundColor = highlightPalette(props.activeIndex)
  chart.update('none')
}

watch(() => props.users, () => buildChart(), { deep: true })
watch(() => props.activeIndex, () => updateHighlight())

onMounted(() => buildChart())

onUnmounted(() => {
  if (chart) {
    chart.destroy()
    chart = null
  }
})
</script>

<style scoped lang="scss">
.traffic-donut {
  position: relative;
  width: 300px;
  min-width: 300px;
}
</style>
