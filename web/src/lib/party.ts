// Parties as the pages show them: a colour token and a shape, always
// together, and the party's own name beside them (docs/web-conventions.md,
// party colour). The shapes make the seat chart readable without colour.

export type PartyCode = 'D' | 'R' | 'I' | 'O' | '';
export type Shape = 'circle' | 'square' | 'diamond' | 'triangle' | 'ring';

export interface PartyStyle {
  token: string;
  shape: Shape;
}

const styles: Record<string, PartyStyle> = {
  D: { token: '--party-dem', shape: 'circle' },
  R: { token: '--party-rep', shape: 'square' },
  I: { token: '--party-ind', shape: 'diamond' },
  O: { token: '--party-other', shape: 'triangle' },
  '': { token: '--vacant', shape: 'ring' },
};

/** partyStyle returns the colour token and shape for a party code. */
export function partyStyle(code: string): PartyStyle {
  return styles[code] ?? styles['O']!;
}

/** vacantStyle is how an empty seat is drawn. */
export const vacantStyle: PartyStyle = { token: '--vacant', shape: 'ring' };

export interface SeatGroup {
  party: string;
  caucus?: string;
  code: string;
  seats: number;
}

/**
 * seatingOrder sorts groups left to right for the seat chart, as charts of
 * Congress conventionally seat it: Democrats, then those who caucus with
 * them, then others, then those who caucus with Republicans, then
 * Republicans. It is a drawing convention, not a ranking; the legend lists
 * groups by size.
 */
export function seatingOrder<T extends SeatGroup>(groups: T[]): T[] {
  const rank = (g: SeatGroup): number => {
    if (g.code === 'D') return 0;
    if (g.caucus === 'Democratic') return 1;
    if (g.caucus === 'Republican') return 3;
    if (g.code === 'R') return 4;
    return 2;
  };
  return [...groups].sort((a, b) => rank(a) - rank(b) || a.party.localeCompare(b.party));
}

/** partyLabel is the party's name, with the caucus when it differs. */
export function partyLabel(party: string, caucus?: string): string {
  if (!party) return 'Party unknown';
  if (caucus && caucus !== party) return `${party}, caucuses with ${members(caucus)}`;
  return party;
}

/** members names a party's members: Democrats, Republicans. */
export function members(party: string): string {
  if (party === 'Democratic') return 'Democrats';
  if (party === 'Republican') return 'Republicans';
  return `${party} members`;
}

/** partyCode maps a party's name to its code, as the server's congress.PartyCode does. */
export function partyCode(party: string | undefined): PartyCode {
  if (!party) return '';
  return ({ Democratic: 'D', Republican: 'R', Independent: 'I' } as Record<string, PartyCode>)[party] ?? 'O';
}
