package test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Tereius/gcp-hosted-github-runner/pkg"
	"github.com/stretchr/testify/assert"
)

var PORT = 9999

var scaler *pkg.Autoscaler

const PROJECT_ID = "my-gcp-project-id"
const REGION = "us-east1"
const ZONE = "us-east1-c"
const GIT_HUB_ORG = "Privatehive"
const TEST_REPO = "Privatehive/runner-test"
const TEST_REPO_KEY = "repository-" + TEST_REPO
const SOURCE_QUERY_PARAM_NAME = "src"

func init() {

	scaler = pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteWebhook:     "/webhook",
		RouteCreateVm:    "/create",
		RouteDeleteVm:    "/delete",
		ProjectId:        PROJECT_ID,
		Zone:             ZONE,
		TaskQueue:        "projects/" + PROJECT_ID + "/locations/" + REGION + "/queues/autoscaler-callback-queue",
		InstanceTemplate: "projects/" + PROJECT_ID + "/global/instanceTemplates/ephemeral-github-runner",
		SecretVersion:    "projects/" + PROJECT_ID + "/secrets/github-pat-token/versions/latest",
		RunnerPrefix:     "runner",
		RunnerGroupId:    1,
		RunnerLabels:     []string{"self-hosted"},
		SourceQueryParam: SOURCE_QUERY_PARAM_NAME,
		RegisteredSources: map[string]pkg.Source{
			TEST_REPO_KEY: {
				Name:       TEST_REPO,
				SourceType: pkg.TypeRepository,
				Secret:     "It's a Secret to Everybody",
			},
		},
	})
	go scaler.Srv(PORT)
	time.Sleep(1 * time.Second)
}

func TestWebhookSignature(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://127.0.0.1:%d/webhook?%s=%s", PORT, SOURCE_QUERY_PARAM_NAME, url.QueryEscape(TEST_REPO_KEY)), strings.NewReader("Hello, World!"))
	req.Header.Add("x-hub-signature-256", "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17")
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGenerateRunnerJitConfig(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	jitConfig, err := scaler.GenerateRunnerJitConfig(ctx, fmt.Sprintf(pkg.RUNNER_REPO_JIT_CONFIG_ENDPOINT, TEST_REPO), "unit_test_runner_"+pkg.RandStringRunes(10), 1, []string{"self-hosted"})
	assert.Nil(t, err)
	assert.NotEmpty(t, jitConfig)
}

func TestGetMagicLabelValue(t *testing.T) {

	job := pkg.Job{
		Labels: []string{"test", "@foo:bar", "@machine:test"},
	}
	result := job.GetMagicLabelValue(pkg.MagicLabelMachine)
	assert.NotNil(t, result)
	assert.Equal(t, "test", *result)
}

func TestCreateCallbackTask(t *testing.T) {

	job := pkg.Job{
		Id:     rand.Int63n(math.MaxInt64),
		Labels: []string{"test", "@foo:bar", "@machine:test"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := scaler.DeleteCallbackTask(ctx, job)
	assert.Nil(t, err)
}

func TestHasAllLabels(t *testing.T) {

	job := pkg.Job{
		Labels: []string{"test", "@foo:bar", "@machine:test"},
	}
	result, missing := job.HasAllLabels([]string{"test"})
	assert.True(t, result)
	assert.Empty(t, missing)
	result, missing = job.HasAllLabels([]string{"test", "foo"})
	assert.False(t, result)
	assert.NotEmpty(t, missing)
	assert.Len(t, missing, 1)
}
