/**
 * static/js/app.js
 * Inicialización de la aplicación, listeners de eventos globales y atajos de teclado.
 */

document.addEventListener('DOMContentLoaded', () => {
    const searchInput = document.getElementById('filter-search');
    const projectSelect = document.getElementById('filter-project');
    const employeeSelect = document.getElementById('filter-employee');
    const sidebarEmployeeSelect = document.getElementById('sidebar-employee-select');

    if (searchInput) searchInput.addEventListener('input', applyTimesheetFilters);
    if (projectSelect) {
        projectSelect.addEventListener('change', () => {
            if (projectSelect.value) {
                activeSidebarProjectName = projectSelect.value;
                activeSidebarProjectId = '';
                activeSidebarProject = projectSelect.value;
            } else {
                activeSidebarProjectName = '';
                activeSidebarProjectId = '';
                activeSidebarProject = '';
            }
            applyTimesheetFilters();
        });
    }

    // Sincronizar selectores de empleado
    if (employeeSelect) {
        employeeSelect.addEventListener('change', () => {
            const val = employeeSelect.value;
            if (sidebarEmployeeSelect && sidebarEmployeeSelect.value !== val) {
                sidebarEmployeeSelect.value = val;
            }
            rebuildSidebarProjects(val);
            applyTimesheetFilters();
        });
    }

    if (sidebarEmployeeSelect) {
        sidebarEmployeeSelect.addEventListener('change', () => {
            const val = sidebarEmployeeSelect.value;
            if (employeeSelect && employeeSelect.value !== val) {
                employeeSelect.value = val;
            }
            rebuildSidebarProjects(val);
            applyTimesheetFilters();
        });
    }

    // Buscador en tiempo real dentro del panel lateral de proyectos con recarga desde Odoo
    const sidebarSearch = document.getElementById('sidebar-project-search');
    let searchDebounceTimer = null;

    async function searchOdooProjects(query) {
        if (!query) return;
        try {
            const resp = await fetch(`/api/projects?search=${encodeURIComponent(query)}`);
            if (!resp.ok) return;
            const projects = await resp.json();
            if (!Array.isArray(projects) || projects.length === 0) return;

            const allListContainer = document.getElementById('sidebar-all-projects-list');
            const modalSelect = document.getElementById('modal-project-select');
            const filterSelect = document.getElementById('filter-project');

            projects.forEach(p => {
                const pId = String(p.id);
                const pName = p.display_name || p.name || `Proyecto #${p.id}`;
                const pPartner = (p.partner_id && p.partner_id.name) ? p.partner_id.name : '';

                // Añadir a la lista "Todos los proyectos" del sidebar si es nuevo
                if (allListContainer) {
                    let existing = allListContainer.querySelector(`.sidebar-all-project-item[data-project-id="${pId}"]`);
                    if (!existing) {
                        const btn = document.createElement('button');
                        btn.type = 'button';
                        btn.setAttribute('onclick', 'selectSidebarProject(this.dataset.projectName, this.dataset.projectId)');
                        btn.dataset.projectName = pName;
                        btn.dataset.projectId = pId;
                        btn.className = 'sidebar-all-project-item group w-full text-left p-2.5 rounded-xl border border-slate-200/80 hover:border-indigo-400 hover:bg-indigo-50/60 bg-slate-50/50 transition-all flex flex-col space-y-1 relative cursor-pointer';
                        btn.innerHTML = `
                            <div class="flex items-start justify-between gap-1.5">
                                <span class="text-xs font-bold text-slate-800 group-hover:text-indigo-700 line-clamp-2 leading-tight">
                                    ${escapeHTML(pName)}
                                </span>
                            </div>
                            <div class="flex items-center justify-between text-[10px] text-slate-400 pt-0.5">
                                <span class="truncate max-w-[150px]" title="${escapeHTML(pPartner)}">
                                    ${escapeHTML(pPartner)}
                                </span>
                                <span class="font-mono text-slate-400">#${pId}</span>
                            </div>
                        `;
                        allListContainer.appendChild(btn);
                        existing = btn;
                    }
                    if (existing) {
                        const curQuery = (sidebarSearch ? sidebarSearch.value : '').toLowerCase().trim();
                        existing.style.display = (!curQuery || pName.toLowerCase().includes(curQuery)) ? '' : 'none';
                    }
                }

                // Añadir a los selectores si no existe
                if (modalSelect && !modalSelect.querySelector(`option[value="${pId}"]`)) {
                    const opt = document.createElement('option');
                    opt.value = pId;
                    opt.textContent = pName;
                    modalSelect.appendChild(opt);
                }
                if (filterSelect && !filterSelect.querySelector(`option[value="${pId}"]`)) {
                    const opt = document.createElement('option');
                    opt.value = pId;
                    opt.textContent = pName;
                    filterSelect.appendChild(opt);
                }
            });

            // Si hay resultados y el usuario sigue buscando, asegurar que se muestre en la pestaña "Todos"
            const curQuery = (sidebarSearch ? sidebarSearch.value : '').trim();
            if (curQuery && typeof switchProjectTab === 'function') {
                switchProjectTab('all');
            }
        } catch (err) {
            console.warn('Error al buscar proyectos en Odoo:', err);
        }
    }

    if (sidebarSearch) {
        sidebarSearch.addEventListener('input', () => {
            const query = sidebarSearch.value.toLowerCase().trim();
            const recentItems = document.querySelectorAll('.sidebar-project-item');
            const allItems = document.querySelectorAll('.sidebar-all-project-item');

            recentItems.forEach(item => {
                const name = (item.dataset.projectName || '').toLowerCase();
                if (!query || name.includes(query)) {
                    item.style.display = '';
                } else {
                    item.style.display = 'none';
                }
            });

            allItems.forEach(item => {
                const name = (item.dataset.projectName || '').toLowerCase();
                if (!query || name.includes(query)) {
                    item.style.display = '';
                } else {
                    item.style.display = 'none';
                }
            });

            // Recargar proyectos desde Odoo con debounce para incorporar proyectos recién creados
            if (searchDebounceTimer) clearTimeout(searchDebounceTimer);
            if (query.length >= 2) {
                searchDebounceTimer = setTimeout(() => {
                    searchOdooProjects(query);
                }, 300);
            }
        });
    }

    // Cerrar modal de imputación al hacer clic en el backdrop
    const modalEl = document.getElementById('timesheet-modal');
    if (modalEl) {
        modalEl.addEventListener('click', (e) => {
            if (e.target.id === 'timesheet-modal') {
                closeTimesheetModal();
            }
        });
    }

    // Cerrar modal de borrado al hacer clic en el backdrop
    const delModalEl = document.getElementById('delete-timesheet-modal');
    if (delModalEl) {
        delModalEl.addEventListener('click', (e) => {
            if (e.target.id === 'delete-timesheet-modal') {
                closeDeleteModal();
            }
        });
    }

    // Cerrar modales con la tecla Escape
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            closeTimesheetModal();
            closeDeleteModal();
        }
    });

    // Input y visualización dinámica de horas:minutos
    const hoursInputEl = document.getElementById('modal-hours-input');
    if (hoursInputEl) {
        hoursInputEl.addEventListener('input', () => {
            updateModalTimeBadge();
        });
        hoursInputEl.addEventListener('blur', () => {
            normalizeModalTimeInput();
        });
    }

    // Al pulsar Enter en el modal se considera Aceptar
    const timesheetFormEl = document.getElementById('timesheet-form');
    if (timesheetFormEl) {
        timesheetFormEl.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                // Si está en el campo de crear tarea inline, crear la tarea en vez del envío general
                if (e.target && e.target.id === 'inline-task-name') {
                    e.preventDefault();
                    submitInlineCreateTask();
                    return;
                }
                // En inputs y selects, Enter equivale a Aceptar
                e.preventDefault();
                submitTimesheetForm(e);
            }
        });
    }

    // Inicializar controles de semana y filtros iniciales
    updateWeekControls();
    applyTimesheetFilters();
});
