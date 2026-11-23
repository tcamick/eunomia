// The shapes `make geo` generates into lib/geo/ (docs/adr/0006). Paths are
// SVG path data in their map's own frame; a label is where a name or
// number can sit, the shape's centroid.

export interface StateShape {
  /** The postal code: "CA". */
  code: string;
  name: string;
  d: string;
  label: [number, number];
}

export interface DistrictShape {
  /** The district's number; 0 for a state's single at-large seat. */
  n: number;
  d: string;
  label: [number, number];
}

export interface DistrictMap {
  state: string;
  /** The Congress whose district lines these are. */
  congress: number;
  viewBox: string;
  districts: DistrictShape[];
}
