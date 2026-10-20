// Iconos de trazo fino (24×24, estilo minimalista). Siempre acompañados de title/aria-label en el botón.
const trazos: Record<string, JSX.Element> = {
  enviar: <path d="M12 19V5M5 12l7-7 7 7" />,
  parar: <rect x="7" y="7" width="10" height="10" rx="1.5" />,
  nuevo: <path d="M12 5v14M5 12h14" />,
  panel: <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M15 4v16" /></>,
  recargar: <><path d="M20 11a8 8 0 1 0-2.3 5.7" /><path d="M20 4v7h-7" /></>,
  salir: <><path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3" /><path d="M10 16l-4-4 4-4M6 12h10" /></>,
  atras: <path d="M15 18l-6-6 6-6" />,
  ir: <path d="M5 12h14M13 6l6 6-6 6" />,
  externo: <><path d="M14 4h6v6" /><path d="M20 4l-9 9" /><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4" /></>,
  copiar: <><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" /></>,
  hecho: <path d="M5 12l5 5 9-10" />,
  grabar: <circle cx="12" cy="12" r="6" fill="currentColor" stroke="none" />,
  video: <><rect x="3" y="6" width="13" height="12" rx="2" /><path d="M16 10l5-3v10l-5-3" /></>,
  descargar: <><path d="M12 4v11M7 10l5 5 5-5" /><path d="M5 20h14" /></>,
  conectar: <><path d="M9 7V3M15 7V3" /><path d="M6 7h12v4a6 6 0 0 1-12 0V7z" /><path d="M12 17v4" /></>,
  desconectar: <><path d="M9 7V3M15 7V3" /><path d="M6 7h12v4a6 6 0 0 1-12 0V7z" /><path d="M12 17v4" /><path d="M3 3l18 18" /></>,
  tv: <><rect x="3" y="5" width="18" height="12" rx="2" /><path d="M8 21h8M12 17v4" /></>,
  web: <><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" /></>,
  cerrar: <path d="M6 6l12 12M18 6L6 18" />,
  info: <><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 8h.01" /></>,
};

export default function Icono({ n, t = 18 }: { n: keyof typeof trazos; t?: number }) {
  return (
    <svg viewBox="0 0 24 24" width={t} height={t} fill="none" stroke="currentColor" strokeWidth="1.8"
      strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      {trazos[n]}
    </svg>
  );
}
