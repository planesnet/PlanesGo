"""Emisor hacia Studio TV (opcional): si existen PCO_URL y PCO_TOKEN, envía los fotogramas JPEG y el estado del
caso al estudio. PCO_CANAL elige el canal (por defecto «principal»), para emitir varias pruebas a la vez.
Los datos salen del entorno o, si no están, de ~/.planesgo/studio_tv.env (lo crea «/planesgo stream <canal>»).
Va en un hilo propio: si el estudio va lento se descartan fotogramas, nunca se frena la prueba."""
import http.client, json, os, ssl, threading, time, urllib.parse


def _config():
    """PCO_URL, PCO_TOKEN y PCO_CANAL del entorno o, si faltan, de ~/.planesgo/studio_tv.env."""
    c = {k: os.environ.get(k) for k in ('PCO_URL', 'PCO_TOKEN', 'PCO_CANAL')}
    if not (c['PCO_URL'] and c['PCO_TOKEN']):
        try:
            with open(os.path.expanduser('~/.planesgo/studio_tv.env')) as fh:
                for linea in fh:
                    k, _, v = linea.strip().partition('=')
                    if k in c and v and not c[k]:
                        c[k] = v
        except OSError:
            pass
    return c


class Emisor:
    def __init__(self, fps=8, canal=None):
        cfg = _config()
        self.url, self.tok = cfg['PCO_URL'], cfg['PCO_TOKEN']
        self.activo = bool(self.url and self.tok)
        self.min_dt, self.ult_envio = 1.0 / fps, 0
        self._frame, self._estado, self._ev = None, None, threading.Event()
        canal = canal or cfg['PCO_CANAL']
        self._q = '?canal=' + urllib.parse.quote(canal, safe='') if canal else ''
        if self.activo:
            u = urllib.parse.urlparse(self.url)
            self.https, self.host = u.scheme == 'https', u.netloc
            # Certificado propio del estudio (-selfsigned): PCO_INSEGURO=1 acepta el certificado sin verificar.
            self.ctx = ssl._create_unverified_context() if os.environ.get('PCO_INSEGURO') else ssl.create_default_context()
            self.cx = None
            threading.Thread(target=self._bucle, daemon=True).start()

    def frame(self, jpeg):
        if self.activo and time.time() - self.ult_envio >= self.min_dt:
            self.ult_envio = time.time(); self._frame = jpeg; self._ev.set()

    def estado(self, d):
        if self.activo:
            self._estado = json.dumps(d).encode(); self._ev.set()

    def _post(self, ruta, cuerpo, tipo):
        for _ in range(2):
            try:
                if self.cx is None:
                    self.cx = (http.client.HTTPSConnection(self.host, context=self.ctx, timeout=10) if self.https
                               else http.client.HTTPConnection(self.host, timeout=10))
                self.cx.request('POST', ruta + self._q, body=cuerpo,
                                headers={'Authorization': 'Bearer ' + self.tok, 'Content-Type': tipo})
                self.cx.getresponse().read()
                return
            except Exception:
                try: self.cx.close()
                except Exception: pass
                self.cx = None

    def _bucle(self):
        while True:
            self._ev.wait(); self._ev.clear()
            e, self._estado = self._estado, None
            f, self._frame = self._frame, None
            if e: self._post('/api/estado', e, 'application/json')
            if f: self._post('/api/frame', f, 'image/jpeg')
