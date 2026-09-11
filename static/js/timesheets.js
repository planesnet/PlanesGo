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
    const task = btn.dataset.taskName || '';
    const hours = btn.dataset.hours || '0';
    const desc = btn.dataset.desc || '';

    const idInput = document.getElementById('delete-timesheet-id');
    const dateEl = document.getElementById('delete-info-date') || document.getElementById('del-modal-date');
    const projEl = document.getElementById('delete-info-project') || document.getElementById('del-modal-project');
    const taskEl = document.getElementById('delete-info-task') || document.getElementById('del-modal-task');
    const taskRow = document.getElementById('delete-info-task-row');
    const hoursEl = document.getElementById('delete-info-hours') || document.getElementById('del-modal-hours');
    const descEl = document.getElementById('delete-info-desc') || document.getElementById('del-modal-desc');

    if (idInput) idInput.value = pendingDeleteId;
    if (dateEl) dateEl.textContent = date;
    if (projEl) projEl.textContent = proj;
    if (taskEl) taskEl.textContent = task || '(Sin tarea asignada)';
    if (taskRow) {
        if (task) {
            taskRow.classList.remove('hidden');
        } else {
            taskRow.classList.add('hidden');
        }
    }
    if (hoursEl) hoursEl.textContent = `${hours} h`;
    if (descEl) descEl.textContent = desc || '(Sin descripción)';

    const feedback = document.getElementById('delete-feedback') || document.getElementById('delete-modal-feedback');
    if (feedback) {
        feedback.className = 'hidden';
        feedback.textContent = '';
    }

    const submitBtn = document.getElementById('btn-confirm-delete');
    const spinner = document.getElementById('btn-confirm-delete-spinner') || document.getElementById('btn-delete-spinner');
    if (submitBtn) submitBtn.disabled = false;
    if (spinner) spinner.classList.add('hidden');

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
    const submitBtn = document.getElementById('btn-confirm-delete');
    const spinner = document.getElementById('btn-confirm-delete-spinner') || document.getElementById('btn-delete-spinner');
    if (submitBtn) submitBtn.disabled = false;
    if (spinner) spinner.classList.add('hidden');

    if (!modal || !container) return;

    container.classList.remove('scale-100', 'opacity-100');
    container.classList.add('scale-95', 'opacity-0');
    setTimeout(() => {
        modal.classList.add('hidden');
    }, 150);
}

function executeDeleteTimesheet() {
    if (!pendingDeleteId) return;

    const targetId = pendingDeleteId;
    const submitBtn = document.getElementById('btn-confirm-delete');
    const spinner = document.getElementById('btn-confirm-delete-spinner') || document.getElementById('btn-delete-spinner');
    const feedback = document.getElementById('delete-feedback') || document.getElementById('delete-modal-feedback');

    if (submitBtn) submitBtn.disabled = true;
    if (spinner) spinner.classList.remove('hidden');
    if (feedback) {
        feedback.className = 'hidden';
        feedback.textContent = '';
    }

    // 1. Enviar petición para borrar en Odoo
    fetch('/api/timesheets/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: parseInt(targetId, 10) })
    })
    .then(async res => {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            throw new Error(data.error || `Error HTTP ${res.status}`);
        }
        return data;
    })
    .then(data => {
        // 2. Cerrar el modal de confirmación inmediatamente
        closeDeleteModal();

        // 3. Si el cronómetro en activo correspondía a este parte de horas, limpiarlo
        if (typeof getTimerState === 'function') {
            const currentTimer = getTimerState();
            if (currentTimer && String(currentTimer.timesheetId) === String(targetId)) {
                if (typeof clearTimer === 'function') {
                    clearTimer();
                }
            }
        }

        // 4. Quitar la fila del DOM con transición suave sin recargar la página entera
        const rows = document.querySelectorAll(`.timesheet-row[data-id="${targetId}"]`);
        if (rows.length > 0) {
            rows.forEach(row => {
                row.style.transition = 'all 0.25s ease-out';
                row.style.opacity = '0';
                row.style.transform = 'translateX(20px)';
                setTimeout(() => {
                    row.remove();

                    // Recalcular filtros, métricas reactivas y vistas
                    if (typeof applyTimesheetFilters === 'function') {
                        applyTimesheetFilters();
                    }
                    if (typeof updateWeekControls === 'function') {
                        updateWeekControls();
                    }
                    if (typeof rebuildSidebarProjects === 'function') {
                        const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
                        rebuildSidebarProjects(workerVal);
                    }

                    // Si no quedan partes en la tabla, mostrar el mensaje de tabla vacía
                    const remainingRows = document.querySelectorAll('.timesheet-row');
                    if (remainingRows.length === 0) {
                        const emptyRow = document.getElementById('empty-row');
                        if (emptyRow) emptyRow.classList.remove('hidden');
                    }
                }, 250);
            });
        } else {
            // Si no estaba visible en la tabla actual (ej. eliminado desde otra vista)
            if (typeof applyTimesheetFilters === 'function') {
                applyTimesheetFilters();
            }
        }
    })
    .catch(err => {
        if (submitBtn) submitBtn.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (feedback) {
            feedback.className = 'mt-4 p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
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

function openCreateTimesheetModal(preselectedProjectId, preselectedProjectName, initialDate, isStartTimerMode, preselectedDesc, preselectedTaskId) {
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

    modal.dataset.mode = isStartTimerMode ? 'timer' : 'standard';

    // Reset estado
    entryIdInput.value = '';
    title.innerText = isStartTimerMode ? 'Iniciar Trabajo' : 'Registrar Horas';
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
    descInput.value = preselectedDesc || '';
    updateModalTimeBadge();

    // Proyecto a preseleccionar: argumento explícito o proyecto activo del panel lateral
    let targetProjectId = preselectedProjectId || activeSidebarProjectId;
    const targetProjectName = preselectedProjectName || activeSidebarProjectName;
    if (!targetProjectId && targetProjectName && projectSelect) {
        for (let i = 0; i < projectSelect.options.length; i++) {
            const optText = projectSelect.options[i].text.toLowerCase().trim();
            const searchName = targetProjectName.toLowerCase().trim();
            if (optText === searchName || optText.includes(searchName) || searchName.includes(optText)) {
                targetProjectId = projectSelect.options[i].value;
                break;
            }
        }
    }

    // Buscar si ya existe una imputación para esta fecha de este proyecto en la tabla para precargar tarea / desc si no se pasaron explícitas
    const targetDateStr = dateInput.value;
    let existingRow = null;
    if (targetProjectId) {
        existingRow = document.querySelector(`.timesheet-row[data-project-id="${targetProjectId}"][data-date="${targetDateStr}"]`);
    }
    if (!existingRow && targetProjectName) {
        try {
            existingRow = document.querySelector(`.timesheet-row[data-project-name="${CSS.escape(targetProjectName)}"][data-date="${targetDateStr}"]`);
        } catch (e) {}
    }

    let finalTaskId = preselectedTaskId || null;
    if (existingRow) {
        if (!finalTaskId) {
            finalTaskId = existingRow.dataset.taskId || null;
        }
        if (!descInput.value && existingRow.dataset.desc) {
            descInput.value = existingRow.dataset.desc;
        }
    }

    if (targetProjectId && projectSelect) {
        projectSelect.value = String(targetProjectId);
        loadTasksForProject(targetProjectId, finalTaskId);
    } else {
        if (projectSelect) projectSelect.value = '';
        const taskSelect = document.getElementById('modal-task-select');
        if (taskSelect) {
            taskSelect.innerHTML = '<option value="">-- Sin tarea asignada --</option>';
        }
    }

    // Adaptar horas y botón según el modo
    if (hoursInput) {
        if (isStartTimerMode) {
            hoursInput.removeAttribute('required');
        } else {
            hoursInput.setAttribute('required', 'required');
        }
    }

    const startTimerBtn = document.getElementById('btn-modal-start-timer');
    if (startTimerBtn) {
        if (isStartTimerMode) {
            startTimerBtn.className = 'px-4 py-2 text-xs font-bold text-white bg-emerald-600 hover:bg-emerald-700 active:bg-emerald-800 rounded-xl transition flex items-center space-x-1.5 shadow-xs ring-2 ring-emerald-400 cursor-pointer';
        } else {
            startTimerBtn.className = 'px-3.5 py-2 text-xs font-bold text-emerald-700 bg-emerald-50 hover:bg-emerald-100 border border-emerald-200 rounded-xl transition flex items-center space-x-1.5 cursor-pointer';
        }
    }

    modal.classList.remove('hidden');
    requestAnimationFrame(() => {
        container.classList.remove('scale-95', 'opacity-0');
        container.classList.add('scale-100', 'opacity-100');

        if (isStartTimerMode) {
            // En modo iniciar trabajo, dar foco a la tarea o a la descripción
            const taskSelect = document.getElementById('modal-task-select');
            if (taskSelect) {
                setTimeout(() => taskSelect.focus(), 100);
            } else if (descInput) {
                setTimeout(() => descInput.focus(), 100);
            }
        } else {
            // Foco inteligente: si ya hay proyecto, enfocar tiempo para escribir y pulsar Enter
            if (targetProjectId && hoursInput) {
                setTimeout(() => hoursInput.focus(), 100);
            } else if (projectSelect) {
                setTimeout(() => projectSelect.focus(), 100);
            }
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
    const submitBtn = document.getElementById('btn-submit-timesheet');
    const spinner = document.getElementById('btn-submit-timesheet-spinner');
    const hoursInput = document.getElementById('modal-hours-input');
    const startTimerBtn = document.getElementById('btn-modal-start-timer');

    if (submitBtn) submitBtn.disabled = false;
    if (spinner) spinner.classList.add('hidden');
    if (hoursInput) hoursInput.setAttribute('required', 'required');
    if (modal) delete modal.dataset.mode;
    if (startTimerBtn) {
        startTimerBtn.className = 'px-3.5 py-2 text-xs font-bold text-emerald-700 bg-emerald-50 hover:bg-emerald-100 border border-emerald-200 rounded-xl transition flex items-center space-x-1.5 cursor-pointer';
    }

    if (!modal) return;

    if (container) {
        container.classList.remove('scale-100', 'opacity-100');
        container.classList.add('scale-95', 'opacity-0');
    }
    setTimeout(() => {
        modal.classList.add('hidden');
        updateModalTimeBadge();
    }, 150);
}

// Alias de seguridad para asegurar disponibilidad global entre scripts
window.closeTimesheetModal = closeTimesheetModal;
window.closeCreateTimesheetModal = closeTimesheetModal;
window.openCreateTimesheetModal = openCreateTimesheetModal;

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

/**
 * Inserta de forma optimista una nueva fila de parte de horas en el DOM
 */
function insertOptimisticTimesheetRow(data) {
    const tbody = document.querySelector('#timesheet-table tbody');
    if (!tbody) return null;

    // Eliminar fila vacía ("No hay partes de horas") si existe
    const emptyRow = tbody.querySelector('#empty-row');
    if (emptyRow) {
        emptyRow.remove();
    }

    const todayStr = (typeof formatISODate === 'function') ? formatISODate(new Date()) : new Date().toISOString().split('T')[0];
    const isToday = (data.date === todayStr);
    const workerName = data.employeeName || (typeof getActiveWorkerName === 'function' ? getActiveWorkerName() : (document.body ? document.body.dataset.currentWorker : '') || 'Yo');
    const workerInitial = workerName.charAt(0).toUpperCase() || 'U';
    const hoursFormatted = parseFloat(data.hours || 0).toFixed(2);
    const isRunning = Boolean(data.timerRunning || data.isRunning);

    const tr = document.createElement('tr');
    tr.className = `timesheet-row hover:bg-slate-50/80 transition-colors ${isRunning ? 'bg-emerald-50/70 ring-1 ring-emerald-300' : 'bg-emerald-100/80'}`;
    tr.dataset.id = data.id;
    tr.dataset.date = data.date;
    tr.dataset.timerRunning = isRunning ? 'true' : 'false';
    tr.dataset.employee = workerName;
    tr.dataset.project = data.projectName;
    tr.dataset.projectName = data.projectName;
    tr.dataset.projectId = data.projectId;
    tr.dataset.task = data.taskName || '';
    tr.dataset.taskId = data.taskId || '';
    tr.dataset.taskName = data.taskName || '';
    tr.dataset.desc = data.desc || '';
    tr.dataset.hours = hoursFormatted;
    tr.dataset.invoiced = 'false';

    tr.innerHTML = `
        <td class="py-3 px-4 sm:px-6 whitespace-nowrap">
            <span class="font-medium text-slate-900 font-mono text-xs">${data.date}</span>
        </td>
        <td class="py-3 px-4 whitespace-nowrap">
            <div class="flex items-center space-x-2">
                <div class="w-6 h-6 rounded-full bg-slate-200 text-slate-600 flex items-center justify-center font-bold text-[10px] shrink-0">
                    ${workerInitial}
                </div>
                <span class="font-medium text-slate-800">${workerName}</span>
            </div>
        </td>
        <td class="py-3 px-4 whitespace-nowrap col-project-cell">
            ${data.projectName ? `
            <button type="button"
                    onclick="selectSidebarProject(this.dataset.projectName, this.dataset.projectId)"
                    data-project-name="${data.projectName}"
                    data-project-id="${data.projectId}"
                    class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-sky-50 text-sky-800 border border-sky-100 hover:bg-sky-100 transition cursor-pointer"
                    title="Filtrar por este proyecto">
                ${data.projectName}
            </button>` : `<span class="text-slate-400 text-xs">-</span>`}
        </td>
        <td class="py-3 px-4 whitespace-nowrap">
            ${data.taskName ? `
            <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-slate-100 text-slate-700">
                ${data.taskName}
            </span>` : `<span class="text-slate-400 text-xs">-</span>`}
        </td>
        <td class="py-3 px-4 text-slate-600 max-w-xs truncate" title="${data.desc || ''}">
            ${data.desc ? data.desc : `<span class="italic text-slate-400">Sin descripción</span>`}
        </td>
        <td class="py-3 px-4 sm:px-6 text-right whitespace-nowrap">
            <div class="inline-flex items-center justify-end space-x-1.5">
                <span class="timesheet-hours-badge ${isRunning ? 'inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-emerald-100 text-emerald-900 border border-emerald-300 font-mono shadow-xs' : 'inline-block px-2.5 py-0.5 rounded-lg text-xs font-bold bg-sky-50 text-sky-700 border border-sky-100 font-mono'}">
                    ${isRunning ? `
                    <span class="relative flex h-2 w-2">
                        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                        <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                    </span>
                    <span class="timer-live-clock font-mono font-bold text-emerald-900">00:00:00</span>
                    <span class="text-[10px] text-emerald-700 font-medium">(${hoursFormatted}h)</span>
                    ` : `${hoursFormatted} h`}
                </span>
            </div>
        </td>
        <td class="py-3 px-3 text-right whitespace-nowrap">
            <div class="inline-flex items-center justify-end space-x-1">
                ${isToday ? `
                <button type="button"
                        onclick="toggleTimesheetRowTimer(this)"
                        data-id="${data.id}"
                        data-date="${data.date}"
                        data-project-id="${data.projectId}"
                        data-project-name="${data.projectName}"
                        data-task-id="${data.taskId || ''}"
                        data-task-name="${data.taskName || ''}"
                        data-hours="${hoursFormatted}"
                        data-desc="${data.desc || ''}"
                        class="btn-row-timer-play inline-flex items-center justify-center w-7 h-7 ${isRunning ? 'text-amber-700 bg-amber-100 hover:bg-amber-200 border-amber-300 animate-pulse' : 'text-emerald-600 hover:text-emerald-700 bg-emerald-50 hover:bg-emerald-100 border border-emerald-200/80'} rounded-lg transition cursor-pointer"
                        title="${isRunning ? 'Pausar cronómetro de esta imputación' : 'Activar o reanudar cronómetro en esta imputación'}">
                    <svg class="w-3.5 h-3.5 icon-play ${isRunning ? 'hidden' : ''}" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z" clip-rule="evenodd" />
                    </svg>
                    <svg class="w-3.5 h-3.5 icon-pause ${isRunning ? '' : 'hidden'}" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zM7 8a1 1 0 012 0v4a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v4a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd" />
                    </svg>
                </button>
                <button type="button"
                        onclick="finalizeActiveTimer()"
                        class="btn-row-timer-stop inline-flex items-center justify-center w-7 h-7 text-rose-600 hover:text-rose-700 bg-rose-50 hover:bg-rose-100 border border-rose-200 rounded-lg transition cursor-pointer ${isRunning ? '' : 'hidden'}"
                        title="Detener y consolidar cronómetro en Odoo">
                    <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 00-1 1v4a1 1 0 001 1h4a1 1 0 001-1V8a1 1 0 00-1-1H8z" clip-rule="evenodd" />
                    </svg>
                </button>` : ''}

                <button type="button"
                        onclick="openEditTimesheetModal(this)"
                        data-id="${data.id}"
                        data-date="${data.date}"
                        data-project-id="${data.projectId}"
                        data-project-name="${data.projectName}"
                        data-task-id="${data.taskId || ''}"
                        data-task-name="${data.taskName || ''}"
                        data-name="${data.desc || ''}"
                        data-hours="${hoursFormatted}"
                        class="inline-flex items-center justify-center w-7 h-7 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded-lg transition cursor-pointer"
                        title="Editar este parte de horas">
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                    </svg>
                </button>

                <button type="button"
                        onclick="openDeleteTimesheetModal(this)"
                        data-id="${data.id}"
                        data-date="${data.date}"
                        data-project-name="${data.projectName}"
                        data-task-name="${data.taskName || ''}"
                        data-hours="${hoursFormatted}"
                        data-desc="${data.desc || ''}"
                        class="inline-flex items-center justify-center w-7 h-7 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded-lg transition cursor-pointer"
                        title="Eliminar este parte de horas">
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                </button>
            </div>
        </td>
    `;

    // Si está corriendo el cronómetro, insertar arriba de todo para visibilidad inmediata; si no, insertar ordenado por fecha
    if (isRunning) {
        tbody.insertBefore(tr, tbody.firstChild);
    } else {
        const existingRows = Array.from(tbody.querySelectorAll('.timesheet-row'));
        let inserted = false;
        for (const r of existingRows) {
            if ((r.dataset.date || '') < data.date) {
                tbody.insertBefore(tr, r);
                inserted = true;
                break;
            }
        }
        if (!inserted) {
            tbody.appendChild(tr);
        }
    }

    if (!isRunning) {
        setTimeout(() => {
            tr.classList.remove('bg-emerald-100/80');
        }, 1500);
    }

    return tr;
}

function submitTimesheetForm(event) {
    if (event && event.preventDefault) event.preventDefault();

    const entryId = document.getElementById('modal-entry-id').value;
    const projectSelect = document.getElementById('modal-project-select');
    const projectId = projectSelect ? projectSelect.value : '';
    const projectName = (projectSelect && projectSelect.selectedIndex >= 0) ? projectSelect.options[projectSelect.selectedIndex].text.trim() : '';
    const taskSelect = document.getElementById('modal-task-select');
    const taskId = taskSelect ? taskSelect.value : '';
    const taskName = (taskSelect && taskSelect.selectedIndex > 0) ? taskSelect.options[taskSelect.selectedIndex].text.trim() : '';
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
        if (projectSelect) projectSelect.focus();
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

    if (!desc) {
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes indicar una descripción del trabajo realizado.';
        }
        const descInp = document.getElementById('modal-desc-input');
        if (descInp) {
            descInp.focus();
            descInp.classList.add('ring-2', 'ring-rose-400', 'border-rose-300');
            setTimeout(() => descInp.classList.remove('ring-2', 'ring-rose-400', 'border-rose-300'), 3000);
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

    let tempRow = null;

    // Actualización optimista ultra-rápida si es edición existente
    if (isEdit) {
        const row = document.querySelector(`.timesheet-row[data-id="${entryId}"]`);
        if (row) {
            row.dataset.date = date;
            row.dataset.desc = desc;
            row.dataset.hours = hours.toFixed(2);
            row.dataset.taskId = taskId || '';
            row.dataset.taskName = taskName;

            // Actualizar celda de fecha
            const dateSpan = row.querySelector('td:nth-child(1) span');
            if (dateSpan) dateSpan.textContent = date;

            // Actualizar celda de tarea
            const taskCell = row.querySelector('td:nth-child(4)');
            if (taskCell) {
                if (taskName) {
                    taskCell.innerHTML = `<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-slate-100 text-slate-700">${taskName}</span>`;
                } else {
                    taskCell.innerHTML = `<span class="text-slate-400 text-xs">-</span>`;
                }
            }

            // Actualizar celda de descripción
            const descCell = row.querySelector('td:nth-child(5)');
            if (descCell) {
                descCell.title = desc;
                descCell.innerHTML = desc ? desc : `<span class="italic text-slate-400">Sin descripción</span>`;
            }

            // Actualizar celda de horas
            const hoursBadge = row.querySelector('td:nth-child(6) span.font-mono');
            if (hoursBadge) {
                hoursBadge.textContent = `${hours.toFixed(2)} h`;
            }

            // Actualizar data-* en botones de editar, borrar y play
            const editBtn = row.querySelector('button[onclick*="openEditTimesheetModal"]');
            if (editBtn) {
                editBtn.dataset.date = date;
                editBtn.dataset.taskId = taskId || '';
                editBtn.dataset.taskName = taskName;
                editBtn.dataset.name = desc;
                editBtn.dataset.hours = hours.toFixed(2);
            }
            const delBtn = row.querySelector('button[onclick*="openDeleteTimesheetModal"]');
            if (delBtn) {
                delBtn.dataset.date = date;
                delBtn.dataset.taskName = taskName;
                delBtn.dataset.desc = desc;
                delBtn.dataset.hours = hours.toFixed(2);
            }
            const playBtn = row.querySelector('.btn-row-timer-play');
            if (playBtn) {
                playBtn.dataset.date = date;
                playBtn.dataset.taskId = taskId || '';
                playBtn.dataset.taskName = taskName;
                playBtn.dataset.desc = desc;
                playBtn.dataset.hours = hours.toFixed(2);
            }

            // Destello visual de éxito instantáneo
            row.classList.add('bg-emerald-100/80', 'transition-colors', 'duration-500');
            setTimeout(() => {
                row.classList.remove('bg-emerald-100/80');
            }, 1200);

            // Recalcular métricas reactivas
            if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
            if (typeof updateWeekControls === 'function') updateWeekControls();
            if (typeof rebuildSidebarProjects === 'function') {
                const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
                rebuildSidebarProjects(workerVal);
            }
        }
    } else {
        // En creación nueva: inserción optimista instantánea en el DOM
        const employeeName = (typeof getActiveWorkerName === 'function') ? getActiveWorkerName() : (document.body?.dataset.currentWorker || 'Yo');

        const tempId = 'temp-' + Date.now();
        tempRow = insertOptimisticTimesheetRow({
            id: tempId,
            date: date,
            projectId: projectId,
            projectName: projectName,
            taskId: taskId,
            taskName: taskName,
            hours: hours,
            desc: desc,
            employeeName: employeeName
        });

        // Recalcular métricas y vistas al instante
        if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
        if (typeof updateWeekControls === 'function') updateWeekControls();
        if (typeof rebuildSidebarProjects === 'function') {
            const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
            rebuildSidebarProjects(workerVal);
        }
    }

    // Cerrar modal de inmediato (0 ms de espera para el usuario)
    closeTimesheetModal();

    // Enviar a Odoo en segundo plano
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

        // Si era una nueva inserción, actualizar el ID temporal con el ID real retornado por Odoo
        if (!isEdit && tempRow && data.id) {
            tempRow.dataset.id = data.id;
            tempRow.querySelectorAll('[data-id]').forEach(el => {
                el.dataset.id = data.id;
            });
        }
    })
    .catch(err => {
        console.error('[PlanesGo] Error guardando imputación en Odoo:', err);
        // Si fue una nueva inserción optimista que falló, remover la fila temporal
        if (!isEdit && tempRow) {
            tempRow.remove();
            if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
            if (typeof updateWeekControls === 'function') updateWeekControls();
            if (typeof rebuildSidebarProjects === 'function') {
                const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
                rebuildSidebarProjects(workerVal);
            }
        }
        alert('⚠️ No se pudo guardar en Odoo: ' + err.message);
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

    // Obtener la fecha seleccionada en el diálogo (modal-date-input)
    const dateInput = document.getElementById('modal-date-input');
    let workDate = dateInput ? dateInput.value.trim() : '';
    if (!workDate) {
        const today = new Date();
        const yyyy = today.getFullYear();
        const mm = String(today.getMonth() + 1).padStart(2, '0');
        const dd = String(today.getDate()).padStart(2, '0');
        workDate = `${yyyy}-${mm}-${dd}`;
    }

    if (!projectId) {
        const feedback = document.getElementById('modal-feedback');
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes seleccionar un proyecto para iniciar el trabajo.';
        }
        if (projectSelect) projectSelect.focus();
        return;
    }

    if (!description) {
        const feedback = document.getElementById('modal-feedback');
        if (feedback) {
            feedback.className = 'p-3 rounded-xl text-xs font-semibold bg-rose-50 border border-rose-200 text-rose-700 block';
            feedback.innerText = 'Debes indicar una descripción del trabajo realizado.';
        }
        if (descInput) {
            descInput.focus();
            descInput.classList.add('ring-2', 'ring-rose-400', 'border-rose-300');
            setTimeout(() => descInput.classList.remove('ring-2', 'ring-rose-400', 'border-rose-300'), 3000);
        }
        return;
    }

    // Si venimos de editar o reanudar un parte existente, usamos su id
    const entryId = document.getElementById('modal-entry-id')?.value?.trim();
    let tsId = entryId ? parseInt(entryId, 10) : null;
    let accumulatedMs = 0;

    if (tsId) {
        const existingRow = document.querySelector(`.timesheet-row[data-id="${tsId}"]`);
        if (existingRow) {
            const h = parseFloat(existingRow.dataset.hours) || 0;
            accumulatedMs = Math.round(h * 3600 * 1000);
        }
    }

    // Si el usuario introdujo horas previas en el input de tiempo, considerarlas como base acumulada
    const hoursRaw = document.getElementById('modal-hours-input')?.value?.trim();
    if (hoursRaw) {
        const manualHours = typeof parseTimeToDecimal === 'function' ? parseTimeToDecimal(hoursRaw) : parseFloat(hoursRaw);
        if (!isNaN(manualHours) && manualHours > 0) {
            accumulatedMs = Math.max(accumulatedMs, Math.round(manualHours * 3600 * 1000));
        }
    }

    if (typeof startWorkTimer === 'function') {
        startWorkTimer(projectId ? parseInt(projectId, 10) : 0, projectName, taskId ? parseInt(taskId, 10) : null, taskName, description, tsId, accumulatedMs, workDate, true);
    }
}
window.startTimerFromModal = startTimerFromModal;
window.insertOptimisticTimesheetRow = insertOptimisticTimesheetRow;
