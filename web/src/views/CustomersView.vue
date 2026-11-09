<template>
    <div>
        <div class="flex align-items-center justify-content-between mb-3 flex-wrap gap-2">
            <h1 class="page-title m-0">Customers</h1>
            <Button v-if="auth.isAdmin" label="New customer" icon="pi pi-plus" @click="openCreate" />
        </div>

        <div class="card-panel">
            <DataTable
                :value="items"
                :loading="loading"
                paginator
                :rows="15"
                striped-rows
                size="small"
                data-key="id"
                empty-message="No customers found"
            >
                <Column field="id" header="ID" style="width: 5rem" />
                <Column field="name" header="Name" sortable />
                <Column field="description" header="Description" />
                <Column header="Active" style="width: 6rem">
                    <template #body="{ data }">
                        <Tag :severity="data.active ? 'success' : 'secondary'" :value="data.active ? 'Yes' : 'No'" />
                    </template>
                </Column>
                <Column header="" :style="auth.isAdmin ? 'width: 12rem' : 'width: 7rem'">
                    <template #body="{ data }">
                        <Button
                            icon="pi pi-folder"
                            text
                            rounded
                            size="small"
                            v-tooltip.top="'Projects'"
                            @click="openProjects(data)"
                        />
                        <template v-if="auth.isAdmin">
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
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Customer create/edit -->
        <Dialog
            v-model:visible="dialogVisible"
            :header="editing ? 'Edit customer' : 'New customer'"
            modal
            style="width: min(520px, 95vw)"
        >
            <div class="flex flex-column gap-3">
                <div class="flex flex-column gap-1">
                    <label for="cname">Name</label>
                    <InputText id="cname" v-model="form.name" class="w-full" />
                </div>
                <div class="flex flex-column gap-1">
                    <label for="cdesc">Description</label>
                    <Textarea id="cdesc" v-model="form.description" rows="3" class="w-full" auto-resize />
                </div>
                <div class="flex align-items-center gap-2">
                    <Checkbox v-model="form.active" binary input-id="cactive" />
                    <label for="cactive">Active</label>
                </div>
            </div>
            <template #footer>
                <Button label="Cancel" text @click="dialogVisible = false" />
                <Button label="Save" :loading="saving" @click="save" />
            </template>
        </Dialog>

        <!-- Projects for one customer -->
        <Dialog
            v-model:visible="projectsDialogVisible"
            :header="projectsCustomer ? `Projects — ${projectsCustomer.name}` : 'Projects'"
            modal
            style="width: min(720px, 96vw)"
            @hide="onProjectsDialogHide"
        >
            <div class="flex justify-content-end mb-3" v-if="auth.isAdmin">
                <Button label="New project" icon="pi pi-plus" size="small" @click="openProjectCreate" />
            </div>

            <DataTable
                :value="projectItems"
                :loading="projectsLoading"
                size="small"
                striped-rows
                data-key="id"
                empty-message="No projects for this customer"
            >
                <Column field="name" header="Name" sortable />
                <Column field="cost_center" header="Cost center" />
                <Column field="unit" header="Unit" style="width: 8rem" />
                <Column header="Active" style="width: 5rem">
                    <template #body="{ data }">
                        <Tag :severity="data.active ? 'success' : 'secondary'" :value="data.active ? 'Yes' : 'No'" />
                    </template>
                </Column>
                <Column v-if="auth.isAdmin" header="" style="width: 7rem">
                    <template #body="{ data }">
                        <Button icon="pi pi-pencil" text rounded size="small" @click="openProjectEdit(data)" />
                        <Button
                            icon="pi pi-trash"
                            text
                            rounded
                            size="small"
                            severity="danger"
                            @click="confirmProjectDelete(data)"
                        />
                    </template>
                </Column>
            </DataTable>

            <template #footer>
                <Button label="Close" text @click="projectsDialogVisible = false" />
            </template>
        </Dialog>

        <!-- Project create/edit (nested) -->
        <Dialog
            v-model:visible="projectFormVisible"
            :header="projectEditing ? 'Edit project' : 'New project'"
            modal
            style="width: min(480px, 95vw)"
        >
            <div class="flex flex-column gap-3">
                <div class="flex flex-column gap-1">
                    <label for="pname">Name</label>
                    <InputText id="pname" v-model="projectForm.name" class="w-full" />
                </div>
                <div class="flex flex-column gap-1">
                    <label for="pcc">Cost center</label>
                    <InputText id="pcc" v-model="projectForm.cost_center" class="w-full" />
                </div>
                <div class="flex flex-column gap-1">
                    <label for="punit">Unit</label>
                    <InputText
                        id="punit"
                        v-model="projectForm.unit"
                        class="w-full"
                        placeholder="hours, kilometers, …"
                        list="project-unit-suggestions"
                    />
                    <datalist id="project-unit-suggestions">
                        <option value="hours" />
                        <option value="kilometers" />
                        <option value="pieces" />
                        <option value="days" />
                    </datalist>
                    <small class="muted">Shown on timesheet PDF summary (not converted by the server)</small>
                </div>
                <div class="flex align-items-center gap-2">
                    <Checkbox v-model="projectForm.active" binary input-id="pactive" />
                    <label for="pactive">Active</label>
                </div>
            </div>
            <template #footer>
                <Button label="Cancel" text @click="projectFormVisible = false" />
                <Button label="Save" :loading="projectSaving" @click="saveProject" />
            </template>
        </Dialog>
    </div>
</template>

<script setup>
import { onMounted, reactive, ref } from "vue";
import { useToast } from "primevue/usetoast";
import { useConfirm } from "primevue/useconfirm";
import DataTable from "primevue/datatable";
import Column from "primevue/column";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputText from "primevue/inputtext";
import Textarea from "primevue/textarea";
import Checkbox from "primevue/checkbox";
import Tag from "primevue/tag";
import api, { errorMessage } from "@/api/client";
import { useAuthStore } from "@/stores/auth";

const auth = useAuthStore();
const toast = useToast();
const confirm = useConfirm();

const items = ref([]);
const loading = ref(false);
const saving = ref(false);
const dialogVisible = ref(false);
const editing = ref(null);
const form = reactive({ name: "", description: "", active: true });

// Projects modal
const projectsDialogVisible = ref(false);
const projectsCustomer = ref(null);
const projectItems = ref([]);
const projectsLoading = ref(false);
const projectFormVisible = ref(false);
const projectEditing = ref(null);
const projectSaving = ref(false);
const projectForm = reactive({
    name: "",
    cost_center: "",
    unit: "hours",
    active: true,
});

function sortCustomers(list) {
    return [...list].sort((a, b) => {
        // Active first, then name (case-insensitive)
        const activeDiff = Number(!!b.active) - Number(!!a.active);
        if (activeDiff !== 0) return activeDiff;
        return String(a.name || "").localeCompare(String(b.name || ""), undefined, { sensitivity: "base" });
    });
}

async function load() {
    loading.value = true;
    try {
        const { data } = await api.get("/customers");
        items.value = sortCustomers(data.data || []);
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    editing.value = null;
    form.name = "";
    form.description = "";
    form.active = true;
    dialogVisible.value = true;
}

function openEdit(row) {
    editing.value = row;
    form.name = row.name;
    form.description = row.description || "";
    form.active = !!row.active;
    dialogVisible.value = true;
}

async function save() {
    if (!form.name.trim()) {
        toast.add({ severity: "warn", summary: "Validation", detail: "Name is required", life: 3000 });
        return;
    }
    saving.value = true;
    try {
        const body = {
            name: form.name.trim(),
            description: form.description.trim(),
            active: form.active,
        };
        if (editing.value) {
            await api.put(`/customers/${editing.value.id}`, body);
        } else {
            await api.post("/customers", body);
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
        message: `Delete customer "${row.name}"?`,
        header: "Confirm",
        icon: "pi pi-exclamation-triangle",
        acceptClass: "p-button-danger",
        accept: async () => {
            try {
                await api.delete(`/customers/${row.id}`);
                await load();
                toast.add({ severity: "success", summary: "Deleted", life: 2000 });
            } catch (err) {
                toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
            }
        },
    });
}

async function openProjects(row) {
    projectsCustomer.value = row;
    projectsDialogVisible.value = true;
    await loadProjects();
}

function onProjectsDialogHide() {
    projectsCustomer.value = null;
    projectItems.value = [];
    projectFormVisible.value = false;
}

async function loadProjects() {
    if (!projectsCustomer.value) return;
    projectsLoading.value = true;
    try {
        const { data } = await api.get("/projects", {
            params: { customer_id: projectsCustomer.value.id },
        });
        projectItems.value = data.data || [];
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        projectsLoading.value = false;
    }
}

function openProjectCreate() {
    projectEditing.value = null;
    projectForm.name = "";
    projectForm.cost_center = "";
    projectForm.unit = "hours";
    projectForm.active = true;
    projectFormVisible.value = true;
}

function openProjectEdit(row) {
    projectEditing.value = row;
    projectForm.name = row.name;
    projectForm.cost_center = row.cost_center || "";
    projectForm.unit = row.unit || "hours";
    projectForm.active = !!row.active;
    projectFormVisible.value = true;
}

async function saveProject() {
    if (!projectsCustomer.value) return;
    if (!projectForm.name.trim()) {
        toast.add({ severity: "warn", summary: "Validation", detail: "Name is required", life: 3000 });
        return;
    }
    projectSaving.value = true;
    try {
        const body = {
            customer_id: projectsCustomer.value.id,
            name: projectForm.name.trim(),
            cost_center: projectForm.cost_center.trim(),
            unit: (projectForm.unit || "hours").trim() || "hours",
            active: projectForm.active,
        };
        if (projectEditing.value) {
            await api.put(`/projects/${projectEditing.value.id}`, body);
        } else {
            await api.post("/projects", body);
        }
        projectFormVisible.value = false;
        await loadProjects();
        toast.add({ severity: "success", summary: "Saved", life: 2000 });
    } catch (err) {
        toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
    } finally {
        projectSaving.value = false;
    }
}

function confirmProjectDelete(row) {
    confirm.require({
        message: `Delete project "${row.name}"?`,
        header: "Confirm",
        icon: "pi pi-exclamation-triangle",
        acceptClass: "p-button-danger",
        accept: async () => {
            try {
                await api.delete(`/projects/${row.id}`);
                await loadProjects();
                toast.add({ severity: "success", summary: "Deleted", life: 2000 });
            } catch (err) {
                toast.add({ severity: "error", summary: "Error", detail: errorMessage(err), life: 4000 });
            }
        },
    });
}

onMounted(load);
</script>
