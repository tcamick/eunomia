"""Usage: scripts/checks/bundle.py <dist dir> <geo dir>   (make bundle-check)
Checks that each state's district shapes are their own chunk and that the
entry bundle carries none of them, but does carry the state map."""
import glob
import json
import os
import re
import sys

dist, geo = sys.argv[1], sys.argv[2]
index = open(os.path.join(dist, 'index.html')).read()
entry = re.search(r'src="/assets/([^"]+\.js)"', index).group(1)
entry_js = open(os.path.join(dist, 'assets', entry)).read()
manifest = json.load(open(os.path.join(geo, 'manifest.json')))

def first_path(code):
    s = open(os.path.join(geo, 'districts', f'{code}.ts')).read()
    return re.search(r'"d": "(M[^"]{24})', s).group(1)

def state_path(code):
    s = open(os.path.join(geo, 'states.ts')).read()
    return re.search(r'"code": "%s",\s*"name": "[^"]+",\s*"d": "(M[^"]{24})' % code, s).group(1)

chunks = glob.glob(os.path.join(dist, 'assets', '*.js'))
bad, found = [], 0
for code in manifest['districts']:
    p = first_path(code)
    if p in entry_js:
        bad.append(code)
    holders = [c for c in chunks if p in open(c).read()]
    if len(holders) == 1 and os.path.basename(holders[0]) != entry:
        found += 1
print(f'entry {entry}: {os.path.getsize(os.path.join(dist, "assets", entry))} bytes')
print(f'district modules in a chunk of their own: {found} of {len(manifest["districts"])}')
print(f'district shapes in the entry bundle: {bad or "none"}')
print(f'state map in the entry bundle: {state_path("CA") in entry_js}')
sizes = sorted(os.path.getsize(c) for c in chunks if os.path.basename(c) != entry)
print(f'other chunks: {len(sizes)}, smallest {sizes[0]} bytes, largest {sizes[-1]} bytes')
sys.exit(1 if bad or found != len(manifest['districts']) else 0)
