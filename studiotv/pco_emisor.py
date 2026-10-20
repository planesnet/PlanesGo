"""Emisor hacia Studio TV (opcional): si existen PCO_URL y PCO_TOKEN en el entorno, envía los fotogramas JPEG
y el estado del caso al estudio. PCO_CANAL elige el canal (por defecto «principal»), para emitir varias pruebas a
la vez. Va en un hilo propio: si el estudio va lento se descartan fotogramas, nunca se frena la prueba."""
import http.client, json, os, ssl, threading, time, urllib.parse


class Emisor:
    def __init__(self, fps=8, canal=None):
        self.url, self.tok = os.environ.get('PCO_URL'), os.environ.get('PCO_TOKEN')
        self.activo = bool(self.url and self.tok)
        self.min_dt, self.ult_envio = 1.0 / fps, 0
        self._frame, self._estado, self._ev = None, None, threading.Event()
        canal = canal or os.environ.get('PCO_CANAL')
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
