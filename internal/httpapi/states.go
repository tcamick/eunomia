package httpapi

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/tcamick/eunomia/internal/congress"
	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

// districtShapesName credits the Census Bureau for the district maps, as its
// terms ask (ADR 0006).
const districtShapesName = "District boundaries: U.S. Census Bureau, 119th Congress"

// StateVacancy is an empty seat on a state's page, with who last held it
// when the roster saw them leave (store.LastHolders).
type StateVacancy struct {
	Body       string            `json:"body"`
	District   *int              `json:"district,omitempty"`
	Voting     bool              `json:"voting"`
	LastHolder *store.LastHolder `json:"lastHolder,omitempty"`
}

// StateResponse is one state's page.
type StateResponse struct {
	State     congress.StateSummary `json:"state"`
	Senators  []Member              `json:"senators"`
	House     []Member              `json:"house"`
	Vacancies []StateVacancy        `json:"vacancies"`
	// DistrictMap is the Congress whose district lines the state's map
	// draws; 0 when it has none (DC and the territories).
	DistrictMap int          `json:"districtMap,omitempty"`
	Sources     []SourceLine `json:"sources"`
}

func (a *api) statesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ms, err := a.roster.Sittings(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"states": congress.States(ms), keySources: a.rosterSource(ctx)})
}

func (a *api) stateHandler(w http.ResponseWriter, r *http.Request) {
	j, ok := geo.Lookup(r.PathValue("code"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	ms, err := a.roster.Sittings(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	holders, err := a.roster.LastHolders(ctx, j.Code)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := StateResponse{Senators: []Member{}, House: []Member{}, Vacancies: []StateVacancy{}}
	for _, s := range congress.States(ms) {
		if s.Code == j.Code {
			resp.State = s
		}
	}
	for _, m := range ms {
		if m.State != j.Code {
			continue
		}
		if m.Body == congress.Senate {
			resp.Senators = append(resp.Senators, member(m))
		} else {
			resp.House = append(resp.House, member(m))
		}
	}
	sort.SliceStable(resp.House, func(x, y int) bool { return districtOf(resp.House[x]) < districtOf(resp.House[y]) })
	for _, c := range congress.Summarize(ms, nil) {
		for _, v := range c.Vacancies {
			if v.State != j.Code {
				continue
			}
			sv := StateVacancy{Body: c.Body, District: v.District, Voting: v.Voting}
			if v.District != nil {
				if h, ok := holders["house/"+j.Code+"/"+strconv.Itoa(*v.District)]; ok {
					sv.LastHolder = &h
				}
			}
			resp.Vacancies = append(resp.Vacancies, sv)
		}
	}
	resp.Sources = a.rosterSource(ctx)
	resp.Sources = append(resp.Sources, SourceLine{Name: "Census Bureau, 2020 apportionment", URL: geo.ApportionmentSource,
		CheckedAt: geo.ApportionmentChecked})
	if j.Voting() {
		resp.DistrictMap = geo.DistrictShapesCongress
		resp.Sources = append(resp.Sources, SourceLine{Name: districtShapesName, URL: geo.DistrictShapesSource,
			CheckedAt: geo.DistrictShapesChecked})
	}
	writeJSON(w, resp)
}

func districtOf(m Member) int {
	if m.District == nil {
		return 0
	}
	return *m.District
}
