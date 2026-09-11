/**
 * PlanesGo - Temporizador de Trabajo Activo y Recordatorio de 15 Minutos
 * Gestiona el cronómetro en vivo, persistencia en localStorage, avisos periódicos y atajo Shift+Ctrl+T.
 */

const PLANESGO_TIMER_KEY = 'planesgo_active_timer';
const TIMER_PROMPT_MINUTES = 15;
const TIMER_PROMPT_INTERVAL_MS = TIMER_PROMPT_MINUTES * 60 * 1000;
const TIMER_UNCONFIRMED_TIMEOUT_MINUTES = 5; // 5 minutos sin confirmación para auto-pausar
const TIMER_UNCONFIRMED_TIMEOUT_MS = TIMER_UNCONFIRMED_TIMEOUT_MINUTES * 60 * 1000;

let timerIntervalId = null;
let titleFlashIntervalId = null;
let activeSystemNotification = null;
let originalDocumentTitle = document.title || 'PlanesGo - Proyectos y Horas Odoo';

// Inicialización automática al cargar el DOM
document.addEventListener('DOMContentLoaded', function () {
    originalDocumentTitle = document.title;
    initTimerFromStorage();
    setupGlobalTimerKeyboardShortcut();
    setupConfirmModalKeyboardListener();
});

/**
 * Atajo Intro (Enter) y Escape para el diálogo de confirmación
 */
function setupConfirmModalKeyboardListener() {
    document.addEventListener('keydown', function (e) {
        const modal = document.getElementById('timer-confirm-modal');
        if (modal && !modal.classList.contains('hidden')) {
            if (e.key === 'Enter') {
                e.preventDefault();
                confirmContinueTimer();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                hideTimerConfirmModal();
            }
        }
    });
}

/**
 * Obtiene el estado del temporizador desde localStorage
 */
function getTimerState() {
    try {
        const raw = localStorage.getItem(PLANESGO_TIMER_KEY);
        return raw ? JSON.parse(raw) : null;
    } catch (e) {
        console.error('Error leyendo temporizador de localStorage:', e);
        return null;
    }
}

/**
 * Guarda el estado del temporizador en localStorage
 */
function saveTimerState(state) {
    try {
        if (!state) {
            localStorage.removeItem(PLANESGO_TIMER_KEY);
        } else {
            localStorage.setItem(PLANESGO_TIMER_KEY, JSON.stringify(state));
        }
    } catch (e) {
        console.error('Error guardando temporizador en localStorage:', e);
    }
}

/**
 * Inicia o reanuda un temporizador de trabajo (admite imputación existente y fecha de inicio)
 */
function startWorkTimer(projectId, projectName, taskId, taskName, description, timesheetId, accumulatedMs, workDate, fromModal) {
    // Si no viene confirmado explícitamente desde el modal de imputación y no es reanudar una fila existente con timesheetId,
    // DEBE abrir el diálogo modal para que el usuario pueda revisar o modificar fecha, tarea y notas antes de iniciar
    if (!fromModal && !timesheetId) {
        if (typeof openCreateTimesheetModal === 'function') {
            openCreateTimesheetModal(projectId, projectName, workDate, true, description, taskId);
            return;
        }
    }

    if (!projectId && !timesheetId) {
        alert('Debes seleccionar un proyecto o imputación para iniciar el trabajo.');
        return;
    }

    // Solicitar permiso de notificaciones de forma proactiva al iniciar
    requestNotificationPermission();

    // Determinar la fecha objetivo de trabajo (parámetro, input modal o hoy)
    const targetDate = workDate || (document.getElementById('modal-date-input')?.value?.trim()) || new Date().toISOString().split('T')[0];

    const now = Date.now();
    const initialAccumulated = (typeof accumulatedMs === 'number' && accumulatedMs >= 0) ? accumulatedMs : 0;
    const initialHours = parseFloat((initialAccumulated / 3600000).toFixed(2));

    const isNewTimesheet = !timesheetId;
    const tempId = isNewTimesheet ? ('temp-' + now) : null;

    const state = {
        timesheetId: timesheetId ? parseInt(timesheetId, 10) : tempId,
        projectId: projectId ? parseInt(projectId, 10) : 0,
        projectName: projectName || (projectId ? 'Proyecto #' + projectId : 'Imputación activa'),
        taskId: taskId ? parseInt(taskId, 10) : null,
        taskName: taskName || '',
        description: description || '',
        date: targetDate,
        status: 'running', // 'running' | 'paused'
        startedAt: now - initialAccumulated,
        lastStartTime: now,
        accumulatedMs: initialAccumulated,
        lastPromptAccumulatedMs: initialAccumulated,
        lastPromptTime: now
    };

    // Si es un nuevo trabajo, crear fila optimista inmediatamente en la tabla para feedback visual instantáneo
    let optimisticRow = null;
    const insertFn = window.insertOptimisticTimesheetRow || (typeof insertOptimisticTimesheetRow === 'function' ? insertOptimisticTimesheetRow : null);
    if (isNewTimesheet && insertFn) {
        const employeeName = (typeof getActiveWorkerName === 'function') ? getActiveWorkerName() : (document.body?.dataset.currentWorker || 'Yo');
        optimisticRow = insertFn({
            id: tempId,
            date: state.date,
            projectId: state.projectId,
            projectName: state.projectName,
            taskId: state.taskId,
            taskName: state.taskName,
            desc: state.description,
            hours: initialHours,
            employeeName: employeeName,
            timerRunning: true,
            isRunning: true
        });
        if (optimisticRow) {
            optimisticRow.dataset.timerRunning = 'true';
            optimisticRow.classList.add('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
        }
    }

    saveTimerState(state);
    renderTimerBar(state);
    startTimerTicker();
    updateAllRowTimerButtonStates();

    if (typeof showToast === 'function') {
        showToast(`⏱️ Cronómetro iniciado en "${state.projectName}"`, 'success');
    }

    // Sincronizar inicio con Odoo en segundo plano (action_timer_start / is_timer_running=true) enviando horas acumuladas y fecha
    fetch('/api/timer/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            project_id: state.projectId || 0,
            project_name: state.projectName || '',
            task_id: state.taskId || 0,
            task_name: state.taskName || '',
            timesheet_id: isNewTimesheet ? 0 : (parseInt(timesheetId, 10) || 0),
            description: state.description,
            unit_amount: initialHours,
            date: state.date
        })
    }).then(res => res.json()).then(data => {
        if (data && data.timesheet_id) {
            const current = getTimerState() || state;
            current.timesheetId = data.timesheet_id;
            if (data.date) current.date = data.date;
            // Preservar tiempo acumulado
            if ((!current.accumulatedMs || current.accumulatedMs === 0) && data.accumulated_ms > 0) {
                current.accumulatedMs = data.accumulated_ms;
            }
            saveTimerState(current);

            // Actualizar la fila optimista con el ID real de Odoo
            if (optimisticRow) {
                optimisticRow.dataset.id = data.timesheet_id;
                optimisticRow.querySelectorAll('[data-id]').forEach(el => {
                    el.dataset.id = data.timesheet_id;
                });
            } else {
                ensureTimesheetRowExists(data, current);
            }
            updateAllRowTimerButtonStates();
            if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
            if (typeof updateWeekControls === 'function') updateWeekControls();
            if (typeof rebuildSidebarProjects === 'function') {
                const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
                rebuildSidebarProjects(workerVal);
            }
        } else if (data && data.error) {
            console.error('[PlanesGo Timer] Error de Odoo al iniciar temporizador:', data.error);
            if (optimisticRow) {
                optimisticRow.remove();
                if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
            }
            if (typeof showToast === 'function') {
                showToast(data.error, 'error');
            } else {
                alert('⚠️ Error al iniciar temporizador en Odoo: ' + data.error);
            }
        }
    }).catch(err => {
        console.warn('[PlanesGo Timer] Error sincronizando inicio con Odoo:', err);
        if (optimisticRow) {
            optimisticRow.remove();
            if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
        }
    });

    // Cerrar modal de imputación si estaba abierto
    if (typeof closeTimesheetModal === 'function') {
        closeTimesheetModal();
    } else if (typeof closeCreateTimesheetModal === 'function') {
        closeCreateTimesheetModal();
    }

    // Sincronizar selección de proyecto en el sidebar si no estaba ya seleccionado
    if (typeof selectSidebarProject === 'function' && state.projectId) {
        const currentActiveId = (typeof activeSidebarProjectId !== 'undefined') ? activeSidebarProjectId : '';
        if (String(state.projectId) !== String(currentActiveId)) {
            selectSidebarProject(state.projectName, state.projectId);
        }
    }

    if (typeof applyTimesheetFilters === 'function') applyTimesheetFilters();
    if (typeof updateWeekControls === 'function') updateWeekControls();
    if (typeof updateAllRowTimerButtonStates === 'function') updateAllRowTimerButtonStates();
    if (typeof rebuildSidebarProjects === 'function') {
        const workerVal = document.getElementById('sidebar-employee-select')?.value || '';
        rebuildSidebarProjects(workerVal);
    }

    console.log(`[PlanesGo Timer] Trabajo iniciado en "${state.projectName}" (Fecha: ${state.date}, Timesheet ID: ${state.timesheetId || 'nuevo'}, Acumulado: ${initialAccumulated}ms)`);
}

/**
 * Alterna entre Pausar y Reanudar el temporizador y sincroniza con Odoo
 */
function togglePauseTimer() {
    const state = getTimerState();
    if (!state) return;

    const now = Date.now();

    if (state.status === 'running') {
        // Pausar
        const sessionMs = now - (state.lastStartTime || now);
        state.accumulatedMs = (state.accumulatedMs || 0) + sessionMs;
        state.status = 'paused';
        state.lastStartTime = null;

        const totalHoursDecimal = parseFloat((state.accumulatedMs / 3600000).toFixed(2));

        // Sincronizar pausa con Odoo (action_timer_pause / is_timer_running=false)
        fetch('/api/timer/pause', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                timesheet_id: state.timesheetId || 0,
                task_id: state.taskId || 0,
                unit_amount: totalHoursDecimal
            })
        }).catch(err => console.warn('[PlanesGo Timer] Error pausando en Odoo:', err));

        // Si hay una fila en la tabla para esta imputación, actualizar sus horas y estado
        if (state.timesheetId) {
            const row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
            if (row) {
                row.dataset.hours = totalHoursDecimal.toFixed(2);
                row.dataset.timerRunning = 'false';
                const hoursBadge = row.querySelector('.timesheet-hours-badge');
                if (hoursBadge) {
                    hoursBadge.className = 'timesheet-hours-badge inline-flex items-center space-x-1.5 px-2.5 py-0.5 rounded-lg text-xs font-bold bg-amber-50 text-amber-800 border border-amber-200 font-mono';
                    hoursBadge.innerHTML = `
                        <span class="inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
                        <span class="font-mono font-bold text-amber-900">${formatElapsedMs(state.accumulatedMs)}</span>
                        <span class="text-[10px] text-amber-700 font-medium">(${totalHoursDecimal.toFixed(2)}h - Pausado)</span>
                    `;
                }
            }
        }

        stopTimerTicker();
        console.log('[PlanesGo Timer] Trabajo en pausa. Tiempo acumulado:', formatElapsedMs(state.accumulatedMs));
        if (typeof showToast === 'function') {
            showToast(`⏸️ Cronómetro pausado (${totalHoursDecimal.toFixed(2)}h)`, 'warning');
        }
    } else {
        // Reanudar
        state.status = 'running';
        state.lastStartTime = now;
        state.lastPromptTime = now;

        if (state.timesheetId) {
            const row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
            if (row) {
                row.dataset.timerRunning = 'true';
            }
        }

        // Sincronizar reanudación con Odoo (action_timer_resume / is_timer_running=true)
        fetch('/api/timer/resume', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                timesheet_id: state.timesheetId || 0,
                task_id: state.taskId || 0
            })
        }).catch(err => console.warn('[PlanesGo Timer] Error reanudando en Odoo:', err));

        startTimerTicker();
        console.log('[PlanesGo Timer] Trabajo reanudado');
        if (typeof showToast === 'function') {
            showToast(`▶️ Cronómetro reanudado: "${state.projectName}"`, 'info');
        }
    }

    saveTimerState(state);
    renderTimerBar(state);
    updateAllRowTimerButtonStates();
}

/**
 * Abre el modal de imputación con las horas calculadas del cronómetro
 */
function finalizeActiveTimer() {
    const state = getTimerState();
    if (!state) return;

    let totalMs = state.accumulatedMs || 0;
    if (state.status === 'running' && state.lastStartTime) {
        totalMs += (Date.now() - state.lastStartTime);
    }

    // Convertir a horas decimales (mínimo 0.05 para que no sea 0 si fue muy breve)
    let hoursDecimal = totalMs / 3600000;
    if (hoursDecimal < 0.02) {
        hoursDecimal = 0.05;
    }
    const formattedHours = hoursDecimal.toFixed(2);

    // Detener parpadeo de título si estaba activo
    stopTitleFlash();

    // Abrir modal de imputación con los datos del trabajo
    if (typeof openCreateTimesheetModal === 'function') {
        openCreateTimesheetModal(state.projectId, state.projectName);
        
        // Asignar los campos en el modal una vez abierto
        setTimeout(() => {
            const unitAmountInput = document.getElementById('modal-unit-amount');
            if (unitAmountInput) {
                unitAmountInput.value = formattedHours;
            }

            const descInput = document.getElementById('modal-description');
            if (descInput && state.description) {
                descInput.value = state.description;
            }

            // Seleccionar tarea si existía
            if (state.taskId) {
                const taskSelect = document.getElementById('modal-task-select');
                if (taskSelect) {
                    taskSelect.value = state.taskId;
                }
            }
        }, 150);
    }
}

/**
 * Descarta el temporizador activo tras confirmación
 */
function confirmDiscardTimer() {
    if (!confirm('¿Deseas descartar el cronómetro actual sin imputar las horas a Odoo?')) {
        return;
    }
    clearTimer();
}

/**
 * Limpia y oculta el temporizador completamente
 */
function clearTimer() {
    const state = getTimerState();
    if (state && (state.timesheetId || state.taskId)) {
        let totalMs = state.accumulatedMs || 0;
        if (state.status === 'running' && state.lastStartTime) {
            totalMs += (Date.now() - state.lastStartTime);
        }
        const totalHours = parseFloat((totalMs / 3600000).toFixed(2));
        if (state.timesheetId) {
            const row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
            if (row) {
                row.dataset.timerRunning = 'false';
                row.classList.remove('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
                row.dataset.hours = totalHours.toFixed(2);
                const hoursBadge = row.querySelector('.timesheet-hours-badge');
                if (hoursBadge) {
                    hoursBadge.className = 'timesheet-hours-badge inline-block px-2.5 py-0.5 rounded-lg text-xs font-bold bg-sky-50 text-sky-700 border border-sky-100 font-mono';
                    hoursBadge.textContent = `${totalHours.toFixed(2)} h`;
                }
            }
        }
        fetch('/api/timer/stop', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                timesheet_id: state.timesheetId || 0,
                task_id: state.taskId || 0,
                unit_amount: totalHours,
                description: state.description
            })
        }).catch(err => console.warn('[PlanesGo Timer] Error deteniendo en Odoo:', err));
    }

    saveTimerState(null);
    stopTimerTicker();
    stopTitleFlash();
    hideTimerConfirmModal();

    updateAllRowTimerButtonStates();
}

/**
 * Inicia el loop del ticker que actualiza el reloj y comprueba los 15 minutos
 */
function startTimerTicker() {
    stopTimerTicker();
    updateTimerTick();
    timerIntervalId = setInterval(updateTimerTick, 1000);
}

/**
 * Detiene el loop del ticker
 */
function stopTimerTicker() {
    if (timerIntervalId) {
        clearInterval(timerIntervalId);
        timerIntervalId = null;
    }
}

let lastAlertChimeSec = -1;

/**
 * Actualiza cada segundo el cronómetro en pantalla y evalúa el recordatorio
 */
function updateTimerTick() {
    const state = getTimerState();
    if (!state) {
        stopTimerTicker();
        return;
    }

    const now = Date.now();
    let totalMs = state.accumulatedMs || 0;

    if (state.status === 'running' && state.lastStartTime) {
        totalMs += (now - state.lastStartTime);

        // Si hay una alerta de 15 minutos pendiente de confirmación, evaluar si han pasado 5 minutos
        if (state.promptTriggeredAt) {
            const timeSinceAlert = now - state.promptTriggeredAt;
            const remainingTimeoutMs = Math.max(0, TIMER_UNCONFIRMED_TIMEOUT_MS - timeSinceAlert);

            // Actualizar cuenta regresiva en el modal si está visible
            const countdownEl = document.getElementById('confirm-modal-countdown');
            if (countdownEl) {
                const totalSec = Math.ceil(remainingTimeoutMs / 1000);
                const min = String(Math.floor(totalSec / 60)).padStart(2, '0');
                const sec = String(totalSec % 60).padStart(2, '0');
                countdownEl.textContent = `${min}:${sec}`;
            }

            // Repetir aviso acústico suave cada 20 segundos para que no pase desapercibido
            const secondsSinceAlert = Math.floor(timeSinceAlert / 1000);
            if (secondsSinceAlert > 0 && secondsSinceAlert % 20 === 0 && secondsSinceAlert !== lastAlertChimeSec) {
                lastAlertChimeSec = secondsSinceAlert;
                playChimeSound(true);
            }

            if (timeSinceAlert >= TIMER_UNCONFIRMED_TIMEOUT_MS) {
                // El usuario no confirmó en los próximos 5 minutos.
                // Parar el cronómetro retrocediendo al momento de los 15 minutos exactos:
                autoStopTimerDueToInactivity(state);
                return;
            }
        } else {
            // Comprobar si han transcurrido los minutos configurados de TRABAJO REAL desde el último prompt
            const lastPromptAccum = (typeof state.lastPromptAccumulatedMs === 'number')
                ? state.lastPromptAccumulatedMs
                : ((state.startedAt && state.lastPromptTime) ? (state.accumulatedMs || 0) : 0);

            const workDoneSincePrompt = totalMs - lastPromptAccum;
            const wallClockSincePrompt = now - (state.lastPromptTime || state.startedAt || now);

            if (workDoneSincePrompt >= TIMER_PROMPT_INTERVAL_MS || wallClockSincePrompt >= TIMER_PROMPT_INTERVAL_MS) {
                trigger15MinuteReminder(state, totalMs);
            }
        }
    }

    // Si el modal de confirmación de 15 minutos está visible en pantalla, mantener su contador activo en tiempo real
    const modalTimeEl = document.getElementById('confirm-modal-time');
    if (modalTimeEl) {
        modalTimeEl.textContent = formatElapsedMs(totalMs);
    }

    // Actualizar directamente la fila activa de la tarea / imputación
    const hoursDecimal = (totalMs / 3600000).toFixed(2);
    const formattedClock = formatElapsedMs(totalMs);

    let row = null;
    if (state.timesheetId) {
        row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
    }
    if (!row) {
        row = document.querySelector(`.timesheet-row[data-timer-running="true"]`);
    }

    if (row) {
        row.dataset.hours = hoursDecimal;
        row.dataset.timerRunning = (state.status === 'running') ? 'true' : 'false';

        const hoursBadge = row.querySelector('.timesheet-hours-badge');
        if (hoursBadge) {
            if (state.status === 'running') {
                hoursBadge.className = 'timesheet-hours-badge inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-emerald-100 text-emerald-900 border border-emerald-300 font-mono shadow-xs';
                hoursBadge.innerHTML = `
                    <span class="relative flex h-2 w-2">
                        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                        <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                    </span>
                    <span class="timer-live-clock font-mono font-bold text-emerald-900">${formattedClock}</span>
                    <span class="text-[10px] text-emerald-700 font-medium">(${hoursDecimal}h)</span>
                `;
            } else {
                hoursBadge.className = 'timesheet-hours-badge inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-amber-50 text-amber-800 border border-amber-200 font-mono';
                hoursBadge.innerHTML = `
                    <span class="inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
                    <span class="font-mono font-bold text-amber-900">${formattedClock}</span>
                    <span class="text-[10px] text-amber-700 font-medium">(${hoursDecimal}h - Pausado)</span>
                `;
            }
        }
    }

    // Sincronización periódica liviana a Odoo cada 30 segundos mientras corre
    if (state.status === 'running' && state.timesheetId && Math.floor(totalMs / 1000) % 30 === 0) {
        const currentHours = parseFloat((totalMs / 3600000).toFixed(2));
        fetch('/api/timer/tick', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                timesheet_id: state.timesheetId,
                unit_amount: currentHours
            })
        }).catch(() => {});
    }
}

/**
 * Auto-pausa el cronómetro si el usuario no confirmó tras el tiempo límite de la alerta,
 * fijando el tiempo registrado exactamente en los minutos en que sonó la alerta.
 */
function autoStopTimerDueToInactivity(state) {
    if (!state || state.status !== 'running') return;

    console.warn(`[PlanesGo Timer] ${TIMER_UNCONFIRMED_TIMEOUT_MINUTES} minutos sin confirmar alerta de ${TIMER_PROMPT_MINUTES} minutos. Auto-pausando y fijando en ${TIMER_PROMPT_MINUTES} minutos.`);

    // 1. Fijar tiempo acumulado exactamente en el snapshot de los minutos de la alerta
    const snapshotMs = (typeof state.promptSnapshotMs === 'number') ? state.promptSnapshotMs : (state.accumulatedMs || 0);
    state.accumulatedMs = snapshotMs;
    state.status = 'paused';
    state.lastStartTime = null;
    state.promptTriggeredAt = null;
    state.promptSnapshotMs = null;
    saveTimerState(state);

    // 2. Detener loop y alertas visuales
    stopTimerTicker();
    stopTitleFlash();
    hideTimerConfirmModal();

    if (activeSystemNotification) {
        try { activeSystemNotification.close(); } catch (e) {}
        activeSystemNotification = null;
    }

    // 3. Actualizar fila visual
    const hoursDecimal = (snapshotMs / 3600000).toFixed(2);
    let row = null;
    if (state.timesheetId) {
        row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
    }
    if (!row) {
        row = document.querySelector(`.timesheet-row[data-timer-running="true"]`);
    }
    if (row) {
        row.dataset.hours = hoursDecimal;
        row.dataset.timerRunning = 'false';
        const hoursBadge = row.querySelector('.timesheet-hours-badge');
        if (hoursBadge) {
            hoursBadge.className = 'timesheet-hours-badge inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-amber-50 text-amber-800 border border-amber-200 font-mono';
            hoursBadge.innerHTML = `
                <span class="inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
                <span class="font-mono font-bold text-amber-900">${formatElapsedMs(snapshotMs)}</span>
                <span class="text-[10px] text-amber-700 font-medium">(${hoursDecimal}h - Pausado a los ${TIMER_PROMPT_MINUTES}m)</span>
            `;
        }
    }
    updateAllRowTimerButtonStates();

    // 4. Notificar a Odoo para pausar y registrar las unidades ajustadas
    const hoursFloat = parseFloat(hoursDecimal);
    fetch('/api/timer/pause', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            timesheet_id: state.timesheetId || 0,
            accumulated_ms: snapshotMs,
            unit_amount: hoursFloat
        })
    }).catch(err => console.warn('[PlanesGo Timer] Error sincronizando auto-pausa con Odoo:', err));

    // 5. Notificación estándar del sistema informando que se detuvo
    triggerSystemNotification(
        'PlanesGo: Cronómetro parado por inactividad',
        `No se confirmó en los últimos ${TIMER_UNCONFIRMED_TIMEOUT_MINUTES} minutos. El cronómetro se ha pausado fijado en los ${TIMER_PROMPT_MINUTES} minutos (${hoursDecimal}h).`
    );

    // 6. Mensaje emergente en pantalla
    showNotificationToast(`Cronómetro pausado por inactividad a los ${TIMER_PROMPT_MINUTES} minutos (${hoursDecimal}h)`);
}

/**
 * Dispara la alerta periódica (sonido, notificación estándar del sistema y modal)
 */
function trigger15MinuteReminder(state, currentTotalMs) {
    // Fijar el snapshot exacto de la alerta y la marca de activación
    state.promptTriggeredAt = Date.now();
    state.promptSnapshotMs = currentTotalMs;
    saveTimerState(state);

    // 1. Reproducir sonido suave de aviso (Web Audio API)
    playChimeSound();

    // 2. Disparar notificación estándar del sistema operativo con clic para abrir diálogo
    triggerSystemNotification(
        `⏱️ PlanesGo: ¿Sigues en "${state.projectName}"?`,
        `Han transcurrido ${TIMER_PROMPT_MINUTES} minutos de trabajo. Haz clic aquí para confirmar que sigues con este trabajo (se detendrá si no se confirma en ${TIMER_UNCONFIRMED_TIMEOUT_MINUTES} min).`
    );

    // 3. Parpadeo del título de la pestaña
    startTitleFlash();

    // 4. Mostrar modal interactivo en pantalla
    showTimerConfirmModal(state, currentTotalMs);
}

/**
 * Muestra el modal de confirmación con el botón Continuar enfocado por defecto
 */
function showTimerConfirmModal(state, totalMs) {
    if (!state) state = getTimerState();
    if (!state) return;

    if (!totalMs) {
        totalMs = state.accumulatedMs || 0;
        if (state.status === 'running' && state.lastStartTime) {
            totalMs += (Date.now() - state.lastStartTime);
        }
    }

    const modal = document.getElementById('timer-confirm-modal');
    if (!modal) return;

    const projEl = document.getElementById('confirm-modal-project');
    if (projEl) projEl.textContent = state.projectName;

    const taskEl = document.getElementById('confirm-modal-task');
    if (taskEl) taskEl.textContent = state.taskName || 'Sin tarea específica';

    const timeEl = document.getElementById('confirm-modal-time');
    if (timeEl) timeEl.textContent = formatElapsedMs(totalMs);

    const descEl = document.getElementById('confirm-modal-desc');
    if (descEl) {
        descEl.textContent = `Han pasado ${TIMER_PROMPT_MINUTES} minutos de trabajo. Si no confirmas en ${TIMER_UNCONFIRMED_TIMEOUT_MINUTES} minutos, el cronómetro se detendrá fijado en los ${TIMER_PROMPT_MINUTES} minutos.`;
    }

    modal.classList.remove('hidden');

    // Botón Continuar como botón por defecto enfocado
    const continueBtn = document.getElementById('confirm-modal-continue-btn');
    if (continueBtn) {
        setTimeout(() => {
            continueBtn.focus();
        }, 80);
    }
}

/**
 * Cierra el modal de confirmación
 */
function hideTimerConfirmModal() {
    const modal = document.getElementById('timer-confirm-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

/**
 * Acción del usuario en el modal: "Sí, continuar"
 */
function confirmContinueTimer() {
    const state = getTimerState();
    let totalMs = 0;
    if (state) {
        totalMs = state.accumulatedMs || 0;
        if (state.status === 'running' && state.lastStartTime) {
            totalMs += (Date.now() - state.lastStartTime);
        }
        state.lastPromptAccumulatedMs = totalMs;
        state.lastPromptTime = Date.now();
        state.promptTriggeredAt = null;
        state.promptSnapshotMs = null;
        saveTimerState(state);
    }
    stopTitleFlash();
    hideTimerConfirmModal();
    if (activeSystemNotification) {
        try { activeSystemNotification.close(); } catch (e) {}
        activeSystemNotification = null;
    }
    if (typeof showToast === 'function') {
        showToast('✅ Trabajo reconfirmado: Sigues cronometrando este proyecto', 'success');
    } else {
        showNotificationToast('Trabajo reconfirmado: Sigues cronometrando este proyecto');
    }
}

/**
 * Acción del usuario en el modal: "Pausar trabajo"
 */
function confirmPauseTimer() {
    const state = getTimerState();
    if (state) {
        state.promptTriggeredAt = null;
        state.promptSnapshotMs = null;
        saveTimerState(state);
        if (state.status === 'running') {
            togglePauseTimer();
        }
    }
    stopTitleFlash();
    hideTimerConfirmModal();
    if (activeSystemNotification) {
        try { activeSystemNotification.close(); } catch (e) {}
        activeSystemNotification = null;
    }
}

/**
 * Acción del usuario en el modal: "Finalizar y guardar"
 */
function confirmFinalizeTimerFromModal() {
    const state = getTimerState();
    if (state) {
        state.promptTriggeredAt = null;
        state.promptSnapshotMs = null;
        saveTimerState(state);
    }
    hideTimerConfirmModal();
    if (activeSystemNotification) {
        try { activeSystemNotification.close(); } catch (e) {}
        activeSystemNotification = null;
    }
    finalizeActiveTimer();
}

/**
 * Actualiza el estado visual de los cronómetros en las tareas/filas activas
 */
function renderTimerBar(state) {
    // El cronómetro en cabecera fue retirado a petición del usuario.
    // Mantenemos sincronizado el estado visual de los botones y tiempos en las tareas activas.
    updateAllRowTimerButtonStates();
    if (state && typeof updateTimerTick === 'function') {
        updateTimerTick();
    }
}

/**
 * Inicializa el temporizador si ya existía al cargar la página y sincroniza con Odoo
 */
async function initTimerFromStorage() {
    // 1. Detección inmediata SSR (Server Side Rendering) si el servidor ya detectó cronómetro activo
    const container = document.getElementById('active-timer-container');
    if (container && container.dataset.serverTimer === 'true') {
        const tsId = parseInt(container.dataset.timesheetId, 10);
        const pId = parseInt(container.dataset.projectId, 10);
        const pName = container.dataset.projectName || '';
        const taskId = parseInt(container.dataset.taskId, 10) || null;
        const taskName = container.dataset.taskName || '';
        const desc = container.dataset.desc || '';
        let startedAt = parseInt(container.dataset.startedAt, 10) || Date.now();
        if (startedAt > 0 && startedAt < 1000000000000) {
            startedAt *= 1000;
        }
        const accumMs = parseInt(container.dataset.accumulatedMs, 10) || 0;
        const isRunning = container.dataset.isRunning === 'true';

        const existingState = getTimerState();
        const serverState = {
            timesheetId: tsId,
            projectId: pId,
            projectName: pName,
            taskId: taskId,
            taskName: taskName,
            description: desc,
            status: isRunning ? 'running' : 'paused',
            startedAt: startedAt,
            lastStartTime: isRunning ? Date.now() : null,
            accumulatedMs: accumMs,
            lastPromptAccumulatedMs: (existingState && existingState.timesheetId === tsId && typeof existingState.lastPromptAccumulatedMs === 'number')
                ? existingState.lastPromptAccumulatedMs
                : accumMs,
            lastPromptTime: (existingState && existingState.timesheetId === tsId && existingState.lastPromptTime)
                ? existingState.lastPromptTime
                : Date.now(),
            promptTriggeredAt: (existingState && existingState.timesheetId === tsId)
                ? existingState.promptTriggeredAt
                : null,
            promptSnapshotMs: (existingState && existingState.timesheetId === tsId)
                ? existingState.promptSnapshotMs
                : null
        };
        saveTimerState(serverState);
        renderTimerBar(serverState);
        if (isRunning) {
            startTimerTicker();
        }
        updateAllRowTimerButtonStates();

        if (serverState.promptTriggeredAt) {
            showTimerConfirmModal();
        }
    } else {
        const local = getTimerState();
        if (local) {
            renderTimerBar(local);
            startTimerTicker();
            updateAllRowTimerButtonStates();
        }
        // Consultar a Odoo de inmediato para verificar si está activo o pausado
        await syncActiveTimerFromOdoo();
    }

    // Sincronización periódica con Odoo (sin reactivar al foco)
    setInterval(syncActiveTimerFromOdoo, TIMER_PROMPT_INTERVAL_MS);
    requestNotificationPermission();
}

/**
 * Consulta a Odoo (/api/timer/active) para mantener el estado como espejo fiel
 */
async function syncActiveTimerFromOdoo() {
    try {
        const resp = await fetch('/api/timer/active', { cache: 'no-store' });
        if (resp.ok) {
            const data = await resp.json();
            if (data && data.active && data.active.is_running) {
                const act = data.active;
                const accumulatedMs = Math.round((act.unit_amount || 0) * 3600 * 1000);
                const current = getTimerState();

                // Si ya está corriendo para la misma imputación, mantener la sincronización sin brincos
                if (current && current.timesheetId === act.timesheet_id && current.status === 'running') {
                    current.unitAmount = act.unit_amount;
                    saveTimerState(current);
                    return;
                }

                let startedAt = act.started_at || (Date.now() - accumulatedMs);
                if (startedAt > 0 && startedAt < 1000000000000) {
                    startedAt *= 1000;
                }

                const serverState = {
                    timesheetId: act.timesheet_id,
                    projectId: act.project_id,
                    projectName: act.project_name || ('Proyecto #' + act.project_id),
                    taskId: act.task_id || null,
                    taskName: act.task_name || '',
                    description: act.description || '',
                    status: 'running',
                    startedAt: startedAt,
                    lastStartTime: Date.now(),
                    accumulatedMs: accumulatedMs,
                    lastPromptAccumulatedMs: (current && current.timesheetId === act.timesheet_id && typeof current.lastPromptAccumulatedMs === 'number')
                        ? current.lastPromptAccumulatedMs
                        : accumulatedMs,
                    lastPromptTime: (current && current.timesheetId === act.timesheet_id && current.lastPromptTime)
                        ? current.lastPromptTime
                        : Date.now(),
                    promptTriggeredAt: (current && current.timesheetId === act.timesheet_id)
                        ? current.promptTriggeredAt
                        : null,
                    promptSnapshotMs: (current && current.timesheetId === act.timesheet_id)
                        ? current.promptSnapshotMs
                        : null
                };
                saveTimerState(serverState);
                renderTimerBar(serverState);
                startTimerTicker();
                ensureTimesheetRowExists(act, serverState);
                updateAllRowTimerButtonStates();

                if (serverState.promptTriggeredAt) {
                    showTimerConfirmModal();
                }
            } else {
                // En Odoo no se reporta cronómetro activo en este instante.
                // IMPORTANTE: NO borrar el cronómetro local si el usuario lo inició en PlanesGo.
                // PlanesGo mantiene la persistencia local y sincroniza hacia Odoo, evitando apagados inesperados.
                const current = getTimerState();
                let started = current ? current.startedAt : 0;
                if (started > 0 && started < 1000000000000) {
                    started *= 1000;
                }
                if (current && started && (Date.now() - started > 86400000)) {
                    // Solo si lleva más de 24 horas continuo lo consideramos obsoleto
                    saveTimerState(null);
                    stopTimerTicker();
                    stopTitleFlash();
                    const container = document.getElementById('active-timer-container');
                    if (container) container.classList.add('hidden');
                    updateAllRowTimerButtonStates();
                }
            }
        }
    } catch (e) {
        console.warn('[PlanesGo Timer] Error sincronizando temporizador con Odoo:', e);
    }
}

let sharedAudioContext = null;

function getAudioContext() {
    try {
        const AudioCtx = window.AudioContext || window.webkitAudioContext;
        if (!AudioCtx) return null;
        if (!sharedAudioContext) {
            sharedAudioContext = new AudioCtx();
        }
        if (sharedAudioContext.state === 'suspended') {
            sharedAudioContext.resume().catch(() => {});
        }
        return sharedAudioContext;
    } catch (e) {
        console.warn('[PlanesGo Audio] Error inicializando AudioContext:', e);
        return null;
    }
}

// Desbloquear AudioContext en la primera interacción del usuario con la página
['click', 'keydown', 'touchstart'].forEach(evt => {
    window.addEventListener(evt, () => {
        if (sharedAudioContext && sharedAudioContext.state === 'suspended') {
            sharedAudioContext.resume().catch(() => {});
        }
    }, { once: false, passive: true });
});

/**
 * Sonido de aviso usando Web Audio API (alta audibilidad armónica sin dependencias externas)
 * @param {boolean} isGentleReminder Si es true, reproduce un bip suave de recordatorio en lugar del acorde completo
 */
function playChimeSound(isGentleReminder = false) {
    try {
        const ctx = getAudioContext();
        if (!ctx) return;

        const playTone = (freq, delay, duration, volume = 0.6) => {
            const osc = ctx.createOscillator();
            const gain = ctx.createGain();

            osc.type = 'triangle'; // Más nítido y audible en altavoces que sine pura
            osc.frequency.setValueAtTime(freq, ctx.currentTime + delay);

            gain.gain.setValueAtTime(0.001, ctx.currentTime + delay);
            gain.gain.exponentialRampToValueAtTime(volume, ctx.currentTime + delay + 0.04);
            gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + delay + duration);

            osc.connect(gain);
            gain.connect(ctx.destination);

            osc.start(ctx.currentTime + delay);
            osc.stop(ctx.currentTime + delay + duration);
        };

        if (isGentleReminder) {
            // Tono recordatorio corto y sutil: E5 (659Hz) -> A5 (880Hz)
            playTone(659.25, 0, 0.35, 0.4);
            playTone(880.00, 0.12, 0.45, 0.5);
        } else {
            // Acorde de atención armonioso y de amplio rango: D5, F#5, A5, D6
            playTone(587.33, 0.00, 0.60, 0.65);
            playTone(739.99, 0.12, 0.70, 0.65);
            playTone(880.00, 0.24, 0.85, 0.70);
            playTone(1174.66, 0.38, 1.10, 0.75);
        }
    } catch (e) {
        console.warn('[PlanesGo Audio] AudioContext bloqueado o no disponible:', e);
    }
}

/**
 * Solicita permiso de notificaciones nativas de escritorio
 */
function requestNotificationPermission() {
    if ('Notification' in window && Notification.permission === 'default') {
        Notification.requestPermission().catch(e => console.log('Permiso de notificaciones denegado o cerrado:', e));
    }
}

/**
 * Muestra notificación de escritorio estándar del sistema operativo
 * Permite hacer clic directamente en la notificación para reconfirmar el trabajo en curso
 */
function triggerSystemNotification(title, body) {
    if (!('Notification' in window)) return;

    const displayNotif = () => {
        try {
            if (activeSystemNotification) {
                try { activeSystemNotification.close(); } catch (e) {}
                activeSystemNotification = null;
            }

            const notif = new Notification(title, {
                body: body,
                icon: '/static/img/logo.png',
                tag: 'planesgo-timer-alert',
                renotify: true,
                requireInteraction: true // Notificación persistente en el sistema operativo
            });

            activeSystemNotification = notif;

            notif.onclick = function () {
                try {
                    window.focus();
                } catch (e) {}

                // Al hacer clic en la notificación, abrir el diálogo del parte de horas con el botón Continuar por defecto
                showTimerConfirmModal();
                try { notif.close(); } catch (e) {}
                activeSystemNotification = null;
            };

            notif.onclose = function () {
                if (activeSystemNotification === notif) {
                    activeSystemNotification = null;
                }
            };
        } catch (e) {
            console.warn('Error al mostrar notificación de escritorio:', e);
        }
    };

    if (Notification.permission === 'granted') {
        displayNotif();
    } else if (Notification.permission !== 'denied') {
        Notification.requestPermission().then(permission => {
            if (permission === 'granted') {
                displayNotif();
            }
        });
    }
}

/**
 * Diagnóstico interactivo para comprobar sonidos y notificaciones nativas del cronómetro
 */
window.testTimerNotification = async function () {
    console.log('[PlanesGo Test] Probando sonido y notificaciones...');

    // 1. Probar sonido armónico inmediatamente
    playChimeSound(false);

    // 2. Verificar o solicitar permiso de notificación
    if (!('Notification' in window)) {
        if (typeof showToast === 'function') {
            showToast('⚠️ Tu navegador no soporta notificaciones de escritorio nativas.', 'warning', 5000);
        }
        return;
    }

    if (Notification.permission === 'denied') {
        if (typeof showToast === 'function') {
            showToast('🚫 Las notificaciones están bloqueadas en los ajustes del navegador.', 'error', 6000);
        }
        return;
    }

    if (Notification.permission === 'default') {
        const perm = await Notification.requestPermission();
        if (perm !== 'granted') {
            if (typeof showToast === 'function') {
                showToast('ℹ️ Permiso de notificaciones no concedido.', 'warning', 4000);
            }
            return;
        }
    }

    // 3. Emitir notificación de prueba
    triggerSystemNotification('🔔 PlanesGo - Prueba de Notificación', '¡El sistema de avisos sonoros y de escritorio está activo y funcionando correctamente!');

    if (typeof showToast === 'function') {
        showToast('🔔 Aviso emitido: sonido reproducido y notificación de escritorio enviada.', 'success', 5000);
    }
};

/**
 * Muestra un aviso emergente visual (toast) no intrusivo en la interfaz
 */
function showNotificationToast(message) {
    let toast = document.getElementById('planesgo-toast');
    if (!toast) {
        toast = document.createElement('div');
        toast.id = 'planesgo-toast';
        toast.className = 'fixed bottom-6 right-6 z-50 transform transition-all duration-300 ease-out translate-y-10 opacity-0 pointer-events-none';
        toast.innerHTML = `
            <div class="flex items-center space-x-3 px-4 py-3 rounded-2xl bg-slate-900 text-white shadow-2xl border border-slate-700/80">
                <span class="flex h-2.5 w-2.5 relative">
                    <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                    <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
                </span>
                <span id="planesgo-toast-message" class="text-xs font-semibold tracking-wide"></span>
            </div>
        `;
        document.body.appendChild(toast);
    }
    const msgEl = toast.querySelector('#planesgo-toast-message');
    if (msgEl) msgEl.textContent = message;

    toast.classList.remove('translate-y-10', 'opacity-0', 'pointer-events-none');
    toast.classList.add('translate-y-0', 'opacity-100');

    setTimeout(() => {
        toast.classList.add('translate-y-10', 'opacity-0', 'pointer-events-none');
        toast.classList.remove('translate-y-0', 'opacity-100');
    }, 4000);
}

/**
 * Alterna el título de la pestaña para llamar la atención visualmente
 */
function startTitleFlash() {
    stopTitleFlash();
    let isAlert = true;
    titleFlashIntervalId = setInterval(() => {
        document.title = isAlert ? '🔔 (Confirmar) PlanesGo' : originalDocumentTitle;
        isAlert = !isAlert;
    }, 1000);
}

/**
 * Restaura el título normal de la pestaña
 */
function stopTitleFlash() {
    if (titleFlashIntervalId) {
        clearInterval(titleFlashIntervalId);
        titleFlashIntervalId = null;
    }
    document.title = originalDocumentTitle;
}

/**
 * Configura el atajo de teclado Shift + Ctrl + T (o Shift + Cmd + T)
 */
function setupGlobalTimerKeyboardShortcut() {
    window.addEventListener('keydown', function (e) {
        const isCtrlOrCmd = e.ctrlKey || e.metaKey;
        const isShift = e.shiftKey;
        const isKeyT = e.key === 't' || e.key === 'T' || e.code === 'KeyT';

        if (isCtrlOrCmd && isShift && isKeyT) {
            e.preventDefault();
            console.log('[PlanesGo] Atajo Shift+Ctrl+T activado');

            const state = getTimerState();
            if (state) {
                // Si ya hay un trabajo activo, abrir el modal de confirmación / control
                showTimerConfirmModal(state);
            } else {
                // Si no hay trabajo activo, abrir modal para iniciar uno
                if (typeof openCreateTimesheetModal === 'function') {
                    openCreateTimesheetModal();
                }
            }
        }
    });
}

/**
 * Formatea milisegundos en HH:MM:SS
 */
function formatElapsedMs(ms) {
    const totalSeconds = Math.floor(ms / 1000);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
}

/**
 * Formatea milisegundos restantes para cuenta regresiva MM:SS
 */
function formatCountdown(ms) {
    const totalSeconds = Math.ceil(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(minutes)}:${pad(seconds)}`;
}

/**
 * Utilidad de prueba para desarrolladores o test manual inmediato:
 * Ejecutable desde la consola con: testTimerReminder()
 */
window.testTimerReminder = function () {
    let state = getTimerState();
    if (!state) {
        state = {
            projectId: 999,
            projectName: 'PROYECTO DE PRUEBA',
            taskId: null,
            taskName: 'Tarea de Test',
            description: 'Prueba de alerta inmediata',
            status: 'running',
            startedAt: Date.now() - 900000,
            lastStartTime: Date.now() - 900000,
            accumulatedMs: 900000,
            lastPromptTime: Date.now()
        };
        saveTimerState(state);
        renderTimerBar(state);
        startTimerTicker();
    }
    trigger15MinuteReminder(state, 900000);
};

// Exportar funciones globalmente para interactuar desde HTML
/**
 * Activa o pausa el cronómetro desde el botón Play de una fila de imputación de hoy
 */
function toggleTimesheetRowTimer(btn) {
    if (!btn) return;
    const row = btn.closest('.timesheet-row');
    const tsId = parseInt(btn.dataset.id, 10);
    const pId = parseInt(btn.dataset.projectId, 10) || (row ? parseInt(row.dataset.projectId, 10) : 0) || 0;
    const pName = btn.dataset.projectName || (row ? row.dataset.projectName : '') || '';
    const taskId = parseInt(btn.dataset.taskId, 10) || (row ? parseInt(row.dataset.taskId, 10) : 0) || 0;
    const taskName = btn.dataset.taskName || (row ? row.dataset.taskName : '') || '';
    const hours = parseFloat(btn.dataset.hours) || (row ? parseFloat(row.dataset.hours) : 0) || 0;
    const desc = btn.dataset.desc || (row ? row.dataset.desc : '') || '';

    const current = getTimerState();

    // Si este mismo registro ya está seleccionado, alternar directamente entre pausar y reanudar
    if (current && current.timesheetId === tsId) {
        togglePauseTimer();
        return;
    }

    // Si hay otro temporizador activo, pausarlo primero
    if (current && current.status === 'running') {
        togglePauseTimer();
    }

    // Iniciar o reanudar el cronómetro para esta imputación concreta
    const rowDate = btn.dataset.date || (row ? row.dataset.date : '') || '';
    const accumulatedMs = Math.round(hours * 3600 * 1000);
    startWorkTimer(pId, pName, taskId, taskName, desc, tsId, accumulatedMs, rowDate, false);
}

/**
 * Asegura que la fila de la imputación exista en la tabla de partes de horas.
 * Si no existe (porque se creó en Odoo al iniciar trabajo desde un proyecto), la inserta dinámicamente.
 */
function ensureTimesheetRowExists(serverData, timerState) {
    const tsId = (serverData && serverData.timesheet_id) || (timerState && timerState.timesheetId);
    if (!tsId) return;

    let row = document.querySelector(`.timesheet-row[data-id="${tsId}"]`);
    if (row) {
        row.dataset.timerRunning = 'true';
        row.classList.add('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
        return;
    }

    // Si la fila no existe en la tabla actual, la inyectamos al principio del tbody
    const tbody = document.querySelector('#timesheet-table tbody');
    if (!tbody) return;

    // Eliminar fila vacía ("No hay imputaciones") si existe
    const emptyRow = tbody.querySelector('#empty-row');
    if (emptyRow) {
        emptyRow.remove();
    }

    const todayStr = (serverData && serverData.date) || ((typeof formatISODate === 'function') ? formatISODate(new Date()) : new Date().toISOString().split('T')[0]);
    const projectName = (serverData && serverData.project_name) || (timerState && timerState.projectName) || ('Proyecto #' + ((timerState && timerState.projectId) || ''));
    const projectId = (timerState && timerState.projectId) || (serverData && serverData.project_id) || '';
    const taskName = (serverData && serverData.task_name) || (timerState && timerState.taskName) || '';
    const taskId = (timerState && timerState.taskId) || (serverData && serverData.task_id) || '';
    const desc = (serverData && serverData.description) || (timerState && timerState.description) || '';
    const accumMs = (timerState && timerState.accumulatedMs) || ((serverData && serverData.accumulated_ms) || 0);
    const unitAmount = (accumMs > 0) ? (accumMs / 3600000) : ((serverData && typeof serverData.unit_amount === 'number') ? serverData.unit_amount : 0);
    const hours = unitAmount.toFixed(2);

    // Obtener nombre del trabajador
    const workerName = (serverData && serverData.employee_name) || (typeof getActiveWorkerName === 'function' ? getActiveWorkerName() : (document.body ? document.body.dataset.currentWorker : '') || 'Yo');
    const workerInitial = workerName.charAt(0).toUpperCase() || 'U';

    const tr = document.createElement('tr');
    tr.className = 'timesheet-row hover:bg-slate-50/80 transition-colors bg-emerald-50/70 ring-1 ring-emerald-300';
    tr.dataset.id = tsId;
    tr.dataset.date = todayStr;
    tr.dataset.timerRunning = 'true';
    tr.dataset.employee = workerName;
    tr.dataset.project = projectName;
    tr.dataset.projectName = projectName;
    tr.dataset.projectId = projectId;
    tr.dataset.task = taskName;
    tr.dataset.taskId = taskId;
    tr.dataset.taskName = taskName;
    tr.dataset.desc = desc;
    tr.dataset.hours = hours;
    tr.dataset.invoiced = 'false';

    tr.innerHTML = `
        <td class="py-3 px-4 sm:px-6 whitespace-nowrap">
            <span class="font-medium text-slate-900 font-mono text-xs">${todayStr}</span>
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
            <button type="button"
                    onclick="selectSidebarProject(this.dataset.projectName, this.dataset.projectId)"
                    data-project-name="${projectName}"
                    data-project-id="${projectId}"
                    class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-sky-50 text-sky-800 border border-sky-100 hover:bg-sky-100 transition cursor-pointer"
                    title="Filtrar por este proyecto">
                ${projectName}
            </button>
        </td>
        <td class="py-3 px-4 whitespace-nowrap">
            ${taskName ? `<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-slate-100 text-slate-700">${taskName}</span>` : '<span class="text-slate-400 text-xs">-</span>'}
        </td>
        <td class="py-3 px-4 text-slate-600 max-w-xs truncate" title="${desc || 'Sin descripción'}">
            ${desc || '<span class="italic text-slate-400">Sin descripción</span>'}
        </td>
        <td class="py-3 px-4 sm:px-6 text-right whitespace-nowrap">
            <div class="inline-flex items-center justify-end space-x-1.5">
                <span class="timesheet-hours-badge inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-emerald-100 text-emerald-900 border border-emerald-300 font-mono shadow-xs">
                    <span class="relative flex h-2 w-2">
                        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                        <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                    </span>
                    <span class="timer-live-clock font-mono font-bold text-emerald-900">00:00:00</span>
                    <span class="text-[10px] text-emerald-700 font-medium">(${hours}h)</span>
                </span>
            </div>
        </td>
        <td class="py-3 px-3 text-right whitespace-nowrap">
            <div class="inline-flex items-center justify-end space-x-1">
                <button type="button"
                        onclick="toggleTimesheetRowTimer(this)"
                        data-id="${tsId}"
                        data-date="${todayStr}"
                        data-project-id="${projectId}"
                        data-project-name="${projectName}"
                        data-task-id="${taskId}"
                        data-task-name="${taskName}"
                        data-hours="${hours}"
                        data-desc="${desc}"
                        class="btn-row-timer-play inline-flex items-center justify-center w-7 h-7 text-amber-700 bg-amber-100 hover:bg-amber-200 border-amber-300 animate-pulse rounded-lg transition cursor-pointer"
                        title="Pausar cronómetro de esta imputación">
                    <svg class="w-3.5 h-3.5 icon-play hidden" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z" clip-rule="evenodd" />
                    </svg>
                    <svg class="w-3.5 h-3.5 icon-pause" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zM7 8a1 1 0 012 0v4a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v4a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd" />
                    </svg>
                </button>
                <button type="button"
                        onclick="finalizeActiveTimer()"
                        class="btn-row-timer-stop inline-flex items-center justify-center w-7 h-7 text-rose-600 hover:text-rose-700 bg-rose-50 hover:bg-rose-100 border border-rose-200 rounded-lg transition cursor-pointer"
                        title="Detener y consolidar cronómetro en Odoo">
                    <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 00-1 1v4a1 1 0 001 1h4a1 1 0 001-1V8a1 1 0 00-1-1H8z" clip-rule="evenodd" />
                    </svg>
                </button>
                <button type="button"
                        onclick="openEditTimesheetModal(this)"
                        data-id="${tsId}"
                        data-date="${todayStr}"
                        data-project-id="${projectId}"
                        data-project-name="${projectName}"
                        data-task-id="${taskId}"
                        data-task-name="${taskName}"
                        data-name="${desc}"
                        data-hours="${hours}"
                        class="inline-flex items-center justify-center w-7 h-7 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded-lg transition cursor-pointer"
                        title="Editar este parte de horas">
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                    </svg>
                </button>
            </div>
        </td>
    `;

    tbody.insertBefore(tr, tbody.firstChild);
    if (typeof applyTimesheetFilters === 'function') {
        applyTimesheetFilters();
    }
}

/**
 * Actualiza el aspecto de todos los botones de play y stop en las filas de imputaciones
 */
function updateAllRowTimerButtonStates() {
    const current = getTimerState();
    const activeTsId = current ? current.timesheetId : null;
    const isRunning = current && current.status === 'running';

    document.querySelectorAll('.timesheet-row').forEach(row => {
        const rowId = parseInt(row.dataset.id, 10);
        const playBtn = row.querySelector('.btn-row-timer-play');
        const stopBtn = row.querySelector('.btn-row-timer-stop');
        if (!playBtn) return;

        const iconPlay = playBtn.querySelector('.icon-play');
        const iconPause = playBtn.querySelector('.icon-pause');

        if (activeTsId && rowId === activeTsId) {
            if (isRunning) {
                // Fila activa corriendo
                row.classList.add('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
                playBtn.classList.remove('text-emerald-600', 'bg-emerald-50', 'hover:bg-emerald-100', 'border-emerald-200/80');
                playBtn.classList.add('text-amber-700', 'bg-amber-100', 'hover:bg-amber-200', 'border-amber-300', 'animate-pulse');
                playBtn.title = 'Pausar cronómetro de esta imputación';
                if (iconPlay) iconPlay.classList.add('hidden');
                if (iconPause) iconPause.classList.remove('hidden');
            } else {
                // Fila activa pero en pausa
                row.classList.remove('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
                playBtn.classList.remove('text-amber-700', 'bg-amber-100', 'hover:bg-amber-200', 'border-amber-300', 'animate-pulse');
                playBtn.classList.add('text-emerald-600', 'bg-emerald-50', 'hover:bg-emerald-100', 'border-emerald-200/80');
                playBtn.title = 'Reanudar cronómetro en esta imputación';
                if (iconPlay) iconPlay.classList.remove('hidden');
                if (iconPause) iconPause.classList.add('hidden');
            }
            if (stopBtn) stopBtn.classList.remove('hidden');
        } else {
            // Fila normal inactiva
            row.classList.remove('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
            playBtn.classList.remove('text-amber-700', 'bg-amber-100', 'hover:bg-amber-200', 'border-amber-300', 'animate-pulse');
            playBtn.classList.add('text-emerald-600', 'bg-emerald-50', 'hover:bg-emerald-100', 'border-emerald-200/80');
            playBtn.title = 'Activar o reanudar cronómetro en esta imputación';
            if (iconPlay) iconPlay.classList.remove('hidden');
            if (iconPause) iconPause.classList.add('hidden');
            if (stopBtn) stopBtn.classList.add('hidden');
        }
    });
}

window.startWorkTimer = startWorkTimer;
window.togglePauseTimer = togglePauseTimer;
window.finalizeActiveTimer = finalizeActiveTimer;
window.confirmDiscardTimer = confirmDiscardTimer;
window.confirmContinueTimer = confirmContinueTimer;
window.confirmPauseTimer = confirmPauseTimer;
window.confirmFinalizeTimerFromModal = confirmFinalizeTimerFromModal;
window.clearTimer = clearTimer;
window.toggleTimesheetRowTimer = toggleTimesheetRowTimer;
window.updateAllRowTimerButtonStates = updateAllRowTimerButtonStates;
window.getTimerState = getTimerState;
