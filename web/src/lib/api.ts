// The JSON API's shapes and a fetch helper. Every call is same-origin.

export interface JobStatus {
  name: string;
  lastRun: string;
  lastOk?: string;
  ok: boolean;
  durationMs: number;
  message?: string;
}

export interface Status {
  version: string;
  now: string;
  jobs: JobStatus[];
  jobsError?: string;
}

export interface SourceLine {
  name: string;
  url: string;
  checkedAt?: string;
  changedAt?: string;
}

export interface Sitting {
  id: string;
  name: string;
  lastName: string;
  body: 'senate' | 'house';
  state: string;
  district?: number;
  class?: number;
  stateRank?: string;
  party?: string;
  caucus?: string;
  start: string;
  end: string;
  how?: string;
  portrait: boolean;
}

export interface Member extends Sitting {
  partyCode: string;
  stateName: string;
  voting: boolean;
}

export interface Role {
  person: string;
  name: string;
  party?: string;
  chamber: string;
  title: string;
  start: string;
  end?: string;
  /** When the source stopped listing the role while it was open (an instant). */
  unlistedOn?: string;
}

export interface PartyCount {
  party: string;
  caucus?: string;
  code: string;
  seats: number;
}

export interface Vacancy {
  state: string;
  stateName: string;
  district?: number;
  voting: boolean;
}

export interface Chamber {
  body: 'senate' | 'house';
  seats: number;
  filled: number;
  majority: number;
  parties: PartyCount[];
  vacancies: Vacancy[];
  delegates: Sitting[];
  leaders: Role[];
}

export interface CongressResponse {
  chambers: Chamber[];
  sources: SourceLine[];
  apportionment: SourceLine;
}

export interface Jurisdiction {
  code: string;
  name: string;
  kind: 'state' | 'district' | 'territory';
  houseSeats: number;
  senateSeats: number;
}

export interface MembersResponse {
  members: Member[];
  sources: SourceLine[];
  states: Jurisdiction[];
}

export interface Affiliation {
  start: string;
  end: string;
  party: string;
  caucus?: string;
}

export interface PersonTerm {
  office: string;
  body: 'senate' | 'house';
  state: string;
  stateName: string;
  district?: number;
  class?: number;
  start: string;
  end: string;
  party?: string;
  partyCode: string;
  caucus?: string;
  how?: string;
  stateRank?: string;
  url?: string;
  current: boolean;
  leftOn?: string;
  affiliations?: Affiliation[];
}

export interface PersonSeat {
  committee: string;
  name: string;
  parent?: string;
  chamber: string;
  side: 'majority' | 'minority';
  rank?: number;
  title?: string;
  /** When the source stopped listing the seat (an instant); absent while listed. */
  unlistedOn?: string;
}

export interface Person {
  id: string;
  name: string;
  first: string;
  last: string;
  birthday?: string;
  inCongress: boolean;
  portrait: boolean;
  terms: PersonTerm[];
  roles: Role[];
  seats: PersonSeat[];
  links: { label: string; url: string }[];
  sources: SourceLine[];
}

export interface CommitteeSummary {
  id: string;
  name: string;
  chamber: 'house' | 'senate' | 'joint';
  parent?: string;
  members: number;
  chair?: string;
  chairId?: string;
  ranking?: string;
  rankingId?: string;
  subcommittees: number;
  url?: string;
  jurisdiction?: string;
}

export interface CommitteeMember {
  person: string;
  name: string;
  party?: string;
  partyCode: string;
  state?: string;
  side: 'majority' | 'minority';
  rank?: number;
  title?: string;
  portrait: boolean;
}

export interface CommitteeResponse {
  committee: CommitteeSummary;
  parent?: CommitteeSummary | null;
  subcommittees: CommitteeSummary[];
  members: CommitteeMember[];
  sources: SourceLine[];
}

export interface SearchResult {
  kind: 'person' | 'state' | 'committee';
  id: string;
  label: string;
  detail: string;
  partyCode?: string;
}

/** One chamber's seats for one state (congress.Delegation). */
export interface Delegation {
  seats: number;
  vacant: number;
  /** A party code with the most filled seats, or 'even', 'vacant', 'none'; '' is party unknown. */
  lead: string;
  parties: PartyCount[];
}

export interface StateSummary {
  code: string;
  name: string;
  kind: 'state' | 'district' | 'territory';
  voting: boolean;
  senate: Delegation;
  house: Delegation;
}

export interface StatesResponse {
  states: StateSummary[];
  sources: SourceLine[];
}

export interface LastHolder {
  person: string;
  name: string;
  party: string;
  /** The date the source stopped listing them (YYYY-MM-DD). */
  leftOn: string;
}

export interface StateVacancy {
  body: 'senate' | 'house';
  district?: number;
  voting: boolean;
  lastHolder?: LastHolder;
}

export interface StateResponse {
  state: StateSummary;
  senators: Member[];
  house: Member[];
  vacancies: StateVacancy[];
  /** The Congress whose district lines the map draws; absent without a map. */
  districtMap?: number;
  /** The Congress the sitting House members belong to. */
  houseCongress?: number;
  sources: SourceLine[];
}

/** NotFound is a 404 from the API: the thing asked for does not exist. */
export class NotFound extends Error {}

/** getJSON fetches a path under /api and throws with the status on failure. */
export async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const resp = await fetch(path, { headers: { Accept: 'application/json' }, signal });
  if (resp.status === 404) throw new NotFound(path);
  if (!resp.ok) throw new Error(`the server answered ${resp.status}`);
  return (await resp.json()) as T;
}
