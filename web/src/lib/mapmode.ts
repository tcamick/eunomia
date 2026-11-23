// What the map shows for a state in each colour mode (PRD §6.2): a fill, a
// short label in letters, and an accessible name in words. The map and the
// list beside it both read these, so they cannot disagree. Party is never
// colour alone: the label and the name say it too, and a tie is stripes of
// both parties, not a blend (docs/web-conventions.md, party colour).

import type { Delegation, StateSummary } from './api.ts';
import { members, partyStyle } from './party.ts';

/** The party codes a fill can hold, in the order patterns are made. */
export const fillCodes = ['D', 'I', 'O', 'R', ''];

/** patternId names the stripes of a tie between two parties (MapDefs). */
export function patternId(codes: [string, string]): string {
  return `even-${codes[0] || 'U'}-${codes[1] || 'U'}`;
}

/** paint is the SVG fill for a Fill: a party token, or a pattern in MapDefs. */
export function paint(f: Fill): string {
  switch (f.kind) {
    case 'party':
      return `var(${f.code ? partyStyle(f.code).token : '--muted'})`;
    case 'even':
      return `url(#${patternId(f.codes)})`;
    case 'vacant':
      return 'url(#vacant)';
    default:
      return 'var(--sunk)';
  }
}

export type Mode = 'senate' | 'house';

export const modes: { id: Mode; label: string }[] = [
  { id: 'senate', label: 'Senate delegation' },
  { id: 'house', label: 'House delegation' },
];

export type Fill =
  | { kind: 'party'; code: string }
  | { kind: 'even'; codes: [string, string] }
  | { kind: 'vacant' }
  | { kind: 'none' };

/** delegationOf is a state's delegation in the chamber a mode colours by. */
export function delegationOf(s: StateSummary, mode: Mode): Delegation {
  return mode === 'senate' ? s.senate : s.house;
}

/** fillFor is how the map paints a delegation. */
export function fillFor(d: Delegation): Fill {
  if (d.lead === 'none') return { kind: 'none' };
  if (d.lead === 'vacant') return { kind: 'vacant' };
  // The server decides a tie's two codes on seats per party (congress.States),
  // the same way for every party; the page only draws them.
  if (d.lead === 'even') return { kind: 'even', codes: [d.stripes?.[0] ?? '', d.stripes?.[1] ?? ''] };
  return { kind: 'party', code: d.lead };
}

const letter = (code: string): string => code || '?';

/** shortLabel is the seats in letters: "1 D · 1 R", "1 R · 1 vacant". */
export function shortLabel(d: Delegation): string {
  const parts = d.parties.map((p) => `${p.seats} ${letter(p.code)}`);
  if (d.vacant > 0) parts.push(`${d.vacant} vacant`);
  return parts.join(' · ');
}

function partyWords(p: { party: string; caucus?: string }): string {
  if (!p.party) return 'party unknown';
  if (p.caucus && p.caucus !== p.party) return `${p.party} (caucuses with ${members(p.caucus)})`;
  return p.party;
}

/** spoken is the accessible name: "Ohio: senators 1 Democratic, 1 Republican". */
export function spoken(s: StateSummary, mode: Mode): string {
  const d = delegationOf(s, mode);
  if (mode === 'senate' && d.seats === 0) return `${s.name}: no senators`;
  const who = mode === 'senate' ? 'senators' : s.voting ? 'House' : 'non-voting delegate';
  const parts = d.parties.map((p) => `${p.seats} ${partyWords(p)}`);
  if (d.vacant > 0) parts.push(`${d.vacant} vacant`);
  return `${s.name}: ${who} ${parts.join(', ')}`;
}

export interface LegendEntry {
  key: string;
  fill: Fill;
  label: string;
}

const codeOrder = ['D', 'I', 'O', 'R', ''];

/** legend lists what the map shows in a mode: parties, then ties, then vacant. */
export function legend(states: StateSummary[], mode: Mode): LegendEntry[] {
  const names = new Map<string, Set<string>>();
  const ties = new Map<string, [string, string]>();
  let vacant = false;
  for (const s of states) {
    const d = delegationOf(s, mode);
    const f = fillFor(d);
    // A party is in the legend only where it is a fill of its own; inside
    // a tie it is named by the tie's entry.
    if (f.kind === 'party') {
      if (!names.has(f.code)) names.set(f.code, new Set());
      for (const p of d.parties) if (p.code === f.code) names.get(f.code)!.add(partyWords(p));
    }
    if (f.kind === 'even') ties.set(f.codes.join('-'), f.codes);
    if (f.kind === 'vacant') vacant = true;
  }
  const out: LegendEntry[] = [];
  for (const code of codeOrder) {
    const n = names.get(code);
    if (n) out.push({ key: `party:${code}`, fill: { kind: 'party', code }, label: `${letter(code)}: ${[...n].sort().join('; ')}` });
  }
  for (const [key, codes] of [...ties].sort(([a], [b]) => a.localeCompare(b))) {
    out.push({ key: `even:${key}`, fill: { kind: 'even', codes }, label: `Even: ${codes.map(letter).join(' and ')}` });
  }
  if (vacant) out.push({ key: 'vacant', fill: { kind: 'vacant' }, label: 'Every seat vacant' });
  return out;
}
