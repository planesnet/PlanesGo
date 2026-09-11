/**
 * static/js/views.js
 * Lógica de navegación por semanas, selector de vistas (Lista, Calendario, Gantt),
 * cálculo de KPIs reactivos y renderizado de las vistas visuales.
 */

const currentMonday = getMonday(new Date());
let selectedWeekMonday = new Date(currentMonday);
let currentView = 'list';

function navigateWeek(delta) {
    selectedWeekMonday.setDate(selectedWeekMonday.getDate() + delta * 7);
    updateWeekControls();
    applyTimesheetFilters();
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

    if (minMonday && selectedWeekMonday.getTime() > minMonday.getTime()) {
        prevViable = true;
    }
    // Siguiente es viable si estamos antes de la semana actual o si hay registros futuros
    if (selectedWeekMonday.getTime() < currentMonday.getTime() || (maxMonday && selectedWeekMonday.getTime() < maxMonday.getTime())) {
        nextViable = true;
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
        if (targetProjectId && rowProjectId && targetProjectId !== '0' && rowProjectId !== '0') {
            matchProject = (rowProjectId === targetProjectId);
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

        // Si el temporizador está corriendo para el usuario, debe ser SIEMPRE visible en la semana actual
        const shouldShow = isTimerRunning ? (matchEmployee && (matchWeek || !rowDateStr)) : (matchSearch && matchProject && matchEmployee && matchWeek);

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
    if (emptyFilterRow) {
        if (visibleCount === 0 && rows.length > 0) {
            emptyFilterRow.classList.remove('hidden');
        } else {
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

    const emptyRow = document.getElementById('empty-row');
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

    // Sincronizar vistas Calendario y Gantt
    if (currentView === 'calendar') {
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
