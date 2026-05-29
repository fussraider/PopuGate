<template>
  <div class="connections-chart">
    <canvas ref="chartCanvas"></canvas>
  </div>
</template>

<script setup lang="ts">
import {onMounted, onUnmounted, ref, watch} from 'vue'
import type {ChartOptions} from 'chart.js'
import {
  CategoryScale,
  Chart,
  Filler,
  Legend,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  TimeScale,
  Tooltip
} from 'chart.js'
import type {TrafficHistoryRecord} from '@/types/models'

Chart.register(LineController, LineElement, PointElement, LinearScale, TimeScale, Filler, Tooltip, Legend, CategoryScale)

const props = defineProps<{
  records: TrafficHistoryRecord[]
}>()

const chartCanvas = ref<HTMLCanvasElement | null>(null)
let chart: Chart | null = null

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

  const data = props.records.map(r => r.connections || 0)

  chart = new Chart(chartCanvas.value, {
    type: 'line',
    data: {
      labels,
      datasets: [
        {
          label: 'Connections',
          data,
          borderColor: '#8b5cf6',
          backgroundColor: 'rgba(139, 92, 246, 0.1)',
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
            label: (ctx) => `Connections: ${ctx.parsed.y ?? 0}`,
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
            precision: 0,
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
.connections-chart {
  position: relative;
  height: 300px;
}
</style>
