/**
 * static/js/views.js
 * Lógica de navegación por semanas, selector de vistas (Lista, Calendario, Gantt),
 * cálculo de KPIs reactivos y renderizado de las vistas visuales.
 */

const currentMonday = getMonday(new Date());
let selectedWeekMonday = new Date(currentMonday);
let currentView = 'list';

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

        const safeEmpName = (typeof escapeHtml === 'function') ? escapeHtml(empName) : empName;
        const safeProjName = (typeof escapeHtml === 'function') ? escapeHtml(projName) : projName;
        const safeTaskName = (typeof escapeHtml === 'function') ? escapeHtml(taskName) : taskName;
        const safeDesc = (typeof escapeHtml === 'function') ? escapeHtml(desc) : desc;
        const safeInvoice = (typeof escapeHtml === 'function') ? escapeHtml(invoiceName) : invoiceName;

        tr.innerHTML = `
            <td class="py-3 px-4 sm:px-6 whitespace-nowrap">
                <span class="font-medium text-slate-900 font-mono text-xs">${entry.date}</span>
            </td>
            <td class="py-3 px-4 whitespace-nowrap">
                <div class="flex items-center space-x-2">
                    <div class="w-6 h-6 rounded-full bg-slate-200 text-slate-600 flex items-center justify-center font-bold text-[10px] shrink-0">
                        ${empInitial}
                    </div>
                    <span class="font-medium text-slate-800">${safeEmpName}</span>
                </div>
            </td>
            <td class="py-3 px-4 whitespace-nowrap col-project-cell">
                ${projName ? `
                <button type="button"
                        onclick="selectSidebarProject(this.dataset.projectName, this.dataset.projectId)"
                        data-project-name="${safeProjName}"
                        data-project-id="${projId}"
                        class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-sky-50 text-sky-800 border border-sky-100 hover:bg-sky-100 transition cursor-pointer"
                        title="Filtrar por este proyecto">
                    ${safeProjName}
                </button>` : `<span class="text-slate-400 text-xs">-</span>`}
            </td>
            <td class="py-3 px-4 whitespace-nowrap">
                ${taskName ? `
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium bg-slate-100 text-slate-700">
                    ${safeTaskName}
                </span>` : `<span class="text-slate-400 text-xs">-</span>`}
            </td>
            <td class="py-3 px-4 text-slate-600 max-w-xs truncate" title="${safeDesc}">
                ${desc ? safeDesc : `<span class="italic text-slate-400">Sin descripción</span>`}
            </td>
            <td class="py-3 px-4 sm:px-6 text-right whitespace-nowrap">
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
            <td class="py-3 px-3 text-right whitespace-nowrap">
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

function switchView(viewName) {
    currentView = viewName;

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
        }
    }

    applyTimesheetFilters();
}

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

        const matchSearch = !searchVal || desc.includes(searchVal) || task.includes(searchVal) || project.includes(searchVal) || projectName.includes(searchVal);

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
        if (td) td.colSpan = hasActiveProject ? 6 : 7;
    }
    if (emptyRow) {
        const td = emptyRow.querySelector('td');
        if (td) td.colSpan = hasActiveProject ? 6 : 7;
    }

    // Actualizar KPIs superiores
    if (kpiHours) kpiHours.textContent = visibleHours.toFixed(2);
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
// VISTA EXPRESS: BOTONERA DE TAREAS RECIENTES (ÚLTIMAS 2 SEMANAS)
// ============================================================================

/**
 * Carga desde el servidor las imputaciones de las dos últimas semanas.
 */
function loadExpressTimesheets(forceReload) {
    if (isExpressLoading && !forceReload) return;

    const loadingEl = document.getElementById('express-loading-state');
    const gridEl = document.getElementById('express-grid-container');
    const emptyEl = document.getElementById('express-empty-state');

    if (expressTimesheets && !forceReload) {
        renderExpressView();
        return;
    }

    if (loadingEl) loadingEl.classList.remove('hidden');
    if (gridEl) gridEl.classList.add('hidden');
    if (emptyEl) emptyEl.classList.add('hidden');

    isExpressLoading = true;

    // Calcular rango de 2 semanas: desde hace 14 días (o lunes de hace 2 semanas) hasta hoy
    const today = new Date();
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];

    const twoWeeksAgo = new Date(today);
    twoWeeksAgo.setDate(twoWeeksAgo.getDate() - 14);
    const dateFromStr = (typeof formatISODate === 'function') ? formatISODate(twoWeeksAgo) : twoWeeksAgo.toISOString().split('T')[0];

    fetch(`/api/timesheets?date_from=${dateFromStr}&date_to=${todayStr}`)
        .then(res => {
            if (!res.ok) throw new Error('Error al cargar imputaciones de las últimas 2 semanas');
            return res.json();
        })
        .then(data => {
            expressTimesheets = Array.isArray(data) ? data : [];
            isExpressLoading = false;
            if (loadingEl) loadingEl.classList.add('hidden');
            if (gridEl) gridEl.classList.remove('hidden');
            renderExpressView();
        })
        .catch(err => {
            console.error('[PlanesGo Express] Error:', err);
            isExpressLoading = false;
            if (loadingEl) loadingEl.classList.add('hidden');
            if (gridEl) gridEl.classList.remove('hidden');
            expressTimesheets = expressTimesheets || [];
            renderExpressView();
        });
}

/**
 * Formatea una fecha para mostrar en las tarjetas de la botonera.
 */
function formatCardDate(dateStr) {
    if (!dateStr) return '';
    const today = new Date();
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];
    if (dateStr === todayStr) return 'Hoy';

    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    const yesterdayStr = (typeof formatISODate === 'function') ? formatISODate(yesterday) : yesterday.toISOString().split('T')[0];
    if (dateStr === yesterdayStr) return 'Ayer';

    const parts = dateStr.split('-');
    if (parts.length === 3) {
        const d = new Date(parseInt(parts[0], 10), parseInt(parts[1], 10) - 1, parseInt(parts[2], 10));
        return d.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });
    }
    return dateStr;
}

/**
 * Renderiza la cuadrícula de la botonera Express con las tareas de las últimas 2 semanas.
 */
function renderExpressView() {
    const gridEl = document.getElementById('express-grid-container');
    const emptyEl = document.getElementById('express-empty-state');
    const countBadge = document.getElementById('express-tasks-count-badge');
    if (!gridEl) return;

    // Si aún no hemos cargado los datos de las últimas 2 semanas, disparar la carga
    if (!expressTimesheets && !isExpressLoading) {
        loadExpressTimesheets();
        return;
    }

    const today = new Date();
    const todayStr = (typeof formatISODate === 'function') ? formatISODate(today) : today.toISOString().split('T')[0];

    // Filtros activos actuales
    const searchInput = document.getElementById('filter-search');
    const searchVal = (searchInput ? searchInput.value : '').toLowerCase().trim();
    const sidebarEmployeeSelect = document.getElementById('sidebar-employee-select');
    const employeeSelect = document.getElementById('filter-employee');
    const employeeVal = (sidebarEmployeeSelect ? sidebarEmployeeSelect.value : (employeeSelect ? employeeSelect.value : '')).toLowerCase().trim();

    const targetProjectId = activeSidebarProjectId;
    const targetProjectName = (typeof activeSidebarProjectName !== 'undefined' ? activeSidebarProjectName : '').toLowerCase().trim();

    // 1. Recopilar datos de expressTimesheets y de las filas actuales del DOM
    const allEntries = [];

    if (Array.isArray(expressTimesheets)) {
        expressTimesheets.forEach(ts => {
            const empName = (ts.employee_id && ts.employee_id.name) ? ts.employee_id.name :
                            ((ts.user_id && ts.user_id.name) ? ts.user_id.name : '');
            allEntries.push({
                id: ts.id,
                date: ts.date,
                projectId: ts.project_id ? ts.project_id.id : 0,
                projectName: ts.project_id ? (ts.project_id.name || '') : '',
                taskId: ts.task_id ? ts.task_id.id : 0,
                taskName: ts.task_id ? (ts.task_id.name || '') : '',
                desc: ts.name || '',
                hours: typeof ts.unit_amount === 'number' ? ts.unit_amount : 0,
                employee: empName,
                isTimerRunning: Boolean(ts.is_timer_running)
            });
        });
    }

    // Integrar filas del DOM (incluidas imputaciones creadas u optimistas hoy)
    const existingIds = new Set(allEntries.map(e => String(e.id)));
    document.querySelectorAll('#timesheet-table .timesheet-row').forEach(row => {
        const id = row.dataset.id;
        if (id && !existingIds.has(String(id))) {
            allEntries.push({
                id: id,
                date: row.dataset.date,
                projectId: parseInt(row.dataset.projectId, 10) || 0,
                projectName: row.dataset.projectName || row.dataset.project || '',
                taskId: parseInt(row.dataset.taskId, 10) || 0,
                taskName: row.dataset.taskName || row.dataset.task || '',
                desc: row.dataset.desc || '',
                hours: parseFloat(row.dataset.hours) || 0,
                employee: row.dataset.employee || '',
                isTimerRunning: row.dataset.timerRunning === 'true'
            });
        }
    });

    // 2. Comprobar estado actual del temporizador
    const timerState = (typeof getTimerState === 'function') ? getTimerState() : null;

    // 3. Agrupar por tarea única (o proyecto si no hay tarea)
    const taskMap = new Map();

    allEntries.forEach(entry => {
        const emp = (entry.employee || '').toLowerCase();
        if (employeeVal) {
            const matchEmp = emp.includes(employeeVal) || employeeVal.includes(emp) || (entry.isTimerRunning && (emp === 'yo' || !emp));
            if (!matchEmp) return;
        }

        // Filtro por proyecto del sidebar
        const pIdStr = String(entry.projectId || '');
        const pNameLower = (entry.projectName || '').toLowerCase();
        if (targetProjectId && targetProjectId !== '0') {
            if (pIdStr !== String(targetProjectId)) {
                if (!targetProjectName || !pNameLower.includes(targetProjectName)) return;
            }
        } else if (targetProjectName) {
            if (!pNameLower.includes(targetProjectName)) return;
        }

        // Filtro por buscador de texto
        if (searchVal) {
            const descLower = (entry.desc || '').toLowerCase();
            const taskLower = (entry.taskName || '').toLowerCase();
            const projLower = (entry.projectName || '').toLowerCase();
            if (!descLower.includes(searchVal) && !taskLower.includes(searchVal) && !projLower.includes(searchVal)) {
                return;
            }
        }

        const pId = entry.projectId || 0;
        const tId = entry.taskId || 0;
        if (!pId) return;

        const key = `${pId}_${tId}`;

        if (!taskMap.has(key)) {
            taskMap.set(key, {
                key: key,
                projectId: pId,
                projectName: entry.projectName || `Proyecto #${pId}`,
                taskId: tId,
                taskName: entry.taskName || (tId ? `Tarea #${tId}` : 'General (Sin tarea)'),
                lastDate: entry.date || '',
                lastDescription: entry.desc || '',
                lastTimesheetId: entry.id,
                totalHours: 0,
                todayTimesheetId: (entry.date === todayStr) ? entry.id : null,
                todayHours: (entry.date === todayStr) ? entry.hours : 0,
                hasTodayEntry: (entry.date === todayStr)
            });
        }

        const item = taskMap.get(key);
        item.totalHours += (entry.hours || 0);

        if (entry.date && (!item.lastDate || entry.date > item.lastDate)) {
            item.lastDate = entry.date;
            item.lastDescription = entry.desc || item.lastDescription;
            item.lastTimesheetId = entry.id;
        }

        if (entry.date === todayStr) {
            item.hasTodayEntry = true;
            item.todayTimesheetId = entry.id;
            item.todayHours = Math.max(item.todayHours, entry.hours);
        }
    });

    // Asegurarse de que si hay un temporizador activo en marcha, su tarea aparezca en la botonera
    if (timerState && timerState.projectId) {
        const activeKey = `${timerState.projectId}_${timerState.taskId || 0}`;
        if (!taskMap.has(activeKey)) {
            taskMap.set(activeKey, {
                key: activeKey,
                projectId: timerState.projectId,
                projectName: timerState.projectName || `Proyecto #${timerState.projectId}`,
                taskId: timerState.taskId || 0,
                taskName: timerState.taskName || (timerState.taskId ? `Tarea #${timerState.taskId}` : 'General (Sin tarea)'),
                lastDate: timerState.date || todayStr,
                lastDescription: timerState.description || '',
                lastTimesheetId: timerState.timesheetId,
                totalHours: 0,
                todayTimesheetId: timerState.timesheetId,
                todayHours: 0,
                hasTodayEntry: (timerState.date === todayStr)
            });
        }
    }

    const tasks = Array.from(taskMap.values());

    // Marcar si está corriendo actualmente
    tasks.forEach(t => {
        let isRunning = false;
        if (timerState && timerState.status === 'running') {
            if (t.todayTimesheetId && timerState.timesheetId && String(t.todayTimesheetId) === String(timerState.timesheetId)) {
                isRunning = true;
            } else if (t.lastTimesheetId && timerState.timesheetId && String(t.lastTimesheetId) === String(timerState.timesheetId)) {
                isRunning = true;
            } else if (t.taskId && timerState.taskId && t.taskId === timerState.taskId) {
                isRunning = true;
            } else if (!t.taskId && !timerState.taskId && t.projectId === timerState.projectId) {
                isRunning = true;
            }
        }
        t.isRunning = isRunning;
    });

    // Ordenación: 1º En marcha, 2º Fecha más reciente desc, 3º Horas hoy desc
    tasks.sort((a, b) => {
        if (a.isRunning && !b.isRunning) return -1;
        if (!a.isRunning && b.isRunning) return 1;
        if (a.lastDate > b.lastDate) return -1;
        if (a.lastDate < b.lastDate) return 1;
        return (b.todayHours || 0) - (a.todayHours || 0);
    });

    if (countBadge) {
        countBadge.textContent = `${tasks.length} ${tasks.length === 1 ? 'tarea' : 'tareas'}`;
    }

    if (tasks.length === 0) {
        gridEl.innerHTML = '';
        if (emptyEl) emptyEl.classList.remove('hidden');
        return;
    }

    if (emptyEl) emptyEl.classList.add('hidden');

    // Tiempo transcurrido formateado para el timer activo
    let liveClockStr = '00:00:00';
    if (timerState) {
        const totalMs = (timerState.accumulatedMs || 0) + (timerState.status === 'running' && timerState.lastStartTime ? (Date.now() - timerState.lastStartTime) : 0);
        if (typeof formatElapsedMs === 'function') {
            liveClockStr = formatElapsedMs(totalMs);
        }
    }

    let cardsHtml = '';
    tasks.forEach(item => {
        const safeProj = escapeHtml(item.projectName);
        const safeTask = escapeHtml(item.taskName);
        const safeDesc = escapeHtml(item.lastDescription);

        let dateBadgeHtml = '';
        if (item.hasTodayEntry) {
            dateBadgeHtml = `
                <span class="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md text-[10px] font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200/80">
                    <span>Hoy</span>
                    ${item.todayHours > 0 ? `<span class="font-mono">(${item.todayHours.toFixed(2)}h)</span>` : ''}
                </span>
            `;
        } else if (item.lastDate) {
            dateBadgeHtml = `
                <span class="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md text-[10px] font-medium bg-amber-50 text-amber-700 border border-amber-200/80" title="Última fecha: ${item.lastDate}. Se duplicará para hoy al pulsar">
                    <svg class="w-2.5 h-2.5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2z"/></svg>
                    <span>${formatCardDate(item.lastDate)}</span>
                </span>
            `;
        }

        const isRunning = item.isRunning;
        const cardBorderClass = isRunning 
            ? 'border-emerald-400 ring-2 ring-emerald-300 shadow-md bg-emerald-50/25'
            : 'border-slate-200/80 hover:border-sky-300 hover:shadow-md bg-white';

        cardsHtml += `
            <div class="express-card group relative rounded-2xl border ${cardBorderClass} p-4 transition-all duration-150 flex flex-col justify-between cursor-pointer select-none"
                 role="button" tabindex="0"
                 data-project-id="${item.projectId}"
                 data-project-name="${escapeAttr(item.projectName)}"
                 data-task-id="${item.taskId || 0}"
                 data-task-name="${escapeAttr(item.taskName)}"
                 data-description="${escapeAttr(item.lastDescription)}"
                 data-timesheet-id="${item.lastTimesheetId || 0}"
                 data-today-timesheet-id="${item.todayTimesheetId || 0}"
                 data-today-hours="${item.todayHours || 0}"
                 data-last-date="${item.lastDate || ''}"
                 data-has-today="${item.hasTodayEntry ? 'true' : 'false'}"
                 onclick="handleExpressCardClick(this)"
                 onkeydown="if(event.key === 'Enter' || event.key === ' ') { event.preventDefault(); handleExpressCardClick(this); }">
                
                <!-- Cabecera de la tarjeta: Proyecto y Estado/Fecha -->
                <div class="flex items-start justify-between gap-2">
                    <span class="inline-flex items-center px-2 py-0.5 rounded-lg text-[11px] font-semibold bg-sky-50 text-sky-800 border border-sky-100 truncate max-w-[62%]" title="${safeProj}">
                        ${safeProj}
                    </span>
                    <div class="shrink-0">
                        <span class="express-badge-running ${isRunning ? '' : 'hidden'} inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 text-emerald-800 border border-emerald-300 animate-pulse">
                            <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                            <span>EN MARCHA</span>
                        </span>
                        <span class="express-badge-idle ${isRunning ? 'hidden' : ''}">
                            ${dateBadgeHtml}
                        </span>
                    </div>
                </div>

                <!-- Cuerpo de la tarjeta: Tarea y Descripción previa -->
                <div class="my-2.5">
                    <h4 class="text-sm font-bold text-slate-800 group-hover:text-sky-700 leading-snug line-clamp-2 transition-colors" title="${safeTask}">
                        ${safeTask}
                    </h4>
                    <p class="text-xs text-slate-500 line-clamp-2 italic font-normal mt-1 min-h-[1.75rem]" title="${safeDesc}">
                        ${safeDesc || '<span class="text-slate-300 not-italic">Sin descripción previa</span>'}
                    </p>
                </div>

                <!-- Pie de la tarjeta: Métricas / Reloj y Botón de Acción -->
                <div class="pt-2.5 border-t border-slate-100 flex items-center justify-between gap-2 mt-auto">
                    <div class="min-w-0">
                        <div class="express-live-clock-container ${isRunning ? '' : 'hidden'} inline-flex items-center space-x-1 text-emerald-800 font-mono font-bold text-xs bg-emerald-100/90 px-2 py-0.5 rounded-lg border border-emerald-200">
                            <span class="relative flex h-1.5 w-1.5">
                                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                                <span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span>
                            </span>
                            <span class="express-live-clock font-mono font-bold text-emerald-900">${liveClockStr}</span>
                        </div>
                        <div class="express-hours-summary ${isRunning ? 'hidden' : ''} text-[11px] text-slate-400 font-medium">
                            <span>2 sem: </span>
                            <span class="font-bold text-slate-600 font-mono">${item.totalHours.toFixed(2)}h</span>
                        </div>
                    </div>

                    <div class="shrink-0">
                        <button type="button" class="express-btn-action inline-flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-bold transition shadow-xs ${isRunning ? 'bg-amber-500 hover:bg-amber-600 text-white' : 'bg-sky-600 hover:bg-sky-700 active:bg-sky-800 text-white'}">
                            ${isRunning ? `
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6"/></svg>
                                <span>Pausar</span>
                            ` : `
                                <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
                                <span>${item.hasTodayEntry ? 'Reanudar' : 'Iniciar'}</span>
                            `}
                        </button>
                    </div>
                </div>
            </div>
        `;
    });

    gridEl.innerHTML = cardsHtml;
}

/**
 * Gestiona el click en una tarjeta de la botonera Express.
 * Inicia el cronómetro deteniendo el anterior sin mostrar modales.
 * Si la fecha de la imputación es anterior a hoy, realiza un duplicado automático para hoy.
 */
function handleExpressCardClick(cardEl) {
    if (!cardEl) return;
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

    // Verificar si esta tarjeta es la que está activa
    let isThisCardActive = false;
    if (current) {
        if (current.timesheetId && (current.timesheetId === todayTsId || current.timesheetId === tsId)) {
            isThisCardActive = true;
        } else if (tId && current.taskId && current.taskId === tId) {
            isThisCardActive = true;
        } else if (!tId && !current.taskId && current.projectId === pId) {
            isThisCardActive = true;
        }
    }

    if (isThisCardActive) {
        // Pausar o reanudar el cronómetro actual
        if (typeof togglePauseTimer === 'function') {
            togglePauseTimer();
        }
        return;
    }

    // Si había otro cronómetro en marcha, startWorkTimer lo detiene automáticamente (stopPreviousRunningTimer).
    // Comprobamos si tiene fecha anterior a hoy:
    if (!hasToday) {
        // La fecha es anterior a hoy (o no tiene registro hoy):
        // Hacemos el duplicado para poner la fecha de hoy, iniciando el cronómetro sin abrir modales
        const initialDesc = desc || 'Trabajo en curso';
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
                true  // fromModal = true: NO abre modal ni ventana de detalle
            );
        }
        if (typeof showToast === 'function') {
            showToast(`⏱️ Tarea duplicada para hoy. Cronómetro iniciado en "${pName}"`, 'success');
        }
    } else {
        // Ya tiene imputación para hoy:
        // Iniciar / reanudar en la imputación de hoy sin abrir modales
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
                true // fromModal = true: NO abre modal ni ventana de detalle
            );
        }
        if (typeof showToast === 'function') {
            showToast(`⏱️ Cronómetro iniciado en "${pName}"`, 'success');
        }
    }

    // Refrescar el estado visual de la botonera express de inmediato
    setTimeout(() => {
        updateExpressTimerState();
    }, 50);
}

/**
 * Sincroniza en tiempo real el reloj y las clases activas de la botonera Express.
 */
function updateExpressTimerState() {
    const current = (typeof getTimerState === 'function') ? getTimerState() : null;
    const isRunning = current && current.status === 'running';
    const totalMs = current ? ((current.accumulatedMs || 0) + (isRunning && current.lastStartTime ? (Date.now() - current.lastStartTime) : 0)) : 0;
    const formattedClock = (typeof formatElapsedMs === 'function') ? formatElapsedMs(totalMs) : '00:00:00';

    document.querySelectorAll('.express-card').forEach(card => {
        const pId = parseInt(card.dataset.projectId, 10) || 0;
        const tId = parseInt(card.dataset.taskId, 10) || 0;
        const tsId = parseInt(card.dataset.timesheetId, 10) || 0;
        const todayTsId = parseInt(card.dataset.todayTimesheetId, 10) || 0;

        let isMatch = false;
        if (current) {
            if (current.timesheetId && (current.timesheetId === todayTsId || current.timesheetId === tsId)) {
                isMatch = true;
            } else if (tId && current.taskId && current.taskId === tId) {
                isMatch = true;
            } else if (!tId && !current.taskId && current.projectId === pId) {
                isMatch = true;
            }
        }

        const clockEl = card.querySelector('.express-live-clock');
        const clockContainer = card.querySelector('.express-live-clock-container');
        const hoursSummary = card.querySelector('.express-hours-summary');
        const badgeRunning = card.querySelector('.express-badge-running');
        const badgeIdle = card.querySelector('.express-badge-idle');
        const btnAction = card.querySelector('.express-btn-action');

        if (isMatch && isRunning) {
            card.classList.add('border-emerald-400', 'ring-2', 'ring-emerald-300', 'shadow-md', 'bg-emerald-50/25');
            card.classList.remove('border-slate-200/80', 'bg-white');

            if (clockEl) clockEl.textContent = formattedClock;
            if (clockContainer) clockContainer.classList.remove('hidden');
            if (hoursSummary) hoursSummary.classList.add('hidden');
            if (badgeRunning) badgeRunning.classList.remove('hidden');
            if (badgeIdle) badgeIdle.classList.add('hidden');

            if (btnAction) {
                btnAction.className = 'express-btn-action inline-flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-bold transition shadow-xs bg-amber-500 hover:bg-amber-600 text-white';
                btnAction.innerHTML = `
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6"/></svg>
                    <span>Pausar</span>
                `;
            }
        } else {
            card.classList.remove('border-emerald-400', 'ring-2', 'ring-emerald-300', 'shadow-md', 'bg-emerald-50/25');
            card.classList.add('border-slate-200/80', 'bg-white');

            if (clockContainer) clockContainer.classList.add('hidden');
            if (hoursSummary) hoursSummary.classList.remove('hidden');
            if (badgeRunning) badgeRunning.classList.add('hidden');
            if (badgeIdle) badgeIdle.classList.remove('hidden');

            if (btnAction) {
                const hasToday = card.dataset.hasToday === 'true';
                btnAction.className = 'express-btn-action inline-flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-bold transition shadow-xs bg-sky-600 hover:bg-sky-700 active:bg-sky-800 text-white';
                btnAction.innerHTML = `
                    <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
                    <span>${hasToday ? 'Reanudar' : 'Iniciar'}</span>
                `;
            }
        }
    });
}

// Exportar globalmente para vistas y temporizador
window.switchView = switchView;
window.loadExpressTimesheets = loadExpressTimesheets;
window.renderExpressView = renderExpressView;
window.handleExpressCardClick = handleExpressCardClick;
window.updateExpressTimerState = updateExpressTimerState;

