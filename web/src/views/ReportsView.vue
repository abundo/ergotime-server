<template>
    <div>
        <div class="flex align-items-center justify-content-between mb-3 flex-wrap gap-2">
            <h1 class="page-title m-0">Time reports</h1>
            <Button label="New report" icon="pi pi-plus" @click="openCreate" />
        </div>

        <div class="card-panel mb-3">
            <div class="flex flex-wrap gap-3 align-items-end">
                <div class="flex flex-column gap-1">
                    <label>Customer</label>
                    <Select
                        v-model="filters.customer_id"
                        :options="customerOptions"
                        option-label="label"
                        option-value="value"
                        placeholder="All"
                        show-clear
                        style="min-width: 12rem"
                        @change="onFilterCustomerChange"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>Project</label>
                    <Select
                        v-model="filters.project_id"
                        :options="filterProjectOptions"
                        option-label="label"
                        option-value="value"
                        placeholder="All"
                        show-clear
                        style="min-width: 12rem"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>From</label>
                    <InputText
                        v-model="filters.from"
                        placeholder="YYYY-MM-DD"
                        class="w-full"
                        style="min-width: 10rem"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>To</label>
                    <InputText
                        v-model="filters.to"
                        placeholder="YYYY-MM-DD"
                        class="w-full"
                        style="min-width: 10rem"
                    />
                </div>
                <Button label="Filter" icon="pi pi-filter" @click="load" />
            </div>
        </div>

        <div class="card-panel">
            <DataTable
                :value="items"
                :loading="loading"
                paginator
                :rows="20"
                striped-rows
                size="small"
                data-key="id"
                empty-message="No reports found"
            >
                <Column field="id" header="ID" style="width: 5rem" />
                <Column header="Customer">
                    <template #body="{ data }">
                        {{ projectCustomerName(data.project_id) }}
                    </template>
                </Column>
                <Column header="Project">
                    <template #body="{ data }">
                        {{ projectName(data.project_id) }}
                    </template>
                </Column>
                <Column header="Start">
                    <template #body="{ data }">{{ formatDateTime(data.start) }}</template>
                </Column>
                <Column header="Stop">
                    <template #body="{ data }">{{ formatDateTime(data.stop) }}</template>
                </Column>
                <Column field="comment" header="Comment" />
                <Column header="" style="width: 8rem">
                    <template #body="{ data }">
                        <Button icon="pi pi-pencil" text rounded size="small" @click="openEdit(data)" />
                        <Button
                            icon="pi pi-trash"
                            text
                            rounded
                            size="small"
                            severity="danger"
                            @click="confirmDelete(data)"
                        />
                    </template>
                </Column>
            </DataTable>
        </div>

        <Dialog
            v-model:visible="dialogVisible"
            :header="editing ? 'Edit report' : 'New report'"
            modal
            style="width: min(520px, 95vw)"
        >
            <div class="flex flex-column gap-3">
                <div class="flex flex-column gap-1">
                    <label>Customer</label>
                    <Select
                        v-model="form.customer_id"
                        :options="customerOptions"
                        option-label="label"
                        option-value="value"
                        placeholder="Select customer"
                        class="w-full"
                        @change="onFormCustomerChange"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>Project</label>
                    <Select
                        v-model="form.project_id"
                        :options="formProjectOptions"
                        option-label="label"
                        option-value="value"
                        placeholder="Select project"
                        class="w-full"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>Start</label>
                    <InputText
                        v-model="form.start"
                        placeholder="YYYY-MM-DD HH:MM"
                        class="w-full"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>Stop</label>
                    <InputText
                        v-model="form.stop"
                        placeholder="YYYY-MM-DD HH:MM"
                        class="w-full"
                    />
                </div>
                <div class="flex flex-column gap-1">
                    <label>Comment</label>
                    <Textarea v-model="form.comment" rows="2" class="w-full" auto-resize />
                </div>
            </div>
            <template #footer>
                <Button label="Cancel" text @click="dialogVisible = false" />
                <Button label="Save" :loading="saving" @click="save" />
            </template>
        </Dialog>
    </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { useToast } from "primevue/usetoast";
import { useConfirm } from "primevue/useconfirm";
import DataTable from "primevue/datatable";
import Column from "primevue/column";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputText from "primevue/inputtext";
import Textarea from "primevue/textarea";
import Select from "primevue/select";
import api, { errorMessage } from "@/api/client";
import { formatDateTime, toLocalInput, fromLocalInput } from "@/utils/datetime";

const toast = useToast();
const confirm = useConfirm();

const items = ref([]);
const customers = ref([]);
const projects = ref([]);
const loading = ref(false);
const saving = ref(false);
const dialogVisible = ref(false);
const editing = ref(null);
const filters = reactive({ customer_id: null, project_id: null, from: "", to: "" });
const form = reactive({ customer_id: null, project_id: null, start: "", stop: "", comment: "" });

const customerOptions = computed(() => customers.value.map((c) => ({ label: c.name, value: c.id })));

const filterProjectOptions = computed(() => {
    let list = projects.value;
    if (filters.customer_id) {
        list = list.filter((p) => p.customer_id === filters.customer_id);
    }
    return list.map((p) => ({ label: p.name, value: p.id }));
});

const formProjectOptions = computed(() => {
    let list = projects.value;
    if (form.customer_id) {
        list = list.filter((p) => p.customer_id === form.customer_id);
    }
    return list.map((p) => ({ label: p.name, value: p.id }));
});

function projectName(id) {
    return projects.value.find((p) => p.id === id)?.name || id;
}

function projectCustomerName(projectId) {
    const p = projects.value.find((x) => x.id === projectId);
    if (!p) return "—";
    return customers.value.find((c) => c.id === p.customer_id)?.name || p.customer_id;
}

function onFilterCustomerChange() {
    filters.project_id = null;
}

function onFormCustomerChange() {
    form.project_id = null;
}

async function loadCatalog() {
    const [c, p] = await Promise.all([
        api.get("/customers", { params: { active: "true" } }),
        api.get("/projects", { params: { active: "true" } }),
    ]);
    customers.value = c.data.data || [];
    projects.value = p.data.data || [];
}

const RE_DATE = /^\d{4}-\d{2}-\d{2}$/;

async function load() {
    if (filters.from && !RE_DATE.test(filters.from.trim())) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "From date must be YYYY-MM-DD",
            life: 3000,
        });
        return;
    }
    if (filters.to && !RE_DATE.test(filters.to.trim())) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "To date must be YYYY-MM-DD",
            life: 3000,
        });
        return;
    }
    loading.value = true;
    try {
        const params = { limit: 500 };
        if (filters.customer_id) params.customer_id = filters.customer_id;
        if (filters.project_id) params.project_id = filters.project_id;
        if (filters.from) params.from = filters.from.trim() + " 00:00:00";
        if (filters.to) params.to = filters.to.trim() + " 23:59:59";
        const { data } = await api.get("/reports", { params });
        items.value = data.data || [];
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    editing.value = null;
    form.customer_id = null;
    form.project_id = null;
    form.start = "";
    form.stop = "";
    form.comment = "";
    dialogVisible.value = true;
}

function openEdit(row) {
    editing.value = row;
    const p = projects.value.find((x) => x.id === row.project_id);
    form.customer_id = p?.customer_id ?? null;
    form.project_id = row.project_id;
    form.start = toLocalInput(row.start);
    form.stop = toLocalInput(row.stop);
    form.comment = row.comment || "";
    dialogVisible.value = true;
}

async function save() {
    if (!form.project_id || !form.start) {
        toast.add({ severity: "warn", summary: "Validation", detail: "Project and start are required", life: 3000 });
        return;
    }
    const start = fromLocalInput(form.start);
    const stop = form.stop ? fromLocalInput(form.stop) : null;
    if (!start || (form.stop && !stop)) {
        toast.add({
            severity: "warn",
            summary: "Validation",
            detail: "Use date/time as YYYY-MM-DD HH:MM (24-hour)",
            life: 3500,
        });
        return;
    }
    saving.value = true;
    try {
        const body = {
            project_id: form.project_id,
            start,
            stop,
            comment: form.comment,
        };
        if (editing.value) {
            await api.put(`/reports/${editing.value.id}`, body);
        } else {
            await api.post("/reports", body);
        }
        dialogVisible.value = false;
        await load();
        toast.add({ severity: "success", summary: "Saved", life: 2000 });
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        saving.value = false;
    }
}

function confirmDelete(row) {
    confirm.require({
        message: "Mark this report as deleted?",
        header: "Confirm",
        icon: "pi pi-exclamation-triangle",
        acceptClass: "p-button-danger",
        accept: async () => {
            try {
                await api.delete(`/reports/${row.id}`);
                await load();
                toast.add({ severity: "success", summary: "Deleted", life: 2000 });
            } catch (err) {
                toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
            }
        },
    });
}

onMounted(async () => {
    await loadCatalog();
    await load();
});
</script>
