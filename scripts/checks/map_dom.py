"""Usage: scripts/checks/map_dom.py <dumped DOM file> <mode>   (make map-check)
Checks the rendered map page: every jurisdiction is one tab stop on the
map, in alphabetical order, and each map link's accessible name equals the
list's words for the same state in the mode shown."""
import html
import re
import sys

dom = open(sys.argv[1]).read()
mode = sys.argv[2]
svg = re.search(r'<svg[^>]*class="map[^"]*"[^>]*>(.*?)</svg>', dom, re.S).group(1)
stops = []
for m in re.finditer(r'<a ([^>]*)>', svg):
    attrs = m.group(1)
    if 'tabindex="-1"' in attrs:
        continue
    href = re.search(r'href="/states/([A-Z]{2})"', attrs).group(1)
    label = html.unescape(re.search(r'aria-label="([^"]*)"', attrs).group(1))
    stops.append((href, label))
names = [label.split(':')[0] for _, label in stops]
codes = [c for c, _ in stops]
print(f'tab stops on the map: {len(stops)}, distinct states: {len(set(codes))}')
print(f'alphabetical: {names == sorted(names)}')

table = re.search(r'<tbody[^>]*>(.*?)</tbody>', dom, re.S).group(1)
col = 0 if mode == 'senate' else 1
listed = {}
for row in re.finditer(r'<tr[^>]*>(.*?)</tr>', table, re.S):
    code = re.search(r'href="/states/([A-Z]{2})"', row.group(1)).group(1)
    cells = re.findall(r'<td[^>]*>(.*?)</td>', row.group(1), re.S)
    hidden = re.search(r'class="visually-hidden[^"]*">([^<]*)<', cells[col])
    listed[code] = html.unescape(hidden.group(1)) if hidden else None
mismatch = [(c, l, listed.get(c)) for c, l in stops if listed.get(c) not in (l, None) or (listed.get(c) is None and 'no senators' not in l)]
print(f'map names that differ from the list ({mode}): {mismatch or "none"}')
sys.exit(1 if mismatch or len(stops) != 56 or len(set(codes)) != 56 or names != sorted(names) else 0)
