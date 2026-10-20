#!/usr/bin/env bash
# Arranque de PSF (Planes Software Factory) al empezar cada sesión de Claude Code (hook SessionStart).
# Lo instala scripts/install-claude-hook.sh. Hace falta sobre todo en la nube: el «Setup script» del entorno no
# tiene variables de entorno, así que PSF (que solo se descarga con token de empleado) se descarga aquí.
#   - Si la máquina ya tiene el plugin psf@psf (instalado con PSF/install.sh), no hace nada: lo carga el plugin.
#   - Descarga PSF de PlanesGo (/install/psf.tar.gz) con $PLANESGO_TOKEN en ~/.planesgo/PSF.
#   - Instala las skills psf-*, el comando /psf y las herramientas psf-*, y devuelve el contexto de PSF del proyecto.
# Nunca impide que arranque la sesión. Registro: /tmp/psf-setup.log
log=/tmp/psf-setup.log
grep -qs '"psf@psf"' "${HOME}/.claude/plugins/installed_plugins.json" && exit 0

proj="$(git -C "${CLAUDE_PROJECT_DIR:-$PWD}" rev-parse --show-toplevel 2>/dev/null || echo "${CLAUDE_PROJECT_DIR:-$PWD}")"
# Tras reiniciarse el worker de la nube, el directorio de trabajo puede quedar en /home/user: se toma el único
# repositorio de la sesión que tenga perfil PSF
if [ ! -f "${proj}/.psf/proyecto.yml" ]; then
    perfiles=( "${proj}"/*/.psf/proyecto.yml )
    [ "${#perfiles[@]}" -eq 1 ] && [ -f "${perfiles[0]}" ] && proj="$(dirname "$(dirname "${perfiles[0]}")")"
fi
perfil="${proj}/.psf/proyecto.yml"
[ -f "${perfil}" ] || exit 0

url="${PLANESGO_URL:-https://planesgo.autopyme.com}"
token="${PLANESGO_TOKEN:-${ANTIGRAVITY_TOKEN:-}}"
psf_dir="${PSF_DIR:-${HOME}/.planesgo/PSF}"
if [ -n "${token}" ]; then
    tmp="$(mktemp -d)"
    if curl -fsS --max-time 20 -H "Authorization: Bearer ${token}" "${url%/}/install/psf.tar.gz" | tar -xz -C "${tmp}" 2>/dev/null \
        && [ -f "${tmp}/PSF/hooks/psf_hook.py" ]; then
        rm -rf "${psf_dir}" && mkdir -p "$(dirname "${psf_dir}")" && mv "${tmp}/PSF" "${psf_dir}"
    fi
    rm -rf "${tmp}"
fi
[ -f "${psf_dir}/hooks/psf_hook.py" ] || psf_dir=""
echo "== PSF arranque $(date -u +%FT%TZ) proyecto=${proj} psf=${psf_dir:-no}" >> "${log}"

if [ -n "${psf_dir}" ]; then
    mkdir -p "${HOME}/.claude/skills" "${HOME}/.claude/commands" "${HOME}/.local/bin"
    for s in "${psf_dir}"/skills/psf-*; do ln -sfn "${s}" "${HOME}/.claude/skills/$(basename "${s}")"; done
    sed "s#\${CLAUDE_PLUGIN_ROOT}#${psf_dir}#g" "${psf_dir}/commands/psf.md" > "${HOME}/.claude/commands/psf.md"
    for t in "${psf_dir}"/bin/psf-*; do
        chmod +x "${t}"
        ln -sf "${t}" "/usr/local/bin/$(basename "${t}")" 2>/dev/null || ln -sf "${t}" "${HOME}/.local/bin/$(basename "${t}")"
    done
    CLAUDE_PLUGIN_ROOT="${psf_dir}" CLAUDE_PROJECT_DIR="${proj}" python3 "${psf_dir}/hooks/psf_hook.py" session-start
    exit 0
fi

if ! grep -qiE '^activo:[[:space:]]*(false|no|off)' "${perfil}"; then
    echo "PSF pendiente: no se pudo descargar de ${url} (¿falta PLANESGO_TOKEN?)" >> "${log}"
    python3 - <<'PY'
import json
msg = """# PSF activo en este proyecto (no se ha podido cargar)

Este proyecto sigue Planes Software Factory (perfil en `.psf/proyecto.yml`), pero PSF no se ha podido descargar de
PlanesGo al empezar la sesión (falta el token de empleado `PLANESGO_TOKEN` o el servidor no responde; detalle en
/tmp/psf-setup.log). Díselo a la persona responsable y sigue con el AGENTS.md del proyecto y el perfil: la rama de
producción nunca la toca un agente y lo que no esté indicado se pregunta."""
print(json.dumps({'hookSpecificOutput': {'hookEventName': 'SessionStart', 'additionalContext': msg}}, ensure_ascii=False))
PY
fi
exit 0
