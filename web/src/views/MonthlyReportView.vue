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
            <div class="flex justify-content-between align-items-center mb-3 gap-2 flex-wrap">
                <div>
                    <strong>{{ cust.name }}</strong>
                    <span v-if="cust.description" class="muted"> — {{ cust.description }}</span>
                    <Tag
                        class="ml-2"
                        :severity="cust.invoiced ? 'success' : 'secondary'"
                        :value="cust.invoiced ? 'Invoiced' : 'Not invoiced'"
                    />
                </div>
                <div class="flex flex-wrap gap-2 align-items-center">
                    <Tag :value="cust.total_hm" severity="info" />
                    <Button
                        label="Generate PDF"
                        icon="pi pi-file-pdf"
                        size="small"
                        severity="info"
                        :loading="pdfLoadingId === cust.customer_id"
                        @click="generatePdf(cust)"
                    />
                    <Button
                        :label="cust.invoiced ? 'Unmark invoiced' : 'Mark as invoiced'"
                        :icon="cust.invoiced ? 'pi pi-times' : 'pi pi-check'"
                        size="small"
                        :severity="cust.invoiced ? 'secondary' : 'success'"
                        :loading="invoiceLoadingId === cust.customer_id"
                        @click="toggleInvoiced(cust)"
                    />
                </div>
            </div>

            <p v-if="cust.invoiced" class="muted mt-0 mb-3" style="font-size: 0.85rem">
                This month is marked invoiced — unmark it before moving time.
            </p>
            <p v-else-if="projectId" class="muted mt-0 mb-3" style="font-size: 0.85rem">
                Clear the project filter to move time.
            </p>

            <div v-for="day in cust.days" :key="day.date" class="day-block">
                <div class="day-heading">{{ dayHeading(day.date) }}</div>
                <table class="sheet">
                    <thead>
                        <tr>
                            <th class="col-time">Start</th>
                            <th class="col-time">Stop</th>
                            <th class="col-project">Project</th>
                            <th class="col-comment">Comment</th>
                            <th class="col-time">Length</th>
                        </tr>
                    </thead>
                    <tbody>
                        <template v-for="(entry, ei) in day.entries" :key="entry.id || ei">
                            <tr v-for="(note, ni) in entryNotes(entry)" :key="ni" :class="{ stripe: ei % 2 === 1 }">
                                <td v-if="ni === 0" class="num" :rowspan="entryNotes(entry).length">
                                    {{ entry.start }}
                                </td>
                                <td v-if="ni === 0" class="num" :rowspan="entryNotes(entry).length">
                                    {{ entry.stop }}
                                </td>
                                <td class="project" :class="{ sep: ni > 0 }">{{ note.project }}</td>
                                <td class="comment" :class="{ sep: ni > 0 }">{{ note.comment }}</td>
                                <td class="num" :class="{ sep: ni > 0 }">{{ note.length_hm }}</td>
                            </tr>
                        </template>
                        <tr class="day-total">
                            <td colspan="4">Day total</td>
                            <td class="num">{{ day.total_hm }}</td>
                        </tr>
                    </tbody>
                </table>

                <div v-if="canMove(day) || projectId" class="move-panel">
                    <p class="muted" style="margin-top: 0">
                        Move time between the projects on this day. The total that can be moved is
                        <strong>{{ day.total_hm }}</strong
                        >. Only the length changes.
                    </p>
                    <div class="flex flex-wrap gap-3 align-items-end">
                        <div class="flex flex-column gap-1" style="min-width: 10rem; flex: 1">
                            <label :for="'move-from-' + cust.customer_id + '-' + day.date">From project</label>
                            <Select
                                :input-id="'move-from-' + cust.customer_id + '-' + day.date"
                                v-model="moveState(cust, day).from"
                                :options="fromProjectOptions(day)"
                                option-label="label"
                                option-value="value"
                                placeholder="Select source"
                                class="w-full"
                                :disabled="moveDisabled(cust, day)"
                                @update:model-value="onMoveFromChange(cust, day, $event)"
                            />
                        </div>
                        <div class="flex flex-column gap-1" style="min-width: 10rem; flex: 1">
                            <label :for="'move-to-' + cust.customer_id + '-' + day.date">To project</label>
                            <Select
                                :input-id="'move-to-' + cust.customer_id + '-' + day.date"
                                v-model="moveState(cust, day).to"
                                :options="toProjectOptions(cust, day)"
                                option-label="label"
                                option-value="value"
                                placeholder="Select target"
                                class="w-full"
                                :disabled="moveDisabled(cust, day)"
                            />
                        </div>
                        <div class="flex flex-column gap-1" style="width: 7rem">
                            <label :for="'move-hm-' + cust.customer_id + '-' + day.date">Amount (HH:MM)</label>
                            <InputText
                                :id="'move-hm-' + cust.customer_id + '-' + day.date"
                                v-model="moveState(cust, day).hm"
                                placeholder="1:30"
                                class="w-full"
                                :disabled="moveDisabled(cust, day)"
                                @keyup.enter="applyMove(cust, day)"
                            />
                        </div>
                        <Button
                            label="Move"
                            icon="pi pi-arrows-h"
                            :loading="moveLoadingKey === moveKey(cust, day)"
                            :disabled="
                                moveDisabled(cust, day) ||
                                sameProject(moveState(cust, day).from, moveState(cust, day).to)
                            "
                            @click="applyMove(cust, day)"
                        />
                    </div>
                </div>
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
import { currentMonth, weekdayName } from "@/utils/datetime";

const toast = useToast();
const loading = ref(false);
const report = ref(null);
const customers = ref([]);
const projects = ref([]);
const customerId = ref(null);
const projectId = ref(null);
const month = ref(currentMonth());
const pdfLoadingId = ref(null);
const invoiceLoadingId = ref(null);
const moveLoadingKey = ref("");
const moveByDay = ref({});

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
    const day = weekdayName(date);
    return day ? `${m[1]}-${m[2]}-${m[3]} ${day}` : `${m[1]}-${m[2]}-${m[3]}`;
}

function entryNotes(entry) {
    if (entry?.notes?.length) return entry.notes;
    return [{ project: "", comment: "" }];
}

function moveKey(cust, day) {
    return `${cust.customer_id}|${day.date}`;
}

function moveState(cust, day) {
    const key = moveKey(cust, day);
    if (!moveByDay.value[key]) {
        moveByDay.value[key] = { from: null, to: null, hm: "" };
    }
    return moveByDay.value[key];
}

function namedProjects(day) {
    return (day.projects || []).filter((p) => p.project_id > 0);
}

function canMove(day) {
    return namedProjects(day).length > 1;
}

function moveDisabled(cust, day) {
    return cust.invoiced || !!projectId.value || !canMove(day);
}

function projectOptionLabel(proj) {
    return `${proj.name} (${proj.total_hm} ${proj.unit || ""})`.trim();
}

function sameProject(a, b) {
    const left = Number(a);
    const right = Number(b);
    return left > 0 && left === right;
}

function onMoveFromChange(cust, day, fromId) {
    const state = moveState(cust, day);
    if (sameProject(fromId, state.to)) {
        state.to = null;
    }
}

function fromProjectOptions(day) {
    return namedProjects(day)
        .filter((p) => p.total_hm !== "00:00")
        .map((p) => ({ value: p.project_id, label: projectOptionLabel(p) }));
}

function toProjectOptions(cust, day) {
    const from = moveState(cust, day).from;
    return namedProjects(day)
        .filter((p) => !sameProject(p.project_id, from))
        .map((p) => ({ value: p.project_id, label: projectOptionLabel(p) }));
}

async function generatePdf(cust) {
    if (cust.invoiced) {
        toast.add({
            severity: "warn",
            summary: "Already invoiced",
            detail: "This month is already marked as invoiced. Generating PDF anyway.",
            life: 5000,
        });
    }
    pdfLoadingId.value = cust.customer_id;
    try {
        const res = await api.get("/timesheets/pdf", {
            params: { month: month.value, customer_id: cust.customer_id },
            responseType: "blob",
        });
        const blob = new Blob([res.data], { type: "application/pdf" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        const safe = (cust.name || "customer").replace(/[^\w\-]+/g, "-").toLowerCase();
        a.href = url;
        a.download = `timesheet-${safe}-${month.value}.pdf`;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        toast.add({ severity: "success", summary: "PDF generated", life: 2000 });
    } catch (err) {
        let detail = errorMessage(err);
        if (err?.response?.data instanceof Blob) {
            try {
                const text = await err.response.data.text();
                const parsed = JSON.parse(text);
                if (parsed.message) detail = parsed.message;
            } catch {
                /* keep default */
            }
        }
        toast.add({ severity: "error", summary: "Error", detail, life: 4000 });
    } finally {
        pdfLoadingId.value = null;
    }
}

async function toggleInvoiced(cust) {
    invoiceLoadingId.value = cust.customer_id;
    const next = !cust.invoiced;
    try {
        await api.put("/timesheets/invoiced", {
            customer_id: cust.customer_id,
            month: month.value,
            invoiced: next,
        });
        cust.invoiced = next;
        toast.add({
            severity: "success",
            summary: next ? "Marked as invoiced" : "Invoice mark cleared",
            life: 2000,
        });
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        invoiceLoadingId.value = null;
    }
}

async function applyMove(cust, day) {
    const state = moveState(cust, day);
    if (moveDisabled(cust, day)) return;
    if (!state.from || !state.to) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Select both from and to projects",
            life: 3000,
        });
        return;
    }
    if (sameProject(state.from, state.to)) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Choose a different project to move the time to",
            life: 3000,
        });
        return;
    }
    const hm = (state.hm || "").trim();
    if (!hm) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Enter amount as HH:MM",
            life: 3000,
        });
        return;
    }

    const prevTotal = cust.total_hm;
    const prevDay = day.total_hm;
    const date = day.date;
    const customerId = cust.customer_id;
    moveLoadingKey.value = moveKey(cust, day);
    try {
        const { data } = await api.put("/timesheets/reallocate", {
            customer_id: customerId,
            month: month.value,
            date,
            moves: [{ from_project_id: state.from, to_project_id: state.to, hm }],
        });
        state.hm = "";
        await load();
        const afterCust = (report.value?.customers || []).find((c) => c.customer_id === customerId);
        const afterDay = (afterCust?.days || []).find((d) => d.date === date);
        if (prevTotal && data.total_hm && data.total_hm !== prevTotal) {
            toast.add({
                severity: "error",
                summary: "Sum mismatch",
                detail: `Month total changed from ${prevTotal} to ${data.total_hm} — please refresh and check`,
                life: 6000,
            });
        } else if (afterDay && prevDay && afterDay.total_hm !== prevDay) {
            toast.add({
                severity: "error",
                summary: "Sum mismatch",
                detail: `Day total changed from ${prevDay} to ${afterDay.total_hm} — please refresh and check`,
                life: 6000,
            });
        } else {
            toast.add({ severity: "success", summary: "Time moved", life: 2000 });
        }
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        moveLoadingKey.value = "";
    }
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
.move-panel {
    margin-top: 1rem;
    padding: 0.75rem;
    background: #f8f9fa;
    border: 1px solid #e0e0e0;
}
</style>
