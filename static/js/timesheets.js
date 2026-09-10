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

    // Actualización optimista ultra-rápida si es edición existente
    if (isEdit) {
        const row = document.querySelector(`.timesheet-row[data-id="${entryId}"]`);
        if (row) {
            row.dataset.date = date;
            row.dataset.desc = desc;
            row.dataset.hours = hours.toFixed(2);
            row.dataset.taskId = taskId || '';
            const tName = (taskSelect && taskSelect.selectedIndex > 0) ? taskSelect.options[taskSelect.selectedIndex].text.trim() : '';
            row.dataset.taskName = tName;

            // Actualizar celda de fecha
            const dateSpan = row.querySelector('td:nth-child(1) span');
            if (dateSpan) dateSpan.textContent = date;

            // Actualizar celda de tarea
            const taskCell = row.querySelector('td:nth-child(4)');
            if (taskCell) {
                if (tName) {
                    taskCell.innerHTML = `<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-slate-100 text-slate-700">${tName}</span>`;
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

            // Actualizar data-* en botones de editar y play
            const editBtn = row.querySelector('button[onclick*="openEditTimesheetModal"]');
            if (editBtn) {
                editBtn.dataset.date = date;
                editBtn.dataset.taskId = taskId || '';
                editBtn.dataset.taskName = tName;
                editBtn.dataset.name = desc;
                editBtn.dataset.hours = hours.toFixed(2);
            }
            const playBtn = row.querySelector('.btn-row-timer-play');
            if (playBtn) {
                playBtn.dataset.date = date;
                playBtn.dataset.taskId = taskId || '';
                playBtn.dataset.taskName = tName;
                playBtn.dataset.desc = desc;
                playBtn.dataset.hours = hours.toFixed(2);
            }

            // Destello visual de éxito instantáneo
            row.classList.add('bg-emerald-100/80', 'transition-colors', 'duration-500');
            setTimeout(() => {
                row.classList.remove('bg-emerald-100/80');
            }, 1200);
        }

        // Cerrar modal de inmediato sin esperar a Odoo
        closeCreateTimesheetModal();
    } else {
        // En creación nueva, cerrar modal y dar feedback inmediato
        closeCreateTimesheetModal();
    }

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
        if (!isEdit) {
            // Si era un nuevo registro, recargar la vista para incluirlo con todos sus datos y relaciones de Odoo
            window.location.reload();
        }
    })
    .catch(err => {
        console.error('[PlanesGo] Error guardando imputación en Odoo:', err);
        alert('Error al guardar en Odoo: ' + err.message);
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
