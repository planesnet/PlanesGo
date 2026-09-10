/**
 * PlanesGo - Temporizador de Trabajo Activo y Recordatorio de 15 Minutos
 * Gestiona el cronómetro en vivo, persistencia en localStorage, avisos periódicos y atajo Shift+Ctrl+T.
 */

const PLANESGO_TIMER_KEY = 'planesgo_active_timer';
const TIMER_PROMPT_INTERVAL_MS = 15 * 60 * 1000; // 15 minutos en milisegundos

let timerIntervalId = null;
let titleFlashIntervalId = null;
let originalDocumentTitle = document.title || 'PlanesGo - Proyectos y Horas Odoo';

// Inicialización automática al cargar el DOM
document.addEventListener('DOMContentLoaded', function () {
    originalDocumentTitle = document.title;
    initTimerFromStorage();
    setupGlobalTimerKeyboardShortcut();
});

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
 * Inicia o reanuda un temporizador de trabajo (admite imputación existente)
 */
function startWorkTimer(projectId, projectName, taskId, taskName, description, timesheetId, accumulatedMs) {
    if (!projectId && !timesheetId) {
        alert('Debes seleccionar un proyecto o imputación para iniciar el trabajo.');
        return;
    }

    // Solicitar permiso de notificaciones de forma proactiva al iniciar
    requestNotificationPermission();

    const now = Date.now();
    const initialAccumulated = (typeof accumulatedMs === 'number' && accumulatedMs >= 0) ? accumulatedMs : 0;

    const state = {
        timesheetId: timesheetId ? parseInt(timesheetId, 10) : null,
        projectId: projectId ? parseInt(projectId, 10) : 0,
        projectName: projectName || (projectId ? 'Proyecto #' + projectId : 'Imputación activa'),
        taskId: taskId ? parseInt(taskId, 10) : null,
        taskName: taskName || '',
        description: description || '',
        status: 'running', // 'running' | 'paused'
        startedAt: now - initialAccumulated,
        lastStartTime: now,
        accumulatedMs: initialAccumulated,
        lastPromptTime: now
    };

    saveTimerState(state);
    renderTimerBar(state);
    startTimerTicker();
    updateAllRowTimerButtonStates();

    // Sincronizar inicio con Odoo en segundo plano (action_timer_start / is_timer_running=true)
    fetch('/api/timer/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            project_id: state.projectId || 0,
            project_name: state.projectName || '',
            task_id: state.taskId || 0,
            task_name: state.taskName || '',
            timesheet_id: state.timesheetId || 0,
            description: state.description
        })
    }).then(res => res.json()).then(data => {
        if (data && data.timesheet_id) {
            state.timesheetId = data.timesheet_id;
            saveTimerState(state);
            ensureTimesheetRowExists(data, state);
            updateAllRowTimerButtonStates();
        }
    }).catch(err => console.warn('[PlanesGo Timer] Error sincronizando inicio con Odoo:', err));

    // Cerrar modal de imputación si estaba abierto
    if (typeof closeCreateTimesheetModal === 'function') {
        closeCreateTimesheetModal();
    }

    console.log(`[PlanesGo Timer] Trabajo iniciado en "${state.projectName}" (Timesheet ID: ${state.timesheetId || 'nuevo'})`);
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
                const hoursBadge = row.querySelector('.timesheet-hours-badge, td:nth-last-child(2) span.font-mono');
                if (hoursBadge) hoursBadge.textContent = `${totalHoursDecimal.toFixed(2)} h`;
            }
        }

        console.log('[PlanesGo Timer] Trabajo en pausa. Tiempo acumulado:', formatElapsedMs(state.accumulatedMs));
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

        console.log('[PlanesGo Timer] Trabajo reanudado');
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
                const hoursBadge = row.querySelector('.timesheet-hours-badge, td:nth-last-child(2) span.font-mono');
                if (hoursBadge) hoursBadge.textContent = `${totalHours.toFixed(2)} h`;
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

    const container = document.getElementById('active-timer-container');
    if (container) {
        container.classList.add('hidden');
    }

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

/**
 * Actualiza cada segundo el cronómetro en pantalla y evalúa el recordatorio
 */
function updateTimerTick() {
    const state = getTimerState();
    if (!state) {
        stopTimerTicker();
        const container = document.getElementById('active-timer-container');
        if (container) container.classList.add('hidden');
        return;
    }

    const now = Date.now();
    let totalMs = state.accumulatedMs || 0;

    if (state.status === 'running' && state.lastStartTime) {
        totalMs += (now - state.lastStartTime);

        // Comprobar si han transcurrido 15 minutos desde el último prompt
        const timeSincePrompt = now - (state.lastPromptTime || state.startedAt || now);
        if (timeSincePrompt >= TIMER_PROMPT_INTERVAL_MS) {
            trigger15MinuteReminder(state, totalMs);
        }

        // Cuenta regresiva al próximo aviso
        const remainingForNextPrompt = Math.max(0, TIMER_PROMPT_INTERVAL_MS - timeSincePrompt);
        const nextPromptEl = document.getElementById('timer-next-prompt');
        if (nextPromptEl) {
            nextPromptEl.textContent = formatCountdown(remainingForNextPrompt);
        }
    } else {
        const nextPromptEl = document.getElementById('timer-next-prompt');
        if (nextPromptEl) {
            nextPromptEl.textContent = 'En pausa';
        }
    }

    // Actualizar visualización del reloj
    const clockEl = document.getElementById('timer-clock-display');
    if (clockEl) {
        clockEl.textContent = formatElapsedMs(totalMs);
    }

    // Si el modal de confirmación de 15 minutos está visible en pantalla, mantener su contador activo en tiempo real
    const modalTimeEl = document.getElementById('confirm-modal-time');
    if (modalTimeEl) {
        modalTimeEl.textContent = formatElapsedMs(totalMs);
    }

    // Si el temporizador corresponde a una imputación de la tabla, actualizar sus horas en pantalla en tiempo real
    if (state.timesheetId) {
        const row = document.querySelector(`.timesheet-row[data-id="${state.timesheetId}"]`);
        if (row) {
            const hoursDecimal = (totalMs / 3600000).toFixed(2);
            row.dataset.hours = hoursDecimal;
            row.dataset.timerRunning = (state.status === 'running') ? 'true' : 'false';
            const hoursBadge = row.querySelector('.timesheet-hours-badge, td:nth-last-child(2) span.font-mono');
            if (hoursBadge) {
                hoursBadge.textContent = `${hoursDecimal} h`;
            }
        }
    }
}

/**
 * Dispara la alerta de 15 minutos (sonido, notificación del sistema y modal)
 */
function trigger15MinuteReminder(state, currentTotalMs) {
    // Actualizar marca de tiempo para evitar disparo repetitivo inmediato
    state.lastPromptTime = Date.now();
    saveTimerState(state);

    // 1. Reproducir sonido suave de aviso (Web Audio API)
    playChimeSound();

    // 2. Disparar notificación del sistema operativo / navegador
    triggerSystemNotification(
        'PlanesGo: ¿Sigues trabajando?',
        `Han transcurrido 15 minutos en: ${state.projectName}\nHaz clic para confirmar o pausar.`
    );

    // 3. Parpadeo del título de la pestaña
    startTitleFlash();

    // 4. Mostrar modal interactivo en pantalla
    showTimerConfirmModal(state, currentTotalMs);
}

/**
 * Muestra el modal de confirmación de 15 minutos
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

    modal.classList.remove('hidden');
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
    if (state) {
        state.lastPromptTime = Date.now();
        saveTimerState(state);
    }
    stopTitleFlash();
    hideTimerConfirmModal();
}

/**
 * Acción del usuario en el modal: "Pausar trabajo"
 */
function confirmPauseTimer() {
    const state = getTimerState();
    if (state && state.status === 'running') {
        togglePauseTimer();
    }
    stopTitleFlash();
    hideTimerConfirmModal();
}

/**
 * Acción del usuario en el modal: "Finalizar y guardar"
 */
function confirmFinalizeTimerFromModal() {
    hideTimerConfirmModal();
    finalizeActiveTimer();
}

/**
 * Renderiza la barra visual según el estado del temporizador
 */
function renderTimerBar(state) {
    const container = document.getElementById('active-timer-container');
    if (!container) return;

    if (!state) {
        container.classList.add('hidden');
        return;
    }

    container.classList.remove('hidden');

    // Nombre de Proyecto y Tarea
    const projEl = document.getElementById('timer-project-name');
    if (projEl) projEl.textContent = state.projectName || 'Proyecto';

    const taskEl = document.getElementById('timer-task-name');
    if (taskEl) {
        taskEl.textContent = state.taskName ? `- ${state.taskName}` : '- Sin tarea';
    }

    const descEl = document.getElementById('timer-description-preview');
    if (descEl) {
        if (state.description) {
            descEl.textContent = state.description;
            descEl.classList.remove('hidden');
        } else {
            descEl.classList.add('hidden');
        }
    }

    // Elementos de estado
    const badge = document.getElementById('timer-badge');
    const ping = document.getElementById('timer-ping');
    const dot = document.getElementById('timer-dot');
    const statusBox = document.getElementById('timer-status-indicator');
    const btnPause = document.getElementById('btn-timer-toggle-pause');
    const iconPause = document.getElementById('icon-timer-pause');
    const iconResume = document.getElementById('icon-timer-resume');
    const textPause = document.getElementById('text-timer-pause');

    if (state.status === 'running') {
        if (badge) {
            badge.textContent = 'En curso';
            badge.className = 'px-2 py-0.5 text-[10px] font-bold rounded-full bg-emerald-100 text-emerald-800 tracking-wide uppercase';
        }
        if (ping) ping.className = 'animate-ping absolute inline-flex h-4 w-4 rounded-full bg-emerald-400 opacity-75';
        if (dot) dot.className = 'relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500';
        if (statusBox) statusBox.className = 'relative flex items-center justify-center flex-shrink-0 w-8 h-8 rounded-xl bg-emerald-50 border border-emerald-200';
        
        if (btnPause) {
            btnPause.className = 'px-3.5 py-2 text-xs font-bold rounded-xl border transition shadow-sm flex items-center space-x-1.5 cursor-pointer bg-white border-amber-300 text-amber-700 hover:bg-amber-50';
        }
        if (iconPause) iconPause.classList.remove('hidden');
        if (iconResume) iconResume.classList.add('hidden');
        if (textPause) textPause.textContent = 'Pausar';
    } else {
        if (badge) {
            badge.textContent = 'En pausa';
            badge.className = 'px-2 py-0.5 text-[10px] font-bold rounded-full bg-amber-100 text-amber-800 tracking-wide uppercase';
        }
        if (ping) ping.className = 'hidden';
        if (dot) dot.className = 'relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-500';
        if (statusBox) statusBox.className = 'relative flex items-center justify-center flex-shrink-0 w-8 h-8 rounded-xl bg-amber-50 border border-amber-200';

        if (btnPause) {
            btnPause.className = 'px-3.5 py-2 text-xs font-bold rounded-xl border transition shadow-sm flex items-center space-x-1.5 cursor-pointer bg-amber-500 hover:bg-amber-600 text-white border-amber-600';
        }
        if (iconPause) iconPause.classList.add('hidden');
        if (iconResume) iconResume.classList.remove('hidden');
        if (textPause) textPause.textContent = 'Reanudar';
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
        const startedAt = parseInt(container.dataset.startedAt, 10) || Date.now();
        const accumMs = parseInt(container.dataset.accumulatedMs, 10) || 0;
        const isRunning = container.dataset.isRunning === 'true';

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
            lastPromptTime: Date.now()
        };
        saveTimerState(serverState);
        renderTimerBar(serverState);
        if (isRunning) {
            startTimerTicker();
        }
        updateAllRowTimerButtonStates();
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

    // Sincronización periódica liviana con Odoo cada 15s y al recuperar foco de ventana
    setInterval(syncActiveTimerFromOdoo, 15000);
    document.addEventListener('visibilitychange', () => {
        if (!document.hidden) {
            syncActiveTimerFromOdoo();
        }
    });
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

                const serverState = {
                    timesheetId: act.timesheet_id,
                    projectId: act.project_id,
                    projectName: act.project_name || ('Proyecto #' + act.project_id),
                    taskId: act.task_id || null,
                    taskName: act.task_name || '',
                    description: act.description || '',
                    status: 'running',
                    startedAt: act.started_at || (Date.now() - accumulatedMs),
                    lastStartTime: Date.now(),
                    accumulatedMs: accumulatedMs,
                    lastPromptTime: Date.now()
                };
                saveTimerState(serverState);
                renderTimerBar(serverState);
                startTimerTicker();
                ensureTimesheetRowExists(act, serverState);
                updateAllRowTimerButtonStates();
            } else {
                // En Odoo NO hay cronómetro corriendo -> Limpiar temporizador local si estaba activo
                const current = getTimerState();
                if (current && current.status === 'running') {
                    console.log('[PlanesGo Timer] Odoo no tiene cronómetro activo. Limpiando estado local.');
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

/**
 * Sonido de campana suave usando Web Audio API (sin mp3s externos)
 */
function playChimeSound() {
    try {
        const AudioCtx = window.AudioContext || window.webkitAudioContext;
        if (!AudioCtx) return;
        const ctx = new AudioCtx();

        const playTone = (freq, delay, duration) => {
            const osc = ctx.createOscillator();
            const gain = ctx.createGain();

            osc.type = 'sine';
            osc.frequency.setValueAtTime(freq, ctx.currentTime + delay);

            gain.gain.setValueAtTime(0.001, ctx.currentTime + delay);
            gain.gain.exponentialRampToValueAtTime(0.2, ctx.currentTime + delay + 0.05);
            gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + delay + duration);

            osc.connect(gain);
            gain.connect(ctx.destination);

            osc.start(ctx.currentTime + delay);
            osc.stop(ctx.currentTime + delay + duration);
        };

        // Acorde suave bifónico: D5 (587 Hz) y A5 (880 Hz)
        playTone(587.33, 0, 0.6);
        playTone(880.00, 0.15, 0.8);
    } catch (e) {
        console.warn('AudioContext no disponible o bloqueado por el navegador:', e);
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
 * Muestra notificación de escritorio del sistema operativo
 */
function triggerSystemNotification(title, body) {
    if (!('Notification' in window)) return;

    if (Notification.permission === 'granted') {
        try {
            const notif = new Notification(title, {
                body: body,
                icon: '/static/favicon.ico',
                requireInteraction: true
            });

            notif.onclick = function () {
                window.focus();
                showTimerConfirmModal();
                notif.close();
            };
        } catch (e) {
            console.warn('Error al mostrar notificación de escritorio:', e);
        }
    }
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
    const accumulatedMs = Math.round(hours * 3600 * 1000);
    startWorkTimer(pId, pName, taskId, taskName, desc, tsId, accumulatedMs);
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
    const emptyRow = tbody.querySelector('tr td[colspan]');
    if (emptyRow) {
        emptyRow.closest('tr').remove();
    }

    const todayStr = (serverData && serverData.date) || new Date().toISOString().split('T')[0];
    const projectName = (serverData && serverData.project_name) || (timerState && timerState.projectName) || ('Proyecto #' + ((timerState && timerState.projectId) || ''));
    const projectId = (timerState && timerState.projectId) || (serverData && serverData.project_id) || '';
    const taskName = (serverData && serverData.task_name) || (timerState && timerState.taskName) || '';
    const taskId = (timerState && timerState.taskId) || (serverData && serverData.task_id) || '';
    const desc = (serverData && serverData.description) || (timerState && timerState.description) || '';
    const unitAmount = (serverData && typeof serverData.unit_amount === 'number') ? serverData.unit_amount : 0;
    const hours = unitAmount.toFixed(2);

    // Obtener nombre del trabajador
    const workerBadge = document.querySelector('.timesheet-row[data-employee]');
    const workerName = (serverData && serverData.employee_name) || (workerBadge ? workerBadge.dataset.employee : (document.querySelector('#user-menu-btn span')?.textContent?.trim() || 'Yo'));
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
                <span class="timesheet-hours-badge inline-block px-2.5 py-0.5 rounded-lg text-xs font-bold bg-sky-50 text-sky-700 border border-sky-100 font-mono">
                    ${hours} h
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
}

/**
 * Actualiza el aspecto de todos los botones de play en las filas de imputaciones
 */
function updateAllRowTimerButtonStates() {
    const current = getTimerState();
    const activeTsId = (current && current.status === 'running') ? current.timesheetId : null;

    document.querySelectorAll('.timesheet-row').forEach(row => {
        const rowId = parseInt(row.dataset.id, 10);
        const playBtn = row.querySelector('.btn-row-timer-play');
        if (!playBtn) return;

        const iconPlay = playBtn.querySelector('.icon-play');
        const iconPause = playBtn.querySelector('.icon-pause');

        if (activeTsId && rowId === activeTsId) {
            // Fila activa: resaltado visual y botón de pausa pulsante
            row.classList.add('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
            playBtn.classList.remove('text-emerald-600', 'bg-emerald-50', 'hover:bg-emerald-100', 'border-emerald-200/80');
            playBtn.classList.add('text-amber-700', 'bg-amber-100', 'hover:bg-amber-200', 'border-amber-300', 'animate-pulse');
            playBtn.title = 'Pausar cronómetro de esta imputación';
            if (iconPlay) iconPlay.classList.add('hidden');
            if (iconPause) iconPause.classList.remove('hidden');
        } else {
            // Fila normal inactiva
            row.classList.remove('bg-emerald-50/70', 'ring-1', 'ring-emerald-300');
            playBtn.classList.remove('text-amber-700', 'bg-amber-100', 'hover:bg-amber-200', 'border-amber-300', 'animate-pulse');
            playBtn.classList.add('text-emerald-600', 'bg-emerald-50', 'hover:bg-emerald-100', 'border-emerald-200/80');
            playBtn.title = 'Activar o reanudar cronómetro en esta imputación';
            if (iconPlay) iconPlay.classList.remove('hidden');
            if (iconPause) iconPause.classList.add('hidden');
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
