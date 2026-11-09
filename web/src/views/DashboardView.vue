<template>
  <div>
    <h1 class="page-title">Dashboard</h1>
    <div class="stat-grid">
      <div class="stat-card">
        <div class="label">Customers</div>
        <div class="value">{{ stats.customers }}</div>
      </div>
      <div class="stat-card">
        <div class="label">Projects</div>
        <div class="value">{{ stats.projects }}</div>
      </div>
      <div class="stat-card">
        <div class="label">Reports (loaded)</div>
        <div class="value">{{ stats.reports }}</div>
      </div>
      <div class="stat-card">
        <div class="label">This month</div>
        <div class="value">{{ stats.monthHM }}</div>
      </div>
    </div>

    <div class="card-panel">
      <h2 class="mt-0 mb-2" style="font-size: 1.1rem">Welcome, {{ auth.displayName }}</h2>
      <p class="muted m-0">
        Use the navigation to manage customers and time reports.
        Monthly summaries are under <strong>Monthly</strong>; PDF timesheets under
        <strong>Timesheets</strong>.
      </p>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive } from 'vue'
import api from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { currentMonth } from '@/utils/datetime'

const auth = useAuthStore()
const stats = reactive({
  customers: '—',
  projects: '—',
  reports: '—',
  monthHM: '—',
})

onMounted(async () => {
  try {
    const [c, p, r, m] = await Promise.all([
      api.get('/customers'),
      api.get('/projects'),
      api.get('/reports', { params: { limit: 500 } }),
      api.get('/reports/monthly', {
        params: { month: currentMonth() },
      }),
    ])
    stats.customers = c.data?.data?.length ?? 0
    stats.projects = p.data?.data?.length ?? 0
    stats.reports = r.data?.data?.length ?? 0
    stats.monthHM = m.data?.total_hm ?? '00:00'
  } catch {
    /* stats stay as placeholders */
  }
})
</script>
