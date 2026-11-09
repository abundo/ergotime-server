<template>
  <div>
    <h1 class="page-title">Monthly report</h1>

    <div class="card-panel mb-3">
      <div class="flex flex-wrap gap-3 align-items-end">
        <div class="flex flex-column gap-1">
          <label>Month</label>
          <InputText v-model="month" type="month" @change="load" />
        </div>
        <div class="flex flex-column gap-1">
          <label>Customer</label>
          <Select
            v-model="customerId"
            :options="customerOptions"
            option-label="label"
            option-value="value"
            placeholder="All customers"
            show-clear
            style="min-width: 14rem"
            @change="onCustomerChange"
          />
        </div>
        <div class="flex flex-column gap-1">
          <label>Project</label>
          <Select
            v-model="projectId"
            :options="projectOptions"
            option-label="label"
            option-value="value"
            placeholder="All projects"
            show-clear
            style="min-width: 14rem"
            @change="load"
          />
        </div>
        <Button label="Refresh" icon="pi pi-refresh" @click="load" :loading="loading" />
      </div>
    </div>

    <div class="stat-grid">
      <div class="stat-card">
        <div class="label">Total time</div>
        <div class="value">{{ report?.total_hm || '00:00' }}</div>
      </div>
      <div class="stat-card">
        <div class="label">Hours (decimal)</div>
        <div class="value">{{ (report?.total_hours || 0).toFixed(2) }}</div>
      </div>
    </div>

    <div v-if="!loading && (!report?.customers || report.customers.length === 0)" class="card-panel muted">
      No time reported for this period.
    </div>

    <div
      v-for="cust in report?.customers || []"
      :key="cust.customer_id"
      class="card-panel mb-3"
    >
      <div class="flex justify-content-between align-items-center mb-2">
        <div>
          <strong>{{ cust.name }}</strong>
          <span v-if="cust.description" class="muted"> — {{ cust.description }}</span>
        </div>
        <Tag :value="cust.total_hm" severity="info" />
      </div>

      <div v-for="proj in cust.projects" :key="proj.project_id" class="mb-3">
        <div class="flex justify-content-between mb-1">
          <span>
            <strong>{{ proj.name }}</strong>
            <span v-if="proj.cost_center" class="muted"> ({{ proj.cost_center }})</span>
          </span>
          <span class="muted">{{ proj.total_hm }}</span>
        </div>
        <div v-for="day in proj.days" :key="day.date" class="mb-2 ml-2">
          <div class="flex justify-content-between mb-1">
            <span>{{ formatDate(day.date) }}</span>
            <span class="muted">{{ day.total_hm }}</span>
          </div>
          <DataTable :value="day.entries" size="small" striped-rows>
            <Column header="Start" style="width: 6rem">
              <template #body="{ data }">{{ formatTime(data.start) }}</template>
            </Column>
            <Column header="Stop" style="width: 6rem">
              <template #body="{ data }">{{ formatTime(data.stop) }}</template>
            </Column>
            <Column field="hm" header="Duration" style="width: 6rem" />
            <Column field="comment" header="Comment" />
          </DataTable>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import api, { errorMessage } from '@/api/client'
import { formatDate, formatTime, currentMonth } from '@/utils/datetime'

const toast = useToast()
const loading = ref(false)
const report = ref(null)
const customers = ref([])
const projects = ref([])
const customerId = ref(null)
const projectId = ref(null)
const month = ref(currentMonth())

const customerOptions = computed(() =>
  customers.value.map((c) => ({ label: c.name, value: c.id })),
)

const projectOptions = computed(() => {
  let list = projects.value
  if (customerId.value) {
    list = list.filter((p) => p.customer_id === customerId.value)
  }
  return list.map((p) => ({ label: p.name, value: p.id }))
})

function onCustomerChange() {
  projectId.value = null
  load()
}

async function load() {
  loading.value = true
  try {
    const params = { month: month.value }
    if (customerId.value) params.customer_id = customerId.value
    if (projectId.value) params.project_id = projectId.value
    const { data } = await api.get('/reports/monthly', { params })
    report.value = data
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Error', detail: errorMessage(err), life: 4000 })
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    const [c, p] = await Promise.all([
      api.get('/customers'),
      api.get('/projects'),
    ])
    customers.value = c.data.data || []
    projects.value = p.data.data || []
  } catch {
    /* ignore */
  }
  await load()
})
</script>
