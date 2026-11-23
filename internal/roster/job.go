package roster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/legislators"
	"github.com/tcamick/eunomia/internal/store"
)

// Store is what the roster job needs of the store; *store.Store implements it.
type Store interface {
	RosterState(ctx context.Context) (store.RosterState, error)
	ApplyRoster(ctx context.Context, p store.RosterPlan) error
	SourceState(ctx context.Context, source string) (store.SourceState, bool, error)
	RecordSourceCheck(ctx context.Context, st store.SourceState, changed bool) error
}

// Source fetches the three files; *legislators.Client implements it.
type Source interface {
	Fetch(ctx context.Context, prev legislators.Validators) (*legislators.Snapshot, legislators.Validators, bool, error)
}

// Job is `eunomia job roster`: fetch the congress-legislators files, and
// write what changed. An unchanged source costs three conditional requests
// and writes no roster row.
type Job struct {
	src     Source
	st      Store
	base    string // the dataset's URL, recorded as each row's source
	now     func() time.Time
	changes *prometheus.CounterVec
}

// Kinds are every change kind the roster records: the values of the changes
// metric's kind label.
var Kinds = []string{KindArrived, KindLeft, KindPartyChanged, KindOfficeChanged, KindLeadershipGained,
	KindLeadershipLost, KindChairChanged, KindSeatJoined, KindSeatLeft, KindRecordCorrected}

// NewJob returns the roster job. base is the dataset's URL (the files'
// directory), which every row it writes names as its source. reg, if not
// nil, gets eunomia_roster_changes_total{kind}, which the job pod pushes.
func NewJob(src Source, st Store, base string, now func() time.Time, reg prometheus.Registerer) (*Job, error) {
	changes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "eunomia_roster_changes_total",
		Help: "Changes the roster job recorded, by kind: " + strings.Join(Kinds, ", ") + ".",
	}, []string{"kind"})
	for _, k := range Kinds {
		changes.WithLabelValues(k) // every kind is present, at zero, from the first push
	}
	if reg != nil {
		if err := reg.Register(changes); err != nil {
			return nil, fmt.Errorf("register roster metrics: %w", err)
		}
	}
	return &Job{src: src, st: st, base: base, now: now, changes: changes}, nil
}

// Name is the job's registered name.
func (*Job) Name() string { return JobName }

// JobName is the roster job's registered name.
const JobName = "roster"

var sourceNames = []string{legislators.SourceLegislators, legislators.SourceCommittees, legislators.SourceMembership}

// Run fetches and applies the roster.
func (j *Job) Run(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	var prev legislators.Validators
	vals := []*fetch.Validators{&prev.Members, &prev.Committees, &prev.Membership}
	for i, name := range sourceNames {
		st, ok, err := j.st.SourceState(ctx, name)
		if err != nil {
			return err
		}
		if ok {
			*vals[i] = fetch.Validators{ETag: st.ETag, LastModified: st.LastModified}
		}
	}

	snap, next, unchanged, err := j.src.Fetch(ctx, prev)
	if err != nil {
		return err
	}
	now := j.now()
	if unchanged {
		log.Info().Msg("Found the roster unchanged")
		return j.recordChecks(ctx, prev, now, false)
	}

	want, err := Desired(snap)
	if err != nil {
		return err
	}
	have, err := j.st.RosterState(ctx)
	if err != nil {
		return err
	}
	plan := Diff(have, want, Today(now))
	plan.At, plan.Source = now, j.base
	if err := j.st.ApplyRoster(ctx, plan); err != nil {
		return err
	}
	if err := j.recordChecks(ctx, next, now, true); err != nil {
		return err
	}
	for _, c := range plan.Changes {
		j.changes.WithLabelValues(c.Kind).Inc()
	}
	log.Info().Int("members", len(snap.Legislators)).Int("people", len(plan.People)).Int("terms", len(plan.Terms)).
		Int("seats", len(plan.Seats)+len(plan.UnlistSeats)).Int("changed", len(plan.Changes)).Msg("Applied the roster")
	return nil
}

func (j *Job) recordChecks(ctx context.Context, v legislators.Validators, at time.Time, changed bool) error {
	urls := map[string]string{
		legislators.SourceLegislators: j.base + "legislators-current.json",
		legislators.SourceCommittees:  j.base + "committees-current.json",
		legislators.SourceMembership:  j.base + "committee-membership-current.json",
	}
	for i, fv := range []fetch.Validators{v.Members, v.Committees, v.Membership} {
		name := sourceNames[i]
		st := store.SourceState{Source: name, URL: urls[name], ETag: fv.ETag, LastModified: fv.LastModified, CheckedAt: at}
		if err := j.st.RecordSourceCheck(ctx, st, changed); err != nil {
			return fmt.Errorf("record %s check: %w", name, err)
		}
	}
	return nil
}
