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
 * Inicia un nuevo temporizador de trabajo
 */
function startWorkTimer(projectId, projectName, taskId, taskName, description) {
    if (!projectId) {
        alert('Debes seleccionar un proyecto para iniciar el trabajo.');
        return;
    }

    // Solicitar permiso de notificaciones de forma proactiva al iniciar
    requestNotificationPermission();

    const now = Date.now();
    const state = {
        projectId: parseInt(projectId, 10),
        projectName: projectName || ('Proyecto #' + projectId),
        taskId: taskId ? parseInt(taskId, 10) : null,
        taskName: taskName || '',
        description: description || '',
        status: 'running', // 'running' | 'paused'
        startedAt: now,
        lastStartTime: now,
        accumulatedMs: 0,
        lastPromptTime: now
    };

    saveTimerState(state);
    renderTimerBar(state);
    startTimerTicker();

    // Cerrar modal de imputación si estaba abierto
    if (typeof closeCreateTimesheetModal === 'function') {
        closeCreateTimesheetModal();
    }

    // Feedback visual al usuario
    console.log(`[PlanesGo Timer] Trabajo iniciado en "${state.projectName}"`);
}

/**
 * Alterna entre Pausar y Reanudar el temporizador
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
        console.log('[PlanesGo Timer] Trabajo en pausa. Tiempo acumulado:', formatElapsedMs(state.accumulatedMs));
    } else {
        // Reanudar
        state.status = 'running';
        state.lastStartTime = now;
        state.lastPromptTime = now; // reinicia el ciclo de 15 minutos al reanudar
        console.log('[PlanesGo Timer] Trabajo reanudado');
    }

    saveTimerState(state);
    renderTimerBar(state);
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
    saveTimerState(null);
    stopTimerTicker();
    stopTitleFlash();
    hideTimerConfirmModal();

    const container = document.getElementById('active-timer-container');
    if (container) {
        container.classList.add('hidden');
    }
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
 * Inicializa el temporizador si ya existía en localStorage al cargar la página
 */
function initTimerFromStorage() {
    const state = getTimerState();
    if (state) {
        renderTimerBar(state);
        startTimerTicker();
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
window.startWorkTimer = startWorkTimer;
window.togglePauseTimer = togglePauseTimer;
window.finalizeActiveTimer = finalizeActiveTimer;
window.confirmDiscardTimer = confirmDiscardTimer;
window.confirmContinueTimer = confirmContinueTimer;
window.confirmPauseTimer = confirmPauseTimer;
window.confirmFinalizeTimerFromModal = confirmFinalizeTimerFromModal;
window.clearTimer = clearTimer;
