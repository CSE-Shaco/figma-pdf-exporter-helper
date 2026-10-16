#!/usr/bin/env python3
"""Authenticated, loopback-only installed-font bridge. Python 3.9+, no packages."""
import argparse
import hashlib
import hmac
import json
import os
from pathlib import Path
import secrets
import struct
import sys
import unicodedata
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MAX_FONT = 20_000_000
MAX_COLLECTION = 64_000_000
MAX_BODY = 32_768
SFNT_SIGNATURES = (b'\x00\x01\x00\x00', b'OTTO', b'true', b'ttcf')

def normalize(value):
    return ''.join(c for c in unicodedata.normalize('NFKC', value).casefold() if c.isalnum())

def font_offsets(data):
    if len(data) >= 12 and data[:4] == b'ttcf':
        count = struct.unpack_from('>I', data, 8)[0]
        if not 0 < count <= 256 or 12 + count * 4 > len(data):
            return []
        return [struct.unpack_from('>I', data, 12 + i * 4)[0] for i in range(count)]
    return [0]

def font_names(data, base=0):
    """Read one SFNT face's name records; collection table offsets are absolute."""
    if base + 12 > len(data) or data[base:base + 4] not in (b'\x00\x01\x00\x00', b'OTTO', b'true'):
        return None
    count = struct.unpack_from('>H', data, base + 4)[0]
    if count > 512 or base + 12 + count * 16 > len(data):
        return None
    for pos in range(base + 12, base + 12 + count * 16, 16):
        tag, _, offset, length = struct.unpack_from('>4sIII', data, pos)
        if tag != b'name':
            continue
        if length < 6 or offset + length > len(data):
            return None
        table = data[offset:offset + length]
        _, records, strings = struct.unpack_from('>HHH', table)
        if records > 4096 or 6 + records * 12 > len(table):
            return None
        values = {}
        for p in range(6, 6 + records * 12, 12):
            platform, encoding, language, name_id, size, start = struct.unpack_from('>HHHHHH', table, p)
            if name_id not in (1, 2, 4, 6, 16, 17, 21) or strings + start + size > len(table):
                continue
            try:
                value = table[strings + start:strings + start + size].decode('utf-16-be' if platform in (0, 3) else 'mac_roman').strip('\x00 ')
            except UnicodeError:
                continue
            if value and len(value) < 512:
                values.setdefault(name_id, []).append(value)
        families = list(dict.fromkeys(values.get(16, []) + values.get(1, []) + values.get(21, [])))
        if not families:
            return None
        aliases = list(dict.fromkeys(families + values.get(4, []) + values.get(6, [])))
        return {'family': families[0], 'families': aliases, 'style': (values.get(17) or values.get(2) or ['Regular'])[0]}
    return None

def coretext_paths():
    """Ask macOS for installed font URLs; fall back to standard folders."""
    try:
        import ctypes
        cf = ctypes.CDLL('/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation')
        ct = ctypes.CDLL('/System/Library/Frameworks/CoreText.framework/CoreText')
        ct.CTFontManagerCopyAvailableFontURLs.restype = ctypes.c_void_p
        cf.CFArrayGetCount.argtypes = [ctypes.c_void_p]
        cf.CFArrayGetCount.restype = ctypes.c_long
        cf.CFArrayGetValueAtIndex.argtypes = [ctypes.c_void_p, ctypes.c_long]
        cf.CFArrayGetValueAtIndex.restype = ctypes.c_void_p
        cf.CFURLCopyFileSystemPath.argtypes = [ctypes.c_void_p, ctypes.c_int]
        cf.CFURLCopyFileSystemPath.restype = ctypes.c_void_p
        cf.CFStringGetCString.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_long, ctypes.c_uint]
        cf.CFStringGetCString.restype = ctypes.c_bool
        cf.CFRelease.argtypes = [ctypes.c_void_p]
        urls = ct.CTFontManagerCopyAvailableFontURLs()
        if not urls:
            return []
        paths = []
        try:
            for i in range(cf.CFArrayGetCount(urls)):
                url = cf.CFArrayGetValueAtIndex(urls, i)
                string = cf.CFURLCopyFileSystemPath(url, 0)
                if not string:
                    continue
                try:
                    buffer = ctypes.create_string_buffer(4096)
                    if cf.CFStringGetCString(string, buffer, len(buffer), 0x08000100):
                        paths.append(Path(os.fsdecode(buffer.value)))
                finally:
                    cf.CFRelease(string)
        finally:
            cf.CFRelease(urls)
        return paths
    except (OSError, AttributeError, ValueError):
        return []

def installed_paths():
    """OS installation locations plus Windows registered font paths."""
    roots = []
    extra = []
    if sys.platform == 'darwin':
        roots = [Path.home() / 'Library/Fonts', Path('/Library/Fonts'), Path('/System/Library/Fonts'), Path('/Network/Library/Fonts')]
        extra = coretext_paths()
    elif sys.platform == 'win32':
        import winreg
        system = Path(os.environ.get('WINDIR', r'C:\Windows')) / 'Fonts'
        user = Path(os.environ.get('LOCALAPPDATA', str(Path.home() / 'AppData/Local'))) / 'Microsoft/Windows/Fonts'
        roots = [system, user]
        for hive, base in ((winreg.HKEY_LOCAL_MACHINE, system), (winreg.HKEY_CURRENT_USER, user)):
            try:
                with winreg.OpenKey(hive, r'SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts') as key:
                    for i in range(winreg.QueryInfoKey(key)[1]):
                        _, value, kind = winreg.EnumValue(key, i)
                        if kind in (winreg.REG_SZ, winreg.REG_EXPAND_SZ) and isinstance(value, str):
                            path = Path(os.path.expandvars(value))
                            extra.append(path if path.is_absolute() else base / path)
            except OSError:
                pass
    else:
        roots = [Path.home() / '.local/share/fonts', Path.home() / '.fonts', Path('/usr/share/fonts')]
    yield from discover_paths(roots, extra)

def discover_paths(roots, registered=()):
    """Inspect only OS font locations; activated fonts need not have extensions."""
    seen = set()
    def candidates():
        yield from registered
        for root in roots:
            if root.is_dir():
                yield from root.rglob('*')
    for path in candidates():
        try:
            path = Path(path).resolve(strict=True)
            if path in seen or not path.is_file():
                continue
            seen.add(path)
            # Validate bytes, not names: CoreText can expose extensionless URLs.
            with path.open('rb') as source:
                signature = source.read(4)
            limit = MAX_COLLECTION if signature == b'ttcf' else MAX_FONT
            if signature in SFNT_SIGNATURES and 0 < path.stat().st_size <= limit:
                yield path
        except (OSError, ValueError):
            continue

class FontIndex:
    def __init__(self, paths=None):
        self.fonts = {}
        for path in installed_paths() if paths is None else paths:
            try:
                path = Path(path).resolve(strict=True)
                stat = path.stat()
                if not 0 < stat.st_size <= MAX_COLLECTION:
                    continue
                with path.open('rb') as source:
                    data = source.read(MAX_COLLECTION + 1)
                if len(data) > MAX_COLLECTION:
                    continue
                if len(data) > MAX_FONT and data[:4] != b'ttcf':
                    continue
                digest = hashlib.sha256(data).digest()
                for face_index, offset in enumerate(font_offsets(data)):
                    names = font_names(data, offset)
                    if names:
                        identifier = hashlib.sha256(digest + struct.pack('>I', face_index)).hexdigest()
                        self.fonts[identifier] = dict(names, path=path, name=path.name, id=identifier,
                                                      digest=digest, collectionIndex=face_index if data[:4] == b'ttcf' else None)
            except (OSError, ValueError, struct.error):
                continue

    def match(self, families):
        requested = {normalize(f) for f in families}
        return [{k: font[k] for k in ('id', 'name', 'family', 'families', 'style', 'collectionIndex')} for font in self.fonts.values() if requested.intersection(normalize(f) for f in font['families'])]

    def read(self, identifier):
        font = self.fonts.get(identifier)
        if not font:
            return None
        # Reject replaced/modified files and bound memory even after startup.
        with font['path'].open('rb') as source:
            data = source.read(MAX_COLLECTION + 1)
        return data if len(data) <= MAX_COLLECTION and hmac.compare_digest(hashlib.sha256(data).digest(), font['digest']) else None

def make_server(index, token, port=38473):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass  # Never record pairing tokens or requested font names.

        def respond(self, status, body=b'', mime='application/json'):
            self.send_response(status)
            self.send_header('Access-Control-Allow-Origin', '*')
            self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
            self.send_header('Access-Control-Allow-Headers', 'Authorization, Content-Type')
            self.send_header('Access-Control-Allow-Private-Network', 'true')
            self.send_header('Cache-Control', 'no-store')
            self.send_header('X-Content-Type-Options', 'nosniff')
            self.send_header('Content-Type', mime)
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def authorized(self):
            if self.headers.get('Host') not in ('localhost:' + str(self.server.server_port), '127.0.0.1:' + str(self.server.server_port)):
                self.respond(403, b'{"error":"host"}')
                return False
            supplied = self.headers.get('Authorization', '')
            if not hmac.compare_digest(supplied, 'Bearer ' + token):
                self.respond(401, b'{"error":"pairing"}')
                return False
            return True

        def do_OPTIONS(self):
            self.respond(204)

        def do_POST(self):
            if not self.authorized():
                return
            if self.path != '/v1/match':
                return self.respond(404)
            try:
                length = int(self.headers.get('Content-Length', '0'))
                if not 0 < length <= MAX_BODY:
                    return self.respond(413)
                payload = json.loads(self.rfile.read(length))
                families = payload['families']
                if not isinstance(families, list) or len(families) > 200 or any(not isinstance(x, str) or len(x) > 256 for x in families):
                    raise ValueError()
                self.respond(200, json.dumps({'fonts': index.match(families)}).encode())
            except (ValueError, KeyError, TypeError):
                self.respond(400, b'{"error":"request"}')

        def do_GET(self):
            if not self.authorized():
                return
            prefix = '/v1/fonts/'
            if not self.path.startswith(prefix):
                return self.respond(404)
            try:
                data = index.read(self.path[len(prefix):])
            except OSError:
                data = None
            self.respond(200, data, 'application/octet-stream') if data else self.respond(404)

    class Server(ThreadingHTTPServer):
        daemon_threads = True
        def get_request(self):
            sock, address = super().get_request()
            sock.settimeout(15)
            return sock, address
    return Server(('127.0.0.1', port), Handler)

if __name__ == '__main__':
    # Redirected Windows output must retain Korean startup text and pairing codes.
    if hasattr(sys.stdout, 'reconfigure'):
        sys.stdout.reconfigure(encoding='utf-8')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, default=38473, help='loopback port (0 chooses a free port)')
    args = parser.parse_args()
    if not 0 <= args.port <= 65535:
        parser.error('port must be between 0 and 65535')
    print('설치된 TTF/OTF 글꼴을 검색합니다…', flush=True)
    index = FontIndex()
    token = secrets.token_hex(32)
    try:
        server = make_server(index, token, args.port)
    except OSError as error:
        raise SystemExit('도우미를 시작할 수 없습니다. 이미 실행 중인지 확인하세요: ' + str(error))
    print(f'주소: http://localhost:{server.server_port}', flush=True)
    print(f'글꼴 {len(index.fonts)}개 준비됨. 이 창을 열어 두세요. 종료: Ctrl+C\n플러그인 연결 코드: {token}', flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
