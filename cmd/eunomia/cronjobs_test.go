package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// cronJob is the part of a CronJob manifest this test reads.
type cronJob struct {
	Kind string `yaml:"kind"`
	Spec struct {
		JobTemplate struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Args []string `yaml:"args"`
						} `yaml:"containers"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		} `yaml:"jobTemplate"`
	} `yaml:"spec"`
}

// scheduled returns the job each CronJob under k8s/app runs: the last
// argument of its container.
func scheduled(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "k8s", "app", "*.yaml"))
	require.NoError(t, err, "listing the manifests")
	require.NotEmpty(t, files, "the manifests are found")
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // the repository's own manifests
		require.NoError(t, err, "reading %s", f)
		dec := yaml.NewDecoder(bytes.NewReader(b))
		for {
			var cj cronJob
			err := dec.Decode(&cj)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err, "decoding %s", f)
			if cj.Kind != "CronJob" {
				continue
			}
			cs := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
			require.Len(t, cs, 1, "a CronJob in %s has one container", f)
			args := cs[0].Args
			require.NotEmpty(t, args, "a CronJob in %s has args", f)
			out = append(out, args[len(args)-1])
		}
	}
	return out
}

// A job with no CronJob never runs, and one with no limit can stop running
// without the status page saying so.
func TestJobs_EachHasACronJobAndALimit(t *testing.T) {
	t.Parallel()
	js, err := buildJobs(deps{})
	require.NoError(t, err, "building the jobs")
	cron := scheduled(t)
	names := make([]string, 0, len(js))
	for _, j := range js {
		names = append(names, j.Name())
		assert.Contains(t, cron, j.Name(), "%s has a CronJob in k8s/app", j.Name())
		assert.Contains(t, jobMaxAge, j.Name(), "%s has a limit in jobMaxAge", j.Name())
	}
	for _, c := range cron {
		assert.True(t, slices.Contains(names, c), "the CronJob for %s runs a registered job", c)
	}
	for name := range jobMaxAge {
		assert.True(t, slices.Contains(names, name), "the limit for %s is for a registered job", name)
	}
}
