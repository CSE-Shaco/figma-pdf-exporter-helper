#!/usr/bin/env python3
"""Exercise a frozen helper against actual OS fonts without logging tokens or fonts."""
import hashlib
import http.client
import json
import queue
import re
import subprocess
import sys
import threading
import time
from pathlib import Path
from helper import FontIndex


def main():
    binary = str(Path(sys.argv[1]).resolve())
    index = FontIndex()
    assert index.fonts, 'No OS fonts discovered on the test machine'
    process = subprocess.Popen([binary, '--port', '0'], stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True)
    lines = queue.Queue()
    def read_output():
        for line in process.stdout:
            lines.put(line)
    threading.Thread(target=read_output, daemon=True).start()
    try:
        deadline = time.monotonic() + 90
        port = token = None
        while time.monotonic() < deadline and not (port and token):
            try:
                line = lines.get(timeout=0.2)
            except queue.Empty:
                assert process.poll() is None, 'Helper exited before startup'
                continue
            address = re.search(r'http://localhost:(\d+)', line)
            pairing = re.search(r'플러그인 연결 코드: ([0-9a-f]{64})', line)
            if address:
                port = int(address.group(1))
            if pairing:
                token = pairing.group(1)
        assert port and token, 'Startup timeout'
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=10)
        client.request('POST', '/v1/match', json.dumps({'families': []}))
        response = client.getresponse()
        assert response.status == 401, 'Unauthenticated request accepted'
        response.read()
        candidate = next(iter(index.fonts.values()))
        headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'}
        client.request('POST', '/v1/match', json.dumps({'families': [candidate['family']]}), headers)
        response = client.getresponse()
        assert response.status == 200
        matches = json.loads(response.read())['fonts']
        assert matches, 'Native OS font match failed'
        found = matches[0]
        client.request('GET', '/v1/fonts/' + found['id'], headers=headers)
        response = client.getresponse()
        assert response.status == 200
        actual = response.read()
        expected = index.read(found['id'])
        assert expected and hashlib.sha256(actual).digest() == hashlib.sha256(expected).digest()
        client.close()
        print('PASS: standalone startup, unauthorized rejection, OS font match and SHA-256 download')
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)
        process.stdout.close()

if __name__ == '__main__':
    main()
