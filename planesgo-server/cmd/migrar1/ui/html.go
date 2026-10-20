package ui

const IndexHTML = `<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Migrar1 - Traspaso Odoo Idempotente</title>
    <style>
        :root {
            --bg-primary: #0f172a;
            --bg-secondary: #1e293b;
            --bg-card: #1e293b;
            --bg-hover: #334155;
            --border-color: #334155;
            --text-main: #f8fafc;
            --text-muted: #94a3b8;
            --accent-primary: #3b82f6;
            --accent-hover: #2563eb;
            --success: #10b981;
            --warning: #f59e0b;
            --danger: #ef4444;
            --purple: #8b5cf6;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
        }

        body {
            background-color: var(--bg-primary);
            color: var(--text-main);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }

        header {
            background-color: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
            padding: 1rem 2rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .brand {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .brand-badge {
            background: linear-gradient(135deg, #3b82f6, #8b5cf6);
            color: white;
            font-weight: 700;
            font-size: 0.85rem;
            padding: 0.25rem 0.6rem;
            border-radius: 6px;
        }

        .brand h1 {
            font-size: 1.25rem;
            font-weight: 700;
            letter-spacing: -0.02em;
        }

        .top-stats {
            display: flex;
            gap: 1.5rem;
            font-size: 0.85rem;
        }

        .stat-item {
            display: flex;
            align-items: center;
            gap: 0.4rem;
        }

        .stat-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }

        .dot-green { background-color: var(--success); }
        .dot-yellow { background-color: var(--warning); }
        .dot-red { background-color: var(--danger); }
        .dot-blue { background-color: var(--accent-primary); }

        .tabs {
            display: flex;
            background-color: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
            padding: 0 2rem;
            gap: 0.5rem;
        }

        .tab-btn {
            background: none;
            border: none;
            color: var(--text-muted);
            padding: 0.85rem 1.25rem;
            font-size: 0.9rem;
            font-weight: 600;
            cursor: pointer;
            border-bottom: 2px solid transparent;
            transition: all 0.2s;
        }

        .tab-btn:hover {
            color: var(--text-main);
        }

        .tab-btn.active {
            color: var(--accent-primary);
            border-bottom-color: var(--accent-primary);
        }

        main {
            padding: 1.5rem 2rem;
            flex: 1;
            max-width: 1400px;
            margin: 0 auto;
            width: 100%;
        }

        .tab-content {
            display: none;
        }

        .tab-content.active {
            display: block;
        }

        .grid-2 {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
            margin-bottom: 1.5rem;
        }

        .grid-3 {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 1.5rem;
            margin-bottom: 1.5rem;
        }

        .card {
            background-color: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 1.25rem;
        }

        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1rem;
            padding-bottom: 0.5rem;
            border-bottom: 1px solid var(--border-color);
        }

        .card-title {
            font-size: 1rem;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .form-group {
            margin-bottom: 0.85rem;
        }

        .form-group label {
            display: block;
            font-size: 0.8rem;
            color: var(--text-muted);
            margin-bottom: 0.25rem;
        }

        .form-control {
            width: 100%;
            background-color: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 0.55rem 0.75rem;
            color: var(--text-main);
            font-size: 0.85rem;
        }

        .form-control:focus {
            outline: none;
            border-color: var(--accent-primary);
        }

        .btn {
            background-color: var(--accent-primary);
            color: white;
            border: none;
            padding: 0.6rem 1.1rem;
            border-radius: 6px;
            font-size: 0.85rem;
            font-weight: 600;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 0.4rem;
            transition: all 0.2s;
        }

        .btn:hover {
            background-color: var(--accent-hover);
        }

        .btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .btn-success { background-color: var(--success); }
        .btn-success:hover { background-color: #059669; }

        .btn-warning { background-color: var(--warning); color: #000; }
        .btn-warning:hover { background-color: #d97706; }

        .btn-secondary { background-color: var(--bg-hover); color: var(--text-main); }
        .btn-secondary:hover { background-color: #475569; }

        .btn-danger { background-color: var(--danger); }
        .btn-danger:hover { background-color: #dc2626; }

        .btn-group {
            display: flex;
            gap: 0.75rem;
            flex-wrap: wrap;
        }

        .console-box {
            background-color: #090d16;
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 1rem;
            height: 320px;
            overflow-y: auto;
            font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
            font-size: 0.8rem;
            color: #e2e8f0;
            white-space: pre-wrap;
            word-break: break-all;
        }

        .log-entry {
            margin-bottom: 0.35rem;
            line-height: 1.4;
        }

        .log-INFO { color: #60a5fa; }
        .log-WARN { color: #f59e0b; }
        .log-ERROR { color: #f87171; font-weight: bold; }
        .log-DEBUG { color: #94a3b8; }

        table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.85rem;
            text-align: left;
        }

        th {
            background-color: var(--bg-primary);
            padding: 0.75rem;
            color: var(--text-muted);
            border-bottom: 1px solid var(--border-color);
            font-weight: 600;
        }

        td {
            padding: 0.75rem;
            border-bottom: 1px solid var(--border-color);
        }

        tr:hover td {
            background-color: var(--bg-hover);
        }

        .badge {
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
            display: inline-block;
        }

        .badge-synced { background: rgba(16, 185, 129, 0.2); color: #34d399; }
        .badge-pending { background: rgba(245, 158, 11, 0.2); color: #fbbf24; }
        .badge-error { background: rgba(239, 68, 68, 0.2); color: #f87171; }
        .badge-resolved { background: rgba(59, 130, 246, 0.2); color: #60a5fa; }

        .progress-bar-container {
            background-color: var(--bg-primary);
            border-radius: 6px;
            height: 12px;
            width: 100%;
            overflow: hidden;
            margin: 1rem 0 0.5rem 0;
        }

        .progress-bar {
            background: linear-gradient(90deg, var(--accent-primary), var(--success));
            height: 100%;
            width: 0%;
            transition: width 0.3s;
        }

        .modal-overlay {
            position: fixed;
            top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.7);
            display: none;
            justify-content: center;
            align-items: center;
            z-index: 100;
        }

        .modal {
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 1.5rem;
            width: 500px;
            max-width: 90%;
        }
    </style>
</head>
<body>

    <header>
        <div class="brand">
            <span class="brand-badge">MIGRAR1</span>
            <h1>Traspaso Idempotente de Tickets y Partes de Horas</h1>
        </div>
        <div class="top-stats" id="headerStats">
            <div class="stat-item"><span class="stat-dot dot-blue"></span> Tickets: <b id="statTickets">0</b></div>
            <div class="stat-item"><span class="stat-dot dot-green"></span> Migrados: <b id="statSynced">0</b></div>
            <div class="stat-item"><span class="stat-dot dot-yellow"></span> Pendientes: <b id="statPending">0</b></div>
            <div class="stat-item"><span class="stat-dot dot-red"></span> Errores: <b id="statErrors">0</b></div>
            <div class="stat-item"><span class="stat-dot dot-purple"></span> Horas: <b id="statTimesheets">0</b></div>
        </div>
    </header>

    <div class="tabs">
        <button class="tab-btn active" onclick="showTab('dashboard')">Panel de Control y Migración</button>
        <button class="tab-btn" onclick="showTab('mappings')">Mapeo de Maestros (Clientes / Usuarios / Proyectos)</button>
        <button class="tab-btn" onclick="showTab('records')">Registros Sincronizados (SQLite)</button>
        <button class="tab-btn" onclick="showTab('logs')">Auditoría y Logs</button>
    </div>

    <main>
        <!-- TAB 1: DASHBOARD -->
        <div id="tab-dashboard" class="tab-content active">
            <div class="grid-2">
                <!-- Conexión Origen -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title">🌐 Odoo Origen (Lectura)</span>
                        <span id="srcStatusBadge" class="badge badge-pending">Desconectado</span>
                    </div>
                    <div class="form-group">
                        <label>URL Odoo Origen</label>
                        <input id="srcURL" class="form-control" value="https://planesnet.autopyme.com">
                    </div>
                    <div class="form-group">
                        <label>Base de Datos</label>
                        <input id="srcDB" class="form-control" value="ap113">
                    </div>
                    <div class="grid-2" style="margin-bottom:0">
                        <div class="form-group">
                            <label>Usuario</label>
                            <input id="srcUser" class="form-control" placeholder="usuario@empresa.com">
                        </div>
                        <div class="form-group">
                            <label>Contraseña / API Key</label>
                            <input id="srcPass" type="password" class="form-control" placeholder="••••••••">
                        </div>
                    </div>
                </div>

                <!-- Conexión Destino -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title">🎯 Odoo Destino (Escritura Idempotente)</span>
                        <span id="tgtStatusBadge" class="badge badge-pending">Desconectado</span>
                    </div>
                    <div class="form-group">
                        <label>URL Odoo Destino</label>
                        <input id="tgtURL" class="form-control" value="https://pasi-test.autopyme.com">
                    </div>
                    <div class="form-group">
                        <label>Base de Datos</label>
                        <input id="tgtDB" class="form-control" value="pasi">
                    </div>
                    <div class="grid-2" style="margin-bottom:0">
                        <div class="form-group">
                            <label>Usuario</label>
                            <input id="tgtUser" class="form-control" placeholder="usuario@empresa.com">
                        </div>
                        <div class="form-group">
                            <label>Contraseña / API Key</label>
                            <input id="tgtPass" type="password" class="form-control" placeholder="••••••••">
                        </div>
                    </div>
                </div>
            </div>

            <!-- Parámetros y Acciones -->
            <div class="card" style="margin-bottom: 1.5rem;">
                <div class="card-header">
                    <span class="card-title">⚡ Controles de Ejecución y Depuración</span>
                    <div>
                        <span style="font-size: 0.8rem; color: var(--text-muted); margin-right: 0.5rem;">Fecha Inicio:</span>
                        <input id="filterStartDate" type="date" value="2025-01-01" style="background:var(--bg-primary); color:#fff; border:1px solid var(--border-color); padding:0.25rem 0.5rem; border-radius:4px;">
                    </div>
                </div>

                <div class="btn-group">
                    <button id="btnTestConn" class="btn btn-secondary" onclick="testConnections()">🔌 Probar Conexiones</button>
                    <button id="btnDryRun" class="btn btn-warning" onclick="runDryRun()">🔍 Simular (Dry-Run)</button>
                    <button id="btnMigrateOne" class="btn btn-secondary" onclick="openMigrateOneModal()">🐛 Migrar 1 Ticket (Debug)</button>
                    <button id="btnMigrateBatch" class="btn btn-secondary" onclick="runBatch(10)">📦 Migrar Lote (10)</button>
                    <button id="btnMigrateAll" class="btn btn-success" onclick="runFullSync()">🚀 Sincronización Total</button>
                    <button id="btnClearLogs" class="btn btn-secondary" onclick="clearConsole()">🧹 Limpiar Consola</button>
                </div>

                <div class="progress-bar-container">
                    <div id="migrationProgressBar" class="progress-bar"></div>
                </div>
                <div style="display:flex; justify-content:space-between; font-size:0.8rem; color:var(--text-muted);">
                    <span id="progressText">Listo para operar</span>
                    <span id="progressPercent">0%</span>
                </div>
            </div>

            <!-- Consola de Eventos en Tiempo Real -->
            <div class="card">
                <div class="card-header">
                    <span class="card-title">📟 Consola de Salida y Registro de Operaciones en Vivo</span>
                    <button class="btn btn-secondary" style="padding:0.25rem 0.5rem; font-size:0.75rem;" onclick="refreshLogs()">Actualizar</button>
                </div>
                <div id="consoleBox" class="console-box">
                    <div class="log-entry log-INFO">[SISTEMA] Migrar1 iniciado y listo. Base de datos SQLite inicializada.</div>
                </div>
            </div>
        </div>

        <!-- TAB 2: MAPEOS DE MAESTROS -->
        <div id="tab-mappings" class="tab-content">
            <div class="card">
                <div class="card-header">
                    <span class="card-title">📑 Mapeos de Clientes (Partners), Usuarios y Proyectos</span>
                    <div class="btn-group">
                        <button class="btn btn-secondary" onclick="loadMappings()">🔄 Recargar Mapeos</button>
                        <button class="btn btn-secondary" onclick="exportMappingsJSON()">💾 Descargar mappings.json</button>
                    </div>
                </div>
                <p style="font-size:0.85rem; color:var(--text-muted); margin-bottom:1rem;">
                    <b>Regla de Oro:</b> No se crea ningún Partner ni Usuario en destino. Si un cliente no coincide automáticamente por CIF, Email o Nombre, asígnalo manualmente introduciendo su ID de destino.
                </p>
                <div style="overflow-x:auto;">
                    <table>
                        <thead>
                            <tr>
                                <th>Entidad</th>
                                <th>ID Origen</th>
                                <th>Nombre en Origen</th>
                                <th>Clave (CIF / Email)</th>
                                <th>ID Destino</th>
                                <th>Nombre en Destino</th>
                                <th>Criterio</th>
                                <th>Estado</th>
                                <th>Acción</th>
                            </tr>
                        </thead>
                        <tbody id="mappingsTableBody">
                            <tr><td colspan="9" style="text-align:center; color:var(--text-muted);">Cargando mapeos...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <!-- TAB 3: REGISTROS SINCRONIZADOS -->
        <div id="tab-records" class="tab-content">
            <div class="card">
                <div class="card-header">
                    <span class="card-title">🗄️ Trazabilidad de Registros Sincronizados (SQLite Local)</span>
                    <button class="btn btn-secondary" onclick="loadSyncRecords()">🔄 Actualizar Registros</button>
                </div>
                <div style="overflow-x:auto;">
                    <table>
                        <thead>
                            <tr>
                                <th>Entidad</th>
                                <th>ID Origen</th>
                                <th>ID Destino</th>
                                <th>Hash Contenido</th>
                                <th>Estado</th>
                                <th>Error / Detalle</th>
                                <th>Última Sincronización</th>
                            </tr>
                        </thead>
                        <tbody id="syncRecordsTableBody">
                            <tr><td colspan="7" style="text-align:center; color:var(--text-muted);">Cargando registros...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <!-- TAB 4: LOGS -->
        <div id="tab-logs" class="tab-content">
            <div class="card">
                <div class="card-header">
                    <span class="card-title">📜 Historial Completo de Auditoría y Errores RPC</span>
                    <button class="btn btn-secondary" onclick="loadFullLogs()">🔄 Recargar</button>
                </div>
                <div style="overflow-x:auto;">
                    <table>
                        <thead>
                            <tr>
                                <th>Fecha/Hora</th>
                                <th>Nivel</th>
                                <th>Entidad</th>
                                <th>ID Origen</th>
                                <th>ID Destino</th>
                                <th>Mensaje</th>
                                <th>Detalle / Payload</th>
                            </tr>
                        </thead>
                        <tbody id="logsTableBody">
                            <tr><td colspan="7" style="text-align:center; color:var(--text-muted);">Cargando logs...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </main>

    <!-- MODAL MIGRAR 1 TICKET -->
    <div id="migrateOneModal" class="modal-overlay">
        <div class="modal">
            <h3 style="margin-bottom:1rem;">🐛 Migrar 1 Ticket Individual para Depuración</h3>
            <div class="form-group">
                <label>ID del Ticket en Origen (planesnet.autopyme.com)</label>
                <input id="singleTicketID" type="number" class="form-control" placeholder="Ej: 14502">
            </div>
            <div style="display:flex; justify-content:flex-end; gap:0.5rem; margin-top:1.5rem;">
                <button class="btn btn-secondary" onclick="closeMigrateOneModal()">Cancelar</button>
                <button class="btn btn-warning" onclick="executeMigrateOne()">Migrar Ticket y Horas</button>
            </div>
        </div>
    </div>

    <!-- MODAL ASIGNAR MAPEO MANUAL -->
    <div id="editMappingModal" class="modal-overlay">
        <div class="modal">
            <h3 style="margin-bottom:1rem;">✏️ Asignar Mapeo Manual</h3>
            <input type="hidden" id="editMappingEntityType">
            <input type="hidden" id="editMappingSourceID">
            <div class="form-group">
                <label>Nombre en Origen</label>
                <input id="editMappingSourceName" class="form-control" disabled>
            </div>
            <div class="form-group">
                <label>ID Destino (en pasi-test.autopyme.com)</label>
                <input id="editMappingTargetID" type="number" class="form-control" placeholder="Introduce ID de Odoo destino">
            </div>
            <div class="form-group">
                <label>Nombre / Referencia Destino (opcional)</label>
                <input id="editMappingTargetName" class="form-control" placeholder="Nombre en destino">
            </div>
            <div style="display:flex; justify-content:flex-end; gap:0.5rem; margin-top:1.5rem;">
                <button class="btn btn-secondary" onclick="closeEditMappingModal()">Cancelar</button>
                <button class="btn btn-success" onclick="saveManualMapping()">Guardar Mapeo</button>
            </div>
        </div>
    </div>

    <script>
        function showTab(tabName) {
            document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            event.currentTarget.classList.add('active');
            document.getElementById('tab-' + tabName).classList.add('active');

            if (tabName === 'mappings') loadMappings();
            if (tabName === 'records') loadSyncRecords();
            if (tabName === 'logs') loadFullLogs();
        }

        function logToConsole(level, msg) {
            const box = document.getElementById('consoleBox');
            const entry = document.createElement('div');
            entry.className = 'log-entry log-' + level;
            const time = new Date().toLocaleTimeString();
            entry.textContent = '[' + time + '] [' + level + '] ' + msg;
            box.appendChild(entry);
            box.scrollTop = box.scrollHeight;
        }

        function clearConsole() {
            document.getElementById('consoleBox').innerHTML = '';
        }

        function getCredentials() {
            return {
                source: {
                    url: document.getElementById('srcURL').value.trim(),
                    db: document.getElementById('srcDB').value.trim(),
                    username: document.getElementById('srcUser').value.trim(),
                    password: document.getElementById('srcPass').value.trim(),
                },
                target: {
                    url: document.getElementById('tgtURL').value.trim(),
                    db: document.getElementById('tgtDB').value.trim(),
                    username: document.getElementById('tgtUser').value.trim(),
                    password: document.getElementById('tgtPass').value.trim(),
                },
                startDate: document.getElementById('filterStartDate').value
            };
        }

        async function updateStats() {
            try {
                const res = await fetch('/api/stats');
                const data = await res.json();
                document.getElementById('statTickets').textContent = data.total_tickets || 0;
                document.getElementById('statSynced').textContent = data.synced_tickets || 0;
                document.getElementById('statPending').textContent = data.pending_tickets || 0;
                document.getElementById('statErrors').textContent = data.error_tickets || 0;
                document.getElementById('statTimesheets').textContent = data.synced_timesheets || 0;
            } catch (e) {}
        }

        async function testConnections() {
            logToConsole('INFO', 'Probando conexión con Origen y Destino...');
            const creds = getCredentials();
            try {
                const res = await fetch('/api/test-connections', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(creds)
                });
                const result = await res.json();
                
                const srcBadge = document.getElementById('srcStatusBadge');
                if (result.source_ok) {
                    srcBadge.className = 'badge badge-synced';
                    srcBadge.textContent = 'Conectado (UID: ' + result.source_uid + ')';
                    logToConsole('INFO', '✓ Origen conectado con éxito: ' + creds.source.url);
                } else {
                    srcBadge.className = 'badge badge-error';
                    srcBadge.textContent = 'Error';
                    logToConsole('ERROR', '✗ Fallo en Origen: ' + result.source_error);
                }

                const tgtBadge = document.getElementById('tgtStatusBadge');
                if (result.target_ok) {
                    tgtBadge.className = 'badge badge-synced';
                    tgtBadge.textContent = 'Conectado (UID: ' + result.target_uid + ')';
                    logToConsole('INFO', '✓ Destino conectado con éxito: ' + creds.target.url);
                } else {
                    tgtBadge.className = 'badge badge-error';
                    tgtBadge.textContent = 'Error';
                    logToConsole('ERROR', '✗ Fallo en Destino: ' + result.target_error);
                }

                updateStats();
            } catch (err) {
                logToConsole('ERROR', 'Error de red al probar conexiones: ' + err.message);
            }
        }

        async function runDryRun() {
            logToConsole('INFO', 'Iniciando Simulación (Dry-Run) desde ' + document.getElementById('filterStartDate').value + '...');
            const creds = getCredentials();
            try {
                const res = await fetch('/api/dry-run', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(creds)
                });
                const report = await res.json();
                if (report.error) {
                    logToConsole('ERROR', 'Error en Dry-Run: ' + report.error);
                    return;
                }

                logToConsole('INFO', '=== REPORTE DE SIMULACIÓN (DRY-RUN) ===');
                logToConsole('INFO', 'Tickets de Origen (>= ' + creds.startDate + '): ' + report.total_source_tickets);
                logToConsole('INFO', 'Partes de Horas de Origen: ' + report.total_source_timesheets);
                logToConsole('INFO', 'Tickets ya sincronizados: ' + report.already_synced_tickets);
                logToConsole('INFO', 'Tickets listos para migrar de inmediato: ' + report.ready_to_migrate_tickets);

                if (report.unresolved_partners && report.unresolved_partners.length > 0) {
                    logToConsole('WARN', '⚠️ Partners sin coincidencia en destino (' + report.unresolved_partners.length + '): Revisa pestaña Mapeos');
                }
                if (report.unresolved_users && report.unresolved_users.length > 0) {
                    logToConsole('WARN', '⚠️ Usuarios sin coincidencia (' + report.unresolved_users.length + ')');
                }
                if (report.unresolved_projects && report.unresolved_projects.length > 0) {
                    logToConsole('WARN', '⚠️ Proyectos sin coincidencia (' + report.unresolved_projects.length + ')');
                }

                updateStats();
            } catch (err) {
                logToConsole('ERROR', 'Error ejecutando Dry-Run: ' + err.message);
            }
        }

        function openMigrateOneModal() {
            document.getElementById('migrateOneModal').style.display = 'flex';
        }

        function closeMigrateOneModal() {
            document.getElementById('migrateOneModal').style.display = 'none';
        }

        async function executeMigrateOne() {
            const id = parseInt(document.getElementById('singleTicketID').value);
            if (!id || id <= 0) {
                alert('Introduce un ID de ticket válido');
                return;
            }
            closeMigrateOneModal();
            logToConsole('INFO', 'Migrando ticket ' + id + ' de forma unitaria...');
            const creds = getCredentials();
            try {
                const res = await fetch('/api/migrate-ticket', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({...creds, ticket_id: id})
                });
                const result = await res.json();
                if (result.error) {
                    logToConsole('ERROR', 'Fallo al migrar ticket ' + id + ': ' + result.error);
                } else {
                    logToConsole('INFO', '✓ Ticket ' + id + ' migrado correctamente -> Target ID: ' + result.target_id);
                }
                updateStats();
            } catch (err) {
                logToConsole('ERROR', 'Error: ' + err.message);
            }
        }

        async function runBatch(size) {
            logToConsole('INFO', 'Iniciando migración de lote de ' + size + ' tickets...');
            const creds = getCredentials();
            try {
                const res = await fetch('/api/migrate-batch', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({...creds, batch_size: size})
                });
                const result = await res.json();
                logToConsole('INFO', 'Lote completado: ' + (result.processed || 0) + ' procesados, ' + (result.errors || 0) + ' errores.');
                updateStats();
            } catch (err) {
                logToConsole('ERROR', 'Error en lote: ' + err.message);
            }
        }

        async function runFullSync() {
            if (!confirm('¿Deseas iniciar la sincronización total desde 2025? (El proceso es 100% idempotente y no duplicará datos)')) {
                return;
            }
            logToConsole('INFO', 'Iniciando Sincronización Total Idempotente...');
            const creds = getCredentials();
            try {
                const res = await fetch('/api/migrate-all', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(creds)
                });
                const result = await res.json();
                logToConsole('INFO', 'Sincronización finalizada: ' + (result.synced || 0) + ' registros actualizados.');
                updateStats();
            } catch (err) {
                logToConsole('ERROR', 'Error en sincronización total: ' + err.message);
            }
        }

        async function loadMappings() {
            try {
                const res = await fetch('/api/mappings');
                const mappings = await res.json();
                const tbody = document.getElementById('mappingsTableBody');
                tbody.innerHTML = '';
                if (!mappings || mappings.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="9" style="text-align:center; color:var(--text-muted);">No hay mapeos registrados. Ejecuta primero un "Dry-Run".</td></tr>';
                    return;
                }

                mappings.forEach(m => {
                    const tr = document.createElement('tr');
                    const badgeClass = m.status === 'resolved' ? 'badge-resolved' : 'badge-pending';
                    tr.innerHTML = 
                        '<td>' + m.entity_type + '</td>' +
                        '<td>' + m.source_id + '</td>' +
                        '<td><b>' + (m.source_name || '') + '</b></td>' +
                        '<td>' + (m.source_key || '-') + '</td>' +
                        '<td>' + (m.target_id > 0 ? m.target_id : '<span style="color:var(--danger)">No asignado</span>') + '</td>' +
                        '<td>' + (m.target_name || '-') + '</td>' +
                        '<td>' + (m.match_criterion || '-') + '</td>' +
                        '<td><span class="badge ' + badgeClass + '">' + m.status + '</span></td>' +
                        '<td><button class="btn btn-secondary" style="padding:0.2rem 0.5rem; font-size:0.75rem;" onclick="openEditMappingModal(\'' + m.entity_type + '\', ' + m.source_id + ', \'' + (m.source_name || '') + '\', ' + m.target_id + ')">Asignar</button></td>';
                    tbody.appendChild(tr);
                });
            } catch (err) {
                console.error(err);
            }
        }

        function openEditMappingModal(entity, srcID, srcName, targetID) {
            document.getElementById('editMappingEntityType').value = entity;
            document.getElementById('editMappingSourceID').value = srcID;
            document.getElementById('editMappingSourceName').value = srcName;
            document.getElementById('editMappingTargetID').value = targetID > 0 ? targetID : '';
            document.getElementById('editMappingTargetName').value = '';
            document.getElementById('editMappingModal').style.display = 'flex';
        }

        function closeEditMappingModal() {
            document.getElementById('editMappingModal').style.display = 'none';
        }

        async function saveManualMapping() {
            const entity = document.getElementById('editMappingEntityType').value;
            const srcID = parseInt(document.getElementById('editMappingSourceID').value);
            const srcName = document.getElementById('editMappingSourceName').value;
            const targetID = parseInt(document.getElementById('editMappingTargetID').value);
            const targetName = document.getElementById('editMappingTargetName').value;

            if (!targetID || targetID <= 0) {
                alert('Introduce un ID de destino válido');
                return;
            }

            try {
                await fetch('/api/mappings/save', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({
                        entity_type: entity,
                        source_id: srcID,
                        source_name: srcName,
                        target_id: targetID,
                        target_name: targetName,
                        match_criterion: 'manual',
                        status: 'resolved'
                    })
                });
                closeEditMappingModal();
                loadMappings();
                logToConsole('INFO', 'Mapeo guardado: ' + entity + ' ' + srcID + ' -> ' + targetID);
            } catch (err) {
                alert('Error al guardar mapeo: ' + err.message);
            }
        }

        async function loadSyncRecords() {
            try {
                const res = await fetch('/api/sync-records');
                const records = await res.json();
                const tbody = document.getElementById('syncRecordsTableBody');
                tbody.innerHTML = '';
                if (!records || records.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center; color:var(--text-muted);">Aún no hay registros sincronizados.</td></tr>';
                    return;
                }

                records.forEach(r => {
                    const tr = document.createElement('tr');
                    const badgeClass = r.status === 'synced' ? 'badge-synced' : (r.status === 'error' ? 'badge-error' : 'badge-pending');
                    tr.innerHTML = 
                        '<td>' + r.entity_type + '</td>' +
                        '<td>' + r.source_id + '</td>' +
                        '<td>' + (r.target_id > 0 ? r.target_id : '-') + '</td>' +
                        '<td style="font-family:monospace; font-size:0.75rem;">' + (r.content_hash ? r.content_hash.substring(0, 12) + '...' : '-') + '</td>' +
                        '<td><span class="badge ' + badgeClass + '">' + r.status + '</span></td>' +
                        '<td style="color:' + (r.status === 'error' ? 'var(--danger)' : 'var(--text-muted)') + '">' + (r.error_message || '-') + '</td>' +
                        '<td>' + (r.updated_at ? new Date(r.updated_at).toLocaleString() : '-') + '</td>';
                    tbody.appendChild(tr);
                });
            } catch (err) {
                console.error(err);
            }
        }

        async function loadFullLogs() {
            try {
                const res = await fetch('/api/logs');
                const logs = await res.json();
                const tbody = document.getElementById('logsTableBody');
                tbody.innerHTML = '';
                if (!logs || logs.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center; color:var(--text-muted);">Sin logs de auditoría.</td></tr>';
                    return;
                }

                logs.forEach(l => {
                    const tr = document.createElement('tr');
                    tr.innerHTML = 
                        '<td>' + new Date(l.timestamp).toLocaleTimeString() + '</td>' +
                        '<td class="log-' + l.level + '">' + l.level + '</td>' +
                        '<td>' + (l.entity || '-') + '</td>' +
                        '<td>' + (l.source_id > 0 ? l.source_id : '-') + '</td>' +
                        '<td>' + (l.target_id > 0 ? l.target_id : '-') + '</td>' +
                        '<td>' + l.message + '</td>' +
                        '<td style="font-family:monospace; font-size:0.75rem;">' + (l.payload || '-') + '</td>';
                    tbody.appendChild(tr);
                });
            } catch (err) {
                console.error(err);
            }
        }

        function exportMappingsJSON() {
            window.open('/api/mappings/export', '_blank');
        }

        // Refresco de estadísticas periódico
        updateStats();
        setInterval(updateStats, 5000);
    </script>
</body>
</html>
`
