<template>
    <div>
        <h1 class="page-title">Timesheets</h1>

        <div class="card-panel mb-3">
            <div class="flex flex-wrap gap-3 align-items-end">
                <div class="flex flex-column gap-1">
                    <label for="ts-month">Year / month</label>
                    <InputText id="ts-month" v-model="month" type="month" @change="load" />
                </div>
                <Button label="Refresh" icon="pi pi-refresh" @click="load" :loading="loading" />
            </div>
        </div>

        <div class="card-panel">
            <DataTable
                :value="customers"
                :loading="loading"
                paginator
                :rows="20"
                striped-rows
                size="small"
                data-key="customer_id"
                empty-message="No customers with reported time for this month"
            >
                <Column field="name" header="Customer" sortable>
                    <template #body="{ data }">
                        <div>
                            <strong>{{ data.name }}</strong>
                            <div v-if="data.description" class="muted" style="font-size: 0.85rem">
                                {{ data.description }}
                            </div>
                        </div>
                    </template>
                </Column>
                <Column field="total_hm" header="Total time" style="width: 8rem" sortable />
                <Column field="entry_count" header="Entries" style="width: 6rem" sortable />
                <Column header="Invoiced" style="width: 7rem">
                    <template #body="{ data }">
                        <Tag
                            :severity="data.invoiced ? 'success' : 'secondary'"
                            :value="data.invoiced ? 'Yes' : 'No'"
                        />
                    </template>
                </Column>
                <Column header="" style="width: 22rem">
                    <template #body="{ data }">
                        <div class="flex flex-wrap gap-2">
                            <Button
                                label="Projects"
                                icon="pi pi-sitemap"
                                size="small"
                                @click="openProjectsDialog(data)"
                            />
                            <Button
                                label="Generate PDF"
                                icon="pi pi-file-pdf"
                                size="small"
                                severity="info"
                                :loading="pdfLoadingId === data.customer_id"
                                @click="generatePdf(data)"
                            />
                            <Button
                                :label="data.invoiced ? 'Unmark invoiced' : 'Mark as invoiced'"
                                :icon="data.invoiced ? 'pi pi-times' : 'pi pi-check'"
                                size="small"
                                :severity="data.invoiced ? 'secondary' : 'success'"
                                :loading="invoiceLoadingId === data.customer_id"
                                @click="toggleInvoiced(data)"
                            />
                        </div>
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Per-customer project totals + reallocate time -->
        <Dialog
            v-model:visible="projectsDialogVisible"
            :header="projectsDialogHeader"
            modal
            style="width: min(720px, 96vw)"
            @hide="onProjectsDialogHide"
        >
            <p class="muted mb-3" style="margin-top: 0">
                Reported time by project. Move time between projects without changing the month total (<strong>{{
                    projectsTotalHM
                }}</strong
                >).
            </p>

            <DataTable
                :value="projectRows"
                :loading="projectsLoading"
                size="small"
                striped-rows
                data-key="project_id"
                empty-message="No projects"
                class="mb-3"
            >
                <Column field="name" header="Project" sortable />
                <Column field="unit" header="Unit" style="width: 7rem" />
                <Column field="total_hm" header="Amount" style="width: 7rem" sortable />
                <Column field="entry_count" header="Entries" style="width: 6rem" />
            </DataTable>

            <div class="card-panel" style="background: var(--p-surface-50, #f8f9fa)">
                <div class="flex flex-wrap gap-3 align-items-end">
                    <div class="flex flex-column gap-1" style="min-width: 10rem; flex: 1">
                        <label for="move-from">From project</label>
                        <Select
                            id="move-from"
                            v-model="moveFromId"
                            :options="fromProjectOptions"
                            option-label="label"
                            option-value="value"
                            placeholder="Select source"
                            class="w-full"
                            :disabled="projectsCustomer?.invoiced"
                        />
                    </div>
                    <div class="flex flex-column gap-1" style="min-width: 10rem; flex: 1">
                        <label for="move-to">To project</label>
                        <Select
                            id="move-to"
                            v-model="moveToId"
                            :options="toProjectOptions"
                            option-label="label"
                            option-value="value"
                            placeholder="Select target"
                            class="w-full"
                            :disabled="projectsCustomer?.invoiced"
                        />
                    </div>
                    <div class="flex flex-column gap-1" style="width: 7rem">
                        <label for="move-hm">Amount (HH:MM)</label>
                        <InputText
                            id="move-hm"
                            v-model="moveHM"
                            placeholder="1:30"
                            class="w-full"
                            :disabled="projectsCustomer?.invoiced"
                            @keyup.enter="applyMove"
                        />
                    </div>
                    <Button
                        label="Move"
                        icon="pi pi-arrows-h"
                        :loading="moveLoading"
                        :disabled="projectsCustomer?.invoiced"
                        @click="applyMove"
                    />
                </div>
                <p v-if="projectsCustomer?.invoiced" class="muted mt-2 mb-0" style="font-size: 0.85rem">
                    This timesheet is marked invoiced — unmark it before moving time.
                </p>
            </div>

            <template #footer>
                <Button label="Close" text @click="projectsDialogVisible = false" />
            </template>
        </Dialog>
    </div>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { useToast } from "primevue/usetoast";
import InputText from "primevue/inputtext";
import Button from "primevue/button";
import DataTable from "primevue/datatable";
import Column from "primevue/column";
import Tag from "primevue/tag";
import Dialog from "primevue/dialog";
import Select from "primevue/select";
import api, { errorMessage } from "@/api/client";
import { currentMonth } from "@/utils/datetime";

const toast = useToast();
const loading = ref(false);
const customers = ref([]);
const month = ref(currentMonth());
const pdfLoadingId = ref(null);
const invoiceLoadingId = ref(null);

const projectsDialogVisible = ref(false);
const projectsCustomer = ref(null);
const projectRows = ref([]);
const projectsLoading = ref(false);
const projectsTotalHM = ref("00:00");
const projectsTotalSec = ref(0);
const moveFromId = ref(null);
const moveToId = ref(null);
const moveHM = ref("");
const moveLoading = ref(false);

const projectsDialogHeader = computed(() => {
    const name = projectsCustomer.value?.name || "Customer";
    return `Projects — ${name} (${month.value})`;
});

const fromProjectOptions = computed(() =>
    projectRows.value
        .filter((p) => p.total_sec > 0)
        .map((p) => ({
            value: p.project_id,
            label: `${p.name} (${p.total_hm} ${p.unit || ""})`.trim(),
        })),
);

const toProjectOptions = computed(() =>
    projectRows.value
        .filter((p) => p.project_id !== moveFromId.value)
        .map((p) => ({
            value: p.project_id,
            label: `${p.name} (${p.total_hm} ${p.unit || ""})`.trim(),
        })),
);

async function load() {
    loading.value = true;
    try {
        const { data } = await api.get("/timesheets", { params: { month: month.value } });
        customers.value = data.customers || [];
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function openProjectsDialog(row) {
    projectsCustomer.value = row;
    projectsDialogVisible.value = true;
    moveFromId.value = null;
    moveToId.value = null;
    moveHM.value = "";
    await loadProjects();
}

function onProjectsDialogHide() {
    projectsCustomer.value = null;
    projectRows.value = [];
    moveFromId.value = null;
    moveToId.value = null;
    moveHM.value = "";
}

function applyProjectsResponse(data) {
    projectRows.value = data.projects || [];
    projectsTotalHM.value = data.total_hm || "00:00";
    projectsTotalSec.value = data.total_sec || 0;
}

async function loadProjects() {
    if (!projectsCustomer.value) return;
    projectsLoading.value = true;
    try {
        const { data } = await api.get("/timesheets/projects", {
            params: { month: month.value, customer_id: projectsCustomer.value.customer_id },
        });
        applyProjectsResponse(data);
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        projectsLoading.value = false;
    }
}

async function applyMove() {
    if (!projectsCustomer.value) return;
    if (!moveFromId.value || !moveToId.value) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Select both from and to projects",
            life: 3000,
        });
        return;
    }
    if (moveFromId.value === moveToId.value) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "From and to must be different projects",
            life: 3000,
        });
        return;
    }
    const hm = (moveHM.value || "").trim();
    if (!hm) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Enter amount as HH:MM",
            life: 3000,
        });
        return;
    }

    const prevTotal = projectsTotalSec.value;
    moveLoading.value = true;
    try {
        const { data } = await api.put("/timesheets/reallocate", {
            customer_id: projectsCustomer.value.customer_id,
            month: month.value,
            moves: [
                {
                    from_project_id: moveFromId.value,
                    to_project_id: moveToId.value,
                    hm,
                },
            ],
        });
        applyProjectsResponse(data);
        // Invariant: month total must not change.
        if (prevTotal > 0 && data.total_sec !== prevTotal) {
            toast.add({
                severity: "error",
                summary: "Sum mismatch",
                detail: `Total changed from ${prevTotal}s to ${data.total_sec}s — please refresh and check`,
                life: 6000,
            });
        } else {
            toast.add({ severity: "success", summary: "Time moved", life: 2000 });
        }
        moveHM.value = "";
        // Refresh list row totals (entry counts may change after splits)
        await load();
        if (projectsCustomer.value) {
            const updated = customers.value.find((c) => c.customer_id === projectsCustomer.value.customer_id);
            if (updated) projectsCustomer.value = updated;
        }
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        moveLoading.value = false;
    }
}

async function generatePdf(row) {
    if (row.invoiced) {
        toast.add({
            severity: "warn",
            summary: "Already invoiced",
            detail: "This timesheet is already marked as invoiced. Generating PDF anyway.",
            life: 5000,
        });
    }
    pdfLoadingId.value = row.customer_id;
    try {
        const res = await api.get("/timesheets/pdf", {
            params: { month: month.value, customer_id: row.customer_id },
            responseType: "blob",
        });
        const blob = new Blob([res.data], { type: "application/pdf" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        const safe = (row.name || "customer").replace(/[^\w\-]+/g, "-").toLowerCase();
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

async function toggleInvoiced(row) {
    invoiceLoadingId.value = row.customer_id;
    const next = !row.invoiced;
    try {
        await api.put("/timesheets/invoiced", {
            customer_id: row.customer_id,
            month: month.value,
            invoiced: next,
        });
        row.invoiced = next;
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

onMounted(load);
</script>
