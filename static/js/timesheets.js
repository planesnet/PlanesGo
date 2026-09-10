/**
 * static/js/timesheets.js
 * Modales de creación/edición e imputación de partes de horas,
 * creación inline de tareas y confirmación de borrado con validación de factura.
 */

// ==============================================================
// --- MODAL DE BORRADO DE PARTES DE HORAS ---
// ==============================================================

let pendingDeleteId = null;

function openDeleteTimesheetModal(btn) {
    if (!btn) return;
    pendingDeleteId = btn.dataset.id;
    const date = btn.dataset.date || '';
    const proj = btn.dataset.projectName || 'Sin proyecto';
    const task = btn.dataset.taskName || 'Sin tarea específica';
    const hours = btn.dataset.hours || '0';
    const desc = btn.dataset.desc || '';

    const dateEl = document.getElementById('del-modal-date');
    const projEl = document.getElementById('del-modal-project');
    const taskEl = document.getElementById('del-modal-task');
    const hoursEl = document.getElementById('del-modal-hours');
    const descEl = document.getElementById('del-modal-desc');

    if (dateEl) dateEl.textContent = date;
    if (projEl) projEl.textContent = proj;
    if (taskEl) taskEl.textContent = task;
    if (hoursEl) hoursEl.textContent = `${hours} h`;
    if (descEl) descEl.textContent = desc || '(Sin descripción)';

    const feedback = document.getElementById('delete-modal-feedback');
    if (feedback) feedback.className = 'hidden';

    const modal = document.getElementById('delete-timesheet-modal');
    const container = document.getElementById('delete-modal-container');
    if (modal && container) {
        modal.classList.remove('hidden');
        requestAnimationFrame(() => {
            container.classList.remove('scale-95', 'opacity-0');
            container.classList.add('scale-100', 'opacity-100');
        });
    }
}

function openDeleteTimesheetModalFromData(id, date, projectName, taskName, hours, desc) {
    const fakeBtn = {
        dataset: {
            id: id,
            date: date,
            projectName: projectName,
            taskName: taskName,
            hours: hours,
            desc: desc
        }
    };
    openDeleteTimesheetModal(fakeBtn);
}

function closeDeleteModal() {
    pendingDeleteId = null;
    const modal = document.getElementById('delete-timesheet-modal');
    const container = document.getElementById('delete-modal-container');
    if (!modal || !container) return;

    container.classList.remove('scale-100', 'opacity-100');
    container.classList.add('scale-95', 'opacity-0');
    setTimeout(() => {
        modal.classList.add('hidden');
    }, 150);
}

function executeDeleteTimesheet() {
    if (!pendingDeleteId) return;

    const submitBtn = document.getElementById('btn-confirm-delete');
    const spinner = document.getElementById('btn-delete-spinner');
    const feedback = document.getElementById('delete-modal-feedback');

    if (submitBtn) submitBtn.disabled = true;
    if (spinner) spinner.classList.remove('hidden');
    if (feedback) feedback.className = 'hidden';

    fetch('/api/timesheets/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: parseInt(pendingDeleteId, 10) })
    })
    .then(async res => {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            throw new Error(data.error || `Error HTTP ${res.status}`);
        }
        return data;
    })
    .then(data => {
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-emerald-50 border border-emerald-200 text-emerald-700 block';
            feedback.innerText = '✓ Parte de horas eliminado correctamente de Odoo. Recargando...';
        }
        setTimeout(() => {
            window.location.reload();
        }, 750);
    })
    .catch(err => {
        if (submitBtn) submitBtn.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = '⚠️ ' + err.message;
        }
    });
}

// ==============================================================
// --- MODAL DE CREACIÓN Y EDICIÓN DE PARTES DE HORAS ---
// ==============================================================

function openEditTimesheetModalFromRowData(id, date, projectId, taskId, desc, hours) {
    const fakeBtn = {
        dataset: {
            id: id,
            date: date,
            projectId: projectId,
            taskId: taskId,
            name: desc,
            hours: hours
        }
    };
    openEditTimesheetModal(fakeBtn);
}

function openCreateTimesheetModal(preselectedProjectId, preselectedProjectName, initialDate) {
    const modal = document.getElementById('timesheet-modal');
    const container = document.getElementById('timesheet-modal-container');
    const title = document.getElementById('modal-title');
    const entryIdInput = document.getElementById('modal-entry-id');
    const projectSelect = document.getElementById('modal-project-select');
    const dateInput = document.getElementById('modal-date-input');
    const hoursInput = document.getElementById('modal-hours-input');
    const descInput = document.getElementById('modal-desc-input');
    const submitBtnText = document.getElementById('btn-submit-timesheet-text');
    const feedback = document.getElementById('modal-feedback');

    if (!modal) return;

    // Reset estado
    entryIdInput.value = '';
    title.innerText = 'Registrar Horas';
    submitBtnText.innerText = 'Guardar';
    toggleInlineCreateTask(false);
    if (feedback) {
        feedback.className = 'hidden p-3 rounded-xl text-xs font-medium';
        feedback.innerText = '';
    }

    // Fecha por defecto: initialDate o hoy (YYYY-MM-DD)
    if (initialDate) {
        dateInput.value = initialDate;
    } else {
        const today = new Date();
        const yyyy = today.getFullYear();
        const mm = String(today.getMonth() + 1).padStart(2, '0');
        const dd = String(today.getDate()).padStart(2, '0');
        dateInput.value = `${yyyy}-${mm}-${dd}`;
    }

    hoursInput.value = '';
    descInput.value = '';
    updateModalTimeBadge();

    // Proyecto a preseleccionar: argumento explícito o proyecto activo del panel lateral
    let targetProjectId = preselectedProjectId || activeSidebarProjectId;
    if (!targetProjectId && activeSidebarProjectName && projectSelect) {
        for (let i = 0; i < projectSelect.options.length; i++) {
            if (projectSelect.options[i].text.toLowerCase().includes(activeSidebarProjectName.toLowerCase())) {
                targetProjectId = projectSelect.options[i].value;
                break;
            }
        }
    }

    if (targetProjectId && projectSelect) {
        projectSelect.value = targetProjectId;
        loadTasksForProject(targetProjectId);
    } else {
        if (projectSelect) projectSelect.value = '';
        const taskSelect = document.getElementById('modal-task-select');
        if (taskSelect) {
            taskSelect.innerHTML = '<option value="">-- Sin tarea asignada --</option>';
        }
    }

    modal.classList.remove('hidden');
    requestAnimationFrame(() => {
        container.classList.remove('scale-95', 'opacity-0');
        container.classList.add('scale-100', 'opacity-100');
        // Foco inteligente: si ya hay proyecto, enfocar tiempo para escribir y pulsar Enter
        if (targetProjectId && hoursInput) {
            setTimeout(() => hoursInput.focus(), 100);
        } else if (projectSelect) {
            setTimeout(() => projectSelect.focus(), 100);
        }
    });
}

function openEditTimesheetModal(btn) {
    const modal = document.getElementById('timesheet-modal');
    const container = document.getElementById('timesheet-modal-container');
    const title = document.getElementById('modal-title');
    const entryIdInput = document.getElementById('modal-entry-id');
    const projectSelect = document.getElementById('modal-project-select');
    const dateInput = document.getElementById('modal-date-input');
    const hoursInput = document.getElementById('modal-hours-input');
    const descInput = document.getElementById('modal-desc-input');
    const submitBtnText = document.getElementById('btn-submit-timesheet-text');
    const feedback = document.getElementById('modal-feedback');

    if (!modal || !btn) return;

    const id = btn.dataset.id;
    const date = btn.dataset.date;
    const projectId = btn.dataset.projectId;
    const taskId = btn.dataset.taskId;
    const desc = btn.dataset.name;
    const hours = btn.dataset.hours;

    entryIdInput.value = id || '';
    title.innerText = `Editar Parte de Horas #${id}`;
    submitBtnText.innerText = 'Actualizar';
    toggleInlineCreateTask(false);
    if (feedback) {
        feedback.className = 'hidden p-3 rounded-xl text-xs font-medium';
        feedback.innerText = '';
    }

    dateInput.value = date || '';
    hoursInput.value = hours ? formatDecimalToTime(parseFloat(hours)) : '';
    descInput.value = desc || '';
    updateModalTimeBadge();

    if (projectSelect && projectId) {
        projectSelect.value = projectId;
        loadTasksForProject(projectId, taskId);
    } else {
        const taskSelect = document.getElementById('modal-task-select');
        if (taskSelect) {
            taskSelect.innerHTML = '<option value="">-- Sin tarea asignada --</option>';
        }
    }

    modal.classList.remove('hidden');
    requestAnimationFrame(() => {
        container.classList.remove('scale-95', 'opacity-0');
        container.classList.add('scale-100', 'opacity-100');
        if (hoursInput) {
            setTimeout(() => {
                hoursInput.focus();
                hoursInput.select();
            }, 100);
        }
    });
}

function closeTimesheetModal() {
    const modal = document.getElementById('timesheet-modal');
    const container = document.getElementById('timesheet-modal-container');
    if (!modal) return;

    container.classList.remove('scale-100', 'opacity-100');
    container.classList.add('scale-95', 'opacity-0');
    setTimeout(() => {
        modal.classList.add('hidden');
        updateModalTimeBadge();
    }, 150);
}

function onModalProjectChange(projectId) {
    toggleInlineCreateTask(false);
    loadTasksForProject(projectId);
}

function loadTasksForProject(projectId, preselectedTaskId) {
    const taskSelect = document.getElementById('modal-task-select');
    if (!taskSelect) return;

    if (!projectId) {
        taskSelect.innerHTML = '<option value="">-- Sin tarea asignada --</option>';
        return;
    }

    taskSelect.innerHTML = '<option value="">Cargando tareas actualizadas...</option>';
    taskSelect.disabled = true;

    fetch(`/api/tasks?project_id=${projectId}&_t=${Date.now()}`, {
        cache: 'no-store',
        headers: {
            'Cache-Control': 'no-cache, no-store, must-revalidate',
            'Pragma': 'no-cache'
        }
    })
        .then(res => {
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            return res.json();
        })
        .then(tasks => {
            taskSelect.disabled = false;
            let html = '<option value="">-- Sin tarea específica asignada --</option>';
            if (Array.isArray(tasks) && tasks.length > 0) {
                tasks.forEach(t => {
                    const selected = (preselectedTaskId && String(t.id) === String(preselectedTaskId)) ? 'selected' : '';
                    html += `<option value="${t.id}" ${selected}>${t.name || ('Tarea #' + t.id)}</option>`;
                });
            } else {
                html += '<option value="" disabled>(No hay tareas creadas aún)</option>';
            }
            taskSelect.innerHTML = html;
        })
        .catch(err => {
            console.error('Error cargando tareas:', err);
            taskSelect.disabled = false;
            taskSelect.innerHTML = '<option value="">-- Sin tarea asignada (error al cargar) --</option>';
        });
}

function refreshModalTasks() {
    const projectSelect = document.getElementById('modal-project-select');
    const taskSelect = document.getElementById('modal-task-select');
    const currentTaskId = taskSelect ? taskSelect.value : '';
    const projectId = projectSelect ? projectSelect.value : '';
    if (projectId) {
        const icon = document.getElementById('modal-refresh-tasks-icon');
        if (icon) icon.classList.add('animate-spin');
        loadTasksForProject(projectId, currentTaskId);
        setTimeout(() => {
            if (icon) icon.classList.remove('animate-spin');
        }, 600);
    }
}

function toggleInlineCreateTask(forceState) {
    const box = document.getElementById('inline-task-box');
    const input = document.getElementById('inline-task-name');
    const msg = document.getElementById('inline-task-msg');
    if (!box) return;

    const shouldShow = (typeof forceState === 'boolean') ? forceState : box.classList.contains('hidden');
    if (shouldShow) {
        box.classList.remove('hidden');
        if (input) {
            input.value = '';
            input.focus();
        }
        if (msg) msg.className = 'hidden';
    } else {
        box.classList.add('hidden');
        if (msg) msg.className = 'hidden';
    }
}

function submitInlineCreateTask() {
    const projectSelect = document.getElementById('modal-project-select');
    const projectId = projectSelect ? projectSelect.value : '';
    const nameInput = document.getElementById('inline-task-name');
    const taskName = nameInput ? nameInput.value.trim() : '';
    const msg = document.getElementById('inline-task-msg');
    const submitBtn = document.getElementById('btn-submit-inline-task');
    const submitBtnLabel = document.getElementById('btn-submit-inline-task-label');

    if (!projectId) {
        if (msg) {
            msg.className = 'text-[11px] font-semibold text-rose-600 block';
            msg.innerText = 'Debes seleccionar un proyecto antes de crear una tarea.';
        }
        return;
    }

    if (!taskName) {
        if (msg) {
            msg.className = 'text-[11px] font-semibold text-rose-600 block';
            msg.innerText = 'Por favor, introduce el nombre o título de la tarea.';
        }
        if (nameInput) nameInput.focus();
        return;
    }

    if (submitBtn) submitBtn.disabled = true;
    if (submitBtnLabel) submitBtnLabel.innerText = 'Creando...';
    if (msg) {
        msg.className = 'text-[11px] font-semibold text-sky-700 block';
        msg.innerText = 'Creando tarea en Odoo...';
    }

    fetch('/api/tasks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            project_id: parseInt(projectId, 10),
            name: taskName
        })
    })
    .then(async res => {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            const errorMsg = data.error || (res.status === 403 ? 'No tienes permisos en Odoo para crear tareas en este proyecto.' : `Error HTTP ${res.status}`);
            throw new Error(errorMsg);
        }
        return data;
    })
    .then(data => {
        if (submitBtn) submitBtn.disabled = false;
        if (submitBtnLabel) submitBtnLabel.innerText = 'Añadir';
        if (msg) {
            msg.className = 'text-[11px] font-semibold text-emerald-600 block';
            msg.innerText = '✓ Tarea creada correctamente.';
        }

        // Añadir al selector y seleccionar
        const taskSelect = document.getElementById('modal-task-select');
        if (taskSelect) {
            const opt = document.createElement('option');
            opt.value = data.id;
            opt.text = data.name || taskName;
            opt.selected = true;
            taskSelect.appendChild(opt);
        }

        setTimeout(() => {
            toggleInlineCreateTask(false);
            const hoursInput = document.getElementById('modal-hours-input');
            if (hoursInput) hoursInput.focus();
        }, 800);
    })
    .catch(err => {
        if (submitBtn) submitBtn.disabled = false;
        if (submitBtnLabel) submitBtnLabel.innerText = 'Añadir';
        if (msg) {
            msg.className = 'text-[11px] font-semibold text-rose-600 block';
            msg.innerText = '⚠️ ' + err.message;
        }
    });
}

function submitTimesheetForm(event) {
    if (event && event.preventDefault) event.preventDefault();

    const entryId = document.getElementById('modal-entry-id').value;
    const projectId = document.getElementById('modal-project-select').value;
    const taskSelect = document.getElementById('modal-task-select');
    const taskId = taskSelect ? taskSelect.value : '';
    const date = document.getElementById('modal-date-input').value;
    const hoursRaw = document.getElementById('modal-hours-input').value;
    const hours = parseTimeToDecimal(hoursRaw);
    const desc = document.getElementById('modal-desc-input').value.trim();

    const feedback = document.getElementById('modal-feedback');
    const submitBtn = document.getElementById('btn-submit-timesheet');
    const spinner = document.getElementById('btn-submit-timesheet-spinner');

    if (!projectId && !entryId) {
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes seleccionar un proyecto.';
        }
        const projSelect = document.getElementById('modal-project-select');
        if (projSelect) projSelect.focus();
        return;
    }

    if (!date) {
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes especificar una fecha válida.';
        }
        const dateInp = document.getElementById('modal-date-input');
        if (dateInp) dateInp.focus();
        return;
    }

    if (isNaN(hours) || hours <= 0) {
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Introduce un tiempo válido en formato horas:minutos (ej: 1:30 para 1h y 30m, 0:45 para 45m).';
        }
        const hoursInp = document.getElementById('modal-hours-input');
        if (hoursInp) {
            hoursInp.focus();
            hoursInp.select();
        }
        return;
    }

    // Iniciar loading
    if (submitBtn) submitBtn.disabled = true;
    if (spinner) spinner.classList.remove('hidden');
    if (feedback) feedback.className = 'hidden';

    const isEdit = Boolean(entryId);
    const url = isEdit ? '/api/timesheets/update' : '/api/timesheets';
    const payload = isEdit ? {
        id: parseInt(entryId, 10),
        date: date,
        task_id: taskId ? parseInt(taskId, 10) : 0,
        unit_amount: hours,
        description: desc
    } : {
        date: date,
        project_id: parseInt(projectId, 10),
        task_id: taskId ? parseInt(taskId, 10) : 0,
        unit_amount: hours,
        description: desc
    };

    fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
    })
    .then(async res => {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            throw new Error(data.error || `Error HTTP ${res.status}`);
        }
        return data;
    })
    .then(data => {
        if (typeof clearTimer === 'function') {
            clearTimer();
        }
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-emerald-50 border border-emerald-200 text-emerald-700 block';
            feedback.innerText = isEdit ? '✓ Parte de horas actualizado con éxito. Recargando...' : '✓ Horas imputadas con éxito en Odoo. Recargando...';
        }
        setTimeout(() => {
            window.location.reload();
        }, 800);
    })
    .catch(err => {
        if (submitBtn) submitBtn.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = '⚠️ ' + err.message;
        }
    });
}

/**
 * Inicia el temporizador de trabajo desde los datos seleccionados en el modal de imputación
 */
function startTimerFromModal() {
    const projectSelect = document.getElementById('modal-project-select');
    const taskSelect = document.getElementById('modal-task-select');
    const descInput = document.getElementById('modal-desc-input');

    const projectId = projectSelect ? projectSelect.value : '';
    let projectName = '';
    if (projectSelect && projectSelect.selectedIndex >= 0) {
        projectName = projectSelect.options[projectSelect.selectedIndex].text.trim();
    }

    const taskId = taskSelect ? taskSelect.value : '';
    let taskName = '';
    if (taskSelect && taskSelect.selectedIndex > 0) {
        taskName = taskSelect.options[taskSelect.selectedIndex].text.trim();
    }

    const description = descInput ? descInput.value.trim() : '';

    if (!projectId) {
        const feedback = document.getElementById('modal-feedback');
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes seleccionar un proyecto para iniciar el trabajo.';
        }
        if (projectSelect) projectSelect.focus();
        return;
    }

    if (typeof startWorkTimer === 'function') {
        startWorkTimer(projectId, projectName, taskId, taskName, description);
    }
}
window.startTimerFromModal = startTimerFromModal;
