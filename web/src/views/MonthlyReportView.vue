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
                <div class="value">{{ report?.total_hm || "00:00" }}</div>
            </div>
            <div class="stat-card">
                <div class="label">Hours (decimal)</div>
                <div class="value">{{ (report?.total_hours || 0).toFixed(2) }}</div>
            </div>
        </div>

        <div v-if="!loading && (!report?.customers || report.customers.length === 0)" class="card-panel muted">
            No time reported for this period.
        </div>

        <div v-for="cust in report?.customers || []" :key="cust.customer_id" class="card-panel mb-3">
            <div class="flex justify-content-between align-items-center mb-3">
                <div>
                    <strong>{{ cust.name }}</strong>
                    <span v-if="cust.description" class="muted"> — {{ cust.description }}</span>
                </div>
                <Tag :value="cust.total_hm" severity="info" />
            </div>

            <div v-for="day in cust.days" :key="day.date" class="day-block">
                <div class="day-heading">{{ dayHeading(day.date) }}</div>
                <table class="sheet">
                    <thead>
                        <tr>
                            <th class="col-project">Project</th>
                            <th class="col-comment">Comment</th>
                            <th class="col-time">Start</th>
                            <th class="col-time">Stop</th>
                            <th class="col-time">Length</th>
                        </tr>
                    </thead>
                    <tbody>
                        <template v-for="(entry, ei) in day.entries" :key="entry.id || ei">
                            <tr v-for="(note, ni) in entryNotes(entry)" :key="ni" :class="{ stripe: ei % 2 === 1 }">
                                <td class="project" :class="{ sep: ni > 0 }">{{ note.project }}</td>
                                <td class="comment" :class="{ sep: ni > 0 }">{{ note.comment }}</td>
                                <td v-if="ni === 0" class="num" :rowspan="entryNotes(entry).length">
                                    {{ entry.start }}
                                </td>
                                <td v-if="ni === 0" class="num" :rowspan="entryNotes(entry).length">
                                    {{ entry.stop }}
                                </td>
                                <td v-if="ni === 0" class="num" :rowspan="entryNotes(entry).length">{{ entry.hm }}</td>
                            </tr>
                        </template>
                        <tr class="day-total">
                            <td colspan="4">Day total</td>
                            <td class="num">{{ day.total_hm }}</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <h2 class="summary-title">Summary by project</h2>
            <table class="sheet summary">
                <thead>
                    <tr>
                        <th>Project</th>
                        <th class="col-amount">Amount</th>
                        <th class="col-unit">Unit</th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="proj in cust.projects" :key="proj.project_id + proj.name">
                        <td>
                            {{ proj.name }}
                            <span v-if="proj.cost_center" class="muted">({{ proj.cost_center }})</span>
                        </td>
                        <td class="num">{{ proj.total_hm }}</td>
                        <td class="num">{{ proj.unit }}</td>
                    </tr>
                    <tr class="grand">
                        <td>Total</td>
                        <td class="num">{{ cust.total_hm }}</td>
                        <td></td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { useToast } from "primevue/usetoast";
import InputText from "primevue/inputtext";
import Select from "primevue/select";
import Button from "primevue/button";
import Tag from "primevue/tag";
import api, { errorMessage } from "@/api/client";
import { currentMonth } from "@/utils/datetime";

const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

const toast = useToast();
const loading = ref(false);
const report = ref(null);
const customers = ref([]);
const projects = ref([]);
const customerId = ref(null);
const projectId = ref(null);
const month = ref(currentMonth());

const customerOptions = computed(() => customers.value.map((c) => ({ label: c.name, value: c.id })));

const projectOptions = computed(() => {
    let list = projects.value;
    if (customerId.value) {
        list = list.filter((p) => p.customer_id === customerId.value);
    }
    return list.map((p) => ({ label: p.name, value: p.id }));
});

function onCustomerChange() {
    projectId.value = null;
    load();
}

function dayHeading(date) {
    const m = String(date || "").match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (!m) return date || "";
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
    return `${m[1]}-${m[2]}-${m[3]} ${weekdays[d.getDay()]}`;
}

function entryNotes(entry) {
    if (entry?.notes?.length) return entry.notes;
    return [{ project: "", comment: "" }];
}

async function load() {
    loading.value = true;
    try {
        const params = { month: month.value };
        if (customerId.value) params.customer_id = customerId.value;
        if (projectId.value) params.project_id = projectId.value;
        const { data } = await api.get("/reports/monthly", { params });
        report.value = data;
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        loading.value = false;
    }
}

onMounted(async () => {
    try {
        const [c, p] = await Promise.all([api.get("/customers"), api.get("/projects")]);
        customers.value = c.data.data || [];
        projects.value = p.data.data || [];
    } catch {
        /* ignore */
    }
    await load();
});
</script>

<style scoped>
.day-block {
    margin-bottom: 1.25rem;
}
.day-heading {
    font-weight: 700;
    font-size: 1.05rem;
    background: #f5f5f5;
    border-bottom: 1px solid #b4b4b4;
    padding: 0.35rem 0.5rem;
}
.sheet {
    width: 100%;
    border-collapse: collapse;
    margin-bottom: 0.25rem;
}
.sheet th,
.sheet td {
    border: 1px solid #222;
    padding: 0.35rem 0.5rem;
    vertical-align: top;
    text-align: left;
}
.sheet th {
    background: #e6e6e6;
    font-size: 0.85rem;
}
.sheet td.num,
.sheet th.col-time,
.sheet th.col-amount,
.sheet th.col-unit {
    text-align: center;
    white-space: nowrap;
}
.col-project {
    width: 22%;
}
.col-time,
.col-amount,
.col-unit {
    width: 5.5rem;
}
.sheet tr.stripe td {
    background: #f8f8fa;
}
.sheet td.project {
    font-weight: 700;
}
.sheet td.comment {
    white-space: pre-wrap;
}
.sheet td.sep {
    border-top: 1px solid #d2d2d2;
}
.sheet tr.day-total td {
    border: none;
    font-weight: 700;
    text-align: right;
    padding-top: 0.35rem;
}
.sheet tr.day-total td.num {
    text-align: center;
}
.summary-title {
    margin: 1.25rem 0 0.5rem;
    font-size: 1.05rem;
}
.sheet.summary tr.grand td {
    font-weight: 700;
    background: #dcebfa;
}
</style>
