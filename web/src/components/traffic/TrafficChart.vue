<template>
  <div class="traffic-chart">
    <canvas ref="chartCanvas"></canvas>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { Chart, LineController, LineElement, PointElement, LinearScale, TimeScale, Filler, Tooltip, Legend, CategoryScale } from 'chart.js'
import type { ChartOptions } from 'chart.js'
import type { TrafficHistoryRecord } from '@/types/models'

Chart.register(LineController, LineElement, PointElement, LinearScale, TimeScale, Filler, Tooltip, Legend, CategoryScale)

const props = defineProps<{
  records: TrafficHistoryRecord[]
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

function getThemeColors() {
  const style = getComputedStyle(document.documentElement)
  return {
    text: style.getPropertyValue('--text-secondary').trim() || '#4b5563',
    grid: style.getPropertyValue('--border-color').trim() || '#e5e7eb',
  }
}

function buildChart() {
  if (!chartCanvas.value || props.records.length === 0) return

  if (chart) {
    chart.destroy()
    chart = null
  }

  const colors = getThemeColors()
  const labels = props.records.map(r => {
    const d = new Date(r.timestamp * 1000)
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  })

  const downloadData = props.records.map(r => r.bytes_in)
  const uploadData = props.records.map(r => r.bytes_out)

  chart = new Chart(chartCanvas.value, {
    type: 'line',
    data: {
      labels,
      datasets: [
        {
          label: 'Download',
          data: downloadData,
          borderColor: '#3b82f6',
          backgroundColor: 'rgba(59, 130, 246, 0.1)',
          fill: true,
          tension: 0.3,
          pointRadius: 0,
          pointHitRadius: 10,
          borderWidth: 2,
        },
        {
          label: 'Upload',
          data: uploadData,
          borderColor: '#10b981',
          backgroundColor: 'rgba(16, 185, 129, 0.1)',
          fill: true,
          tension: 0.3,
          pointRadius: 0,
          pointHitRadius: 10,
          borderWidth: 2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: {
        mode: 'index',
        intersect: false,
      },
      plugins: {
        tooltip: {
          callbacks: {
            label: (ctx) => `${ctx.dataset.label}: ${formatBytes(ctx.parsed.y ?? 0)}`,
          },
        },
        legend: {
          labels: { color: colors.text },
        },
      },
      scales: {
        x: {
          ticks: {
            color: colors.text,
            maxTicksLimit: 8,
            maxRotation: 0,
          },
          grid: { color: colors.grid },
        },
        y: {
          ticks: {
            color: colors.text,
            callback: (value) => formatBytes(value as number),
          },
          grid: { color: colors.grid },
          beginAtZero: true,
        },
      },
    } satisfies ChartOptions<'line'>,
  })
}

watch(() => props.records, () => buildChart(), { deep: true })

onMounted(() => buildChart())

onUnmounted(() => {
  if (chart) {
    chart.destroy()
    chart = null
  }
})
</script>

<style scoped lang="scss">
.traffic-chart {
  position: relative;
  height: 300px;
}
</style>
