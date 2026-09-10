/**
 * static/js/sidebar.js
 * Lógica del panel lateral de proyectos recientes y todos los proyectos.
 */

let activeSidebarProjectName = '';
let activeSidebarProjectId = '';
let activeSidebarProject = '';

function switchSidebarTab(tab) {
    const btnRecent = document.getElementById('sidebar-tab-btn-recent');
    const btnAll = document.getElementById('sidebar-tab-btn-all');
    const listRecent = document.getElementById('sidebar-projects-list');
    const listAll = document.getElementById('sidebar-all-projects-list');

    if (tab === 'all') {
        if (btnAll) {
            btnAll.className = "flex-1 py-1 text-center font-bold text-xs rounded-lg transition-all bg-white text-indigo-700 shadow-2xs";
        }
        if (btnRecent) {
            btnRecent.className = "flex-1 py-1 text-center font-medium text-xs text-slate-500 hover:text-slate-800 rounded-lg transition-all";
        }
        if (listAll) listAll.classList.remove('hidden');
        if (listRecent) listRecent.classList.add('hidden');
    } else {
        if (btnRecent) {
            btnRecent.className = "flex-1 py-1 text-center font-bold text-xs rounded-lg transition-all bg-white text-sky-700 shadow-2xs";
        }
        if (btnAll) {
            btnAll.className = "flex-1 py-1 text-center font-medium text-xs text-slate-500 hover:text-slate-800 rounded-lg transition-all";
        }
        if (listRecent) listRecent.classList.remove('hidden');
        if (listAll) listAll.classList.add('hidden');
    }
}

function highlightSidebarProject(projectName, projectId) {
    const recentItems = document.querySelectorAll('.sidebar-project-item');
    const allItems = document.querySelectorAll('.sidebar-all-project-item');

    const targetId = projectId ? String(projectId) : '';
    const targetName = projectName ? projectName.toLowerCase().trim() : '';

    const applyHighlight = (btn) => {
        const bId = btn.dataset.projectId || '';
        const bName = (btn.dataset.projectName || '').toLowerCase().trim();

        let isMatch = false;
        if (targetId && bId && targetId !== '0' && bId !== '0') {
            isMatch = (bId === targetId);
        } else if (targetName && bName) {
            isMatch = (bName === targetName || bName.includes(targetName) || targetName.includes(bName));
        }

        // Limpiar cualquier clase residual inline de versiones anteriores
        btn.classList.remove('!bg-blue-600', '!border-blue-700', 'shadow-md', 'ring-2', 'ring-blue-400');
        btn.querySelectorAll('*').forEach(el => {
            el.classList.remove('!text-white', '!text-blue-100', '!text-blue-200', '!bg-blue-700', '!bg-blue-800', '!border-blue-500');
        });

        if (isMatch) {
            btn.classList.add('is-active-project-card');
        } else {
            btn.classList.remove('is-active-project-card');
        }
    };

    recentItems.forEach(btn => applyHighlight(btn));
    allItems.forEach(btn => applyHighlight(btn));
}

function selectSidebarProject(projectName, projectId) {
    const pId = projectId ? String(projectId) : '';
    const pName = projectName ? projectName.trim() : '';

    // Toggle off si se vuelve a clickear el mismo proyecto activo
    if ((activeSidebarProjectId && pId && activeSidebarProjectId === pId) ||
        (!activeSidebarProjectId && !pId && activeSidebarProjectName && pName && activeSidebarProjectName.toLowerCase() === pName.toLowerCase())) {
        clearSidebarProjectFilter();
        return;
    }

    activeSidebarProjectName = pName;
    activeSidebarProjectId = pId;
    activeSidebarProject = pName;

    // Limpiar buscador de texto libre para no restringir las imputaciones del proyecto
    const searchInput = document.getElementById('filter-search');
    if (searchInput) searchInput.value = '';

    // Sincronizar select si existe la opción
    const projectSelect = document.getElementById('filter-project');
    if (projectSelect) {
        let foundOption = false;
        for (let i = 0; i < projectSelect.options.length; i++) {
            const optVal = projectSelect.options[i].value.toLowerCase().trim();
            if (optVal && (optVal === pName.toLowerCase() || (pId && optVal === pId))) {
                projectSelect.selectedIndex = i;
                foundOption = true;
                break;
            }
        }
        if (!foundOption) {
            projectSelect.value = '';
        }
    }

    const btnLabel = document.getElementById('btn-imputar-label');
    if (btnLabel) {
        btnLabel.innerText = pName ? `Imputar en ${pName.length > 15 ? pName.slice(0, 14) + '...' : pName}` : 'Imputar Horas';
    }

    applyTimesheetFilters();
}

function clearSidebarProjectFilter() {
    activeSidebarProjectName = '';
    activeSidebarProjectId = '';
    activeSidebarProject = '';

    const btnLabel = document.getElementById('btn-imputar-label');
    if (btnLabel) btnLabel.innerText = 'Imputar Horas';

    const projectSelect = document.getElementById('filter-project');
    if (projectSelect) projectSelect.value = '';

    applyTimesheetFilters();
}

function filterByProjectName(projectName) {
    selectSidebarProject(projectName, '');
}

function rebuildSidebarProjects(workerName) {
    const rows = document.querySelectorAll('.timesheet-row');
    const container = document.getElementById('sidebar-projects-list');
    const countBadge = document.getElementById('sidebar-project-count');
    const workerLabel = document.getElementById('sidebar-worker-name');

    if (workerLabel) {
        workerLabel.textContent = workerName ? workerName : 'Trabajador';
        workerLabel.title = workerName ? workerName : 'Trabajador';
    }

    if (!container) return;

    const projectsMap = new Map();
    const projectOrder = [];

    rows.forEach(row => {
        const emp = (row.dataset.employee || '').trim();
        if (workerName && emp.toLowerCase() !== workerName.toLowerCase()) {
            return;
        }

        const pName = (row.dataset.projectName || row.dataset.project || '').trim();
        const pId = (row.dataset.projectId || '').trim();
        if (!pName || pName === '-') return;

        const hours = parseFloat(row.dataset.hours) || 0;
        const date = row.dataset.date || '';
        const task = row.dataset.task || '';

        if (projectsMap.has(pName)) {
            const existing = projectsMap.get(pName);
            existing.totalHours += hours;
            existing.entryCount += 1;
            if (pId && !existing.id) existing.id = pId;
        } else {
            const item = {
                id: pId,
                name: pName,
                lastDate: date,
                totalHours: hours,
                entryCount: 1,
                lastTask: task
            };
            projectsMap.set(pName, item);
            projectOrder.push(pName);
        }
    });

    if (countBadge) {
        countBadge.textContent = projectOrder.length;
    }

    if (projectOrder.length === 0) {
        container.innerHTML = `
            <div class="py-6 px-3 text-center text-slate-400">
                <svg class="w-8 h-8 mx-auto text-slate-300 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                </svg>
                <p class="text-xs font-medium text-slate-500">Sin proyectos registrados</p>
                <p class="text-[11px] text-slate-400 mt-0.5">No hay partes de horas para este trabajador</p>
            </div>
        `;
        highlightSidebarProject('', '');
        return;
    }

    let html = '';
    projectOrder.forEach(pName => {
        const p = projectsMap.get(pName);
        const safeName = escapeHTML(p.name);
        const safeTask = escapeHTML(p.lastTask);
        const safeId = escapeHTML(p.id || '');
        const formattedHours = formatHoursJS(p.totalHours);

        html += `
            <button type="button"
                    onclick="selectSidebarProject(this.dataset.projectName, this.dataset.projectId)"
                    data-project-name="${safeName}"
                    data-project-id="${safeId}"
                    class="sidebar-project-item group w-full text-left p-2.5 rounded-xl border border-slate-200/80 hover:border-sky-400 hover:bg-sky-50/60 bg-slate-50/50 transition-all flex flex-col space-y-1 relative cursor-pointer">
                <div class="flex items-start justify-between gap-1.5">
                    <span class="text-xs font-bold text-slate-800 group-hover:text-sky-700 line-clamp-2 leading-tight">
                        ${safeName}
                    </span>
                    ${p.totalHours > 0 ? `
                    <span class="text-[10px] font-extrabold text-sky-700 bg-white px-1.5 py-0.5 rounded-md border border-slate-200/80 shadow-2xs font-mono flex-shrink-0">
                        ${formattedHours}
                    </span>` : ''}
                </div>
                ${safeTask ? `
                <div class="text-[10px] text-slate-500 truncate bg-slate-100/80 px-1.5 py-0.5 rounded font-mono">
                    ↳ ${safeTask}
                </div>
                ` : ''}
            </button>
        `;
    });

    container.innerHTML = html;
    highlightSidebarProject(activeSidebarProjectName, activeSidebarProjectId);
}
