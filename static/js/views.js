/**
 * static/js/views.js
 * Lógica de navegación por semanas, selector de vistas (Lista, Calendario, Gantt),
 * cálculo de KPIs reactivos y renderizado de las vistas visuales.
 */

const currentMonday = getMonday(new Date());
let selectedWeekMonday = new Date(currentMonday);
function getSavedView() {
    try {
        const urlParams = new URLSearchParams(window.location.search);
        const urlView = urlParams.get('view');
        if (urlView && ['list', 'calendar', 'gantt', 'express'].includes(urlView)) {
            return urlView;
        }
        const saved = localStorage.getItem('planesgo_active_view');
        if (saved && ['list', 'calendar', 'gantt', 'express'].includes(saved)) {
            return saved;
        }
        const match = document.cookie.match(/(?:^|;\s*)planesgo_view=([^;]+)/);
        if (match && ['list', 'calendar', 'gantt', 'express'].includes(match[1])) {
            return match[1];
        }
    } catch (e) {
        console.warn('[PlanesGo] Error obteniendo vista guardada:', e);
    }
    return 'list';
}

let currentView = getSavedView();

// Registro de semanas ya cargadas en el DOM (clave: YYYY-MM-DD del lunes)
const loadedWeeks = new Set();
if (typeof formatISODate === 'function') {
    loadedWeeks.add(formatISODate(currentMonday));
}

let isWeekLoading = false;

function showWeekLoadingIndicator(isLoading) {
    const titleEl = document.getElementById('week-title-display');
    const btnPrev = document.getElementById('btn-prev-week');
    const btnNext = document.getElementById('btn-next-week');

    if (isLoading) {
        if (btnPrev) btnPrev.classList.add('opacity-50', 'pointer-events-none');
        if (btnNext) btnNext.classList.add('opacity-50', 'pointer-events-none');
        if (titleEl && !titleEl.querySelector('.week-spinner')) {
            const sp = document.createElement('span');
            sp.className = 'week-spinner inline-block w-3.5 h-3.5 ml-2 border-2 border-sky-600 border-t-transparent rounded-full animate-spin align-middle';
            titleEl.appendChild(sp);
        }
    } else {
        if (btnPrev) btnPrev.classList.remove('opacity-50', 'pointer-events-none');
        if (btnNext) btnNext.classList.remove('opacity-50', 'pointer-events-none');
        const sp = document.querySelector('.week-spinner');
        if (sp) sp.remove();
    }
}

/**
 * Inserta partes de horas devueltos por la API para una semana concreta
 */
function insertWeekEntriesIntoTable(entries) {
    const tbody = document.querySelector('#timesheet-table tbody');
    if (!tbody || !Array.isArray(entries)) return;

    // Si había una fila de "No hay partes", la quitamos si vienen nuevos datos
    const emptyRow = tbody.querySelector('#empty-row');
    if (emptyRow && entries.length > 0) {
        emptyRow.remove();
    }

    const todayStr = (typeof formatISODate === 'function') ? formatISODate(new Date()) : new Date().toISOString().split('T')[0];

    entries.forEach(entry => {
        // Evitar duplicados si ya existe en el DOM
        if (tbody.querySelector(`.timesheet-row[data-id="${entry.id}"]`)) {
            return;
        }

        const isRunning = Boolean(entry.is_timer_running);
        const empName = (entry.employee_id && entry.employee_id.name) ? entry.employee_id.name : 
                        ((entry.user_id && entry.user_id.name) ? entry.user_id.name : 'Sin asignar');
        const empInitial = empName.charAt(0).toUpperCase() || 'U';
        const projName = entry.project_id ? (entry.project_id.name || '') : '';
        const projId = entry.project_id ? entry.project_id.id : 0;
        const taskName = entry.task_id ? (entry.task_id.name || '') : '';
        const taskId = entry.task_id ? entry.task_id.id : 0;
        const hoursFormatted = (typeof entry.unit_amount === 'number') ? entry.unit_amount.toFixed(2) : '0.00';
        const desc = entry.name || '';
        const isToday = (entry.date === todayStr);
        const isInvoiced = Boolean(entry.timesheet_invoice_id && entry.timesheet_invoice_id.id);
        const invoiceName = entry.timesheet_invoice_id ? (entry.timesheet_invoice_id.name || `#${entry.timesheet_invoice_id.id}`) : '';

        const isAgy = Boolean(entry.is_antigravity || (typeof isAntigravityTask === 'function' && isAntigravityTask(taskName, desc)));
        const isMaquina = Boolean(entry.is_hora_maquina || isAgy);
        const isHombre = entry.is_hora_hombre !== undefined ? entry.is_hora_hombre : !isMaquina;

        const tr = document.createElement('tr');
        tr.className = `timesheet-row hover:bg-slate-50/80 transition-colors ${isRunning ? 'bg-emerald-50/70 ring-1 ring-emerald-300' : ''}`;
        tr.dataset.id = entry.id;
        tr.dataset.date = entry.date;
        tr.dataset.timerRunning = isRunning ? 'true' : 'false';
        tr.dataset.employee = empName;
        tr.dataset.project = projName;
        tr.dataset.projectName = projName;
        tr.dataset.projectId = projId;
        tr.dataset.task = taskName;
        tr.dataset.taskId = taskId;
        tr.dataset.taskName = taskName;
        tr.dataset.desc = desc;
        tr.dataset.hours = hoursFormatted;
        tr.dataset.invoiced = isInvoiced ? 'true' : 'false';
        tr.dataset.horaMaquina = isMaquina ? 'true' : 'false';
        tr.dataset.horaHombre = isHombre ? 'true' : 'false';
        tr.dataset.isAntigravity = isAgy ? 'true' : 'false';

        const safeEmpName = (typeof escapeHtml === 'function') ? escapeHtml(empName) : empName;
        const safeProjName = (typeof escapeHtml === 'function') ? escapeHtml(projName) : projName;
        const safeTaskName = (typeof escapeHtml === 'function') ? escapeHtml(taskName) : taskName;
        const safeDesc = (typeof escapeHtml === 'function') ? escapeHtml(desc) : desc;
        const safeInvoice = (typeof escapeHtml === 'function') ? escapeHtml(invoiceName) : invoiceName;

        tr.innerHTML = `
            <td class="py-2.5 px-3 whitespace-nowrap">
                <span class="font-medium text-slate-900 font-mono text-xs">${entry.date}</span>
            </td>
            <td class="py-2.5 px-3 whitespace-nowrap">
                <div class="flex items-center space-x-1.5">
                    <div class="w-5 h-5 rounded-full bg-slate-200 text-slate-600 flex items-center justify-center font-bold text-[9px] shrink-0">
                        ${empInitial}
                    </div>
                    <span class="font-medium text-slate-800 text-xs sm:text-sm">${safeEmpName}</span>
                </div>
            </td>
            <td class="py-2.5 px-3 whitespace-nowrap col-project-cell">
                ${projName ? `
                <button type="button"
                        onclick="selectSidebarProject(this.dataset.projectName, this.dataset.projectId)"
                        data-project-name="${safeProjName}"
                        data-project-id="${projId}"
                        class="inline-flex items-center gap-1.5 font-medium text-slate-800 hover:text-sky-600 transition cursor-pointer text-left truncate max-w-[170px] xl:max-w-[220px]"
                        title="Filtrar por este proyecto: ${safeProjName}">
                    <span class="truncate">${safeProjName}</span>
                </button>` : `<span class="text-slate-400 text-xs">-</span>`}
            </td>
            <td class="py-2.5 px-3 whitespace-nowrap">
                ${(typeof renderTaskBadgeHTML === 'function') ? renderTaskBadgeHTML(taskName) : (taskName ? `<span class="text-slate-600 font-medium text-xs sm:text-sm truncate block max-w-[150px] xl:max-w-[190px]" title="${safeTaskName}">${safeTaskName}</span>` : `<span class="text-slate-400 text-xs">-</span>`)}
            </td>
            <td class="py-2.5 px-2 whitespace-nowrap">
                ${(typeof renderTagsHTML === 'function') ? renderTagsHTML(entry.tags, isAgy, isMaquina, isHombre) : `<span class="text-slate-300 text-xs">-</span>`}
            </td>
            <td class="py-2.5 px-3 text-slate-600 max-w-xs xl:max-w-md truncate" title="${safeDesc}">
                ${desc ? safeDesc : `<span class="italic text-slate-400">Sin descripción</span>`}
            </td>
            <td class="py-2.5 px-3 text-right whitespace-nowrap">
                <div class="inline-flex items-center justify-end space-x-1.5">
                    ${isInvoiced ? `
                    <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200/80" title="Factura: ${safeInvoice}">
                        <svg class="w-2.5 h-2.5 mr-1 text-emerald-600" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
                        </svg>
                        Facturado
                    </span>` : ''}
                    <span class="timesheet-hours-badge ${isRunning ? 'inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-lg text-xs font-bold bg-emerald-100 text-emerald-900 border border-emerald-300 font-mono shadow-xs' : 'inline-block px-2.5 py-0.5 rounded-lg text-xs font-bold bg-sky-50 text-sky-700 border border-sky-100 font-mono'}">
                        ${isRunning ? `
                        <span class="relative flex h-2 w-2">
                            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                            <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                        </span>
                        <span class="timer-live-clock font-mono font-bold text-emerald-900">${hoursFormatted} h</span>
                        <span class="text-[10px] text-emerald-700 font-medium">(Activo)</span>
                        ` : `${hoursFormatted} h`}
                    </span>
                </div>
            </td>
            <td class="py-2.5 px-3 text-right whitespace-nowrap">
                <div class="inline-flex items-center justify-end space-x-1">
                    ${(isToday && !isInvoiced) ? `
                    <button type="button"
                            onclick="toggleTimesheetRowTimer(this)"
                            data-id="${entry.id}"
                            data-date="${entry.date}"
                            data-project-id="${projId}"
                            data-project-name="${safeProjName}"
                            data-task-id="${taskId}"
                            data-task-name="${safeTaskName}"
                            data-hours="${hoursFormatted}"
                            data-desc="${safeDesc}"
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
                            onclick="finalizeActiveTimer(this)"
                            data-id="${entry.id}"
                            data-date="${entry.date}"
                            data-project-id="${projId}"
                            data-project-name="${safeProjName}"
                            data-task-id="${taskId}"
                            data-task-name="${safeTaskName}"
                            data-hours="${hoursFormatted}"
                            data-desc="${safeDesc}"
                            class="btn-row-timer-stop inline-flex items-center justify-center w-7 h-7 text-rose-600 hover:text-rose-700 bg-rose-50 hover:bg-rose-100 border border-rose-200 rounded-lg transition cursor-pointer ${isRunning ? '' : 'hidden'}"
                            title="Detener y consolidar cronómetro en Odoo">
                        <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 00-1 1v4a1 1 0 001 1h4a1 1 0 001-1V8a1 1 0 00-1-1H8z" clip-rule="evenodd" />
                        </svg>
                    </button>` : ''}
                    
                    <button type="button"
                            onclick="openEditTimesheetModal(this)"
                            data-id="${entry.id}"
                            data-date="${entry.date}"
                            data-project-id="${projId}"
                            data-project-name="${safeProjName}"
                            data-task-id="${taskId}"
                            data-task-name="${safeTaskName}"
                            data-name="${safeDesc}"
                            data-hours="${hoursFormatted}"
                            class="inline-flex items-center justify-center w-7 h-7 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded-lg transition cursor-pointer"
                            title="Editar este parte de horas">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                        </svg>
                    </button>

                    ${isInvoiced ? `
                    <span class="inline-flex items-center justify-center w-7 h-7 text-slate-300 rounded-lg cursor-not-allowed"
                          title="No se puede borrar: Imputación ya facturada en Odoo (Factura ${safeInvoice})">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                        </svg>
                    </span>` : `
                    <button type="button"
                            onclick="openDeleteTimesheetModal(this)"
                            data-id="${entry.id}"
                            data-date="${entry.date}"
                            data-project-name="${safeProjName}"
                            data-task-name="${safeTaskName}"
                            data-hours="${hoursFormatted}"
                            data-desc="${safeDesc}"
                            class="inline-flex items-center justify-center w-7 h-7 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded-lg transition cursor-pointer"
                            title="Eliminar este parte de horas">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                    </button>`}
                </div>
            </td>
        `;
        tbody.appendChild(tr);
    });
}

/**
 * Consulta la API para traer partes de horas de una semana específica
 */
async function fetchWeekTimesheets(mondayStr, sundayStr) {
    isWeekLoading = true;
    showWeekLoadingIndicator(true);

    try {
        const resp = await fetch(`/api/timesheets?date_from=${mondayStr}&date_to=${sundayStr}`, {
            cache: 'no-cache'
        });
        if (!resp.ok) {
            throw new Error(`HTTP ${resp.status}`);
        }
        const entries = await resp.json();
        insertWeekEntriesIntoTable(entries);
        loadedWeeks.add(mondayStr);
    } catch (err) {
        console.error('[PlanesGo] Error cargando partes de la semana:', err);
        if (typeof showToast === 'function') {
            showToast('No se pudieron obtener partes de horas para esa semana', 'error');
        }
    } finally {
        isWeekLoading = false;
        showWeekLoadingIndicator(false);
        updateWeekControls();
        applyTimesheetFilters();
    }
}

async function navigateWeek(delta) {
    if (isWeekLoading) return;

    selectedWeekMonday.setDate(selectedWeekMonday.getDate() + delta * 7);
    const mondayStr = (typeof formatISODate === 'function') ? formatISODate(selectedWeekMonday) : selectedWeekMonday.toISOString().split('T')[0];
    const sundayDate = getSunday(selectedWeekMonday);
    const sundayStr = (typeof formatISODate === 'function') ? formatISODate(sundayDate) : sundayDate.toISOString().split('T')[0];

    updateWeekControls();

    if (loadedWeeks.has(mondayStr)) {
        applyTimesheetFilters();
        return;
    }

    await fetchWeekTimesheets(mondayStr, sundayStr);
}

function updateWeekControls() {
    const titleEl = document.getElementById('week-title-display');
    const datesEl = document.getElementById('week-dates-display');
    const btnPrev = document.getElementById('btn-prev-week');
    const btnNext = document.getElementById('btn-next-week');

    const kpiHoursSub = document.getElementById('kpi-hours-sub');
    const kpiProjectsSub = document.getElementById('kpi-projects-sub');
    const kpiEntriesSub = document.getElementById('kpi-entries-sub');
    const kpiEmployeesSub = document.getElementById('kpi-employees-sub');

    // La navegación hacia semanas pasadas siempre es viable (se cargan on-demand)
    const prevViable = true;
    // Siguiente es viable hasta 1 semana en el futuro respecto a la semana actual
    const maxFutureMs = currentMonday.getTime() + (7 * 24 * 3600 * 1000);
    const nextViable = (selectedWeekMonday.getTime() <= maxFutureMs);

    if (btnPrev) {
        btnPrev.disabled = !prevViable;
        if (prevViable) {
            btnPrev.classList.remove('opacity-40', 'cursor-not-allowed', 'pointer-events-none');
        } else {
            btnPrev.classList.add('opacity-40', 'cursor-not-allowed', 'pointer-events-none');
        }
    }

    if (btnNext) {
        btnNext.disabled = !nextViable;
        if (nextViable) {
            btnNext.classList.remove('opacity-40', 'cursor-not-allowed', 'pointer-events-none');
        } else {
            btnNext.classList.add('opacity-40', 'cursor-not-allowed', 'pointer-events-none');
        }
    }

    const isCurrent = isSameWeek(selectedWeekMonday, currentMonday);
    const sunday = getSunday(selectedWeekMonday);
    const weekNum = getISOWeekNumber(selectedWeekMonday);

    if (titleEl) {
        titleEl.textContent = isCurrent ? `Semana actual (S${weekNum})` : `Semana ${weekNum}`;
    }
    
    const monStr = selectedWeekMonday.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });
    const sunStr = sunday.toLocaleDateString('es-ES', { day: 'numeric', month: 'short', year: 'numeric' });
    if (datesEl) datesEl.textContent = `${monStr} – ${sunStr}`;

    if (kpiHoursSub) kpiHoursSub.textContent = 'En semana seleccionada';
    if (kpiProjectsSub) kpiProjectsSub.textContent = 'con partes esta semana';
    if (kpiEntriesSub) kpiEntriesSub.textContent = 'imputaciones esta semana';
    if (kpiEmployeesSub) kpiEmployeesSub.textContent = 'con actividad esta semana';
}

let expressTimesheets = null;
let isExpressLoading = false;
let isExpressSilentLoading = false;

function switchView(viewName) {
    if (!['list', 'calendar', 'gantt', 'express'].includes(viewName)) {
        viewName = 'list';
    }
    currentView = viewName;

    try {
        localStorage.setItem('planesgo_active_view', viewName);
        document.cookie = `planesgo_view=${viewName}; path=/; max-age=31536000; SameSite=Lax`;
        const url = new URL(window.location);
        if (viewName === 'list') {
            url.searchParams.delete('view');
        } else {
            url.searchParams.set('view', viewName);
        }
        window.history.replaceState({ view: viewName }, '', url.toString());
    } catch (e) {
        console.warn('[PlanesGo] Error guardando vista activa:', e);
    }

    const btnExpress = document.getElementById('btn-view-express');
    const btnList = document.getElementById('btn-view-list');
    const btnCal = document.getElementById('btn-view-calendar');
    const btnGantt = document.getElementById('btn-view-gantt');

    const containerExpress = document.getElementById('view-container-express');
    const containerList = document.getElementById('view-container-list');
    const containerCal = document.getElementById('view-container-calendar');
    const containerGantt = document.getElementById('view-container-gantt');

    const activeClasses = ['bg-white', 'text-sky-700', 'shadow-2xs', 'border-slate-200/80', 'font-bold'];
    const inactiveClasses = ['text-slate-600', 'hover:text-slate-900', 'hover:bg-slate-200/50', 'border-transparent', 'font-medium'];

    function applyBtnStyle(btn, isActive) {
        if (!btn) return;
        const icon = btn.querySelector('svg');
        if (isActive) {
            btn.classList.add(...activeClasses);
            btn.classList.remove(...inactiveClasses);
            if (icon) icon.className = 'w-3.5 h-3.5 text-sky-600';
        } else {
            btn.classList.remove(...activeClasses);
            btn.classList.add(...inactiveClasses);
            if (icon) icon.className = 'w-3.5 h-3.5 text-slate-500';
        }
    }

    applyBtnStyle(btnExpress, viewName === 'express');
    applyBtnStyle(btnList, viewName === 'list');
    applyBtnStyle(btnCal, viewName === 'calendar');
    applyBtnStyle(btnGantt, viewName === 'gantt');

    if (containerExpress) containerExpress.classList.toggle('hidden', viewName !== 'express');
    if (containerList) containerList.classList.toggle('hidden', viewName !== 'list');
    if (containerCal) containerCal.classList.toggle('hidden', viewName !== 'calendar');
    if (containerGantt) containerGantt.classList.toggle('hidden', viewName !== 'gantt');

    if (viewName === 'express') {
        if (!expressTimesheets && !isExpressLoading) {
            loadExpressTimesheets();
        } else {
            renderExpressView();
            loadExpressTimesheets(true, true);
        }
    }

    applyTimesheetFilters();
}

window.addEventListener('popstate', (e) => {
    const v = (e.state && e.state.view) || getSavedView();
    if (v && v !== currentView && typeof switchView === 'function') {
        switchView(v);
    }
});

function applyTimesheetFilters() {
    const searchInput = document.getElementById('filter-search');
    const projectSelect = document.getElementById('filter-project');
    const employeeSelect = document.getElementById('filter-employee');
    const sidebarEmployeeSelect = document.getElementById('sidebar-employee-select');
    const rows = document.querySelectorAll('.timesheet-row');
    const emptyFilterRow = document.getElementById('empty-filter-row');
    
    const kpiHours = document.getElementById('kpi-total-hours');
    const kpiEntries = document.getElementById('kpi-total-entries');
    const kpiProjects = document.getElementById('kpi-total-projects');
    const kpiEmployees = document.getElementById('kpi-total-employees');

    const searchVal = (searchInput ? searchInput.value : '').toLowerCase().trim();
    const projectVal = (projectSelect ? projectSelect.value : '').toLowerCase().trim();
    const employeeVal = (sidebarEmployeeSelect ? sidebarEmployeeSelect.value : (employeeSelect ? employeeSelect.value : '')).toLowerCase().trim();

    const targetProjectId = activeSidebarProjectId;
    const targetProjectName = (projectVal || activeSidebarProjectName).toLowerCase().trim();

    let visibleHours = 0;
    let visibleHoursHombre = 0;
    let visibleHoursMaquina = 0;
    let visibleCount = 0;
    const visibleProjects = new Set();
    const visibleEmployees = new Set();
    const matchingRows = [];

    const selectedWeekSunday = getSunday(selectedWeekMonday);

    rows.forEach(row => {
        const desc = (row.dataset.desc || '').toLowerCase();
        const task = (row.dataset.task || '').toLowerCase();
        const project = (row.dataset.project || '').toLowerCase();
        const projectName = (row.dataset.projectName || '').toLowerCase();
        const rowProjectId = (row.dataset.projectId || '').trim();
        const employee = (row.dataset.employee || '').toLowerCase();
        const hours = parseFloat(row.dataset.hours) || 0;
        const rowDateStr = row.dataset.date || '';

        const isTimerRunning = (row.dataset.timerRunning === 'true');
        const isMaquina = (row.dataset.horaMaquina === 'true');
        const typeSearch = isMaquina ? 'hora máquina maquina antigravity agy hm ag computo ia' : 'hora hombre humano persona hh';

        const matchSearch = !searchVal || desc.includes(searchVal) || task.includes(searchVal) || project.includes(searchVal) || projectName.includes(searchVal) || typeSearch.includes(searchVal);

        let matchProject = true;
        const hasProjectFilter = Boolean((targetProjectId && targetProjectId !== '0') || targetProjectName);
        if (targetProjectId && targetProjectId !== '0') {
            matchProject = (rowProjectId === targetProjectId);
            if (!matchProject && targetProjectName) {
                matchProject = (projectName === targetProjectName || project === targetProjectName || projectName.includes(targetProjectName) || project.includes(targetProjectName));
            }
        } else if (targetProjectName) {
            matchProject = (projectName === targetProjectName || project === targetProjectName || projectName.includes(targetProjectName) || project.includes(targetProjectName));
        }

        const matchEmployee = !employeeVal || employee.includes(employeeVal) || (employeeVal.includes(employee) && employee.length > 2) || (isTimerRunning && (employee === 'yo' || !employee));

        let matchWeek = false;
        if (rowDateStr) {
            const rowDate = parseISODate(rowDateStr);
            if (rowDate) {
                matchWeek = (rowDate >= selectedWeekMonday && rowDate <= selectedWeekSunday);
            }
        }

        // Si el temporizador está corriendo para el usuario, debe ser visible en la semana actual respetando el filtro estricto de proyecto
        const shouldShow = isTimerRunning
            ? ((!hasProjectFilter || matchProject) && matchEmployee && (matchWeek || !rowDateStr))
            : (matchSearch && matchProject && matchEmployee && matchWeek);

        if (shouldShow) {
            row.style.display = '';
            visibleHours += hours;
            if (isMaquina) {
                visibleHoursMaquina += hours;
            } else {
                visibleHoursHombre += hours;
            }
            visibleCount++;
            if (row.dataset.project) visibleProjects.add(row.dataset.project);
            if (row.dataset.employee) visibleEmployees.add(row.dataset.employee);

            matchingRows.push({
                id: row.dataset.id,
                date: row.dataset.date,
                hours: hours,
                hoursFormatted: row.dataset.hoursFormatted || `${hours.toFixed(2)}h`,
                project: row.dataset.project || '',
                projectName: row.dataset.projectName || row.dataset.project || '',
                projectId: row.dataset.projectId || '',
                task: row.dataset.task || '',
                taskId: row.dataset.taskId || '',
                desc: row.dataset.desc || '',
                employee: row.dataset.employee || '',
                invoiced: row.dataset.invoiced === 'true',
                invoiceId: row.dataset.invoiceId || '',
                invoiceName: row.dataset.invoiceName || ''
            });
        } else {
            row.style.display = 'none';
        }
    });

    // Fila de tabla vacía en vista Lista
    const emptyRow = document.getElementById('empty-row');
    if (emptyFilterRow) {
        if (visibleCount === 0) {
            if (emptyRow && rows.length === 0) {
                emptyRow.classList.remove('hidden');
                emptyFilterRow.classList.add('hidden');
            } else {
                if (emptyRow) emptyRow.classList.add('hidden');
                emptyFilterRow.classList.remove('hidden');
            }
        } else {
            if (emptyRow) emptyRow.classList.add('hidden');
            emptyFilterRow.classList.add('hidden');
        }
    }

    // Ocultar columna Proyecto si hay un proyecto seleccionado; mostrarla si no hay proyecto seleccionado
    const hasActiveProject = Boolean(targetProjectId || targetProjectName);
    const colProjectHeaders = document.querySelectorAll('.col-project-header');
    const colProjectCells = document.querySelectorAll('.col-project-cell');
    colProjectHeaders.forEach(th => {
        th.style.display = hasActiveProject ? 'none' : '';
    });
    colProjectCells.forEach(td => {
        td.style.display = hasActiveProject ? 'none' : '';
    });

    if (emptyFilterRow) {
        const td = emptyFilterRow.querySelector('td');
        if (td) td.colSpan = hasActiveProject ? 7 : 8;
    }
    if (emptyRow) {
        const td = emptyRow.querySelector('td');
        if (td) td.colSpan = hasActiveProject ? 7 : 8;
    }

    // Actualizar KPIs superiores
    if (kpiHours) kpiHours.textContent = visibleHours.toFixed(2);
    const kpiHoursHombre = document.getElementById('kpi-hours-hombre');
    const kpiHoursMaquina = document.getElementById('kpi-hours-maquina');
    if (kpiHoursHombre) kpiHoursHombre.textContent = visibleHoursHombre.toFixed(2);
    if (kpiHoursMaquina) kpiHoursMaquina.textContent = visibleHoursMaquina.toFixed(2);
    if (kpiEntries) kpiEntries.textContent = visibleCount;
    if (kpiProjects) kpiProjects.textContent = visibleProjects.size;
    if (kpiEmployees) kpiEmployees.textContent = visibleEmployees.size;

    // Resaltar proyecto en el sidebar
    highlightSidebarProject(targetProjectName, targetProjectId);
    updateWeekControls();

    // Sincronizar vistas (Express, Calendario, Gantt)
    if (currentView === 'express') {
        renderExpressView();
    } else if (currentView === 'calendar') {
        renderCalendarView(matchingRows);
    } else if (currentView === 'gantt') {
        renderGanttView(matchingRows);
    }
}

function renderCalendarView(matchingRows) {
    const container = document.getElementById('calendar-grid-container') || document.getElementById('calendar-grid-days');
    if (!container) return;

    const rangeEl = document.getElementById('calendar-week-range');
    const totalEl = document.getElementById('calendar-week-total');

    // 1. Días de la semana: L, M, X, J, V, S, D
    const dayLetters = ['L', 'M', 'X', 'J', 'V', 'S', 'D'];
    const dayFullNames = ['Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado', 'Domingo'];
    const today = new Date();

    const weekDays = [];
    for (let i = 0; i < 7; i++) {
        const dayDate = new Date(selectedWeekMonday);
        dayDate.setDate(dayDate.getDate() + i);
        weekDays.push({
            letter: dayLetters[i],
            fullName: dayFullNames[i],
            date: dayDate,
            iso: formatISODate(dayDate),
            formatted: dayDate.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' }),
            isToday: isSameDay(dayDate, today)
        });
    }

    if (rangeEl && weekDays.length === 7) {
        const startStr = weekDays[0].date.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });
        const endStr = weekDays[6].date.toLocaleDateString('es-ES', { day: 'numeric', month: 'short', year: 'numeric' });
        rangeEl.textContent = `(${startStr} - ${endStr})`;
    }

    // 2. Agrupar por Proyecto (columna más a la izquierda, uno por cada fila)
    const projectsMap = new Map();
    matchingRows.forEach(r => {
        const pKey = r.projectId || r.projectName || 'sin_proyecto';
        if (!projectsMap.has(pKey)) {
            projectsMap.set(pKey, {
                projectId: r.projectId || '',
                projectName: r.projectName || 'Sin proyecto asignado',
                days: {},
                dayEntries: {},
                totalHours: 0
            });
        }
        const p = projectsMap.get(pKey);
        p.totalHours += r.hours;
        p.days[r.date] = (p.days[r.date] || 0) + r.hours;
        if (!p.dayEntries[r.date]) p.dayEntries[r.date] = [];
        p.dayEntries[r.date].push(r);
    });

    // Si hay un proyecto activo seleccionado en el sidebar pero no tiene imputaciones esta semana
    if (activeSidebarProjectId && activeSidebarProjectId !== '0' && !projectsMap.has(activeSidebarProjectId)) {
        projectsMap.set(activeSidebarProjectId, {
            projectId: activeSidebarProjectId,
            projectName: activeSidebarProjectName || 'Proyecto Seleccionado',
            days: {},
            dayEntries: {},
            totalHours: 0
        });
    }

    // 3. Totales por cada día y total general de la semana
    const dailyTotals = [0, 0, 0, 0, 0, 0, 0];
    let grandTotal = 0;

    for (let i = 0; i < 7; i++) {
        const iso = weekDays[i].iso;
        projectsMap.forEach(proj => {
            dailyTotals[i] += (proj.days[iso] || 0);
        });
        grandTotal += dailyTotals[i];
    }

    if (totalEl) totalEl.textContent = grandTotal.toFixed(2);

    // 4. Si no hay proyectos con imputaciones para esta semana
    if (projectsMap.size === 0) {
        container.innerHTML = `
            <div class="py-12 px-4 text-center text-slate-400">
                <svg class="mx-auto h-10 w-10 text-slate-300 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2z"/>
                </svg>
                <p class="font-semibold text-slate-600 text-sm">No hay partes de horas registrados para esta semana</p>
                <p class="text-xs text-slate-400 mt-1">Usa los botones de navegación de semana o registra horas para tus proyectos.</p>
                <button type="button" onclick="openCreateTimesheetModal()" class="mt-4 px-4 py-2 bg-sky-600 hover:bg-sky-700 text-white rounded-xl text-xs font-semibold shadow-xs transition cursor-pointer">
                    + Imputar Horas
                </button>
            </div>
        `;
        return;
    }

    // 5. Construir la cuadrícula / hoja de datos
    let html = `
        <table class="w-full min-w-[700px] border-collapse text-left text-xs sm:text-sm">
            <thead>
                <tr class="bg-slate-50/90 border-b border-slate-200 text-slate-700">
                    <th class="py-3 px-4 font-bold text-slate-700 w-64 sm:w-80">
                        Proyecto
                    </th>
    `;

    // Fila de cabecera con los días L, M, X, J, V, S, D
    weekDays.forEach(wd => {
        html += `
            <th class="py-2.5 px-2 text-center w-20 sm:w-24 border-l border-slate-200/60 ${wd.isToday ? 'bg-sky-50/90 text-sky-900 ring-1 ring-inset ring-sky-300' : ''}">
                <div class="text-sm sm:text-base font-black ${wd.isToday ? 'text-sky-700' : 'text-slate-800'}">
                    ${wd.letter}
                </div>
                <div class="text-[10px] sm:text-[11px] font-mono font-medium ${wd.isToday ? 'text-sky-600 font-bold' : 'text-slate-400'}">
                    ${wd.formatted}
                </div>
            </th>
        `;
    });

    html += `
                    <th class="py-3 px-4 text-right font-bold text-slate-700 w-24 sm:w-28 border-l border-slate-200/70">
                        Total
                    </th>
                </tr>
            </thead>
            <tbody class="divide-y divide-slate-100 bg-white">
    `;

    // Filas de proyectos
    projectsMap.forEach(proj => {
        const safeProjName = escapeHtml(proj.projectName);
        html += `
            <tr class="hover:bg-slate-50/70 transition-colors">
                <td class="py-3 px-4 text-slate-800 font-medium">
                    <div class="flex items-center space-x-2.5">
                        <span class="w-2.5 h-2.5 rounded-full ${proj.totalHours > 0 ? 'bg-sky-500' : 'bg-slate-300'} shrink-0"></span>
                        <span class="truncate max-w-[200px] sm:max-w-[280px]" title="${safeProjName}">
                            ${safeProjName}
                        </span>
                    </div>
                </td>
        `;

        // Columnas L, M, X, J, V, S, D para cada proyecto
        weekDays.forEach(wd => {
            const dayHours = proj.days[wd.iso] || 0;
            const entries = proj.dayEntries[wd.iso] || [];

            html += `
                <td class="py-2 px-1.5 sm:px-2 text-center border-l border-slate-100 ${wd.isToday ? 'bg-sky-50/30' : ''}">
            `;

            if (dayHours > 0) {
                // Tooltip con desglose de imputaciones del día
                const tooltipLines = entries.map(e => {
                    const taskStr = e.task ? ` [${e.task}]` : '';
                    const descStr = e.desc ? ` - ${e.desc}` : '';
                    return `• ${e.employee}: ${e.hours.toFixed(2)}h${taskStr}${descStr}`;
                }).join('\n');

                html += `
                    <button type="button" 
                            onclick="openCreateTimesheetModal('${proj.projectId}', '', '${wd.iso}')"
                            title="${escapeAttr(tooltipLines)}"
                            class="inline-flex items-center justify-center font-mono font-bold text-xs sm:text-sm px-2.5 py-1 rounded-lg bg-sky-50 text-sky-800 hover:bg-sky-100 hover:text-sky-900 border border-sky-200/80 shadow-2xs transition-all cursor-pointer">
                        ${dayHours.toFixed(2)}
                    </button>
                `;
            } else {
                html += `
                    <button type="button" 
                            onclick="openCreateTimesheetModal('${proj.projectId}', '', '${wd.iso}')"
                            title="Añadir horas en ${wd.fullName} para ${safeProjName}"
                            class="inline-flex items-center justify-center text-slate-300 hover:text-sky-600 hover:bg-sky-50 font-mono text-xs w-7 h-7 rounded-lg transition cursor-pointer">
                        -
                    </button>
                `;
            }

            html += `</td>`;
        });

        // Columna Total por Proyecto
        html += `
                <td class="py-3 px-4 text-right font-mono font-bold border-l border-slate-200/70 ${proj.totalHours > 0 ? 'text-slate-900' : 'text-slate-400'}">
                    ${proj.totalHours > 0 ? proj.totalHours.toFixed(2) + ' h' : '-'}
                </td>
            </tr>
        `;
    });

    // 6. Última fila: Total de cada día de las horas realizadas
    html += `
            </tbody>
            <tfoot>
                <tr class="bg-slate-100/90 border-t-2 border-slate-300 font-bold text-slate-800">
                    <td class="py-3 px-4 font-black uppercase text-[11px] tracking-wider text-slate-700">
                        Total Diario
                    </td>
    `;

    for (let i = 0; i < 7; i++) {
        const isToday = weekDays[i].isToday;
        const dayTotal = dailyTotals[i];

        html += `
            <td class="py-3 px-2 text-center font-mono border-l border-slate-200 ${isToday ? 'bg-sky-100/60 text-sky-900' : ''}">
                <span class="text-xs sm:text-sm font-extrabold ${dayTotal > 0 ? 'text-slate-900' : 'text-slate-400'}">
                    ${dayTotal > 0 ? dayTotal.toFixed(2) : '-'}
                </span>
            </td>
        `;
    }

    html += `
                    <td class="py-3 px-4 text-right font-mono font-black text-sky-800 text-xs sm:text-sm border-l border-slate-300 bg-slate-200/50">
                        ${grandTotal.toFixed(2)} h
                    </td>
                </tr>
            </tfoot>
        </table>
    `;

    container.innerHTML = html;
}

function renderGanttView(matchingRows) {
    const tableContainer = document.getElementById('gantt-table-container');
    const totalEl = document.getElementById('gantt-week-total');
    if (!tableContainer) return;

    const dayNamesShort = ['Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb', 'Dom'];
    const today = new Date();

    const weekDays = [];
    for (let i = 0; i < 7; i++) {
        const dayDate = new Date(selectedWeekMonday);
        dayDate.setDate(dayDate.getDate() + i);
        weekDays.push({
            index: i,
            date: dayDate,
            iso: formatISODate(dayDate),
            name: dayNamesShort[i],
            formatted: dayDate.toLocaleDateString('es-ES', { day: 'numeric', month: 'numeric' }),
            isToday: isSameDay(dayDate, today)
        });
    }

    let totalHours = 0;

    // Agrupar por Proyecto y dentro por Tarea
    const projectsMap = new Map();
    matchingRows.forEach(r => {
        totalHours += r.hours;
        const pKey = r.projectId || r.projectName || 'sin_proyecto';
        if (!projectsMap.has(pKey)) {
            projectsMap.set(pKey, {
                projectId: r.projectId,
                projectName: r.projectName || 'Proyecto General',
                totalHours: 0,
                tasks: new Map()
            });
        }
        const p = projectsMap.get(pKey);
        p.totalHours += r.hours;

        const tKey = r.taskId || r.task || 'sin_tarea';
        if (!p.tasks.has(tKey)) {
            p.tasks.set(tKey, {
                taskId: r.taskId,
                taskName: r.task || 'Actividad general sin tarea',
                totalHours: 0,
                days: {}
            });
        }
        const t = p.tasks.get(tKey);
        t.totalHours += r.hours;
        if (!t.days[r.date]) {
            t.days[r.date] = { hours: 0, entries: [] };
        }
        t.days[r.date].hours += r.hours;
        t.days[r.date].entries.push(r);
    });

    if (totalEl) totalEl.textContent = totalHours.toFixed(2);

    if (projectsMap.size === 0) {
        tableContainer.innerHTML = `
            <div class="py-12 text-center text-slate-400">
                <svg class="w-10 h-10 mx-auto text-slate-300 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                </svg>
                <p class="text-xs font-semibold text-slate-600">No hay imputaciones registradas para el período y filtros actuales</p>
                <p class="text-[11px] text-slate-400 mt-0.5">Usa el selector de semanas o selecciona otro proyecto</p>
            </div>
        `;
        return;
    }

    let html = `
        <table class="w-full min-w-[700px] border-collapse text-left">
            <thead>
                <tr class="bg-slate-50 border-b border-slate-200/80 text-[11px] font-bold text-slate-600">
                    <th class="py-2.5 px-3 w-64">Proyecto / Tarea</th>
    `;

    weekDays.forEach(wd => {
        html += `
            <th class="py-2.5 px-2 text-center w-24 ${wd.isToday ? 'bg-sky-100/50 text-sky-800' : ''}">
                <div>${wd.name}</div>
                <div class="text-[10px] font-mono font-normal ${wd.isToday ? 'text-sky-600 font-bold' : 'text-slate-400'}">${wd.formatted}</div>
            </th>
        `;
    });

    html += `
                    <th class="py-2.5 px-3 text-right w-20">Total</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-slate-100 text-xs">
    `;

    projectsMap.forEach(proj => {
        const safeProjName = escapeHtml(proj.projectName);
        html += `
            <tr class="bg-slate-100/70 font-bold text-slate-800">
                <td class="py-2 px-3 flex items-center space-x-2">
                    <span class="w-2 h-2 rounded-full bg-sky-500"></span>
                    <span class="truncate max-w-[240px]" title="${safeProjName}">${safeProjName}</span>
                </td>
        `;

        weekDays.forEach(wd => {
            let projDayHours = 0;
            proj.tasks.forEach(t => {
                if (t.days[wd.iso]) projDayHours += t.days[wd.iso].hours;
            });

            html += `
                <td class="py-2 px-2 text-center font-mono text-[11px] ${wd.isToday ? 'bg-sky-50/40' : ''}">
                    ${projDayHours > 0 ? `<span class="font-extrabold text-sky-700">${projDayHours.toFixed(1)}h</span>` : `<span class="text-slate-300">-</span>`}
                </td>
            `;
        });

        html += `
                <td class="py-2 px-3 text-right font-mono font-extrabold text-indigo-700">
                    ${proj.totalHours.toFixed(2)}h
                </td>
            </tr>
        `;

        proj.tasks.forEach(task => {
            const safeTaskName = escapeHtml(task.taskName);
            html += `
                <tr class="hover:bg-slate-50/60 transition">
                    <td class="py-2 px-3 pl-7 text-slate-600">
                        <span class="truncate block max-w-[220px]" title="${safeTaskName}">↳ ${safeTaskName}</span>
                    </td>
            `;

            weekDays.forEach(wd => {
                const dayData = task.days[wd.iso];
                html += `
                    <td class="py-2 px-2 text-center ${wd.isToday ? 'bg-sky-50/20' : ''}">
                `;

                if (dayData && dayData.hours > 0) {
                    const entriesDesc = dayData.entries.map(e => `${e.employee}: ${e.hoursFormatted} - ${escapeAttr(e.desc || 'Sin desc.')}`).join('\n');
                    html += `
                        <div class="bg-gradient-to-r from-sky-500 to-indigo-600 text-white font-mono text-[10px] font-bold py-1 px-1.5 rounded-lg shadow-2xs hover:opacity-90 transition cursor-help truncate"
                             title="${entriesDesc}">
                            ${dayData.hours.toFixed(1)}h
                        </div>
                    `;
                } else {
                    html += `<span class="text-slate-200">·</span>`;
                }

                html += `</td>`;
            });

            html += `
                    <td class="py-2 px-3 text-right font-mono font-semibold text-slate-700">
                        ${task.totalHours.toFixed(2)}h
                    </td>
                </tr>
            `;
        });
    });

    html += `
            </tbody>
        </table>
    `;

    tableContainer.innerHTML = html;
}

// ============================================================================
// VISTA Y VENTANA FLOTANTE EXPRESS: BOTONERA DE MÁQUINA CONCENTRADA
// ============================================================================

let isExpressMinimized = false;

/**
 * Abre o cierra la ventana flotante de la botonera Express.
 */
function toggleExpressFloating() {
    const win = document.getElementById('express-floating-window');
    if (!win) return;

    const isHidden = win.classList.contains('hidden');
    if (isHidden) {
        openExpressFloating();
    } else {
        closeExpressFloating();
    }
}

/**
 * Abre y muestra la ventana flotante de la botonera Express en modo ordenador.
 */
function openExpressFloating() {
    const win = document.getElementById('express-floating-window');
    const btn = document.getElementById('btn-view-express');
    if (!win) return;

    win.classList.remove('hidden');
    if (btn) {
        btn.classList.add('bg-white', 'text-amber-600', 'shadow-2xs', 'border-amber-200/80', 'font-bold');
        btn.classList.remove('text-slate-600');
    }
    const body = document.getElementById('express-window-body');
    if (body && body.classList.contains('hidden') && typeof toggleMinimizeExpress === 'function') {
        toggleMinimizeExpress();
    }
    loadExpressTimesheets();
    initExpressWindowInteractions();
}

/**
 * Cierra la ventana flotante Express.
 */
function closeExpressFloating() {
    const win = document.getElementById('express-floating-window');
    const btn = document.getElementById('btn-view-express');
    if (win) win.classList.add('hidden');
    if (btn) {
        btn.classList.remove('bg-white', 'text-amber-600', 'shadow-2xs', 'border-amber-200/80', 'font-bold');
        btn.classList.add('text-slate-600');
    }
}

/**
 * Minimiza o restaura la ventana flotante Express.
 */
function toggleMinimizeExpress() {
    const win = document.getElementById('express-floating-window');
    const body = document.getElementById('express-window-body');
    const minBtn = document.getElementById('btn-minimize-express');
    if (!win || !body) return;

    isExpressMinimized = !isExpressMinimized;
    if (isExpressMinimized) {
        body.classList.add('hidden');
        win.style.height = 'auto';
        win.style.minHeight = 'auto';
        win.style.resize = 'none';
        if (minBtn) {
            minBtn.innerHTML = `
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7" />
                </svg>
            `;
            minBtn.title = 'Restaurar ventana';
        }
    } else {
        body.classList.remove('hidden');
        const savedSize = localStorage.getItem('planesgo_express_size');
        if (savedSize) {
            try {
                const size = JSON.parse(savedSize);
                if (size.height) win.style.height = size.height;
                if (size.width) win.style.width = size.width;
            } catch (e) {}
        } else {
            win.style.height = '530px';
        }
        win.style.minHeight = '200px';
        win.style.resize = 'both';
        if (minBtn) {
            minBtn.innerHTML = `
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
            `;
            minBtn.title = 'Minimizar ventana';
        }
    }
}

/**
 * Desacopla la botonera Express o la vista de Tickets abriéndola en una mini-ventana nativa independiente de escritorio (pop-out).
 * @param {string} initialTab - Pestaña inicial a abrir ('express' o 'tickets')
 */
function openExpressPopout(initialTab = 'express') {
    const tab = (initialTab === 'tickets') ? 'tickets' : 'express';
    const w = 540;
    const h = 720;
    const left = Math.max(0, (window.screen.width - w) / 2);
    const top = Math.max(0, (window.screen.height - h) / 2);

    const targetUrl = `/express?tab=${tab}`;
    const win = window.open(
        targetUrl,
        'PlanesGoExpress',
        `width=${w},height=${h},top=${top},left=${left},menubar=no,status=no,toolbar=no,location=no,resizable=yes,scrollbars=yes`
    );

    if (win) {
        try {
            if (win.location && win.location.href && !win.location.href.includes(`tab=${tab}`)) {
                win.location.href = targetUrl;
            } else if (typeof win.switchExpressTab === 'function') {
                win.switchExpressTab(tab);
            }
        } catch (e) {
            // Manejo silencioso ante navegación o cross-origin
        }
        win.focus();
        closeExpressFloating();
    } else {
        openExpressFloating();
        switchExpressTab(tab);
        if (typeof showToast === 'function') {
            showToast('El navegador bloqueó la ventana emergente. Se abrió el panel flotante integrado.', 'warning');
        }
    }
}

let expressDraggableInitialized = false;

/**
 * Inicializa el comportamiento de arrastre (drag) y redimensión para la ventana flotante.
 */
function initExpressWindowInteractions() {
    const win = document.getElementById('express-floating-window');
    const header = document.getElementById('express-drag-handle');
    if (!win || !header || expressDraggableInitialized) return;

    expressDraggableInitialized = true;

    // Restaurar posición guardada
    const savedPos = localStorage.getItem('planesgo_express_pos');
    if (savedPos) {
        try {
            const pos = JSON.parse(savedPos);
            if (pos.top && pos.left) {
                win.style.top = pos.top;
                win.style.left = pos.left;
                win.style.right = 'auto';
            }
        } catch (e) {}
    }

    // Restaurar tamaño guardado
    const savedSize = localStorage.getItem('planesgo_express_size');
    if (savedSize) {
        try {
            const size = JSON.parse(savedSize);
            if (size.width) win.style.width = size.width;
            if (size.height) win.style.height = size.height;
        } catch (e) {}
    }

    let isDragging = false;
    let startX = 0, startY = 0;
    let initialLeft = 0, initialTop = 0;

    header.addEventListener('mousedown', function(e) {
        if (e.target.closest('button') || e.target.closest('input')) return;
        isDragging = true;
        startX = e.clientX;
        startY = e.clientY;
        const rect = win.getBoundingClientRect();
        initialLeft = rect.left;
        initialTop = rect.top;

        win.style.right = 'auto';
        win.style.left = `${initialLeft}px`;
        win.style.top = `${initialTop}px`;

        document.body.style.userSelect = 'none';
    });

    document.addEventListener('mousemove', function(e) {
        if (!isDragging) return;
        const deltaX = e.clientX - startX;
        const deltaY = e.clientY - startY;

        let newLeft = initialLeft + deltaX;
        let newTop = initialTop + deltaY;

        // Limitar dentro de la pantalla
        newLeft = Math.max(10, Math.min(window.innerWidth - win.offsetWidth - 10, newLeft));
        newTop = Math.max(10, Math.min(window.innerHeight - 50, newTop));

        win.style.left = `${newLeft}px`;
        win.style.top = `${newTop}px`;
    });

    document.addEventListener('mouseup', function() {
        if (isDragging) {
            isDragging = false;
            document.body.style.userSelect = '';
            localStorage.setItem('planesgo_express_pos', JSON.stringify({
                left: win.style.left,
                top: win.style.top
            }));
        }
        // Guardar tamaño si el usuario redimensionó
        if (win && win.style.width && win.style.height) {
            localStorage.setItem('planesgo_express_size', JSON.stringify({
                width: win.style.width,
                height: win.style.height
            }));
        }
    });
}

/**
 * Carga desde el servidor las imputaciones de las dos últimas semanas.
 * Soporta actualización silenciosa en segundo plano (isSilent = true) sin spinners ni parpadeos.
 */
function loadExpressTimesheets(forceReload = false, isSilent = false) {
    if (isExpressLoading && !forceReload && !isSilent) return;
    if (isExpressSilentLoading && isSilent) return;

    const loadingEl = document.getElementById('express-loading-state');
    const gridEl = document.getElementById('express-grid-container');
    const emptyEl = document.getElementById('express-empty-state');

    if (expressTimesheets && !forceReload && !isSilent) {
        if (loadingEl) {
            loadingEl.classList.add('hidden');
            loadingEl.classList.remove('flex');
            loadingEl.style.display = 'none';
        }
        if (gridEl) gridEl.classList.remove('hidden');
        renderExpressView(false);
        return;
    }

    if (!isSilent) {
        if (loadingEl) {
            loadingEl.classList.remove('hidden');
            loadingEl.classList.add('flex');
            loadingEl.style.display = '';
        }
        if (gridEl) gridEl.classList.add('hidden');
        if (emptyEl) emptyEl.classList.add('hidden');
        isExpressLoading = true;
    } else {
        isExpressSilentLoading = true;
    }

    const today = new Date();
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];

    const tomorrow = new Date(today);
    tomorrow.setDate(tomorrow.getDate() + 1);
    const dateToStr = (typeof formatISODate === 'function') ? formatISODate(tomorrow) : tomorrow.toISOString().split('T')[0];

    const pastDays = new Date(today);
    pastDays.setDate(pastDays.getDate() - 30);
    const dateFromStr = (typeof formatISODate === 'function') ? formatISODate(pastDays) : pastDays.toISOString().split('T')[0];

    const pTimesheets = fetch(`/api/timesheets?date_from=${dateFromStr}&date_to=${dateToStr}`, { cache: 'no-store' })
        .then(res => {
            if (!res.ok) throw new Error('Error al cargar imputaciones');
            return res.json();
        })
        .catch(err => {
            console.error('[PlanesGo Express] Error cargando imputaciones:', err);
            return [];
        });

    const pActive = fetch('/api/timer/active', { cache: 'no-store' })
        .then(res => res.ok ? res.json() : null)
        .catch(err => null);

    return Promise.all([pTimesheets, pActive])
        .then(([data, activeData]) => {
            expressTimesheets = Array.isArray(data) ? data : [];
            window.expressTimesheets = expressTimesheets;

            const allActiveTimers = [];
            if (activeData) {
                if (Array.isArray(activeData.active_list)) {
                    allActiveTimers.push(...activeData.active_list);
                }
                if (activeData.active && activeData.active.timesheet_id) {
                    if (!allActiveTimers.some(t => t.timesheet_id === activeData.active.timesheet_id)) {
                        allActiveTimers.push(activeData.active);
                    }
                }
            }

            // Actualizar mapa global de cronómetros en memoria
            if (!window.__activeTimersMap) {
                window.__activeTimersMap = new Map();
            }

            allActiveTimers.forEach(act => {
                if (!act || !act.timesheet_id) return;
                const tsId = act.timesheet_id;
                const accumulatedMs = (typeof act.accumulated_ms === 'number' && act.accumulated_ms >= 0)
                    ? act.accumulated_ms
                    : Math.round((act.unit_amount || 0) * 3600 * 1000);
                let startedAt = act.started_at || (Date.now() - accumulatedMs);
                if (startedAt > 0 && startedAt < 1000000000000) {
                    startedAt *= 1000;
                }
                const isAgy = Boolean(act.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(act.task_name, act.description)));

                window.__activeTimersMap.set(tsId, {
                    timesheetId: tsId,
                    projectId: act.project_id,
                    projectName: act.project_name || ('Proyecto #' + act.project_id),
                    taskId: act.task_id || null,
                    taskName: act.task_name || '',
                    description: act.description || '',
                    status: act.is_running ? 'running' : 'paused',
                    startedAt: startedAt,
                    lastStartTime: Date.now(),
                    accumulatedMs: accumulatedMs,
                    unitAmount: act.unit_amount,
                    source: act.source || '',
                    isAntigravity: isAgy
                });

                // Inyectar o asegurar en expressTimesheets
                const found = expressTimesheets.some(ts =>
                    (tsId > 0 && ts.id === tsId) ||
                    (act.task_id > 0 && ts.task_id && ts.task_id.id === act.task_id)
                );
                if (!found) {
                    expressTimesheets.unshift({
                        id: tsId,
                        date: act.date || todayStr,
                        name: act.description || '',
                        unit_amount: act.unit_amount || 0,
                        project_id: { id: act.project_id, name: act.project_name || ('Proyecto #' + act.project_id) },
                        task_id: { id: act.task_id, name: act.task_name || '' },
                        employee_id: { name: act.employee_name || '' },
                        user_id: { name: act.employee_name || '' },
                        is_timer_running: Boolean(act.is_running),
                        is_antigravity: isAgy,
                        source: act.source || ''
                    });
                } else {
                    expressTimesheets.forEach(ts => {
                        if ((tsId > 0 && ts.id === tsId) ||
                            (act.task_id > 0 && ts.task_id && ts.task_id.id === act.task_id)) {
                            ts.is_timer_running = Boolean(act.is_running);
                            if (typeof act.unit_amount === 'number') {
                                ts.unit_amount = act.unit_amount;
                            }
                            if (isAgy) {
                                ts.is_antigravity = true;
                            }
                        }
                    });
                }
            });

            if (activeData && activeData.active && activeData.active.is_running) {
                const act = activeData.active;
                const accumulatedMs = (typeof act.accumulated_ms === 'number' && act.accumulated_ms >= 0)
                    ? act.accumulated_ms
                    : Math.round((act.unit_amount || 0) * 3600 * 1000);
                let startedAt = act.started_at || (Date.now() - accumulatedMs);
                if (startedAt > 0 && startedAt < 1000000000000) {
                    startedAt *= 1000;
                }
                const cur = (typeof getTimerState === 'function') ? getTimerState() : null;
                const isLocalRecent = window.__lastTimerActionTime && (Date.now() - window.__lastTimerActionTime < 4000);
                if (!isLocalRecent && (!cur || cur.timesheetId !== act.timesheet_id || cur.status !== 'running')) {
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
                        lastPromptAccumulatedMs: accumulatedMs,
                        lastPromptTime: Date.now(),
                        promptTriggeredAt: null,
                        promptSnapshotMs: null
                    };
                    if (typeof saveTimerState === 'function') {
                        saveTimerState(serverState);
                    }
                    if (typeof renderTimerBar === 'function') {
                        renderTimerBar(serverState);
                    }
                    if (typeof startTimerTicker === 'function') {
                        startTimerTicker();
                    }
                }
            }

            if (!isSilent) {
                isExpressLoading = false;
                if (loadingEl) {
                    loadingEl.classList.add('hidden');
                    loadingEl.classList.remove('flex');
                    loadingEl.style.display = 'none';
                }
                if (gridEl) gridEl.classList.remove('hidden');
            } else {
                isExpressSilentLoading = false;
            }
            renderExpressView(isSilent);
        })
        .catch(err => {
            console.error('[PlanesGo Express] Error:', err);
            if (!isSilent) {
                isExpressLoading = false;
                if (loadingEl) {
                    loadingEl.classList.add('hidden');
                    loadingEl.classList.remove('flex');
                    loadingEl.style.display = 'none';
                }
                if (gridEl) gridEl.classList.remove('hidden');
            } else {
                isExpressSilentLoading = false;
            }
            expressTimesheets = expressTimesheets || [];
            window.expressTimesheets = expressTimesheets;
            renderExpressView(isSilent);
        });
}

/**
 * Determina si una fecha corresponde a la jornada anterior (ayer, o viernes si hoy es lunes).
 */
function isYesterdayDate(dateStr, today) {
    if (!dateStr) return false;
    const refToday = today || new Date();
    const yesterday = new Date(refToday);
    yesterday.setDate(yesterday.getDate() - 1);
    const yesterdayStr = (typeof formatISODate === 'function') ? formatISODate(yesterday) : yesterday.toISOString().split('T')[0];
    if (dateStr === yesterdayStr) return true;

    // Si hoy es lunes (día 1), incluir también viernes, sábado y domingo como jornada anterior inmediata
    if (refToday.getDay() === 1) {
        const friday = new Date(refToday);
        friday.setDate(friday.getDate() - 3);
        const fridayStr = (typeof formatISODate === 'function') ? formatISODate(friday) : friday.toISOString().split('T')[0];
        if (dateStr >= fridayStr && dateStr <= yesterdayStr) {
            return true;
        }
    }
    return false;
}

/**
 * Formatea una fecha para mostrar en las teclas de la botonera.
 */
function formatCardDate(dateStr) {
    if (!dateStr) return '';
    const today = new Date();
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];
    if (dateStr === todayStr) return 'Hoy';
    if (isYesterdayDate(dateStr, today)) return 'Ayer';

    const parts = dateStr.split('-');
    if (parts.length === 3) {
        const d = new Date(parseInt(parts[0], 10), parseInt(parts[1], 10) - 1, parseInt(parts[2], 10));
        return d.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });
    }
    return dateStr;
}

/**
 * Renderiza la matriz concentrada de la botonera tipo máquina.
 * Soporta actualización minuciosa y silenciosa (isSilent = true) sin parpadeo de DOM.
 */
function renderExpressView(isSilent = false) {
    const gridEl = document.getElementById('express-grid-container');
    const emptyEl = document.getElementById('express-empty-state');
    const loadingEl = document.getElementById('express-loading-state');
    const countBadge = document.getElementById('express-tasks-count-badge');
    if (!gridEl) return;

    if (!isSilent && loadingEl) {
        loadingEl.classList.add('hidden');
        loadingEl.classList.remove('flex');
        loadingEl.style.display = 'none';
    }

    if (!expressTimesheets && !isExpressLoading) {
        loadExpressTimesheets();
        return;
    }

    try {
        if (typeof window.projectPartnerMap === 'string') {
            try { window.projectPartnerMap = JSON.parse(window.projectPartnerMap); } catch (e) {}
        }
        const today = new Date();
        const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];

        // Filtros activos
        const searchInput = document.getElementById('filter-search');
        const searchVal = (searchInput ? searchInput.value : '').toLowerCase().trim();
        const currentWorkerGlobal = (typeof window.currentWorker === 'string' && window.currentWorker.trim()) ? window.currentWorker.trim() : (document.body.dataset.currentWorker || '');
        const sidebarEmployeeSelect = document.getElementById('sidebar-employee-select');
        const employeeSelect = document.getElementById('filter-employee');
        let employeeVal = (sidebarEmployeeSelect ? sidebarEmployeeSelect.value : (employeeSelect ? employeeSelect.value : currentWorkerGlobal)).toLowerCase().trim();
        if (employeeVal === 'todos' || employeeVal === 'todas' || employeeVal === 'all') {
            employeeVal = '';
        }

        const targetProjectId = (typeof activeSidebarProjectId !== 'undefined') ? activeSidebarProjectId : null;
        const targetProjectName = (typeof activeSidebarProjectName !== 'undefined' ? activeSidebarProjectName : '').toLowerCase().trim();

        // 1. Recopilar datos
        const allEntries = [];

        if (Array.isArray(expressTimesheets)) {
            expressTimesheets.forEach(ts => {
                const empName = (ts.employee_id && ts.employee_id.name) ? ts.employee_id.name :
                                ((ts.user_id && ts.user_id.name) ? ts.user_id.name : '');
                const pId = ts.project_id ? ts.project_id.id : 0;
                let partnerId = (ts.partner_id && ts.partner_id.id) ? ts.partner_id.id : 0;
                if (!partnerId && pId && window.projectPartnerMap && window.projectPartnerMap[pId]) {
                    partnerId = window.projectPartnerMap[pId];
                }
                const parsedHours = typeof ts.unit_amount === 'number' ? ts.unit_amount : (parseFloat(ts.unit_amount) || 0);
                allEntries.push({
                    id: ts.id,
                    date: ts.date,
                    projectId: pId,
                    projectName: ts.project_id ? (ts.project_id.name || '') : '',
                    partnerId: partnerId,
                    taskId: ts.task_id ? ts.task_id.id : 0,
                    taskName: ts.task_id ? (ts.task_id.name || '') : '',
                    desc: ts.name || '',
                    hours: parsedHours,
                    employee: empName,
                    isTimerRunning: Boolean(ts.is_timer_running)
                });
            });
        }

        // Integrar filas del DOM
        const existingIds = new Set(allEntries.map(e => String(e.id)));
        document.querySelectorAll('#timesheet-table .timesheet-row').forEach(row => {
            const id = row.dataset.id;
            if (id && !existingIds.has(String(id))) {
                const pId = parseInt(row.dataset.projectId, 10) || 0;
                let partnerId = parseInt(row.dataset.partnerId, 10) || 0;
                if (!partnerId && pId && window.projectPartnerMap && window.projectPartnerMap[pId]) {
                    partnerId = window.projectPartnerMap[pId];
                }
                allEntries.push({
                    id: id,
                    date: row.dataset.date,
                    projectId: pId,
                    projectName: row.dataset.projectName || row.dataset.project || '',
                    partnerId: partnerId,
                    taskId: parseInt(row.dataset.taskId, 10) || 0,
                    taskName: row.dataset.taskName || row.dataset.task || '',
                    desc: row.dataset.desc || '',
                    hours: parseFloat(row.dataset.hours) || 0,
                    employee: row.dataset.employee || '',
                    isTimerRunning: row.dataset.timerRunning === 'true'
                });
            }
        });

        const timerState = (typeof getTimerState === 'function') ? getTimerState() : null;

        // Ordenar allEntries de más recientes a menos recientes (fecha desc, id desc)
        allEntries.sort((a, b) => {
            if (a.date > b.date) return -1;
            if (a.date < b.date) return 1;
            return (parseInt(b.id, 10) || 0) - (parseInt(a.id, 10) || 0);
        });

        // 2. Agrupar por tarea única de la botonera con función reutilizable
        function buildTaskMap(applyEmployeeFilter) {
            const map = new Map();
            allEntries.forEach(entry => {
                const emp = (entry.employee || '').toLowerCase();
                // Si la tarea está corriendo actualmente, nunca filtrarla por trabajador
                if (applyEmployeeFilter && employeeVal && !entry.isTimerRunning) {
                    const cleanWorker = employeeVal.includes('@') ? employeeVal.split('@')[0] : employeeVal;
                    const workerTokens = cleanWorker.split(/[\s._-]+/).filter(w => w.length > 1);
                    const matchEmp = !emp || emp.includes(cleanWorker) || cleanWorker.includes(emp) || 
                                     (workerTokens.length > 0 && workerTokens.some(w => emp.includes(w)));
                    if (!matchEmp) return;
                }

                // Filtro por proyecto del sidebar
                const pIdStr = String(entry.projectId || '');
                const pNameLower = (entry.projectName || '').toLowerCase();
                if (targetProjectId && targetProjectId !== '0' && !entry.isTimerRunning) {
                    if (pIdStr !== String(targetProjectId)) {
                        if (!targetProjectName || !pNameLower.includes(targetProjectName)) return;
                    }
                } else if (targetProjectName && !entry.isTimerRunning) {
                    if (!pNameLower.includes(targetProjectName)) return;
                }

                // Filtro por buscador (soporte multitoken para palabras compuestas)
                if (searchVal) {
                    const descLower = (entry.desc || '').toLowerCase();
                    const taskLower = (entry.taskName || '').toLowerCase();
                    const projLower = (entry.projectName || '').toLowerCase();
                    let partnerName = '';
                    if (entry.partnerId && window.partnersMap && window.partnersMap[entry.partnerId]) {
                        partnerName = (window.partnersMap[entry.partnerId].name || '').toLowerCase();
                    }
                    const combined = `${descLower} ${taskLower} ${projLower} ${partnerName}`;
                    const tokens = searchVal.split(/\s+/).filter(Boolean);
                    const matchesAll = tokens.every(token => combined.includes(token));
                    if (!matchesAll && !entry.isTimerRunning) return;
                }

                // En la botonera deben aparecer los partes de trabajo en los que se han imputado horas (hours > 0 o timer corriendo)
                if ((entry.hours || 0) <= 0 && !entry.isTimerRunning) {
                    return;
                }

                const pId = entry.projectId || 0;
                const tId = entry.taskId || 0;
                const descClean = (entry.desc || '').trim();
                const descKey = descClean.toLowerCase();
                if (!pId) return;

                // Botonera: agrupamos por Proyecto + Tarea + Descripción de la tarea realizada
                // Cada parte de trabajo con descripción realizada constituye su propia tecla
                const key = `${pId}_${tId}_${descKey}`;
                const isToday = (entry.date === todayStr);
                const isYesterday = isYesterdayDate(entry.date, today);

                if (!map.has(key)) {
                    map.set(key, {
                        key: key,
                        projectId: pId,
                        projectName: entry.projectName || `Proyecto #${pId}`,
                        partnerId: entry.partnerId || 0,
                        taskId: tId,
                        taskName: (entry.taskName && entry.taskName.trim()) ? entry.taskName.trim() : '',
                        lastDate: entry.date || '',
                        lastDescription: descClean,
                        lastTimesheetId: entry.id,
                        totalHours: 0,
                        todayTimesheetId: isToday ? entry.id : null,
                        todayHours: 0,
                        yesterdayHours: 0,
                        hasTodayEntry: isToday,
                        hasYesterdayEntry: isYesterday,
                        hasRunningTimer: Boolean(entry.isTimerRunning)
                    });
                }

                const item = map.get(key);
                item.totalHours += (entry.hours || 0);
                if (entry.isTimerRunning) {
                    item.hasRunningTimer = true;
                }
                if (entry.partnerId && !item.partnerId) {
                    item.partnerId = entry.partnerId;
                }

                if (isToday) {
                    item.hasTodayEntry = true;
                    if (!item.todayTimesheetId) item.todayTimesheetId = entry.id;
                    item.todayHours += (entry.hours || 0);
                } else if (isYesterday) {
                    item.hasYesterdayEntry = true;
                    item.yesterdayHours += (entry.hours || 0);
                }

                // Mantener siempre la referencia al parte de trabajo más reciente
                if (entry.date > item.lastDate || (entry.date === item.lastDate && (parseInt(entry.id, 10) || 0) > (parseInt(item.lastTimesheetId, 10) || 0))) {
                    item.lastDate = entry.date;
                    item.lastTimesheetId = entry.id;
                    if (descClean) item.lastDescription = descClean;
                }
            });
            return map;
        }

        let taskMap = buildTaskMap(true);
        // Si filtrar por empleado dejó 0 resultados pero hay datos generales, relajar el filtro
        if (taskMap.size === 0 && allEntries.length > 0 && employeeVal && !searchVal) {
            taskMap = buildTaskMap(false);
        }

        // Recopilar todos los cronómetros activos (principal + concurrentes en memoria y servidor)
        const activeTimersMap = new Map();
        if (timerState && (timerState.status === 'running' || timerState.status === 'paused')) {
            const key = timerState.timesheetId ? String(timerState.timesheetId) : `${timerState.projectId}_${timerState.taskId || 0}_${(timerState.description || '').trim().toLowerCase()}`;
            activeTimersMap.set(key, timerState);
        }
        if (window.__activeTimersMap) {
            for (const [id, t] of window.__activeTimersMap.entries()) {
                const key = id ? String(id) : `${t.projectId}_${t.taskId || 0}_${(t.description || '').trim().toLowerCase()}`;
                if (!activeTimersMap.has(key)) {
                    activeTimersMap.set(key, t);
                }
            }
        }
        if (Array.isArray(window.__activeTimersList)) {
            for (const t of window.__activeTimersList) {
                const key = t.timesheet_id ? String(t.timesheet_id) : `${t.project_id}_${t.task_id || 0}_${(t.description || '').trim().toLowerCase()}`;
                if (!activeTimersMap.has(key)) {
                    activeTimersMap.set(key, {
                        timesheetId: t.timesheet_id,
                        projectId: t.project_id,
                        projectName: t.project_name || `Proyecto #${t.project_id}`,
                        taskId: t.task_id || 0,
                        taskName: t.task_name || '',
                        description: t.description || '',
                        status: t.is_running ? 'running' : 'paused',
                        startedAt: t.started_at ? (t.started_at > 1e11 ? t.started_at : t.started_at * 1000) : Date.now(),
                        lastStartTime: Date.now(),
                        accumulatedMs: t.accumulated_ms || Math.round((t.unit_amount || 0) * 3600 * 1000),
                        unitAmount: t.unit_amount,
                        source: t.source || '',
                        isAntigravity: Boolean(t.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(t.task_name, t.description)))
                    });
                }
            }
        }

        // Asegurar que todas las tareas activas estén en taskMap
        for (const act of activeTimersMap.values()) {
            if (act.status !== 'running' && act.status !== 'paused') continue;
            const actDescClean = (act.description || '').trim();
            const pId = act.projectId || 0;
            const tId = act.taskId || 0;
            if (!pId) continue;
            const activeKey = `${pId}_${tId}_${actDescClean.toLowerCase()}`;
            const isAgy = Boolean(act.isAntigravity || act.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(act.taskName, act.description)));

            if (!taskMap.has(activeKey)) {
                let partnerId = 0;
                if (window.projectPartnerMap && window.projectPartnerMap[pId]) {
                    partnerId = window.projectPartnerMap[pId];
                }
                taskMap.set(activeKey, {
                    key: activeKey,
                    projectId: pId,
                    projectName: act.projectName || `Proyecto #${pId}`,
                    partnerId: partnerId,
                    taskId: tId,
                    taskName: (act.taskName && act.taskName.trim()) ? act.taskName.trim() : '',
                    lastDate: act.date || todayStr,
                    lastDescription: actDescClean,
                    lastTimesheetId: act.timesheetId,
                    totalHours: 0,
                    todayTimesheetId: act.timesheetId,
                    todayHours: (act.unitAmount || 0),
                    yesterdayHours: 0,
                    hasTodayEntry: true,
                    hasYesterdayEntry: false,
                    hasRunningTimer: (act.status === 'running'),
                    isAntigravity: isAgy
                });
            } else {
                const activeItem = taskMap.get(activeKey);
                activeItem.todayTimesheetId = act.timesheetId || activeItem.todayTimesheetId;
                activeItem.hasTodayEntry = true;
                if (act.status === 'running') {
                    activeItem.hasRunningTimer = true;
                }
                activeItem.lastDate = todayStr;
                if (actDescClean) activeItem.lastDescription = actDescClean;
                if (isAgy) activeItem.isAntigravity = true;
            }
        }

    let tasks = Array.from(taskMap.values());

    // Marcar si está corriendo (coincidencia con CUALQUIER cronómetro activo)
    tasks.forEach(t => {
        let isRunning = false;
        let matchedActiveTimer = null;
        const tDesc = (t.lastDescription || '').trim().toLowerCase();

        for (const act of activeTimersMap.values()) {
            if (act.status !== 'running') continue;
            const actDesc = (act.description || '').trim().toLowerCase();
            const descMatches = (!actDesc && !tDesc) || (actDesc === tDesc);

            const matchesId = (t.todayTimesheetId && act.timesheetId && String(t.todayTimesheetId) === String(act.timesheetId)) ||
                              (t.lastTimesheetId && act.timesheetId && String(t.lastTimesheetId) === String(act.timesheetId));
            const matchesTaskDesc = (t.taskId && act.taskId && t.taskId === act.taskId && descMatches);
            const matchesProjDesc = (t.projectId === act.projectId && descMatches);

            if (matchesId || matchesTaskDesc || matchesProjDesc) {
                isRunning = true;
                matchedActiveTimer = act;
                break;
            }
        }

        t.isRunning = isRunning;
        t.activeTimer = matchedActiveTimer;
        if (matchedActiveTimer) {
            const isAgy = Boolean(matchedActiveTimer.isAntigravity || matchedActiveTimer.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(matchedActiveTimer.taskName, matchedActiveTimer.description)));
            if (isAgy) t.isAntigravity = true;
        } else if (typeof isAntigravityTask === 'function' && isAntigravityTask(t.taskName, t.lastDescription)) {
            t.isAntigravity = true;
        }
    });

    // Ordenación y filtrado:
    // 1. En primer lugar saldrán TODAS las tarjetas que están en ejecución (runningTasks)
    // 2. A continuación el resto de tarjetas asociadas a los 15 últimos partes de hora
    const runningTasks = tasks.filter(t => t.isRunning);
    const nonRunningTasks = tasks.filter(t => !t.isRunning);

    // Ordenar runningTasks: humanas primero, luego Antigravity, y luego por ID más reciente
    runningTasks.sort((a, b) => {
        const aAgy = a.isAntigravity ? 1 : 0;
        const bAgy = b.isAntigravity ? 1 : 0;
        if (aAgy !== bAgy) return aAgy - bAgy;
        return (parseInt(b.lastTimesheetId, 10) || 0) - (parseInt(a.lastTimesheetId, 10) || 0);
    });

    nonRunningTasks.sort((a, b) => {
        // Orden estrictamente por fecha más reciente a menos reciente
        if (a.lastDate > b.lastDate) return -1;
        if (a.lastDate < b.lastDate) return 1;
        // Si coinciden en fecha, por ID de parte más reciente a menos reciente
        const aId = parseInt(a.lastTimesheetId, 10) || 0;
        const bId = parseInt(b.lastTimesheetId, 10) || 0;
        if (aId !== bId) return bId - aId;
        // Desempate por horas
        const aH = (a.todayHours || 0) + (a.yesterdayHours || 0) + (a.totalHours || 0);
        const bH = (b.todayHours || 0) + (b.yesterdayHours || 0) + (b.totalHours || 0);
        return bH - aH;
    });

    const top15NonRunning = nonRunningTasks.slice(0, 15);
    tasks = [...runningTasks, ...top15NonRunning];

    if (countBadge) {
        countBadge.textContent = `${tasks.length}`;
    }

    if (tasks.length === 0) {
        gridEl.innerHTML = '';
        if (emptyEl) emptyEl.classList.remove('hidden');
        return;
    }

    if (emptyEl) emptyEl.classList.add('hidden');

    function getItemDateBadgeHtml(item) {
        if (item.hasTodayEntry) {
            return `
                <span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] sm:text-[11px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-700/60 font-mono">
                    <span>HOY</span>
                    ${item.todayHours > 0 ? `<span>${item.todayHours.toFixed(1)}h</span>` : ''}
                </span>
            `;
        } else if (item.hasYesterdayEntry) {
            return `
                <span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] sm:text-[11px] font-bold bg-amber-950/80 text-amber-300 border border-amber-700/60 font-mono" title="Parte de ayer: ${item.lastDate}. Se duplicará para hoy al pulsar">
                    <span>AYER</span>
                    ${item.yesterdayHours > 0 ? `<span>${item.yesterdayHours.toFixed(1)}h</span>` : ''}
                </span>
            `;
        } else if (item.lastDate) {
            return `
                <span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] sm:text-[11px] font-medium bg-slate-800 text-slate-400 border border-slate-700 font-mono" title="Última fecha: ${item.lastDate}. Se duplicará para hoy al pulsar">
                    <span>${formatCardDate(item.lastDate)}</span>
                </span>
            `;
        }
        return '';
    }

    function getItemHoursSummaryHtml(item) {
        if (item.hasTodayEntry && item.todayHours > 0) {
            let txt = `<span class="text-emerald-400 font-bold">${item.todayHours.toFixed(2)}h hoy</span>`;
            if (item.yesterdayHours > 0) {
                txt += ` <span class="text-slate-400 text-xs">(${item.yesterdayHours.toFixed(1)}h ayer)</span>`;
            }
            return txt;
        } else if (item.hasYesterdayEntry && item.yesterdayHours > 0) {
            return `<span class="text-amber-300 font-bold">${item.yesterdayHours.toFixed(2)}h ayer</span>`;
        } else if (item.totalHours > 0) {
            return `<span class="text-slate-400">${item.totalHours.toFixed(1)}h</span>`;
        }
        return '';
    }

    // ACTUALIZACIÓN MINUCIOSA Y SILENCIOSA: Si es silent y las teclas coinciden, actualizar in-place
    if (isSilent && gridEl.children.length > 0) {
        const existingCards = Array.from(gridEl.querySelectorAll('.express-card'));
        const existingKeys = existingCards.map(c => c.dataset.cardKey).filter(Boolean);
        const newKeys = tasks.map(t => t.key);

        const isSameKeys = (existingKeys.length === newKeys.length) &&
                           existingKeys.every((k, idx) => k === newKeys[idx]);

        if (isSameKeys) {
            tasks.forEach((item, idx) => {
                const card = existingCards[idx];
                if (!card) return;

                card.dataset.timesheetId = item.lastTimesheetId || 0;
                card.dataset.todayTimesheetId = item.todayTimesheetId || 0;
                card.dataset.todayHours = item.todayHours || 0;
                card.dataset.yesterdayHours = item.yesterdayHours || 0;
                card.dataset.lastDate = item.lastDate || '';
                card.dataset.hasToday = item.hasTodayEntry ? 'true' : 'false';

                const badgeIdle = card.querySelector('.express-badge-idle');
                const newBadgeHtml = getItemDateBadgeHtml(item);
                if (badgeIdle && badgeIdle.innerHTML.trim() !== newBadgeHtml.trim()) {
                    badgeIdle.innerHTML = newBadgeHtml;
                }

                const hoursSummary = card.querySelector('.express-hours-summary');
                const newSummaryHtml = getItemHoursSummaryHtml(item);
                if (hoursSummary && hoursSummary.innerHTML.trim() !== newSummaryHtml.trim()) {
                    hoursSummary.innerHTML = newSummaryHtml;
                }
            });

            updateExpressTimerState();
            return;
        }
    }

    let liveClockStr = '00:00:00';
    if (timerState && timerState.status === 'running' && timerState.lastStartTime) {
        const totalMs = (timerState.accumulatedMs || 0) + (Date.now() - timerState.lastStartTime);
        if (typeof formatElapsedMs === 'function') {
            liveClockStr = formatElapsedMs(totalMs);
        }
    }

    let cardsHtml = '';
    tasks.forEach(item => {
        const hasTask = item.taskName && item.taskName.trim().length > 0;
        const hasDesc = item.lastDescription && item.lastDescription.trim().length > 0;
        const safeProj = escapeHtml(item.projectName);
        const safeTask = hasTask ? escapeHtml(item.taskName) : '';
        // Lo que identifica la tecla es el nombre del proyecto y la descripción de la tarea realizada
        const safeDesc = hasDesc ? escapeHtml(item.lastDescription) : (hasTask ? safeTask : 'Trabajo general');

        const dateBadgeHtml = getItemDateBadgeHtml(item);

        const partnerId = item.partnerId || (window.projectPartnerMap && window.projectPartnerMap[item.projectId]) || 0;
        const partnerLogoHtml = (partnerId > 0 || item.projectId > 0)
            ? `<span class="inline-flex items-center justify-center w-7 h-7 sm:w-8 sm:h-8 rounded-lg bg-white/95 p-0.5 shadow-sm border border-slate-600/50 shrink-0 overflow-hidden"><img src="/api/partner/avatar?id=${partnerId}&project_id=${item.projectId || 0}" alt="" class="w-full h-full object-contain" loading="lazy" onerror="this.parentElement.remove()"></span>`
            : '';

        const isRunning = Boolean(item.isRunning);
        const isAgy = Boolean(item.isAntigravity);

        let cardBorderClass = 'border-slate-700/80 hover:border-slate-500 shadow-sm bg-slate-800/90 hover:bg-slate-750 text-slate-100';
        if (isRunning) {
            if (isAgy) {
                cardBorderClass = 'express-card-running express-card-running-agy border-purple-500 text-purple-100 shadow-lg';
            } else {
                cardBorderClass = 'express-card-running border-emerald-400 text-emerald-100 shadow-lg';
            }
        }

        // Reloj individual por tarjeta
        let itemClockStr = '00:00:00';
        if (isRunning) {
            if (item.activeTimer) {
                const act = item.activeTimer;
                const actAccum = (typeof act.accumulatedMs === 'number' && act.accumulatedMs >= 0)
                    ? act.accumulatedMs
                    : Math.round((act.unitAmount || 0) * 3600 * 1000);
                const actStart = act.lastStartTime || act.startedAt || Date.now();
                const itemTotalMs = actAccum + (Date.now() - actStart);
                itemClockStr = (typeof formatElapsedMs === 'function') ? formatElapsedMs(Math.max(0, itemTotalMs)) : '00:00:00';
            } else {
                itemClockStr = liveClockStr;
            }
        }

        cardsHtml += `
            <div class="express-card group relative rounded-xl border ${cardBorderClass} p-3 sm:p-3.5 transition-all duration-100 flex flex-col justify-between cursor-pointer select-none active:scale-[0.98]"
                 role="button" tabindex="0"
                 data-card-key="${escapeAttr(item.key)}"
                 data-project-id="${item.projectId}"
                 data-project-name="${escapeAttr(item.projectName)}"
                 data-task-id="${item.taskId || 0}"
                 data-task-name="${escapeAttr(item.taskName || '')}"
                 data-description="${escapeAttr(item.lastDescription || '')}"
                 data-timesheet-id="${item.lastTimesheetId || 0}"
                 data-today-timesheet-id="${item.todayTimesheetId || 0}"
                 data-today-hours="${item.todayHours || 0}"
                 data-yesterday-hours="${item.yesterdayHours || 0}"
                 data-last-date="${item.lastDate || ''}"
                 data-has-today="${item.hasTodayEntry ? 'true' : 'false'}"
                 data-is-antigravity="${isAgy ? 'true' : 'false'}"
                 onclick="handleExpressCardClick(this, false)"
                 ondblclick="handleExpressCardClick(this, true)"
                 onkeydown="if(event.key === 'Enter' || event.key === ' ') { event.preventDefault(); handleExpressCardClick(this, true); }">
                
                <!-- Encabezado de la tecla: Logotipo Partner, Nombre del Proyecto y Estado -->
                <div class="flex items-center justify-between gap-2 mb-2">
                    <div class="flex items-center space-x-2 min-w-0">
                        ${partnerLogoHtml}
                        <div class="min-w-0 flex items-center space-x-1.5">
                            <span class="w-2 h-2 rounded-full ${isRunning ? (isAgy ? 'bg-purple-400 animate-pulse' : 'bg-emerald-400') : 'bg-sky-400'} shrink-0"></span>
                            <span class="text-xs sm:text-[13px] uppercase font-black tracking-wider text-sky-400 truncate" title="${safeProj}">
                                ${safeProj}
                            </span>
                        </div>
                    </div>
                    <div class="shrink-0 flex items-center space-x-1">
                        <span class="express-badge-running-agy ${isRunning && isAgy ? '' : 'hidden'} inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] sm:text-[11px] font-bold bg-purple-500/25 text-purple-200 border border-purple-500/50">
                            <span class="w-1.5 h-1.5 rounded-full bg-purple-400 animate-pulse"></span>
                            <span>🤖 AGY</span>
                        </span>
                        <span class="express-badge-running ${isRunning && !isAgy ? '' : 'hidden'} inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] sm:text-[11px] font-bold bg-emerald-500/20 text-emerald-300 border border-emerald-500/40 animate-pulse">
                            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                            <span>ACTIVO</span>
                        </span>
                        <span class="express-badge-idle ${isRunning ? 'hidden' : ''}">
                            ${dateBadgeHtml}
                        </span>
                    </div>
                </div>

                <!-- Cuerpo de la tecla: MÁXIMA IMPORTANCIA A LA DESCRIPCIÓN DE LA TAREA REALIZADA -->
                <div class="mb-2.5 space-y-1">
                    <h4 class="express-card-title text-sm sm:text-base font-bold text-slate-100 group-hover:text-amber-300 leading-snug line-clamp-2 transition-colors" title="${safeDesc}">
                        ${safeDesc}
                    </h4>
                    ${hasTask ? `
                    <div class="flex items-center space-x-1 text-xs text-slate-400 truncate pt-0.5" title="Tarea: ${safeTask}">
                        <span class="text-slate-500 font-normal">Tarea:</span>
                        <span class="font-medium text-slate-300 truncate">${safeTask}</span>
                    </div>
                    ` : ''}
                </div>

                <!-- Pie de la tecla: Reloj / Horas y Botón de Acción -->
                <div class="pt-2 border-t border-slate-700/60 flex items-center justify-between gap-1 mt-auto">
                    <div class="min-w-0">
                        <div class="express-live-clock-container ${isRunning ? '' : 'hidden'} inline-flex items-center space-x-1.5 ${isAgy ? 'text-purple-300' : 'text-emerald-400'} font-mono font-bold text-xs sm:text-sm">
                            <span class="w-2 h-2 rounded-full ${isAgy ? 'bg-purple-400' : 'bg-emerald-400'} animate-ping"></span>
                            <span class="express-live-clock font-mono">${itemClockStr}</span>
                        </div>
                        <div class="express-hours-summary ${isRunning ? 'hidden' : ''} text-xs sm:text-sm text-slate-400 font-mono">
                            ${getItemHoursSummaryHtml(item)}
                        </div>
                    </div>

                    <div class="shrink-0">
                        <span class="express-btn-action inline-flex items-center px-2.5 py-1 rounded-lg text-xs font-extrabold tracking-wide uppercase transition ${isRunning ? (isAgy ? 'bg-purple-600 hover:bg-purple-500 text-white font-black' : 'bg-amber-500 hover:bg-amber-400 text-slate-950 font-black') : 'bg-slate-700/80 hover:bg-sky-600 text-slate-200 hover:text-white'}">
                            ${isRunning ? 'PAUSAR' : (item.hasTodayEntry ? 'REANUDAR' : 'INICIAR')}
                        </span>
                    </div>
                </div>
            </div>
        `;
    });

    gridEl.innerHTML = cardsHtml;
    } catch (err) {
        console.error('[PlanesGo Express] Error renderizando vista:', err);
    }
}

// Manejo de eventos táctiles en móviles para evitar disparos accidentales al hacer scroll
let __expressTouchStartX = 0;
let __expressTouchStartY = 0;
let __expressTouchMoved = false;

if (typeof window !== 'undefined' && !window.__expressTouchListenersAttached) {
    window.__expressTouchListenersAttached = true;
    window.addEventListener('touchstart', (e) => {
        if (e.touches && e.touches.length > 0) {
            __expressTouchStartX = e.touches[0].clientX;
            __expressTouchStartY = e.touches[0].clientY;
            __expressTouchMoved = false;
        }
    }, { passive: true });

    window.addEventListener('touchmove', (e) => {
        if (e.touches && e.touches.length > 0) {
            const dx = Math.abs(e.touches[0].clientX - __expressTouchStartX);
            const dy = Math.abs(e.touches[0].clientY - __expressTouchStartY);
            if (dx > 10 || dy > 10) {
                __expressTouchMoved = true;
            }
        }
    }, { passive: true });
}

function resetExpressCardConfirm(cardEl) {
    if (!cardEl) return;
    if (cardEl.__confirmTimeout) {
        clearTimeout(cardEl.__confirmTimeout);
        cardEl.__confirmTimeout = null;
    }
    cardEl.classList.remove('express-card-confirming');
    updateExpressTimerState();
}

function resetAllExpressCardConfirms() {
    document.querySelectorAll('.express-card.express-card-confirming').forEach(c => {
        resetExpressCardConfirm(c);
    });
}

/**
 * Gestiona el click o doble toque en una tarjeta de la botonera Express.
 * Requiere confirmación (doble clic o doble pulsación en 1.8s) para evitar
 * pausas o reanudaciones accidentales con el dedo o ratón.
 * Soporta pausar tanto el cronómetro principal como cronómetros concurrentes de Antigravity.
 */
function handleExpressCardClick(cardEl, isDirectDoubleClick = false) {
    if (!cardEl) return;

    // Si el usuario estaba desplazándose en pantalla táctil, ignorar el toque
    if (__expressTouchMoved) {
        __expressTouchMoved = false;
        return;
    }

    const pId = parseInt(cardEl.dataset.projectId, 10) || 0;
    const pName = cardEl.dataset.projectName || '';
    const tId = parseInt(cardEl.dataset.taskId, 10) || 0;
    const tName = cardEl.dataset.taskName || '';
    const desc = cardEl.dataset.description || '';
    const tsId = parseInt(cardEl.dataset.timesheetId, 10) || 0;
    const todayTsId = parseInt(cardEl.dataset.todayTimesheetId, 10) || 0;
    const todayHours = parseFloat(cardEl.dataset.todayHours) || 0;
    const hasToday = (cardEl.dataset.hasToday === 'true') || Boolean(todayTsId);

    const current = (typeof getTimerState === 'function') ? getTimerState() : null;
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(new Date()) : new Date().toISOString().split('T')[0];

    // Determinar si esta tarjeta está en ejecución y qué temporizador le corresponde
    let matchedRunningTimer = null;
    let isPrimaryRunning = false;

    if (current && current.status === 'running') {
        const cardDesc = (desc || '').trim().toLowerCase();
        const currentDesc = (current.description || '').trim().toLowerCase();
        const descMatches = (!currentDesc && !cardDesc) || (currentDesc === cardDesc);

        if (current.timesheetId && (current.timesheetId === todayTsId || current.timesheetId === tsId)) {
            matchedRunningTimer = current;
            isPrimaryRunning = true;
        } else if (tId && current.taskId && current.taskId === tId && descMatches) {
            matchedRunningTimer = current;
            isPrimaryRunning = true;
        } else if (!tId && !current.taskId && current.projectId === pId && descMatches) {
            matchedRunningTimer = current;
            isPrimaryRunning = true;
        } else if (current.projectId === pId && descMatches) {
            matchedRunningTimer = current;
            isPrimaryRunning = true;
        }
    }

    if (!matchedRunningTimer && window.__activeTimersMap) {
        for (const [id, t] of window.__activeTimersMap.entries()) {
            if (t.status !== 'running') continue;
            const tDesc = (t.description || '').trim().toLowerCase();
            const cardDesc = (desc || '').trim().toLowerCase();
            const descMatches = (!tDesc && !cardDesc) || (tDesc === cardDesc);
            if ((id && (String(id) === String(todayTsId) || String(id) === String(tsId))) ||
                (tId && t.taskId && t.taskId === tId && descMatches) ||
                (pId && t.projectId === pId && descMatches)) {
                matchedRunningTimer = t;
                break;
            }
        }
    }

    const isRunning = Boolean(matchedRunningTimer);
    const isConfirming = cardEl.classList.contains('express-card-confirming');

    // MECANISMO DE SEGURIDAD (DOBLE CLIC / DOBLE PULSACIÓN):
    // Si no es un doble clic directo nativo y la tarjeta no estaba en modo confirmación,
    // activamos el estado de confirmación durante 1.8 segundos y solicitamos el segundo toque.
    if (!isDirectDoubleClick && !isConfirming) {
        resetAllExpressCardConfirms();
        cardEl.classList.add('express-card-confirming');
        const btnAction = cardEl.querySelector('.express-btn-action');
        if (btnAction) {
            btnAction.textContent = isRunning ? '¿PAUSAR? (Toca de nuevo)' : (hasToday ? '¿REANUDAR? (Toca de nuevo)' : '¿INICIAR? (Toca de nuevo)');
            btnAction.className = 'express-btn-action inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-black tracking-wide uppercase transition bg-amber-400 text-slate-950 shadow-md animate-pulse';
        }
        cardEl.__confirmTimeout = setTimeout(() => {
            resetExpressCardConfirm(cardEl);
        }, 1800);
        return;
    }

    // Si llegamos aquí, la acción está confirmada (segundo toque o dblclick directo)
    if (cardEl.__confirmTimeout) {
        clearTimeout(cardEl.__confirmTimeout);
        cardEl.__confirmTimeout = null;
    }
    cardEl.classList.remove('express-card-confirming');

    if (isRunning) {
        if (isPrimaryRunning) {
            if (typeof togglePauseTimer === 'function') {
                togglePauseTimer();
            }
        } else if (matchedRunningTimer) {
            // Pausar temporizador concurrente o de Antigravity en backend
            const timerTsId = matchedRunningTimer.timesheetId || todayTsId || tsId;
            const accumMs = (typeof matchedRunningTimer.accumulatedMs === 'number' && matchedRunningTimer.accumulatedMs >= 0)
                ? matchedRunningTimer.accumulatedMs
                : Math.round((matchedRunningTimer.unitAmount || 0) * 3600 * 1000);
            const startMs = matchedRunningTimer.lastStartTime || matchedRunningTimer.startedAt || Date.now();
            const totalMs = accumMs + (Date.now() - startMs);
            const totalHours = Math.max(0, totalMs / (3600 * 1000));

            matchedRunningTimer.status = 'paused';
            matchedRunningTimer.accumulatedMs = totalMs;
            matchedRunningTimer.unitAmount = totalHours;

            fetch('/api/timer/pause', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    timesheet_id: timerTsId,
                    task_id: matchedRunningTimer.taskId || tId || null,
                    unit_amount: totalHours
                })
            }).catch(e => console.warn('[Express] Error pausando tarea concurrente:', e))
              .finally(() => {
                  if (typeof loadExpressTimesheets === 'function') {
                      loadExpressTimesheets(true, true);
                  }
              });
        }

        window.__lastTimerActionTime = Date.now();
        try {
            localStorage.setItem('planesgo_timer_action', JSON.stringify({ action: 'pause', ts: Date.now() }));
        } catch (e) {}
        updateExpressTimerState();
        return;
    }

    // Si no está corriendo, iniciar o reanudar
    window.__lastTimerActionTime = Date.now();

    if (!hasToday) {
        // La fecha es anterior a hoy: duplicar para hoy e iniciar sin modales ni alertas
        const initialDesc = desc || (tName ? tName : 'Trabajo en curso');
        if (typeof startWorkTimer === 'function') {
            startWorkTimer(
                pId,
                pName,
                tId || null,
                tName,
                initialDesc,
                null, // null fuerza creación de nueva línea para hoy
                0,    // 0 horas acumuladas
                todayStr,
                true, // fromModal = true: NO abre modal ni ventana de detalle
                true  // silent = true: NO mostrar mensaje de inicio (solicitud explícita)
            );
        }
    } else {
        // Ya tiene imputación para hoy: reanudarla sin modales ni alertas
        const targetTsId = todayTsId || tsId;
        const accumMs = Math.round(todayHours * 3600 * 1000);
        if (typeof startWorkTimer === 'function') {
            startWorkTimer(
                pId,
                pName,
                tId || null,
                tName,
                desc,
                targetTsId,
                accumMs,
                todayStr,
                true, // fromModal = true: NO abre modal ni ventana de detalle
                true  // silent = true: NO mostrar mensaje de inicio (solicitud explícita)
            );
        }
    }

    // Sincronización entre pestañas / ventanas
    try {
        localStorage.setItem('planesgo_timer_action', JSON.stringify({ action: 'start', ts: Date.now() }));
    } catch (e) {}

    setTimeout(() => {
        updateExpressTimerState();
    }, 50);
}

/**
 * Sincroniza en tiempo real el reloj y las clases activas de la botonera Express.
 * Soporta múltiples temporizadores concurrentes en paralelo (Antigravity y servidor).
 */
function updateExpressTimerState() {
    const current = (typeof getTimerState === 'function') ? getTimerState() : null;
    const isPrimaryRunning = current && current.status === 'running';
    const primaryTotalMs = current ? ((current.accumulatedMs || 0) + (isPrimaryRunning && current.lastStartTime ? (Date.now() - current.lastStartTime) : 0)) : 0;
    const formattedClock = (typeof formatElapsedMs === 'function') ? formatElapsedMs(primaryTotalMs) : '00:00:00';

    // Recopilar mapa de todos los cronómetros activos (principal + concurrentes)
    const activeMap = new Map();
    if (current && (current.status === 'running' || current.status === 'paused')) {
        const key = current.timesheetId ? String(current.timesheetId) : `${current.projectId}_${current.taskId || 0}_${(current.description || '').trim().toLowerCase()}`;
        activeMap.set(key, current);
    }
    if (window.__activeTimersMap) {
        for (const [id, t] of window.__activeTimersMap.entries()) {
            const key = id ? String(id) : `${t.projectId}_${t.taskId || 0}_${(t.description || '').trim().toLowerCase()}`;
            if (!activeMap.has(key)) {
                activeMap.set(key, t);
            }
        }
    }
    if (Array.isArray(window.__activeTimersList)) {
        for (const t of window.__activeTimersList) {
            const key = t.timesheet_id ? String(t.timesheet_id) : `${t.project_id}_${t.task_id || 0}_${(t.description || '').trim().toLowerCase()}`;
            if (!activeMap.has(key)) {
                activeMap.set(key, {
                    timesheetId: t.timesheet_id,
                    projectId: t.project_id,
                    taskId: t.task_id || 0,
                    description: t.description || '',
                    status: t.is_running ? 'running' : 'paused',
                    startedAt: t.started_at ? (t.started_at > 1e11 ? t.started_at : t.started_at * 1000) : Date.now(),
                    lastStartTime: Date.now(),
                    accumulatedMs: t.accumulated_ms || Math.round((t.unit_amount || 0) * 3600 * 1000),
                    unitAmount: t.unit_amount,
                    source: t.source || '',
                    isAntigravity: Boolean(t.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(t.task_name, t.description)))
                });
            }
        }
    }

    document.querySelectorAll('.express-card').forEach(card => {
        const pId = parseInt(card.dataset.projectId, 10) || 0;
        const tId = parseInt(card.dataset.taskId, 10) || 0;
        const tsId = parseInt(card.dataset.timesheetId, 10) || 0;
        const todayTsId = parseInt(card.dataset.todayTimesheetId, 10) || 0;
        const cardDesc = (card.dataset.description || '').trim().toLowerCase();

        let matchedTimer = null;
        for (const act of activeMap.values()) {
            const actDesc = (act.description || '').trim().toLowerCase();
            const descMatches = (!actDesc && !cardDesc) || (actDesc === cardDesc);
            const matchesId = (todayTsId && act.timesheetId && String(todayTsId) === String(act.timesheetId)) ||
                              (tsId && act.timesheetId && String(tsId) === String(act.timesheetId));
            const matchesTaskDesc = (tId && act.taskId && tId === act.taskId && descMatches);
            const matchesProjDesc = (pId && act.projectId === pId && descMatches);

            if (matchesId || matchesTaskDesc || matchesProjDesc) {
                matchedTimer = act;
                break;
            }
        }

        const isRunning = matchedTimer && matchedTimer.status === 'running';
        const isPaused = matchedTimer && matchedTimer.status === 'paused';
        const isAgy = Boolean(
            card.dataset.isAntigravity === 'true' ||
            (matchedTimer && (matchedTimer.isAntigravity || matchedTimer.source === 'antigravity' || (typeof isAntigravityTask === 'function' && isAntigravityTask(matchedTimer.taskName, matchedTimer.description))))
        );

        // Reloj individual para la tarjeta
        let cardClockStr = formattedClock;
        if (matchedTimer && isRunning) {
            const actAccum = (typeof matchedTimer.accumulatedMs === 'number' && matchedTimer.accumulatedMs >= 0)
                ? matchedTimer.accumulatedMs
                : Math.round((matchedTimer.unitAmount || 0) * 3600 * 1000);
            const actStart = matchedTimer.lastStartTime || matchedTimer.startedAt || Date.now();
            const totalMs = actAccum + (Date.now() - actStart);
            if (typeof formatElapsedMs === 'function') {
                cardClockStr = formatElapsedMs(Math.max(0, totalMs));
            }
        }

        const clockEl = card.querySelector('.express-live-clock');
        const clockContainer = card.querySelector('.express-live-clock-container');
        const hoursSummary = card.querySelector('.express-hours-summary');
        const badgeRunningAgy = card.querySelector('.express-badge-running-agy');
        const badgeRunning = card.querySelector('.express-badge-running');
        const badgeIdle = card.querySelector('.express-badge-idle');
        const btnAction = card.querySelector('.express-btn-action');
        const isConfirming = card.classList.contains('express-card-confirming');

        if (isRunning) {
            if (isAgy) {
                card.classList.add('express-card-running', 'express-card-running-agy', 'border-purple-500', 'text-purple-100');
                card.classList.remove('border-emerald-400', 'text-emerald-100', 'express-card-paused', 'border-slate-700/80', 'bg-slate-800/90', 'text-slate-100', 'border-amber-500/80', 'bg-amber-950/30', 'text-amber-100');
                if (badgeRunningAgy) badgeRunningAgy.classList.remove('hidden');
                if (badgeRunning) badgeRunning.classList.add('hidden');
            } else {
                card.classList.add('express-card-running', 'border-emerald-400', 'text-emerald-100');
                card.classList.remove('express-card-running-agy', 'border-purple-500', 'text-purple-100', 'express-card-paused', 'border-slate-700/80', 'bg-slate-800/90', 'text-slate-100', 'border-amber-500/80', 'bg-amber-950/30', 'text-amber-100');
                if (badgeRunningAgy) badgeRunningAgy.classList.add('hidden');
                if (badgeRunning) badgeRunning.classList.remove('hidden');
            }

            if (clockEl) clockEl.textContent = cardClockStr;
            if (clockContainer) {
                clockContainer.classList.remove('hidden');
                if (isAgy) {
                    clockContainer.classList.add('text-purple-300');
                    clockContainer.classList.remove('text-emerald-400');
                } else {
                    clockContainer.classList.add('text-emerald-400');
                    clockContainer.classList.remove('text-purple-300');
                }
            }
            if (hoursSummary) hoursSummary.classList.add('hidden');
            if (badgeIdle) badgeIdle.classList.add('hidden');

            if (btnAction && !isConfirming) {
                btnAction.className = isAgy
                    ? 'express-btn-action inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-black tracking-wide uppercase transition bg-purple-600 hover:bg-purple-500 text-white'
                    : 'express-btn-action inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-black tracking-wide uppercase transition bg-amber-500 hover:bg-amber-400 text-slate-950';
                btnAction.textContent = 'PAUSAR';
            }
        } else if (isPaused) {
            card.classList.add('express-card-paused', 'border-amber-500/80', 'text-amber-100');
            card.classList.remove('express-card-running', 'express-card-running-agy', 'border-purple-500', 'border-emerald-400', 'border-slate-700/80', 'bg-slate-800/90');

            if (clockEl) clockEl.textContent = cardClockStr;
            if (clockContainer) clockContainer.classList.remove('hidden');
            if (hoursSummary) hoursSummary.classList.add('hidden');
            if (badgeRunningAgy) badgeRunningAgy.classList.add('hidden');
            if (badgeRunning) badgeRunning.classList.add('hidden');
            if (badgeIdle) badgeIdle.classList.remove('hidden');

            if (btnAction && !isConfirming) {
                btnAction.className = 'express-btn-action inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-bold tracking-wide uppercase transition bg-amber-500 hover:bg-amber-400 text-slate-950 font-extrabold';
                btnAction.textContent = 'REANUDAR';
            }
        } else {
            card.classList.remove('express-card-running', 'express-card-running-agy', 'express-card-paused', 'border-emerald-400', 'border-purple-500', 'border-amber-500/80', 'bg-amber-950/30', 'text-amber-100');
            card.classList.add('border-slate-700/80', 'bg-slate-800/90', 'text-slate-100');

            if (clockContainer) clockContainer.classList.add('hidden');
            if (hoursSummary) hoursSummary.classList.remove('hidden');
            if (badgeRunningAgy) badgeRunningAgy.classList.add('hidden');
            if (badgeRunning) badgeRunning.classList.add('hidden');
            if (badgeIdle) badgeIdle.classList.remove('hidden');

            if (btnAction && !isConfirming) {
                const hasToday = card.dataset.hasToday === 'true';
                btnAction.className = 'express-btn-action inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-bold tracking-wide uppercase transition bg-slate-700/80 hover:bg-sky-600 text-slate-200 hover:text-white';
                btnAction.textContent = hasToday ? 'REANUDAR' : 'INICIAR';
            }
        }
    });

    // Actualizar también tarjetas de tickets si están en pantalla
    updateTicketsTimerState(activeMap, current, formattedClock);
}

/**
 * Sincroniza en tiempo real el reloj y estado de las tarjetas de tickets.
 */
function updateTicketsTimerState(activeMap, current, formattedClock) {
    document.querySelectorAll('.ticket-card').forEach(card => {
        const ticketId = parseInt(card.dataset.ticketId, 10) || 0;
        let matchedTimer = null;
        if (ticketId && activeMap) {
            for (const act of activeMap.values()) {
                if (act.ticketId && parseInt(act.ticketId, 10) === ticketId) {
                    matchedTimer = act;
                    break;
                }
            }
        }
        if (!matchedTimer && current && current.ticketId && parseInt(current.ticketId, 10) === ticketId) {
            matchedTimer = current;
        }

        const isRunning = matchedTimer && matchedTimer.status === 'running';
        const isPaused = matchedTimer && matchedTimer.status === 'paused';

        const clockEl = card.querySelector('.ticket-live-clock');
        const playBtn = card.querySelector('.ticket-btn-play');
        const pauseBtn = card.querySelector('.ticket-btn-pause');
        const stopBtn = card.querySelector('.ticket-btn-stop');

        if (isRunning) {
            card.classList.add('ticket-card-running', 'border-amber-400');
            card.classList.remove('border-slate-800');
            if (clockEl) {
                let cardClockStr = formattedClock;
                if (matchedTimer) {
                    const actAccum = (typeof matchedTimer.accumulatedMs === 'number' && matchedTimer.accumulatedMs >= 0)
                        ? matchedTimer.accumulatedMs
                        : Math.round((matchedTimer.unitAmount || 0) * 3600 * 1000);
                    const actStart = matchedTimer.lastStartTime || matchedTimer.startedAt || Date.now();
                    const liveMs = actAccum + (Date.now() - actStart);
                    if (typeof formatElapsedMs === 'function') cardClockStr = formatElapsedMs(liveMs);
                }
                clockEl.textContent = cardClockStr;
                clockEl.classList.remove('hidden');
            }
            if (playBtn) playBtn.classList.add('hidden');
            if (pauseBtn) pauseBtn.classList.remove('hidden');
            if (stopBtn) stopBtn.classList.remove('hidden');
        } else if (isPaused) {
            card.classList.remove('ticket-card-running', 'border-slate-800');
            card.classList.add('border-amber-500/80');
            if (clockEl) clockEl.classList.remove('hidden');
            if (playBtn) playBtn.classList.remove('hidden');
            if (pauseBtn) pauseBtn.classList.add('hidden');
            if (stopBtn) stopBtn.classList.remove('hidden');
        } else {
            card.classList.remove('ticket-card-running', 'border-amber-400', 'border-amber-500/80');
            card.classList.add('border-slate-800');
            if (clockEl) clockEl.classList.add('hidden');
            if (playBtn) playBtn.classList.remove('hidden');
            if (pauseBtn) pauseBtn.classList.add('hidden');
            if (stopBtn) stopBtn.classList.add('hidden');
        }
    });
}

// ============================================================================
// PESTAÑA TICKETS PENDIENTES & GESTIÓN DE TICKETS HELPDESK
// ============================================================================

window.__activeExpressTab = 'express';
window.pendingTickets = [];
let __cachedProjectsForTickets = null;
let __cachedPartnersForTickets = null;

/**
 * Alterna entre la pestaña Express y la pestaña Tickets
 */
function switchExpressTab(tabName) {
    window.__activeExpressTab = tabName;

    const btnExpress = document.getElementById('tab-btn-express');
    const btnTickets = document.getElementById('tab-btn-tickets');
    const floatBtnExpress = document.getElementById('float-tab-btn-express');
    const floatBtnTickets = document.getElementById('float-tab-btn-tickets');

    const expressGrid = document.getElementById('express-grid-container');
    const expressLoading = document.getElementById('express-loading-state');
    const expressEmpty = document.getElementById('express-empty-state');

    const ticketsGrid = document.getElementById('tickets-grid-container');
    const ticketsLoading = document.getElementById('tickets-loading-state');
    const ticketsEmpty = document.getElementById('tickets-empty-state');

    const fabBtn = document.getElementById('fab-create-ticket');
    const subtitleInd = document.getElementById('tab-subtitle-indicator');
    const searchInput = document.getElementById('filter-search');

    if (tabName === 'tickets') {
        // Estilos pestaña activa: Tickets
        if (btnTickets) {
            btnTickets.className = 'px-3 py-1.5 rounded-lg text-xs font-bold transition-all flex items-center space-x-1.5 cursor-pointer bg-amber-600 text-white shadow-sm';
            btnTickets.setAttribute('aria-selected', 'true');
        }
        if (btnExpress) {
            btnExpress.className = 'px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 transition-all flex items-center space-x-1.5 cursor-pointer';
            btnExpress.setAttribute('aria-selected', 'false');
        }

        if (floatBtnTickets) {
            floatBtnTickets.className = 'px-2.5 py-1 rounded-md text-[11px] font-bold bg-amber-600 text-white shadow-xs transition flex items-center space-x-1 cursor-pointer';
        }
        if (floatBtnExpress) {
            floatBtnExpress.className = 'px-2.5 py-1 rounded-md text-[11px] font-medium text-slate-400 hover:text-slate-200 transition flex items-center space-x-1 cursor-pointer';
        }

        // Mostrar Tickets, ocultar Express
        if (expressGrid) expressGrid.classList.add('hidden');
        if (expressLoading) expressLoading.classList.add('hidden');
        if (expressEmpty) expressEmpty.classList.add('hidden');

        if (ticketsGrid) ticketsGrid.classList.remove('hidden');
        if (fabBtn) fabBtn.classList.remove('hidden');

        if (subtitleInd) subtitleInd.textContent = 'Tickets asignados';
        if (searchInput) searchInput.placeholder = 'Buscar por nº, título, proyecto, cliente...';

        try {
            document.title = 'Tickets de Soporte - PlanesGo';
            if (window.location.pathname.startsWith('/express') || window.location.pathname.startsWith('/m')) {
                const url = new URL(window.location);
                url.searchParams.set('tab', 'tickets');
                window.history.replaceState({ tab: 'tickets' }, '', url.toString());
            }
        } catch (e) {}

        loadTicketsView();
    } else {
        // Estilos pestaña activa: Express
        if (btnExpress) {
            btnExpress.className = 'px-3 py-1.5 rounded-lg text-xs font-bold transition-all flex items-center space-x-1.5 cursor-pointer bg-sky-600 text-white shadow-sm';
            btnExpress.setAttribute('aria-selected', 'true');
        }
        if (btnTickets) {
            btnTickets.className = 'px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 transition-all flex items-center space-x-1.5 cursor-pointer';
            btnTickets.setAttribute('aria-selected', 'false');
        }

        if (floatBtnExpress) {
            floatBtnExpress.className = 'px-2.5 py-1 rounded-md text-[11px] font-bold bg-sky-600 text-white shadow-xs transition flex items-center space-x-1 cursor-pointer';
        }
        if (floatBtnTickets) {
            floatBtnTickets.className = 'px-2.5 py-1 rounded-md text-[11px] font-medium text-slate-400 hover:text-slate-200 transition flex items-center space-x-1 cursor-pointer';
        }

        // Mostrar Express, ocultar Tickets
        if (ticketsGrid) ticketsGrid.classList.add('hidden');
        if (ticketsLoading) ticketsLoading.classList.add('hidden');
        if (ticketsEmpty) ticketsEmpty.classList.add('hidden');

        if (expressGrid) expressGrid.classList.remove('hidden');
        if (fabBtn) fabBtn.classList.add('hidden');

        if (subtitleInd) subtitleInd.textContent = 'Tareas recientes';
        if (searchInput) searchInput.placeholder = 'Buscar proyecto o tarea...';

        try {
            document.title = 'Botonera Express - PlanesGo';
            if (window.location.pathname.startsWith('/express') || window.location.pathname.startsWith('/m')) {
                const url = new URL(window.location);
                if (url.searchParams.has('tab')) {
                    url.searchParams.set('tab', 'express');
                    window.history.replaceState({ tab: 'express' }, '', url.toString());
                }
            }
        } catch (e) {}

        loadExpressTimesheets(false);
    }
}

/**
 * Recarga la pestaña que esté activa actualmente
 */
function reloadCurrentTab(force = true) {
    if (window.__activeExpressTab === 'tickets') {
        loadTicketsView(force);
    } else {
        loadExpressTimesheets(force);
    }
}

/**
 * Control del input de búsqueda con filtrado reactivo
 */
function handleExpressSearchInput(inputEl) {
    const val = inputEl ? inputEl.value : '';
    const clearBtn = document.getElementById('clear-search-btn');
    if (clearBtn) {
        clearBtn.classList.toggle('hidden', !val);
    }
    if (window.__activeExpressTab === 'tickets') {
        renderTicketsView();
    } else {
        renderExpressView();
    }
}

/**
 * Limpia el texto de búsqueda
 */
function clearExpressSearch() {
    const searchInput = document.getElementById('filter-search');
    if (searchInput) {
        searchInput.value = '';
    }
    const clearBtn = document.getElementById('clear-search-btn');
    if (clearBtn) clearBtn.classList.add('hidden');
    if (window.__activeExpressTab === 'tickets') {
        renderTicketsView();
    } else {
        renderExpressView();
    }
}

/**
 * Carga los tickets pendientes desde el backend
 */
let __isTicketsLoading = false;
async function loadTicketsView(forceReload = false) {
    if (__isTicketsLoading) return;
    __isTicketsLoading = true;

    const loadingEl = document.getElementById('tickets-loading-state');
    const emptyEl = document.getElementById('tickets-empty-state');
    const gridEl = document.getElementById('tickets-grid-container');

    if (!window.pendingTickets || window.pendingTickets.length === 0 || forceReload) {
        if (loadingEl) loadingEl.classList.remove('hidden');
        if (emptyEl) emptyEl.classList.add('hidden');
        if (gridEl) gridEl.classList.add('hidden');
    }

    try {
        const url = `/api/tickets${forceReload ? '?refresh=true' : ''}`;
        const res = await fetch(url, { cache: 'no-store' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const tickets = await res.json();
        window.pendingTickets = Array.isArray(tickets) ? tickets : [];

        // Actualizar contadores en badges
        const count = window.pendingTickets.length;
        const badge = document.getElementById('tickets-tab-count-badge');
        if (badge) badge.textContent = count;
        const floatBadge = document.getElementById('float-tickets-count-badge');
        if (floatBadge) floatBadge.textContent = count;

        renderTicketsView();
    } catch (err) {
        console.error('[PlanesGo Tickets] Error al cargar tickets:', err);
        if (typeof showToast === 'function') {
            showToast('⚠️ No se pudieron cargar los tickets de Helpdesk', 'error');
        }
    } finally {
        __isTicketsLoading = false;
        if (loadingEl) loadingEl.classList.add('hidden');
    }
}

/**
 * Renderiza las tarjetas de tickets con todos los datos y controles
 */
function renderTicketsView() {
    const gridEl = document.getElementById('tickets-grid-container');
    const emptyEl = document.getElementById('tickets-empty-state');
    if (!gridEl) return;

    const tickets = window.pendingTickets || [];
    const searchInput = document.getElementById('filter-search');
    const q = (searchInput ? searchInput.value : '').trim().toLowerCase();

    // Filtrar según búsqueda
    const filtered = tickets.filter(t => {
        if (!q) return true;
        const ref = (t.ticket_ref || String(t.id) || '').toLowerCase();
        const name = (t.name || '').toLowerCase();
        const desc = (t.description || '').toLowerCase();
        const proj = (t.project_id && t.project_id.name ? t.project_id.name : '').toLowerCase();
        const task = (t.task_id && t.task_id.name ? t.task_id.name : '').toLowerCase();
        const partner = (t.partner_id && t.partner_id.name ? t.partner_id.name : '').toLowerCase();
        return ref.includes(q) || name.includes(q) || desc.includes(q) || proj.includes(q) || task.includes(q) || partner.includes(q);
    });

    if (filtered.length === 0) {
        gridEl.innerHTML = '';
        gridEl.classList.add('hidden');
        if (emptyEl) emptyEl.classList.remove('hidden');
        return;
    }

    if (emptyEl) emptyEl.classList.add('hidden');
    gridEl.classList.remove('hidden');

    const currentTimer = (typeof getTimerState === 'function') ? getTimerState() : null;

    gridEl.innerHTML = filtered.map(t => {
        const ticketId = t.id;
        const ticketRef = t.ticket_ref || String(t.id);
        const title = t.name || 'Sin título';
        const desc = t.description ? t.description.replace(/<[^>]*>?/gm, '').trim() : '';
        const projName = (t.project_id && t.project_id.name) ? t.project_id.name : 'Sin proyecto';
        const taskName = (t.task_id && t.task_id.name) ? t.task_id.name : 'Sin tarea asignada';
        const partnerName = (t.partner_id && t.partner_id.name) ? t.partner_id.name : 'Cliente no asignado';
        const createDate = t.create_date ? t.create_date.split(' ')[0] : '';
        const hoursSpent = typeof t.total_hours_spent === 'number' ? t.total_hours_spent.toFixed(2) : '0.00';

        // Widget de Prioridad (Estrellas)
        const priorityVal = parseInt(t.priority, 10) || 0;
        let starsHtml = '';
        for (let i = 1; i <= 3; i++) {
            if (i <= priorityVal) {
                starsHtml += '<span class="text-amber-400 text-sm">★</span>';
            } else {
                starsHtml += '<span class="text-slate-600 text-sm">★</span>';
            }
        }

        // Estado del temporizador
        const isTimerRunning = currentTimer && currentTimer.status === 'running' && currentTimer.ticketId === ticketId;
        const isTimerPaused = currentTimer && currentTimer.status === 'paused' && currentTimer.ticketId === ticketId;

        return `
        <div class="ticket-card bg-slate-900/90 border border-slate-800 rounded-2xl p-3.5 sm:p-4 text-slate-100 shadow-sm relative group overflow-hidden transition-all ${isTimerRunning ? 'ticket-card-running border-amber-400' : ''}"
             data-ticket-id="${ticketId}" data-ticket-ref="${ticketRef}">

            <!-- Fila Superior: Prioridad, Referencia, Fecha y Horas Acumuladas -->
            <div class="flex items-center justify-between gap-2 mb-1.5">
                <div class="flex items-center space-x-2">
                    <!-- Widget de estrellas de prioridad -->
                    <div class="inline-flex items-center space-x-0.5 bg-slate-850 px-2 py-0.5 rounded-lg border border-slate-750" title="Prioridad: ${priorityVal} de 3">
                        ${starsHtml}
                    </div>
                    <!-- Número de Ticket -->
                    <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-amber-500/15 text-amber-300 border border-amber-500/30">
                        #${ticketRef}
                    </span>
                    <!-- Fecha -->
                    <span class="text-[11px] text-slate-400 font-mono hidden sm:inline">
                        📅 ${createDate}
                    </span>
                </div>

                <!-- Tiempo Acumulado en el Ticket -->
                <div class="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-xl text-xs font-mono font-bold bg-slate-800/90 text-slate-300 border border-slate-700/80 shadow-inner" title="Tiempo acumulado en este ticket">
                    <svg class="w-3.5 h-3.5 text-sky-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <span>${hoursSpent} h</span>
                </div>
            </div>

            <!-- Título del Ticket -->
            <h4 class="ticket-card-title text-sm sm:text-base font-bold text-slate-100 group-hover:text-amber-300 transition-colors line-clamp-2">
                ${title}
            </h4>

            <!-- Asunto / Descripción breve -->
            ${desc ? `<p class="text-xs text-slate-400 mt-1 line-clamp-2 italic leading-relaxed">${desc}</p>` : ''}

            <!-- Metadata: Cliente, Proyecto y Tarea -->
            <div class="mt-2.5 pt-2 border-t border-slate-800/90 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[11px]">
                <!-- Cliente asociado -->
                <span class="inline-flex items-center space-x-1 text-slate-400" title="Cliente">
                    <span class="text-slate-500">👤</span>
                    <span class="text-slate-300 font-medium">${partnerName}</span>
                </span>
                <!-- Proyecto -->
                <span class="inline-flex items-center space-x-1 text-slate-400" title="Proyecto">
                    <span class="text-slate-500">📁</span>
                    <span class="text-sky-300 font-medium">${projName}</span>
                </span>
                <!-- Tarea -->
                <span class="inline-flex items-center space-x-1 text-slate-400" title="Tarea">
                    <span class="text-slate-500">📌</span>
                    <span class="text-slate-300">${taskName}</span>
                </span>
            </div>

            <!-- Barra Inferior de Acciones y Controles de Tiempo -->
            <div class="mt-3 pt-2.5 border-t border-slate-800/90 flex items-center justify-between gap-2">
                <!-- Reloj en vivo (visible si está activo) -->
                <div class="flex items-center space-x-2">
                    <span class="ticket-live-clock text-xs font-mono font-bold text-amber-300 ${isTimerRunning ? '' : 'hidden'} animate-pulse">
                        00:00:00
                    </span>
                </div>

                <!-- Controles: Iniciar/Reanudar, Pausar, Parar y Cerrar -->
                <div class="flex items-center space-x-1.5 ml-auto">
                    <!-- Botón Play (Iniciar/Reanudar) -->
                    <button type="button" onclick="startTimerOnTicket(${ticketId})"
                            class="ticket-btn-play px-2.5 py-1 rounded-lg text-xs font-bold transition flex items-center space-x-1 cursor-pointer bg-sky-600 hover:bg-sky-500 text-white shadow-sm ${isTimerRunning ? 'hidden' : ''}"
                            title="Empezar a imputar tiempo a este ticket">
                        <span>▶</span>
                        <span>${isTimerPaused ? 'Reanudar' : 'Iniciar'}</span>
                    </button>

                    <!-- Botón Pause -->
                    <button type="button" onclick="pauseTimerOnTicket(${ticketId})"
                            class="ticket-btn-pause px-2.5 py-1 rounded-lg text-xs font-bold transition flex items-center space-x-1 cursor-pointer bg-amber-500 hover:bg-amber-400 text-slate-950 shadow-sm ${isTimerRunning ? '' : 'hidden'}"
                            title="Pausar cronómetro">
                        <span>⏸</span>
                        <span>Pausar</span>
                    </button>

                    <!-- Botón Stop -->
                    <button type="button" onclick="stopTimerOnTicket(${ticketId})"
                            class="ticket-btn-stop px-2.5 py-1 rounded-lg text-xs font-bold transition flex items-center space-x-1 cursor-pointer bg-rose-600 hover:bg-rose-500 text-white shadow-sm ${(isTimerRunning || isTimerPaused) ? '' : 'hidden'}"
                            title="Detener y registrar tiempo en Odoo">
                        <span>⏹</span>
                        <span>Parar</span>
                    </button>

                    <!-- Botón Dar por Cerrado -->
                    <button type="button" onclick="openCloseTicketModal(${ticketId}, '${ticketRef}', '${title.replace(/'/g, "\\'")}')"
                            class="px-2.5 py-1 rounded-lg text-xs font-semibold transition flex items-center space-x-1 cursor-pointer bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700"
                            title="Cerrar ticket definitivamente en Helpdesk">
                        <span>🔒</span>
                        <span>Cerrar</span>
                    </button>
                </div>
            </div>
        </div>
        `;
    }).join('');

    // Sincronizar reloj en caliente tras render
    updateExpressTimerState();
}

/**
 * Inicia cronómetro en un ticket de Helpdesk
 */
function startTimerOnTicket(ticketId) {
    const ticket = (window.pendingTickets || []).find(t => t.id === ticketId);
    if (!ticket) return;

    const pId = ticket.project_id ? ticket.project_id.id : 0;
    const pName = ticket.project_id ? ticket.project_id.name : 'Proyecto Ticket #' + (ticket.ticket_ref || ticket.id);
    const tId = ticket.task_id ? ticket.task_id.id : null;
    const tName = ticket.task_id ? ticket.task_id.name : '';
    const desc = `Ticket #${ticket.ticket_ref || ticket.id}: ${ticket.name}`;
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(new Date()) : new Date().toISOString().split('T')[0];

    if (typeof startWorkTimer === 'function') {
        startWorkTimer(
            pId,
            pName,
            tId,
            tName,
            desc,
            null, // timesheetId null para nueva fila
            0,
            todayStr,
            true, // fromModal = true (sin modal de horas previo)
            false, // silent = false (muestra toast)
            ticketId // ticketId explícito
        );
    }

    if (typeof showToast === 'function') {
        showToast(`⏱️ Cronómetro iniciado en Ticket #${ticket.ticket_ref || ticket.id}`, 'success');
    }

    setTimeout(() => {
        updateExpressTimerState();
    }, 80);
}

/**
 * Pausa cronómetro del ticket
 */
function pauseTimerOnTicket(ticketId) {
    if (typeof togglePauseTimer === 'function') {
        togglePauseTimer();
    }
    setTimeout(() => {
        updateExpressTimerState();
    }, 80);
}

/**
 * Detiene cronómetro del ticket y abre modal de finalización
 */
function stopTimerOnTicket(ticketId) {
    if (typeof openStopTimerModal === 'function') {
        openStopTimerModal();
    } else if (typeof clearTimer === 'function') {
        clearTimer();
    }
    setTimeout(() => {
        updateExpressTimerState();
    }, 80);
}

// ============================================================================
// MODAL DE CREACIÓN RÁPIDA DE TICKET (FAB +)
// ============================================================================

/**
 * Abre el modal para crear un nuevo ticket rápido
 */
async function openCreateTicketModal() {
    const modal = document.getElementById('modal-create-ticket');
    if (!modal) return;

    modal.classList.remove('hidden');

    // Cargar proyectos si no están en caché
    const projSelect = document.getElementById('create-ticket-project');
    if (projSelect && (!__cachedProjectsForTickets || projSelect.options.length <= 1)) {
        try {
            const res = await fetch('/api/projects');
            if (res.ok) {
                __cachedProjectsForTickets = await res.json();
                projSelect.innerHTML = '<option value="">-- Selecciona un proyecto --</option>' +
                    __cachedProjectsForTickets.map(p => `<option value="${p.id}">${p.name}</option>`).join('');
            }
        } catch (e) {
            console.warn('[PlanesGo] Error cargando proyectos:', e);
        }
    }

    // Cargar contactos/partners
    const partnerSelect = document.getElementById('create-ticket-partner');
    if (partnerSelect && (!__cachedPartnersForTickets || partnerSelect.options.length <= 1)) {
        try {
            const res = await fetch('/api/partners');
            if (res.ok) {
                __cachedPartnersForTickets = await res.json();
                partnerSelect.innerHTML = '<option value="">-- Sin contacto específico / Cliente de proyecto --</option>' +
                    __cachedPartnersForTickets.map(pt => `<option value="${pt.id}">${pt.name}${pt.email ? ' (' + pt.email + ')' : ''}</option>`).join('');
            }
        } catch (e) {
            console.warn('[PlanesGo] Error cargando partners:', e);
        }
    }

    // Resetear campos
    setTicketPriorityStar(0);
    const nameInput = document.getElementById('create-ticket-name');
    if (nameInput) nameInput.value = '';
    const descInput = document.getElementById('create-ticket-desc');
    if (descInput) descInput.value = '';
}

function closeCreateTicketModal() {
    const modal = document.getElementById('modal-create-ticket');
    if (modal) modal.classList.add('hidden');
}

/**
 * Actualiza las tareas y contacto cuando cambia el proyecto en el modal de creación
 */
async function onTicketModalProjectChange(projectId) {
    const taskSelect = document.getElementById('create-ticket-task');
    if (!taskSelect) return;

    taskSelect.innerHTML = '<option value="">Cargando tareas...</option>';
    if (!projectId) {
        taskSelect.innerHTML = '<option value="">-- Sin tarea específica asignada --</option>';
        return;
    }

    try {
        const res = await fetch(`/api/tasks?project_id=${projectId}`);
        if (res.ok) {
            const tasks = await res.json();
            taskSelect.innerHTML = '<option value="">-- Sin tarea específica asignada --</option>' +
                tasks.map(t => `<option value="${t.id}">${t.name}</option>`).join('');
        }
    } catch (e) {
        taskSelect.innerHTML = '<option value="">-- Sin tarea específica asignada --</option>';
    }

    // Si el proyecto tiene partner_id y el selector no tiene partner, auto-asignarlo
    if (__cachedProjectsForTickets && projectId) {
        const proj = __cachedProjectsForTickets.find(p => p.id === parseInt(projectId, 10));
        if (proj && proj.partner_id && proj.partner_id.id) {
            const partnerSelect = document.getElementById('create-ticket-partner');
            if (partnerSelect && !partnerSelect.value) {
                partnerSelect.value = proj.partner_id.id;
            }
        }
    }
}

/**
 * Establece la prioridad del ticket con estrellas en el modal
 */
function setTicketPriorityStar(rating) {
    const input = document.getElementById('create-ticket-priority');
    if (input) input.value = rating;

    const starsContainer = document.getElementById('ticket-priority-stars');
    if (starsContainer) {
        starsContainer.querySelectorAll('[data-star]').forEach(starEl => {
            const starVal = parseInt(starEl.dataset.star, 10);
            if (starVal <= rating) {
                starEl.className = 'text-xl text-amber-400 hover:text-amber-300 transition cursor-pointer';
            } else {
                starEl.className = 'text-xl text-slate-600 hover:text-amber-400 transition cursor-pointer';
            }
        });
    }

    const label = document.getElementById('ticket-priority-label');
    if (label) {
        const labels = ['Baja (0★)', 'Media (1★)', 'Alta (2★)', 'Urgente (3★)'];
        label.textContent = labels[rating] || 'Baja (0★)';
    }
}

/**
 * Envía el formulario para crear un ticket rápido en Odoo
 */
async function submitCreateTicket(event) {
    if (event) event.preventDefault();

    const name = document.getElementById('create-ticket-name')?.value?.trim();
    const projId = parseInt(document.getElementById('create-ticket-project')?.value, 10) || 0;
    const taskId = parseInt(document.getElementById('create-ticket-task')?.value, 10) || 0;
    const partnerId = parseInt(document.getElementById('create-ticket-partner')?.value, 10) || 0;
    const priority = document.getElementById('create-ticket-priority')?.value || '0';
    const desc = document.getElementById('create-ticket-desc')?.value?.trim() || '';

    if (!name || !projId) {
        alert('Por favor indica un título y selecciona un proyecto.');
        return;
    }

    const btnSpinner = document.getElementById('btn-create-ticket-spinner');
    const btnLabel = document.getElementById('btn-create-ticket-label');
    const btnSubmit = document.getElementById('btn-submit-create-ticket');

    if (btnSpinner) btnSpinner.classList.remove('hidden');
    if (btnLabel) btnLabel.textContent = 'Creando en Odoo...';
    if (btnSubmit) btnSubmit.disabled = true;

    try {
        const res = await fetch('/api/tickets/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: name,
                description: desc,
                project_id: projId,
                task_id: taskId,
                partner_id: partnerId,
                priority: priority
            })
        });

        const data = await res.json();
        if (!res.ok || data.error) {
            throw new Error(data.error || 'Error del servidor');
        }

        closeCreateTicketModal();
        if (typeof showToast === 'function') {
            showToast(`✅ Ticket #${data.ticket_ref || data.id} creado con éxito`, 'success');
        }

        // Recargar lista de tickets
        await loadTicketsView(true);
    } catch (err) {
        console.error('[PlanesGo Tickets] Error creando ticket:', err);
        if (typeof showToast === 'function') {
            showToast(`⚠️ No se pudo crear el ticket: ${err.message}`, 'error');
        } else {
            alert(`Error creando ticket: ${err.message}`);
        }
    } finally {
        if (btnSpinner) btnSpinner.classList.add('hidden');
        if (btnLabel) btnLabel.textContent = 'Crear Ticket';
        if (btnSubmit) btnSubmit.disabled = false;
    }
}

// ============================================================================
// MODAL DE CIERRE DEFINITIVO DE TICKET (INMUTABLE)
// ============================================================================

function openCloseTicketModal(ticketId, ticketRef, ticketTitle) {
    const modal = document.getElementById('modal-close-ticket');
    if (!modal) return;

    document.getElementById('close-ticket-id').value = ticketId;
    document.getElementById('close-ticket-ref').value = ticketRef;
    const titleEl = document.getElementById('close-ticket-ref-title');
    if (titleEl) {
        titleEl.textContent = `Ticket #${ticketRef}: ${ticketTitle}`;
    }

    const subjInput = document.getElementById('close-ticket-subject');
    if (subjInput) subjInput.value = 'Incidencia resuelta';
    const descInput = document.getElementById('close-ticket-description');
    if (descInput) descInput.value = '';

    modal.classList.remove('hidden');
}

function closeCloseTicketModal() {
    const modal = document.getElementById('modal-close-ticket');
    if (modal) modal.classList.add('hidden');
}

async function submitCloseTicket(event) {
    if (event) event.preventDefault();

    const ticketId = parseInt(document.getElementById('close-ticket-id')?.value, 10) || 0;
    const ticketRef = document.getElementById('close-ticket-ref')?.value || '';
    const subject = document.getElementById('close-ticket-subject')?.value?.trim();
    const description = document.getElementById('close-ticket-description')?.value?.trim() || '';

    if (!ticketId || !subject) {
        alert('Por favor indica el asunto o motivo del cierre.');
        return;
    }

    const btnSpinner = document.getElementById('btn-close-ticket-spinner');
    const btnLabel = document.getElementById('btn-close-ticket-label');
    const btnSubmit = document.getElementById('btn-submit-close-ticket');

    if (btnSpinner) btnSpinner.classList.remove('hidden');
    if (btnLabel) btnLabel.textContent = 'Cerrando en Odoo...';
    if (btnSubmit) btnSubmit.disabled = true;

    try {
        const res = await fetch('/api/tickets/close', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                ticket_id: ticketId,
                ticket_ref: ticketRef,
                subject: subject,
                description: description
            })
        });

        const data = await res.json();
        if (!res.ok || data.error) {
            throw new Error(data.error || 'Error al cerrar ticket');
        }

        closeCloseTicketModal();
        if (typeof showToast === 'function') {
            showToast(`🔒 Ticket #${ticketRef || ticketId} cerrado definitivamente`, 'success');
        }

        // Recargar tickets para quitarlo de pendientes
        await loadTicketsView(true);
    } catch (err) {
        console.error('[PlanesGo Tickets] Error cerrando ticket:', err);
        if (typeof showToast === 'function') {
            showToast(`⚠️ No se pudo cerrar el ticket: ${err.message}`, 'error');
        } else {
            alert(`Error: ${err.message}`);
        }
    } finally {
        if (btnSpinner) btnSpinner.classList.add('hidden');
        if (btnLabel) btnLabel.textContent = 'Dar por Cerrado';
        if (btnSubmit) btnSubmit.disabled = false;
    }
}

/**
 * Abre el popout de escritorio directamente en la pestaña de tickets
 */
function openTicketsPopout() {
    openExpressPopout('tickets');
}

// Exportar globalmente para vistas y temporizador
window.switchView = switchView;
window.toggleExpressFloating = toggleExpressFloating;
window.openExpressFloating = openExpressFloating;
window.closeExpressFloating = closeExpressFloating;
window.toggleMinimizeExpress = toggleMinimizeExpress;
window.openExpressPopout = openExpressPopout;
window.openTicketsPopout = openTicketsPopout;
window.loadExpressTimesheets = loadExpressTimesheets;
window.renderExpressView = renderExpressView;
window.handleExpressCardClick = handleExpressCardClick;
window.updateExpressTimerState = updateExpressTimerState;
window.initExpressWindowInteractions = initExpressWindowInteractions;

// Exportar funciones de tickets
window.switchExpressTab = switchExpressTab;
window.reloadCurrentTab = reloadCurrentTab;
window.handleExpressSearchInput = handleExpressSearchInput;
window.clearExpressSearch = clearExpressSearch;
window.loadTicketsView = loadTicketsView;
window.renderTicketsView = renderTicketsView;
window.startTimerOnTicket = startTimerOnTicket;
window.pauseTimerOnTicket = pauseTimerOnTicket;
window.stopTimerOnTicket = stopTimerOnTicket;
window.openCreateTicketModal = openCreateTicketModal;
window.closeCreateTicketModal = closeCreateTicketModal;
window.onTicketModalProjectChange = onTicketModalProjectChange;
window.setTicketPriorityStar = setTicketPriorityStar;
window.submitCreateTicket = submitCreateTicket;
window.openCloseTicketModal = openCloseTicketModal;
window.closeCloseTicketModal = closeCloseTicketModal;
window.submitCloseTicket = submitCloseTicket;

// Sincronización periódica y silenciosa en segundo plano (cada 20 segundos)
if (!window.__expressSilentSyncInterval) {
    window.__expressSilentSyncInterval = setInterval(() => {
        if (document.visibilityState !== 'hidden') {
            const isExpressVisible = (window.location.pathname === '/express' || window.location.pathname === '/m') ||
                                    document.getElementById('express-grid-container')?.offsetParent !== null;

            if (isExpressVisible) {
                if (window.__activeExpressTab === 'tickets') {
                    loadTicketsView(false);
                } else if (typeof loadExpressTimesheets === 'function') {
                    loadExpressTimesheets(true, true);
                }
            }
        }
    }, 20000);
}

if (!window.__expressVisibilityListenerAdded) {
    window.__expressVisibilityListenerAdded = true;
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') {
            const isExpressVisible = (window.location.pathname === '/express' || window.location.pathname === '/m') ||
                                    document.getElementById('express-grid-container')?.offsetParent !== null;

            if (isExpressVisible) {
                if (window.__activeExpressTab === 'tickets') {
                    loadTicketsView(false);
                } else if (typeof loadExpressTimesheets === 'function') {
                    loadExpressTimesheets(true, true);
                }
            }
        }
    });
}



