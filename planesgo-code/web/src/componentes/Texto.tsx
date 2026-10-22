import { memo } from 'react';
import Markdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { FotoRespuesta } from './Capturas';

// Texto enriquecido de las respuestas del agente (Markdown con tablas, listas y código), como en Claude Code.
// No se interpreta el HTML que venga en el texto; los enlaces se abren en el navegador del sistema.
const abre = (e: React.MouseEvent<HTMLAnchorElement>) => {
  const url = e.currentTarget.href;
  e.preventDefault();
  if (/^https?:\/\//i.test(url)) window.psf?.abrirFuera(url).catch(() => {});
};

const PLUGINS = [remarkGfm];
const COMPONENTES: Components = {
  a: ({ href, children }) => <a href={href} onClick={abre} title={href}>{children}</a>,
  table: ({ children }) => <div className="md-tabla"><table>{children}</table></div>,
  // Las imágenes, en miniatura (al pulsar, el visor): a tamaño real ocupaban toda la conversación.
  img: ({ src, alt }) => <FotoRespuesta src={typeof src === 'string' ? src : undefined} alt={alt} />,
};

// memo: el Markdown de los mensajes anteriores no se vuelve a interpretar con cada trozo de la respuesta en curso.
export default memo(function Texto({ md }: { md: string }) {
  return (
    <div className="md">
      <Markdown remarkPlugins={PLUGINS} skipHtml components={COMPONENTES}>{md}</Markdown>
    </div>
  );
});
