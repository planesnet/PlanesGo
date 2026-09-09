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

    // Buscador en tiempo real dentro del panel lateral de proyectos
    const sidebarSearch = document.getElementById('sidebar-project-search');
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
