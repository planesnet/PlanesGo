/**
 * static/js/views.js
 * Lógica de navegación por semanas, selector de vistas (Lista, Calendario, Gantt),
 * cálculo de KPIs reactivos y renderizado de las vistas visuales.
 */

const currentMonday = getMonday(new Date());
let selectedWeekMonday = new Date(currentMonday);
let isWeekFilterActive = true;
let currentView = 'list';

function navigateWeek(delta) {
    isWeekFilterActive = true;
    selectedWeekMonday.setDate(selectedWeekMonday.getDate() + delta * 7);
    updateWeekControls();
    applyTimesheetFilters();
}

function goToCurrentWeek() {
    isWeekFilterActive = true;
    selectedWeekMonday = new Date(currentMonday);
    updateWeekControls();
    applyTimesheetFilters();
}

function toggleAllWeeksFilter() {
    isWeekFilterActive = !isWeekFilterActive;
    updateWeekControls();
    applyTimesheetFilters();
}

function updateWeekControls() {
    const titleEl = document.getElementById('week-title-display');
    const datesEl = document.getElementById('week-dates-display');
    const currentBadge = document.getElementById('current-week-badge');
    const btnCurrent = document.getElementById('btn-current-week');
    const btnToggleText = document.getElementById('btn-toggle-all-weeks-text');
    const btnPrev = document.getElementById('btn-prev-week');
    const btnNext = document.getElementById('btn-next-week');

    const kpiHoursSub = document.getElementById('kpi-hours-sub');
    const kpiProjectsSub = document.getElementById('kpi-projects-sub');
    const kpiEntriesSub = document.getElementById('kpi-entries-sub');
    const kpiEmployeesSub = document.getElementById('kpi-employees-sub');

    // Determinar viabilidad de botones según fechas existentes en las imputaciones
    const rows = document.querySelectorAll('.timesheet-row');
    let minMonday = null;
    let maxMonday = null;

    rows.forEach(r => {
        const dStr = r.dataset.date;
        if (dStr) {
            const d = parseISODate(dStr);
            if (d) {
                const mon = getMonday(d);
                if (!minMonday || mon < minMonday) minMonday = mon;
                if (!maxMonday || mon > maxMonday) maxMonday = mon;
            }
        }
    });

    let prevViable = false;
    let nextViable = false;

    if (isWeekFilterActive) {
        if (minMonday && selectedWeekMonday.getTime() > minMonday.getTime()) {
            prevViable = true;
        }
        // Siguiente es viable si estamos antes de la semana actual o si hay registros futuros
        if (selectedWeekMonday.getTime() < currentMonday.getTime() || (maxMonday && selectedWeekMonday.getTime() < maxMonday.getTime())) {
            nextViable = true;
        }
    }

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

    if (isWeekFilterActive) {
        const weekNum = getISOWeekNumber(selectedWeekMonday);
        if (titleEl) titleEl.textContent = `Semana ${weekNum}`;
        
        const monStr = selectedWeekMonday.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });
        const sunStr = sunday.toLocaleDateString('es-ES', { day: 'numeric', month: 'short', year: 'numeric' });
        if (datesEl) datesEl.textContent = `${monStr} – ${sunStr}`;

        if (currentBadge) {
            if (isCurrent) currentBadge.classList.remove('hidden');
            else currentBadge.classList.add('hidden');
        }

        if (btnCurrent) {
            if (!isCurrent) btnCurrent.classList.remove('hidden');
            else btnCurrent.classList.add('hidden');
        }

        if (btnToggleText) btnToggleText.textContent = 'Ver todas las semanas';

        if (kpiHoursSub) kpiHoursSub.textContent = 'En semana seleccionada';
        if (kpiProjectsSub) kpiProjectsSub.textContent = 'con partes esta semana';
        if (kpiEntriesSub) kpiEntriesSub.textContent = 'imputaciones esta semana';
        if (kpiEmployeesSub) kpiEmployeesSub.textContent = 'con actividad esta semana';
    } else {
        if (titleEl) titleEl.textContent = 'Todas las semanas';
        if (datesEl) datesEl.textContent = 'Mostrando todo el historial de imputaciones sin restricción semanal';
        if (currentBadge) currentBadge.classList.add('hidden');
        if (btnCurrent) btnCurrent.classList.remove('hidden');
        if (btnToggleText) btnToggleText.textContent = 'Ver solo semana actual';

        if (kpiHoursSub) kpiHoursSub.textContent = 'En todo el histórico';
        if (kpiProjectsSub) kpiProjectsSub.textContent = 'con partes en total';
        if (kpiEntriesSub) kpiEntriesSub.textContent = 'imputaciones totales';
        if (kpiEmployeesSub) kpiEmployeesSub.textContent = 'con actividad total';
    }
}

function switchView(viewName) {
    currentView = viewName;

    const btnList = document.getElementById('btn-view-list');
    const btnCal = document.getElementById('btn-view-calendar');
    const btnGantt = document.getElementById('btn-view-gantt');

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

    applyBtnStyle(btnList, viewName === 'list');
    applyBtnStyle(btnCal, viewName === 'calendar');
    applyBtnStyle(btnGantt, viewName === 'gantt');

    if (containerList) containerList.classList.toggle('hidden', viewName !== 'list');
    if (containerCal) containerCal.classList.toggle('hidden', viewName !== 'calendar');
    if (containerGantt) containerGantt.classList.toggle('hidden', viewName !== 'gantt');

    applyTimesheetFilters();
}

function applyTimesheetFilters() {
    const searchInput = document.getElementById('filter-search');
    const projectSelect = document.getElementById('filter-project');
    const employeeSelect = document.getElementById('filter-employee');
    const rows = document.querySelectorAll('.timesheet-row');
    const emptyFilterRow = document.getElementById('empty-filter-row');
    
    const kpiHours = document.getElementById('kpi-total-hours');
    const kpiEntries = document.getElementById('kpi-total-entries');
    const kpiProjects = document.getElementById('kpi-total-projects');
    const kpiEmployees = document.getElementById('kpi-total-employees');

    const searchVal = (searchInput ? searchInput.value : '').toLowerCase().trim();
    const projectVal = (projectSelect ? projectSelect.value : '').toLowerCase().trim();
    const employeeVal = (employeeSelect ? employeeSelect.value : '').toLowerCase().trim();

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

        const matchSearch = !searchVal || desc.includes(searchVal) || task.includes(searchVal) || project.includes(searchVal) || projectName.includes(searchVal);

        let matchProject = true;
        if (targetProjectId && rowProjectId && targetProjectId !== '0' && rowProjectId !== '0') {
            matchProject = (rowProjectId === targetProjectId);
        } else if (targetProjectName) {
            matchProject = (projectName === targetProjectName || project === targetProjectName || projectName.includes(targetProjectName) || project.includes(targetProjectName));
        }

        const matchEmployee = !employeeVal || employee.includes(employeeVal);

        let matchWeek = true;
        if (isWeekFilterActive && rowDateStr) {
            const rowDate = parseISODate(rowDateStr);
            if (rowDate) {
                matchWeek = (rowDate >= selectedWeekMonday && rowDate <= selectedWeekSunday);
            } else {
                matchWeek = false;
            }
        }

        if (matchSearch && matchProject && matchEmployee && matchWeek) {
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
    if (emptyFilterRow) {
        if (visibleCount === 0 && rows.length > 0) {
            emptyFilterRow.classList.remove('hidden');
        } else {
            emptyFilterRow.classList.add('hidden');
        }
    }

    // Actualizar paneles generales de métricas (KPIs)
    if (kpiHours) kpiHours.textContent = visibleHours.toFixed(2);
    if (kpiEntries) kpiEntries.textContent = visibleCount;
    if (kpiProjects) kpiProjects.textContent = visibleProjects.size;
    if (kpiEmployees) kpiEmployees.textContent = visibleEmployees.size;

    highlightSidebarProject(targetProjectName, targetProjectId);
    updateWeekControls();

    // Sincronizar vistas Calendario y Gantt
    if (currentView === 'calendar') {
        renderCalendarView(matchingRows);
    } else if (currentView === 'gantt') {
        renderGanttView(matchingRows);
    }
}

function renderCalendarView(matchingRows) {
    const container = document.getElementById('calendar-grid-days');
    if (!container) return;

    const dayNames = ['Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado', 'Domingo'];
    const today = new Date();

    let html = '';
    for (let i = 0; i < 7; i++) {
        const dayDate = new Date(selectedWeekMonday);
        dayDate.setDate(dayDate.getDate() + i);
        const dayISO = formatISODate(dayDate);
        const isToday = isSameDay(dayDate, today);

        const dayEntries = matchingRows.filter(r => r.date === dayISO);
        const dayTotalHours = dayEntries.reduce((sum, r) => sum + r.hours, 0);

        const dayFormatted = dayDate.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' });

        html += `
        <div class="bg-slate-50/70 rounded-2xl border ${isToday ? 'border-sky-400 ring-2 ring-sky-100 bg-sky-50/20' : 'border-slate-200/80'} p-3 flex flex-col justify-between min-h-[300px] transition-all">
            <div>
                <!-- Cabecera de Día -->
                <div class="flex items-center justify-between pb-2.5 mb-2.5 border-b ${isToday ? 'border-sky-200' : 'border-slate-200/70'}">
                    <div>
                        <span class="text-xs font-bold ${isToday ? 'text-sky-700' : 'text-slate-800'} block">
                            ${dayNames[i]}
                        </span>
                        <span class="text-[11px] text-slate-400 font-medium">
                            ${dayFormatted}
                        </span>
                    </div>
                    <div class="flex items-center space-x-1">
                        ${isToday ? '<span class="text-[9px] font-extrabold uppercase px-1.5 py-0.5 rounded bg-sky-500 text-white tracking-wider">Hoy</span>' : ''}
                        <span class="text-[11px] font-mono font-bold ${dayTotalHours > 0 ? 'text-indigo-700 bg-indigo-50 border border-indigo-100' : 'text-slate-400 bg-slate-100'} px-1.5 py-0.5 rounded-md">
                            ${dayTotalHours.toFixed(2)}h
                        </span>
                    </div>
                </div>

                <!-- Lista de Partes de Horas del Día -->
                <div class="space-y-2">
        `;

        if (dayEntries.length === 0) {
            html += `
                <div class="py-6 text-center text-slate-300">
                    <span class="text-[11px] font-medium block">Sin horas</span>
                </div>
            `;
        } else {
            dayEntries.forEach(r => {
                const safeDesc = escapeHtml(r.desc);
                const safeTask = escapeHtml(r.task);
                const safeProject = escapeHtml(r.projectName);
                const safeEmployee = escapeHtml(r.employee);

                html += `
                <div class="bg-white rounded-xl p-2.5 border border-slate-200/80 shadow-2xs hover:shadow-xs transition group relative">
                    <div class="flex items-start justify-between gap-1 mb-1">
                        <span class="text-[10px] font-bold text-sky-700 bg-sky-50 px-1.5 py-0.5 rounded border border-sky-100 truncate max-w-[120px]" title="${safeProject}">
                            ${safeProject}
                        </span>
                        <span class="text-[11px] font-mono font-extrabold text-slate-700 bg-slate-100 px-1.5 py-0.5 rounded flex-shrink-0">
                            ${r.hoursFormatted}
                        </span>
                    </div>
                    ${safeTask ? `
                    <p class="text-[11px] font-semibold text-slate-800 truncate" title="${safeTask}">
                        ↳ ${safeTask}
                    </p>
                    ` : ''}
                    ${safeDesc ? `
                    <p class="text-[10px] text-slate-500 line-clamp-2 mt-0.5" title="${safeDesc}">
                        ${safeDesc}
                    </p>
                    ` : ''}
                    
                    <div class="flex items-center justify-between pt-2 mt-1.5 border-t border-slate-100 text-[10px] text-slate-400">
                        <span class="truncate max-w-[90px]" title="${safeEmployee}">
                            ${safeEmployee}
                        </span>
                        <div class="flex items-center space-x-1">
                            ${r.invoiced ? `
                            <span title="Parte Facturado en Odoo (No se puede eliminar)" class="p-1 text-slate-400 cursor-not-allowed">
                                <svg class="w-3.5 h-3.5 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
                                </svg>
                            </span>
                            ` : `
                            <button type="button" onclick="openDeleteTimesheetModalFromData('${r.id}', '${r.date}', '${escapeAttr(r.projectName)}', '${escapeAttr(r.task)}', '${r.hours}', '${escapeAttr(r.desc)}')"
                                    class="p-1 text-slate-400 hover:text-rose-600 rounded transition cursor-pointer" title="Eliminar parte no facturado">
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                </svg>
                            </button>
                            `}
                            <button type="button" onclick="openEditTimesheetModalFromRowData('${r.id}', '${r.date}', '${r.projectId}', '${r.taskId}', '${escapeAttr(r.desc)}', '${r.hours}')"
                                    class="p-1 text-slate-400 hover:text-sky-600 rounded transition cursor-pointer" title="Editar parte">
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                </svg>
                            </button>
                        </div>
                    </div>
                </div>
                `;
            });
        }

        html += `
                </div>
            </div>

            <!-- Botón rápido para imputar en este día concreto -->
            <button type="button" onclick="openCreateTimesheetModal('', '', '${dayISO}')"
                    class="w-full mt-3 py-1.5 px-2 bg-white hover:bg-sky-50 text-slate-600 hover:text-sky-700 text-[11px] font-semibold rounded-xl border border-slate-200/80 hover:border-sky-300 transition flex items-center justify-center space-x-1 shadow-2xs cursor-pointer">
                <svg class="w-3 h-3 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
                <span>Imputar</span>
            </button>
        </div>
        `;
    }

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
