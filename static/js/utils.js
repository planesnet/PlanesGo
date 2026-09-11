/**
 * static/js/utils.js
 * Funciones de utilidad para fechas ISO, formateo de horas y minutos, y sanitización de cadenas.
 */

// --- MANEJO DE FECHAS ISO Y SEMANAS (LUNES A DOMINGO) ---

function parseISODate(dateStr) {
    if (!dateStr) return null;
    const parts = dateStr.split('-');
    if (parts.length < 3) return null;
    return new Date(parseInt(parts[0], 10), parseInt(parts[1], 10) - 1, parseInt(parts[2], 10), 12, 0, 0);
}

function formatISODate(d) {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${day}`;
}

function getMonday(d) {
    const date = new Date(d);
    const day = date.getDay();
    const diff = date.getDate() - day + (day === 0 ? -6 : 1);
    const monday = new Date(date.setDate(diff));
    monday.setHours(0, 0, 0, 0);
    return monday;
}

function getSunday(monday) {
    const sunday = new Date(monday);
    sunday.setDate(sunday.getDate() + 6);
    sunday.setHours(23, 59, 59, 999);
    return sunday;
}

function getISOWeekNumber(d) {
    const date = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const dayNum = date.getUTCDay() || 7;
    date.setUTCDate(date.getUTCDate() + 4 - dayNum);
    const yearStart = new Date(Date.UTC(date.getUTCFullYear(), 0, 1));
    return Math.ceil((((date - yearStart) / 86400000) + 1) / 7);
}

function isSameDay(d1, d2) {
    return d1.getFullYear() === d2.getFullYear() &&
           d1.getMonth() === d2.getMonth() &&
           d1.getDate() === d2.getDate();
}

function isSameWeek(m1, m2) {
    return isSameDay(m1, m2);
}

// --- FORMATEO Y PARSEO DE HORAS:MINUTOS / DECIMAL ---

function parseTimeToDecimal(val) {
    if (!val) return 0;
    val = String(val).trim().toLowerCase();
    if (!val) return 0;

    // Caso 1: Horas:Minutos (ej: "1:30", "01:30", "0:45", ":30", "2:00")
    if (val.includes(':')) {
        const parts = val.split(':');
        const h = parts[0] === '' ? 0 : parseInt(parts[0], 10);
        const m = parts[1] === '' ? 0 : parseInt(parts[1], 10);
        if (isNaN(h) || isNaN(m) || m < 0) return NaN;
        return Math.round((h + m / 60) * 100) / 100;
    }

    // Caso 2: Formato texto estilo "1h 30m", "1h", "45m"
    if (val.includes('h') || val.includes('m')) {
        let hours = 0;
        let mins = 0;
        const hMatch = val.match(/(\d+(?:[.,]\d+)?)\s*h/);
        const mMatch = val.match(/(\d+)\s*m/);
        if (hMatch) hours = parseFloat(hMatch[1].replace(',', '.'));
        if (mMatch) mins = parseInt(mMatch[1], 10);
        if (!hMatch && !mMatch) return NaN;
        return Math.round((hours + mins / 60) * 100) / 100;
    }

    // Caso 3: Número decimal o entero (ej: "1.5", "1,5", "2")
    val = val.replace(',', '.');
    const num = parseFloat(val);
    if (isNaN(num)) return NaN;
    return Math.round(num * 100) / 100;
}

function formatDecimalToTime(decimalVal) {
    if (decimalVal === null || decimalVal === undefined || isNaN(decimalVal) || decimalVal <= 0) return '';
    const totalMinutes = Math.round(decimalVal * 60);
    const hours = Math.floor(totalMinutes / 60);
    const mins = totalMinutes % 60;
    return `${hours}:${String(mins).padStart(2, '0')}`;
}

function updateModalTimeBadge() {
    const input = document.getElementById('modal-hours-input');
    const badge = document.getElementById('modal-time-badge');
    if (!input || !badge) return;

    const raw = input.value.trim();
    if (!raw) {
        badge.innerText = '0h 00m (0.00h)';
        badge.className = 'font-mono text-slate-500 bg-slate-100 px-2 py-0.5 rounded text-[11px] font-semibold';
        return;
    }

    const dec = parseTimeToDecimal(raw);
    if (isNaN(dec) || dec < 0) {
        badge.innerText = 'Formato inválido';
        badge.className = 'font-mono text-rose-600 bg-rose-50 px-2 py-0.5 rounded text-[11px] font-semibold';
        return;
    }

    const totalMinutes = Math.round(dec * 60);
    const h = Math.floor(totalMinutes / 60);
    const m = totalMinutes % 60;
    badge.innerText = `${h}h ${String(m).padStart(2, '0')}m (${dec.toFixed(2)}h dec)`;
    badge.className = 'font-mono text-sky-700 bg-sky-50 border border-sky-200 px-2 py-0.5 rounded text-[11px] font-semibold';
}

function normalizeModalTimeInput() {
    const input = document.getElementById('modal-hours-input');
    if (!input) return;
    const raw = input.value.trim();
    if (!raw) return;
    const dec = parseTimeToDecimal(raw);
    if (!isNaN(dec) && dec > 0) {
        input.value = formatDecimalToTime(dec);
        updateModalTimeBadge();
    }
}

function addModalTimeMinutes(minutesToAdd) {
    const input = document.getElementById('modal-hours-input');
    if (!input) return;
    const currentDecimal = parseTimeToDecimal(input.value) || 0;
    const currentMinutes = Math.round(currentDecimal * 60);
    const newMinutes = Math.max(0, currentMinutes + minutesToAdd);
    input.value = formatDecimalToTime(newMinutes / 60);
    updateModalTimeBadge();
    input.focus();
}

function addModalHours(amount) {
    addModalTimeMinutes(Math.round(amount * 60));
}

// --- SANITIZACIÓN Y FORMATEO VISUAL ---

function escapeHtml(str) {
    if (!str) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

function escapeAttr(str) {
    if (!str) return '';
    return String(str)
        .replace(/'/g, "\\'")
        .replace(/"/g, '&quot;')
        .replace(/\n/g, ' ');
}

function escapeHTML(str) {
    return escapeHtml(str);
}

function formatHoursJS(totalHours) {
    const hours = Math.floor(totalHours);
    const minutes = Math.round((totalHours - hours) * 60);
    if (minutes === 0) {
        return `${hours}h`;
    }
    return `${hours}h ${String(minutes).padStart(2, '0')}m`;
}

function formatDateJS(dateStr) {
    if (!dateStr) return '-';
    const parts = dateStr.split('-');
    if (parts.length === 3) {
        return `${parts[2]}/${parts[1]}/${parts[0]}`;
    }
    return dateStr;
}

function getActiveWorkerName() {
    const sidebarSelect = document.getElementById('sidebar-employee-select');
    if (sidebarSelect && sidebarSelect.value && sidebarSelect.value.trim() && sidebarSelect.value.trim().toLowerCase() !== 'todos') {
        return sidebarSelect.value.trim();
    }
    const bodyWorker = document.body ? document.body.dataset.currentWorker : '';
    if (bodyWorker && bodyWorker.trim() && bodyWorker.trim() !== 'Trabajador') {
        return bodyWorker.trim();
    }
    const sidebarName = document.getElementById('sidebar-worker-name')?.textContent?.trim();
    if (sidebarName && sidebarName !== 'Trabajador') {
        return sidebarName;
    }
    const badge = document.querySelector('.timesheet-row[data-employee]');
    if (badge && badge.dataset.employee && badge.dataset.employee.trim()) {
        return badge.dataset.employee.trim();
    }
    return 'Yo';
}
window.getActiveWorkerName = getActiveWorkerName;
