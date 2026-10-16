import http.client
import json
from pathlib import Path
import struct
import tempfile
import threading
import unittest
from unittest.mock import patch

from helper import FontIndex, make_server, discover_paths


def fixture_font(legacy_name='Fixture Sans', preferred_name='Fixture Sans Pro', base=0, full_name=None, postscript_name=None):
    names = [(1, legacy_name), (16, preferred_name)]
    if full_name:
        names.append((4, full_name))
    if postscript_name:
        names.append((6, postscript_name))
    strings = b''
    records = b''
    for name_id, name in names:
        encoded = name.encode('utf-16-be')
        records += struct.pack('>HHHHHH', 3, 1, 0x409, name_id, len(encoded), len(strings))
        strings += encoded
    table = struct.pack('>HHH', 0, len(names), 6 + len(records)) + records + strings
    return b'\x00\x01\x00\x00' + struct.pack('>HHHH', 1, 0, 0, 0) + struct.pack('>4sIII', b'name', 0, base + 28, len(table)) + table


class HelperTest(unittest.TestCase):
    def test_extensionless_registered_fonts_and_invalid_files(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            activated = root / '.12345'
            activated.write_bytes(fixture_font(postscript_name='FixtureSansPro-Activated'))
            invalid = root / 'invalid.otf'
            invalid.write_bytes(b'not a font')
            unrelated = root / 'outside'
            unrelated.write_bytes(fixture_font())
            paths = list(discover_paths([], [activated, invalid, activated, root / 'missing']))
            self.assertEqual(paths, [activated.resolve()])
            index = FontIndex(paths)
            self.assertEqual(len(index.match(['FixtureSansPro-Activated'])), 1)
            self.assertNotIn(unrelated, paths)

    def test_discovery_skips_unreadable_and_oversized_files(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            large = root / 'large'
            large.write_bytes(fixture_font() + b'padding')
            with patch('helper.MAX_FONT', 4):
                self.assertEqual(list(discover_paths([root])), [])
            with patch.object(Path, 'open', side_effect=PermissionError):
                self.assertEqual(list(discover_paths([], [large])), [])

    def test_extensionless_collection_discovery(self):
        data = b'ttcf' + struct.pack('>II', 0x00010000, 1) + struct.pack('>I', 16) + fixture_font(base=16)
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'opaque-collection'
            path.write_bytes(data)
            index = FontIndex(discover_paths([Path(folder)]))
            font = index.match(['Fixture Sans'])[0]
            self.assertEqual(font['collectionIndex'], 0)
            self.assertEqual(index.read(font['id']), data)

    def test_full_and_postscript_names_are_searchable(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'Alias.ttf'
            path.write_bytes(fixture_font(full_name='Fixture Sans Pro Demi', postscript_name='FixtureSansPro-Demi'))
            index = FontIndex([path])
            for name in ('Fixture Sans Pro Demi', 'FixtureSansPro-Demi'):
                matches = index.match([name])
                self.assertEqual(len(matches), 1)
                self.assertIn(name, matches[0]['families'])

    def test_collection_indexes_individual_faces(self):
        first = fixture_font('First Family', 'First Family', 20)
        second = fixture_font('Second Family', 'Second Family', 20 + len(first))
        data = b'ttcf' + struct.pack('>II', 0x00010000, 2) + struct.pack('>II', 20, 20 + len(first)) + first + second
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'Families.ttc'
            path.write_bytes(data)
            index = FontIndex([path])
            self.assertEqual(len(index.fonts), 2)
            for family, member in [('First Family', 0), ('Second Family', 1)]:
                matches = index.match([family])
                self.assertEqual(len(matches), 1)
                self.assertEqual(matches[0]['collectionIndex'], member)
                self.assertEqual(index.read(matches[0]['id']), data)

    def test_authenticated_match_and_font_bytes(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / '.activated-fixture'
            data = fixture_font()
            path.write_bytes(data)
            index = FontIndex(discover_paths([], [path]))
            self.assertEqual(len(index.match(['Ｆｉｘｔｕｒｅ Ｓａｎｓ'])), 1)
            token = 'a' * 64
            server = make_server(index, token, 0)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                port = server.server_port
                headers = {'Host': f'localhost:{port}', 'Authorization': f'Bearer {token}'}
                conn = http.client.HTTPConnection('127.0.0.1', port)
                conn.request('POST', '/v1/match', json.dumps({'families': ['fixture sans']}).encode(), {**headers, 'Content-Type': 'application/json'})
                response = conn.getresponse()
                self.assertEqual(response.status, 200)
                matches = json.loads(response.read())['fonts']
                self.assertEqual(len(matches), 1)
                self.assertEqual(matches[0]['family'], 'Fixture Sans Pro')
                self.assertEqual(matches[0]['families'], ['Fixture Sans Pro', 'Fixture Sans'])
                self.assertNotIn('path', matches[0])
                conn.request('GET', '/v1/fonts/' + matches[0]['id'], headers=headers)
                response = conn.getresponse()
                self.assertEqual(response.status, 200)
                self.assertEqual(response.read(), data)
                conn.request('GET', '/v1/fonts/' + matches[0]['id'], headers={**headers, 'Host': f'127.0.0.1:{port}'})
                response = conn.getresponse()
                self.assertEqual(response.status, 200)
                self.assertEqual(response.read(), data)
                conn.request('GET', '/v1/fonts/' + matches[0]['id'], headers={**headers, 'Authorization': 'Bearer wrong'})
                self.assertEqual(conn.getresponse().status, 401)
                conn.request('GET', '/v1/fonts/' + matches[0]['id'], headers={**headers, 'Host': f'evil.example:{port}'})
                self.assertEqual(conn.getresponse().status, 403)
                path.write_bytes(data + b'changed')
                conn.request('GET', '/v1/fonts/' + matches[0]['id'], headers=headers)
                self.assertEqual(conn.getresponse().status, 404)
                conn.close()
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)


if __name__ == '__main__':
    unittest.main()
